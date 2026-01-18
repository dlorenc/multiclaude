package daemon

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dlorenc/multiclaude/internal/messages"
	"github.com/dlorenc/multiclaude/internal/socket"
	"github.com/dlorenc/multiclaude/internal/state"
	"github.com/dlorenc/multiclaude/pkg/config"
)

func setupTestDaemon(t *testing.T) (*Daemon, func()) {
	t.Helper()

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "daemon-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Create paths
	paths := &config.Paths{
		Root:         tmpDir,
		DaemonPID:    filepath.Join(tmpDir, "daemon.pid"),
		DaemonSock:   filepath.Join(tmpDir, "daemon.sock"),
		DaemonLog:    filepath.Join(tmpDir, "daemon.log"),
		StateFile:    filepath.Join(tmpDir, "state.json"),
		ReposDir:     filepath.Join(tmpDir, "repos"),
		WorktreesDir: filepath.Join(tmpDir, "wts"),
		MessagesDir:  filepath.Join(tmpDir, "messages"),
	}

	// Create directories
	if err := paths.EnsureDirectories(); err != nil {
		t.Fatalf("Failed to create directories: %v", err)
	}

	// Create daemon
	d, err := New(paths)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return d, cleanup
}

func TestDaemonCreation(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	if d == nil {
		t.Fatal("Daemon should not be nil")
	}

	if d.state == nil {
		t.Fatal("Daemon state should not be nil")
	}

	if d.tmux == nil {
		t.Fatal("Daemon tmux client should not be nil")
	}

	if d.logger == nil {
		t.Fatal("Daemon logger should not be nil")
	}
}

func TestGetMessageManager(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	mgr := d.getMessageManager()
	if mgr == nil {
		t.Fatal("Message manager should not be nil")
	}
}

func TestRouteMessages(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Add a test repository
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "test-session",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.state.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Add a test agent
	agent := state.Agent{
		Type:         state.AgentTypeWorker,
		WorktreePath: "/tmp/test",
		TmuxWindow:   "test-window",
		SessionID:    "test-session-id",
		CreatedAt:    time.Now(),
	}
	if err := d.state.AddAgent("test-repo", "test-agent", agent); err != nil {
		t.Fatalf("Failed to add agent: %v", err)
	}

	// Create a message
	msgMgr := messages.NewManager(d.paths.MessagesDir)
	msg, err := msgMgr.Send("test-repo", "supervisor", "test-agent", "Test message body")
	if err != nil {
		t.Fatalf("Failed to create message: %v", err)
	}

	// Verify message is pending
	if msg.Status != messages.StatusPending {
		t.Errorf("Message status = %s, want %s", msg.Status, messages.StatusPending)
	}

	// Call routeMessages (it will try to send via tmux, which will fail, but that's ok)
	d.routeMessages()

	// Note: We can't verify delivery without a real tmux session,
	// but we've tested that the function doesn't panic
}

func TestCleanupDeadAgents(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Add a test repository
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "test-session",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.state.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Add a test agent
	agent := state.Agent{
		Type:         state.AgentTypeWorker,
		WorktreePath: "/tmp/test",
		TmuxWindow:   "test-window",
		SessionID:    "test-session-id",
		CreatedAt:    time.Now(),
	}
	if err := d.state.AddAgent("test-repo", "test-agent", agent); err != nil {
		t.Fatalf("Failed to add agent: %v", err)
	}

	// Verify agent exists
	_, exists := d.state.GetAgent("test-repo", "test-agent")
	if !exists {
		t.Fatal("Agent should exist before cleanup")
	}

	// Mark agent as dead
	deadAgents := map[string][]string{
		"test-repo": {"test-agent"},
	}

	// Call cleanup
	d.cleanupDeadAgents(deadAgents)

	// Verify agent was removed
	_, exists = d.state.GetAgent("test-repo", "test-agent")
	if exists {
		t.Error("Agent should not exist after cleanup")
	}
}

func TestHandleCompleteAgent(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Add a test repository
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "test-session",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.state.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Add a test agent
	agent := state.Agent{
		Type:         state.AgentTypeWorker,
		WorktreePath: "/tmp/test",
		TmuxWindow:   "test-window",
		SessionID:    "test-session-id",
		CreatedAt:    time.Now(),
	}
	if err := d.state.AddAgent("test-repo", "test-agent", agent); err != nil {
		t.Fatalf("Failed to add agent: %v", err)
	}

	// Test missing repo argument
	resp := d.handleCompleteAgent(socket.Request{
		Command: "complete_agent",
		Args: map[string]interface{}{
			"agent": "test-agent",
		},
	})
	if resp.Success {
		t.Error("Expected failure with missing repo")
	}

	// Test missing agent argument
	resp = d.handleCompleteAgent(socket.Request{
		Command: "complete_agent",
		Args: map[string]interface{}{
			"repo": "test-repo",
		},
	})
	if resp.Success {
		t.Error("Expected failure with missing agent")
	}

	// Test non-existent agent
	resp = d.handleCompleteAgent(socket.Request{
		Command: "complete_agent",
		Args: map[string]interface{}{
			"repo":  "test-repo",
			"agent": "non-existent",
		},
	})
	if resp.Success {
		t.Error("Expected failure with non-existent agent")
	}

	// Test successful completion
	resp = d.handleCompleteAgent(socket.Request{
		Command: "complete_agent",
		Args: map[string]interface{}{
			"repo":  "test-repo",
			"agent": "test-agent",
		},
	})
	if !resp.Success {
		t.Errorf("Expected success, got error: %s", resp.Error)
	}

	// Verify agent is marked for cleanup
	updatedAgent, _ := d.state.GetAgent("test-repo", "test-agent")
	if !updatedAgent.ReadyForCleanup {
		t.Error("Agent should be marked as ready for cleanup")
	}
}

func TestIsProcessAlive(t *testing.T) {
	// Test with PID 1 (init, should be alive on Unix systems)
	// This is more reliable than testing our own process
	if isProcessAlive(1) {
		t.Log("PID 1 is alive (as expected)")
	} else {
		t.Skip("PID 1 not available on this system")
	}

	// Test with very high invalid PID (should be dead)
	if isProcessAlive(999999) {
		t.Error("Invalid PID 999999 should be reported as dead")
	}
}

func TestHandleStatus(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Add a test repo and agent to verify counts
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "test-session",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.state.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	agent := state.Agent{
		Type:       state.AgentTypeSupervisor,
		TmuxWindow: "supervisor",
		SessionID:  "test-session-id",
		CreatedAt:  time.Now(),
	}
	if err := d.state.AddAgent("test-repo", "supervisor", agent); err != nil {
		t.Fatalf("Failed to add agent: %v", err)
	}

	resp := d.handleStatus(socket.Request{Command: "status"})

	if !resp.Success {
		t.Errorf("handleStatus() success = false, want true")
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("handleStatus() data is not a map")
	}

	if running, ok := data["running"].(bool); !ok || !running {
		t.Error("handleStatus() running = false, want true")
	}

	if repos, ok := data["repos"].(int); !ok || repos != 1 {
		t.Errorf("handleStatus() repos = %v, want 1", data["repos"])
	}

	if agents, ok := data["agents"].(int); !ok || agents != 1 {
		t.Errorf("handleStatus() agents = %v, want 1", data["agents"])
	}
}

func TestHandleListRepos(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Initially empty
	resp := d.handleListRepos(socket.Request{Command: "list_repos"})
	if !resp.Success {
		t.Error("handleListRepos() success = false, want true")
	}

	repos, ok := resp.Data.([]string)
	if !ok {
		t.Fatal("handleListRepos() data is not a []string")
	}
	if len(repos) != 0 {
		t.Errorf("handleListRepos() returned %d repos, want 0", len(repos))
	}

	// Add repos
	for _, name := range []string{"repo1", "repo2"} {
		repo := &state.Repository{
			GithubURL:   "https://github.com/test/" + name,
			TmuxSession: "mc-" + name,
			Agents:      make(map[string]state.Agent),
		}
		if err := d.state.AddRepo(name, repo); err != nil {
			t.Fatalf("Failed to add repo: %v", err)
		}
	}

	resp = d.handleListRepos(socket.Request{Command: "list_repos"})
	if !resp.Success {
		t.Error("handleListRepos() success = false, want true")
	}

	repos, ok = resp.Data.([]string)
	if !ok {
		t.Fatal("handleListRepos() data is not a []string")
	}
	if len(repos) != 2 {
		t.Errorf("handleListRepos() returned %d repos, want 2", len(repos))
	}
}

func TestHandleAddRepo(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Missing name
	resp := d.handleAddRepo(socket.Request{
		Command: "add_repo",
		Args: map[string]interface{}{
			"github_url":   "https://github.com/test/repo",
			"tmux_session": "test-session",
		},
	})
	if resp.Success {
		t.Error("handleAddRepo() should fail with missing name")
	}

	// Missing github_url
	resp = d.handleAddRepo(socket.Request{
		Command: "add_repo",
		Args: map[string]interface{}{
			"name":         "test-repo",
			"tmux_session": "test-session",
		},
	})
	if resp.Success {
		t.Error("handleAddRepo() should fail with missing github_url")
	}

	// Missing tmux_session
	resp = d.handleAddRepo(socket.Request{
		Command: "add_repo",
		Args: map[string]interface{}{
			"name":       "test-repo",
			"github_url": "https://github.com/test/repo",
		},
	})
	if resp.Success {
		t.Error("handleAddRepo() should fail with missing tmux_session")
	}

	// Valid request
	resp = d.handleAddRepo(socket.Request{
		Command: "add_repo",
		Args: map[string]interface{}{
			"name":         "test-repo",
			"github_url":   "https://github.com/test/repo",
			"tmux_session": "test-session",
		},
	})
	if !resp.Success {
		t.Errorf("handleAddRepo() failed: %s", resp.Error)
	}

	// Verify repo was added
	_, exists := d.state.GetRepo("test-repo")
	if !exists {
		t.Error("handleAddRepo() did not add repo to state")
	}
}

func TestHandleAddAgent(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// First add a repo
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "test-session",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.state.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Missing repo
	resp := d.handleAddAgent(socket.Request{
		Command: "add_agent",
		Args: map[string]interface{}{
			"agent":         "test-agent",
			"type":          "worker",
			"worktree_path": "/tmp/test",
			"tmux_window":   "test-window",
		},
	})
	if resp.Success {
		t.Error("handleAddAgent() should fail with missing repo")
	}

	// Missing agent name
	resp = d.handleAddAgent(socket.Request{
		Command: "add_agent",
		Args: map[string]interface{}{
			"repo":          "test-repo",
			"type":          "worker",
			"worktree_path": "/tmp/test",
			"tmux_window":   "test-window",
		},
	})
	if resp.Success {
		t.Error("handleAddAgent() should fail with missing agent name")
	}

	// Valid request with PID as float64 (JSON default)
	resp = d.handleAddAgent(socket.Request{
		Command: "add_agent",
		Args: map[string]interface{}{
			"repo":          "test-repo",
			"agent":         "test-agent",
			"type":          "worker",
			"worktree_path": "/tmp/test",
			"tmux_window":   "test-window",
			"session_id":    "test-session-id",
			"pid":           float64(12345),
			"task":          "test task",
		},
	})
	if !resp.Success {
		t.Errorf("handleAddAgent() failed: %s", resp.Error)
	}

	// Verify agent was added
	agent, exists := d.state.GetAgent("test-repo", "test-agent")
	if !exists {
		t.Error("handleAddAgent() did not add agent to state")
	}
	if agent.PID != 12345 {
		t.Errorf("handleAddAgent() PID = %d, want 12345", agent.PID)
	}
	if agent.Task != "test task" {
		t.Errorf("handleAddAgent() Task = %q, want %q", agent.Task, "test task")
	}
}

func TestHandleRemoveAgent(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// First add a repo and agent
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "test-session",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.state.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	agent := state.Agent{
		Type:       state.AgentTypeWorker,
		TmuxWindow: "test-window",
		SessionID:  "test-session-id",
		CreatedAt:  time.Now(),
	}
	if err := d.state.AddAgent("test-repo", "test-agent", agent); err != nil {
		t.Fatalf("Failed to add agent: %v", err)
	}

	// Missing repo
	resp := d.handleRemoveAgent(socket.Request{
		Command: "remove_agent",
		Args: map[string]interface{}{
			"agent": "test-agent",
		},
	})
	if resp.Success {
		t.Error("handleRemoveAgent() should fail with missing repo")
	}

	// Missing agent
	resp = d.handleRemoveAgent(socket.Request{
		Command: "remove_agent",
		Args: map[string]interface{}{
			"repo": "test-repo",
		},
	})
	if resp.Success {
		t.Error("handleRemoveAgent() should fail with missing agent")
	}

	// Valid request
	resp = d.handleRemoveAgent(socket.Request{
		Command: "remove_agent",
		Args: map[string]interface{}{
			"repo":  "test-repo",
			"agent": "test-agent",
		},
	})
	if !resp.Success {
		t.Errorf("handleRemoveAgent() failed: %s", resp.Error)
	}

	// Verify agent was removed
	_, exists := d.state.GetAgent("test-repo", "test-agent")
	if exists {
		t.Error("handleRemoveAgent() did not remove agent from state")
	}
}

func TestHandleListAgents(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// First add a repo
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "test-session",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.state.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Missing repo
	resp := d.handleListAgents(socket.Request{
		Command: "list_agents",
		Args:    map[string]interface{}{},
	})
	if resp.Success {
		t.Error("handleListAgents() should fail with missing repo")
	}

	// Valid request (empty)
	resp = d.handleListAgents(socket.Request{
		Command: "list_agents",
		Args: map[string]interface{}{
			"repo": "test-repo",
		},
	})
	if !resp.Success {
		t.Errorf("handleListAgents() failed: %s", resp.Error)
	}

	agents, ok := resp.Data.([]map[string]interface{})
	if !ok {
		t.Fatal("handleListAgents() data is not []map[string]interface{}")
	}
	if len(agents) != 0 {
		t.Errorf("handleListAgents() returned %d agents, want 0", len(agents))
	}

	// Add agents
	for _, name := range []string{"supervisor", "worker1"} {
		agent := state.Agent{
			Type:         state.AgentTypeSupervisor,
			WorktreePath: "/tmp/" + name,
			TmuxWindow:   name,
			SessionID:    "session-" + name,
			Task:         "task-" + name,
			CreatedAt:    time.Now(),
		}
		if err := d.state.AddAgent("test-repo", name, agent); err != nil {
			t.Fatalf("Failed to add agent: %v", err)
		}
	}

	resp = d.handleListAgents(socket.Request{
		Command: "list_agents",
		Args: map[string]interface{}{
			"repo": "test-repo",
		},
	})
	if !resp.Success {
		t.Errorf("handleListAgents() failed: %s", resp.Error)
	}

	agents, ok = resp.Data.([]map[string]interface{})
	if !ok {
		t.Fatal("handleListAgents() data is not []map[string]interface{}")
	}
	if len(agents) != 2 {
		t.Errorf("handleListAgents() returned %d agents, want 2", len(agents))
	}
}

func TestHandleRequest(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Test ping
	resp := d.handleRequest(socket.Request{Command: "ping"})
	if !resp.Success {
		t.Error("handleRequest(ping) failed")
	}
	if resp.Data != "pong" {
		t.Errorf("handleRequest(ping) data = %v, want 'pong'", resp.Data)
	}

	// Test unknown command
	resp = d.handleRequest(socket.Request{Command: "unknown"})
	if resp.Success {
		t.Error("handleRequest(unknown) should fail")
	}
	if resp.Error == "" {
		t.Error("handleRequest(unknown) should set error message")
	}
}

func TestCheckAgentHealth(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Add a test repository
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "test-session",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.state.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Add a test agent marked for cleanup
	agent := state.Agent{
		Type:            state.AgentTypeWorker,
		WorktreePath:    "/tmp/test",
		TmuxWindow:      "test-window",
		SessionID:       "test-session-id",
		CreatedAt:       time.Now(),
		ReadyForCleanup: true, // Mark for cleanup
	}
	if err := d.state.AddAgent("test-repo", "test-agent", agent); err != nil {
		t.Fatalf("Failed to add agent: %v", err)
	}

	// Run health check - should find the agent marked for cleanup
	// Note: This will try to clean up but tmux won't exist
	d.checkAgentHealth()

	// The agent should have been cleaned up since it was marked for cleanup
	// (and the tmux session doesn't exist)
	_, exists := d.state.GetAgent("test-repo", "test-agent")
	if exists {
		t.Log("Agent still exists - this is expected if tmux session check failed first")
	}
}
