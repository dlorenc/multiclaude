package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// createTestRepoWithRemote creates a test repo with an origin remote
func createTestRepoWithRemote(t *testing.T) (string, func()) {
	t.Helper()

	// Create temp directory for the "remote" bare repo
	remoteDir, err := os.MkdirTemp("", "worktree-remote-*")
	if err != nil {
		t.Fatalf("Failed to create remote dir: %v", err)
	}

	// Initialize bare repo as remote with initial branch
	cmd := exec.Command("git", "init", "--bare", "--initial-branch=main")
	cmd.Dir = remoteDir
	if err := cmd.Run(); err != nil {
		os.RemoveAll(remoteDir)
		t.Fatalf("Failed to init bare repo: %v", err)
	}

	// Create temp directory for the local repo
	localDir, err := os.MkdirTemp("", "worktree-local-*")
	if err != nil {
		os.RemoveAll(remoteDir)
		t.Fatalf("Failed to create local dir: %v", err)
	}

	// Initialize local repo
	cmd = exec.Command("git", "init", "-b", "main")
	cmd.Dir = localDir
	if err := cmd.Run(); err != nil {
		os.RemoveAll(remoteDir)
		os.RemoveAll(localDir)
		t.Fatalf("Failed to init local repo: %v", err)
	}

	// Configure git user
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = localDir
	cmd.Run()

	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = localDir
	cmd.Run()

	// Create initial commit
	testFile := filepath.Join(localDir, "README.md")
	if err := os.WriteFile(testFile, []byte("# Test Repo\n"), 0644); err != nil {
		os.RemoveAll(remoteDir)
		os.RemoveAll(localDir)
		t.Fatalf("Failed to create test file: %v", err)
	}

	cmd = exec.Command("git", "add", "README.md")
	cmd.Dir = localDir
	if err := cmd.Run(); err != nil {
		os.RemoveAll(remoteDir)
		os.RemoveAll(localDir)
		t.Fatalf("Failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = localDir
	if err := cmd.Run(); err != nil {
		os.RemoveAll(remoteDir)
		os.RemoveAll(localDir)
		t.Fatalf("Failed to commit: %v", err)
	}

	// Add remote
	cmd = exec.Command("git", "remote", "add", "origin", remoteDir)
	cmd.Dir = localDir
	if err := cmd.Run(); err != nil {
		os.RemoveAll(remoteDir)
		os.RemoveAll(localDir)
		t.Fatalf("Failed to add remote: %v", err)
	}

	// Push to remote
	cmd = exec.Command("git", "push", "-u", "origin", "main")
	cmd.Dir = localDir
	if err := cmd.Run(); err != nil {
		os.RemoveAll(remoteDir)
		os.RemoveAll(localDir)
		t.Fatalf("Failed to push: %v", err)
	}

	cleanup := func() {
		os.RemoveAll(remoteDir)
		os.RemoveAll(localDir)
	}

	return localDir, cleanup
}

// addCommitToRemote adds a commit to the remote by creating another clone,
// committing, and pushing
func addCommitToRemote(t *testing.T, localDir string, message string) {
	t.Helper()

	// Get the remote URL
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = localDir
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to get remote URL: %v", err)
	}
	remoteURL := strings.TrimSpace(string(output))

	// Create a temp clone to make changes
	tempClone, err := os.MkdirTemp("", "temp-clone-*")
	if err != nil {
		t.Fatalf("Failed to create temp clone dir: %v", err)
	}
	defer os.RemoveAll(tempClone)

	cmd = exec.Command("git", "clone", remoteURL, tempClone)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to clone for remote commit: %v", err)
	}

	// Configure git user
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tempClone
	cmd.Run()
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tempClone
	cmd.Run()

	// Create a new file and commit
	newFile := filepath.Join(tempClone, message+".txt")
	if err := os.WriteFile(newFile, []byte(message+"\n"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = tempClone
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", message)
	cmd.Dir = tempClone
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	cmd = exec.Command("git", "push", "origin", "main")
	cmd.Dir = tempClone
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to push: %v", err)
	}
}

func TestRefreshWorktree_DetachedHead(t *testing.T) {
	repoPath, cleanup := createTestRepo(t)
	defer cleanup()

	manager := NewManager(repoPath)

	// Create a worktree
	wtPath := filepath.Join(repoPath, "wt-detached")
	if err := manager.CreateNewBranch(wtPath, "feature-branch", "main"); err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}

	// Detach HEAD in the worktree
	cmd := exec.Command("git", "checkout", "--detach")
	cmd.Dir = wtPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to detach HEAD: %v", err)
	}

	// RefreshWorktree should skip detached HEAD
	result := RefreshWorktree(wtPath, "origin", "main")
	if !result.Skipped {
		t.Error("Expected RefreshWorktree to skip detached HEAD")
	}
	if result.SkipReason == "" || !strings.Contains(result.SkipReason, "detached HEAD") {
		t.Errorf("Expected detached HEAD skip reason, got: %s", result.SkipReason)
	}
}

func TestRefreshWorktree_OnMainBranch(t *testing.T) {
	repoPath, cleanup := createTestRepo(t)
	defer cleanup()

	// RefreshWorktree should skip if on main branch
	result := RefreshWorktree(repoPath, "origin", "main")
	if !result.Skipped {
		t.Error("Expected RefreshWorktree to skip main branch")
	}
	if result.SkipReason == "" || !strings.Contains(result.SkipReason, "main branch") {
		t.Errorf("Expected main branch skip reason, got: %s", result.SkipReason)
	}
}

func TestRefreshWorktree_MidRebase(t *testing.T) {
	repoPath, cleanup := createTestRepo(t)
	defer cleanup()

	manager := NewManager(repoPath)

	// Create a worktree
	wtPath := filepath.Join(repoPath, "wt-rebase")
	if err := manager.CreateNewBranch(wtPath, "feature-branch", "main"); err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}

	// Simulate mid-rebase state by creating the rebase-merge directory
	gitDir := filepath.Join(wtPath, ".git")
	content, err := os.ReadFile(gitDir)
	if err == nil && strings.HasPrefix(string(content), "gitdir:") {
		gitDir = strings.TrimSpace(strings.TrimPrefix(string(content), "gitdir:"))
	}
	rebaseDir := filepath.Join(gitDir, "rebase-merge")
	if err := os.MkdirAll(rebaseDir, 0755); err != nil {
		t.Fatalf("Failed to create rebase-merge dir: %v", err)
	}

	// RefreshWorktree should skip mid-rebase
	result := RefreshWorktree(wtPath, "origin", "main")
	if !result.Skipped {
		t.Error("Expected RefreshWorktree to skip mid-rebase state")
	}
	if result.SkipReason == "" || !strings.Contains(result.SkipReason, "mid-rebase") {
		t.Errorf("Expected mid-rebase skip reason, got: %s", result.SkipReason)
	}
}

func TestRefreshWorktree_MidMerge(t *testing.T) {
	repoPath, cleanup := createTestRepo(t)
	defer cleanup()

	manager := NewManager(repoPath)

	// Create a worktree
	wtPath := filepath.Join(repoPath, "wt-merge")
	if err := manager.CreateNewBranch(wtPath, "feature-branch", "main"); err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}

	// Simulate mid-merge state by creating MERGE_HEAD file
	gitDir := filepath.Join(wtPath, ".git")
	content, err := os.ReadFile(gitDir)
	if err == nil && strings.HasPrefix(string(content), "gitdir:") {
		gitDir = strings.TrimSpace(strings.TrimPrefix(string(content), "gitdir:"))
	}
	mergeHead := filepath.Join(gitDir, "MERGE_HEAD")
	if err := os.WriteFile(mergeHead, []byte("abc123"), 0644); err != nil {
		t.Fatalf("Failed to create MERGE_HEAD: %v", err)
	}

	// RefreshWorktree should skip mid-merge
	result := RefreshWorktree(wtPath, "origin", "main")
	if !result.Skipped {
		t.Error("Expected RefreshWorktree to skip mid-merge state")
	}
	if result.SkipReason == "" || !strings.Contains(result.SkipReason, "mid-merge") {
		t.Errorf("Expected mid-merge skip reason, got: %s", result.SkipReason)
	}
}

func TestRefreshWorktree_NonExistentPath(t *testing.T) {
	// RefreshWorktree should return error for non-existent path
	result := RefreshWorktree("/nonexistent/path", "origin", "main")
	if result.Error == nil {
		t.Error("Expected error for non-existent path")
	}
}

func TestGetWorktreeState_UpToDate(t *testing.T) {
	repoPath, cleanup := createTestRepoWithRemote(t)
	defer cleanup()

	manager := NewManager(repoPath)

	// Create a worktree on a feature branch
	wtPath := filepath.Join(repoPath, "wt-state")
	if err := manager.CreateNewBranch(wtPath, "feature-branch", "main"); err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}

	// Get worktree state
	state, err := GetWorktreeState(wtPath, "origin", "main")
	if err != nil {
		t.Fatalf("GetWorktreeState() failed: %v", err)
	}

	// Should be up to date initially
	if state.CommitsBehind != 0 {
		t.Errorf("Expected 0 commits behind, got %d", state.CommitsBehind)
	}
	if state.CanRefresh {
		t.Error("Should not be able to refresh when up to date")
	}
}

func TestGetWorktreeState_DetachedHead(t *testing.T) {
	repoPath, cleanup := createTestRepo(t)
	defer cleanup()

	manager := NewManager(repoPath)

	// Create a worktree
	wtPath := filepath.Join(repoPath, "wt-detached-state")
	if err := manager.CreateNewBranch(wtPath, "feature-branch", "main"); err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}

	// Detach HEAD
	cmd := exec.Command("git", "checkout", "--detach")
	cmd.Dir = wtPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to detach HEAD: %v", err)
	}

	// GetWorktreeState should indicate can't refresh
	state, err := GetWorktreeState(wtPath, "origin", "main")
	if err != nil {
		t.Fatalf("GetWorktreeState() failed: %v", err)
	}

	if state.CanRefresh {
		t.Error("Should not be able to refresh with detached HEAD")
	}
	if !strings.Contains(state.RefreshReason, "detached") {
		t.Errorf("Expected detached HEAD reason, got: %s", state.RefreshReason)
	}
}

func TestGetWorktreeState_OnMainBranch(t *testing.T) {
	repoPath, cleanup := createTestRepoWithRemote(t)
	defer cleanup()

	// GetWorktreeState for main branch
	state, err := GetWorktreeState(repoPath, "origin", "main")
	if err != nil {
		t.Fatalf("GetWorktreeState() failed: %v", err)
	}

	if state.CanRefresh {
		t.Error("Should not be able to refresh main branch")
	}
	if !strings.Contains(state.RefreshReason, "main branch") {
		t.Errorf("Expected main branch reason, got: %s", state.RefreshReason)
	}
}

func TestGetDefaultBranch_Main(t *testing.T) {
	repoPath, cleanup := createTestRepoWithRemote(t)
	defer cleanup()

	manager := NewManager(repoPath)

	// Get default branch
	branch, err := manager.GetDefaultBranch("origin")
	if err != nil {
		t.Fatalf("GetDefaultBranch() failed: %v", err)
	}

	// Should be main
	if branch != "main" {
		t.Errorf("Expected 'main', got %q", branch)
	}
}

func TestGetDefaultBranch_NoRemote(t *testing.T) {
	repoPath, cleanup := createTestRepo(t)
	defer cleanup()

	manager := NewManager(repoPath)

	// Get default branch for non-existent remote
	_, err := manager.GetDefaultBranch("nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent remote")
	}
}

func TestGetUpstreamRemote_OnlyOrigin(t *testing.T) {
	repoPath, cleanup := createTestRepoWithRemote(t)
	defer cleanup()

	manager := NewManager(repoPath)

	// Should return origin when no upstream
	remote, err := manager.GetUpstreamRemote()
	if err != nil {
		t.Fatalf("GetUpstreamRemote() failed: %v", err)
	}
	if remote != "origin" {
		t.Errorf("Expected 'origin', got %q", remote)
	}
}

func TestGetUpstreamRemote_WithUpstream(t *testing.T) {
	repoPath, cleanup := createTestRepoWithRemote(t)
	defer cleanup()

	// Add upstream remote
	cmd := exec.Command("git", "remote", "add", "upstream", "https://github.com/test/upstream.git")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to add upstream: %v", err)
	}

	manager := NewManager(repoPath)

	// Should return upstream when present
	remote, err := manager.GetUpstreamRemote()
	if err != nil {
		t.Fatalf("GetUpstreamRemote() failed: %v", err)
	}
	if remote != "upstream" {
		t.Errorf("Expected 'upstream', got %q", remote)
	}
}

func TestGetUpstreamRemote_NoRemotes(t *testing.T) {
	repoPath, cleanup := createTestRepo(t)
	defer cleanup()

	manager := NewManager(repoPath)

	// Should return error when no remotes
	_, err := manager.GetUpstreamRemote()
	if err == nil {
		t.Error("Expected error when no remotes configured")
	}
}

func TestIsBehindMain_UpToDate(t *testing.T) {
	repoPath, cleanup := createTestRepoWithRemote(t)
	defer cleanup()

	manager := NewManager(repoPath)

	// Create a worktree
	wtPath := filepath.Join(repoPath, "wt-behind")
	if err := manager.CreateNewBranch(wtPath, "feature-branch", "main"); err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}

	// Should not be behind initially
	behind, count, err := IsBehindMain(wtPath, "origin", "main")
	if err != nil {
		t.Fatalf("IsBehindMain() failed: %v", err)
	}
	if behind {
		t.Error("Should not be behind when up to date")
	}
	if count != 0 {
		t.Errorf("Expected 0 commits behind, got %d", count)
	}
}

func TestRefreshWorktreeWithDefaults_NoRemote(t *testing.T) {
	repoPath, cleanup := createTestRepo(t)
	defer cleanup()

	manager := NewManager(repoPath)

	// Create a worktree
	wtPath := filepath.Join(repoPath, "wt-no-remote")
	if err := manager.CreateNewBranch(wtPath, "feature-branch", "main"); err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}

	// RefreshWorktreeWithDefaults should fail without remote
	result := manager.RefreshWorktreeWithDefaults(wtPath)
	if result.Error == nil {
		t.Error("Expected error when no remote configured")
	}
}

func TestRefreshWorktree_WithUncommittedChanges(t *testing.T) {
	repoPath, cleanup := createTestRepoWithRemote(t)
	defer cleanup()

	manager := NewManager(repoPath)

	// Create a worktree
	wtPath := filepath.Join(repoPath, "wt-uncommitted")
	if err := manager.CreateNewBranch(wtPath, "feature-branch", "main"); err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}

	// Create uncommitted changes
	testFile := filepath.Join(wtPath, "uncommitted.txt")
	if err := os.WriteFile(testFile, []byte("uncommitted content"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	// RefreshWorktree should handle uncommitted changes (stash and restore)
	result := RefreshWorktree(wtPath, "origin", "main")
	// Since there's nothing new on main, this might skip or succeed
	// The key is it shouldn't lose the uncommitted changes
	if result.Error != nil && !strings.Contains(result.Error.Error(), "fetch") {
		t.Errorf("Unexpected error: %v", result.Error)
	}

	// Verify uncommitted file still exists
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Error("Uncommitted file should still exist after refresh")
	}
}

func TestRefreshResult_Fields(t *testing.T) {
	// Test RefreshResult struct
	result := RefreshResult{
		WorktreePath:   "/test/path",
		Branch:         "feature",
		CommitsRebased: 3,
		WasStashed:     true,
		StashRestored:  true,
		HasConflicts:   false,
		ConflictFiles:  nil,
		Error:          nil,
		Skipped:        false,
		SkipReason:     "",
	}

	if result.WorktreePath != "/test/path" {
		t.Errorf("WorktreePath = %q, want %q", result.WorktreePath, "/test/path")
	}
	if result.Branch != "feature" {
		t.Errorf("Branch = %q, want %q", result.Branch, "feature")
	}
	if result.CommitsRebased != 3 {
		t.Errorf("CommitsRebased = %d, want %d", result.CommitsRebased, 3)
	}
	if !result.WasStashed {
		t.Error("WasStashed should be true")
	}
	if !result.StashRestored {
		t.Error("StashRestored should be true")
	}
}

func TestGetWorktreeState_WithMidRebaseApply(t *testing.T) {
	repoPath, cleanup := createTestRepo(t)
	defer cleanup()

	manager := NewManager(repoPath)

	// Create a worktree
	wtPath := filepath.Join(repoPath, "wt-rebase-apply")
	if err := manager.CreateNewBranch(wtPath, "feature-branch", "main"); err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}

	// Simulate mid-rebase-apply state
	gitDir := filepath.Join(wtPath, ".git")
	content, err := os.ReadFile(gitDir)
	if err == nil && strings.HasPrefix(string(content), "gitdir:") {
		gitDir = strings.TrimSpace(strings.TrimPrefix(string(content), "gitdir:"))
	}
	rebaseApplyDir := filepath.Join(gitDir, "rebase-apply")
	if err := os.MkdirAll(rebaseApplyDir, 0755); err != nil {
		t.Fatalf("Failed to create rebase-apply dir: %v", err)
	}

	// GetWorktreeState should detect mid-rebase
	state, err := GetWorktreeState(wtPath, "origin", "main")
	if err != nil {
		t.Fatalf("GetWorktreeState() failed: %v", err)
	}

	if state.CanRefresh {
		t.Error("Should not be able to refresh during rebase-apply")
	}
	if !strings.Contains(state.RefreshReason, "mid-rebase") {
		t.Errorf("Expected mid-rebase reason, got: %s", state.RefreshReason)
	}
}
