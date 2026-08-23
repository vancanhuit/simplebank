package notification

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestHubPublishesOnlyToOwner(t *testing.T) {
	hub := NewHub()
	alice, unsubscribeAlice := hub.Subscribe("alice")
	t.Cleanup(unsubscribeAlice)
	bob, unsubscribeBob := hub.Subscribe("bob")
	t.Cleanup(unsubscribeBob)

	id := uuid.New()
	hub.Publish("alice", id)
	if got := <-alice; got != id {
		t.Fatalf("alice received %s, want %s", got, id)
	}
	select {
	case got := <-bob:
		t.Fatalf("bob received alice event %s", got)
	default:
	}
}

func TestHubSlowSubscriberDoesNotBlock(t *testing.T) {
	hub := NewHub()
	subscriber, unsubscribe := hub.Subscribe("alice")
	t.Cleanup(unsubscribe)

	const wantBuffered = 16
	for range wantBuffered {
		hub.Publish("alice", uuid.New())
	}

	published := make(chan struct{})
	go func() {
		hub.Publish("alice", uuid.New())
		close(published)
	}()

	select {
	case <-published:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("publish blocked on a slow subscriber")
	}

	if got := len(subscriber); got != wantBuffered {
		t.Fatalf("subscriber buffered %d events, want %d", got, wantBuffered)
	}
}

func TestHubUnsubscribeRemovesSubscriber(t *testing.T) {
	hub := NewHub()
	subscriber, unsubscribe := hub.Subscribe("alice")
	unsubscribe()
	unsubscribe()

	hub.Publish("alice", uuid.New())
	select {
	case got := <-subscriber:
		t.Fatalf("unsubscribed subscriber received event %s", got)
	default:
	}
}

func TestHubConcurrentPublishAndUnsubscribe(t *testing.T) {
	hub := NewHub()
	const iterations = 100

	var wg sync.WaitGroup
	for range iterations {
		_, unsubscribe := hub.Subscribe("alice")
		wg.Add(2)
		go func() {
			defer wg.Done()
			for range iterations {
				hub.Publish("alice", uuid.New())
			}
		}()
		go func() {
			defer wg.Done()
			unsubscribe()
			unsubscribe()
		}()
	}
	wg.Wait()
}
