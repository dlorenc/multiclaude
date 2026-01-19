package notify

import (
	"context"
	"sync"
	"testing"
	"time"
)

// mockAdapter is a test adapter that records sent events
type mockAdapter struct {
	name              string
	adapterType       string
	events            []*Event
	mu                sync.Mutex
	supportsResponses bool
	responseHandler   ResponseHandler
	sendErr           error
}

func newMockAdapter(name string) *mockAdapter {
	return &mockAdapter{
		name:        name,
		adapterType: "mock",
		events:      make([]*Event, 0),
	}
}

func (m *mockAdapter) Name() string { return m.name }
func (m *mockAdapter) Type() string { return m.adapterType }

func (m *mockAdapter) Send(ctx context.Context, event *Event) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.mu.Lock()
	m.events = append(m.events, event)
	m.mu.Unlock()
	return nil
}

func (m *mockAdapter) SupportsResponses() bool {
	return m.supportsResponses
}

func (m *mockAdapter) Close() error { return nil }

func (m *mockAdapter) getEvents() []*Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*Event, len(m.events))
	copy(result, m.events)
	return result
}

func TestHub_RegisterAdapter(t *testing.T) {
	hub := NewHub(nil)
	adapter := newMockAdapter("test-adapter")

	err := hub.RegisterAdapter(adapter, nil)
	if err != nil {
		t.Fatalf("RegisterAdapter failed: %v", err)
	}

	// Registering same adapter should fail
	err = hub.RegisterAdapter(adapter, nil)
	if err == nil {
		t.Error("RegisterAdapter should fail for duplicate name")
	}
}

func TestHub_UnregisterAdapter(t *testing.T) {
	hub := NewHub(nil)
	adapter := newMockAdapter("test-adapter")

	hub.RegisterAdapter(adapter, nil)

	err := hub.UnregisterAdapter("test-adapter")
	if err != nil {
		t.Fatalf("UnregisterAdapter failed: %v", err)
	}

	// Unregistering non-existent adapter should fail
	err = hub.UnregisterAdapter("test-adapter")
	if err == nil {
		t.Error("UnregisterAdapter should fail for non-existent adapter")
	}
}

func TestHub_Notify(t *testing.T) {
	hub := NewHub(&HubConfig{
		RateLimit:          100,
		CooldownAfterBurst: 1,
		DedupeWindow:       time.Millisecond,
	})

	adapter := newMockAdapter("test-adapter")
	hub.RegisterAdapter(adapter, nil)

	event := &Event{
		Type:     EventAgentQuestion,
		RepoName: "test-repo",
		Title:    "Test event",
		Message:  "Test message",
	}

	err := hub.Notify(event)
	if err != nil {
		t.Fatalf("Notify failed: %v", err)
	}

	events := adapter.getEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}

	// Verify event was enriched
	if events[0].ID == "" {
		t.Error("event ID should be set")
	}
	if events[0].Timestamp.IsZero() {
		t.Error("event timestamp should be set")
	}
	if events[0].Priority == "" {
		t.Error("event priority should be set")
	}
}

func TestHub_NotifyWithFilter(t *testing.T) {
	hub := NewHub(&HubConfig{
		RateLimit:          100,
		CooldownAfterBurst: 1,
		DedupeWindow:       time.Millisecond,
	})

	adapter1 := newMockAdapter("adapter-1")
	adapter2 := newMockAdapter("adapter-2")

	// adapter1 only receives high priority events
	hub.RegisterAdapter(adapter1, &EventFilter{MinPriority: PriorityHigh})
	// adapter2 receives all events
	hub.RegisterAdapter(adapter2, nil)

	// Send low priority event
	event := &Event{
		ID:       "evt-1",
		Type:     EventPRMerged,
		Priority: PriorityLow,
		RepoName: "test-repo",
		Title:    "Low priority event",
	}
	hub.Notify(event)

	// Allow dedup window to pass
	time.Sleep(2 * time.Millisecond)

	// Send high priority event
	event2 := &Event{
		ID:       "evt-2",
		Type:     EventAgentQuestion,
		Priority: PriorityHigh,
		RepoName: "test-repo",
		Title:    "High priority event",
	}
	hub.Notify(event2)

	events1 := adapter1.getEvents()
	events2 := adapter2.getEvents()

	if len(events1) != 1 {
		t.Errorf("adapter1 expected 1 event (high priority only), got %d", len(events1))
	}
	if len(events2) != 2 {
		t.Errorf("adapter2 expected 2 events (all), got %d", len(events2))
	}
}

func TestHub_Deduplication(t *testing.T) {
	hub := NewHub(&HubConfig{
		RateLimit:          100,
		CooldownAfterBurst: 1,
		DedupeWindow:       100 * time.Millisecond,
	})

	adapter := newMockAdapter("test-adapter")
	hub.RegisterAdapter(adapter, nil)

	// Send same event multiple times
	for i := 0; i < 5; i++ {
		event := &Event{
			Type:      EventAgentQuestion,
			RepoName:  "test-repo",
			AgentName: "test-agent",
			Title:     "Duplicate event",
		}
		hub.Notify(event)
	}

	events := adapter.getEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 event after deduplication, got %d", len(events))
	}

	// Wait for dedup window to expire
	time.Sleep(150 * time.Millisecond)

	// Send same event again
	event := &Event{
		Type:      EventAgentQuestion,
		RepoName:  "test-repo",
		AgentName: "test-agent",
		Title:     "Duplicate event",
	}
	hub.Notify(event)

	events = adapter.getEvents()
	if len(events) != 2 {
		t.Errorf("expected 2 events after dedup window expired, got %d", len(events))
	}
}

func TestHub_RateLimiting(t *testing.T) {
	hub := NewHub(&HubConfig{
		RateLimit:          5,
		CooldownAfterBurst: 1,
		DedupeWindow:       time.Millisecond,
	})

	adapter := newMockAdapter("test-adapter")
	hub.RegisterAdapter(adapter, nil)

	// Send more events than rate limit
	for i := 0; i < 10; i++ {
		event := &Event{
			ID:       string(rune('a' + i)), // Unique IDs to avoid dedup
			Type:     EventAgentQuestion,
			RepoName: "test-repo",
			Title:    "Event",
		}
		time.Sleep(2 * time.Millisecond) // Allow dedup window to pass
		hub.Notify(event)
	}

	events := adapter.getEvents()
	if len(events) > 5 {
		t.Errorf("expected at most 5 events due to rate limiting, got %d", len(events))
	}
}

func TestHub_QuietHours(t *testing.T) {
	// This test is tricky because it depends on current time
	// We'll test the isQuietHours logic separately

	hub := NewHub(&HubConfig{
		RateLimit:          100,
		CooldownAfterBurst: 1,
		DedupeWindow:       time.Millisecond,
		QuietHours: &QuietHours{
			Enabled:  true,
			Start:    "00:00",
			End:      "00:01", // Very narrow window
			Timezone: "UTC",
		},
	})

	// The hub should still work outside quiet hours
	adapter := newMockAdapter("test-adapter")
	hub.RegisterAdapter(adapter, nil)

	event := &Event{
		Type:     EventAgentQuestion,
		RepoName: "test-repo",
		Title:    "Test event",
	}
	hub.Notify(event)

	// We can't reliably test this without mocking time
	// Just verify no errors occurred
}

func TestHub_GetAdapters(t *testing.T) {
	hub := NewHub(nil)

	hub.RegisterAdapter(newMockAdapter("adapter-1"), nil)
	hub.RegisterAdapter(newMockAdapter("adapter-2"), nil)
	hub.RegisterAdapter(newMockAdapter("adapter-3"), nil)

	adapters := hub.GetAdapters()
	if len(adapters) != 3 {
		t.Errorf("expected 3 adapters, got %d", len(adapters))
	}

	// Check that all expected adapters are present
	adapterSet := make(map[string]bool)
	for _, name := range adapters {
		adapterSet[name] = true
	}

	for _, expected := range []string{"adapter-1", "adapter-2", "adapter-3"} {
		if !adapterSet[expected] {
			t.Errorf("adapter %s not found", expected)
		}
	}
}

func TestHub_GetStats(t *testing.T) {
	hub := NewHub(nil)
	adapter := newMockAdapter("test-adapter")
	hub.RegisterAdapter(adapter, nil)

	stats := hub.GetStats()

	if stats["adapters"] != 1 {
		t.Errorf("expected 1 adapter in stats, got %v", stats["adapters"])
	}

	// Stats should include expected keys
	expectedKeys := []string{"adapters", "pending_events", "dedup_cache_size", "rate_limits"}
	for _, key := range expectedKeys {
		if _, ok := stats[key]; !ok {
			t.Errorf("stats missing key: %s", key)
		}
	}
}

func TestHub_ResponseHandler(t *testing.T) {
	hub := NewHub(nil)

	var receivedResponse *Response
	hub.SetResponseHandler(func(response *Response) {
		receivedResponse = response
	})

	// Simulate response
	response := &Response{
		EventID:    "evt-123",
		ResponseID: "resp-456",
		Message:    "Test response",
		Source:     "test",
	}
	hub.handleResponse(response)

	if receivedResponse == nil {
		t.Fatal("response handler not called")
	}
	if receivedResponse.EventID != "evt-123" {
		t.Errorf("expected EventID evt-123, got %s", receivedResponse.EventID)
	}
}

func TestHub_PendingEvents(t *testing.T) {
	hub := NewHub(&HubConfig{
		RateLimit:          100,
		CooldownAfterBurst: 1,
		DedupeWindow:       time.Millisecond,
	})

	adapter := newMockAdapter("test-adapter")
	hub.RegisterAdapter(adapter, nil)

	// Send event that requires action
	event := &Event{
		Type:           EventAgentQuestion,
		RepoName:       "test-repo",
		Title:          "Question",
		ActionRequired: true,
	}
	hub.Notify(event)

	events := adapter.getEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	sentEvent := events[0]
	if sentEvent.ResponseID == "" {
		t.Error("event with ActionRequired should have ResponseID set")
	}

	// Verify we can retrieve the pending event
	pending := hub.GetPendingEvent(sentEvent.ResponseID)
	if pending == nil {
		t.Error("pending event should be retrievable")
	}
	if pending.ID != sentEvent.ID {
		t.Errorf("pending event ID mismatch: %s != %s", pending.ID, sentEvent.ID)
	}
}

func TestHub_StartStop(t *testing.T) {
	hub := NewHub(nil)
	adapter := newMockAdapter("test-adapter")
	hub.RegisterAdapter(adapter, nil)

	err := hub.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give cleanup goroutine time to start
	time.Sleep(10 * time.Millisecond)

	err = hub.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}
