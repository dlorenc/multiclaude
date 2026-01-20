package coordination

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	client := NewClient("test-token")

	if client.baseURL != DefaultAPIEndpoint {
		t.Errorf("expected default baseURL %q, got %q", DefaultAPIEndpoint, client.baseURL)
	}
	if client.token != "test-token" {
		t.Errorf("expected token 'test-token', got %q", client.token)
	}
}

func TestNewClientWithOptions(t *testing.T) {
	customURL := "https://custom.api.dev"
	client := NewClient("test-token",
		WithBaseURL(customURL),
		WithTimeout(60*time.Second),
	)

	if client.baseURL != customURL {
		t.Errorf("expected baseURL %q, got %q", customURL, client.baseURL)
	}
	if client.httpClient.Timeout != 60*time.Second {
		t.Errorf("expected timeout 60s, got %v", client.httpClient.Timeout)
	}
}

func TestRegisterAgent(t *testing.T) {
	expectedAgent := &Agent{
		Name:      "supervisor",
		Type:      "supervisor",
		Location:  LocationLocal,
		Owner:     "test-user",
		Repo:      "owner/repo",
		Status:    StatusActive,
		CreatedAt: time.Now(),
		LastSeen:  time.Now(),
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/repos/owner/repo/agents" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
			t.Errorf("unexpected auth header: %s", auth)
		}

		// Decode request body
		var req RegisterAgentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}
		if req.Name != "supervisor" {
			t.Errorf("expected name 'supervisor', got %q", req.Name)
		}

		// Return response
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expectedAgent)
	}))
	defer server.Close()

	client := NewClient("test-token", WithBaseURL(server.URL))
	agent, err := client.RegisterAgent("owner/repo", &RegisterAgentRequest{
		Name:     "supervisor",
		Type:     "supervisor",
		Location: LocationLocal,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if agent.Name != expectedAgent.Name {
		t.Errorf("expected name %q, got %q", expectedAgent.Name, agent.Name)
	}
}

func TestListAgents(t *testing.T) {
	agents := []*Agent{
		{Name: "supervisor", Type: "supervisor", Location: LocationLocal},
		{Name: "merge-queue", Type: "merge-queue", Location: LocationRemote},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(agents)
	}))
	defer server.Close()

	client := NewClient("test-token", WithBaseURL(server.URL))
	result, err := client.ListAgents("owner/repo")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 agents, got %d", len(result))
	}
}

func TestSendMessage(t *testing.T) {
	expectedMsg := &Message{
		ID:        "msg-12345",
		From:      "worker-a",
		To:        "supervisor",
		Body:      "Need help with auth",
		Status:    MessagePending,
		CreatedAt: time.Now(),
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		var req SendMessageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}
		if req.From != "worker-a" {
			t.Errorf("expected from 'worker-a', got %q", req.From)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expectedMsg)
	}))
	defer server.Close()

	client := NewClient("test-token", WithBaseURL(server.URL))
	msg, err := client.SendMessage("owner/repo", &SendMessageRequest{
		From: "worker-a",
		To:   "supervisor",
		Body: "Need help with auth",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.ID != expectedMsg.ID {
		t.Errorf("expected ID %q, got %q", expectedMsg.ID, msg.ID)
	}
}

func TestSpawnWorker(t *testing.T) {
	expectedSpawn := &SpawnRequest{
		ID:        "spawn-12345",
		Repo:      "owner/repo",
		Task:      "Fix auth bug",
		SpawnedBy: "workspace:dan",
		Status:    SpawnPending,
		CreatedAt: time.Now(),
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/repos/owner/repo/workers/spawn" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expectedSpawn)
	}))
	defer server.Close()

	client := NewClient("test-token", WithBaseURL(server.URL))
	spawn, err := client.SpawnWorker("owner/repo", &SpawnWorkerRequest{
		Task:      "Fix auth bug",
		SpawnedBy: "workspace:dan",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spawn.ID != expectedSpawn.ID {
		t.Errorf("expected ID %q, got %q", expectedSpawn.ID, spawn.ID)
	}
	if spawn.Status != SpawnPending {
		t.Errorf("expected status %q, got %q", SpawnPending, spawn.Status)
	}
}

func TestAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(APIError{
			Code:    "not_found",
			Message: "Agent not found",
		})
	}))
	defer server.Close()

	client := NewClient("test-token", WithBaseURL(server.URL))
	_, err := client.GetAgent("owner/repo", "nonexistent")

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.Code != "not_found" {
		t.Errorf("expected code 'not_found', got %q", apiErr.Code)
	}
}

func TestDeregisterAgent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient("test-token", WithBaseURL(server.URL))
	err := client.DeregisterAgent("owner/repo", "supervisor")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHeartbeat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH, got %s", r.Method)
		}

		var req UpdateAgentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}
		if req.Status == nil || *req.Status != StatusActive {
			t.Errorf("expected status 'active', got %v", req.Status)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(&Agent{
			Name:     "supervisor",
			Status:   StatusActive,
			LastSeen: time.Now(),
		})
	}))
	defer server.Close()

	client := NewClient("test-token", WithBaseURL(server.URL))
	err := client.Heartbeat("owner/repo", "supervisor", StatusActive)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
