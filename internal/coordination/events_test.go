package coordination

import (
	"testing"
	"time"
)

func TestNewEventBus(t *testing.T) {
	eb := NewEventBus()

	if eb == nil {
		t.Fatal("expected event bus")
	}
	if eb.subscribers == nil {
		t.Error("expected subscribers map to be initialized")
	}
}

func TestEventBus_Subscribe(t *testing.T) {
	eb := NewEventBus()

	ch := eb.Subscribe()
	if ch == nil {
		t.Fatal("expected channel")
	}

	if eb.SubscriberCount() != 1 {
		t.Errorf("expected 1 subscriber, got %d", eb.SubscriberCount())
	}
}

func TestEventBus_Unsubscribe(t *testing.T) {
	eb := NewEventBus()

	ch := eb.Subscribe()
	eb.Unsubscribe(ch)

	// Give goroutine time to clean up
	time.Sleep(10 * time.Millisecond)

	if eb.SubscriberCount() != 0 {
		t.Errorf("expected 0 subscribers after unsubscribe, got %d", eb.SubscriberCount())
	}
}

func TestEventBus_Publish(t *testing.T) {
	eb := NewEventBus()

	ch := eb.Subscribe()

	event := Event{
		Type:   EventTypeTaskCreated,
		Repo:   "my-repo",
		TaskID: "task-1",
	}

	eb.Publish(event)

	select {
	case received := <-ch:
		if received.Type != EventTypeTaskCreated {
			t.Errorf("expected type %s, got %s", EventTypeTaskCreated, received.Type)
		}
		if received.Repo != "my-repo" {
			t.Errorf("expected repo 'my-repo', got '%s'", received.Repo)
		}
		if received.ID == "" {
			t.Error("expected ID to be set")
		}
		if received.Timestamp.IsZero() {
			t.Error("expected timestamp to be set")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for event")
	}
}

func TestEventBus_PublishMultipleSubscribers(t *testing.T) {
	eb := NewEventBus()

	ch1 := eb.Subscribe()
	ch2 := eb.Subscribe()
	ch3 := eb.Subscribe()

	if eb.SubscriberCount() != 3 {
		t.Errorf("expected 3 subscribers, got %d", eb.SubscriberCount())
	}

	event := Event{
		Type: EventTypeNodeRegistered,
	}

	eb.Publish(event)

	// All subscribers should receive the event
	for i, ch := range []chan Event{ch1, ch2, ch3} {
		select {
		case <-ch:
			// Good
		case <-time.After(100 * time.Millisecond):
			t.Errorf("subscriber %d did not receive event", i+1)
		}
	}
}

func TestEventBus_PublishFullChannel(t *testing.T) {
	eb := NewEventBus()

	ch := eb.Subscribe()

	// Fill the channel
	for i := 0; i < 100; i++ {
		eb.Publish(Event{Type: EventTypeTaskCreated})
	}

	// This should not block even if channel is full
	done := make(chan bool)
	go func() {
		eb.Publish(Event{Type: EventTypeTaskCreated})
		done <- true
	}()

	select {
	case <-done:
		// Good - publish didn't block
	case <-time.After(100 * time.Millisecond):
		t.Error("publish blocked on full channel")
	}

	// Clean up by draining
	close(done)
	for range ch {
		// drain
		break
	}
}

func TestEventBus_PublishWithID(t *testing.T) {
	eb := NewEventBus()

	ch := eb.Subscribe()

	event := Event{
		ID:        "custom-id",
		Type:      EventTypeAgentSpawned,
		Timestamp: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
	}

	eb.Publish(event)

	select {
	case received := <-ch:
		// ID should be preserved if already set
		if received.ID != "custom-id" {
			t.Errorf("expected ID 'custom-id', got '%s'", received.ID)
		}
		// Timestamp should be preserved if already set
		if received.Timestamp.Year() != 2024 {
			t.Error("expected timestamp to be preserved")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for event")
	}
}

func TestEventBus_SubscriberCount(t *testing.T) {
	eb := NewEventBus()

	if eb.SubscriberCount() != 0 {
		t.Errorf("expected 0 subscribers initially, got %d", eb.SubscriberCount())
	}

	ch1 := eb.Subscribe()
	if eb.SubscriberCount() != 1 {
		t.Errorf("expected 1 subscriber, got %d", eb.SubscriberCount())
	}

	ch2 := eb.Subscribe()
	if eb.SubscriberCount() != 2 {
		t.Errorf("expected 2 subscribers, got %d", eb.SubscriberCount())
	}

	eb.Unsubscribe(ch1)
	time.Sleep(10 * time.Millisecond)
	if eb.SubscriberCount() != 1 {
		t.Errorf("expected 1 subscriber after unsubscribe, got %d", eb.SubscriberCount())
	}

	eb.Unsubscribe(ch2)
	time.Sleep(10 * time.Millisecond)
	if eb.SubscriberCount() != 0 {
		t.Errorf("expected 0 subscribers after all unsubscribe, got %d", eb.SubscriberCount())
	}
}

func TestEventBus_ConcurrentPublish(t *testing.T) {
	eb := NewEventBus()

	ch := eb.Subscribe()

	// Start multiple publishers
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(n int) {
			for j := 0; j < 10; j++ {
				eb.Publish(Event{Type: EventTypeTaskCreated})
			}
			done <- true
		}(i)
	}

	// Wait for all publishers to finish
	for i := 0; i < 10; i++ {
		<-done
	}

	// Drain received events
	received := 0
	for {
		select {
		case <-ch:
			received++
		case <-time.After(50 * time.Millisecond):
			// No more events
			goto done
		}
	}
done:

	// Should have received at least some events (may drop some due to full channel)
	if received == 0 {
		t.Error("expected to receive at least some events")
	}
}
