package coordination

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		config  ClientConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: ClientConfig{
				BaseURL:  "https://api.example.com",
				APIToken: "test-token",
			},
			wantErr: false,
		},
		{
			name: "missing base URL",
			config: ClientConfig{
				APIToken: "test-token",
			},
			wantErr: true,
		},
		{
			name: "invalid URL",
			config: ClientConfig{
				BaseURL: "://invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.config)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if client == nil {
				t.Error("expected client, got nil")
			}
		})
	}
}

func TestNewClientFromHybridConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  HybridConfig
		wantErr bool
	}{
		{
			name: "enabled config",
			config: HybridConfig{
				Enabled:            true,
				CoordinationAPIURL: "https://api.example.com",
				APIToken:           "token",
			},
			wantErr: false,
		},
		{
			name: "disabled config",
			config: HybridConfig{
				Enabled: false,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewClientFromHybridConfig(tt.config)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestClientCache(t *testing.T) {
	cache := newClientCache(100 * time.Millisecond)

	agent := &AgentInfo{
		Name:     "test-agent",
		Type:     "worker",
		RepoName: "test-repo",
		Location: LocationLocal,
		Status:   StatusActive,
	}

	// Test Set and Get
	cache.Set("test-repo", "test-agent", agent)
	got, ok := cache.Get("test-repo", "test-agent")
	if !ok {
		t.Error("expected cache hit, got miss")
	}
	if got.Name != agent.Name {
		t.Errorf("got name %q, want %q", got.Name, agent.Name)
	}

	// Test Get for non-existent entry
	_, ok = cache.Get("test-repo", "non-existent")
	if ok {
		t.Error("expected cache miss for non-existent entry")
	}

	// Test TTL expiration
	time.Sleep(150 * time.Millisecond)
	_, ok = cache.Get("test-repo", "test-agent")
	if ok {
		t.Error("expected cache miss after TTL expiration")
	}

	// Test Delete
	cache.Set("test-repo", "test-agent", agent)
	cache.Delete("test-repo", "test-agent")
	_, ok = cache.Get("test-repo", "test-agent")
	if ok {
		t.Error("expected cache miss after delete")
	}

	// Test Clear
	cache.Set("test-repo", "agent1", agent)
	cache.Set("test-repo", "agent2", agent)
	cache.Clear()
	_, ok = cache.Get("test-repo", "agent1")
	if ok {
		t.Error("expected cache miss after clear")
	}
}

func TestClientRegister(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/agents" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		// Check auth header
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("unexpected auth header: %s", auth)
		}

		// Parse request body
		var agent AgentInfo
		if err := json.NewDecoder(r.Body).Decode(&agent); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}

		// Return success response
		resp := apiResponse{
			Success: true,
			Data:    json.RawMessage(`{"agent": {"name": "test-agent", "type": "worker", "repo_name": "test-repo"}}`),
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		BaseURL:     server.URL,
		APIToken:    "test-token",
		EnableCache: true,
	})

	agent := &AgentInfo{
		Name:     "test-agent",
		Type:     "worker",
		RepoName: "test-repo",
		Location: LocationLocal,
	}

	err := client.Register(agent)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestClientRegisterValidation(t *testing.T) {
	client, _ := NewClient(ClientConfig{
		BaseURL: "https://api.example.com",
	})

	// Missing name
	err := client.Register(&AgentInfo{RepoName: "repo"})
	if err == nil {
		t.Error("expected error for missing name")
	}

	// Missing repo
	err = client.Register(&AgentInfo{Name: "agent"})
	if err == nil {
		t.Error("expected error for missing repo")
	}
}

func TestClientGet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/agents/test-repo/test-agent" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := apiResponse{
			Success: true,
			Data:    json.RawMessage(`{"agent": {"name": "test-agent", "type": "worker", "repo_name": "test-repo", "status": "active"}}`),
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		BaseURL:     server.URL,
		EnableCache: true,
	})

	agent, err := client.Get("test-repo", "test-agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if agent.Name != "test-agent" {
		t.Errorf("got name %q, want %q", agent.Name, "test-agent")
	}
	if agent.Status != StatusActive {
		t.Errorf("got status %q, want %q", agent.Status, StatusActive)
	}
}

func TestClientGetFromCache(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		resp := apiResponse{
			Success: true,
			Data:    json.RawMessage(`{"agent": {"name": "test-agent", "type": "worker", "repo_name": "test-repo"}}`),
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		BaseURL:     server.URL,
		EnableCache: true,
		CacheTTL:    5 * time.Second,
	})

	// First call should hit API
	_, err := client.Get("test-repo", "test-agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 API call, got %d", callCount)
	}

	// Second call should hit cache
	_, err = client.Get("test-repo", "test-agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 API call (cached), got %d", callCount)
	}
}

func TestClientList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agents/test-repo" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := apiResponse{
			Success: true,
			Data:    json.RawMessage(`{"agents": [{"name": "agent1", "type": "worker"}, {"name": "agent2", "type": "supervisor"}]}`),
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{BaseURL: server.URL})

	agents, err := client.List("test-repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(agents) != 2 {
		t.Errorf("got %d agents, want 2", len(agents))
	}
}

func TestClientListByType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		typeParam := r.URL.Query().Get("type")
		if typeParam != "worker" {
			t.Errorf("expected type=worker, got %s", typeParam)
		}

		resp := apiResponse{
			Success: true,
			Data:    json.RawMessage(`{"agents": [{"name": "worker1", "type": "worker"}]}`),
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{BaseURL: server.URL})

	agents, err := client.ListByType("test-repo", "worker")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(agents) != 1 {
		t.Errorf("got %d agents, want 1", len(agents))
	}
}

func TestClientListByLocation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		locParam := r.URL.Query().Get("location")
		if locParam != "remote" {
			t.Errorf("expected location=remote, got %s", locParam)
		}

		resp := apiResponse{
			Success: true,
			Data:    json.RawMessage(`{"agents": [{"name": "remote1", "type": "worker", "location": "remote"}]}`),
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{BaseURL: server.URL})

	agents, err := client.ListByLocation("test-repo", LocationRemote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(agents) != 1 {
		t.Errorf("got %d agents, want 1", len(agents))
	}
}

func TestClientUnregister(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/agents/test-repo/test-agent" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := apiResponse{Success: true}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		BaseURL:     server.URL,
		EnableCache: true,
	})

	// Pre-populate cache
	client.cache.Set("test-repo", "test-agent", &AgentInfo{Name: "test-agent"})

	err := client.Unregister("test-repo", "test-agent")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Check cache is cleared
	_, ok := client.cache.Get("test-repo", "test-agent")
	if ok {
		t.Error("cache should be cleared after unregister")
	}
}

func TestClientUpdateHeartbeat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/agents/test-repo/test-agent/heartbeat" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := apiResponse{Success: true}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{BaseURL: server.URL})

	err := client.UpdateHeartbeat("test-repo", "test-agent")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestClientUpdateStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/agents/test-repo/test-agent/status" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		// Check request body
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["status"] != "busy" {
			t.Errorf("expected status=busy, got %s", body["status"])
		}

		resp := apiResponse{Success: true}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{BaseURL: server.URL})

	err := client.UpdateStatus("test-repo", "test-agent", StatusBusy)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestClientRequestSpawn(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/spawn" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := apiResponse{
			Success: true,
			Data:    json.RawMessage(`{"spawn": {"worker_name": "new-worker", "location": "remote"}}`),
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{BaseURL: server.URL})

	req := SpawnRequest{
		RepoName:  "test-repo",
		Task:      "Test task",
		SpawnedBy: "supervisor",
	}

	spawnResp, err := client.RequestSpawn(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spawnResp.WorkerName != "new-worker" {
		t.Errorf("got worker name %q, want %q", spawnResp.WorkerName, "new-worker")
	}
	if spawnResp.Location != LocationRemote {
		t.Errorf("got location %q, want %q", spawnResp.Location, LocationRemote)
	}
}

func TestClientRequestSpawnValidation(t *testing.T) {
	client, _ := NewClient(ClientConfig{BaseURL: "https://api.example.com"})

	// Missing repo
	_, err := client.RequestSpawn(SpawnRequest{Task: "test"})
	if err == nil {
		t.Error("expected error for missing repo")
	}

	// Missing task
	_, err = client.RequestSpawn(SpawnRequest{RepoName: "repo"})
	if err == nil {
		t.Error("expected error for missing task")
	}
}

func TestClientSendMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/messages" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := apiResponse{Success: true}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{BaseURL: server.URL})

	msg := &RoutedMessage{
		From:     "supervisor",
		To:       "worker",
		RepoName: "test-repo",
		Body:     "Hello",
	}

	err := client.SendMessage(msg)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestClientSendMessageValidation(t *testing.T) {
	client, _ := NewClient(ClientConfig{BaseURL: "https://api.example.com"})

	// Missing from
	err := client.SendMessage(&RoutedMessage{To: "agent", RepoName: "repo"})
	if err == nil {
		t.Error("expected error for missing from")
	}

	// Missing to
	err = client.SendMessage(&RoutedMessage{From: "agent", RepoName: "repo"})
	if err == nil {
		t.Error("expected error for missing to")
	}

	// Missing repo
	err = client.SendMessage(&RoutedMessage{From: "a", To: "b"})
	if err == nil {
		t.Error("expected error for missing repo")
	}
}

func TestClientGetMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/messages/test-repo/test-agent" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := apiResponse{
			Success: true,
			Data:    json.RawMessage(`{"messages": [{"id": "msg1", "from": "supervisor", "to": "test-agent", "body": "Hello"}]}`),
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{BaseURL: server.URL})

	msgs, err := client.GetMessages("test-repo", "test-agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(msgs) != 1 {
		t.Errorf("got %d messages, want 1", len(msgs))
	}
}

func TestClientAcknowledgeMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/messages/msg123/ack" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := apiResponse{Success: true}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{BaseURL: server.URL})

	err := client.AcknowledgeMessage("msg123")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestClientPing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/health" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := apiResponse{Success: true}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{BaseURL: server.URL})

	err := client.Ping()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestClientAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		resp := apiResponse{
			Success: false,
			Error:   "invalid token",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{BaseURL: server.URL})

	err := client.Ping()
	if err == nil {
		t.Error("expected error for unauthorized request")
	}
}

func TestHybridRegistry(t *testing.T) {
	local := NewLocalRegistry()

	// Create mock server for remote
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := apiResponse{Success: true}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	remote, _ := NewClient(ClientConfig{BaseURL: server.URL})

	config := HybridConfig{
		Enabled:         true,
		FallbackToLocal: true,
	}

	hybrid := NewHybridRegistry(local, remote, config)

	// Test Register
	agent := &AgentInfo{
		Name:     "test-agent",
		Type:     "worker",
		RepoName: "test-repo",
		Location: LocationLocal,
	}

	err := hybrid.Register(agent)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify agent is in local registry
	got, err := local.Get("test-repo", "test-agent")
	if err != nil {
		t.Errorf("agent should be in local registry: %v", err)
	}
	if got.Name != "test-agent" {
		t.Errorf("got name %q, want %q", got.Name, "test-agent")
	}
}

func TestHybridRegistryFallback(t *testing.T) {
	local := NewLocalRegistry()

	// Create a server that always fails
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	remote, _ := NewClient(ClientConfig{BaseURL: server.URL})

	config := HybridConfig{
		Enabled:         true,
		FallbackToLocal: true,
	}

	hybrid := NewHybridRegistry(local, remote, config)

	// Register should succeed (fallback to local)
	agent := &AgentInfo{
		Name:     "test-agent",
		Type:     "worker",
		RepoName: "test-repo",
		Location: LocationLocal,
	}

	err := hybrid.Register(agent)
	if err != nil {
		t.Errorf("register should succeed with fallback: %v", err)
	}

	// Get should also work (fallback to local)
	got, err := hybrid.Get("test-repo", "test-agent")
	if err != nil {
		t.Errorf("get should succeed with fallback: %v", err)
	}
	if got.Name != "test-agent" {
		t.Errorf("got name %q, want %q", got.Name, "test-agent")
	}
}

func TestHybridRegistryDisabled(t *testing.T) {
	local := NewLocalRegistry()

	config := HybridConfig{
		Enabled: false,
	}

	hybrid := NewHybridRegistry(local, nil, config)

	agent := &AgentInfo{
		Name:     "test-agent",
		Type:     "worker",
		RepoName: "test-repo",
		Location: LocationLocal,
	}

	// Should work with just local registry
	err := hybrid.Register(agent)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	got, err := hybrid.Get("test-repo", "test-agent")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if got.Name != "test-agent" {
		t.Errorf("got name %q, want %q", got.Name, "test-agent")
	}
}

func TestClientImplementsRegistry(t *testing.T) {
	// This test verifies at compile time that Client implements Registry
	var _ Registry = (*Client)(nil)
}

func TestHybridRegistryImplementsRegistry(t *testing.T) {
	// This test verifies at compile time that HybridRegistry implements Registry
	var _ Registry = (*HybridRegistry)(nil)
}
