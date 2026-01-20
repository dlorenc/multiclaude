package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewDiscordAdapter(t *testing.T) {
	// Test valid config
	config := &DiscordConfig{
		AdapterConfig: AdapterConfig{Name: "test", Type: "discord", Enabled: true},
		WebhookURL:    "https://discord.com/api/webhooks/xxx/yyy",
	}

	adapter, err := NewDiscordAdapter(config)
	if err != nil {
		t.Fatalf("NewDiscordAdapter failed: %v", err)
	}

	if adapter.Name() != "test" {
		t.Errorf("Name() = %s, want test", adapter.Name())
	}
	if adapter.Type() != "discord" {
		t.Errorf("Type() = %s, want discord", adapter.Type())
	}

	// Test missing webhook URL
	config2 := &DiscordConfig{
		AdapterConfig: AdapterConfig{Name: "test2", Type: "discord"},
	}
	_, err = NewDiscordAdapter(config2)
	if err == nil {
		t.Error("NewDiscordAdapter should fail with missing webhook_url")
	}
}

func TestDiscordAdapter_Send(t *testing.T) {
	var receivedPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	config := &DiscordConfig{
		AdapterConfig: AdapterConfig{Name: "test", Type: "discord", Enabled: true},
		WebhookURL:    server.URL,
		Username:      "TestBot",
	}

	adapter, _ := NewDiscordAdapter(config)

	event := &Event{
		ID:        "evt-123",
		Type:      EventAgentQuestion,
		Priority:  PriorityHigh,
		RepoName:  "test-repo",
		AgentName: "test-agent",
		AgentType: "worker",
		Title:     "Agent needs help",
		Message:   "Should I proceed?",
		Timestamp: time.Now(),
	}

	err := adapter.Send(context.Background(), event)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Verify username
	username, _ := receivedPayload["username"].(string)
	if username != "TestBot" {
		t.Errorf("username = %s, want TestBot", username)
	}

	// Verify embeds
	embeds, ok := receivedPayload["embeds"].([]interface{})
	if !ok || len(embeds) == 0 {
		t.Error("expected embeds in payload")
	}
}

func TestDiscordAdapter_FormatEmbed(t *testing.T) {
	config := &DiscordConfig{
		AdapterConfig: AdapterConfig{Name: "test", Type: "discord", Enabled: true},
		WebhookURL:    "https://discord.com/api/webhooks/xxx/yyy",
	}
	adapter, _ := NewDiscordAdapter(config)

	event := &Event{
		ID:        "evt-123",
		Type:      EventAgentQuestion,
		Priority:  PriorityHigh,
		RepoName:  "test-repo",
		AgentName: "eager-badger",
		AgentType: "worker",
		Title:     "Need guidance",
		Message:   "What should I do?",
		Context:   map[string]string{"branch": "feature/test"},
		Timestamp: time.Now(),
	}

	embed := adapter.formatEmbed(event)

	// Check required fields
	title, _ := embed["title"].(string)
	if title == "" {
		t.Error("expected title in embed")
	}

	description, _ := embed["description"].(string)
	if description != "What should I do?" {
		t.Errorf("description = %s, want 'What should I do?'", description)
	}

	color, ok := embed["color"].(int)
	if !ok {
		t.Error("expected color in embed")
	}
	// High priority should be red
	if color != 15158332 {
		t.Errorf("color = %d, want 15158332 (red for high priority)", color)
	}

	// Check fields
	fields, ok := embed["fields"].([]map[string]interface{})
	if !ok || len(fields) == 0 {
		t.Error("expected fields in embed")
	}

	// Should have repo, agent, priority, and context fields
	if len(fields) < 4 {
		t.Errorf("expected at least 4 fields, got %d", len(fields))
	}
}

func TestDiscordAdapter_EventColor(t *testing.T) {
	config := &DiscordConfig{
		AdapterConfig: AdapterConfig{Name: "test", Type: "discord", Enabled: true},
		WebhookURL:    "https://discord.com/api/webhooks/xxx/yyy",
	}
	adapter, _ := NewDiscordAdapter(config)

	tests := []struct {
		priority Priority
		color    int
	}{
		{PriorityHigh, 15158332},   // Red
		{PriorityMedium, 15105570}, // Orange
		{PriorityLow, 3447003},     // Blue
	}

	for _, tt := range tests {
		event := &Event{Priority: tt.priority}
		color := adapter.eventColor(event)
		if color != tt.color {
			t.Errorf("eventColor(priority=%s) = %d, want %d", tt.priority, color, tt.color)
		}
	}
}

func TestDiscordAdapter_SupportsResponses(t *testing.T) {
	config := &DiscordConfig{
		AdapterConfig: AdapterConfig{Name: "test", Type: "discord", Enabled: true},
		WebhookURL:    "https://discord.com/api/webhooks/xxx/yyy",
	}
	adapter, _ := NewDiscordAdapter(config)

	// Discord webhooks don't support responses
	if adapter.SupportsResponses() {
		t.Error("Discord adapter should not support responses")
	}
}

func TestFormatDiscordStatusSummary(t *testing.T) {
	summary := &StatusSummary{
		RepoName:         "test-repo",
		TotalAgents:      3,
		ActiveWorkers:    2,
		PendingQuestions: 1,
		CompletedTasks:   5,
		Agents: []AgentStatus{
			{Name: "worker-1", Type: "worker", Status: "working", Task: "Feature X"},
			{Name: "worker-2", Type: "worker", Status: "completed"},
		},
		GeneratedAt: time.Now(),
	}

	embed := FormatDiscordStatusSummary(summary)

	// Check title
	title, _ := embed["title"].(string)
	if title == "" {
		t.Error("expected title in embed")
	}

	// Check fields
	fields, ok := embed["fields"].([]map[string]interface{})
	if !ok || len(fields) < 3 {
		t.Error("expected at least 3 fields in embed")
	}

	// First 3 fields should be stats
	statsFields := []string{"Active Workers", "Pending Questions", "Completed Tasks"}
	for i, expected := range statsFields {
		name, _ := fields[i]["name"].(string)
		if name != expected {
			t.Errorf("field %d name = %s, want %s", i, name, expected)
		}
	}
}

func TestDiscordAdapter_SendFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"message": "rate limited"}`))
	}))
	defer server.Close()

	config := &DiscordConfig{
		AdapterConfig: AdapterConfig{Name: "test", Type: "discord", Enabled: true},
		WebhookURL:    server.URL,
	}

	adapter, _ := NewDiscordAdapter(config)

	event := &Event{ID: "evt-1", Type: EventAgentQuestion}
	err := adapter.Send(context.Background(), event)

	if err == nil {
		t.Error("Send should fail with 429 response")
	}
}
