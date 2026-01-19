package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultPaths(t *testing.T) {
	paths, err := DefaultPaths()
	if err != nil {
		t.Fatalf("DefaultPaths() failed: %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir() failed: %v", err)
	}

	expected := filepath.Join(home, ".multiclaude")
	if paths.Root != expected {
		t.Errorf("Root = %q, want %q", paths.Root, expected)
	}

	// Test that all paths are under the root
	if !strings.HasPrefix(paths.DaemonPID, paths.Root) {
		t.Errorf("DaemonPID not under Root: %s", paths.DaemonPID)
	}
	if !strings.HasPrefix(paths.DaemonSock, paths.Root) {
		t.Errorf("DaemonSock not under Root: %s", paths.DaemonSock)
	}
	if !strings.HasPrefix(paths.DaemonLog, paths.Root) {
		t.Errorf("DaemonLog not under Root: %s", paths.DaemonLog)
	}
	if !strings.HasPrefix(paths.StateFile, paths.Root) {
		t.Errorf("StateFile not under Root: %s", paths.StateFile)
	}
	if !strings.HasPrefix(paths.ReposDir, paths.Root) {
		t.Errorf("ReposDir not under Root: %s", paths.ReposDir)
	}
	if !strings.HasPrefix(paths.WorktreesDir, paths.Root) {
		t.Errorf("WorktreesDir not under Root: %s", paths.WorktreesDir)
	}
	if !strings.HasPrefix(paths.MessagesDir, paths.Root) {
		t.Errorf("MessagesDir not under Root: %s", paths.MessagesDir)
	}
}

func TestEnsureDirectories(t *testing.T) {
	tmpDir := t.TempDir()

	paths := &Paths{
		Root:         filepath.Join(tmpDir, "test-multiclaude"),
		ReposDir:     filepath.Join(tmpDir, "test-multiclaude", "repos"),
		WorktreesDir: filepath.Join(tmpDir, "test-multiclaude", "wts"),
		MessagesDir:  filepath.Join(tmpDir, "test-multiclaude", "messages"),
		OutputDir:    filepath.Join(tmpDir, "test-multiclaude", "output"),
	}

	if err := paths.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories() failed: %v", err)
	}

	// Verify directories were created
	dirs := []string{paths.Root, paths.ReposDir, paths.WorktreesDir, paths.MessagesDir, paths.OutputDir}
	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Errorf("Directory not created: %s", dir)
		}
	}

	// Test idempotency - should not fail if called again
	if err := paths.EnsureDirectories(); err != nil {
		t.Errorf("EnsureDirectories() second call failed: %v", err)
	}
}

func TestRepoPaths(t *testing.T) {
	tmpDir := t.TempDir()

	paths := &Paths{
		Root:         tmpDir,
		ReposDir:     filepath.Join(tmpDir, "repos"),
		WorktreesDir: filepath.Join(tmpDir, "wts"),
		MessagesDir:  filepath.Join(tmpDir, "messages"),
	}

	repoName := "test-repo"

	repoDir := paths.RepoDir(repoName)
	expected := filepath.Join(tmpDir, "repos", repoName)
	if repoDir != expected {
		t.Errorf("RepoDir() = %q, want %q", repoDir, expected)
	}

	wtDir := paths.WorktreeDir(repoName)
	expected = filepath.Join(tmpDir, "wts", repoName)
	if wtDir != expected {
		t.Errorf("WorktreeDir() = %q, want %q", wtDir, expected)
	}

	agentName := "supervisor"
	agentWT := paths.AgentWorktree(repoName, agentName)
	expected = filepath.Join(tmpDir, "wts", repoName, agentName)
	if agentWT != expected {
		t.Errorf("AgentWorktree() = %q, want %q", agentWT, expected)
	}

	repoMsgDir := paths.RepoMessagesDir(repoName)
	expected = filepath.Join(tmpDir, "messages", repoName)
	if repoMsgDir != expected {
		t.Errorf("RepoMessagesDir() = %q, want %q", repoMsgDir, expected)
	}

	agentMsgDir := paths.AgentMessagesDir(repoName, agentName)
	expected = filepath.Join(tmpDir, "messages", repoName, agentName)
	if agentMsgDir != expected {
		t.Errorf("AgentMessagesDir() = %q, want %q", agentMsgDir, expected)
	}
}

func TestOutputPaths(t *testing.T) {
	tmpDir := t.TempDir()

	paths := &Paths{
		Root:         tmpDir,
		OutputDir:    filepath.Join(tmpDir, "output"),
	}

	repoName := "test-repo"

	// Test RepoOutputDir
	repoOutputDir := paths.RepoOutputDir(repoName)
	expected := filepath.Join(tmpDir, "output", repoName)
	if repoOutputDir != expected {
		t.Errorf("RepoOutputDir() = %q, want %q", repoOutputDir, expected)
	}

	// Test WorkersOutputDir
	workersDir := paths.WorkersOutputDir(repoName)
	expected = filepath.Join(tmpDir, "output", repoName, "workers")
	if workersDir != expected {
		t.Errorf("WorkersOutputDir() = %q, want %q", workersDir, expected)
	}

	// Test AgentLogFile for system agent (not worker)
	supervisorLog := paths.AgentLogFile(repoName, "supervisor", false)
	expected = filepath.Join(tmpDir, "output", repoName, "supervisor.log")
	if supervisorLog != expected {
		t.Errorf("AgentLogFile(supervisor, false) = %q, want %q", supervisorLog, expected)
	}

	// Test AgentLogFile for worker
	workerLog := paths.AgentLogFile(repoName, "happy-eagle", true)
	expected = filepath.Join(tmpDir, "output", repoName, "workers", "happy-eagle.log")
	if workerLog != expected {
		t.Errorf("AgentLogFile(happy-eagle, true) = %q, want %q", workerLog, expected)
	}
}
