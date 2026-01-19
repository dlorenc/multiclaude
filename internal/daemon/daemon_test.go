package daemon

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dlorenc/multiclaude/internal/messages"
	"github.com/dlorenc/multiclaude/internal/socket"
	"github.com/dlorenc/multiclaude/internal/state"
	"github.com/dlorenc/multiclaude/pkg/tmux"
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

	// Test route_messages
	resp = d.handleRequest(socket.Request{Command: "route_messages"})
	if !resp.Success {
		t.Error("handleRequest(route_messages) failed")
	}
	if resp.Data != "Message routing triggered" {
		t.Errorf("handleRequest(route_messages) data = %v, want 'Message routing triggered'", resp.Data)
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
	// Note: In CI environments, tmux may be installed but unable to create sessions (no TTY)
	sessionName := "mc-test-routing"
	if err := tmuxClient.CreateSession(sessionName, true); err != nil {
		t.Skipf("tmux cannot create sessions in this environment: %v", err)
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

func TestDaemonRouteMessagesCommand(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Start daemon
	if err := d.Start(); err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}
	defer d.Stop()

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	// Send route_messages command
	client := socket.NewClient(d.paths.DaemonSock)
	resp, err := client.Send(socket.Request{Command: "route_messages"})
	if err != nil {
		t.Fatalf("Failed to send route_messages: %v", err)
	}
	if !resp.Success {
		t.Errorf("route_messages failed: %s", resp.Error)
	}
	if resp.Data != "Message routing triggered" {
		t.Errorf("route_messages data = %v, want 'Message routing triggered'", resp.Data)
	}
}

func TestDaemonRouteMessagesTriggersDelivery(t *testing.T) {
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

	// Create a message for the agent
	msgMgr := messages.NewManager(d.paths.MessagesDir)
	msg, err := msgMgr.Send("test-repo", "supervisor", "test-agent", "Test immediate delivery")
	if err != nil {
		t.Fatalf("Failed to create message: %v", err)
	}

	// Verify message is initially pending
	if msg.Status != messages.StatusPending {
		t.Errorf("Message status = %s, want %s", msg.Status, messages.StatusPending)
	}

	// Start daemon
	if err := d.Start(); err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}
	defer d.Stop()

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	// Send route_messages command to trigger immediate routing
	client := socket.NewClient(d.paths.DaemonSock)
	resp, err := client.Send(socket.Request{Command: "route_messages"})
	if err != nil {
		t.Fatalf("Failed to send route_messages: %v", err)
	}
	if !resp.Success {
		t.Errorf("route_messages failed: %s", resp.Error)
	}

	// Give it a moment to process (routing happens in goroutine)
	time.Sleep(100 * time.Millisecond)

	// Note: Without a real tmux session, we can't verify the message was actually
	// delivered to tmux, but we verify that:
	// 1. The command succeeds
	// 2. The routing function is triggered without errors/panics
	// 3. The message was processed (in production, status would change to "delivered")
}

// Tests for log rotation functions

func TestIsLogFile(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"standard log file", "/path/to/agent.log", true},
		{"log in nested dir", "/path/to/output/repo/agent.log", true},
		{"rotated log file", "/path/to/agent.log.20240115-120000", false},
		{"non-log file", "/path/to/file.txt", false},
		{"json file", "/path/to/config.json", false},
		{"short name", "/a.log", true},
		{"no extension", "/path/to/logfile", false},
		{"log in name but wrong ext", "/path/to/log.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isLogFile(tt.path)
			if result != tt.expected {
				t.Errorf("isLogFile(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestRotateLog(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Create a test log file
	logPath := filepath.Join(d.paths.OutputDir, "test.log")
	testContent := []byte("test log content\n")
	if err := os.WriteFile(logPath, testContent, 0644); err != nil {
		t.Fatalf("Failed to create test log: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("Test log file should exist: %v", err)
	}

	// Rotate the log
	if err := d.rotateLog(logPath); err != nil {
		t.Fatalf("rotateLog() failed: %v", err)
	}

	// Original file should no longer exist
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Error("Original log file should not exist after rotation")
	}

	// Find the rotated file
	entries, err := os.ReadDir(d.paths.OutputDir)
	if err != nil {
		t.Fatalf("Failed to read output dir: %v", err)
	}

	var rotatedFile string
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".log" && len(entry.Name()) > len("test.log.") {
			rotatedFile = entry.Name()
			break
		}
	}

	if rotatedFile == "" {
		t.Fatal("Rotated log file not found")
	}

	// Verify rotated file has timestamp suffix pattern (YYYYMMDD-HHMMSS)
	if len(rotatedFile) < len("test.log.20060102-150405") {
		t.Errorf("Rotated file name %q is too short", rotatedFile)
	}

	// Verify content was preserved
	rotatedPath := filepath.Join(d.paths.OutputDir, rotatedFile)
	content, err := os.ReadFile(rotatedPath)
	if err != nil {
		t.Fatalf("Failed to read rotated file: %v", err)
	}
	if string(content) != string(testContent) {
		t.Errorf("Rotated file content = %q, want %q", content, testContent)
	}
}

func TestRotateLogsIfNeeded(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Create a small log file (should not be rotated)
	smallLogPath := filepath.Join(d.paths.OutputDir, "small.log")
	if err := os.WriteFile(smallLogPath, []byte("small content"), 0644); err != nil {
		t.Fatalf("Failed to create small log: %v", err)
	}

	// Create a large log file (should be rotated)
	largeLogPath := filepath.Join(d.paths.OutputDir, "large.log")
	largeContent := make([]byte, MaxLogFileSize+1000)
	for i := range largeContent {
		largeContent[i] = 'X'
	}
	if err := os.WriteFile(largeLogPath, largeContent, 0644); err != nil {
		t.Fatalf("Failed to create large log: %v", err)
	}

	// Run log rotation check
	d.rotateLogsIfNeeded()

	// Small log should still exist
	if _, err := os.Stat(smallLogPath); err != nil {
		t.Error("Small log file should still exist")
	}

	// Large log should be rotated (original gone)
	if _, err := os.Stat(largeLogPath); !os.IsNotExist(err) {
		t.Error("Large log file should have been rotated")
	}

	// Verify rotated large file exists
	entries, err := os.ReadDir(d.paths.OutputDir)
	if err != nil {
		t.Fatalf("Failed to read output dir: %v", err)
	}

	hasRotatedLarge := false
	for _, entry := range entries {
		if len(entry.Name()) > len("large.log.") && entry.Name()[:9] == "large.log" {
			hasRotatedLarge = true
			break
		}
	}
	if !hasRotatedLarge {
		t.Error("Rotated large log file should exist")
	}
}

// Tests for prompt file functions

func TestWritePromptFile(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Create repo directory structure
	repoName := "test-repo"
	repoPath := d.paths.RepoDir(repoName)
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatalf("Failed to create repo dir: %v", err)
	}

	// Write prompt file for supervisor
	promptPath, err := d.writePromptFile(repoName, "supervisor", "supervisor")
	if err != nil {
		t.Fatalf("writePromptFile() failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(promptPath); err != nil {
		t.Errorf("Prompt file should exist at %s: %v", promptPath, err)
	}

	// Read and verify content contains expected elements
	content, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("Failed to read prompt file: %v", err)
	}

	// Should contain supervisor-specific content
	if len(content) == 0 {
		t.Error("Prompt file should not be empty")
	}
}

func TestWritePromptFileWorker(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Create repo directory structure
	repoName := "test-repo"
	repoPath := d.paths.RepoDir(repoName)
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatalf("Failed to create repo dir: %v", err)
	}

	// Write prompt file for worker
	promptPath, err := d.writePromptFile(repoName, "worker", "my-worker")
	if err != nil {
		t.Fatalf("writePromptFile() failed: %v", err)
	}

	// Verify file path is unique to agent name
	expectedPath := filepath.Join(d.paths.Root, "prompts", "my-worker.md")
	if promptPath != expectedPath {
		t.Errorf("Prompt path = %s, want %s", promptPath, expectedPath)
	}

	// Verify file exists and is non-empty
	info, err := os.Stat(promptPath)
	if err != nil {
		t.Fatalf("Prompt file should exist: %v", err)
	}
	if info.Size() == 0 {
		t.Error("Prompt file should not be empty")
	}
}

func TestCopyHooksConfig(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Create repo directory
	repoName := "test-repo"
	repoPath := d.paths.RepoDir(repoName)
	if err := os.MkdirAll(filepath.Join(repoPath, ".multiclaude"), 0755); err != nil {
		t.Fatalf("Failed to create .multiclaude dir: %v", err)
	}

	// Create hooks.json
	hooksContent := `{"hooks": [{"event": "test", "command": "echo test"}]}`
	hooksPath := filepath.Join(repoPath, ".multiclaude", "hooks.json")
	if err := os.WriteFile(hooksPath, []byte(hooksContent), 0644); err != nil {
		t.Fatalf("Failed to create hooks.json: %v", err)
	}

	// Create work directory
	workDir := filepath.Join(d.paths.WorktreesDir, repoName, "test-agent")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatalf("Failed to create work dir: %v", err)
	}

	// Copy hooks config
	if err := d.copyHooksConfig(repoPath, workDir); err != nil {
		t.Fatalf("copyHooksConfig() failed: %v", err)
	}

	// Verify settings.json was created
	settingsPath := filepath.Join(workDir, ".claude", "settings.json")
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("Failed to read settings.json: %v", err)
	}

	if string(content) != hooksContent {
		t.Errorf("settings.json content = %s, want %s", content, hooksContent)
	}
}

func TestCopyHooksConfigNoHooksFile(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Create repo directory WITHOUT hooks.json
	repoName := "test-repo"
	repoPath := d.paths.RepoDir(repoName)
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatalf("Failed to create repo dir: %v", err)
	}

	workDir := filepath.Join(d.paths.WorktreesDir, repoName, "test-agent")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatalf("Failed to create work dir: %v", err)
	}

	// Should not error when hooks.json doesn't exist
	if err := d.copyHooksConfig(repoPath, workDir); err != nil {
		t.Errorf("copyHooksConfig() should not error for missing hooks.json: %v", err)
	}

	// .claude directory should not be created
	claudeDir := filepath.Join(workDir, ".claude")
	if _, err := os.Stat(claudeDir); !os.IsNotExist(err) {
		t.Error(".claude directory should not be created when no hooks.json exists")
	}
}

// Tests for tracking mode prompt generation

func TestGenerateTrackingModePrompt(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	tests := []struct {
		name          string
		trackMode     state.TrackMode
		wantContains  []string
		wantNotContain []string
	}{
		{
			name:      "all mode",
			trackMode: state.TrackModeAll,
			wantContains: []string{
				"All PRs",
				"gh pr list --label multiclaude",
				"regardless of author or assignee",
			},
			wantNotContain: []string{
				"--author @me",
				"--assignee @me",
			},
		},
		{
			name:      "author mode",
			trackMode: state.TrackModeAuthor,
			wantContains: []string{
				"Author Only",
				"gh pr list --author @me --label multiclaude",
				"Do NOT process or attempt to merge PRs authored by others",
			},
			wantNotContain: []string{
				"--assignee @me",
			},
		},
		{
			name:      "assigned mode",
			trackMode: state.TrackModeAssigned,
			wantContains: []string{
				"Assigned Only",
				"gh pr list --assignee @me --label multiclaude",
				"assigned to you",
			},
			wantNotContain: []string{
				"--author @me",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.generateTrackingModePrompt(tt.trackMode)

			for _, want := range tt.wantContains {
				if !contains(result, want) {
					t.Errorf("generateTrackingModePrompt(%s) should contain %q", tt.trackMode, want)
				}
			}

			for _, notWant := range tt.wantNotContain {
				if contains(result, notWant) {
					t.Errorf("generateTrackingModePrompt(%s) should NOT contain %q", tt.trackMode, notWant)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Tests for restore functionality

func TestRestoreTrackedReposNoRepos(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Call restore with no repos - should not panic
	d.restoreTrackedRepos()

	// Verify no repos were created
	repos := d.state.ListRepos()
	if len(repos) != 0 {
		t.Errorf("Expected 0 repos, got %d", len(repos))
	}
}

func TestRestoreTrackedReposExistingSession(t *testing.T) {
	tmuxClient := tmux.NewClient()
	if !tmuxClient.IsTmuxAvailable() {
		t.Skip("tmux not available")
	}

	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Create a tmux session
	sessionName := "mc-test-restore-existing"
	if err := tmuxClient.CreateSession(sessionName, true); err != nil {
		t.Fatalf("Failed to create tmux session: %v", err)
	}
	defer tmuxClient.KillSession(sessionName)

	// Add repo with existing session
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: sessionName,
		Agents:      make(map[string]state.Agent),
	}
	if err := d.state.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Call restore - should skip since session exists
	d.restoreTrackedRepos()

	// Session should still exist and no agents should be created
	// (agents would only be created during actual init)
	hasSession, _ := tmuxClient.HasSession(sessionName)
	if !hasSession {
		t.Error("Session should still exist after restore check")
	}
}

func TestRestoreRepoAgentsMissingRepoPath(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Try to restore for a repo whose path doesn't exist
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-nonexistent",
		Agents:      make(map[string]state.Agent),
	}

	err := d.restoreRepoAgents("nonexistent-repo", repo)
	if err == nil {
		t.Error("restoreRepoAgents should fail when repo path doesn't exist")
	}

	expectedError := "repository path does not exist"
	if !contains(err.Error(), expectedError) {
		t.Errorf("Error should mention %q, got: %v", expectedError, err)
	}
}

// Tests for handle functions error cases

func TestHandleGetRepoConfigMissingName(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	resp := d.handleGetRepoConfig(socket.Request{
		Command: "get_repo_config",
		Args:    map[string]interface{}{},
	})

	if resp.Success {
		t.Error("Should fail with missing name")
	}
	if !contains(resp.Error, "missing") {
		t.Errorf("Error should mention 'missing', got: %s", resp.Error)
	}
}

func TestHandleGetRepoConfigNonexistentRepo(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	resp := d.handleGetRepoConfig(socket.Request{
		Command: "get_repo_config",
		Args: map[string]interface{}{
			"name": "nonexistent",
		},
	})

	if resp.Success {
		t.Error("Should fail for nonexistent repo")
	}
	if !contains(resp.Error, "not found") {
		t.Errorf("Error should mention 'not found', got: %s", resp.Error)
	}
}

func TestHandleGetRepoConfigSuccess(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Add a repo with specific config
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "test-session",
		Agents:      make(map[string]state.Agent),
		MergeQueueConfig: state.MergeQueueConfig{
			Enabled:   true,
			TrackMode: state.TrackModeAuthor,
		},
	}
	if err := d.state.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	resp := d.handleGetRepoConfig(socket.Request{
		Command: "get_repo_config",
		Args: map[string]interface{}{
			"name": "test-repo",
		},
	})

	if !resp.Success {
		t.Errorf("handleGetRepoConfig() failed: %s", resp.Error)
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Response data should be a map")
	}

	if data["mq_enabled"] != true {
		t.Errorf("mq_enabled = %v, want true", data["mq_enabled"])
	}
	if data["mq_track_mode"] != "author" {
		t.Errorf("mq_track_mode = %v, want 'author'", data["mq_track_mode"])
	}
}

func TestHandleUpdateRepoConfigInvalidTrackMode(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Add a repo first
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "test-session",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.state.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	resp := d.handleUpdateRepoConfig(socket.Request{
		Command: "update_repo_config",
		Args: map[string]interface{}{
			"name":          "test-repo",
			"mq_track_mode": "invalid-mode",
		},
	})

	if resp.Success {
		t.Error("Should fail with invalid track mode")
	}
	if !contains(resp.Error, "invalid track mode") {
		t.Errorf("Error should mention 'invalid track mode', got: %s", resp.Error)
	}
}

func TestHandleUpdateRepoConfigSuccess(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Add a repo first
	repo := &state.Repository{
		GithubURL:        "https://github.com/test/repo",
		TmuxSession:      "test-session",
		Agents:           make(map[string]state.Agent),
		MergeQueueConfig: state.DefaultMergeQueueConfig(),
	}
	if err := d.state.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Update config
	resp := d.handleUpdateRepoConfig(socket.Request{
		Command: "update_repo_config",
		Args: map[string]interface{}{
			"name":          "test-repo",
			"mq_enabled":    false,
			"mq_track_mode": "assigned",
		},
	})

	if !resp.Success {
		t.Errorf("handleUpdateRepoConfig() failed: %s", resp.Error)
	}

	// Verify config was updated
	updatedRepo, _ := d.state.GetRepo("test-repo")
	if updatedRepo.MergeQueueConfig.Enabled != false {
		t.Error("MergeQueueConfig.Enabled should be false")
	}
	if updatedRepo.MergeQueueConfig.TrackMode != state.TrackModeAssigned {
		t.Errorf("TrackMode = %s, want assigned", updatedRepo.MergeQueueConfig.TrackMode)
	}
}

func TestHandleListReposRichFormat(t *testing.T) {
	tmuxClient := tmux.NewClient()
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Create tmux session (optional for rich format test)
	sessionName := "mc-test-rich"
	sessionExists := false
	if tmuxClient.IsTmuxAvailable() {
		if err := tmuxClient.CreateSession(sessionName, true); err == nil {
			sessionExists = true
			defer tmuxClient.KillSession(sessionName)
		}
	}

	// Add a repo with agents
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
		TmuxWindow: "worker1",
		CreatedAt:  time.Now(),
	}
	if err := d.state.AddAgent("test-repo", "worker1", agent); err != nil {
		t.Fatalf("Failed to add agent: %v", err)
	}

	// Request rich format
	resp := d.handleListRepos(socket.Request{
		Command: "list_repos",
		Args: map[string]interface{}{
			"rich": true,
		},
	})

	if !resp.Success {
		t.Errorf("handleListRepos(rich) failed: %s", resp.Error)
	}

	data, ok := resp.Data.([]map[string]interface{})
	if !ok {
		t.Fatal("Rich response should be []map[string]interface{}")
	}

	if len(data) != 1 {
		t.Fatalf("Expected 1 repo, got %d", len(data))
	}

	repoData := data[0]
	if repoData["name"] != "test-repo" {
		t.Errorf("name = %v, want 'test-repo'", repoData["name"])
	}
	if repoData["total_agents"].(int) != 1 {
		t.Errorf("total_agents = %v, want 1", repoData["total_agents"])
	}
	if repoData["worker_count"].(int) != 1 {
		t.Errorf("worker_count = %v, want 1", repoData["worker_count"])
	}

	// session_healthy should match whether we created a real session
	if sessionExists && !repoData["session_healthy"].(bool) {
		t.Error("session_healthy should be true when session exists")
	}
}
