package daemon

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/dlorenc/multiclaude/internal/state"
	"github.com/dlorenc/multiclaude/pkg/config"
)

// createTestGitRepo creates a temporary git repository for testing
func createTestGitRepo(t *testing.T, dir string) {
	t.Helper()

	// Initialize git repo with explicit 'main' branch
	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Configure git user (required for commits)
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = dir
	cmd.Run()

	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = dir
	cmd.Run()

	// Create initial commit on main branch
	testFile := filepath.Join(dir, "README.md")
	if err := os.WriteFile(testFile, []byte("# Test Repo\n"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cmd = exec.Command("git", "add", "README.md")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}
}

// setupTestDaemonWithGitRepo creates a test daemon with a real git repository
func setupTestDaemonWithGitRepo(t *testing.T) (*Daemon, string, func()) {
	t.Helper()

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "daemon-worktree-test-*")
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

	// Create a git repo
	repoDir := filepath.Join(paths.ReposDir, "test-repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatalf("Failed to create repo dir: %v", err)
	}
	createTestGitRepo(t, repoDir)

	// Create daemon
	d, err := New(paths)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return d, repoDir, cleanup
}

func TestRefreshWorktrees_NoRepos(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Should not panic with no repos
	d.refreshWorktrees()
}

func TestRefreshWorktrees_NoWorkerAgents(t *testing.T) {
	d, repoDir, cleanup := setupTestDaemonWithGitRepo(t)
	defer cleanup()

	// Add repo with supervisor agent only
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "test-session",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.state.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	supervisorAgent := state.Agent{
		Type:         state.AgentTypeSupervisor,
		WorktreePath: repoDir,
		TmuxWindow:   "supervisor",
		SessionID:    "supervisor-session",
		CreatedAt:    time.Now(),
	}
	if err := d.state.AddAgent("test-repo", "supervisor", supervisorAgent); err != nil {
		t.Fatalf("Failed to add agent: %v", err)
	}

	// Should not panic - supervisor agents are skipped
	d.refreshWorktrees()
}

func TestRefreshWorktrees_EmptyWorktreePath(t *testing.T) {
	d, _, cleanup := setupTestDaemonWithGitRepo(t)
	defer cleanup()

	// Add repo with worker agent with empty worktree path
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "test-session",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.state.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	workerAgent := state.Agent{
		Type:         state.AgentTypeWorker,
		WorktreePath: "", // Empty path should be skipped
		TmuxWindow:   "worker",
		SessionID:    "worker-session",
		CreatedAt:    time.Now(),
	}
	if err := d.state.AddAgent("test-repo", "worker", workerAgent); err != nil {
		t.Fatalf("Failed to add agent: %v", err)
	}

	// Should not panic - empty worktree paths are skipped
	d.refreshWorktrees()
}

func TestRefreshWorktrees_NonExistentWorktreePath(t *testing.T) {
	d, _, cleanup := setupTestDaemonWithGitRepo(t)
	defer cleanup()

	// Add repo with worker agent with non-existent worktree path
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "test-session",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.state.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	workerAgent := state.Agent{
		Type:         state.AgentTypeWorker,
		WorktreePath: "/nonexistent/path", // Non-existent path should be skipped
		TmuxWindow:   "worker",
		SessionID:    "worker-session",
		CreatedAt:    time.Now(),
	}
	if err := d.state.AddAgent("test-repo", "worker", workerAgent); err != nil {
		t.Fatalf("Failed to add agent: %v", err)
	}

	// Should not panic - non-existent worktree paths are skipped
	d.refreshWorktrees()
}

func TestRefreshWorktrees_NonExistentRepoPath(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Add repo that doesn't have a local clone
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/nonexistent-repo",
		TmuxSession: "test-session",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.state.AddRepo("nonexistent-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	workerAgent := state.Agent{
		Type:         state.AgentTypeWorker,
		WorktreePath: "/tmp/some-path",
		TmuxWindow:   "worker",
		SessionID:    "worker-session",
		CreatedAt:    time.Now(),
	}
	if err := d.state.AddAgent("nonexistent-repo", "worker", workerAgent); err != nil {
		t.Fatalf("Failed to add agent: %v", err)
	}

	// Should not panic - repos without local clones are skipped
	d.refreshWorktrees()
}

func TestCleanupOrphanedWorktrees_NoRepos(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Should not panic with no repos
	d.cleanupOrphanedWorktrees()
}

func TestCleanupOrphanedWorktrees_NonExistentWorktreeDir(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Add repo without any worktrees created
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "test-session",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.state.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Should not panic - worktree dir doesn't exist
	d.cleanupOrphanedWorktrees()
}

func TestCleanupOrphanedWorktrees_WithWorktreeDir(t *testing.T) {
	d, repoDir, cleanup := setupTestDaemonWithGitRepo(t)
	defer cleanup()

	// Add repo
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "test-session",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.state.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Create worktrees directory
	wtDir := d.paths.WorktreeDir("test-repo")
	if err := os.MkdirAll(wtDir, 0755); err != nil {
		t.Fatalf("Failed to create worktree dir: %v", err)
	}

	// Create a directory that is NOT a git worktree (orphaned directory)
	orphanDir := filepath.Join(wtDir, "orphan-dir")
	if err := os.MkdirAll(orphanDir, 0755); err != nil {
		t.Fatalf("Failed to create orphan dir: %v", err)
	}

	// Create a git worktree that is properly registered
	gitWtPath := filepath.Join(wtDir, "git-wt")
	cmd := exec.Command("git", "worktree", "add", "-b", "git-branch", gitWtPath, "main")
	cmd.Dir = repoDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}

	// Run cleanup - should remove orphan-dir but NOT git-wt
	d.cleanupOrphanedWorktrees()

	// Orphan directory should be removed (not a git worktree)
	if _, err := os.Stat(orphanDir); !os.IsNotExist(err) {
		t.Error("Orphaned directory should have been cleaned up")
	}

	// Git worktree should still exist (it's tracked by git)
	if _, err := os.Stat(gitWtPath); os.IsNotExist(err) {
		t.Error("Git worktree should NOT have been cleaned up")
	}
}

func TestCleanupMergedBranches_NoRepos(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Should not panic with no repos
	d.cleanupMergedBranches()
}

func TestCleanupMergedBranches_NonExistentRepoPath(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Add repo without local clone
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/nonexistent",
		TmuxSession: "test-session",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.state.AddRepo("nonexistent", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Should not panic - repo path doesn't exist
	d.cleanupMergedBranches()
}

func TestWorktreeRefreshLoopTrigger(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Trigger should not block
	d.TriggerWorktreeRefresh()

	// Call again to test non-blocking behavior
	d.TriggerWorktreeRefresh()
}

func TestAppendToSliceMapWorktree(t *testing.T) {
	m := make(map[string][]string)

	appendToSliceMap(m, "key1", "value1")
	appendToSliceMap(m, "key1", "value2")
	appendToSliceMap(m, "key2", "value3")

	if len(m["key1"]) != 2 {
		t.Errorf("Expected 2 values for key1, got %d", len(m["key1"]))
	}
	if len(m["key2"]) != 1 {
		t.Errorf("Expected 1 value for key2, got %d", len(m["key2"]))
	}
	if m["key1"][0] != "value1" || m["key1"][1] != "value2" {
		t.Errorf("Unexpected values for key1: %v", m["key1"])
	}
	if m["key2"][0] != "value3" {
		t.Errorf("Unexpected value for key2: %v", m["key2"])
	}
}

func TestGetClaudeBinaryPathWorktree(t *testing.T) {
	d, cleanup := setupTestDaemon(t)
	defer cleanup()

	// Get the binary path (returns path and error)
	binaryPath, err := d.getClaudeBinaryPath()

	// If claude is not installed, the test should skip
	if err != nil {
		t.Logf("Claude binary not found (expected in test environments): %v", err)
		return
	}

	// Should return a non-empty path when found
	if binaryPath == "" {
		t.Error("Expected non-empty binary path when found")
	}

	// Path should be an absolute path
	if !filepath.IsAbs(binaryPath) {
		t.Errorf("Expected absolute path, got: %s", binaryPath)
	}
}

func TestRefreshWorktrees_WorkerOnMainBranch(t *testing.T) {
	d, repoDir, cleanup := setupTestDaemonWithGitRepo(t)
	defer cleanup()

	// Add repo with worker agent on main branch (no worktree path)
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "test-session",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.state.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	workerAgent := state.Agent{
		Type:         state.AgentTypeWorker,
		WorktreePath: repoDir, // Points to main repo which is on main branch
		TmuxWindow:   "worker",
		SessionID:    "worker-session",
		CreatedAt:    time.Now(),
	}
	if err := d.state.AddAgent("test-repo", "worker", workerAgent); err != nil {
		t.Fatalf("Failed to add agent: %v", err)
	}

	// Should not panic - worker on main branch is skipped by refresh logic
	d.refreshWorktrees()
}

func TestRefreshWorktrees_MultipleWorkers(t *testing.T) {
	d, repoDir, cleanup := setupTestDaemonWithGitRepo(t)
	defer cleanup()

	// Add repo with multiple worker agents
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "test-session",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.state.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Create worktrees for workers
	wtDir := d.paths.WorktreeDir("test-repo")
	if err := os.MkdirAll(wtDir, 0755); err != nil {
		t.Fatalf("Failed to create worktree dir: %v", err)
	}

	for i, name := range []string{"worker1", "worker2", "worker3"} {
		wtPath := filepath.Join(wtDir, name)
		branchName := "branch-" + name
		cmd := exec.Command("git", "worktree", "add", "-b", branchName, wtPath, "main")
		cmd.Dir = repoDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to create worktree for %s: %v", name, err)
		}

		workerAgent := state.Agent{
			Type:         state.AgentTypeWorker,
			WorktreePath: wtPath,
			TmuxWindow:   name,
			SessionID:    "session-" + name,
			CreatedAt:    time.Now().Add(time.Duration(i) * time.Second),
		}
		if err := d.state.AddAgent("test-repo", name, workerAgent); err != nil {
			t.Fatalf("Failed to add agent %s: %v", name, err)
		}
	}

	// Should not panic - processes all workers
	d.refreshWorktrees()
}

func TestCleanupOrphanedWorktrees_WithActiveWorktrees(t *testing.T) {
	d, repoDir, cleanup := setupTestDaemonWithGitRepo(t)
	defer cleanup()

	// Add repo
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "test-session",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.state.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Create worktrees directory
	wtDir := d.paths.WorktreeDir("test-repo")
	if err := os.MkdirAll(wtDir, 0755); err != nil {
		t.Fatalf("Failed to create worktree dir: %v", err)
	}

	// Create an active worktree (with agent reference)
	activeWtPath := filepath.Join(wtDir, "active-wt")
	cmd := exec.Command("git", "worktree", "add", "-b", "active-branch", activeWtPath, "main")
	cmd.Dir = repoDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}

	// Add agent referencing this worktree
	workerAgent := state.Agent{
		Type:         state.AgentTypeWorker,
		WorktreePath: activeWtPath,
		TmuxWindow:   "worker",
		SessionID:    "worker-session",
		CreatedAt:    time.Now(),
	}
	if err := d.state.AddAgent("test-repo", "worker", workerAgent); err != nil {
		t.Fatalf("Failed to add agent: %v", err)
	}

	// Create an orphaned directory (not a git worktree, just a stray directory)
	orphanDirPath := filepath.Join(wtDir, "orphan-dir")
	if err := os.MkdirAll(orphanDirPath, 0755); err != nil {
		t.Fatalf("Failed to create orphan dir: %v", err)
	}

	// Run cleanup
	d.cleanupOrphanedWorktrees()

	// Active worktree should still exist (it's tracked by git)
	if _, err := os.Stat(activeWtPath); os.IsNotExist(err) {
		t.Error("Active worktree should NOT be cleaned up")
	}

	// Orphaned directory should be removed (not tracked by git)
	if _, err := os.Stat(orphanDirPath); !os.IsNotExist(err) {
		t.Error("Orphaned directory should have been cleaned up")
	}
}

func TestCleanupMergedBranches_WithLocalClone(t *testing.T) {
	d, _, cleanup := setupTestDaemonWithGitRepo(t)
	defer cleanup()

	// Add repo with local clone
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "test-session",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.state.AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Should not panic - the repo has no remote so it will skip cleanup
	d.cleanupMergedBranches()
}
