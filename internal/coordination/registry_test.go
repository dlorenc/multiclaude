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

func TestLocalRegistry_UnregisterErrors(t *testing.T) {
	registry := NewLocalRegistry()

	// Non-existent repo
	err := registry.Unregister("no-repo", "agent")
	if err == nil {
		t.Error("expected error for non-existent repo")
	}

	// Non-existent agent in existing repo
	registry.Register(&AgentInfo{Name: "exists", Type: "worker", RepoName: "repo"})
	err = registry.Unregister("repo", "no-agent")
	if err == nil {
		t.Error("expected error for non-existent agent")
	}
}

func TestLocalRegistry_UpdateHeartbeatErrors(t *testing.T) {
	registry := NewLocalRegistry()

	// Non-existent repo
	err := registry.UpdateHeartbeat("no-repo", "agent")
	if err == nil {
		t.Error("expected error for non-existent repo")
	}

	// Non-existent agent in existing repo
	registry.Register(&AgentInfo{Name: "exists", Type: "worker", RepoName: "repo"})
	err = registry.UpdateHeartbeat("repo", "no-agent")
	if err == nil {
		t.Error("expected error for non-existent agent")
	}
}

func TestLocalRegistry_UpdateStatusErrors(t *testing.T) {
	registry := NewLocalRegistry()

	// Non-existent repo
	err := registry.UpdateStatus("no-repo", "agent", StatusActive)
	if err == nil {
		t.Error("expected error for non-existent repo")
	}

	// Non-existent agent in existing repo
	registry.Register(&AgentInfo{Name: "exists", Type: "worker", RepoName: "repo"})
	err = registry.UpdateStatus("repo", "no-agent", StatusActive)
	if err == nil {
		t.Error("expected error for non-existent agent")
	}
}

func TestLocalRegistry_GetStaleAgentsEmptyRepo(t *testing.T) {
	registry := NewLocalRegistry()

	// Non-existent repo returns empty list (not error)
	stale, err := registry.GetStaleAgents("no-repo", 2*time.Minute)
	if err != nil {
		t.Fatalf("GetStaleAgents on empty repo should not error: %v", err)
	}
	if len(stale) != 0 {
		t.Errorf("expected empty list for non-existent repo, got %d", len(stale))
	}
}

func TestLocalRegistry_GetStaleAgentsTerminated(t *testing.T) {
	registry := NewLocalRegistry()

	// Register a terminated agent with old heartbeat - should NOT be returned as stale
	agent := &AgentInfo{
		Name:          "terminated-worker",
		Type:          "worker",
		RepoName:      "test-repo",
		Location:      LocationLocal,
		Status:        StatusTerminated,
		LastHeartbeat: time.Now().Add(-10 * time.Minute),
	}
	registry.mu.Lock()
	if registry.agents["test-repo"] == nil {
		registry.agents["test-repo"] = make(map[string]*AgentInfo)
	}
	registry.agents["test-repo"]["terminated-worker"] = agent
	registry.mu.Unlock()

	stale, err := registry.GetStaleAgents("test-repo", 2*time.Minute)
	if err != nil {
		t.Fatalf("GetStaleAgents failed: %v", err)
	}

	// Terminated agents should not be in stale list
	if len(stale) != 0 {
		t.Errorf("terminated agents should not be returned as stale, got %d", len(stale))
	}
}

func TestLocalRegistry_ListEmptyRepo(t *testing.T) {
	registry := NewLocalRegistry()

	// Non-existent repo returns empty list
	result, err := registry.List("no-repo")
	if err != nil {
		t.Fatalf("List on empty repo should not error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty list, got %d agents", len(result))
	}
}

func TestLocalRegistry_RegisterUpdate(t *testing.T) {
	registry := NewLocalRegistry()

	// Register initial agent
	agent := &AgentInfo{
		Name:     "test-worker",
		Type:     "worker",
		RepoName: "test-repo",
		Location: LocationLocal,
		Status:   StatusActive,
	}
	registry.Register(agent)

	// Get initial registration time
	got, _ := registry.Get("test-repo", "test-worker")
	initialRegTime := got.RegisteredAt

	time.Sleep(10 * time.Millisecond)

	// Re-register with updated status
	agent2 := &AgentInfo{
		Name:         "test-worker",
		Type:         "worker",
		RepoName:     "test-repo",
		Location:     LocationLocal,
		Status:       StatusBusy,
		RegisteredAt: initialRegTime, // Preserve registration time
	}
	registry.Register(agent2)

	// Verify update
	got, _ = registry.Get("test-repo", "test-worker")
	if got.Status != StatusBusy {
		t.Errorf("Status = %q, want %q", got.Status, StatusBusy)
	}
	if got.RegisteredAt != initialRegTime {
		t.Error("RegisteredAt should be preserved on update")
	}
}

func TestLocalRegistry_GetReturnsACopy(t *testing.T) {
	registry := NewLocalRegistry()

	agent := &AgentInfo{
		Name:     "test-worker",
		Type:     "worker",
		RepoName: "test-repo",
		Location: LocationLocal,
		Status:   StatusActive,
	}
	registry.Register(agent)

	// Get the agent
	got, _ := registry.Get("test-repo", "test-worker")

	// Modify the returned copy
	got.Status = StatusBusy

	// Original in registry should be unchanged
	original, _ := registry.Get("test-repo", "test-worker")
	if original.Status == StatusBusy {
		t.Error("Get should return a copy, but modification affected original")
	}
}

func TestLocalRegistry_ListReturnsACopy(t *testing.T) {
	registry := NewLocalRegistry()

	agent := &AgentInfo{
		Name:     "test-worker",
		Type:     "worker",
		RepoName: "test-repo",
		Location: LocationLocal,
		Status:   StatusActive,
	}
	registry.Register(agent)

	// List agents
	list, _ := registry.List("test-repo")

	// Modify the returned copy
	list[0].Status = StatusBusy

	// Original in registry should be unchanged
	original, _ := registry.Get("test-repo", "test-worker")
	if original.Status == StatusBusy {
		t.Error("List should return copies, but modification affected original")
	}
}

func TestLocalRegistry_ConcurrentAccess(t *testing.T) {
	registry := NewLocalRegistry()

	// Pre-register the agent
	agent := &AgentInfo{
		Name:     "concurrent-worker",
		Type:     "worker",
		RepoName: "test-repo",
		Location: LocationLocal,
		Status:   StatusActive,
	}
	registry.Register(agent)

	// Run concurrent operations
	done := make(chan bool)
	iterations := 100

	// Concurrent heartbeat updates
	go func() {
		for i := 0; i < iterations; i++ {
			registry.UpdateHeartbeat("test-repo", "concurrent-worker")
		}
		done <- true
	}()

	// Concurrent status updates
	go func() {
		for i := 0; i < iterations; i++ {
			status := StatusActive
			if i%2 == 0 {
				status = StatusBusy
			}
			registry.UpdateStatus("test-repo", "concurrent-worker", status)
		}
		done <- true
	}()

	// Concurrent reads
	go func() {
		for i := 0; i < iterations; i++ {
			registry.Get("test-repo", "concurrent-worker")
		}
		done <- true
	}()

	// Concurrent lists
	go func() {
		for i := 0; i < iterations; i++ {
			registry.List("test-repo")
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 4; i++ {
		<-done
	}

	// Verify agent is still accessible
	got, err := registry.Get("test-repo", "concurrent-worker")
	if err != nil {
		t.Fatalf("agent should still exist: %v", err)
	}
	if got.Name != "concurrent-worker" {
		t.Errorf("Name = %q, want %q", got.Name, "concurrent-worker")
	}
}

func TestLocalRegistry_ConcurrentRegisterUnregister(t *testing.T) {
	registry := NewLocalRegistry()

	done := make(chan bool)
	iterations := 50

	// Concurrent registrations
	go func() {
		for i := 0; i < iterations; i++ {
			agent := &AgentInfo{
				Name:     "worker-a",
				Type:     "worker",
				RepoName: "test-repo",
				Location: LocationLocal,
			}
			registry.Register(agent)
		}
		done <- true
	}()

	// Concurrent unregistrations (may fail, that's ok)
	go func() {
		for i := 0; i < iterations; i++ {
			registry.Unregister("test-repo", "worker-a")
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 2; i++ {
		<-done
	}

	// Test should complete without panic or deadlock
}

func TestLocalRegistry_MultipleRepos(t *testing.T) {
	registry := NewLocalRegistry()

	// Register agents in different repos
	agent1 := &AgentInfo{Name: "worker-1", Type: "worker", RepoName: "repo-a", Location: LocationLocal}
	agent2 := &AgentInfo{Name: "worker-2", Type: "worker", RepoName: "repo-b", Location: LocationLocal}
	agent3 := &AgentInfo{Name: "worker-3", Type: "worker", RepoName: "repo-a", Location: LocationLocal}

	registry.Register(agent1)
	registry.Register(agent2)
	registry.Register(agent3)

	// List repo-a
	repoA, _ := registry.List("repo-a")
	if len(repoA) != 2 {
		t.Errorf("repo-a should have 2 agents, got %d", len(repoA))
	}

	// List repo-b
	repoB, _ := registry.List("repo-b")
	if len(repoB) != 1 {
		t.Errorf("repo-b should have 1 agent, got %d", len(repoB))
	}

	// Clear repo-a
	registry.Clear("repo-a")

	// repo-a should be empty
	repoA, _ = registry.List("repo-a")
	if len(repoA) != 0 {
		t.Errorf("repo-a should be empty after Clear, got %d", len(repoA))
	}

	// repo-b should be unaffected
	repoB, _ = registry.List("repo-b")
	if len(repoB) != 1 {
		t.Errorf("repo-b should still have 1 agent, got %d", len(repoB))
	}
}

func TestLocalRegistry_ImplementsRegistryInterface(t *testing.T) {
	// Compile-time check that LocalRegistry implements Registry
	var _ Registry = (*LocalRegistry)(nil)

	// Also test instantiation
	var registry Registry = NewLocalRegistry()

	agent := &AgentInfo{
		Name:     "test-worker",
		Type:     "worker",
		RepoName: "test-repo",
		Location: LocationLocal,
	}

	err := registry.Register(agent)
	if err != nil {
		t.Fatalf("Register via interface failed: %v", err)
	}

	got, err := registry.Get("test-repo", "test-worker")
	if err != nil {
		t.Fatalf("Get via interface failed: %v", err)
	}
	if got.Name != "test-worker" {
		t.Errorf("Name = %q, want %q", got.Name, "test-worker")
	}
}

func TestLocalRegistry_AutoOwnership(t *testing.T) {
	registry := NewLocalRegistry()

	tests := []struct {
		agentType string
		expected  OwnershipLevel
	}{
		{"supervisor", OwnershipRepo},
		{"merge-queue", OwnershipRepo},
		{"workspace", OwnershipUser},
		{"worker", OwnershipTask},
	}

	for _, tt := range tests {
		t.Run(tt.agentType, func(t *testing.T) {
			agent := &AgentInfo{
				Name:     tt.agentType + "-agent",
				Type:     tt.agentType,
				RepoName: "test-repo",
				Location: LocationLocal,
				// Ownership intentionally not set
			}
			registry.Register(agent)

			got, _ := registry.Get("test-repo", tt.agentType+"-agent")
			if got.Ownership != tt.expected {
				t.Errorf("auto Ownership for %s = %q, want %q", tt.agentType, got.Ownership, tt.expected)
			}

			// Clean up
			registry.Unregister("test-repo", tt.agentType+"-agent")
		})
	}
}
