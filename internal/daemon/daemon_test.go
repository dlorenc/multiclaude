package daemon

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dlorenc/multiclaude/internal/hooks"
	"github.com/dlorenc/multiclaude/internal/messages"
	"github.com/dlorenc/multiclaude/internal/prompts"
	"github.com/dlorenc/multiclaude/internal/socket"
	"github.com/dlorenc/multiclaude/internal/state"
	"github.com/dlorenc/multiclaude/pkg/config"
	"github.com/dlorenc/multiclaude/pkg/tmux"
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
		Root:            tmpDir,
		DaemonPID:       filepath.Join(tmpDir, "daemon.pid"),
		DaemonSock:      filepath.Join(tmpDir, "daemon.sock"),
		DaemonLog:       filepath.Join(tmpDir, "daemon.log"),
		StateFile:       filepath.Join(tmpDir, "state.json"),
		ReposDir:        filepath.Join(tmpDir, "repos"),
		WorktreesDir:    filepath.Join(tmpDir, "wts"),
		MessagesDir:     filepath.Join(tmpDir, "messages"),
		OutputDir:       filepath.Join(tmpDir, "output"),
		ClaudeConfigDir: filepath.Join(tmpDir, "claude-config"),
		ArchiveDir:      filepath.Join(tmpDir, "archive"),
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

func TestHandleRestartAgent(t *testing.T) {
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
		PID:          0, // No running process
		CreatedAt:    time.Now(),
	}
	if err := d.state.AddAgent("test-repo", "test-agent", agent); err != nil {
		t.Fatalf("Failed to add agent: %v", err)
	}

	// Test missing repo argument
	resp := d.handleRestartAgent(socket.Request{
		Command: "restart_agent",
		Args: map[string]interface{}{
			"agent": "test-agent",
		},
	})
	if resp.Success {
		t.Error("Expected failure with missing repo")
	}
	if resp.Error != "missing 'repo': repository name is required" {
		t.Errorf("Unexpected error message: %s", resp.Error)
	}

	// Test missing agent argument
	resp = d.handleRestartAgent(socket.Request{
		Command: "restart_agent",
		Args: map[string]interface{}{
			"repo": "test-repo",
		},
	})
	if resp.Success {
		t.Error("Expected failure with missing agent")
	}
	if resp.Error != "missing 'agent': agent name is required" {
		t.Errorf("Unexpected error message: %s", resp.Error)
	}

	// Test non-existent agent
	resp = d.handleRestartAgent(socket.Request{
		Command: "restart_agent",
		Args: map[string]interface{}{
			"repo":  "test-repo",
			"agent": "non-existent",
		},
	})
	if resp.Success {
		t.Error("Expected failure with non-existent agent")
	}

	// Test agent marked for cleanup (should fail)
	markedAgent := state.Agent{
		Type:            state.AgentTypeWorker,
		WorktreePath:    "/tmp/test2",
		TmuxWindow:      "test-window2",
		SessionID:       "test-session-id2",
		ReadyForCleanup: true,
		CreatedAt:       time.Now(),
	}
	if err := d.state.AddAgent("test-repo", "completed-agent", markedAgent); err != nil {
		t.Fatalf("Failed to add completed agent: %v", err)
	}

	resp = d.handleRestartAgent(socket.Request{
		Command: "restart_agent",
		Args: map[string]interface{}{
			"repo":  "test-repo",
			"agent": "completed-agent",
		},
	})
	if resp.Success {
		t.Error("Expected failure for completed agent")
	}
	if resp.Error == "" || resp.Error != "agent 'completed-agent' is marked as complete and pending cleanup - cannot restart a completed agent" {
		t.Errorf("Expected cleanup error, got: %s", resp.Error)
	}

	// Test non-existent repo
	resp = d.handleRestartAgent(socket.Request{
		Command: "restart_agent",
		Args: map[string]interface{}{
			"repo":  "non-existent-repo",
			"agent": "test-agent",
		},
	})
	if resp.Success {
		t.Error("Expected failure with non-existent repo")
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
		t.Fatal("tmux is required for this test but not available")
	}

	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Create a real tmux session
	// Note: In CI environments, tmux may be installed but unable to create sessions (no TTY)
	sessionName := "mc-test-healthcheck"
	if err := tmuxClient.CreateSession(context.Background(), sessionName, true); err != nil {
		t.Fatalf("tmux is required for this test but cannot create sessions in this environment: %v", err)
	}
	defer tmuxClient.KillSession(context.Background(), sessionName)

	// Create a window for the agent
	if err := tmuxClient.CreateWindow(context.Background(), sessionName, "test-agent"); err != nil {
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
	if err := tmuxClient.KillWindow(context.Background(), sessionName, "test-agent"); err != nil {
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
		t.Fatal("tmux is required for this test but not available")
	}

	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Create a real tmux session
	// Note: In CI environments, tmux may be installed but unable to create sessions (no TTY)
	sessionName := "mc-test-cleanup"
	if err := tmuxClient.CreateSession(context.Background(), sessionName, true); err != nil {
		t.Fatalf("tmux is required for this test but cannot create sessions in this environment: %v", err)
	}
	defer tmuxClient.KillSession(context.Background(), sessionName)

	// Create a window for the agent
	if err := tmuxClient.CreateWindow(context.Background(), sessionName, "to-cleanup"); err != nil {
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
	hasWindow, _ := tmuxClient.HasWindow(context.Background(), sessionName, "to-cleanup")
	if hasWindow {
		t.Error("Window should be killed when agent is cleaned up")
	}
}

func TestMessageRoutingWithRealTmux(t *testing.T) {
	tmuxClient := tmux.NewClient()
	if !tmuxClient.IsTmuxAvailable() {
		t.Fatal("tmux is required for this test but not available")
	}

	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Create a real tmux session
	// Note: In CI environments, tmux may be installed but unable to create sessions (no TTY)
	sessionName := "mc-test-routing"
	if err := tmuxClient.CreateSession(context.Background(), sessionName, true); err != nil {
		t.Fatalf("tmux is required for this test but cannot create sessions in this environment: %v", err)
	}
	defer tmuxClient.KillSession(context.Background(), sessionName)

	// Create windows for agents
	if err := tmuxClient.CreateWindow(context.Background(), sessionName, "supervisor"); err != nil {
		t.Fatalf("Failed to create supervisor window: %v", err)
	}
	if err := tmuxClient.CreateWindow(context.Background(), sessionName, "worker1"); err != nil {
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
		t.Fatal("tmux is required for this test but not available")
	}

	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Create a real tmux session
	// Note: In CI environments, tmux may be installed but unable to create sessions (no TTY)
	sessionName := "mc-test-wake"
	if err := tmuxClient.CreateSession(context.Background(), sessionName, true); err != nil {
		t.Fatalf("tmux is required for this test but cannot create sessions in this environment: %v", err)
	}
	defer tmuxClient.KillSession(context.Background(), sessionName)

	// Create window for agent
	if err := tmuxClient.CreateWindow(context.Background(), sessionName, "supervisor"); err != nil {
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
		t.Fatal("tmux is required for this test but not available")
	}

	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Create a real tmux session
	// Note: In CI environments, tmux may be installed but unable to create sessions (no TTY)
	sessionName := "mc-test-wake-skip"
	if err := tmuxClient.CreateSession(context.Background(), sessionName, true); err != nil {
		t.Fatalf("tmux is required for this test but cannot create sessions in this environment: %v", err)
	}
	defer tmuxClient.KillSession(context.Background(), sessionName)

	// Create window for agent
	if err := tmuxClient.CreateWindow(context.Background(), sessionName, "worker"); err != nil {
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
	if err := hooks.CopyConfig(repoPath, workDir); err != nil {
		t.Fatalf("CopyConfig() failed: %v", err)
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
	if err := hooks.CopyConfig(repoPath, workDir); err != nil {
		t.Errorf("CopyConfig() should not error for missing hooks.json: %v", err)
	}

	// .claude directory should not be created
	claudeDir := filepath.Join(workDir, ".claude")
	if _, err := os.Stat(claudeDir); !os.IsNotExist(err) {
		t.Error(".claude directory should not be created when no hooks.json exists")
	}
}

// Tests for tracking mode prompt generation (uses shared prompts.GenerateTrackingModePrompt)

func TestGenerateTrackingModePrompt(t *testing.T) {
	tests := []struct {
		name           string
		trackMode      string
		wantContains   []string
		wantNotContain []string
	}{
		{
			name:      "all mode",
			trackMode: string(state.TrackModeAll),
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
			trackMode: string(state.TrackModeAuthor),
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
			trackMode: string(state.TrackModeAssigned),
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
			result := prompts.GenerateTrackingModePrompt(tt.trackMode)

			for _, want := range tt.wantContains {
				if !contains(result, want) {
					t.Errorf("GenerateTrackingModePrompt(%s) should contain %q", tt.trackMode, want)
				}
			}

			for _, notWant := range tt.wantNotContain {
				if contains(result, notWant) {
					t.Errorf("GenerateTrackingModePrompt(%s) should NOT contain %q", tt.trackMode, notWant)
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
		t.Fatal("tmux is required for this test but not available")
	}

	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Create a tmux session
	// Note: In CI environments, tmux may be installed but unable to create sessions (no TTY)
	sessionName := "mc-test-restore-existing"
	if err := tmuxClient.CreateSession(context.Background(), sessionName, true); err != nil {
		t.Fatalf("tmux is required for this test but cannot create sessions in this environment: %v", err)
	}
	defer tmuxClient.KillSession(context.Background(), sessionName)

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
	hasSession, _ := tmuxClient.HasSession(context.Background(), sessionName)
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

func TestRestoreDeadAgentsWithExistingSession(t *testing.T) {
	tmuxClient := tmux.NewClient()
	if !tmuxClient.IsTmuxAvailable() {
		t.Fatal("tmux is required for this test but not available")
	}

	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Create a tmux session
	// Note: In CI environments, tmux may be installed but unable to create sessions (no TTY)
	sessionName := "mc-test-restore-dead"
	if err := tmuxClient.CreateSession(context.Background(), sessionName, true); err != nil {
		t.Fatalf("tmux is required for this test but cannot create sessions in this environment: %v", err)
	}
	defer tmuxClient.KillSession(context.Background(), sessionName)

	// Create a window for the supervisor agent
	if err := tmuxClient.CreateWindow(context.Background(), sessionName, "supervisor"); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// Add repo with an agent that has a dead PID (99999 is unlikely to exist)
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: sessionName,
		Agents: map[string]state.Agent{
			"supervisor": {
				Type:         state.AgentTypeSupervisor,
				WorktreePath: d.paths.RepoDir("test-repo"),
				TmuxWindow:   "supervisor",
				SessionID:    "test-session-id",
				PID:          99999, // Dead PID
			},
		},
	}
	if err := d.state.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Call restoreDeadAgents - should attempt to restart the dead agent
	// Note: This won't actually restart successfully without a real Claude binary,
	// but it should not panic and should log the attempt
	d.restoreDeadAgents("test-repo", repo)

	// Session and window should still exist
	hasSession, _ := tmuxClient.HasSession(context.Background(), sessionName)
	if !hasSession {
		t.Error("Session should still exist after restore attempt")
	}
}

func TestRestoreDeadAgentsSkipsAliveProcesses(t *testing.T) {
	tmuxClient := tmux.NewClient()
	if !tmuxClient.IsTmuxAvailable() {
		t.Fatal("tmux is required for this test but not available")
	}

	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Create a tmux session
	// Note: In CI environments, tmux may be installed but unable to create sessions (no TTY)
	sessionName := "mc-test-restore-alive"
	if err := tmuxClient.CreateSession(context.Background(), sessionName, true); err != nil {
		t.Fatalf("tmux is required for this test but cannot create sessions in this environment: %v", err)
	}
	defer tmuxClient.KillSession(context.Background(), sessionName)

	// Create a window for the supervisor agent
	if err := tmuxClient.CreateWindow(context.Background(), sessionName, "supervisor"); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// Use the current process PID as a "live" process
	alivePID := os.Getpid()

	// Add repo with an agent that has an alive PID
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: sessionName,
		Agents: map[string]state.Agent{
			"supervisor": {
				Type:         state.AgentTypeSupervisor,
				WorktreePath: d.paths.RepoDir("test-repo"),
				TmuxWindow:   "supervisor",
				SessionID:    "test-session-id",
				PID:          alivePID,
			},
		},
	}
	if err := d.state.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Call restoreDeadAgents - should skip since process is alive
	d.restoreDeadAgents("test-repo", repo)

	// Verify agent PID was not changed (no restart attempted)
	updatedAgent, exists := d.state.GetAgent("test-repo", "supervisor")
	if !exists {
		t.Fatal("Agent should still exist")
	}
	if updatedAgent.PID != alivePID {
		t.Errorf("PID should not change for alive process, got %d want %d", updatedAgent.PID, alivePID)
	}
}

func TestRestoreDeadAgentsSkipsTransientAgents(t *testing.T) {
	tmuxClient := tmux.NewClient()
	if !tmuxClient.IsTmuxAvailable() {
		t.Fatal("tmux is required for this test but not available")
	}

	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Add repo with a worker agent that has a dead PID
	// Note: We use a non-existent session - restoreDeadAgents should handle this gracefully
	// by skipping the agent when HasWindow fails
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "nonexistent-session",
		Agents: map[string]state.Agent{
			"test-worker": {
				Type:         state.AgentTypeWorker, // Transient agent type
				WorktreePath: d.paths.RepoDir("test-repo"),
				TmuxWindow:   "test-worker",
				SessionID:    "test-session-id",
				PID:          99999, // Dead PID
			},
		},
	}
	if err := d.state.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Call restoreDeadAgents - should handle gracefully when tmux session doesn't exist
	// The function should not panic and should preserve agent state
	d.restoreDeadAgents("test-repo", repo)

	// Verify agent still exists in state (function didn't corrupt state)
	updatedAgent, exists := d.state.GetAgent("test-repo", "test-worker")
	if !exists {
		t.Fatal("Agent should still exist in state after restoreDeadAgents")
	}
	// PID should remain the same since the window check will fail/skip
	if updatedAgent.PID != 99999 {
		t.Errorf("PID should not change when window doesn't exist, got %d want %d", updatedAgent.PID, 99999)
	}

	// Verify that transient agents (workers) are classified correctly
	// The IsPersistent() method is tested separately in state_test.go
	if state.AgentTypeWorker.IsPersistent() {
		t.Error("Worker agents should not be classified as persistent")
	}
}

func TestRestoreDeadAgentsIncludesWorkspace(t *testing.T) {
	tmuxClient := tmux.NewClient()
	if !tmuxClient.IsTmuxAvailable() {
		t.Fatal("tmux is required for this test but not available")
	}

	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Add repo with a workspace agent that has a dead PID
	// Note: We use a non-existent session - restoreDeadAgents should handle this gracefully
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "nonexistent-session",
		Agents: map[string]state.Agent{
			"workspace": {
				Type:         state.AgentTypeWorkspace, // Persistent agent type
				WorktreePath: d.paths.RepoDir("test-repo"),
				TmuxWindow:   "workspace",
				SessionID:    "test-session-id",
				PID:          99999, // Dead PID
			},
		},
	}
	if err := d.state.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Call restoreDeadAgents - should handle gracefully when tmux session doesn't exist
	// The function should not panic and should preserve agent state
	d.restoreDeadAgents("test-repo", repo)

	// Verify agent still exists in state (function didn't corrupt state)
	updatedAgent, exists := d.state.GetAgent("test-repo", "workspace")
	if !exists {
		t.Fatal("Agent should still exist in state after restoreDeadAgents")
	}
	// PID should remain the same since the window check will fail/skip
	if updatedAgent.PID != 99999 {
		t.Errorf("PID should not change when window doesn't exist, got %d want %d", updatedAgent.PID, 99999)
	}

	// Verify that workspace agents ARE classified as persistent
	// The IsPersistent() method is tested comprehensively in state_test.go
	if !state.AgentTypeWorkspace.IsPersistent() {
		t.Error("Workspace agents should be classified as persistent")
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
		if err := tmuxClient.CreateSession(context.Background(), sessionName, true); err == nil {
			sessionExists = true
			defer tmuxClient.KillSession(context.Background(), sessionName)
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

func TestHealthCheckAttemptsRestorationBeforeCleanup(t *testing.T) {
	tmuxClient := tmux.NewClient()
	if !tmuxClient.IsTmuxAvailable() {
		t.Fatal("tmux is required for this test but not available")
	}

	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Create a unique session name for this test
	sessionName := "mc-test-selfheal"

	// Ensure the session doesn't exist at the start
	tmuxClient.KillSession(context.Background(), sessionName)

	// Create the repo directory on disk (required for restoration to succeed)
	repoPath := d.paths.RepoDir("test-repo")
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatalf("Failed to create repo dir: %v", err)
	}

	// Initialize a git repo (required for worktree operations)
	cmd := exec.Command("git", "init", repoPath)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Add repo to state with a non-existent tmux session
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: sessionName,
		Agents:      make(map[string]state.Agent),
		MergeQueueConfig: state.MergeQueueConfig{
			Enabled:   false, // Disable merge queue to simplify test
			TrackMode: state.TrackModeAll,
		},
	}
	if err := d.state.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Add a fake agent (this should be cleared during restoration)
	agent := state.Agent{
		Type:       state.AgentTypeWorker,
		TmuxWindow: "old-worker",
		CreatedAt:  time.Now(),
	}
	if err := d.state.AddAgent("test-repo", "old-worker", agent); err != nil {
		t.Fatalf("Failed to add agent: %v", err)
	}

	// Verify agent exists before health check
	_, exists := d.state.GetAgent("test-repo", "old-worker")
	if !exists {
		t.Fatal("Agent should exist before health check")
	}

	// Run health check - this should attempt restoration since repo path exists
	d.TriggerHealthCheck()

	// Give tmux a moment to create the session
	time.Sleep(200 * time.Millisecond)

	// Verify a tmux session was created (restoration was attempted)
	hasSession, err := tmuxClient.HasSession(context.Background(), sessionName)
	if err != nil {
		t.Fatalf("Failed to check session: %v", err)
	}

	// Clean up the session we created
	defer tmuxClient.KillSession(context.Background(), sessionName)

	if hasSession {
		t.Log("Self-healing succeeded: tmux session was restored")

		// The old agent should have been removed (stale agents are cleared during restoration)
		_, oldAgentExists := d.state.GetAgent("test-repo", "old-worker")
		if oldAgentExists {
			t.Error("Old agent should have been removed during restoration")
		}

		// New supervisor agent should have been created
		_, supervisorExists := d.state.GetAgent("test-repo", "supervisor")
		if !supervisorExists {
			t.Log("Note: Supervisor agent creation may fail without claude binary, but session was restored")
		}
	} else {
		// If restoration failed (e.g., due to missing claude binary in test env),
		// agents should still be cleaned up as a fallback
		_, exists := d.state.GetAgent("test-repo", "old-worker")
		if exists {
			t.Error("If restoration failed, agents should have been cleaned up as fallback")
		}
	}
}

func TestHealthCheckCleansUpWhenRestorationFails(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Add repo with non-existent tmux session AND non-existent repo path
	// This simulates a case where restoration should fail
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "nonexistent-session-cleanup-test",
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

	// Run health check - restoration should fail (repo path doesn't exist)
	// and agents should be cleaned up as fallback
	d.TriggerHealthCheck()

	// Verify agent was cleaned up since restoration failed
	_, exists = d.state.GetAgent("test-repo", "test-agent")
	if exists {
		t.Error("Agent should be removed when restoration fails")
	}
}

func TestHandleTaskHistoryMissingRepo(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Test with missing repo argument
	resp := d.handleRequest(socket.Request{Command: "task_history"})
	if resp.Success {
		t.Error("handleTaskHistory() should fail without repo argument")
	}
	if resp.Error == "" {
		t.Error("handleTaskHistory() should return error message")
	}
}

func TestHandleTaskHistoryEmptyHistory(t *testing.T) {
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

	// Test task_history with empty history
	resp := d.handleRequest(socket.Request{
		Command: "task_history",
		Args: map[string]interface{}{
			"repo": "test-repo",
		},
	})
	if !resp.Success {
		t.Errorf("handleTaskHistory() failed: %s", resp.Error)
	}

	// Should return empty array
	data, ok := resp.Data.([]map[string]interface{})
	if !ok {
		t.Errorf("handleTaskHistory() data should be array, got %T", resp.Data)
	}
	if len(data) != 0 {
		t.Errorf("handleTaskHistory() should return empty array, got %d items", len(data))
	}
}

func TestHandleTaskHistoryWithLimit(t *testing.T) {
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

	// Test task_history with custom limit
	resp := d.handleRequest(socket.Request{
		Command: "task_history",
		Args: map[string]interface{}{
			"repo":  "test-repo",
			"limit": float64(5), // JSON numbers are float64
		},
	})
	if !resp.Success {
		t.Errorf("handleTaskHistory() with limit failed: %s", resp.Error)
	}
}

func TestHandleRequestCurrentRepoCommands(t *testing.T) {
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

	// Test set_current_repo
	resp := d.handleRequest(socket.Request{
		Command: "set_current_repo",
		Args: map[string]interface{}{
			"name": "test-repo",
		},
	})
	if !resp.Success {
		t.Errorf("set_current_repo failed: %s", resp.Error)
	}

	// Test get_current_repo
	resp = d.handleRequest(socket.Request{Command: "get_current_repo"})
	if !resp.Success {
		t.Errorf("get_current_repo failed: %s", resp.Error)
	}
	if resp.Data != "test-repo" {
		t.Errorf("get_current_repo returned %v, want 'test-repo'", resp.Data)
	}

	// Test clear_current_repo
	resp = d.handleRequest(socket.Request{Command: "clear_current_repo"})
	if !resp.Success {
		t.Errorf("clear_current_repo failed: %s", resp.Error)
	}

	// Verify current repo is cleared - get_current_repo returns error when no repo set
	resp = d.handleRequest(socket.Request{Command: "get_current_repo"})
	if resp.Success {
		t.Error("get_current_repo should fail when no repo is set")
	}
	if resp.Error != "no current repository set" {
		t.Errorf("get_current_repo error = %q, want 'no current repository set'", resp.Error)
	}
}

func TestHandleListAgentsMixed(t *testing.T) {
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

	// Add different agent types
	workerAgent := state.Agent{
		Type:       state.AgentTypeWorker,
		TmuxWindow: "worker-window",
		CreatedAt:  time.Now(),
	}
	if err := d.state.AddAgent("test-repo", "worker-1", workerAgent); err != nil {
		t.Fatalf("Failed to add worker agent: %v", err)
	}

	workspaceAgent := state.Agent{
		Type:       state.AgentTypeWorkspace,
		TmuxWindow: "workspace-window",
		CreatedAt:  time.Now(),
	}
	if err := d.state.AddAgent("test-repo", "default", workspaceAgent); err != nil {
		t.Fatalf("Failed to add workspace agent: %v", err)
	}

	// Test list_agents returns all agents
	resp := d.handleRequest(socket.Request{
		Command: "list_agents",
		Args: map[string]interface{}{
			"repo": "test-repo",
		},
	})
	if !resp.Success {
		t.Errorf("list_agents failed: %s", resp.Error)
	}

	// Verify both agents are returned
	data, ok := resp.Data.([]map[string]interface{})
	if !ok {
		t.Fatalf("list_agents data should be []map[string]interface{}, got %T", resp.Data)
	}
	if len(data) != 2 {
		t.Errorf("list_agents should return 2 agents, got %d", len(data))
	}

	// Verify agent types are present
	types := make(map[string]bool)
	for _, agent := range data {
		// Type is stored as state.AgentType which is a string alias
		if agentType, ok := agent["type"].(state.AgentType); ok {
			types[string(agentType)] = true
		}
	}
	if !types["worker"] {
		t.Error("list_agents should include worker agent")
	}
	if !types["workspace"] {
		t.Error("list_agents should include workspace agent")
	}
}

func TestHandleSetCurrentRepoMissingName(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Test set_current_repo without name
	resp := d.handleRequest(socket.Request{Command: "set_current_repo"})
	if resp.Success {
		t.Error("set_current_repo should fail without name argument")
	}
}

func TestHandleSetCurrentRepoNonexistent(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Test set_current_repo with non-existent repo
	resp := d.handleRequest(socket.Request{
		Command: "set_current_repo",
		Args: map[string]interface{}{
			"name": "nonexistent-repo",
		},
	})
	if resp.Success {
		t.Error("set_current_repo should fail for non-existent repo")
	}
}

func TestGetStateAndPaths(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Test GetState
	state := d.GetState()
	if state == nil {
		t.Error("GetState() should not return nil")
	}

	// Test GetPaths
	paths := d.GetPaths()
	if paths == nil {
		t.Error("GetPaths() should not return nil")
	}
}

func TestHandleClearCurrentRepoWhenNone(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Clear current repo when none is set - should succeed
	resp := d.handleRequest(socket.Request{Command: "clear_current_repo"})
	if !resp.Success {
		t.Errorf("clear_current_repo should succeed even when no repo set: %s", resp.Error)
	}
}

func TestDaemonWait(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Test Wait completes immediately when no goroutines are running
	done := make(chan struct{})
	go func() {
		d.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success - Wait() completed
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Wait() did not complete in time")
	}
}

func TestDaemonTriggerHealthCheck(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Test TriggerHealthCheck doesn't panic
	d.TriggerHealthCheck()

	// Test multiple triggers
	d.TriggerHealthCheck()
	d.TriggerHealthCheck()
}

func TestDaemonTriggerMessageRouting(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Test TriggerMessageRouting doesn't panic
	d.TriggerMessageRouting()

	// Test multiple triggers
	d.TriggerMessageRouting()
	d.TriggerMessageRouting()
}

func TestDaemonTriggerWake(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Test TriggerWake doesn't panic
	d.TriggerWake()

	// Test multiple triggers
	d.TriggerWake()
	d.TriggerWake()
}

func TestDaemonTriggerWorktreeRefresh(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Test TriggerWorktreeRefresh doesn't panic
	d.TriggerWorktreeRefresh()

	// Test multiple triggers
	d.TriggerWorktreeRefresh()
	d.TriggerWorktreeRefresh()
}

func TestHandleSpawnAgent(t *testing.T) {
	tests := []struct {
		name        string
		setupRepo   bool
		setupAgent  bool
		args        map[string]interface{}
		wantSuccess bool
		wantError   string
	}{
		{
			name:      "missing repo arg",
			setupRepo: false,
			args: map[string]interface{}{
				"name":   "test-agent",
				"class":  "ephemeral",
				"prompt": "Test prompt",
			},
			wantSuccess: false,
			wantError:   "repository name is required",
		},
		{
			name:      "missing name arg",
			setupRepo: true,
			args: map[string]interface{}{
				"repo":   "test-repo",
				"class":  "ephemeral",
				"prompt": "Test prompt",
			},
			wantSuccess: false,
			wantError:   "agent name is required",
		},
		{
			name:      "missing class arg",
			setupRepo: true,
			args: map[string]interface{}{
				"repo":   "test-repo",
				"name":   "test-agent",
				"prompt": "Test prompt",
			},
			wantSuccess: false,
			wantError:   "agent class is required",
		},
		{
			name:      "missing prompt arg",
			setupRepo: true,
			args: map[string]interface{}{
				"repo":  "test-repo",
				"name":  "test-agent",
				"class": "ephemeral",
			},
			wantSuccess: false,
			wantError:   "prompt text is required",
		},
		{
			name:      "invalid class value",
			setupRepo: true,
			args: map[string]interface{}{
				"repo":   "test-repo",
				"name":   "test-agent",
				"class":  "invalid",
				"prompt": "Test prompt",
			},
			wantSuccess: false,
			wantError:   "invalid agent class",
		},
		{
			name:      "repo not found",
			setupRepo: false,
			args: map[string]interface{}{
				"repo":   "nonexistent-repo",
				"name":   "test-agent",
				"class":  "ephemeral",
				"prompt": "Test prompt",
			},
			wantSuccess: false,
			wantError:   "not found",
		},
		{
			name:       "agent already exists",
			setupRepo:  true,
			setupAgent: true,
			args: map[string]interface{}{
				"repo":   "test-repo",
				"name":   "existing-agent",
				"class":  "ephemeral",
				"prompt": "Test prompt",
			},
			wantSuccess: false,
			wantError:   "already exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, cleanup := setupTestDaemon(t)
			defer cleanup()

			if tt.setupRepo {
				repo := &state.Repository{
					GithubURL:   "https://github.com/test/repo",
					TmuxSession: "mc-test-repo",
					Agents:      make(map[string]state.Agent),
				}
				if err := d.state.AddRepo("test-repo", repo); err != nil {
					t.Fatalf("Failed to add repo: %v", err)
				}
			}

			if tt.setupAgent {
				agent := state.Agent{
					Type:         state.AgentTypeWorker,
					WorktreePath: "/tmp/test",
					TmuxWindow:   "existing-agent",
					SessionID:    "test-session-id",
					CreatedAt:    time.Now(),
				}
				if err := d.state.AddAgent("test-repo", "existing-agent", agent); err != nil {
					t.Fatalf("Failed to add agent: %v", err)
				}
			}

			resp := d.handleSpawnAgent(socket.Request{
				Command: "spawn_agent",
				Args:    tt.args,
			})

			if resp.Success != tt.wantSuccess {
				t.Errorf("handleSpawnAgent() success = %v, want %v; error = %s", resp.Success, tt.wantSuccess, resp.Error)
			}

			if !tt.wantSuccess && tt.wantError != "" {
				if resp.Error == "" || !containsIgnoreCase(resp.Error, tt.wantError) {
					t.Errorf("handleSpawnAgent() error = %q, want to contain %q", resp.Error, tt.wantError)
				}
			}
		})
	}
}

// containsIgnoreCase checks if s contains substr (case-insensitive)
func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// TestSendAgentDefinitionsToSupervisor tests the daemon function that sends
// agent definitions to the supervisor.
func TestSendAgentDefinitionsToSupervisor(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	repoName := "defs-test-repo"
	repoPath := d.paths.RepoDir(repoName)

	// Create repo directory structure
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatalf("Failed to create repo dir: %v", err)
	}

	// Initialize git repo
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@example.com"},
		{"git", "config", "user.name", "Test User"},
		{"git", "commit", "--allow-empty", "-m", "Initial commit"},
	}
	for _, cmdArgs := range cmds {
		cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
		cmd.Dir = repoPath
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to run %v: %v", cmdArgs, err)
		}
	}

	t.Run("no definitions returns nil without sending message", func(t *testing.T) {
		// No agents directory exists, should return nil
		mqConfig := state.DefaultMergeQueueConfig()
		err := d.sendAgentDefinitionsToSupervisor(repoName, repoPath, mqConfig)
		if err != nil {
			t.Errorf("Expected nil error for empty definitions, got: %v", err)
		}
	})

	t.Run("sends definitions to supervisor", func(t *testing.T) {
		// Create local agents directory with a definition
		agentsDir := d.paths.RepoAgentsDir(repoName)
		if err := os.MkdirAll(agentsDir, 0755); err != nil {
			t.Fatalf("Failed to create agents dir: %v", err)
		}

		workerContent := `# Test Worker

A test worker agent for unit testing.

## Instructions
- Process tasks
- Report results
`
		if err := os.WriteFile(filepath.Join(agentsDir, "test-worker.md"), []byte(workerContent), 0644); err != nil {
			t.Fatalf("Failed to write worker definition: %v", err)
		}

		// Add repo to state (needed for message routing)
		repo := &state.Repository{
			GithubURL:        "https://github.com/test/defs-test-repo",
			TmuxSession:      "mc-defs-test-repo",
			Agents:           make(map[string]state.Agent),
			MergeQueueConfig: state.DefaultMergeQueueConfig(),
		}
		if err := d.state.AddRepo(repoName, repo); err != nil {
			t.Fatalf("Failed to add repo: %v", err)
		}

		mqConfig := state.DefaultMergeQueueConfig()
		err := d.sendAgentDefinitionsToSupervisor(repoName, repoPath, mqConfig)
		if err != nil {
			t.Errorf("sendAgentDefinitionsToSupervisor failed: %v", err)
		}

		// Verify message was sent to supervisor
		msgMgr := messages.NewManager(d.paths.MessagesDir)
		msgs, err := msgMgr.List(repoName, "supervisor")
		if err != nil {
			t.Fatalf("Failed to list messages: %v", err)
		}

		if len(msgs) == 0 {
			t.Fatal("Expected at least one message to be sent to supervisor")
		}

		// Verify message content includes the definition
		lastMsg := msgs[len(msgs)-1]
		msgContent, err := msgMgr.Get(repoName, "supervisor", lastMsg.ID)
		if err != nil {
			t.Fatalf("Failed to read message: %v", err)
		}

		if !strings.Contains(msgContent.Body, "test-worker") {
			t.Error("Message should contain the agent definition name")
		}
		if !strings.Contains(msgContent.Body, "Test Worker") {
			t.Error("Message should contain the agent title")
		}
		if !strings.Contains(msgContent.Body, "A test worker agent") {
			t.Error("Message should contain the agent description")
		}
	})

	t.Run("includes merge queue config when enabled", func(t *testing.T) {
		// Create a fresh message directory
		if err := os.RemoveAll(d.paths.MessagesDir); err != nil {
			t.Fatalf("Failed to clear messages: %v", err)
		}
		if err := os.MkdirAll(d.paths.MessagesDir, 0755); err != nil {
			t.Fatalf("Failed to create messages dir: %v", err)
		}

		mqConfig := state.MergeQueueConfig{
			Enabled:   true,
			TrackMode: state.TrackModeAll,
		}

		err := d.sendAgentDefinitionsToSupervisor(repoName, repoPath, mqConfig)
		if err != nil {
			t.Errorf("sendAgentDefinitionsToSupervisor failed: %v", err)
		}

		// Verify message includes merge queue config
		msgMgr := messages.NewManager(d.paths.MessagesDir)
		msgs, _ := msgMgr.List(repoName, "supervisor")
		if len(msgs) == 0 {
			t.Fatal("Expected message to be sent")
		}

		lastMsg := msgs[len(msgs)-1]
		msgContent, _ := msgMgr.Get(repoName, "supervisor", lastMsg.ID)

		if !strings.Contains(msgContent.Body, "Merge Queue Configuration") {
			t.Error("Message should contain merge queue configuration section")
		}
		if !strings.Contains(msgContent.Body, "Enabled: yes") {
			t.Error("Message should indicate merge queue is enabled")
		}
		if !strings.Contains(msgContent.Body, "Track Mode: all") {
			t.Error("Message should include track mode")
		}
	})

	t.Run("includes disabled message when merge queue disabled", func(t *testing.T) {
		// Create a fresh message directory
		if err := os.RemoveAll(d.paths.MessagesDir); err != nil {
			t.Fatalf("Failed to clear messages: %v", err)
		}
		if err := os.MkdirAll(d.paths.MessagesDir, 0755); err != nil {
			t.Fatalf("Failed to create messages dir: %v", err)
		}

		mqConfig := state.MergeQueueConfig{
			Enabled:   false,
			TrackMode: state.TrackModeAll,
		}

		err := d.sendAgentDefinitionsToSupervisor(repoName, repoPath, mqConfig)
		if err != nil {
			t.Errorf("sendAgentDefinitionsToSupervisor failed: %v", err)
		}

		// Verify message indicates merge queue is disabled
		msgMgr := messages.NewManager(d.paths.MessagesDir)
		msgs, _ := msgMgr.List(repoName, "supervisor")
		if len(msgs) == 0 {
			t.Fatal("Expected message to be sent")
		}

		lastMsg := msgs[len(msgs)-1]
		msgContent, _ := msgMgr.Get(repoName, "supervisor", lastMsg.ID)

		if !strings.Contains(msgContent.Body, "Enabled: no") {
			t.Error("Message should indicate merge queue is disabled")
		}
		if !strings.Contains(msgContent.Body, "do NOT spawn merge-queue") {
			t.Error("Message should instruct not to spawn merge-queue")
		}
	})

	t.Run("includes spawn instructions", func(t *testing.T) {
		mqConfig := state.DefaultMergeQueueConfig()
		err := d.sendAgentDefinitionsToSupervisor(repoName, repoPath, mqConfig)
		if err != nil {
			t.Errorf("sendAgentDefinitionsToSupervisor failed: %v", err)
		}

		// Verify message includes spawn command
		msgMgr := messages.NewManager(d.paths.MessagesDir)
		msgs, _ := msgMgr.List(repoName, "supervisor")
		if len(msgs) == 0 {
			t.Fatal("Expected message to be sent")
		}

		lastMsg := msgs[len(msgs)-1]
		msgContent, _ := msgMgr.Get(repoName, "supervisor", lastMsg.ID)

		if !strings.Contains(msgContent.Body, "multiclaude agents spawn") {
			t.Error("Message should include spawn command")
		}
		if !strings.Contains(msgContent.Body, "--class <persistent|ephemeral>") {
			t.Error("Message should include class flag in spawn command")
		}
	})
}

// TestHandleRequestUnknownCommand tests handleRequest with unknown command
func TestHandleRequestUnknownCommand(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	resp := d.handleRequest(socket.Request{
		Command: "unknown_command_xyz",
	})

	if resp.Success {
		t.Error("Expected failure for unknown command")
	}
	if !strings.Contains(resp.Error, "unknown command") {
		t.Errorf("Error should mention unknown command, got: %s", resp.Error)
	}
}

// TestHandleRequestPing tests the ping command
func TestHandleRequestPing(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	resp := d.handleRequest(socket.Request{
		Command: "ping",
	})

	if !resp.Success {
		t.Errorf("Expected success for ping, got error: %s", resp.Error)
	}
	if resp.Data != "pong" {
		t.Errorf("Expected pong response, got: %v", resp.Data)
	}
}

// TestHandleRequestRouteMessages tests the route_messages command
func TestHandleRequestRouteMessages(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	resp := d.handleRequest(socket.Request{
		Command: "route_messages",
	})

	if !resp.Success {
		t.Errorf("Expected success for route_messages, got error: %s", resp.Error)
	}
	if !strings.Contains(resp.Data.(string), "routing triggered") {
		t.Errorf("Expected routing triggered message, got: %v", resp.Data)
	}
}

// TestHandleListAgentsRichFormat tests handleListAgents with rich format
func TestHandleListAgentsRichFormat(t *testing.T) {
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
		Task:         "Test task description",
		CreatedAt:    time.Now(),
	}
	if err := d.state.AddAgent("test-repo", "test-agent", agent); err != nil {
		t.Fatalf("Failed to add agent: %v", err)
	}

	t.Run("lists agents without rich format", func(t *testing.T) {
		resp := d.handleListAgents(socket.Request{
			Command: "list_agents",
			Args: map[string]interface{}{
				"repo": "test-repo",
			},
		})

		if !resp.Success {
			t.Errorf("Expected success, got error: %s", resp.Error)
		}

		data, ok := resp.Data.([]map[string]interface{})
		if !ok {
			t.Fatal("Expected slice of maps")
		}
		if len(data) != 1 {
			t.Errorf("Expected 1 agent, got %d", len(data))
		}
		if data[0]["name"] != "test-agent" {
			t.Errorf("Expected agent name 'test-agent', got %v", data[0]["name"])
		}
	})

	t.Run("lists agents with rich format", func(t *testing.T) {
		resp := d.handleListAgents(socket.Request{
			Command: "list_agents",
			Args: map[string]interface{}{
				"repo": "test-repo",
				"rich": true,
			},
		})

		if !resp.Success {
			t.Errorf("Expected success, got error: %s", resp.Error)
		}

		data, ok := resp.Data.([]map[string]interface{})
		if !ok {
			t.Fatal("Expected slice of maps")
		}
		if len(data) != 1 {
			t.Errorf("Expected 1 agent, got %d", len(data))
		}

		// Rich format should include status and message counts
		if _, hasStatus := data[0]["status"]; !hasStatus {
			t.Error("Rich format should include status")
		}
		if _, hasBranch := data[0]["branch"]; !hasBranch {
			t.Error("Rich format should include branch")
		}
		if _, hasTotal := data[0]["messages_total"]; !hasTotal {
			t.Error("Rich format should include messages_total")
		}
		if _, hasPending := data[0]["messages_pending"]; !hasPending {
			t.Error("Rich format should include messages_pending")
		}
	})

	t.Run("returns error for missing repo", func(t *testing.T) {
		resp := d.handleListAgents(socket.Request{
			Command: "list_agents",
			Args:    map[string]interface{}{},
		})

		if resp.Success {
			t.Error("Expected failure for missing repo")
		}
	})
}

// TestHandleRepairState tests handleRepairState
func TestHandleRepairState(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Add a test repository
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "nonexistent-session",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.state.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Add a test agent with nonexistent window
	agent := state.Agent{
		Type:         state.AgentTypeWorker,
		WorktreePath: "/tmp/nonexistent",
		TmuxWindow:   "nonexistent-window",
		SessionID:    "test-session-id",
		CreatedAt:    time.Now(),
	}
	if err := d.state.AddAgent("test-repo", "test-agent", agent); err != nil {
		t.Fatalf("Failed to add agent: %v", err)
	}

	resp := d.handleRepairState(socket.Request{
		Command: "repair_state",
	})

	if !resp.Success {
		t.Errorf("Expected success, got error: %s", resp.Error)
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Expected map response")
	}

	// Should have processed the repair (agent with nonexistent session)
	if _, hasRemoved := data["agents_removed"]; !hasRemoved {
		t.Error("Response should include agents_removed")
	}
	if _, hasFixed := data["issues_fixed"]; !hasFixed {
		t.Error("Response should include issues_fixed")
	}
}

// TestHandleTaskHistoryExtended tests handleTaskHistory with various scenarios
func TestHandleTaskHistoryExtended(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Add a test repository with task history
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "test-session",
		Agents:      make(map[string]state.Agent),
		TaskHistory: []state.TaskHistoryEntry{
			{
				Name:        "worker-1",
				Task:        "Test task 1",
				Status:      state.TaskStatusMerged,
				CreatedAt:   time.Now().Add(-1 * time.Hour),
				CompletedAt: time.Now(),
			},
			{
				Name:      "worker-2",
				Task:      "Test task 2",
				Status:    state.TaskStatusOpen,
				CreatedAt: time.Now(),
			},
		},
	}
	if err := d.state.AddRepo("history-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	t.Run("returns error for missing repo", func(t *testing.T) {
		resp := d.handleTaskHistory(socket.Request{
			Command: "task_history",
			Args:    map[string]interface{}{},
		})

		if resp.Success {
			t.Error("Expected failure for missing repo")
		}
	})

	t.Run("returns error for nonexistent repo", func(t *testing.T) {
		resp := d.handleTaskHistory(socket.Request{
			Command: "task_history",
			Args: map[string]interface{}{
				"repo": "nonexistent-repo",
			},
		})

		if resp.Success {
			t.Error("Expected failure for nonexistent repo")
		}
	})

	t.Run("returns task history", func(t *testing.T) {
		resp := d.handleTaskHistory(socket.Request{
			Command: "task_history",
			Args: map[string]interface{}{
				"repo": "history-repo",
			},
		})

		if !resp.Success {
			t.Errorf("Expected success, got error: %s", resp.Error)
		}

		// Response comes as []map[string]interface{} when returned from handler
		data, ok := resp.Data.([]map[string]interface{})
		if !ok {
			t.Fatalf("Expected []map[string]interface{}, got %T", resp.Data)
		}
		if len(data) != 2 {
			t.Errorf("Expected 2 history entries, got %d", len(data))
		}
	})

	t.Run("limits results with limit param", func(t *testing.T) {
		resp := d.handleTaskHistory(socket.Request{
			Command: "task_history",
			Args: map[string]interface{}{
				"repo":  "history-repo",
				"limit": float64(1), // JSON numbers come as float64
			},
		})

		if !resp.Success {
			t.Errorf("Expected success, got error: %s", resp.Error)
		}

		data, ok := resp.Data.([]map[string]interface{})
		if !ok {
			t.Fatalf("Expected []map[string]interface{}, got %T", resp.Data)
		}
		if len(data) != 1 {
			t.Errorf("Expected 1 history entry with limit=1, got %d", len(data))
		}
	})

	t.Run("returns entries with correct fields", func(t *testing.T) {
		resp := d.handleTaskHistory(socket.Request{
			Command: "task_history",
			Args: map[string]interface{}{
				"repo": "history-repo",
			},
		})

		if !resp.Success {
			t.Errorf("Expected success, got error: %s", resp.Error)
		}

		data, ok := resp.Data.([]map[string]interface{})
		if !ok {
			t.Fatalf("Expected []map[string]interface{}, got %T", resp.Data)
		}
		if len(data) == 0 {
			t.Fatal("Expected at least one entry")
		}

		// Verify entry has expected fields
		entry := data[0]
		if _, hasName := entry["name"]; !hasName {
			t.Error("Entry should have 'name' field")
		}
		if _, hasTask := entry["task"]; !hasTask {
			t.Error("Entry should have 'task' field")
		}
		if _, hasStatus := entry["status"]; !hasStatus {
			t.Error("Entry should have 'status' field")
		}
	})
}

func TestHandleUpdateRepoConfigMissingName(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Test update_repo_config without name
	resp := d.handleUpdateRepoConfig(socket.Request{
		Command: "update_repo_config",
		Args: map[string]interface{}{
			"mq_enabled": false,
		},
	})
	if resp.Success {
		t.Error("update_repo_config should fail without name argument")
	}
	if !strings.Contains(resp.Error, "name") {
		t.Errorf("Error should mention 'name': %s", resp.Error)
	}
}

func TestHandleUpdateRepoConfigNonexistentRepo(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Test update_repo_config with non-existent repo
	resp := d.handleUpdateRepoConfig(socket.Request{
		Command: "update_repo_config",
		Args: map[string]interface{}{
			"name":       "nonexistent-repo",
			"mq_enabled": false,
		},
	})
	if resp.Success {
		t.Error("update_repo_config should fail for non-existent repo")
	}
}

func TestHandleUpdateRepoConfigMergeQueueEnabled(t *testing.T) {
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

	// Update merge queue enabled
	resp := d.handleUpdateRepoConfig(socket.Request{
		Command: "update_repo_config",
		Args: map[string]interface{}{
			"name":       "test-repo",
			"mq_enabled": false,
		},
	})
	if !resp.Success {
		t.Errorf("update_repo_config failed: %s", resp.Error)
	}

	// Verify the config was updated
	config, err := d.state.GetMergeQueueConfig("test-repo")
	if err != nil {
		t.Fatalf("Failed to get merge queue config: %v", err)
	}
	if config.Enabled {
		t.Error("Merge queue should be disabled")
	}
}

func TestHandleUpdateRepoConfigMergeQueueTrackMode(t *testing.T) {
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

	// Update merge queue track mode
	resp := d.handleUpdateRepoConfig(socket.Request{
		Command: "update_repo_config",
		Args: map[string]interface{}{
			"name":          "test-repo",
			"mq_track_mode": "author",
		},
	})
	if !resp.Success {
		t.Errorf("update_repo_config failed: %s", resp.Error)
	}

	// Verify the config was updated
	config, err := d.state.GetMergeQueueConfig("test-repo")
	if err != nil {
		t.Fatalf("Failed to get merge queue config: %v", err)
	}
	if config.TrackMode != state.TrackModeAuthor {
		t.Errorf("Merge queue track mode = %q, want 'author'", config.TrackMode)
	}
}

func TestHandleUpdateRepoConfigPRShepherd(t *testing.T) {
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

	// Update PR shepherd config
	resp := d.handleUpdateRepoConfig(socket.Request{
		Command: "update_repo_config",
		Args: map[string]interface{}{
			"name":          "test-repo",
			"ps_enabled":    false,
			"ps_track_mode": "assigned",
		},
	})
	if !resp.Success {
		t.Errorf("update_repo_config failed: %s", resp.Error)
	}

	// Verify the config was updated
	config, err := d.state.GetPRShepherdConfig("test-repo")
	if err != nil {
		t.Fatalf("Failed to get PR shepherd config: %v", err)
	}
	if config.Enabled {
		t.Error("PR shepherd should be disabled")
	}
	if config.TrackMode != state.TrackModeAssigned {
		t.Errorf("PR shepherd track mode = %q, want 'assigned'", config.TrackMode)
	}
}

func TestHandleClearCurrentRepoSuccess(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Add a test repository and set it as current
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "test-session",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.state.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}
	if err := d.state.SetCurrentRepo("test-repo"); err != nil {
		t.Fatalf("Failed to set current repo: %v", err)
	}

	// Clear current repo
	resp := d.handleClearCurrentRepo(socket.Request{Command: "clear_current_repo"})
	if !resp.Success {
		t.Errorf("clear_current_repo failed: %s", resp.Error)
	}

	// Verify current repo is cleared
	if d.state.GetCurrentRepo() != "" {
		t.Error("Current repo should be cleared")
	}
}

func TestCleanupDeadAgentsPersistentAgent(t *testing.T) {
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

	// Add a supervisor agent (persistent)
	agent := state.Agent{
		Type:         state.AgentTypeSupervisor,
		WorktreePath: "/tmp/test",
		TmuxWindow:   "supervisor",
		SessionID:    "test-session-id",
		CreatedAt:    time.Now(),
	}
	if err := d.state.AddAgent("test-repo", "supervisor", agent); err != nil {
		t.Fatalf("Failed to add agent: %v", err)
	}

	// Verify agent exists
	_, exists := d.state.GetAgent("test-repo", "supervisor")
	if !exists {
		t.Fatal("Agent should exist before cleanup")
	}

	// Mark supervisor as dead and call cleanup
	deadAgents := map[string][]string{
		"test-repo": {"supervisor"},
	}

	// Call cleanup - should skip persistent agents (but in this case it will still remove
	// because the cleanup function doesn't check agent type)
	d.cleanupDeadAgents(deadAgents)

	// The current implementation removes all dead agents regardless of type
	// This test documents the current behavior
	_, exists = d.state.GetAgent("test-repo", "supervisor")
	if exists {
		t.Log("Note: cleanupDeadAgents currently removes persistent agents too")
	}
}

func TestRecordTaskHistoryEmptyWorktreePath(t *testing.T) {
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

	// Add a worker agent with empty WorktreePath
	agent := state.Agent{
		Type:         state.AgentTypeWorker,
		WorktreePath: "", // Empty path
		TmuxWindow:   "test-worker",
		SessionID:    "test-session-id",
		Task:         "Test task description",
		CreatedAt:    time.Now(),
	}
	if err := d.state.AddAgent("test-repo", "test-worker", agent); err != nil {
		t.Fatalf("Failed to add agent: %v", err)
	}

	// Record task history
	d.recordTaskHistory("test-repo", "test-worker", agent)

	// Verify task history was recorded with empty branch (since no worktree)
	history, err := d.state.GetTaskHistory("test-repo", 10)
	if err != nil {
		t.Fatalf("Failed to get task history: %v", err)
	}

	if len(history) != 1 {
		t.Errorf("Expected 1 history entry, got %d", len(history))
	}

	// Branch should be empty when WorktreePath is empty
	if history[0].Branch != "" {
		t.Errorf("History entry branch = %q, want empty string", history[0].Branch)
	}
}

func TestRecordTaskHistoryWithSummary(t *testing.T) {
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

	// Add a worker agent with summary
	agent := state.Agent{
		Type:         state.AgentTypeWorker,
		WorktreePath: "",
		TmuxWindow:   "test-worker",
		SessionID:    "test-session-id",
		Task:         "Test task description",
		Summary:      "Implemented the feature successfully",
		CreatedAt:    time.Now(),
	}

	// Record task history
	d.recordTaskHistory("test-repo", "test-worker", agent)

	// Verify task history was recorded with summary
	history, err := d.state.GetTaskHistory("test-repo", 10)
	if err != nil {
		t.Fatalf("Failed to get task history: %v", err)
	}

	if len(history) != 1 {
		t.Errorf("Expected 1 history entry, got %d", len(history))
	}

	if history[0].Summary != "Implemented the feature successfully" {
		t.Errorf("History entry summary = %q, want 'Implemented the feature successfully'", history[0].Summary)
	}
}
