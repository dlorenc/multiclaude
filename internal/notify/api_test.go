package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewAPIServer(t *testing.T) {
	hub := NewHub(nil)
	config := &APIConfig{
		Enabled:    true,
		ListenAddr: ":8080",
	}

	server := NewAPIServer(config, hub)

	if server == nil {
		t.Fatal("NewAPIServer returned nil")
	}
	if server.config != config {
		t.Error("config not set correctly")
	}
	if server.hub != hub {
		t.Error("hub not set correctly")
	}
	if server.sseClients == nil {
		t.Error("sseClients not initialized")
	}
	if server.recentEvents == nil {
		t.Error("recentEvents not initialized")
	}
	if server.maxEvents != 100 {
		t.Errorf("maxEvents = %d, want 100", server.maxEvents)
	}
}

func TestAPIServer_HandleHealth(t *testing.T) {
	hub := NewHub(nil)
	config := &APIConfig{Enabled: true}
	server := NewAPIServer(config, hub)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response["status"] != "ok" {
		t.Errorf("status = %v, want ok", response["status"])
	}
	if _, ok := response["time"]; !ok {
		t.Error("response should include time")
	}
}

func TestAPIServer_HandleEvents(t *testing.T) {
	hub := NewHub(&HubConfig{
		RateLimit:          100,
		CooldownAfterBurst: 1,
		DedupeWindow:       time.Millisecond,
	})
	config := &APIConfig{Enabled: true}
	server := NewAPIServer(config, hub)

	// Add some test events
	server.BroadcastEvent(&Event{
		ID:       "evt-1",
		Type:     EventAgentQuestion,
		RepoName: "repo-a",
		Title:    "Test event 1",
	})
	server.BroadcastEvent(&Event{
		ID:       "evt-2",
		Type:     EventPRCreated,
		RepoName: "repo-b",
		Title:    "Test event 2",
	})

	t.Run("get all events", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
		w := httptest.NewRecorder()

		server.handleEvents(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
		}

		var response map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		events := response["events"].([]interface{})
		if len(events) != 2 {
			t.Errorf("expected 2 events, got %d", len(events))
		}
	})

	t.Run("filter by type", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/events?type=agent.question", nil)
		w := httptest.NewRecorder()

		server.handleEvents(w, req)

		var response map[string]interface{}
		json.NewDecoder(w.Body).Decode(&response)

		events := response["events"].([]interface{})
		if len(events) != 1 {
			t.Errorf("expected 1 event with type filter, got %d", len(events))
		}
	})

	t.Run("filter by repo", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/events?repo=repo-a", nil)
		w := httptest.NewRecorder()

		server.handleEvents(w, req)

		var response map[string]interface{}
		json.NewDecoder(w.Body).Decode(&response)

		events := response["events"].([]interface{})
		if len(events) != 1 {
			t.Errorf("expected 1 event with repo filter, got %d", len(events))
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/events", nil)
		w := httptest.NewRecorder()

		server.handleEvents(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("status code = %d, want %d", w.Code, http.StatusMethodNotAllowed)
		}
	})
}

func TestAPIServer_HandleAdapters(t *testing.T) {
	hub := NewHub(nil)
	hub.RegisterAdapter(newMockAdapter("adapter-1"), nil)
	hub.RegisterAdapter(newMockAdapter("adapter-2"), nil)

	config := &APIConfig{Enabled: true}
	server := NewAPIServer(config, hub)

	t.Run("get adapters", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/adapters", nil)
		w := httptest.NewRecorder()

		server.handleAdapters(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
		}

		var response map[string]interface{}
		json.NewDecoder(w.Body).Decode(&response)

		if response["count"].(float64) != 2 {
			t.Errorf("expected count=2, got %v", response["count"])
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/adapters", nil)
		w := httptest.NewRecorder()

		server.handleAdapters(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("status code = %d, want %d", w.Code, http.StatusMethodNotAllowed)
		}
	})
}

func TestAPIServer_HandleStats(t *testing.T) {
	hub := NewHub(nil)
	hub.RegisterAdapter(newMockAdapter("test"), nil)

	config := &APIConfig{Enabled: true}
	server := NewAPIServer(config, hub)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	w := httptest.NewRecorder()

	server.handleStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	var response map[string]interface{}
	json.NewDecoder(w.Body).Decode(&response)

	// Should include standard stats plus API-specific ones
	if _, ok := response["adapters"]; !ok {
		t.Error("response should include adapters count")
	}
	if _, ok := response["sse_clients"]; !ok {
		t.Error("response should include sse_clients count")
	}
	if _, ok := response["cached_events"]; !ok {
		t.Error("response should include cached_events count")
	}
}

func TestAPIServer_HandleStatus(t *testing.T) {
	hub := NewHub(nil)
	config := &APIConfig{Enabled: true}
	server := NewAPIServer(config, hub)

	// Register a mock status provider
	provider := &mockStatusProvider{
		repos: []string{"test-repo"},
		statuses: map[string]*StatusSummary{
			"test-repo": {
				RepoName:      "test-repo",
				TotalAgents:   3,
				ActiveWorkers: 2,
			},
		},
	}
	server.RegisterStatusProvider(provider)

	t.Run("get overall status", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
		w := httptest.NewRecorder()

		server.handleStatus(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
		}

		var response map[string]interface{}
		json.NewDecoder(w.Body).Decode(&response)

		if _, ok := response["time"]; !ok {
			t.Error("response should include time")
		}
		if _, ok := response["repos"]; !ok {
			t.Error("response should include repos")
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/status", nil)
		w := httptest.NewRecorder()

		server.handleStatus(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("status code = %d, want %d", w.Code, http.StatusMethodNotAllowed)
		}
	})
}

func TestAPIServer_HandleRepoStatus(t *testing.T) {
	hub := NewHub(nil)
	config := &APIConfig{Enabled: true}
	server := NewAPIServer(config, hub)

	provider := &mockStatusProvider{
		repos: []string{"my-repo"},
		statuses: map[string]*StatusSummary{
			"my-repo": {
				RepoName:      "my-repo",
				TotalAgents:   5,
				ActiveWorkers: 3,
			},
		},
	}
	server.RegisterStatusProvider(provider)

	t.Run("get specific repo status", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/status/my-repo", nil)
		w := httptest.NewRecorder()

		server.handleRepoStatus(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
		}

		var response StatusSummary
		json.NewDecoder(w.Body).Decode(&response)

		if response.RepoName != "my-repo" {
			t.Errorf("RepoName = %s, want my-repo", response.RepoName)
		}
		if response.TotalAgents != 5 {
			t.Errorf("TotalAgents = %d, want 5", response.TotalAgents)
		}
	})

	t.Run("repo not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/status/nonexistent", nil)
		w := httptest.NewRecorder()

		server.handleRepoStatus(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("status code = %d, want %d", w.Code, http.StatusNotFound)
		}
	})

	t.Run("empty repo name", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/status/", nil)
		w := httptest.NewRecorder()

		server.handleRepoStatus(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status code = %d, want %d", w.Code, http.StatusBadRequest)
		}
	})
}

func TestAPIServer_HandleRespond(t *testing.T) {
	hub := NewHub(&HubConfig{
		RateLimit:          100,
		CooldownAfterBurst: 1,
		DedupeWindow:       time.Millisecond,
	})
	adapter := newMockAdapter("test")
	hub.RegisterAdapter(adapter, nil)

	config := &APIConfig{Enabled: true}
	server := NewAPIServer(config, hub)

	// Create a pending event
	event := &Event{
		Type:           EventAgentQuestion,
		RepoName:       "test-repo",
		Title:          "Question",
		ActionRequired: true,
	}
	hub.Notify(event)

	// Get the response ID from the sent event
	events := adapter.getEvents()
	if len(events) == 0 {
		t.Fatal("expected event to be sent")
	}
	responseID := events[0].ResponseID

	t.Run("valid response", func(t *testing.T) {
		body := map[string]string{
			"response_id": responseID,
			"message":     "Test response",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/respond", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.handleRespond(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
		}
	})

	t.Run("missing response_id", func(t *testing.T) {
		body := map[string]string{
			"message": "Test response",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/respond", bytes.NewReader(bodyBytes))
		w := httptest.NewRecorder()

		server.handleRespond(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status code = %d, want %d", w.Code, http.StatusBadRequest)
		}
	})

	t.Run("invalid response_id", func(t *testing.T) {
		body := map[string]string{
			"response_id": "invalid-id",
			"message":     "Test response",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/respond", bytes.NewReader(bodyBytes))
		w := httptest.NewRecorder()

		server.handleRespond(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("status code = %d, want %d", w.Code, http.StatusNotFound)
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/respond", nil)
		w := httptest.NewRecorder()

		server.handleRespond(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("status code = %d, want %d", w.Code, http.StatusMethodNotAllowed)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/respond", bytes.NewReader([]byte("not json")))
		w := httptest.NewRecorder()

		server.handleRespond(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status code = %d, want %d", w.Code, http.StatusBadRequest)
		}
	})
}

func TestAPIServer_WithAuth(t *testing.T) {
	hub := NewHub(nil)
	config := &APIConfig{
		Enabled:   true,
		AuthToken: "secret-token",
	}
	server := NewAPIServer(config, hub)

	handler := server.withAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	t.Run("valid token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
		}
	})

	t.Run("missing token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("status code = %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", "Bearer wrong-token")
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("status code = %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})
}

func TestAPIServer_WithAuth_NoToken(t *testing.T) {
	hub := NewHub(nil)
	config := &APIConfig{
		Enabled:   true,
		AuthToken: "", // No auth required
	}
	server := NewAPIServer(config, hub)

	handler := server.withAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d (no auth required)", w.Code, http.StatusOK)
	}
}

func TestAPIServer_CORSMiddleware(t *testing.T) {
	hub := NewHub(nil)
	config := &APIConfig{
		Enabled:     true,
		CORSOrigins: []string{"https://example.com", "*"},
	}
	server := NewAPIServer(config, hub)

	handler := server.corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("matching origin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Origin", "https://example.com")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Header().Get("Access-Control-Allow-Origin") != "https://example.com" {
			t.Errorf("CORS origin not set correctly")
		}
	})

	t.Run("OPTIONS preflight", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/test", nil)
		req.Header.Set("Origin", "https://example.com")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("OPTIONS should return 200, got %d", w.Code)
		}
	})
}

func TestAPIServer_BroadcastEvent(t *testing.T) {
	hub := NewHub(nil)
	config := &APIConfig{Enabled: true}
	server := NewAPIServer(config, hub)

	// Broadcast more than maxEvents to test trimming
	server.maxEvents = 5
	for i := 0; i < 10; i++ {
		server.BroadcastEvent(&Event{
			ID:    string(rune('a' + i)),
			Type:  EventAgentQuestion,
			Title: "Event",
		})
	}

	server.eventsMu.RLock()
	count := len(server.recentEvents)
	server.eventsMu.RUnlock()

	if count != 5 {
		t.Errorf("recentEvents should be trimmed to 5, got %d", count)
	}
}

func TestAPIServer_HandleSSE_Disabled(t *testing.T) {
	hub := NewHub(nil)
	config := &APIConfig{
		Enabled:   true,
		EnableSSE: false,
	}
	server := NewAPIServer(config, hub)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/stream", nil)
	w := httptest.NewRecorder()

	server.handleSSE(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status code = %d, want %d when SSE disabled", w.Code, http.StatusNotFound)
	}
}

func TestAPIServer_StartStop_DisabledConfig(t *testing.T) {
	hub := NewHub(nil)
	config := &APIConfig{Enabled: false}
	server := NewAPIServer(config, hub)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start with disabled config should return immediately
	err := server.Start(ctx)
	if err != nil {
		t.Errorf("Start with disabled config should not error: %v", err)
	}

	// Stop should work
	err = server.Stop()
	if err != nil {
		t.Errorf("Stop should not error: %v", err)
	}
}

func TestAPIServer_StartStop_NilConfig(t *testing.T) {
	hub := NewHub(nil)
	server := NewAPIServer(nil, hub)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start with nil config should return immediately
	err := server.Start(ctx)
	if err != nil {
		t.Errorf("Start with nil config should not error: %v", err)
	}
}

func TestAPIServer_RegisterStatusProvider(t *testing.T) {
	hub := NewHub(nil)
	config := &APIConfig{Enabled: true}
	server := NewAPIServer(config, hub)

	provider1 := &mockStatusProvider{repos: []string{"repo-1"}}
	provider2 := &mockStatusProvider{repos: []string{"repo-2"}}

	server.RegisterStatusProvider(provider1)
	server.RegisterStatusProvider(provider2)

	server.providerMu.RLock()
	count := len(server.statusProviders)
	server.providerMu.RUnlock()

	if count != 2 {
		t.Errorf("expected 2 providers, got %d", count)
	}
}

func TestSendAPIResponse(t *testing.T) {
	t.Run("success response", func(t *testing.T) {
		w := httptest.NewRecorder()
		data := map[string]string{"key": "value"}

		SendAPIResponse(w, http.StatusOK, data, nil)

		if w.Code != http.StatusOK {
			t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
		}

		var response APIResponse
		json.NewDecoder(w.Body).Decode(&response)

		if !response.Success {
			t.Error("response should be successful")
		}
		if response.Error != "" {
			t.Error("error should be empty")
		}
	})

	t.Run("error response", func(t *testing.T) {
		w := httptest.NewRecorder()
		err := &testError{msg: "something went wrong"}

		SendAPIResponse(w, http.StatusBadRequest, nil, err)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status code = %d, want %d", w.Code, http.StatusBadRequest)
		}

		var response APIResponse
		json.NewDecoder(w.Body).Decode(&response)

		if response.Success {
			t.Error("response should not be successful")
		}
		if response.Error != "something went wrong" {
			t.Errorf("error = %s, want 'something went wrong'", response.Error)
		}
	})
}

// mockStatusProvider implements StatusProvider for testing
type mockStatusProvider struct {
	repos    []string
	statuses map[string]*StatusSummary
}

func (m *mockStatusProvider) GetStatus(repoName string) (*StatusSummary, error) {
	if status, ok := m.statuses[repoName]; ok {
		return status, nil
	}
	return nil, &testError{msg: "repo not found"}
}

func (m *mockStatusProvider) ListRepos() []string {
	return m.repos
}

// testError is a simple error implementation for testing
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
