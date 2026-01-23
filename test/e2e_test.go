package test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dlorenc/multiclaude/internal/daemon"
	"github.com/dlorenc/multiclaude/internal/messages"
	"github.com/dlorenc/multiclaude/internal/socket"
	"github.com/dlorenc/multiclaude/internal/state"
	"github.com/dlorenc/multiclaude/internal/worktree"
	"github.com/dlorenc/multiclaude/pkg/config"
	"github.com/dlorenc/multiclaude/pkg/tmux"
)

// TestPhase2Integration tests the core Phase 2 functionality end-to-end
func TestPhase2Integration(t *testing.T) {
	// Set test mode to skip Claude startup
	os.Setenv("MULTICLAUDE_TEST_MODE", "1")
	defer os.Unsetenv("MULTICLAUDE_TEST_MODE")

	tmuxClient := tmux.NewClient()
	if !tmuxClient.IsTmuxAvailable() {
		t.Fatal("tmux is required for this test but not available")
	}

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "e2e-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

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
	}

	if err := paths.EnsureDirectories(); err != nil {
		t.Fatalf("Failed to create directories: %v", err)
	}

	// Initialize a test git repo
	repoName := "test-repo"
	repoPath := paths.RepoDir(repoName)
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

	// Create daemon
	d, err := daemon.New(paths)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	// Start daemon
	if err := d.Start(); err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}
	defer d.Stop()

	// Give daemon time to start
	time.Sleep(100 * time.Millisecond)

	// Create socket client
	client := socket.NewClient(paths.DaemonSock)

	// Test: Ping daemon
	t.Run("PingDaemon", func(t *testing.T) {
		resp, err := client.Send(socket.Request{Command: "ping"})
		if err != nil {
			t.Fatalf("Failed to ping daemon: %v", err)
		}
		if !resp.Success {
			t.Error("Ping should succeed")
		}
	})

	// Test: Add repository
	tmuxSession := "mc-test-repo"
	// Create tmux session first (will be cleaned up at end of test)
	if err := tmuxClient.CreateSession(context.Background(), tmuxSession, true); err != nil {
		t.Fatalf("Failed to create tmux session: %v", err)
	}
	defer tmuxClient.KillSession(context.Background(), tmuxSession)

	t.Run("AddRepository", func(t *testing.T) {
		// Create windows for supervisor and merge-queue
		if err := tmuxClient.CreateWindow(context.Background(), tmuxSession, "supervisor"); err != nil {
			t.Fatalf("Failed to create supervisor window: %v", err)
		}
		if err := tmuxClient.CreateWindow(context.Background(), tmuxSession, "merge-queue"); err != nil {
			t.Fatalf("Failed to create merge-queue window: %v", err)
		}

		// Add repo to daemon
		resp, err := client.Send(socket.Request{
			Command: "add_repo",
			Args: map[string]interface{}{
				"name":         repoName,
				"github_url":   "https://github.com/test/repo",
				"tmux_session": tmuxSession,
			},
		})
		if err != nil {
			t.Fatalf("Failed to add repo: %v", err)
		}
		if !resp.Success {
			t.Errorf("Add repo failed: %s", resp.Error)
		}
	})

	// Test: Add supervisor agent
	t.Run("AddSupervisorAgent", func(t *testing.T) {
		resp, err := client.Send(socket.Request{
			Command: "add_agent",
			Args: map[string]interface{}{
				"repo":          repoName,
				"agent":         "supervisor",
				"type":          "supervisor",
				"worktree_path": repoPath,
				"tmux_window":   "supervisor",
			},
		})
		if err != nil {
			t.Fatalf("Failed to add supervisor: %v", err)
		}
		if !resp.Success {
			t.Errorf("Add supervisor failed: %s", resp.Error)
		}
	})

	// Test: Create worker
	var workerName string
	var workerPath string
	t.Run("CreateWorker", func(t *testing.T) {
		workerName = "test-worker"
		workerPath = paths.AgentWorktree(repoName, workerName)

		// Create worktree
		wt := worktree.NewManager(repoPath)
		if err := wt.CreateNewBranch(workerPath, "work/test-worker", "HEAD"); err != nil {
			t.Fatalf("Failed to create worktree: %v", err)
		}

		// Create tmux window
		if err := tmuxClient.CreateWindow(context.Background(), tmuxSession, workerName); err != nil {
			t.Fatalf("Failed to create worker window: %v", err)
		}

		// Add worker to daemon
		resp, err := client.Send(socket.Request{
			Command: "add_agent",
			Args: map[string]interface{}{
				"repo":          repoName,
				"agent":         workerName,
				"type":          "worker",
				"worktree_path": workerPath,
				"tmux_window":   workerName,
				"task":          "Test task",
			},
		})
		if err != nil {
			t.Fatalf("Failed to add worker: %v", err)
		}
		if !resp.Success {
			t.Errorf("Add worker failed: %s", resp.Error)
		}
	})

	// Test: List agents
	t.Run("ListAgents", func(t *testing.T) {
		resp, err := client.Send(socket.Request{
			Command: "list_agents",
			Args: map[string]interface{}{
				"repo": repoName,
			},
		})
		if err != nil {
			t.Fatalf("Failed to list agents: %v", err)
		}
		if !resp.Success {
			t.Errorf("List agents failed: %s", resp.Error)
		}

		agents, ok := resp.Data.([]interface{})
		if !ok {
			t.Fatal("Expected agents array")
		}

		if len(agents) != 2 {
			t.Errorf("Expected 2 agents, got %d", len(agents))
		}
	})

	// Test: Message routing
	t.Run("MessageRouting", func(t *testing.T) {
		msgMgr := messages.NewManager(paths.MessagesDir)

		// Send message from supervisor to worker
		msg, err := msgMgr.Send(repoName, "supervisor", workerName, "Test message from supervisor")
		if err != nil {
			t.Fatalf("Failed to send message: %v", err)
		}

		if msg.Status != messages.StatusPending {
			t.Errorf("Message status = %s, want pending", msg.Status)
		}

		// Trigger message routing (via health check since we don't have a separate trigger)
		// In real usage, the message router loop would handle this
		time.Sleep(100 * time.Millisecond)

		// List messages for worker
		msgs, err := msgMgr.List(repoName, workerName)
		if err != nil {
			t.Fatalf("Failed to list messages: %v", err)
		}

		if len(msgs) != 1 {
			t.Errorf("Expected 1 message, got %d", len(msgs))
		}
	})

	// Test: Complete agent
	t.Run("CompleteAgent", func(t *testing.T) {
		resp, err := client.Send(socket.Request{
			Command: "complete_agent",
			Args: map[string]interface{}{
				"repo":  repoName,
				"agent": workerName,
			},
		})
		if err != nil {
			t.Fatalf("Failed to complete agent: %v", err)
		}
		if !resp.Success {
			t.Errorf("Complete agent failed: %s", resp.Error)
		}

		// Verify agent is marked for cleanup
		st, _ := state.Load(paths.StateFile)
		agent, exists := st.GetAgent(repoName, workerName)
		if !exists {
			t.Fatal("Worker should still exist after completion")
		}
		if !agent.ReadyForCleanup {
			t.Error("Worker should be marked for cleanup")
		}
	})

	// Test: Cleanup - remove the worker
	t.Run("Cleanup", func(t *testing.T) {
		resp, err := client.Send(socket.Request{
			Command: "remove_agent",
			Args: map[string]interface{}{
				"repo":  repoName,
				"agent": workerName,
			},
		})
		if err != nil {
			t.Fatalf("Failed to remove agent: %v", err)
		}
		if !resp.Success {
			t.Errorf("Remove agent failed: %s", resp.Error)
		}

		// Verify agent is removed
		st, _ := state.Load(paths.StateFile)
		_, exists := st.GetAgent(repoName, workerName)
		if exists {
			t.Error("Worker should be removed after cleanup")
		}
	})

	// Test: Worktree cleanup
	t.Run("WorktreeCleanup", func(t *testing.T) {
		// Worktree should still exist (daemon doesn't remove it in remove_agent handler)
		// In real usage, the cleanup loop would remove it
		_, err := os.Stat(workerPath)
		if err == nil {
			t.Log("Worktree still exists (expected, would be cleaned by cleanup loop)")
		}
	})
}

// TestWorktreeOperations tests git worktree operations
func TestWorktreeOperations(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "worktree-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize git repo
	repoPath := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatalf("Failed to create repo dir: %v", err)
	}

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

	wt := worktree.NewManager(repoPath)

	// Test: Create worktree
	wtPath := filepath.Join(tmpDir, "wts", "test-worker")
	t.Run("CreateWorktree", func(t *testing.T) {
		if err := wt.CreateNewBranch(wtPath, "test-branch", "HEAD"); err != nil {
			t.Fatalf("Failed to create worktree: %v", err)
		}

		// Verify it exists
		exists, err := wt.Exists(wtPath)
		if err != nil {
			t.Fatalf("Failed to check worktree existence: %v", err)
		}
		if !exists {
			t.Error("Worktree should exist")
		}
	})

	// Test: List worktrees
	t.Run("ListWorktrees", func(t *testing.T) {
		worktrees, err := wt.List()
		if err != nil {
			t.Fatalf("Failed to list worktrees: %v", err)
		}

		// Should have 2: main repo + our test worktree
		if len(worktrees) < 2 {
			t.Errorf("Expected at least 2 worktrees, got %d", len(worktrees))
		}

		// Find our worktree
		found := false
		for _, w := range worktrees {
			if strings.Contains(w.Path, "test-worker") {
				found = true
				if w.Branch != "test-branch" {
					t.Errorf("Branch = %s, want test-branch", w.Branch)
				}
			}
		}
		if !found {
			t.Error("Test worktree not found in list")
		}
	})

	// Test: Remove worktree
	t.Run("RemoveWorktree", func(t *testing.T) {
		if err := wt.Remove(wtPath, false); err != nil {
			t.Fatalf("Failed to remove worktree: %v", err)
		}

		// Verify it's gone
		exists, err := wt.Exists(wtPath)
		if err != nil {
			t.Fatalf("Failed to check worktree existence: %v", err)
		}
		if exists {
			t.Error("Worktree should not exist after removal")
		}
	})
}
