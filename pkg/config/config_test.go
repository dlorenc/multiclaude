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
	if !strings.HasPrefix(paths.ClaudeConfigDir, paths.Root) {
		t.Errorf("ClaudeConfigDir not under Root: %s", paths.ClaudeConfigDir)
	}
}

func TestEnsureDirectories(t *testing.T) {
	tmpDir := t.TempDir()

	paths := &Paths{
		Root:            filepath.Join(tmpDir, "test-multiclaude"),
		ReposDir:        filepath.Join(tmpDir, "test-multiclaude", "repos"),
		WorktreesDir:    filepath.Join(tmpDir, "test-multiclaude", "wts"),
		MessagesDir:     filepath.Join(tmpDir, "test-multiclaude", "messages"),
		OutputDir:       filepath.Join(tmpDir, "test-multiclaude", "output"),
		ClaudeConfigDir: filepath.Join(tmpDir, "test-multiclaude", "claude-config"),
		ArchiveDir:      filepath.Join(tmpDir, "test-multiclaude", "archive"),
	}

	if err := paths.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories() failed: %v", err)
	}

	// Verify directories were created
	dirs := []string{paths.Root, paths.ReposDir, paths.WorktreesDir, paths.MessagesDir, paths.OutputDir, paths.ClaudeConfigDir, paths.ArchiveDir}
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
		Root:      tmpDir,
		OutputDir: filepath.Join(tmpDir, "output"),
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

func TestAgentClaudeConfigDir(t *testing.T) {
	tmpDir := t.TempDir()

	paths := &Paths{
		Root:            tmpDir,
		ClaudeConfigDir: filepath.Join(tmpDir, "claude-config"),
	}

	repoName := "test-repo"
	agentName := "happy-eagle"

	configDir := paths.AgentClaudeConfigDir(repoName, agentName)
	expected := filepath.Join(tmpDir, "claude-config", repoName, agentName)
	if configDir != expected {
		t.Errorf("AgentClaudeConfigDir() = %q, want %q", configDir, expected)
	}

	// Test with different agent types
	supervisorConfigDir := paths.AgentClaudeConfigDir(repoName, "supervisor")
	expected = filepath.Join(tmpDir, "claude-config", repoName, "supervisor")
	if supervisorConfigDir != expected {
		t.Errorf("AgentClaudeConfigDir(supervisor) = %q, want %q", supervisorConfigDir, expected)
	}
}

func TestAgentCommandsDir(t *testing.T) {
	tmpDir := t.TempDir()

	paths := &Paths{
		Root:            tmpDir,
		ClaudeConfigDir: filepath.Join(tmpDir, "claude-config"),
	}

	repoName := "test-repo"
	agentName := "happy-eagle"

	commandsDir := paths.AgentCommandsDir(repoName, agentName)
	expected := filepath.Join(tmpDir, "claude-config", repoName, agentName, "commands")
	if commandsDir != expected {
		t.Errorf("AgentCommandsDir() = %q, want %q", commandsDir, expected)
	}

	// Verify it builds on AgentClaudeConfigDir
	configDir := paths.AgentClaudeConfigDir(repoName, agentName)
	expectedFromConfig := filepath.Join(configDir, "commands")
	if commandsDir != expectedFromConfig {
		t.Errorf("AgentCommandsDir should be AgentClaudeConfigDir + 'commands'")
	}
}

func TestNewTestPaths(t *testing.T) {
	tmpDir := t.TempDir()

	paths := NewTestPaths(tmpDir)

	// Verify all paths are set correctly
	if paths.Root != tmpDir {
		t.Errorf("Root = %q, want %q", paths.Root, tmpDir)
	}

	expectedPaths := map[string]string{
		"DaemonPID":       filepath.Join(tmpDir, "daemon.pid"),
		"DaemonSock":      filepath.Join(tmpDir, "daemon.sock"),
		"DaemonLog":       filepath.Join(tmpDir, "daemon.log"),
		"StateFile":       filepath.Join(tmpDir, "state.json"),
		"ReposDir":        filepath.Join(tmpDir, "repos"),
		"WorktreesDir":    filepath.Join(tmpDir, "wts"),
		"MessagesDir":     filepath.Join(tmpDir, "messages"),
		"OutputDir":       filepath.Join(tmpDir, "output"),
		"ClaudeConfigDir": filepath.Join(tmpDir, "claude-config"),
	}

	if paths.DaemonPID != expectedPaths["DaemonPID"] {
		t.Errorf("DaemonPID = %q, want %q", paths.DaemonPID, expectedPaths["DaemonPID"])
	}
	if paths.DaemonSock != expectedPaths["DaemonSock"] {
		t.Errorf("DaemonSock = %q, want %q", paths.DaemonSock, expectedPaths["DaemonSock"])
	}
	if paths.DaemonLog != expectedPaths["DaemonLog"] {
		t.Errorf("DaemonLog = %q, want %q", paths.DaemonLog, expectedPaths["DaemonLog"])
	}
	if paths.StateFile != expectedPaths["StateFile"] {
		t.Errorf("StateFile = %q, want %q", paths.StateFile, expectedPaths["StateFile"])
	}
	if paths.ReposDir != expectedPaths["ReposDir"] {
		t.Errorf("ReposDir = %q, want %q", paths.ReposDir, expectedPaths["ReposDir"])
	}
	if paths.WorktreesDir != expectedPaths["WorktreesDir"] {
		t.Errorf("WorktreesDir = %q, want %q", paths.WorktreesDir, expectedPaths["WorktreesDir"])
	}
	if paths.MessagesDir != expectedPaths["MessagesDir"] {
		t.Errorf("MessagesDir = %q, want %q", paths.MessagesDir, expectedPaths["MessagesDir"])
	}
	if paths.OutputDir != expectedPaths["OutputDir"] {
		t.Errorf("OutputDir = %q, want %q", paths.OutputDir, expectedPaths["OutputDir"])
	}
	if paths.ClaudeConfigDir != expectedPaths["ClaudeConfigDir"] {
		t.Errorf("ClaudeConfigDir = %q, want %q", paths.ClaudeConfigDir, expectedPaths["ClaudeConfigDir"])
	}

	// Verify helper methods work correctly
	repoDir := paths.RepoDir("test-repo")
	if repoDir != filepath.Join(tmpDir, "repos", "test-repo") {
		t.Errorf("RepoDir() on NewTestPaths result = %q, unexpected", repoDir)
	}
}
