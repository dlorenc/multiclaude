package coordination

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// EventBus provides pub/sub for coordination events.
type EventBus struct {
	subscribers map[chan Event]struct{}
	mu          sync.RWMutex
}

// NewEventBus creates a new event bus.
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[chan Event]struct{}),
	}
}

// Subscribe creates a new subscription channel for events.
func (eb *EventBus) Subscribe() chan Event {
	ch := make(chan Event, 100)

	eb.mu.Lock()
	eb.subscribers[ch] = struct{}{}
	eb.mu.Unlock()

	return ch
}

// Unsubscribe removes a subscription channel.
func (eb *EventBus) Unsubscribe(ch chan Event) {
	eb.mu.Lock()
	delete(eb.subscribers, ch)
	eb.mu.Unlock()

	// Drain and close the channel
	go func() {
		for range ch {
			// Drain remaining events
		}
	}()
	close(ch)
}

// Publish sends an event to all subscribers.
func (eb *EventBus) Publish(event Event) {
	// Ensure event has ID and timestamp
	if event.ID == "" {
		event.ID = uuid.New().String()[:8]
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	eb.mu.RLock()
	defer eb.mu.RUnlock()

	for ch := range eb.subscribers {
		select {
		case ch <- event:
		default:
			// Subscriber channel is full, skip
		}
	}
}

// SubscriberCount returns the number of active subscribers.
func (eb *EventBus) SubscriberCount() int {
	eb.mu.RLock()
	defer eb.mu.RUnlock()
	return len(eb.subscribers)
}
