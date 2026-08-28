package notification

import (
	"sync"
	"uuid"
)

const subscriberBuffer = 16

type Hub struct {
	mu          sync.Mutex
	nextID      uint64
	subscribers map[string]map[uint64]chan uuid.UUID
}

func NewHub() *Hub {
	return &Hub{
		subscribers: make(map[string]map[uint64]chan uuid.UUID),
	}
}

func (h *Hub) Subscribe(owner string) (<-chan uuid.UUID, func()) {
	h.mu.Lock()
	id := h.nextID
	h.nextID++
	subscriber := make(chan uuid.UUID, subscriberBuffer)
	if h.subscribers[owner] == nil {
		h.subscribers[owner] = make(map[uint64]chan uuid.UUID)
	}
	h.subscribers[owner][id] = subscriber
	h.mu.Unlock()

	return subscriber, func() {
		h.mu.Lock()
		defer h.mu.Unlock()

		ownerSubscribers := h.subscribers[owner]
		delete(ownerSubscribers, id)
		if len(ownerSubscribers) == 0 {
			delete(h.subscribers, owner)
		}
	}
}

func (h *Hub) Publish(owner string, id uuid.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, subscriber := range h.subscribers[owner] {
		select {
		case subscriber <- id:
		default:
		}
	}
}
