package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewTelegramAdapter(t *testing.T) {
	// Test valid config
	config := &TelegramConfig{
		AdapterConfig: AdapterConfig{Name: "test", Type: "telegram", Enabled: true},
		BotToken:      "123456:ABC-DEF",
		ChatID:        "-1001234567890",
	}

	adapter, err := NewTelegramAdapter(config)
	if err != nil {
		t.Fatalf("NewTelegramAdapter failed: %v", err)
	}

	if adapter.Name() != "test" {
		t.Errorf("Name() = %s, want test", adapter.Name())
	}
	if adapter.Type() != "telegram" {
		t.Errorf("Type() = %s, want telegram", adapter.Type())
	}

	// Test missing bot token
	config2 := &TelegramConfig{
		AdapterConfig: AdapterConfig{Name: "test2", Type: "telegram"},
		ChatID:        "-1001234567890",
	}
	_, err = NewTelegramAdapter(config2)
	if err == nil {
		t.Error("NewTelegramAdapter should fail with missing bot_token")
	}

	// Test missing chat ID
	config3 := &TelegramConfig{
		AdapterConfig: AdapterConfig{Name: "test3", Type: "telegram"},
		BotToken:      "123456:ABC-DEF",
	}
	_, err = NewTelegramAdapter(config3)
	if err == nil {
		t.Error("NewTelegramAdapter should fail with missing chat_id")
	}
}

func TestTelegramAdapter_Send(t *testing.T) {
	var receivedPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
	}))
	defer server.Close()

	config := &TelegramConfig{
		AdapterConfig: AdapterConfig{Name: "test", Type: "telegram", Enabled: true},
		BotToken:      "test-token",
		ChatID:        "-1001234567890",
	}

	adapter, _ := NewTelegramAdapter(config)
	// Override API base for testing
	adapter.apiBase = server.URL

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

	// Verify payload
	chatID, _ := receivedPayload["chat_id"].(string)
	if chatID != "-1001234567890" {
		t.Errorf("chat_id = %s, want -1001234567890", chatID)
	}

	text, _ := receivedPayload["text"].(string)
	if text == "" {
		t.Error("expected text in payload")
	}

	parseMode, _ := receivedPayload["parse_mode"].(string)
	if parseMode != "HTML" {
		t.Errorf("parse_mode = %s, want HTML", parseMode)
	}
}

func TestTelegramAdapter_SendWithKeyboard(t *testing.T) {
	var receivedPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
	}))
	defer server.Close()

	config := &TelegramConfig{
		AdapterConfig: AdapterConfig{Name: "test", Type: "telegram", Enabled: true},
		BotToken:      "test-token",
		ChatID:        "-1001234567890",
	}

	adapter, _ := NewTelegramAdapter(config)
	adapter.apiBase = server.URL

	event := &Event{
		ID:             "evt-123",
		Type:           EventAgentQuestion,
		ActionRequired: true,
		ResponseID:     "resp-456",
		RepoName:       "test-repo",
		Title:          "Question",
		Timestamp:      time.Now(),
	}

	adapter.Send(context.Background(), event)

	// Should have reply_markup for interactive event
	replyMarkup, ok := receivedPayload["reply_markup"].(map[string]interface{})
	if !ok {
		t.Fatal("expected reply_markup in payload for action-required event")
	}

	keyboard, ok := replyMarkup["inline_keyboard"].([]interface{})
	if !ok || len(keyboard) == 0 {
		t.Error("expected inline_keyboard in reply_markup")
	}
}

func TestTelegramAdapter_FormatMessage(t *testing.T) {
	config := &TelegramConfig{
		AdapterConfig: AdapterConfig{Name: "test", Type: "telegram", Enabled: true},
		BotToken:      "test-token",
		ChatID:        "-1001234567890",
	}
	adapter, _ := NewTelegramAdapter(config)

	event := &Event{
		ID:        "evt-123",
		Type:      EventAgentQuestion,
		Priority:  PriorityHigh,
		RepoName:  "my-repo",
		AgentName: "eager-badger",
		AgentType: "worker",
		Title:     "Need help",
		Message:   "What should I do?",
		Context:   map[string]string{"pr_url": "https://github.com/test/repo/pull/1"},
		Timestamp: time.Now(),
	}

	message := adapter.formatMessage(event)

	// Check for expected content
	if message == "" {
		t.Error("expected non-empty message")
	}

	// Should contain HTML tags
	if !contains(message, "<b>") {
		t.Error("expected HTML bold tags in message")
	}

	// Should contain event details
	if !contains(message, "my-repo") {
		t.Error("expected repo name in message")
	}
	if !contains(message, "eager-badger") {
		t.Error("expected agent name in message")
	}
}

func TestTelegramAdapter_EscapeHTML(t *testing.T) {
	config := &TelegramConfig{
		AdapterConfig: AdapterConfig{Name: "test", Type: "telegram", Enabled: true},
		BotToken:      "test-token",
		ChatID:        "-1001234567890",
	}
	adapter, _ := NewTelegramAdapter(config)

	tests := []struct {
		input    string
		expected string
	}{
		{"hello world", "hello world"},
		{"a < b", "a &lt; b"},
		{"a > b", "a &gt; b"},
		{"a & b", "a &amp; b"},
	}

	for _, tt := range tests {
		result := adapter.escapeHTML(tt.input)
		if result != tt.expected {
			t.Errorf("escapeHTML(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestTelegramAdapter_SupportsResponses(t *testing.T) {
	// Without webhook URL
	config1 := &TelegramConfig{
		AdapterConfig: AdapterConfig{Name: "test", Type: "telegram", Enabled: true},
		BotToken:      "test-token",
		ChatID:        "-123",
	}
	adapter1, _ := NewTelegramAdapter(config1)
	if adapter1.SupportsResponses() {
		t.Error("adapter without webhook should not support responses")
	}

	// With webhook URL
	config2 := &TelegramConfig{
		AdapterConfig: AdapterConfig{Name: "test", Type: "telegram", Enabled: true},
		BotToken:      "test-token",
		ChatID:        "-123",
		ListenAddr:    ":8080",
		WebhookURL:    "https://example.com/webhook",
	}
	adapter2, _ := NewTelegramAdapter(config2)
	if !adapter2.SupportsResponses() {
		t.Error("adapter with webhook should support responses")
	}
}

func TestFormatTelegramStatusSummary(t *testing.T) {
	summary := &StatusSummary{
		RepoName:         "test-repo",
		TotalAgents:      3,
		ActiveWorkers:    2,
		PendingQuestions: 1,
		CompletedTasks:   5,
		Agents: []AgentStatus{
			{Name: "worker-1", Type: "worker", Status: "working"},
			{Name: "worker-2", Type: "worker", Status: "stuck"},
		},
		GeneratedAt: time.Now(),
	}

	message := FormatTelegramStatusSummary(summary)

	if message == "" {
		t.Error("expected non-empty message")
	}

	if !contains(message, "test-repo") {
		t.Error("expected repo name in message")
	}
	if !contains(message, "Active Workers") {
		t.Error("expected stats in message")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
