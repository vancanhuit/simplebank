package notification

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type fakeListenerConn struct {
	mu            sync.Mutex
	execErr       error
	execQueries   []string
	notifications chan fakeWaitResult
	closed        chan struct{}
	closeOnce     sync.Once
}

type fakeWaitResult struct {
	notification *pgconn.Notification
	err          error
}

func newFakeListenerConn() *fakeListenerConn {
	return &fakeListenerConn{
		notifications: make(chan fakeWaitResult, 8),
		closed:        make(chan struct{}),
	}
}

func (c *fakeListenerConn) Exec(_ context.Context, query string, _ ...any) (pgconn.CommandTag, error) {
	c.mu.Lock()
	c.execQueries = append(c.execQueries, query)
	c.mu.Unlock()
	return pgconn.CommandTag{}, c.execErr
}

func (c *fakeListenerConn) WaitForNotification(ctx context.Context) (*pgconn.Notification, error) {
	select {
	case result := <-c.notifications:
		return result.notification, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *fakeListenerConn) Close(context.Context) error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func (c *fakeListenerConn) queries() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.execQueries...)
}

func newTestListener(t *testing.T, hub *Hub, connections ...*fakeListenerConn) *Listener {
	t.Helper()

	config, err := pgx.ParseConfig("postgres://localhost/test")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}

	listener := NewListener(config, hub)
	var mu sync.Mutex
	next := 0
	listener.connect = func(context.Context, *pgx.ConnConfig) (listenerConn, error) {
		mu.Lock()
		defer mu.Unlock()
		if next >= len(connections) {
			return nil, errors.New("unexpected connect")
		}
		connection := connections[next]
		next++
		return connection, nil
	}
	return listener
}

func stopListener(t *testing.T, listener *Listener) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := listener.Stop(ctx); err != nil {
		t.Fatalf("stop listener: %v", err)
	}
}

func TestListenerStartListensBeforeReturning(t *testing.T) {
	connection := newFakeListenerConn()
	listener := newTestListener(t, NewHub(), connection)

	if err := listener.Start(t.Context()); err != nil {
		t.Fatalf("start listener: %v", err)
	}
	t.Cleanup(func() { stopListener(t, listener) })

	queries := connection.queries()
	if len(queries) != 1 || queries[0] != "LISTEN balance_notifications" {
		t.Fatalf("queries before Start returned = %q, want LISTEN balance_notifications", queries)
	}
	if err := listener.Start(t.Context()); err == nil {
		t.Fatal("second Start succeeded")
	}
}

func TestListenerPublishesDecodedNotification(t *testing.T) {
	hub := NewHub()
	subscriber, unsubscribe := hub.Subscribe("alice")
	t.Cleanup(unsubscribe)
	connection := newFakeListenerConn()
	listener := newTestListener(t, hub, connection)

	if err := listener.Start(t.Context()); err != nil {
		t.Fatalf("start listener: %v", err)
	}
	t.Cleanup(func() { stopListener(t, listener) })

	id := uuid.New()
	connection.notifications <- fakeWaitResult{notification: &pgconn.Notification{
		Channel: "balance_notifications",
		Payload: `{"id":"` + id.String() + `","owner":"alice"}`,
	}}

	select {
	case got := <-subscriber:
		if got != id {
			t.Fatalf("subscriber received %s, want %s", got, id)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive notification")
	}
}

func TestListenerIgnoresMalformedPayload(t *testing.T) {
	hub := NewHub()
	subscriber, unsubscribe := hub.Subscribe("alice")
	t.Cleanup(unsubscribe)
	connection := newFakeListenerConn()
	listener := newTestListener(t, hub, connection)

	if err := listener.Start(t.Context()); err != nil {
		t.Fatalf("start listener: %v", err)
	}
	t.Cleanup(func() { stopListener(t, listener) })

	connection.notifications <- fakeWaitResult{notification: &pgconn.Notification{Payload: `{`}}
	connection.notifications <- fakeWaitResult{notification: &pgconn.Notification{Payload: `{"id":"00000000-0000-0000-0000-000000000000","owner":"alice"}`}}
	connection.notifications <- fakeWaitResult{notification: &pgconn.Notification{Payload: `{"id":"` + uuid.NewString() + `","owner":""}`}}

	select {
	case got := <-subscriber:
		t.Fatalf("subscriber received malformed notification %s", got)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestListenerReconnectsAfterWaitFailure(t *testing.T) {
	hub := NewHub()
	subscriber, unsubscribe := hub.Subscribe("alice")
	t.Cleanup(unsubscribe)
	first := newFakeListenerConn()
	second := newFakeListenerConn()
	listener := newTestListener(t, hub, first, second)
	listener.sleep = func(context.Context, time.Duration) error { return nil }

	if err := listener.Start(t.Context()); err != nil {
		t.Fatalf("start listener: %v", err)
	}
	t.Cleanup(func() { stopListener(t, listener) })

	first.notifications <- fakeWaitResult{err: errors.New("connection lost")}
	id := uuid.New()
	second.notifications <- fakeWaitResult{notification: &pgconn.Notification{
		Payload: `{"id":"` + id.String() + `","owner":"alice"}`,
	}}

	select {
	case got := <-subscriber:
		if got != id {
			t.Fatalf("subscriber received %s, want %s", got, id)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive notification after reconnect")
	}
	if got := second.queries(); len(got) != 1 || got[0] != "LISTEN balance_notifications" {
		t.Fatalf("reconnect queries = %q, want LISTEN balance_notifications", got)
	}
}

func TestListenerBackoffIsBounded(t *testing.T) {
	connection := newFakeListenerConn()
	listener := newTestListener(t, NewHub(), connection)

	var mu sync.Mutex
	var delays []time.Duration
	reachedBound := make(chan struct{})
	listener.sleep = func(ctx context.Context, delay time.Duration) error {
		mu.Lock()
		delays = append(delays, delay)
		count := len(delays)
		mu.Unlock()
		if count == 8 {
			close(reachedBound)
			<-ctx.Done()
			return ctx.Err()
		}
		return nil
	}

	// The initial connection uses a successful one-shot seam before reconnects fail.
	initial := true
	listener.connect = func(context.Context, *pgx.ConnConfig) (listenerConn, error) {
		if initial {
			initial = false
			return connection, nil
		}
		return nil, errors.New("database unavailable")
	}
	if err := listener.Start(t.Context()); err != nil {
		t.Fatalf("start listener: %v", err)
	}
	connection.notifications <- fakeWaitResult{err: errors.New("connection lost")}

	select {
	case <-reachedBound:
	case <-time.After(time.Second):
		t.Fatal("listener did not reach bounded backoff")
	}
	stopListener(t, listener)

	mu.Lock()
	got := append([]time.Duration(nil), delays...)
	mu.Unlock()
	want := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond, 800 * time.Millisecond, 1600 * time.Millisecond, 3200 * time.Millisecond, 5 * time.Second, 5 * time.Second}
	if len(got) != len(want) {
		t.Fatalf("backoff delays = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("backoff delay %d = %s, want %s", i, got[i], want[i])
		}
	}
}

func TestListenerStopCancelsWait(t *testing.T) {
	connection := newFakeListenerConn()
	listener := newTestListener(t, NewHub(), connection)
	if err := listener.Start(t.Context()); err != nil {
		t.Fatalf("start listener: %v", err)
	}

	stopListener(t, listener)

	select {
	case <-connection.closed:
	default:
		t.Fatal("Stop returned before the listener connection closed")
	}
}
