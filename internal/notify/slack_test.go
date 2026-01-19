package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewSlackAdapter(t *testing.T) {
	// Test with webhook URL
	config := &SlackConfig{
		AdapterConfig: AdapterConfig{Name: "test", Type: "slack", Enabled: true},
		WebhookURL:    "https://hooks.slack.com/services/xxx",
	}

	adapter, err := NewSlackAdapter(config)
	if err != nil {
		t.Fatalf("NewSlackAdapter failed: %v", err)
	}

	if adapter.Name() != "test" {
		t.Errorf("Name() = %s, want test", adapter.Name())
	}
	if adapter.Type() != "slack" {
		t.Errorf("Type() = %s, want slack", adapter.Type())
	}

	// Test with bot token
	config2 := &SlackConfig{
		AdapterConfig: AdapterConfig{Name: "test2", Type: "slack", Enabled: true},
		BotToken:      "xoxb-xxx",
	}

	_, err = NewSlackAdapter(config2)
	if err != nil {
		t.Fatalf("NewSlackAdapter with bot token failed: %v", err)
	}

	// Test missing both
	config3 := &SlackConfig{
		AdapterConfig: AdapterConfig{Name: "test3", Type: "slack"},
	}
	_, err = NewSlackAdapter(config3)
	if err == nil {
		t.Error("NewSlackAdapter should fail with neither webhook_url nor bot_token")
	}
}

func TestSlackAdapter_Send(t *testing.T) {
	var receivedPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := &SlackConfig{
		AdapterConfig: AdapterConfig{Name: "test", Type: "slack", Enabled: true},
		WebhookURL:    server.URL,
	}

	adapter, _ := NewSlackAdapter(config)

	event := &Event{
		ID:        "evt-123",
		Type:      EventAgentQuestion,
		Priority:  PriorityHigh,
		RepoName:  "test-repo",
		AgentName: "test-agent",
		AgentType: "worker",
		Title:     "Agent needs help",
		Message:   "Should I proceed with the refactor?",
		Timestamp: time.Now(),
	}

	err := adapter.Send(context.Background(), event)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Verify blocks were sent
	blocks, ok := receivedPayload["blocks"].([]interface{})
	if !ok || len(blocks) == 0 {
		t.Error("expected blocks in payload")
	}

	// Verify fallback text
	text, _ := receivedPayload["text"].(string)
	if text == "" {
		t.Error("expected text fallback in payload")
	}
}

func TestSlackAdapter_FormatBlocks(t *testing.T) {
	config := &SlackConfig{
		AdapterConfig: AdapterConfig{Name: "test", Type: "slack", Enabled: true},
		WebhookURL:    "https://hooks.slack.com/services/xxx",
	}
	adapter, _ := NewSlackAdapter(config)

	event := &Event{
		ID:             "evt-123",
		Type:           EventAgentQuestion,
		Priority:       PriorityHigh,
		RepoName:       "test-repo",
		AgentName:      "eager-badger",
		AgentType:      "worker",
		Title:          "Need guidance",
		Message:        "Should I update dependencies?",
		Context:        map[string]string{"pr_url": "https://github.com/test/repo/pull/1"},
		ActionRequired: true,
		ResponseID:     "resp-456",
		Timestamp:      time.Now(),
	}

	blocks := adapter.formatBlocks(event)

	// Should have multiple blocks: header, context, section, context (fields), divider, actions, footer
	if len(blocks) < 4 {
		t.Errorf("expected at least 4 blocks, got %d", len(blocks))
	}

	// First block should be header
	if blocks[0]["type"] != "header" {
		t.Errorf("first block should be header, got %s", blocks[0]["type"])
	}
}

func TestSlackAdapter_EventEmoji(t *testing.T) {
	config := &SlackConfig{
		AdapterConfig: AdapterConfig{Name: "test", Type: "slack", Enabled: true},
		WebhookURL:    "https://hooks.slack.com/services/xxx",
	}
	adapter, _ := NewSlackAdapter(config)

	tests := []struct {
		eventType EventType
		emoji     string
	}{
		{EventAgentQuestion, ":raising_hand:"},
		{EventAgentCompleted, ":white_check_mark:"},
		{EventAgentStuck, ":warning:"},
		{EventAgentError, ":x:"},
		{EventCIFailed, ":rotating_light:"},
	}

	for _, tt := range tests {
		t.Run(string(tt.eventType), func(t *testing.T) {
			emoji := adapter.eventEmoji(tt.eventType)
			if emoji != tt.emoji {
				t.Errorf("eventEmoji(%s) = %s, want %s", tt.eventType, emoji, tt.emoji)
			}
		})
	}
}

func TestSlackAdapter_EscapeSlack(t *testing.T) {
	config := &SlackConfig{
		AdapterConfig: AdapterConfig{Name: "test", Type: "slack", Enabled: true},
		WebhookURL:    "https://hooks.slack.com/services/xxx",
	}
	adapter, _ := NewSlackAdapter(config)

	tests := []struct {
		input    string
		expected string
	}{
		{"hello world", "hello world"},
		{"a < b", "a &lt; b"},
		{"a > b", "a &gt; b"},
		{"a & b", "a &amp; b"},
		{"<script>alert('xss')</script>", "&lt;script&gt;alert('xss')&lt;/script&gt;"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := adapter.escapeSlack(tt.input)
			if result != tt.expected {
				t.Errorf("escapeSlack(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSlackAdapter_SupportsResponses(t *testing.T) {
	// Without bot token and listen addr
	config1 := &SlackConfig{
		AdapterConfig: AdapterConfig{Name: "test", Type: "slack", Enabled: true},
		WebhookURL:    "https://hooks.slack.com/services/xxx",
	}
	adapter1, _ := NewSlackAdapter(config1)
	if adapter1.SupportsResponses() {
		t.Error("adapter without bot token should not support responses")
	}

	// With bot token and listen addr
	config2 := &SlackConfig{
		AdapterConfig: AdapterConfig{Name: "test", Type: "slack", Enabled: true},
		BotToken:      "xoxb-xxx",
		ListenAddr:    ":8080",
	}
	adapter2, _ := NewSlackAdapter(config2)
	if !adapter2.SupportsResponses() {
		t.Error("adapter with bot token and listen addr should support responses")
	}
}

func TestFormatSlackStatusSummary(t *testing.T) {
	summary := &StatusSummary{
		RepoName:         "test-repo",
		TotalAgents:      5,
		ActiveWorkers:    3,
		PendingQuestions: 1,
		CompletedTasks:   10,
		Agents: []AgentStatus{
			{Name: "worker-1", Type: "worker", Status: "working", Task: "Implementing feature X"},
			{Name: "worker-2", Type: "worker", Status: "waiting"},
			{Name: "supervisor", Type: "supervisor", Status: "working"},
		},
		GeneratedAt: time.Now(),
	}

	blocks := FormatSlackStatusSummary(summary)

	if len(blocks) < 3 {
		t.Errorf("expected at least 3 blocks, got %d", len(blocks))
	}

	// First block should be header
	if blocks[0]["type"] != "header" {
		t.Errorf("first block should be header, got %s", blocks[0]["type"])
	}
}
