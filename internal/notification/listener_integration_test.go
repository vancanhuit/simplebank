//go:build integration

package notification

import (
	"context"
	"os"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
)

func TestListenerCrossReplicaDelivery(t *testing.T) {
	dsn := os.Getenv("DB_SOURCE")
	if dsn == "" {
		dsn = "postgres://simplebank_test:simplebank_test@localhost:5433/simplebank_test?sslmode=disable"
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse database config: %v", err)
	}

	publisher, err := pgx.ConnectConfig(t.Context(), config.Copy())
	if err != nil {
		t.Fatalf("connect publisher: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := publisher.Close(ctx); err != nil {
			t.Errorf("close publisher: %v", err)
		}
	})

	hubs := []*Hub{NewHub(), NewHub()}
	listeners := []*Listener{
		NewListener(config.Copy(), hubs[0]),
		NewListener(config.Copy(), hubs[1]),
	}
	subscribers := make([]<-chan uuid.UUID, len(hubs))
	for i, hub := range hubs {
		var unsubscribe func()
		subscribers[i], unsubscribe = hub.Subscribe("alice")
		t.Cleanup(unsubscribe)
		if err := listeners[i].Start(t.Context()); err != nil {
			t.Fatalf("start listener %d: %v", i, err)
		}
		listener := listeners[i]
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := listener.Stop(ctx); err != nil {
				t.Errorf("stop listener: %v", err)
			}
		})
	}

	id := uuid.New()
	_, err = publisher.Exec(t.Context(), `SELECT pg_notify(
		'balance_notifications',
		json_build_object('id', $1::uuid, 'owner', $2::text)::text
	)`, id, "alice")
	if err != nil {
		t.Fatalf("publish notification: %v", err)
	}

	for i, subscriber := range subscribers {
		select {
		case got := <-subscriber:
			if got != id {
				t.Fatalf("listener %d received %s, want %s", i, got, id)
			}
		case <-time.After(time.Second):
			t.Fatalf("listener %d did not receive notification", i)
		}
	}
}
