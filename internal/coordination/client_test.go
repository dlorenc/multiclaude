package coordination

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	c := NewClient("http://localhost:7331", "test-token")

	if c.baseURL != "http://localhost:7331" {
		t.Errorf("expected baseURL 'http://localhost:7331', got '%s'", c.baseURL)
	}
	if c.token != "test-token" {
		t.Errorf("expected token 'test-token', got '%s'", c.token)
	}
}

func TestNewClientWithConfig(t *testing.T) {
	config := &ClientConfig{
		ServerURL:  "http://localhost:7331",
		Token:      "test-token",
		NodeID:     "test-node",
		MaxWorkers: 5,
	}

	c := NewClientWithConfig(config)

	if c.baseURL != "http://localhost:7331" {
		t.Errorf("expected baseURL 'http://localhost:7331', got '%s'", c.baseURL)
	}
	if c.GetNodeID() != "test-node" {
		t.Errorf("expected nodeID 'test-node', got '%s'", c.GetNodeID())
	}
}

func TestClient_Register(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/register" {
			t.Errorf("expected path '/api/v1/register', got '%s'", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got '%s'", r.Method)
		}

		var req RegisterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}

		if req.NodeID == "" {
			t.Error("expected node_id to be set")
		}

		resp := APIResponse{
			Success: true,
			Data: RegisterResponse{
				RegistrationID:           "reg-12345678",
				HeartbeatIntervalSeconds: 30,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(server.URL, "test-token")

	req := &RegisterRequest{
		NodeID:   "test-node",
		Hostname: "localhost",
		Capacity: NodeCapacity{MaxWorkers: 5},
	}

	resp, err := c.Register(context.Background(), req)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if resp.RegistrationID == "" {
		t.Error("expected registration_id")
	}
	if resp.HeartbeatIntervalSeconds != 30 {
		t.Errorf("expected heartbeat_interval_seconds 30, got %d", resp.HeartbeatIntervalSeconds)
	}
	if !c.IsRegistered() {
		t.Error("expected client to be registered")
	}
}

func TestClient_Heartbeat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/register":
			resp := APIResponse{
				Success: true,
				Data: RegisterResponse{
					RegistrationID:           "reg-12345678",
					HeartbeatIntervalSeconds: 30,
				},
			}
			json.NewEncoder(w).Encode(resp)
		case "/api/v1/heartbeat":
			var req HeartbeatRequest
			json.NewDecoder(r.Body).Decode(&req)

			if req.RegistrationID != "reg-12345678" {
				t.Errorf("expected registration_id 'reg-12345678', got '%s'", req.RegistrationID)
			}

			resp := APIResponse{
				Success: true,
				Data: HeartbeatResponse{
					Acknowledged: true,
				},
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	c := NewClient(server.URL, "test-token")

	// Register first
	_, err := c.Register(context.Background(), &RegisterRequest{
		NodeID:   "test-node",
		Hostname: "localhost",
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Send heartbeat
	err = c.Heartbeat(context.Background(), &HeartbeatRequest{
		Status: NodeStatusOnline,
	})
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}
}

func TestClient_HeartbeatNotRegistered(t *testing.T) {
	c := NewClient("http://localhost:7331", "test-token")

	err := c.Heartbeat(context.Background(), &HeartbeatRequest{
		Status: NodeStatusOnline,
	})

	if err == nil {
		t.Error("expected error when not registered")
	}
}

func TestClient_GetPendingTasks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/tasks/pending" {
			t.Errorf("expected path '/api/v1/tasks/pending', got '%s'", r.URL.Path)
		}

		resp := APIResponse{
			Success: true,
			Data: map[string]interface{}{
				"tasks": []Task{
					{
						ID:          "task-1",
						Repo:        "my-repo",
						Description: "Test task",
						Priority:    PriorityMedium,
						Status:      TaskStatusPending,
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(server.URL, "test-token")

	tasks, err := c.GetPendingTasks(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetPendingTasks failed: %v", err)
	}

	if len(tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(tasks))
	}
}

func TestClient_ClaimTask(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/register":
			resp := APIResponse{
				Success: true,
				Data: RegisterResponse{
					RegistrationID:           "reg-12345678",
					HeartbeatIntervalSeconds: 30,
				},
			}
			json.NewEncoder(w).Encode(resp)
		case "/api/v1/tasks/task-1/claim":
			resp := APIResponse{
				Success: true,
				Data: TaskClaimResponse{
					Claimed: true,
					Task: &Task{
						ID:          "task-1",
						Repo:        "my-repo",
						Description: "Test task",
						Status:      TaskStatusClaimed,
					},
				},
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	c := NewClient(server.URL, "test-token")

	// Register first
	_, err := c.Register(context.Background(), &RegisterRequest{
		NodeID:   "test-node",
		Hostname: "localhost",
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	task, err := c.ClaimTask(context.Background(), "task-1", "worker-1")
	if err != nil {
		t.Fatalf("ClaimTask failed: %v", err)
	}

	if task.ID != "task-1" {
		t.Errorf("expected task ID 'task-1', got '%s'", task.ID)
	}
}

func TestClient_ClaimTaskNotRegistered(t *testing.T) {
	c := NewClient("http://localhost:7331", "test-token")

	_, err := c.ClaimTask(context.Background(), "task-1", "worker-1")
	if err == nil {
		t.Error("expected error when not registered")
	}
}

func TestClient_SendMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/messages" {
			t.Errorf("expected path '/api/v1/messages', got '%s'", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got '%s'", r.Method)
		}

		var msg Message
		json.NewDecoder(r.Body).Decode(&msg)

		if msg.From != "worker-1@node-1" {
			t.Errorf("expected from 'worker-1@node-1', got '%s'", msg.From)
		}

		resp := APIResponse{
			Success: true,
			Data: map[string]string{
				"message_id": msg.ID,
				"status":     "routed",
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(server.URL, "test-token")

	err := c.SendMessage(context.Background(), &Message{
		From: "worker-1@node-1",
		To:   "supervisor@node-1",
		Repo: "my-repo",
		Body: "Test message",
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
}

func TestClient_Health(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := APIResponse{
			Success: true,
			Data: map[string]interface{}{
				"status": "ok",
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(server.URL, "test-token")

	err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("Health check failed: %v", err)
	}
}

func TestClient_ListNodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/nodes" {
			t.Errorf("expected path '/api/v1/nodes', got '%s'", r.URL.Path)
		}

		resp := APIResponse{
			Success: true,
			Data: map[string]interface{}{
				"nodes": []Node{
					{
						ID:       "node-1",
						Hostname: "localhost",
						Status:   NodeStatusOnline,
					},
				},
				"count": 1,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(server.URL, "test-token")

	nodes, err := c.ListNodes(context.Background())
	if err != nil {
		t.Fatalf("ListNodes failed: %v", err)
	}

	if len(nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(nodes))
	}
}

func TestClient_CreateTask(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/tasks" {
			t.Errorf("expected path '/api/v1/tasks', got '%s'", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got '%s'", r.Method)
		}

		var req CreateTaskRequest
		json.NewDecoder(r.Body).Decode(&req)

		task := Task{
			ID:          "task-new",
			Repo:        req.Repo,
			Description: req.Description,
			Priority:    req.Priority,
			Status:      TaskStatusPending,
			CreatedAt:   time.Now(),
		}

		resp := APIResponse{
			Success: true,
			Data:    task,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(server.URL, "test-token")

	task, err := c.CreateTask(context.Background(), &CreateTaskRequest{
		Repo:        "my-repo",
		Description: "New test task",
		Priority:    PriorityHigh,
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	if task.Description != "New test task" {
		t.Errorf("expected description 'New test task', got '%s'", task.Description)
	}
}

func TestClient_GetState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/state/my-repo" {
			t.Errorf("expected path '/api/v1/state/my-repo', got '%s'", r.URL.Path)
		}

		resp := APIResponse{
			Success: true,
			Data: StateResponse{
				Repo: "my-repo",
				Agents: map[string]*AgentState{
					"supervisor": {
						Node:   "node-1",
						Status: "running",
					},
				},
				PendingTasks: 3,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(server.URL, "test-token")

	state, err := c.GetState(context.Background(), "my-repo")
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}

	if state.Repo != "my-repo" {
		t.Errorf("expected repo 'my-repo', got '%s'", state.Repo)
	}
	if state.PendingTasks != 3 {
		t.Errorf("expected 3 pending tasks, got %d", state.PendingTasks)
	}
}

func TestClient_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		resp := APIResponse{
			Success: false,
			Error:   "invalid request",
			Code:    "INVALID_REQUEST",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(server.URL, "test-token")

	_, err := c.GetPendingTasks(context.Background(), nil)
	if err == nil {
		t.Error("expected error for bad request")
	}
}

func TestClient_SetNodeID(t *testing.T) {
	c := NewClient("http://localhost:7331", "test-token")

	c.SetNodeID("my-node")

	if c.GetNodeID() != "my-node" {
		t.Errorf("expected nodeID 'my-node', got '%s'", c.GetNodeID())
	}
}

func TestClient_Close(t *testing.T) {
	c := NewClient("http://localhost:7331", "test-token")

	// Should not panic
	if err := c.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}
