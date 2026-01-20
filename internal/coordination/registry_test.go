package coordination

import (
	"testing"
	"time"
)

func TestLocalRegistry_Register(t *testing.T) {
	registry := NewLocalRegistry()

	agent := &AgentInfo{
		Name:     "test-worker",
		Type:     "worker",
		RepoName: "test-repo",
		Location: LocationLocal,
		Status:   StatusActive,
	}

	err := registry.Register(agent)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Verify registration
	got, err := registry.Get("test-repo", "test-worker")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got.Name != "test-worker" {
		t.Errorf("Name = %q, want %q", got.Name, "test-worker")
	}
	if got.Ownership != OwnershipTask {
		t.Errorf("Ownership = %q, want %q", got.Ownership, OwnershipTask)
	}
	if got.RegisteredAt.IsZero() {
		t.Error("RegisteredAt should be set")
	}
}

func TestLocalRegistry_RegisterValidation(t *testing.T) {
	registry := NewLocalRegistry()

	// Missing name
	err := registry.Register(&AgentInfo{RepoName: "repo"})
	if err == nil {
		t.Error("expected error for missing name")
	}

	// Missing repo
	err = registry.Register(&AgentInfo{Name: "agent"})
	if err == nil {
		t.Error("expected error for missing repo")
	}
}

func TestLocalRegistry_Unregister(t *testing.T) {
	registry := NewLocalRegistry()

	agent := &AgentInfo{
		Name:     "test-worker",
		Type:     "worker",
		RepoName: "test-repo",
		Location: LocationLocal,
	}

	registry.Register(agent)

	err := registry.Unregister("test-repo", "test-worker")
	if err != nil {
		t.Fatalf("Unregister failed: %v", err)
	}

	// Verify removed
	_, err = registry.Get("test-repo", "test-worker")
	if err == nil {
		t.Error("expected error after unregister")
	}
}

func TestLocalRegistry_List(t *testing.T) {
	registry := NewLocalRegistry()

	// Register multiple agents
	agents := []*AgentInfo{
		{Name: "supervisor", Type: "supervisor", RepoName: "test-repo", Location: LocationLocal},
		{Name: "worker-1", Type: "worker", RepoName: "test-repo", Location: LocationLocal},
		{Name: "worker-2", Type: "worker", RepoName: "test-repo", Location: LocationRemote},
	}

	for _, a := range agents {
		registry.Register(a)
	}

	// List all
	result, err := registry.List("test-repo")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("List returned %d agents, want 3", len(result))
	}
}

func TestLocalRegistry_ListByType(t *testing.T) {
	registry := NewLocalRegistry()

	agents := []*AgentInfo{
		{Name: "supervisor", Type: "supervisor", RepoName: "test-repo", Location: LocationLocal},
		{Name: "worker-1", Type: "worker", RepoName: "test-repo", Location: LocationLocal},
		{Name: "worker-2", Type: "worker", RepoName: "test-repo", Location: LocationRemote},
	}

	for _, a := range agents {
		registry.Register(a)
	}

	// List workers only
	workers, err := registry.ListByType("test-repo", "worker")
	if err != nil {
		t.Fatalf("ListByType failed: %v", err)
	}
	if len(workers) != 2 {
		t.Errorf("ListByType returned %d workers, want 2", len(workers))
	}
}

func TestLocalRegistry_ListByLocation(t *testing.T) {
	registry := NewLocalRegistry()

	agents := []*AgentInfo{
		{Name: "supervisor", Type: "supervisor", RepoName: "test-repo", Location: LocationLocal},
		{Name: "worker-1", Type: "worker", RepoName: "test-repo", Location: LocationLocal},
		{Name: "worker-2", Type: "worker", RepoName: "test-repo", Location: LocationRemote},
	}

	for _, a := range agents {
		registry.Register(a)
	}

	// List local agents
	local, err := registry.ListByLocation("test-repo", LocationLocal)
	if err != nil {
		t.Fatalf("ListByLocation failed: %v", err)
	}
	if len(local) != 2 {
		t.Errorf("ListByLocation returned %d local agents, want 2", len(local))
	}

	// List remote agents
	remote, err := registry.ListByLocation("test-repo", LocationRemote)
	if err != nil {
		t.Fatalf("ListByLocation failed: %v", err)
	}
	if len(remote) != 1 {
		t.Errorf("ListByLocation returned %d remote agents, want 1", len(remote))
	}
}

func TestLocalRegistry_UpdateHeartbeat(t *testing.T) {
	registry := NewLocalRegistry()

	agent := &AgentInfo{
		Name:     "test-worker",
		Type:     "worker",
		RepoName: "test-repo",
		Location: LocationLocal,
	}
	registry.Register(agent)

	// Sleep briefly to ensure time difference
	time.Sleep(10 * time.Millisecond)

	err := registry.UpdateHeartbeat("test-repo", "test-worker")
	if err != nil {
		t.Fatalf("UpdateHeartbeat failed: %v", err)
	}

	got, _ := registry.Get("test-repo", "test-worker")
	if got.LastHeartbeat.Before(agent.LastHeartbeat) {
		t.Error("LastHeartbeat should be updated")
	}
}

func TestLocalRegistry_UpdateStatus(t *testing.T) {
	registry := NewLocalRegistry()

	agent := &AgentInfo{
		Name:     "test-worker",
		Type:     "worker",
		RepoName: "test-repo",
		Location: LocationLocal,
		Status:   StatusActive,
	}
	registry.Register(agent)

	err := registry.UpdateStatus("test-repo", "test-worker", StatusBusy)
	if err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	got, _ := registry.Get("test-repo", "test-worker")
	if got.Status != StatusBusy {
		t.Errorf("Status = %q, want %q", got.Status, StatusBusy)
	}
}

func TestLocalRegistry_GetStaleAgents(t *testing.T) {
	registry := NewLocalRegistry()

	// Register an agent with old heartbeat
	agent := &AgentInfo{
		Name:          "stale-worker",
		Type:          "worker",
		RepoName:      "test-repo",
		Location:      LocationLocal,
		Status:        StatusActive,
		LastHeartbeat: time.Now().Add(-5 * time.Minute),
	}
	registry.mu.Lock()
	if registry.agents["test-repo"] == nil {
		registry.agents["test-repo"] = make(map[string]*AgentInfo)
	}
	registry.agents["test-repo"]["stale-worker"] = agent
	registry.mu.Unlock()

	// Register a fresh agent
	fresh := &AgentInfo{
		Name:     "fresh-worker",
		Type:     "worker",
		RepoName: "test-repo",
		Location: LocationLocal,
		Status:   StatusActive,
	}
	registry.Register(fresh)

	// Check for stale agents (threshold 2 minutes)
	stale, err := registry.GetStaleAgents("test-repo", 2*time.Minute)
	if err != nil {
		t.Fatalf("GetStaleAgents failed: %v", err)
	}

	if len(stale) != 1 {
		t.Errorf("GetStaleAgents returned %d agents, want 1", len(stale))
	}

	if len(stale) > 0 && stale[0].Name != "stale-worker" {
		t.Errorf("Expected stale-worker, got %s", stale[0].Name)
	}
}

func TestLocalRegistry_Clear(t *testing.T) {
	registry := NewLocalRegistry()

	agents := []*AgentInfo{
		{Name: "agent-1", Type: "worker", RepoName: "test-repo", Location: LocationLocal},
		{Name: "agent-2", Type: "worker", RepoName: "test-repo", Location: LocationLocal},
	}

	for _, a := range agents {
		registry.Register(a)
	}

	registry.Clear("test-repo")

	result, _ := registry.List("test-repo")
	if len(result) != 0 {
		t.Errorf("Clear should remove all agents, got %d", len(result))
	}
}

func TestLocalRegistry_GetNonExistent(t *testing.T) {
	registry := NewLocalRegistry()

	// Non-existent repo
	_, err := registry.Get("no-repo", "agent")
	if err == nil {
		t.Error("expected error for non-existent repo")
	}

	// Non-existent agent
	registry.Register(&AgentInfo{Name: "exists", Type: "worker", RepoName: "repo"})
	_, err = registry.Get("repo", "no-agent")
	if err == nil {
		t.Error("expected error for non-existent agent")
	}
}
