package state

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestNewState(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)
	if s == nil {
		t.Fatal("New() returned nil")
	}

	if s.Repos == nil {
		t.Error("Repos map not initialized")
	}

	if len(s.Repos) != 0 {
		t.Errorf("Repos length = %d, want 0", len(s.Repos))
	}
}

func TestStateSaveLoad(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	// Create state and add a repo
	s := New(statePath)
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "multiclaude-test-repo",
		Agents:      make(map[string]Agent),
	}

	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	// Add an agent
	agent := Agent{
		Type:         AgentTypeSupervisor,
		WorktreePath: "/path/to/worktree",
		TmuxWindow:   "supervisor",
		SessionID:    "test-session",
		PID:          12345,
		CreatedAt:    time.Now(),
	}

	if err := s.AddAgent("test-repo", "supervisor", agent); err != nil {
		t.Fatalf("AddAgent() failed: %v", err)
	}

	// Load state from disk
	loaded, err := Load(statePath)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify repo was loaded
	loadedRepo, exists := loaded.GetRepo("test-repo")
	if !exists {
		t.Fatal("Repository not found after load")
	}

	if loadedRepo.GithubURL != repo.GithubURL {
		t.Errorf("GithubURL = %q, want %q", loadedRepo.GithubURL, repo.GithubURL)
	}

	// Verify agent was loaded
	loadedAgent, exists := loaded.GetAgent("test-repo", "supervisor")
	if !exists {
		t.Fatal("Agent not found after load")
	}

	if loadedAgent.Type != agent.Type {
		t.Errorf("Agent Type = %q, want %q", loadedAgent.Type, agent.Type)
	}

	if loadedAgent.PID != agent.PID {
		t.Errorf("Agent PID = %d, want %d", loadedAgent.PID, agent.PID)
	}
}

func TestLoadNonExistentState(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "nonexistent.json")

	s, err := Load(statePath)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if len(s.Repos) != 0 {
		t.Errorf("Repos length = %d, want 0 for new state", len(s.Repos))
	}
}

func TestAddRepoDuplicate(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "multiclaude-test-repo",
		Agents:      make(map[string]Agent),
	}

	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	// Adding again should fail
	if err := s.AddRepo("test-repo", repo); err == nil {
		t.Error("AddRepo() succeeded for duplicate repo")
	}
}

func TestGetRepoNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	_, exists := s.GetRepo("nonexistent")
	if exists {
		t.Error("GetRepo() found nonexistent repo")
	}
}

func TestRemoveRepo(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Add a repo
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "multiclaude-test-repo",
		Agents:      make(map[string]Agent),
	}
	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	// Verify it exists
	_, exists := s.GetRepo("test-repo")
	if !exists {
		t.Fatal("Repository not found after add")
	}

	// Remove it
	if err := s.RemoveRepo("test-repo"); err != nil {
		t.Fatalf("RemoveRepo() failed: %v", err)
	}

	// Verify it's gone
	_, exists = s.GetRepo("test-repo")
	if exists {
		t.Error("Repository still exists after removal")
	}
}

func TestRemoveRepoNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Removing a non-existent repo should fail
	if err := s.RemoveRepo("nonexistent"); err == nil {
		t.Error("RemoveRepo() succeeded for nonexistent repo")
	}
}

func TestListRepos(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Empty list
	repos := s.ListRepos()
	if len(repos) != 0 {
		t.Errorf("ListRepos() length = %d, want 0", len(repos))
	}

	// Add repos
	for i, name := range []string{"repo1", "repo2", "repo3"} {
		repo := &Repository{
			GithubURL:   "https://github.com/test/" + name,
			TmuxSession: "multiclaude-" + name,
			Agents:      make(map[string]Agent),
		}
		if err := s.AddRepo(name, repo); err != nil {
			t.Fatalf("AddRepo(%d) failed: %v", i, err)
		}
	}

	repos = s.ListRepos()
	if len(repos) != 3 {
		t.Errorf("ListRepos() length = %d, want 3", len(repos))
	}
}

func TestClearAllAgents(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Add repos with agents
	for _, name := range []string{"repo1", "repo2"} {
		repo := &Repository{
			GithubURL:   "https://github.com/test/" + name,
			TmuxSession: "multiclaude-" + name,
			Agents:      make(map[string]Agent),
		}
		if err := s.AddRepo(name, repo); err != nil {
			t.Fatalf("AddRepo() failed: %v", err)
		}

		// Add agents to each repo
		agent := Agent{
			Type:         AgentTypeSupervisor,
			WorktreePath: "/path/to/worktree",
			TmuxWindow:   "supervisor",
			SessionID:    "test-session",
			PID:          12345,
			CreatedAt:    time.Now(),
		}
		if err := s.AddAgent(name, "supervisor", agent); err != nil {
			t.Fatalf("AddAgent() failed: %v", err)
		}

		worker := Agent{
			Type:         AgentTypeWorker,
			WorktreePath: "/path/to/worker",
			TmuxWindow:   "worker-1",
			SessionID:    "test-session",
			PID:          12346,
			Task:         "Test task",
			CreatedAt:    time.Now(),
		}
		if err := s.AddAgent(name, "worker-1", worker); err != nil {
			t.Fatalf("AddAgent() failed: %v", err)
		}
	}

	// Verify agents exist
	agents1, _ := s.ListAgents("repo1")
	agents2, _ := s.ListAgents("repo2")
	if len(agents1) != 2 || len(agents2) != 2 {
		t.Fatalf("Expected 2 agents per repo, got %d and %d", len(agents1), len(agents2))
	}

	// Clear all agents
	if err := s.ClearAllAgents(); err != nil {
		t.Fatalf("ClearAllAgents() failed: %v", err)
	}

	// Verify agents are cleared but repos remain
	repos := s.ListRepos()
	if len(repos) != 2 {
		t.Errorf("ClearAllAgents() removed repos, got %d want 2", len(repos))
	}

	agents1, _ = s.ListAgents("repo1")
	agents2, _ = s.ListAgents("repo2")
	if len(agents1) != 0 || len(agents2) != 0 {
		t.Errorf("ClearAllAgents() did not clear agents, got %d and %d", len(agents1), len(agents2))
	}

	// Verify state was persisted
	loaded, err := Load(statePath)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	loadedAgents1, _ := loaded.ListAgents("repo1")
	loadedAgents2, _ := loaded.ListAgents("repo2")
	if len(loadedAgents1) != 0 || len(loadedAgents2) != 0 {
		t.Errorf("ClearAllAgents() did not persist, got %d and %d agents", len(loadedAgents1), len(loadedAgents2))
	}
}

func TestAddAgentNonExistentRepo(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	agent := Agent{
		Type:         AgentTypeSupervisor,
		WorktreePath: "/path/to/worktree",
		TmuxWindow:   "supervisor",
		SessionID:    "test-session",
		PID:          12345,
		CreatedAt:    time.Now(),
	}

	if err := s.AddAgent("nonexistent", "supervisor", agent); err == nil {
		t.Error("AddAgent() succeeded for nonexistent repo")
	}
}

func TestAddAgentDuplicate(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "multiclaude-test-repo",
		Agents:      make(map[string]Agent),
	}

	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	agent := Agent{
		Type:         AgentTypeSupervisor,
		WorktreePath: "/path/to/worktree",
		TmuxWindow:   "supervisor",
		SessionID:    "test-session",
		PID:          12345,
		CreatedAt:    time.Now(),
	}

	if err := s.AddAgent("test-repo", "supervisor", agent); err != nil {
		t.Fatalf("AddAgent() failed: %v", err)
	}

	// Adding again should fail
	if err := s.AddAgent("test-repo", "supervisor", agent); err == nil {
		t.Error("AddAgent() succeeded for duplicate agent")
	}
}

func TestUpdateAgent(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "multiclaude-test-repo",
		Agents:      make(map[string]Agent),
	}

	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	agent := Agent{
		Type:         AgentTypeWorker,
		WorktreePath: "/path/to/worktree",
		TmuxWindow:   "worker",
		SessionID:    "test-session",
		PID:          12345,
		Task:         "Original task",
		CreatedAt:    time.Now(),
	}

	if err := s.AddAgent("test-repo", "worker", agent); err != nil {
		t.Fatalf("AddAgent() failed: %v", err)
	}

	// Update the agent
	agent.ReadyForCleanup = true
	if err := s.UpdateAgent("test-repo", "worker", agent); err != nil {
		t.Fatalf("UpdateAgent() failed: %v", err)
	}

	// Verify update
	updated, exists := s.GetAgent("test-repo", "worker")
	if !exists {
		t.Fatal("Agent not found after update")
	}

	if !updated.ReadyForCleanup {
		t.Error("ReadyForCleanup not updated")
	}
}

func TestRemoveAgent(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "multiclaude-test-repo",
		Agents:      make(map[string]Agent),
	}

	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	agent := Agent{
		Type:         AgentTypeSupervisor,
		WorktreePath: "/path/to/worktree",
		TmuxWindow:   "supervisor",
		SessionID:    "test-session",
		PID:          12345,
		CreatedAt:    time.Now(),
	}

	if err := s.AddAgent("test-repo", "supervisor", agent); err != nil {
		t.Fatalf("AddAgent() failed: %v", err)
	}

	// Remove agent
	if err := s.RemoveAgent("test-repo", "supervisor"); err != nil {
		t.Fatalf("RemoveAgent() failed: %v", err)
	}

	// Verify removal
	_, exists := s.GetAgent("test-repo", "supervisor")
	if exists {
		t.Error("Agent still exists after removal")
	}
}

func TestListAgents(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "multiclaude-test-repo",
		Agents:      make(map[string]Agent),
	}

	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	// Empty list
	agents, err := s.ListAgents("test-repo")
	if err != nil {
		t.Fatalf("ListAgents() failed: %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("ListAgents() length = %d, want 0", len(agents))
	}

	// Add agents
	agentNames := []string{"supervisor", "merge-queue", "worker1"}
	for _, name := range agentNames {
		agent := Agent{
			Type:         AgentTypeSupervisor,
			WorktreePath: "/path/" + name,
			TmuxWindow:   name,
			SessionID:    "session-" + name,
			PID:          12345,
			CreatedAt:    time.Now(),
		}
		if err := s.AddAgent("test-repo", name, agent); err != nil {
			t.Fatalf("AddAgent(%s) failed: %v", name, err)
		}
	}

	agents, err = s.ListAgents("test-repo")
	if err != nil {
		t.Fatalf("ListAgents() failed: %v", err)
	}
	if len(agents) != 3 {
		t.Errorf("ListAgents() length = %d, want 3", len(agents))
	}
}

func TestStateAtomicSave(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "multiclaude-test-repo",
		Agents:      make(map[string]Agent),
	}

	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	// Verify temp file was cleaned up
	tmpPath := statePath + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("Temp file not cleaned up after save")
	}

	// Verify state file exists
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		t.Error("State file not created")
	}
}

func TestSave(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Add a repo and agent without relying on AddRepo's auto-save
	s.Repos["test-repo"] = &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "multiclaude-test-repo",
		Agents: map[string]Agent{
			"supervisor": {
				Type:       AgentTypeSupervisor,
				TmuxWindow: "supervisor",
				SessionID:  "test-session",
				PID:        12345,
				CreatedAt:  time.Now(),
			},
		},
	}

	// Manually save
	if err := s.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify file exists and is valid JSON
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("Failed to read saved state file: %v", err)
	}

	if len(data) == 0 {
		t.Error("Saved state file is empty")
	}

	// Verify we can load the saved state
	loaded, err := Load(statePath)
	if err != nil {
		t.Fatalf("Failed to load saved state: %v", err)
	}

	if len(loaded.Repos) != 1 {
		t.Errorf("Loaded state has %d repos, want 1", len(loaded.Repos))
	}

	repo, exists := loaded.GetRepo("test-repo")
	if !exists {
		t.Fatal("test-repo not found in loaded state")
	}

	if repo.GithubURL != "https://github.com/test/repo" {
		t.Errorf("GithubURL = %q, want %q", repo.GithubURL, "https://github.com/test/repo")
	}
}

func TestGetAllRepos(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Empty state
	repos := s.GetAllRepos()
	if len(repos) != 0 {
		t.Errorf("GetAllRepos() on empty state returned %d repos, want 0", len(repos))
	}

	// Add multiple repos with agents
	for _, name := range []string{"repo1", "repo2", "repo3"} {
		repo := &Repository{
			GithubURL:   "https://github.com/test/" + name,
			TmuxSession: "mc-" + name,
			Agents:      make(map[string]Agent),
		}
		if err := s.AddRepo(name, repo); err != nil {
			t.Fatalf("AddRepo(%s) failed: %v", name, err)
		}

		// Add an agent to each repo
		agent := Agent{
			Type:       AgentTypeSupervisor,
			TmuxWindow: "supervisor",
			SessionID:  "session-" + name,
			PID:        12345,
			CreatedAt:  time.Now(),
		}
		if err := s.AddAgent(name, "supervisor", agent); err != nil {
			t.Fatalf("AddAgent() failed: %v", err)
		}
	}

	// Get all repos
	repos = s.GetAllRepos()
	if len(repos) != 3 {
		t.Errorf("GetAllRepos() returned %d repos, want 3", len(repos))
	}

	// Verify we got all repos
	for _, name := range []string{"repo1", "repo2", "repo3"} {
		repo, exists := repos[name]
		if !exists {
			t.Errorf("GetAllRepos() missing repo %q", name)
			continue
		}

		expectedURL := "https://github.com/test/" + name
		if repo.GithubURL != expectedURL {
			t.Errorf("repo %s GithubURL = %q, want %q", name, repo.GithubURL, expectedURL)
		}

		// Verify agents were copied
		if len(repo.Agents) != 1 {
			t.Errorf("repo %s has %d agents, want 1", name, len(repo.Agents))
		}
	}
}

func TestGetAllReposIsSnapshot(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Add a repo
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test",
		Agents:      make(map[string]Agent),
	}
	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	// Get snapshot
	snapshot := s.GetAllRepos()

	// Modify the snapshot
	snapshot["test-repo"].GithubURL = "modified"

	// Verify original state is unchanged
	originalRepo, _ := s.GetRepo("test-repo")
	if originalRepo.GithubURL == "modified" {
		t.Error("GetAllRepos() did not return a deep copy - modifying snapshot affected original state")
	}
}

func TestUpdateAgentNonExistentRepo(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	agent := Agent{
		Type:       AgentTypeSupervisor,
		TmuxWindow: "supervisor",
	}

	err := s.UpdateAgent("nonexistent", "supervisor", agent)
	if err == nil {
		t.Error("UpdateAgent() should fail for nonexistent repo")
	}
}

func TestUpdateAgentNonExistentAgent(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Add repo but no agent
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test",
		Agents:      make(map[string]Agent),
	}
	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	agent := Agent{
		Type:       AgentTypeSupervisor,
		TmuxWindow: "supervisor",
	}

	err := s.UpdateAgent("test-repo", "nonexistent", agent)
	if err == nil {
		t.Error("UpdateAgent() should fail for nonexistent agent")
	}
}

func TestRemoveAgentNonExistentRepo(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	err := s.RemoveAgent("nonexistent", "agent")
	if err == nil {
		t.Error("RemoveAgent() should fail for nonexistent repo")
	}
}

func TestGetAgentNonExistentRepo(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	_, exists := s.GetAgent("nonexistent", "agent")
	if exists {
		t.Error("GetAgent() should return false for nonexistent repo")
	}
}

func TestListAgentsNonExistentRepo(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	_, err := s.ListAgents("nonexistent")
	if err == nil {
		t.Error("ListAgents() should fail for nonexistent repo")
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "invalid.json")

	// Write invalid JSON
	if err := os.WriteFile(statePath, []byte("not valid json"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err := Load(statePath)
	if err == nil {
		t.Error("Load() should fail for invalid JSON")
	}
}

func TestAddRepoInitializesAgentsMap(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Add repo with nil agents map
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test",
		Agents:      nil, // Intentionally nil
	}
	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	// Verify agents map was initialized
	addedRepo, _ := s.GetRepo("test-repo")
	if addedRepo.Agents == nil {
		t.Error("AddRepo() did not initialize nil Agents map")
	}
}

func TestDefaultMergeQueueConfig(t *testing.T) {
	config := DefaultMergeQueueConfig()

	if !config.Enabled {
		t.Error("Default config should have Enabled = true")
	}

	if config.TrackMode != TrackModeAll {
		t.Errorf("Default config TrackMode = %q, want %q", config.TrackMode, TrackModeAll)
	}
}

func TestMergeQueueConfigSaveLoad(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Create repo with custom merge queue config
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test",
		Agents:      make(map[string]Agent),
		MergeQueueConfig: MergeQueueConfig{
			Enabled:   false,
			TrackMode: TrackModeAuthor,
		},
	}

	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	// Load state from disk
	loaded, err := Load(statePath)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify merge queue config was loaded
	loadedRepo, exists := loaded.GetRepo("test-repo")
	if !exists {
		t.Fatal("Repository not found after load")
	}

	if loadedRepo.MergeQueueConfig.Enabled != false {
		t.Error("MergeQueueConfig.Enabled not persisted correctly")
	}

	if loadedRepo.MergeQueueConfig.TrackMode != TrackModeAuthor {
		t.Errorf("MergeQueueConfig.TrackMode = %q, want %q", loadedRepo.MergeQueueConfig.TrackMode, TrackModeAuthor)
	}
}

func TestGetMergeQueueConfig(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Test non-existent repo
	_, err := s.GetMergeQueueConfig("nonexistent")
	if err == nil {
		t.Error("GetMergeQueueConfig() should fail for nonexistent repo")
	}

	// Add repo without explicit config (should get defaults)
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test",
		Agents:      make(map[string]Agent),
	}
	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	// Get config - should return defaults for empty config
	config, err := s.GetMergeQueueConfig("test-repo")
	if err != nil {
		t.Fatalf("GetMergeQueueConfig() failed: %v", err)
	}

	if !config.Enabled {
		t.Error("Default config should have Enabled = true")
	}

	if config.TrackMode != TrackModeAll {
		t.Errorf("Default config TrackMode = %q, want %q", config.TrackMode, TrackModeAll)
	}
}

func TestUpdateMergeQueueConfig(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Test non-existent repo
	err := s.UpdateMergeQueueConfig("nonexistent", MergeQueueConfig{})
	if err == nil {
		t.Error("UpdateMergeQueueConfig() should fail for nonexistent repo")
	}

	// Add repo
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test",
		Agents:      make(map[string]Agent),
	}
	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	// Update config
	newConfig := MergeQueueConfig{
		Enabled:   false,
		TrackMode: TrackModeAssigned,
	}

	if err := s.UpdateMergeQueueConfig("test-repo", newConfig); err != nil {
		t.Fatalf("UpdateMergeQueueConfig() failed: %v", err)
	}

	// Verify update
	updatedConfig, err := s.GetMergeQueueConfig("test-repo")
	if err != nil {
		t.Fatalf("GetMergeQueueConfig() failed: %v", err)
	}

	if updatedConfig.Enabled != false {
		t.Error("Config.Enabled not updated correctly")
	}

	if updatedConfig.TrackMode != TrackModeAssigned {
		t.Errorf("Config.TrackMode = %q, want %q", updatedConfig.TrackMode, TrackModeAssigned)
	}

	// Verify persistence - reload state
	loaded, err := Load(statePath)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	loadedConfig, err := loaded.GetMergeQueueConfig("test-repo")
	if err != nil {
		t.Fatalf("GetMergeQueueConfig() after reload failed: %v", err)
	}

	if loadedConfig.TrackMode != TrackModeAssigned {
		t.Error("Config not persisted correctly after update")
	}
}

func TestGetAllReposCopiesMergeQueueConfig(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Add repo with custom merge queue config
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test",
		Agents:      make(map[string]Agent),
		MergeQueueConfig: MergeQueueConfig{
			Enabled:   false,
			TrackMode: TrackModeAuthor,
		},
	}
	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	// Get all repos
	repos := s.GetAllRepos()

	// Verify config was copied
	copiedRepo := repos["test-repo"]
	if copiedRepo.MergeQueueConfig.Enabled != false {
		t.Error("GetAllRepos() did not copy MergeQueueConfig.Enabled")
	}

	if copiedRepo.MergeQueueConfig.TrackMode != TrackModeAuthor {
		t.Errorf("GetAllRepos() MergeQueueConfig.TrackMode = %q, want %q", copiedRepo.MergeQueueConfig.TrackMode, TrackModeAuthor)
	}

	// Modify the copy and verify original is unchanged
	copiedRepo.MergeQueueConfig.TrackMode = TrackModeAssigned

	originalRepo, _ := s.GetRepo("test-repo")
	if originalRepo.MergeQueueConfig.TrackMode == TrackModeAssigned {
		t.Error("GetAllRepos() did not deep copy MergeQueueConfig")
	}
}

func TestTrackModeConstants(t *testing.T) {
	// Verify the track mode constants have the expected values
	if TrackModeAll != "all" {
		t.Errorf("TrackModeAll = %q, want 'all'", TrackModeAll)
	}

	if TrackModeAuthor != "author" {
		t.Errorf("TrackModeAuthor = %q, want 'author'", TrackModeAuthor)
	}

	if TrackModeAssigned != "assigned" {
		t.Errorf("TrackModeAssigned = %q, want 'assigned'", TrackModeAssigned)
	}
}

func TestParseTrackMode(t *testing.T) {
	tests := []struct {
		input   string
		want    TrackMode
		wantErr bool
	}{
		{"all", TrackModeAll, false},
		{"author", TrackModeAuthor, false},
		{"assigned", TrackModeAssigned, false},
		{"invalid", "", true},
		{"ALL", "", true},     // case-sensitive
		{"", "", true},        // empty string
		{"  all  ", "", true}, // no whitespace trimming
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseTrackMode(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseTrackMode(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseTrackMode(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestCurrentRepo(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Add a test repository
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "multiclaude-test-repo",
		Agents:      make(map[string]Agent),
	}
	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	// Test GetCurrentRepo when not set
	if current := s.GetCurrentRepo(); current != "" {
		t.Errorf("GetCurrentRepo() = %q, want empty string", current)
	}

	// Test SetCurrentRepo
	if err := s.SetCurrentRepo("test-repo"); err != nil {
		t.Fatalf("SetCurrentRepo() failed: %v", err)
	}

	// Test GetCurrentRepo after setting
	if current := s.GetCurrentRepo(); current != "test-repo" {
		t.Errorf("GetCurrentRepo() = %q, want 'test-repo'", current)
	}

	// Test SetCurrentRepo with non-existent repo
	if err := s.SetCurrentRepo("nonexistent"); err == nil {
		t.Error("SetCurrentRepo() with non-existent repo should return error")
	}

	// Test ClearCurrentRepo
	if err := s.ClearCurrentRepo(); err != nil {
		t.Fatalf("ClearCurrentRepo() failed: %v", err)
	}

	// Verify cleared
	if current := s.GetCurrentRepo(); current != "" {
		t.Errorf("GetCurrentRepo() after clear = %q, want empty string", current)
	}
}

func TestCurrentRepoPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	// Create state and set current repo
	s := New(statePath)
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "multiclaude-test-repo",
		Agents:      make(map[string]Agent),
	}
	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}
	if err := s.SetCurrentRepo("test-repo"); err != nil {
		t.Fatalf("SetCurrentRepo() failed: %v", err)
	}

	// Load state from disk
	loaded, err := Load(statePath)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify current repo persisted
	if current := loaded.GetCurrentRepo(); current != "test-repo" {
		t.Errorf("Loaded GetCurrentRepo() = %q, want 'test-repo'", current)
	}
}

func TestTaskHistory(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Create a repo
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test-repo",
		Agents:      make(map[string]Agent),
	}
	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	// Add task history entries
	entry1 := TaskHistoryEntry{
		Name:        "worker-1",
		Task:        "Implement feature A",
		Branch:      "work/worker-1",
		Status:      TaskStatusUnknown,
		CreatedAt:   time.Now().Add(-2 * time.Hour),
		CompletedAt: time.Now().Add(-1 * time.Hour),
	}
	entry2 := TaskHistoryEntry{
		Name:        "worker-2",
		Task:        "Fix bug B",
		Branch:      "work/worker-2",
		PRURL:       "https://github.com/test/repo/pull/123",
		PRNumber:    123,
		Status:      TaskStatusMerged,
		CreatedAt:   time.Now().Add(-1 * time.Hour),
		CompletedAt: time.Now(),
	}

	if err := s.AddTaskHistory("test-repo", entry1); err != nil {
		t.Fatalf("AddTaskHistory() failed: %v", err)
	}
	if err := s.AddTaskHistory("test-repo", entry2); err != nil {
		t.Fatalf("AddTaskHistory() failed: %v", err)
	}

	// Get task history (should be in reverse order - most recent first)
	history, err := s.GetTaskHistory("test-repo", 10)
	if err != nil {
		t.Fatalf("GetTaskHistory() failed: %v", err)
	}

	if len(history) != 2 {
		t.Errorf("GetTaskHistory() returned %d entries, want 2", len(history))
	}

	// Verify order (most recent first)
	if history[0].Name != "worker-2" {
		t.Errorf("First history entry name = %q, want 'worker-2'", history[0].Name)
	}
	if history[1].Name != "worker-1" {
		t.Errorf("Second history entry name = %q, want 'worker-1'", history[1].Name)
	}

	// Verify fields
	if history[0].Status != TaskStatusMerged {
		t.Errorf("First entry status = %q, want 'merged'", history[0].Status)
	}
	if history[0].PRNumber != 123 {
		t.Errorf("First entry PRNumber = %d, want 123", history[0].PRNumber)
	}
}

func TestTaskHistoryLimit(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Create a repo
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test-repo",
		Agents:      make(map[string]Agent),
	}
	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	// Add 5 task history entries
	for i := 0; i < 5; i++ {
		entry := TaskHistoryEntry{
			Name:        fmt.Sprintf("worker-%d", i),
			Task:        fmt.Sprintf("Task %d", i),
			Branch:      fmt.Sprintf("work/worker-%d", i),
			Status:      TaskStatusUnknown,
			CreatedAt:   time.Now().Add(time.Duration(-5+i) * time.Hour),
			CompletedAt: time.Now().Add(time.Duration(-4+i) * time.Hour),
		}
		if err := s.AddTaskHistory("test-repo", entry); err != nil {
			t.Fatalf("AddTaskHistory() failed: %v", err)
		}
	}

	// Get limited history
	history, err := s.GetTaskHistory("test-repo", 3)
	if err != nil {
		t.Fatalf("GetTaskHistory() failed: %v", err)
	}

	if len(history) != 3 {
		t.Errorf("GetTaskHistory() with limit=3 returned %d entries, want 3", len(history))
	}

	// Verify we got the most recent 3
	if history[0].Name != "worker-4" {
		t.Errorf("First entry name = %q, want 'worker-4'", history[0].Name)
	}
}

func TestUpdateTaskHistoryStatus(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Create a repo
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test-repo",
		Agents:      make(map[string]Agent),
	}
	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	// Add a task history entry
	entry := TaskHistoryEntry{
		Name:        "worker-1",
		Task:        "Implement feature A",
		Branch:      "work/worker-1",
		Status:      TaskStatusUnknown,
		CreatedAt:   time.Now().Add(-1 * time.Hour),
		CompletedAt: time.Now(),
	}
	if err := s.AddTaskHistory("test-repo", entry); err != nil {
		t.Fatalf("AddTaskHistory() failed: %v", err)
	}

	// Update the status
	if err := s.UpdateTaskHistoryStatus("test-repo", "worker-1", TaskStatusMerged, "https://github.com/test/repo/pull/456", 456); err != nil {
		t.Fatalf("UpdateTaskHistoryStatus() failed: %v", err)
	}

	// Get and verify
	history, err := s.GetTaskHistory("test-repo", 10)
	if err != nil {
		t.Fatalf("GetTaskHistory() failed: %v", err)
	}

	if len(history) != 1 {
		t.Fatalf("GetTaskHistory() returned %d entries, want 1", len(history))
	}

	if history[0].Status != TaskStatusMerged {
		t.Errorf("Updated status = %q, want 'merged'", history[0].Status)
	}
	if history[0].PRURL != "https://github.com/test/repo/pull/456" {
		t.Errorf("Updated PRURL = %q, want 'https://github.com/test/repo/pull/456'", history[0].PRURL)
	}
	if history[0].PRNumber != 456 {
		t.Errorf("Updated PRNumber = %d, want 456", history[0].PRNumber)
	}
}

func TestTaskHistoryPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Create a repo
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test-repo",
		Agents:      make(map[string]Agent),
	}
	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	// Add a task history entry
	entry := TaskHistoryEntry{
		Name:        "worker-1",
		Task:        "Implement feature A",
		Branch:      "work/worker-1",
		PRURL:       "https://github.com/test/repo/pull/789",
		PRNumber:    789,
		Status:      TaskStatusMerged,
		CreatedAt:   time.Now().Add(-1 * time.Hour),
		CompletedAt: time.Now(),
	}
	if err := s.AddTaskHistory("test-repo", entry); err != nil {
		t.Fatalf("AddTaskHistory() failed: %v", err)
	}

	// Load state from disk
	loaded, err := Load(statePath)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify task history persisted
	history, err := loaded.GetTaskHistory("test-repo", 10)
	if err != nil {
		t.Fatalf("GetTaskHistory() failed: %v", err)
	}

	if len(history) != 1 {
		t.Fatalf("Loaded GetTaskHistory() returned %d entries, want 1", len(history))
	}

	if history[0].Name != "worker-1" {
		t.Errorf("Loaded entry name = %q, want 'worker-1'", history[0].Name)
	}
	if history[0].Status != TaskStatusMerged {
		t.Errorf("Loaded entry status = %q, want 'merged'", history[0].Status)
	}
}

func TestConcurrentSaves(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Add initial repo
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test",
		Agents:      make(map[string]Agent),
	}
	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	// Run concurrent saves
	const numGoroutines = 10
	const opsPerGoroutine = 20

	var wg sync.WaitGroup
	errChan := make(chan error, numGoroutines*opsPerGoroutine)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				agentName := fmt.Sprintf("agent-%d-%d", id, j)
				agent := Agent{
					Type:       AgentTypeWorker,
					TmuxWindow: agentName,
					SessionID:  fmt.Sprintf("session-%d-%d", id, j),
					PID:        12345 + id*100 + j,
					CreatedAt:  time.Now(),
				}
				if err := s.AddAgent("test-repo", agentName, agent); err != nil {
					// Agent might already exist from a race - that's OK
					continue
				}
			}
		}(i)
	}

	wg.Wait()
	close(errChan)

	// Collect any errors
	for err := range errChan {
		t.Errorf("Concurrent operation failed: %v", err)
	}

	// Verify state is valid by reloading
	loaded, err := Load(statePath)
	if err != nil {
		t.Fatalf("Load() after concurrent saves failed: %v", err)
	}

	// Should have the repo
	_, exists := loaded.GetRepo("test-repo")
	if !exists {
		t.Error("Repository not found after concurrent saves")
	}
}

func TestGetAllReposCopiesTaskHistory(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Add a repo with task history
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test",
		Agents:      make(map[string]Agent),
	}
	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	// Add task history
	entry := TaskHistoryEntry{
		Name:      "worker-1",
		Task:      "Test task",
		Branch:    "work/worker-1",
		Status:    TaskStatusMerged,
		CreatedAt: time.Now(),
	}
	if err := s.AddTaskHistory("test-repo", entry); err != nil {
		t.Fatalf("AddTaskHistory() failed: %v", err)
	}

	// Get all repos
	repos := s.GetAllRepos()

	// Verify task history was copied
	copiedRepo := repos["test-repo"]
	if copiedRepo.TaskHistory == nil {
		t.Fatal("GetAllRepos() did not copy TaskHistory (nil)")
	}
	if len(copiedRepo.TaskHistory) != 1 {
		t.Fatalf("GetAllRepos() TaskHistory length = %d, want 1", len(copiedRepo.TaskHistory))
	}
	if copiedRepo.TaskHistory[0].Name != "worker-1" {
		t.Errorf("Copied TaskHistory entry name = %q, want 'worker-1'", copiedRepo.TaskHistory[0].Name)
	}

	// Modify the copy and verify original is unchanged
	copiedRepo.TaskHistory[0].Name = "modified"

	originalHistory, _ := s.GetTaskHistory("test-repo", 10)
	if originalHistory[0].Name == "modified" {
		t.Error("GetAllRepos() did not deep copy TaskHistory - modifying snapshot affected original")
	}
}

func TestSaveCleansUpTempFiles(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Add data and save multiple times
	for i := 0; i < 5; i++ {
		repo := &Repository{
			GithubURL:   fmt.Sprintf("https://github.com/test/repo%d", i),
			TmuxSession: fmt.Sprintf("mc-test%d", i),
			Agents:      make(map[string]Agent),
		}
		if err := s.AddRepo(fmt.Sprintf("repo%d", i), repo); err != nil {
			t.Fatalf("AddRepo() failed: %v", err)
		}
	}

	// Check that no .tmp files are left behind
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}

	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".tmp" {
			t.Errorf("Temp file not cleaned up: %s", entry.Name())
		}
	}
}

func TestUpdateAgentPID(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Create a repo
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test",
		Agents:      make(map[string]Agent),
	}
	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	// Add an agent
	agent := Agent{
		Type:       AgentTypeSupervisor,
		TmuxWindow: "supervisor",
		SessionID:  "session-1",
		PID:        12345,
		CreatedAt:  time.Now(),
	}
	if err := s.AddAgent("test-repo", "supervisor", agent); err != nil {
		t.Fatalf("AddAgent() failed: %v", err)
	}

	// Update the PID
	if err := s.UpdateAgentPID("test-repo", "supervisor", 67890); err != nil {
		t.Fatalf("UpdateAgentPID() failed: %v", err)
	}

	// Verify the PID was updated
	updated, exists := s.GetAgent("test-repo", "supervisor")
	if !exists {
		t.Fatal("Agent not found after update")
	}
	if updated.PID != 67890 {
		t.Errorf("PID = %d, want 67890", updated.PID)
	}

	// Test error cases
	if err := s.UpdateAgentPID("nonexistent", "supervisor", 11111); err == nil {
		t.Error("UpdateAgentPID should fail for nonexistent repo")
	}
	if err := s.UpdateAgentPID("test-repo", "nonexistent", 11111); err == nil {
		t.Error("UpdateAgentPID should fail for nonexistent agent")
	}
}

func TestUpdateTaskHistorySummary(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Create a repo
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test",
		Agents:      make(map[string]Agent),
	}
	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	// Add a task history entry
	entry := TaskHistoryEntry{
		Name:      "worker-1",
		Task:      "Test task",
		Branch:    "work/worker-1",
		Status:    TaskStatusOpen,
		CreatedAt: time.Now(),
	}
	if err := s.AddTaskHistory("test-repo", entry); err != nil {
		t.Fatalf("AddTaskHistory() failed: %v", err)
	}

	// Update with summary
	if err := s.UpdateTaskHistorySummary("test-repo", "worker-1", "Completed the task successfully", ""); err != nil {
		t.Fatalf("UpdateTaskHistorySummary() failed: %v", err)
	}

	// Verify the summary was updated
	history, _ := s.GetTaskHistory("test-repo", 10)
	if history[0].Summary != "Completed the task successfully" {
		t.Errorf("Summary = %q, want 'Completed the task successfully'", history[0].Summary)
	}

	// Update with failure reason
	if err := s.UpdateTaskHistorySummary("test-repo", "worker-1", "", "Out of memory"); err != nil {
		t.Fatalf("UpdateTaskHistorySummary() failed: %v", err)
	}

	// Verify the failure reason was updated and status changed
	history, _ = s.GetTaskHistory("test-repo", 10)
	if history[0].FailureReason != "Out of memory" {
		t.Errorf("FailureReason = %q, want 'Out of memory'", history[0].FailureReason)
	}
	if history[0].Status != TaskStatusFailed {
		t.Errorf("Status = %q, want 'failed'", history[0].Status)
	}

	// Test error case
	if err := s.UpdateTaskHistorySummary("test-repo", "nonexistent", "summary", ""); err == nil {
		t.Error("UpdateTaskHistorySummary should fail for nonexistent task")
	}
}

func TestAgentTypeIsPersistent(t *testing.T) {
	tests := []struct {
		agentType  AgentType
		persistent bool
	}{
		// Persistent agents should return true
		{AgentTypeSupervisor, true},
		{AgentTypeMergeQueue, true},
		{AgentTypeWorkspace, true},
		{AgentTypeGenericPersistent, true},
		// Transient agents should return false
		{AgentTypeWorker, false},
		{AgentTypeReview, false},
		// Unknown types should return false (safe default)
		{AgentType("unknown"), false},
		{AgentType(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.agentType), func(t *testing.T) {
			got := tt.agentType.IsPersistent()
			if got != tt.persistent {
				t.Errorf("AgentType(%q).IsPersistent() = %v, want %v", tt.agentType, got, tt.persistent)
			}
		})
	}
}

func TestDefaultPRShepherdConfig(t *testing.T) {
	config := DefaultPRShepherdConfig()

	if !config.Enabled {
		t.Error("Default PR shepherd config should have Enabled = true")
	}

	if config.TrackMode != TrackModeAuthor {
		t.Errorf("Default PR shepherd config TrackMode = %q, want %q", config.TrackMode, TrackModeAuthor)
	}
}

func TestGetPRShepherdConfig(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Test non-existent repo
	_, err := s.GetPRShepherdConfig("nonexistent")
	if err == nil {
		t.Error("GetPRShepherdConfig() should fail for nonexistent repo")
	}

	// Add repo without explicit PR shepherd config (should get defaults)
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test",
		Agents:      make(map[string]Agent),
	}
	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	// Get config - should return defaults for empty config
	config, err := s.GetPRShepherdConfig("test-repo")
	if err != nil {
		t.Fatalf("GetPRShepherdConfig() failed: %v", err)
	}

	if !config.Enabled {
		t.Error("Default PR shepherd config should have Enabled = true")
	}

	if config.TrackMode != TrackModeAuthor {
		t.Errorf("Default PR shepherd config TrackMode = %q, want %q", config.TrackMode, TrackModeAuthor)
	}
}

func TestGetPRShepherdConfigWithExplicitConfig(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Add repo with explicit PR shepherd config
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test",
		Agents:      make(map[string]Agent),
		PRShepherdConfig: PRShepherdConfig{
			Enabled:   false,
			TrackMode: TrackModeAssigned,
		},
	}
	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	// Get config - should return the explicit config
	config, err := s.GetPRShepherdConfig("test-repo")
	if err != nil {
		t.Fatalf("GetPRShepherdConfig() failed: %v", err)
	}

	if config.Enabled {
		t.Error("PR shepherd config should have Enabled = false")
	}

	if config.TrackMode != TrackModeAssigned {
		t.Errorf("PR shepherd config TrackMode = %q, want %q", config.TrackMode, TrackModeAssigned)
	}
}

func TestUpdatePRShepherdConfig(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Test non-existent repo
	err := s.UpdatePRShepherdConfig("nonexistent", PRShepherdConfig{})
	if err == nil {
		t.Error("UpdatePRShepherdConfig() should fail for nonexistent repo")
	}

	// Add repo
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test",
		Agents:      make(map[string]Agent),
	}
	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	// Update config
	newConfig := PRShepherdConfig{
		Enabled:   false,
		TrackMode: TrackModeAll,
	}

	if err := s.UpdatePRShepherdConfig("test-repo", newConfig); err != nil {
		t.Fatalf("UpdatePRShepherdConfig() failed: %v", err)
	}

	// Verify update
	updatedConfig, err := s.GetPRShepherdConfig("test-repo")
	if err != nil {
		t.Fatalf("GetPRShepherdConfig() failed: %v", err)
	}

	if updatedConfig.Enabled != false {
		t.Error("Config.Enabled not updated correctly")
	}

	if updatedConfig.TrackMode != TrackModeAll {
		t.Errorf("Config.TrackMode = %q, want %q", updatedConfig.TrackMode, TrackModeAll)
	}

	// Verify persistence - reload state
	loaded, err := Load(statePath)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	loadedConfig, err := loaded.GetPRShepherdConfig("test-repo")
	if err != nil {
		t.Fatalf("GetPRShepherdConfig() after reload failed: %v", err)
	}

	if loadedConfig.TrackMode != TrackModeAll {
		t.Error("Config not persisted correctly after update")
	}
}

func TestGetForkConfig(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Test non-existent repo
	_, err := s.GetForkConfig("nonexistent")
	if err == nil {
		t.Error("GetForkConfig() should fail for nonexistent repo")
	}

	// Add repo without fork config
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test",
		Agents:      make(map[string]Agent),
	}
	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	// Get config - should return empty ForkConfig
	config, err := s.GetForkConfig("test-repo")
	if err != nil {
		t.Fatalf("GetForkConfig() failed: %v", err)
	}

	if config.IsFork {
		t.Error("Default fork config should have IsFork = false")
	}
	if config.UpstreamURL != "" {
		t.Errorf("Default fork config UpstreamURL = %q, want empty string", config.UpstreamURL)
	}
}

func TestGetForkConfigWithData(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Add repo with fork config
	repo := &Repository{
		GithubURL:   "https://github.com/fork-owner/repo",
		TmuxSession: "mc-test",
		Agents:      make(map[string]Agent),
		ForkConfig: ForkConfig{
			IsFork:        true,
			UpstreamURL:   "https://github.com/upstream-owner/repo",
			UpstreamOwner: "upstream-owner",
			UpstreamRepo:  "repo",
		},
	}
	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	config, err := s.GetForkConfig("test-repo")
	if err != nil {
		t.Fatalf("GetForkConfig() failed: %v", err)
	}

	if !config.IsFork {
		t.Error("Fork config should have IsFork = true")
	}
	if config.UpstreamURL != "https://github.com/upstream-owner/repo" {
		t.Errorf("Fork config UpstreamURL = %q, want 'https://github.com/upstream-owner/repo'", config.UpstreamURL)
	}
	if config.UpstreamOwner != "upstream-owner" {
		t.Errorf("Fork config UpstreamOwner = %q, want 'upstream-owner'", config.UpstreamOwner)
	}
	if config.UpstreamRepo != "repo" {
		t.Errorf("Fork config UpstreamRepo = %q, want 'repo'", config.UpstreamRepo)
	}
}

func TestUpdateForkConfig(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Test non-existent repo
	err := s.UpdateForkConfig("nonexistent", ForkConfig{})
	if err == nil {
		t.Error("UpdateForkConfig() should fail for nonexistent repo")
	}

	// Add repo
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test",
		Agents:      make(map[string]Agent),
	}
	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	// Update config
	newConfig := ForkConfig{
		IsFork:        true,
		UpstreamURL:   "https://github.com/original/repo",
		UpstreamOwner: "original",
		UpstreamRepo:  "repo",
		ForceForkMode: false,
	}

	if err := s.UpdateForkConfig("test-repo", newConfig); err != nil {
		t.Fatalf("UpdateForkConfig() failed: %v", err)
	}

	// Verify update
	updatedConfig, err := s.GetForkConfig("test-repo")
	if err != nil {
		t.Fatalf("GetForkConfig() failed: %v", err)
	}

	if !updatedConfig.IsFork {
		t.Error("Config.IsFork not updated correctly")
	}

	if updatedConfig.UpstreamURL != "https://github.com/original/repo" {
		t.Errorf("Config.UpstreamURL = %q, want 'https://github.com/original/repo'", updatedConfig.UpstreamURL)
	}

	// Verify persistence - reload state
	loaded, err := Load(statePath)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	loadedConfig, err := loaded.GetForkConfig("test-repo")
	if err != nil {
		t.Fatalf("GetForkConfig() after reload failed: %v", err)
	}

	if !loadedConfig.IsFork {
		t.Error("Config.IsFork not persisted correctly after update")
	}
}

func TestIsForkMode(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Test non-existent repo - should return false
	if s.IsForkMode("nonexistent") {
		t.Error("IsForkMode() should return false for nonexistent repo")
	}

	// Add repo without fork config - should return false
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test",
		Agents:      make(map[string]Agent),
	}
	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	if s.IsForkMode("test-repo") {
		t.Error("IsForkMode() should return false for repo without fork config")
	}
}

func TestIsForkModeWithFork(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Add repo that is a fork
	repo := &Repository{
		GithubURL:   "https://github.com/fork-owner/repo",
		TmuxSession: "mc-test",
		Agents:      make(map[string]Agent),
		ForkConfig: ForkConfig{
			IsFork: true,
		},
	}
	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	if !s.IsForkMode("test-repo") {
		t.Error("IsForkMode() should return true for fork repo")
	}
}

func TestIsForkModeWithForceForkMode(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Add repo that is not a fork but has ForceForkMode enabled
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test",
		Agents:      make(map[string]Agent),
		ForkConfig: ForkConfig{
			IsFork:        false,
			ForceForkMode: true,
		},
	}
	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	if !s.IsForkMode("test-repo") {
		t.Error("IsForkMode() should return true when ForceForkMode is enabled")
	}
}

func TestForkConfigPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	// Create state with fork config
	s := New(statePath)
	repo := &Repository{
		GithubURL:   "https://github.com/fork-owner/repo",
		TmuxSession: "mc-test",
		Agents:      make(map[string]Agent),
		ForkConfig: ForkConfig{
			IsFork:        true,
			UpstreamURL:   "https://github.com/original/repo",
			UpstreamOwner: "original",
			UpstreamRepo:  "repo",
			ForceForkMode: false,
		},
	}
	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	// Load state from disk
	loaded, err := Load(statePath)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify fork config persisted
	config, err := loaded.GetForkConfig("test-repo")
	if err != nil {
		t.Fatalf("GetForkConfig() failed: %v", err)
	}

	if !config.IsFork {
		t.Error("ForkConfig.IsFork not persisted correctly")
	}
	if config.UpstreamURL != "https://github.com/original/repo" {
		t.Errorf("ForkConfig.UpstreamURL = %q, want 'https://github.com/original/repo'", config.UpstreamURL)
	}
	if config.UpstreamOwner != "original" {
		t.Errorf("ForkConfig.UpstreamOwner = %q, want 'original'", config.UpstreamOwner)
	}
	if config.UpstreamRepo != "repo" {
		t.Errorf("ForkConfig.UpstreamRepo = %q, want 'repo'", config.UpstreamRepo)
	}

	// Verify IsForkMode works after reload
	if !loaded.IsForkMode("test-repo") {
		t.Error("IsForkMode() should return true after reload")
	}
}

func TestGetAllReposCopiesForkConfig(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Add repo with fork config
	repo := &Repository{
		GithubURL:   "https://github.com/fork-owner/repo",
		TmuxSession: "mc-test",
		Agents:      make(map[string]Agent),
		ForkConfig: ForkConfig{
			IsFork:        true,
			UpstreamURL:   "https://github.com/original/repo",
			UpstreamOwner: "original",
			UpstreamRepo:  "repo",
		},
	}
	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	// Get all repos
	repos := s.GetAllRepos()

	// Verify fork config was copied
	copiedRepo := repos["test-repo"]
	if !copiedRepo.ForkConfig.IsFork {
		t.Error("GetAllRepos() did not copy ForkConfig.IsFork")
	}
	if copiedRepo.ForkConfig.UpstreamOwner != "original" {
		t.Errorf("GetAllRepos() ForkConfig.UpstreamOwner = %q, want 'original'", copiedRepo.ForkConfig.UpstreamOwner)
	}

	// Modify the copy and verify original is unchanged
	copiedRepo.ForkConfig.UpstreamOwner = "modified"

	originalConfig, _ := s.GetForkConfig("test-repo")
	if originalConfig.UpstreamOwner == "modified" {
		t.Error("GetAllRepos() did not deep copy ForkConfig - modifying snapshot affected original")
	}
}

func TestGetAllReposCopiesPRShepherdConfig(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Add repo with PR shepherd config
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test",
		Agents:      make(map[string]Agent),
		PRShepherdConfig: PRShepherdConfig{
			Enabled:   false,
			TrackMode: TrackModeAssigned,
		},
	}
	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	// Get all repos
	repos := s.GetAllRepos()

	// Verify PR shepherd config was copied
	copiedRepo := repos["test-repo"]
	if copiedRepo.PRShepherdConfig.Enabled {
		t.Error("GetAllRepos() did not copy PRShepherdConfig.Enabled correctly")
	}
	if copiedRepo.PRShepherdConfig.TrackMode != TrackModeAssigned {
		t.Errorf("GetAllRepos() PRShepherdConfig.TrackMode = %q, want 'assigned'", copiedRepo.PRShepherdConfig.TrackMode)
	}

	// Modify the copy and verify original is unchanged
	copiedRepo.PRShepherdConfig.TrackMode = TrackModeAll

	originalConfig, _ := s.GetPRShepherdConfig("test-repo")
	if originalConfig.TrackMode == TrackModeAll {
		t.Error("GetAllRepos() did not deep copy PRShepherdConfig - modifying snapshot affected original")
	}
}

func TestTaskHistoryNonExistentRepo(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Test AddTaskHistory on non-existent repo
	entry := TaskHistoryEntry{
		Name:      "worker-1",
		Task:      "Test task",
		CreatedAt: time.Now(),
	}
	err := s.AddTaskHistory("nonexistent", entry)
	if err == nil {
		t.Error("AddTaskHistory() should fail for nonexistent repo")
	}

	// Test GetTaskHistory on non-existent repo
	_, err = s.GetTaskHistory("nonexistent", 10)
	if err == nil {
		t.Error("GetTaskHistory() should fail for nonexistent repo")
	}

	// Test UpdateTaskHistoryStatus on non-existent repo
	err = s.UpdateTaskHistoryStatus("nonexistent", "worker-1", TaskStatusMerged, "", 0)
	if err == nil {
		t.Error("UpdateTaskHistoryStatus() should fail for nonexistent repo")
	}

	// Test UpdateTaskHistorySummary on non-existent repo
	err = s.UpdateTaskHistorySummary("nonexistent", "worker-1", "summary", "")
	if err == nil {
		t.Error("UpdateTaskHistorySummary() should fail for nonexistent repo")
	}
}

func TestGetTaskHistoryEmptyHistory(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Add repo without task history
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test",
		Agents:      make(map[string]Agent),
	}
	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	// Get history - should return empty slice, not nil
	history, err := s.GetTaskHistory("test-repo", 10)
	if err != nil {
		t.Fatalf("GetTaskHistory() failed: %v", err)
	}

	if history == nil {
		t.Error("GetTaskHistory() should return empty slice, not nil")
	}
	if len(history) != 0 {
		t.Errorf("GetTaskHistory() returned %d entries, want 0", len(history))
	}
}

func TestGetTaskHistoryNoLimit(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := New(statePath)

	// Add repo
	repo := &Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test",
		Agents:      make(map[string]Agent),
	}
	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("AddRepo() failed: %v", err)
	}

	// Add 5 task history entries
	for i := 0; i < 5; i++ {
		entry := TaskHistoryEntry{
			Name:      fmt.Sprintf("worker-%d", i),
			Task:      fmt.Sprintf("Task %d", i),
			CreatedAt: time.Now().Add(time.Duration(i) * time.Hour),
		}
		if err := s.AddTaskHistory("test-repo", entry); err != nil {
			t.Fatalf("AddTaskHistory() failed: %v", err)
		}
	}

	// Get history with no limit (0)
	history, err := s.GetTaskHistory("test-repo", 0)
	if err != nil {
		t.Fatalf("GetTaskHistory() failed: %v", err)
	}

	if len(history) != 5 {
		t.Errorf("GetTaskHistory() with limit=0 returned %d entries, want 5", len(history))
	}
}
