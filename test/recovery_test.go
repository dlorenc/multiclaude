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
	"github.com/dlorenc/multiclaude/internal/prompts"
	"github.com/dlorenc/multiclaude/internal/state"
	"github.com/dlorenc/multiclaude/internal/worktree"
	"github.com/dlorenc/multiclaude/pkg/config"
	"github.com/dlorenc/multiclaude/pkg/tmux"
)

// TestCorruptedStateFileRecovery tests that the system can recover from a corrupted state file
func TestCorruptedStateFileRecovery(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	// Test 1: Completely corrupted file
	t.Run("CompletelyCorrupted", func(t *testing.T) {
		if err := os.WriteFile(statePath, []byte("this is not json at all!@#$%"), 0644); err != nil {
			t.Fatalf("Failed to write corrupted file: %v", err)
		}

		// Load should fail
		_, err := state.Load(statePath)
		if err == nil {
			t.Error("Load() should fail for corrupted JSON")
		}

		// But we can create a new state and continue
		s := state.New(statePath)
		if s == nil {
			t.Fatal("New() should work even with corrupted file on disk")
		}
		if err := s.Save(); err != nil {
			t.Fatalf("Save() should overwrite corrupted file: %v", err)
		}

		// Now load should work
		loaded, err := state.Load(statePath)
		if err != nil {
			t.Errorf("Load() should work after Save(): %v", err)
		}
		if loaded == nil {
			t.Error("Loaded state should not be nil")
		}
	})

	// Test 2: Truncated JSON file
	t.Run("TruncatedJSON", func(t *testing.T) {
		if err := os.WriteFile(statePath, []byte(`{"repos": {`), 0644); err != nil {
			t.Fatalf("Failed to write truncated file: %v", err)
		}

		_, err := state.Load(statePath)
		if err == nil {
			t.Error("Load() should fail for truncated JSON")
		}
	})

	// Test 3: Empty file
	t.Run("EmptyFile", func(t *testing.T) {
		if err := os.WriteFile(statePath, []byte(""), 0644); err != nil {
			t.Fatalf("Failed to write empty file: %v", err)
		}

		_, err := state.Load(statePath)
		if err == nil {
			t.Error("Load() should fail for empty file")
		}
	})

	// Test 4: Valid JSON but wrong structure
	t.Run("WrongStructure", func(t *testing.T) {
		if err := os.WriteFile(statePath, []byte(`{"wrong": "structure"}`), 0644); err != nil {
			t.Fatalf("Failed to write wrong structure: %v", err)
		}

		// Should load but repos will be empty
		loaded, err := state.Load(statePath)
		if err != nil {
			t.Fatalf("Load() should handle wrong structure gracefully: %v", err)
		}
		// Repos should be initialized even if the JSON didn't have them
		if loaded == nil {
			t.Fatal("Loaded state should not be nil")
		}
	})
}

// TestOrphanedTmuxSessionCleanup tests that orphaned tmux sessions are detected
func TestOrphanedTmuxSessionCleanup(t *testing.T) {
	tmuxClient := tmux.NewClient()
	if !tmuxClient.IsTmuxAvailable() {
		t.Fatal("tmux is required for this test but not available")
	}

	tmpDir := t.TempDir()
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

	// Create an "orphaned" tmux session (not tracked in state)
	orphanSession := "mc-orphan-test"
	if err := tmuxClient.CreateSession(context.Background(), orphanSession, true); err != nil {
		t.Fatalf("Failed to create orphan session: %v", err)
	}
	defer tmuxClient.KillSession(context.Background(), orphanSession)

	// Verify session exists
	exists, err := tmuxClient.HasSession(context.Background(), orphanSession)
	if err != nil {
		t.Fatalf("Failed to check session: %v", err)
	}
	if !exists {
		t.Fatal("Orphan session should exist")
	}

	// List sessions - should see our orphan
	sessions, err := tmuxClient.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("Failed to list sessions: %v", err)
	}

	found := false
	for _, s := range sessions {
		if s == orphanSession {
			found = true
			break
		}
	}
	if !found {
		t.Error("Orphan session should be in session list")
	}

	// Create daemon and state (without the orphan)
	d, err := daemon.New(paths)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	// Add a legitimate repo
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-legitimate",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.GetState().AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// The health check should identify the orphan session
	// (Note: current implementation logs but doesn't auto-kill orphans)
	d.TriggerHealthCheck()

	// Verify legitimate repo still in state
	_, exists = d.GetState().GetRepo("test-repo")
	if !exists {
		t.Error("Legitimate repo should still exist")
	}
}

// TestOrphanedWorktreeCleanup tests that orphaned worktrees are cleaned up
func TestOrphanedWorktreeCleanup(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a git repo
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
	wtRoot := filepath.Join(tmpDir, "wts")
	if err := os.MkdirAll(wtRoot, 0755); err != nil {
		t.Fatalf("Failed to create worktrees dir: %v", err)
	}

	// Create a legitimate worktree
	legitimateWT := filepath.Join(wtRoot, "legitimate")
	if err := wt.CreateNewBranch(legitimateWT, "legitimate-branch", "HEAD"); err != nil {
		t.Fatalf("Failed to create legitimate worktree: %v", err)
	}

	// Create an "orphaned" directory (not a git worktree)
	orphanDir := filepath.Join(wtRoot, "orphan")
	if err := os.MkdirAll(orphanDir, 0755); err != nil {
		t.Fatalf("Failed to create orphan dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(orphanDir, "test.txt"), []byte("orphan"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Run cleanup
	removed, err := worktree.CleanupOrphaned(wtRoot, wt)
	if err != nil {
		t.Fatalf("CleanupOrphaned failed: %v", err)
	}

	// Orphan should be removed
	if len(removed) != 1 {
		t.Errorf("Expected 1 orphan removed, got %d", len(removed))
	}

	// Verify orphan is gone
	if _, err := os.Stat(orphanDir); !os.IsNotExist(err) {
		t.Error("Orphan directory should be removed")
	}

	// Verify legitimate worktree still exists
	if _, err := os.Stat(legitimateWT); os.IsNotExist(err) {
		t.Error("Legitimate worktree should still exist")
	}
}

// TestStaleSocketCleanup tests that stale socket files are cleaned up
func TestStaleSocketCleanup(t *testing.T) {
	tmpDir := t.TempDir()
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

	// Create a stale socket file (no daemon running)
	if err := os.WriteFile(paths.DaemonSock, []byte("stale"), 0644); err != nil {
		t.Fatalf("Failed to create stale socket: %v", err)
	}

	// Create a stale PID file with non-existent PID
	if err := os.WriteFile(paths.DaemonPID, []byte("999999999"), 0644); err != nil {
		t.Fatalf("Failed to create stale PID: %v", err)
	}

	// Try to start a new daemon - it should handle the stale files
	d, err := daemon.New(paths)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	// Start should work even with stale files
	if err := d.Start(); err != nil {
		t.Fatalf("Failed to start daemon (should handle stale files): %v", err)
	}
	defer d.Stop()

	// Give it a moment
	time.Sleep(50 * time.Millisecond)

	// Verify we can communicate with the daemon
	pidFile := daemon.NewPIDFile(paths.DaemonPID)
	running, _, _ := pidFile.IsRunning()
	if !running {
		t.Error("Daemon should be running")
	}
}

// TestOrphanedMessageDirectoryCleanup tests cleanup of orphaned message directories
func TestOrphanedMessageDirectoryCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	msgDir := filepath.Join(tmpDir, "messages")
	if err := os.MkdirAll(msgDir, 0755); err != nil {
		t.Fatalf("Failed to create messages dir: %v", err)
	}

	repoName := "test-repo"
	msgMgr := messages.NewManager(msgDir)

	// Create message directories for some agents
	for _, agent := range []string{"supervisor", "worker1", "orphan1", "orphan2"} {
		agentDir := filepath.Join(msgDir, repoName, agent)
		if err := os.MkdirAll(agentDir, 0755); err != nil {
			t.Fatalf("Failed to create agent dir: %v", err)
		}
		// Add a dummy message file
		msgFile := filepath.Join(agentDir, "msg-test.json")
		if err := os.WriteFile(msgFile, []byte(`{"id":"test"}`), 0644); err != nil {
			t.Fatalf("Failed to write message: %v", err)
		}
	}

	// Define valid agents (orphan1 and orphan2 are not in this list)
	validAgents := []string{"supervisor", "worker1"}

	// Run cleanup
	count, err := msgMgr.CleanupOrphaned(repoName, validAgents)
	if err != nil {
		t.Fatalf("CleanupOrphaned failed: %v", err)
	}

	// Should have removed 2 orphaned directories
	if count != 2 {
		t.Errorf("Expected 2 orphaned dirs removed, got %d", count)
	}

	// Verify orphans are gone
	for _, orphan := range []string{"orphan1", "orphan2"} {
		orphanDir := filepath.Join(msgDir, repoName, orphan)
		if _, err := os.Stat(orphanDir); !os.IsNotExist(err) {
			t.Errorf("Orphan directory %s should be removed", orphan)
		}
	}

	// Verify valid agents still exist
	for _, valid := range validAgents {
		validDir := filepath.Join(msgDir, repoName, valid)
		if _, err := os.Stat(validDir); os.IsNotExist(err) {
			t.Errorf("Valid agent directory %s should still exist", valid)
		}
	}
}

// TestDaemonCrashRecovery tests that the system can recover from a daemon crash
func TestDaemonCrashRecovery(t *testing.T) {
	// This test requires tmux because the daemon's restoreTrackedRepos() checks
	// for tmux session existence. If the session doesn't exist, it tries to restore
	// agents which would clear/recreate them. We need the session to exist so
	// state is preserved as-is.
	tmuxClient := tmux.NewClient()
	if !tmuxClient.IsTmuxAvailable() {
		t.Fatal("tmux is required for this test but not available")
	}

	tmpDir := t.TempDir()
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

	// Create a unique session name for this test to avoid conflicts
	sessionName := "mc-recovery-test"

	// Create the tmux session BEFORE starting the daemon
	// This is critical: when d2 starts, restoreTrackedRepos() will see the session
	// exists and skip restoration, preserving the state as-is.
	if err := tmuxClient.CreateSession(context.Background(), sessionName, true); err != nil {
		t.Fatalf("Failed to create tmux session: %v", err)
	}
	defer tmuxClient.KillSession(context.Background(), sessionName)

	// Create supervisor window for the agent we'll add
	if err := tmuxClient.CreateWindow(context.Background(), sessionName, "supervisor"); err != nil {
		t.Fatalf("Failed to create supervisor window: %v", err)
	}

	// Start daemon, add state, then simulate crash
	d1, err := daemon.New(paths)
	if err != nil {
		t.Fatalf("Failed to create first daemon: %v", err)
	}

	if err := d1.Start(); err != nil {
		t.Fatalf("Failed to start first daemon: %v", err)
	}

	// Add some state (using the session we created)
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: sessionName,
		Agents:      make(map[string]state.Agent),
	}
	if err := d1.GetState().AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	agent := state.Agent{
		Type:       state.AgentTypeSupervisor,
		TmuxWindow: "supervisor",
		CreatedAt:  time.Now(),
	}
	if err := d1.GetState().AddAgent("test-repo", "supervisor", agent); err != nil {
		t.Fatalf("Failed to add agent: %v", err)
	}

	// Stop cleanly (this saves state)
	if err := d1.Stop(); err != nil {
		t.Fatalf("Failed to stop first daemon: %v", err)
	}

	// Simulate crash: Leave PID file but remove socket
	// (In real crash, the PID file would still exist but socket would be stale)
	os.Remove(paths.DaemonSock)

	// Wait for socket file to be fully removed
	time.Sleep(100 * time.Millisecond)

	// Start new daemon - should recover state from disk
	// Because the tmux session exists, restoreTrackedRepos() will skip restoration
	// and the state will be preserved.
	d2, err := daemon.New(paths)
	if err != nil {
		t.Fatalf("Failed to create second daemon: %v", err)
	}

	if err := d2.Start(); err != nil {
		t.Fatalf("Failed to start second daemon: %v", err)
	}
	defer d2.Stop()

	// Wait for daemon to fully initialize
	time.Sleep(100 * time.Millisecond)

	// Verify state was recovered
	recovered, exists := d2.GetState().GetRepo("test-repo")
	if !exists {
		t.Error("Repo should be recovered from state file")
	}
	if recovered.GithubURL != "https://github.com/test/repo" {
		t.Error("Repo data should be preserved")
	}

	_, exists = d2.GetState().GetAgent("test-repo", "supervisor")
	if !exists {
		t.Error("Agent should be recovered from state file")
	}
}

// TestStateAtomicWrite tests that state writes are atomic
func TestStateAtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := state.New(statePath)

	// Add some data
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test",
		Agents:      make(map[string]state.Agent),
	}
	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Save state
	if err := s.Save(); err != nil {
		t.Fatalf("Failed to save state: %v", err)
	}

	// Verify temp file doesn't exist after save (was renamed to final file)
	tempPath := statePath + ".tmp"
	if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
		t.Error("Temp file should not exist after successful save")
	}

	// Verify final file exists and is valid
	loaded, err := state.Load(statePath)
	if err != nil {
		t.Fatalf("Failed to load saved state: %v", err)
	}

	loadedRepo, exists := loaded.GetRepo("test-repo")
	if !exists {
		t.Error("Repo should exist in loaded state")
	}
	if loadedRepo.GithubURL != repo.GithubURL {
		t.Error("Repo data should match")
	}
}

// TestConcurrentStateAccess tests that concurrent state access is safe
func TestConcurrentStateAccess(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	s := state.New(statePath)

	// Add initial repo
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test",
		Agents:      make(map[string]state.Agent),
	}
	if err := s.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Run concurrent operations
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			defer func() { done <- true }()

			// Concurrent reads
			for j := 0; j < 100; j++ {
				s.GetRepo("test-repo")
				s.ListRepos()
				s.GetAllRepos()
			}
		}(i)

		go func(id int) {
			defer func() { done <- true }()

			// Concurrent writes
			agentName := time.Now().UnixNano()
			agent := state.Agent{
				Type:       state.AgentTypeWorker,
				TmuxWindow: "worker",
				CreatedAt:  time.Now(),
			}
			s.AddAgent("test-repo", string(rune(agentName)), agent)
			s.RemoveAgent("test-repo", string(rune(agentName)))
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}

	// State should still be valid
	_, exists := s.GetRepo("test-repo")
	if !exists {
		t.Error("Repo should still exist after concurrent access")
	}
}

// TestSlashCommandsEmbeddedInPrompts verifies that slash commands are embedded
// in agent prompts rather than via CLAUDE_CONFIG_DIR (which doesn't work for auth)
func TestSlashCommandsEmbeddedInPrompts(t *testing.T) {
	// Get the slash commands prompt
	slashPrompt := prompts.GetSlashCommandsPrompt()

	// Verify all expected commands are in the prompt
	expectedCommands := []string{"/status", "/refresh", "/workers", "/messages"}
	for _, cmd := range expectedCommands {
		if !strings.Contains(slashPrompt, cmd) {
			t.Errorf("Expected slash commands prompt to contain %q", cmd)
		}
	}

	// Verify multiclaude CLI commands are referenced
	expectedCLICommands := []string{"multiclaude list", "multiclaude work list", "multiclaude agent list-messages"}
	for _, cmd := range expectedCLICommands {
		if !strings.Contains(slashPrompt, cmd) {
			t.Errorf("Expected slash commands prompt to contain CLI command %q", cmd)
		}
	}
}
