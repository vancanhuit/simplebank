package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	listenQuery    = "LISTEN balance_notifications"
	initialBackoff = 100 * time.Millisecond
	maximumBackoff = 5 * time.Second
)

type listenerConn interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	WaitForNotification(context.Context) (*pgconn.Notification, error)
	Close(context.Context) error
}

type connectFunc func(context.Context, *pgx.ConnConfig) (listenerConn, error)

type sleepFunc func(context.Context, time.Duration) error

type Listener struct {
	config  *pgx.ConnConfig
	hub     *Hub
	connect connectFunc
	sleep   sleepFunc

	mu      sync.Mutex
	started bool
	cancel  context.CancelFunc
	done    chan struct{}
}

func NewListener(config *pgx.ConnConfig, hub *Hub) *Listener {
	return &Listener{
		config: config.Copy(),
		hub:    hub,
		connect: func(ctx context.Context, config *pgx.ConnConfig) (listenerConn, error) {
			return pgx.ConnectConfig(ctx, config)
		},
		sleep: sleepContext,
	}
}

func (l *Listener) Start(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.started {
		return errors.New("notification listener already started")
	}

	listenerCtx, cancel := context.WithCancel(ctx)
	connection, err := l.connect(listenerCtx, l.config)
	if err != nil {
		cancel()
		return fmt.Errorf("connect notification listener: %w", err)
	}
	if _, err := connection.Exec(listenerCtx, listenQuery); err != nil {
		_ = connection.Close(listenerCtx)
		cancel()
		return fmt.Errorf("listen for balance notifications: %w", err)
	}

	l.started = true
	l.cancel = cancel
	l.done = make(chan struct{})
	go l.run(listenerCtx, connection)
	return nil
}

func (l *Listener) Stop(ctx context.Context) error {
	l.mu.Lock()
	if !l.started {
		l.mu.Unlock()
		return nil
	}
	cancel := l.cancel
	done := l.done
	l.mu.Unlock()

	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (l *Listener) run(ctx context.Context, connection listenerConn) {
	defer close(l.done)

	for {
		notification, err := connection.WaitForNotification(ctx)
		if err == nil {
			l.publish(notification)
			continue
		}

		_ = connection.Close(ctx)
		if ctx.Err() != nil {
			return
		}

		var ok bool
		connection, ok = l.reconnect(ctx)
		if !ok {
			return
		}
	}
}

func (l *Listener) reconnect(ctx context.Context) (listenerConn, bool) {
	backoff := initialBackoff
	for {
		if err := l.sleep(ctx, backoff); err != nil {
			return nil, false
		}

		connection, err := l.connect(ctx, l.config)
		if err == nil {
			if _, err = connection.Exec(ctx, listenQuery); err == nil {
				return connection, true
			}
			_ = connection.Close(ctx)
		}

		backoff = min(backoff*2, maximumBackoff)
	}
}

func (l *Listener) publish(notification *pgconn.Notification) {
	if notification == nil {
		return
	}

	var message struct {
		ID    uuid.UUID `json:"id"`
		Owner string    `json:"owner"`
	}
	if err := json.Unmarshal([]byte(notification.Payload), &message); err != nil || message.ID == uuid.Nil || message.Owner == "" {
		return
	}
	l.hub.Publish(message.Owner, message.ID)
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
