package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewWebhookAdapter(t *testing.T) {
	// Test valid config
	config := &WebhookConfig{
		AdapterConfig: AdapterConfig{Name: "test", Type: "webhook", Enabled: true},
		URL:           "https://example.com/webhook",
	}

	adapter, err := NewWebhookAdapter(config)
	if err != nil {
		t.Fatalf("NewWebhookAdapter failed: %v", err)
	}

	if adapter.Name() != "test" {
		t.Errorf("Name() = %s, want test", adapter.Name())
	}
	if adapter.Type() != "webhook" {
		t.Errorf("Type() = %s, want webhook", adapter.Type())
	}

	// Test missing URL
	config2 := &WebhookConfig{
		AdapterConfig: AdapterConfig{Name: "test2", Type: "webhook"},
	}
	_, err = NewWebhookAdapter(config2)
	if err == nil {
		t.Error("NewWebhookAdapter should fail with missing URL")
	}
}

func TestWebhookAdapter_Send(t *testing.T) {
	// Create test server
	var receivedPayload WebhookPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := &WebhookConfig{
		AdapterConfig: AdapterConfig{Name: "test", Type: "webhook", Enabled: true},
		URL:           server.URL,
		Timeout:       5 * time.Second,
	}

	adapter, _ := NewWebhookAdapter(config)

	event := &Event{
		ID:        "evt-123",
		Type:      EventAgentQuestion,
		RepoName:  "test-repo",
		AgentName: "test-agent",
		Title:     "Test question",
		Message:   "What should I do?",
		Timestamp: time.Now(),
	}

	err := adapter.Send(context.Background(), event)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if receivedPayload.Event.ID != "evt-123" {
		t.Errorf("received event ID = %s, want evt-123", receivedPayload.Event.ID)
	}
	if receivedPayload.Version != "1.0" {
		t.Errorf("received version = %s, want 1.0", receivedPayload.Version)
	}
}

func TestWebhookAdapter_SendWithHeaders(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := &WebhookConfig{
		AdapterConfig: AdapterConfig{Name: "test", Type: "webhook", Enabled: true},
		URL:           server.URL,
		Headers:       map[string]string{"Authorization": "Bearer secret-token"},
	}

	adapter, _ := NewWebhookAdapter(config)

	event := &Event{ID: "evt-1", Type: EventAgentQuestion}
	adapter.Send(context.Background(), event)

	if receivedAuth != "Bearer secret-token" {
		t.Errorf("received Authorization = %s, want Bearer secret-token", receivedAuth)
	}
}

func TestWebhookAdapter_SendWithSignature(t *testing.T) {
	var receivedSignature string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSignature = r.Header.Get("X-Multiclaude-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := &WebhookConfig{
		AdapterConfig: AdapterConfig{Name: "test", Type: "webhook", Enabled: true},
		URL:           server.URL,
		Secret:        "my-secret-key",
	}

	adapter, _ := NewWebhookAdapter(config)

	event := &Event{ID: "evt-1", Type: EventAgentQuestion}
	adapter.Send(context.Background(), event)

	if receivedSignature == "" {
		t.Error("expected signature header to be set")
	}
}

func TestWebhookAdapter_SendFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	config := &WebhookConfig{
		AdapterConfig: AdapterConfig{Name: "test", Type: "webhook", Enabled: true},
		URL:           server.URL,
		RetryCount:    1,
		RetryDelay:    time.Millisecond,
	}

	adapter, _ := NewWebhookAdapter(config)

	event := &Event{ID: "evt-1", Type: EventAgentQuestion}
	err := adapter.Send(context.Background(), event)

	if err == nil {
		t.Error("Send should fail with 500 response")
	}
}

func TestWebhookAdapter_Retry(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := &WebhookConfig{
		AdapterConfig: AdapterConfig{Name: "test", Type: "webhook", Enabled: true},
		URL:           server.URL,
		RetryCount:    3,
		RetryDelay:    time.Millisecond,
	}

	adapter, _ := NewWebhookAdapter(config)

	event := &Event{ID: "evt-1", Type: EventAgentQuestion}
	err := adapter.Send(context.Background(), event)

	if err != nil {
		t.Errorf("Send should succeed after retries: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestVerifySignature(t *testing.T) {
	secret := "my-secret"
	payload := []byte(`{"test": "data"}`)

	// Create a valid signature
	adapter := &WebhookAdapter{config: &WebhookConfig{Secret: secret}}
	signature := adapter.sign(payload)

	// Verify valid signature
	if !VerifySignature(secret, signature, payload) {
		t.Error("valid signature should verify")
	}

	// Verify invalid signature
	if VerifySignature(secret, "invalid-signature", payload) {
		t.Error("invalid signature should not verify")
	}

	// Verify wrong secret
	if VerifySignature("wrong-secret", signature, payload) {
		t.Error("signature with wrong secret should not verify")
	}

	// Verify tampered payload
	if VerifySignature(secret, signature, []byte(`{"test": "modified"}`)) {
		t.Error("signature with tampered payload should not verify")
	}
}

func TestWebhookAdapter_SupportsResponses(t *testing.T) {
	config := &WebhookConfig{
		AdapterConfig: AdapterConfig{Name: "test", Type: "webhook", Enabled: true},
		URL:           "https://example.com/webhook",
	}

	adapter, _ := NewWebhookAdapter(config)

	if adapter.SupportsResponses() {
		t.Error("basic webhook should not support responses")
	}
}

func TestInteractiveWebhookAdapter(t *testing.T) {
	config := &InteractiveWebhookConfig{
		WebhookConfig: WebhookConfig{
			AdapterConfig: AdapterConfig{Name: "test", Type: "webhook", Enabled: true},
			URL:           "https://example.com/webhook",
		},
		ListenAddr:   ":0", // Use any available port
		ResponsePath: "/response",
	}

	adapter, err := NewInteractiveWebhookAdapter(config)
	if err != nil {
		t.Fatalf("NewInteractiveWebhookAdapter failed: %v", err)
	}

	if !adapter.SupportsResponses() {
		t.Error("interactive webhook should support responses")
	}
}
