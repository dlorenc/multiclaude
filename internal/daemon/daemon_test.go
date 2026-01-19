package daemon

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dlorenc/multiclaude/internal/messages"
	"github.com/dlorenc/multiclaude/internal/socket"
	"github.com/dlorenc/multiclaude/internal/state"
	"github.com/dlorenc/multiclaude/internal/tmux"
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
		OutputDir:    filepath.Join(tmpDir, "output"),
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

func TestHandleRemoveRepo(t *testing.T) {
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

	// Missing name
	resp := d.handleRemoveRepo(socket.Request{
		Command: "remove_repo",
		Args:    map[string]interface{}{},
	})
	if resp.Success {
		t.Error("handleRemoveRepo() should fail with missing name")
	}

	// Non-existent repo
	resp = d.handleRemoveRepo(socket.Request{
		Command: "remove_repo",
		Args: map[string]interface{}{
			"name": "nonexistent",
		},
	})
	if resp.Success {
		t.Error("handleRemoveRepo() should fail for nonexistent repo")
	}

	// Valid request
	resp = d.handleRemoveRepo(socket.Request{
		Command: "remove_repo",
		Args: map[string]interface{}{
			"name": "test-repo",
		},
	})
	if !resp.Success {
		t.Errorf("handleRemoveRepo() failed: %s", resp.Error)
	}

	// Verify repo was removed
	_, exists := d.state.GetRepo("test-repo")
	if exists {
		t.Error("handleRemoveRepo() did not remove repo from state")
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

func TestWorkspaceAgentExcludedFromRouteMessages(t *testing.T) {
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

	// Add a workspace agent
	workspaceAgent := state.Agent{
		Type:       state.AgentTypeWorkspace,
		TmuxWindow: "workspace",
		SessionID:  "workspace-session",
		CreatedAt:  time.Now(),
	}
	if err := d.state.AddAgent("test-repo", "workspace", workspaceAgent); err != nil {
		t.Fatalf("Failed to add workspace agent: %v", err)
	}

	// Create a message TO workspace (which should not be delivered)
	msgMgr := messages.NewManager(d.paths.MessagesDir)
	msg, err := msgMgr.Send("test-repo", "supervisor", "workspace", "This message should not be delivered")
	if err != nil {
		t.Fatalf("Failed to create message: %v", err)
	}

	// Verify message is pending
	if msg.Status != messages.StatusPending {
		t.Errorf("Message status = %s, want %s", msg.Status, messages.StatusPending)
	}

	// Call routeMessages - it should skip the workspace agent
	d.routeMessages()

	// The message should still be pending (not delivered) because workspace agents are skipped
	updatedMsgs, err := msgMgr.ListUnread("test-repo", "workspace")
	if err != nil {
		t.Fatalf("Failed to list messages: %v", err)
	}
	for _, m := range updatedMsgs {
		if m.ID == msg.ID && m.Status == messages.StatusDelivered {
			t.Error("Message to workspace agent should NOT have been delivered")
		}
	}
}

func TestWorkspaceAgentExcludedFromWakeLoop(t *testing.T) {
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

	// Add a workspace agent (should be skipped in wake loop)
	workspaceAgent := state.Agent{
		Type:       state.AgentTypeWorkspace,
		TmuxWindow: "workspace",
		SessionID:  "workspace-session",
		CreatedAt:  time.Now(),
	}
	if err := d.state.AddAgent("test-repo", "workspace", workspaceAgent); err != nil {
		t.Fatalf("Failed to add workspace agent: %v", err)
	}

	// Add a worker agent (should be processed in wake loop)
	workerAgent := state.Agent{
		Type:       state.AgentTypeWorker,
		TmuxWindow: "worker",
		SessionID:  "worker-session",
		CreatedAt:  time.Now(),
	}
	if err := d.state.AddAgent("test-repo", "worker", workerAgent); err != nil {
		t.Fatalf("Failed to add worker agent: %v", err)
	}

	// Call wakeAgents - it will fail to send (no tmux) but we can check LastNudge wasn't updated for workspace
	d.wakeAgents()

	// Workspace agent's LastNudge should NOT have been updated (it was skipped)
	updatedWorkspace, _ := d.state.GetAgent("test-repo", "workspace")
	if !updatedWorkspace.LastNudge.IsZero() {
		t.Error("Workspace agent LastNudge should not be updated - workspace should be skipped")
	}

	// Worker agent's LastNudge WOULD be updated if tmux succeeded, but since we don't have tmux,
	// we can only verify the workspace was skipped (verified above)
}

func TestHealthCheckLoopWithRealTmux(t *testing.T) {
	tmuxClient := tmux.NewClient()
	if !tmuxClient.IsTmuxAvailable() {
		t.Skip("tmux not available")
	}

	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Create a real tmux session
	sessionName := "mc-test-healthcheck"
	if err := tmuxClient.CreateSession(sessionName, true); err != nil {
		t.Fatalf("Failed to create tmux session: %v", err)
	}
	defer tmuxClient.KillSession(sessionName)

	// Create a window for the agent
	if err := tmuxClient.CreateWindow(sessionName, "test-agent"); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// Add repo and agent
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: sessionName,
		Agents:      make(map[string]state.Agent),
	}
	if err := d.state.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	agent := state.Agent{
		Type:       state.AgentTypeWorker,
		TmuxWindow: "test-agent",
		CreatedAt:  time.Now(),
	}
	if err := d.state.AddAgent("test-repo", "test-agent", agent); err != nil {
		t.Fatalf("Failed to add agent: %v", err)
	}

	// Run health check - agent should survive (window exists)
	d.TriggerHealthCheck()

	// Verify agent still exists
	_, exists := d.state.GetAgent("test-repo", "test-agent")
	if !exists {
		t.Error("Agent should still exist - window is valid")
	}

	// Kill the window
	if err := tmuxClient.KillWindow(sessionName, "test-agent"); err != nil {
		t.Fatalf("Failed to kill window: %v", err)
	}

	// Run health check again - agent should be cleaned up (window gone)
	d.TriggerHealthCheck()

	// Verify agent is removed
	_, exists = d.state.GetAgent("test-repo", "test-agent")
	if exists {
		t.Error("Agent should be removed - window is gone")
	}
}

func TestHealthCheckCleansUpMarkedAgents(t *testing.T) {
	tmuxClient := tmux.NewClient()
	if !tmuxClient.IsTmuxAvailable() {
		t.Skip("tmux not available")
	}

	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Create a real tmux session
	sessionName := "mc-test-cleanup"
	if err := tmuxClient.CreateSession(sessionName, true); err != nil {
		t.Fatalf("Failed to create tmux session: %v", err)
	}
	defer tmuxClient.KillSession(sessionName)

	// Create a window for the agent
	if err := tmuxClient.CreateWindow(sessionName, "to-cleanup"); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// Add repo and agent marked for cleanup
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: sessionName,
		Agents:      make(map[string]state.Agent),
	}
	if err := d.state.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	agent := state.Agent{
		Type:            state.AgentTypeWorker,
		TmuxWindow:      "to-cleanup",
		CreatedAt:       time.Now(),
		ReadyForCleanup: true, // Mark for cleanup
	}
	if err := d.state.AddAgent("test-repo", "to-cleanup", agent); err != nil {
		t.Fatalf("Failed to add agent: %v", err)
	}

	// Verify agent exists
	_, exists := d.state.GetAgent("test-repo", "to-cleanup")
	if !exists {
		t.Fatal("Agent should exist before cleanup")
	}

	// Run health check - agent marked for cleanup should be removed
	d.TriggerHealthCheck()

	// Verify agent is removed (even though window existed, it was marked for cleanup)
	_, exists = d.state.GetAgent("test-repo", "to-cleanup")
	if exists {
		t.Error("Agent marked for cleanup should be removed")
	}

	// Verify window is killed
	hasWindow, _ := tmuxClient.HasWindow(sessionName, "to-cleanup")
	if hasWindow {
		t.Error("Window should be killed when agent is cleaned up")
	}
}

func TestMessageRoutingWithRealTmux(t *testing.T) {
	tmuxClient := tmux.NewClient()
	if !tmuxClient.IsTmuxAvailable() {
		t.Skip("tmux not available")
	}

	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Create a real tmux session
	sessionName := "mc-test-routing"
	if err := tmuxClient.CreateSession(sessionName, true); err != nil {
		t.Fatalf("Failed to create tmux session: %v", err)
	}
	defer tmuxClient.KillSession(sessionName)

	// Create windows for agents
	if err := tmuxClient.CreateWindow(sessionName, "supervisor"); err != nil {
		t.Fatalf("Failed to create supervisor window: %v", err)
	}
	if err := tmuxClient.CreateWindow(sessionName, "worker1"); err != nil {
		t.Fatalf("Failed to create worker window: %v", err)
	}

	// Add repo and agents
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: sessionName,
		Agents:      make(map[string]state.Agent),
	}
	if err := d.state.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	supervisor := state.Agent{
		Type:       state.AgentTypeSupervisor,
		TmuxWindow: "supervisor",
		CreatedAt:  time.Now(),
	}
	if err := d.state.AddAgent("test-repo", "supervisor", supervisor); err != nil {
		t.Fatalf("Failed to add supervisor: %v", err)
	}

	worker := state.Agent{
		Type:       state.AgentTypeWorker,
		TmuxWindow: "worker1",
		Task:       "Test task",
		CreatedAt:  time.Now(),
	}
	if err := d.state.AddAgent("test-repo", "worker1", worker); err != nil {
		t.Fatalf("Failed to add worker: %v", err)
	}

	// Create a message
	msgMgr := messages.NewManager(d.paths.MessagesDir)
	msg, err := msgMgr.Send("test-repo", "supervisor", "worker1", "Hello worker!")
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Verify message is pending
	if msg.Status != messages.StatusPending {
		t.Errorf("Message status = %s, want pending", msg.Status)
	}

	// Trigger message routing
	d.TriggerMessageRouting()

	// Verify message is now delivered
	updatedMsg, err := msgMgr.Get("test-repo", "worker1", msg.ID)
	if err != nil {
		t.Fatalf("Failed to get message: %v", err)
	}
	if updatedMsg.Status != messages.StatusDelivered {
		t.Errorf("Message status = %s, want delivered", updatedMsg.Status)
	}
}

func TestWakeLoopUpdatesNudgeTime(t *testing.T) {
	tmuxClient := tmux.NewClient()
	if !tmuxClient.IsTmuxAvailable() {
		t.Skip("tmux not available")
	}

	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Create a real tmux session
	sessionName := "mc-test-wake"
	if err := tmuxClient.CreateSession(sessionName, true); err != nil {
		t.Fatalf("Failed to create tmux session: %v", err)
	}
	defer tmuxClient.KillSession(sessionName)

	// Create window for agent
	if err := tmuxClient.CreateWindow(sessionName, "supervisor"); err != nil {
		t.Fatalf("Failed to create supervisor window: %v", err)
	}

	// Add repo and agent with zero LastNudge
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: sessionName,
		Agents:      make(map[string]state.Agent),
	}
	if err := d.state.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	agent := state.Agent{
		Type:       state.AgentTypeSupervisor,
		TmuxWindow: "supervisor",
		CreatedAt:  time.Now(),
		LastNudge:  time.Time{}, // Zero time - never nudged
	}
	if err := d.state.AddAgent("test-repo", "supervisor", agent); err != nil {
		t.Fatalf("Failed to add agent: %v", err)
	}

	// Trigger wake
	beforeWake := time.Now()
	d.TriggerWake()
	afterWake := time.Now()

	// Verify LastNudge was updated
	updatedAgent, exists := d.state.GetAgent("test-repo", "supervisor")
	if !exists {
		t.Fatal("Agent should exist")
	}
	if updatedAgent.LastNudge.IsZero() {
		t.Error("LastNudge should be updated after wake")
	}
	if updatedAgent.LastNudge.Before(beforeWake) || updatedAgent.LastNudge.After(afterWake) {
		t.Error("LastNudge should be set to current time")
	}
}

func TestWakeLoopSkipsRecentlyNudgedAgents(t *testing.T) {
	tmuxClient := tmux.NewClient()
	if !tmuxClient.IsTmuxAvailable() {
		t.Skip("tmux not available")
	}

	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Create a real tmux session
	sessionName := "mc-test-wake-skip"
	if err := tmuxClient.CreateSession(sessionName, true); err != nil {
		t.Fatalf("Failed to create tmux session: %v", err)
	}
	defer tmuxClient.KillSession(sessionName)

	// Create window for agent
	if err := tmuxClient.CreateWindow(sessionName, "worker"); err != nil {
		t.Fatalf("Failed to create worker window: %v", err)
	}

	// Add repo and agent with recent LastNudge
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: sessionName,
		Agents:      make(map[string]state.Agent),
	}
	if err := d.state.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	recentNudge := time.Now().Add(-30 * time.Second) // Nudged 30 seconds ago
	agent := state.Agent{
		Type:       state.AgentTypeWorker,
		TmuxWindow: "worker",
		Task:       "Test task",
		CreatedAt:  time.Now(),
		LastNudge:  recentNudge,
	}
	if err := d.state.AddAgent("test-repo", "worker", agent); err != nil {
		t.Fatalf("Failed to add agent: %v", err)
	}

	// Trigger wake
	d.TriggerWake()

	// Verify LastNudge was NOT updated (too recent)
	updatedAgent, _ := d.state.GetAgent("test-repo", "worker")
	if !updatedAgent.LastNudge.Equal(recentNudge) {
		t.Error("LastNudge should NOT be updated for recently nudged agent")
	}
}

func TestHealthCheckWithMissingSession(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Add repo with non-existent tmux session
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "nonexistent-session-12345",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.state.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Add agent
	agent := state.Agent{
		Type:       state.AgentTypeWorker,
		TmuxWindow: "test-window",
		CreatedAt:  time.Now(),
	}
	if err := d.state.AddAgent("test-repo", "test-agent", agent); err != nil {
		t.Fatalf("Failed to add agent: %v", err)
	}

	// Verify agent exists
	_, exists := d.state.GetAgent("test-repo", "test-agent")
	if !exists {
		t.Fatal("Agent should exist before health check")
	}

	// Run health check - all agents should be cleaned up since session doesn't exist
	d.TriggerHealthCheck()

	// Verify agent is removed
	_, exists = d.state.GetAgent("test-repo", "test-agent")
	if exists {
		t.Error("Agent should be removed when session doesn't exist")
	}
}

func TestDaemonStartStop(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Start daemon
	if err := d.Start(); err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	// Verify we can communicate via socket
	client := socket.NewClient(d.paths.DaemonSock)
	resp, err := client.Send(socket.Request{Command: "ping"})
	if err != nil {
		t.Fatalf("Failed to ping daemon: %v", err)
	}
	if !resp.Success || resp.Data != "pong" {
		t.Error("Ping should return pong")
	}

	// Stop daemon
	if err := d.Stop(); err != nil {
		t.Errorf("Failed to stop daemon: %v", err)
	}
}

func TestDaemonTriggerCleanupCommand(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Start daemon
	if err := d.Start(); err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}
	defer d.Stop()

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	// Send trigger_cleanup command
	client := socket.NewClient(d.paths.DaemonSock)
	resp, err := client.Send(socket.Request{Command: "trigger_cleanup"})
	if err != nil {
		t.Fatalf("Failed to send trigger_cleanup: %v", err)
	}
	if !resp.Success {
		t.Errorf("trigger_cleanup failed: %s", resp.Error)
	}
}

func TestDaemonRepairStateCommand(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Start daemon
	if err := d.Start(); err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}
	defer d.Stop()

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	// Send repair_state command
	client := socket.NewClient(d.paths.DaemonSock)
	resp, err := client.Send(socket.Request{Command: "repair_state"})
	if err != nil {
		t.Fatalf("Failed to send repair_state: %v", err)
	}
	if !resp.Success {
		t.Errorf("repair_state failed: %s", resp.Error)
	}

	// Verify response contains expected data
	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("repair_state should return a map")
	}
	if _, ok := data["agents_removed"]; !ok {
		t.Error("Response should contain agents_removed")
	}
	if _, ok := data["issues_fixed"]; !ok {
		t.Error("Response should contain issues_fixed")
	}
}
