package notify

import (
	"testing"
	"time"
)

func TestEventPriority(t *testing.T) {
	tests := []struct {
		eventType EventType
		expected  Priority
	}{
		{EventAgentQuestion, PriorityHigh},
		{EventAgentStuck, PriorityHigh},
		{EventAgentError, PriorityHigh},
		{EventCIFailed, PriorityHigh},
		{EventAgentCompleted, PriorityMedium},
		{EventPRCreated, PriorityMedium},
		{EventPRMerged, PriorityLow},
		{EventStatusUpdate, PriorityLow},
		{EventType("unknown"), PriorityMedium},
	}

	for _, tt := range tests {
		t.Run(string(tt.eventType), func(t *testing.T) {
			got := EventPriority(tt.eventType)
			if got != tt.expected {
				t.Errorf("EventPriority(%s) = %s, want %s", tt.eventType, got, tt.expected)
			}
		})
	}
}

func TestEventFields(t *testing.T) {
	event := &Event{
		ID:             "evt-123",
		Type:           EventAgentQuestion,
		Priority:       PriorityHigh,
		Timestamp:      time.Now(),
		RepoName:       "test-repo",
		AgentName:      "test-agent",
		AgentType:      "worker",
		Title:          "Test Title",
		Message:        "Test message body",
		Context:        map[string]string{"key": "value"},
		ActionRequired: true,
		ResponseID:     "resp-456",
	}

	if event.ID != "evt-123" {
		t.Errorf("Event.ID = %s, want evt-123", event.ID)
	}
	if event.Type != EventAgentQuestion {
		t.Errorf("Event.Type = %s, want agent.question", event.Type)
	}
	if !event.ActionRequired {
		t.Error("Event.ActionRequired = false, want true")
	}
}

func TestResponse(t *testing.T) {
	response := &Response{
		EventID:    "evt-123",
		ResponseID: "resp-456",
		Message:    "User response",
		Action:     "respond",
		Source:     "slack",
		UserID:     "U12345",
		Timestamp:  time.Now(),
	}

	if response.EventID != "evt-123" {
		t.Errorf("Response.EventID = %s, want evt-123", response.EventID)
	}
	if response.Source != "slack" {
		t.Errorf("Response.Source = %s, want slack", response.Source)
	}
}

func TestStatusSummary(t *testing.T) {
	summary := &StatusSummary{
		RepoName:         "test-repo",
		TotalAgents:      5,
		ActiveWorkers:    3,
		PendingQuestions: 2,
		CompletedTasks:   10,
		Agents: []AgentStatus{
			{Name: "agent-1", Type: "worker", Status: "working"},
			{Name: "agent-2", Type: "supervisor", Status: "waiting"},
		},
		GeneratedAt: time.Now(),
	}

	if summary.TotalAgents != 5 {
		t.Errorf("StatusSummary.TotalAgents = %d, want 5", summary.TotalAgents)
	}
	if len(summary.Agents) != 2 {
		t.Errorf("len(StatusSummary.Agents) = %d, want 2", len(summary.Agents))
	}
}
