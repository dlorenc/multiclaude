package worktree

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMain ensures git is available
func TestMain(m *testing.M) {
	// Check if git is available
	if exec.Command("git", "version").Run() != nil {
		fmt.Fprintln(os.Stderr, "Warning: git not available, skipping worktree tests")
		os.Exit(0)
	}

	os.Exit(m.Run())
}

// createTestRepo creates a temporary git repository for testing
func createTestRepo(t *testing.T) (string, func()) {
	t.Helper()

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "worktree-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	// Initialize git repo with explicit 'main' branch
	// This ensures consistency across different git versions and CI environments
	// (older git versions default to 'master', newer ones may use 'main')
	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		cleanup()
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Configure git user (required for commits)
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tmpDir
	cmd.Run()

	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tmpDir
	cmd.Run()

	// Create initial commit on main branch
	testFile := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(testFile, []byte("# Test Repo\n"), 0644); err != nil {
		cleanup()
		t.Fatalf("Failed to create test file: %v", err)
	}

	cmd = exec.Command("git", "add", "README.md")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		cleanup()
		t.Fatalf("Failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		cleanup()
		t.Fatalf("Failed to commit: %v", err)
	}

	return tmpDir, cleanup
}

// createBranch creates a new branch in the repo
func createBranch(t *testing.T, repoPath, branchName string) {
	t.Helper()

	cmd := exec.Command("git", "branch", branchName)
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create branch %s: %v", branchName, err)
	}
}

func TestNewManager(t *testing.T) {
	repoPath := "/tmp/test-repo"
	manager := NewManager(repoPath)

	if manager == nil {
		t.Fatal("NewManager returned nil")
	}

	if manager.repoPath != repoPath {
		t.Errorf("Expected repoPath %s, got %s", repoPath, manager.repoPath)
	}
}

func TestCreateWorktree(t *testing.T) {
	repoPath, cleanup := createTestRepo(t)
	defer cleanup()

	manager := NewManager(repoPath)

	// Create a branch first (can't use main as it's already checked out)
	createBranch(t, repoPath, "test-branch")

	// Create worktree path
	wtPath := filepath.Join(repoPath, "wt-test")

	// Create worktree from test branch
	if err := manager.Create(wtPath, "test-branch"); err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}

	// Verify worktree directory exists
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		t.Error("Worktree directory was not created")
	}

	// Verify worktree is registered in git
	exists, err := manager.Exists(wtPath)
	if err != nil {
		t.Fatalf("Failed to check worktree existence: %v", err)
	}
	if !exists {
		t.Error("Worktree not registered in git")
	}

	// Verify README.md exists in worktree
	readmePath := filepath.Join(wtPath, "README.md")
	if _, err := os.Stat(readmePath); os.IsNotExist(err) {
		t.Error("README.md not found in worktree")
	}
}

func TestCreateWorktreeNewBranch(t *testing.T) {
	repoPath, cleanup := createTestRepo(t)
	defer cleanup()

	manager := NewManager(repoPath)

	// Create worktree with new branch
	wtPath := filepath.Join(repoPath, "wt-feature")
	newBranch := "feature-branch"

	if err := manager.CreateNewBranch(wtPath, newBranch, "main"); err != nil {
		t.Fatalf("Failed to create worktree with new branch: %v", err)
	}

	// Verify worktree directory exists
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		t.Error("Worktree directory was not created")
	}

	// Verify correct branch is checked out
	currentBranch, err := GetCurrentBranch(wtPath)
	if err != nil {
		t.Fatalf("Failed to get current branch: %v", err)
	}
	if currentBranch != newBranch {
		t.Errorf("Expected branch %s, got %s", newBranch, currentBranch)
	}

	// Verify branch exists in main repo
	cmd := exec.Command("git", "branch", "--list", newBranch)
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to list branches: %v", err)
	}
	if !strings.Contains(string(output), newBranch) {
		t.Errorf("Branch %s not found in main repo", newBranch)
	}
}

func TestRemoveWorktree(t *testing.T) {
	repoPath, cleanup := createTestRepo(t)
	defer cleanup()

	manager := NewManager(repoPath)

	// Create worktree with new branch
	wtPath := filepath.Join(repoPath, "wt-remove")
	if err := manager.CreateNewBranch(wtPath, "wt-remove-branch", "main"); err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}

	// Remove worktree
	if err := manager.Remove(wtPath, false); err != nil {
		t.Fatalf("Failed to remove worktree: %v", err)
	}

	// Verify worktree directory is gone
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Error("Worktree directory still exists after removal")
	}

	// Verify worktree is not registered in git
	exists, err := manager.Exists(wtPath)
	if err != nil {
		t.Fatalf("Failed to check worktree existence: %v", err)
	}
	if exists {
		t.Error("Worktree still registered in git after removal")
	}
}

func TestRemoveWorktreeForce(t *testing.T) {
	repoPath, cleanup := createTestRepo(t)
	defer cleanup()

	manager := NewManager(repoPath)

	// Create worktree with new branch
	wtPath := filepath.Join(repoPath, "wt-force")
	if err := manager.CreateNewBranch(wtPath, "wt-force-branch", "main"); err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}

	// Make uncommitted changes
	testFile := filepath.Join(wtPath, "uncommitted.txt")
	if err := os.WriteFile(testFile, []byte("uncommitted"), 0644); err != nil {
		t.Fatalf("Failed to create uncommitted file: %v", err)
	}

	// Normal remove should fail with uncommitted changes
	err := manager.Remove(wtPath, false)
	if err == nil {
		t.Error("Remove without force should fail with uncommitted changes")
	}

	// Force remove should succeed
	if err := manager.Remove(wtPath, true); err != nil {
		t.Fatalf("Failed to force remove worktree: %v", err)
	}

	// Verify worktree is gone
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Error("Worktree directory still exists after force removal")
	}
}

func TestListWorktrees(t *testing.T) {
	repoPath, cleanup := createTestRepo(t)
	defer cleanup()

	manager := NewManager(repoPath)

	// List should initially show only main repo
	worktrees, err := manager.List()
	if err != nil {
		t.Fatalf("Failed to list worktrees: %v", err)
	}
	initialCount := len(worktrees)

	// Create multiple worktrees with new branches
	wt1Path := filepath.Join(repoPath, "wt1")
	wt2Path := filepath.Join(repoPath, "wt2")

	if err := manager.CreateNewBranch(wt1Path, "wt1-branch", "main"); err != nil {
		t.Fatalf("Failed to create wt1: %v", err)
	}
	if err := manager.CreateNewBranch(wt2Path, "wt2-branch", "main"); err != nil {
		t.Fatalf("Failed to create wt2: %v", err)
	}

	// List worktrees
	worktrees, err = manager.List()
	if err != nil {
		t.Fatalf("Failed to list worktrees: %v", err)
	}

	// Should have 2 more worktrees
	if len(worktrees) != initialCount+2 {
		t.Errorf("Expected %d worktrees, got %d", initialCount+2, len(worktrees))
	}

	// Verify our worktrees are in the list
	found1 := false
	found2 := false
	for _, wt := range worktrees {
		absWt1, _ := filepath.Abs(wt1Path)
		absWt2, _ := filepath.Abs(wt2Path)
		absWtPath, _ := filepath.Abs(wt.Path)

		// Resolve symlinks for accurate comparison
		evalWt1, _ := filepath.EvalSymlinks(absWt1)
		evalWt2, _ := filepath.EvalSymlinks(absWt2)
		evalWtPath, _ := filepath.EvalSymlinks(absWtPath)

		if evalWtPath == evalWt1 {
			found1 = true
		}
		if evalWtPath == evalWt2 {
			found2 = true
		}
	}

	if !found1 || !found2 {
		t.Error("Created worktrees not found in list")
	}
}

func TestWorktreeExists(t *testing.T) {
	repoPath, cleanup := createTestRepo(t)
	defer cleanup()

	manager := NewManager(repoPath)

	wtPath := filepath.Join(repoPath, "wt-exists")

	// Should not exist initially
	exists, err := manager.Exists(wtPath)
	if err != nil {
		t.Fatalf("Failed to check existence: %v", err)
	}
	if exists {
		t.Error("Worktree should not exist initially")
	}

	// Create worktree with new branch
	if err := manager.CreateNewBranch(wtPath, "exists-branch", "main"); err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}

	// Should exist now
	exists, err = manager.Exists(wtPath)
	if err != nil {
		t.Fatalf("Failed to check existence: %v", err)
	}
	if !exists {
		t.Error("Worktree should exist after creation")
	}
}

func TestPrune(t *testing.T) {
	repoPath, cleanup := createTestRepo(t)
	defer cleanup()

	manager := NewManager(repoPath)

	// Create worktree with new branch
	wtPath := filepath.Join(repoPath, "wt-prune")
	if err := manager.CreateNewBranch(wtPath, "prune-branch", "main"); err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}

	// Manually delete worktree directory (simulate orphaned state)
	if err := os.RemoveAll(wtPath); err != nil {
		t.Fatalf("Failed to remove worktree directory: %v", err)
	}

	// Worktree should still be registered in git
	worktrees, err := manager.List()
	if err != nil {
		t.Fatalf("Failed to list worktrees: %v", err)
	}

	foundOrphaned := false
	for _, wt := range worktrees {
		absWtPath, _ := filepath.Abs(wtPath)
		absPath, _ := filepath.Abs(wt.Path)
		evalWtPath, _ := filepath.EvalSymlinks(absWtPath)
		evalPath, _ := filepath.EvalSymlinks(absPath)
		if evalPath == evalWtPath {
			foundOrphaned = true
			break
		}
	}

	if !foundOrphaned {
		t.Error("Orphaned worktree should still be in git list before pruning")
	}

	// Prune should clean it up
	if err := manager.Prune(); err != nil {
		t.Fatalf("Failed to prune: %v", err)
	}

	// Worktree should no longer exist in git
	exists, err := manager.Exists(wtPath)
	if err != nil {
		t.Fatalf("Failed to check existence: %v", err)
	}
	if exists {
		t.Error("Worktree should not exist after pruning")
	}
}

func TestHasUncommittedChanges(t *testing.T) {
	repoPath, cleanup := createTestRepo(t)
	defer cleanup()

	manager := NewManager(repoPath)

	// Create worktree with new branch
	wtPath := filepath.Join(repoPath, "wt-uncommitted")
	if err := manager.CreateNewBranch(wtPath, "uncommitted-branch", "main"); err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}

	// Should have no uncommitted changes initially
	hasChanges, err := HasUncommittedChanges(wtPath)
	if err != nil {
		t.Fatalf("Failed to check uncommitted changes: %v", err)
	}
	if hasChanges {
		t.Error("Should have no uncommitted changes initially")
	}

	// Make uncommitted change
	testFile := filepath.Join(wtPath, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Should now have uncommitted changes
	hasChanges, err = HasUncommittedChanges(wtPath)
	if err != nil {
		t.Fatalf("Failed to check uncommitted changes: %v", err)
	}
	if !hasChanges {
		t.Error("Should have uncommitted changes after creating file")
	}

	// Commit the change
	cmd := exec.Command("git", "add", "test.txt")
	cmd.Dir = wtPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git add: %v", err)
	}

	// Should still have uncommitted changes (staged but not committed)
	hasChanges, err = HasUncommittedChanges(wtPath)
	if err != nil {
		t.Fatalf("Failed to check uncommitted changes: %v", err)
	}
	if !hasChanges {
		t.Error("Should have uncommitted changes with staged files")
	}

	// Commit
	cmd = exec.Command("git", "commit", "-m", "Add test file")
	cmd.Dir = wtPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Should have no uncommitted changes after commit
	hasChanges, err = HasUncommittedChanges(wtPath)
	if err != nil {
		t.Fatalf("Failed to check uncommitted changes: %v", err)
	}
	if hasChanges {
		t.Error("Should have no uncommitted changes after commit")
	}
}

func TestHasUnpushedCommits(t *testing.T) {
	repoPath, cleanup := createTestRepo(t)
	defer cleanup()

	manager := NewManager(repoPath)

	// Create worktree
	wtPath := filepath.Join(repoPath, "wt-unpushed")
	if err := manager.CreateNewBranch(wtPath, "feature", "main"); err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}

	// No tracking branch, so no unpushed commits
	hasUnpushed, err := HasUnpushedCommits(wtPath)
	if err != nil {
		t.Fatalf("Failed to check unpushed commits: %v", err)
	}
	if hasUnpushed {
		t.Error("Should have no unpushed commits without tracking branch")
	}

	// Create a commit
	testFile := filepath.Join(wtPath, "feature.txt")
	if err := os.WriteFile(testFile, []byte("feature"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cmd := exec.Command("git", "add", "feature.txt")
	cmd.Dir = wtPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Add feature")
	cmd.Dir = wtPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Still no tracking branch
	hasUnpushed, err = HasUnpushedCommits(wtPath)
	if err != nil {
		t.Fatalf("Failed to check unpushed commits: %v", err)
	}
	if hasUnpushed {
		t.Error("Should have no unpushed commits without tracking branch")
	}
}

func TestGetCurrentBranch(t *testing.T) {
	repoPath, cleanup := createTestRepo(t)
	defer cleanup()

	manager := NewManager(repoPath)

	// Test current branch of main repo
	branch, err := GetCurrentBranch(repoPath)
	if err != nil {
		t.Fatalf("Failed to get current branch of main repo: %v", err)
	}
	if branch != "main" {
		t.Errorf("Expected branch 'main' in main repo, got '%s'", branch)
	}

	// Create worktree with new branch
	wt2Path := filepath.Join(repoPath, "wt-feature")
	if err := manager.CreateNewBranch(wt2Path, "my-feature", "main"); err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}

	branch, err = GetCurrentBranch(wt2Path)
	if err != nil {
		t.Fatalf("Failed to get current branch: %v", err)
	}
	if branch != "my-feature" {
		t.Errorf("Expected branch 'my-feature', got '%s'", branch)
	}
}

func TestCleanupOrphaned(t *testing.T) {
	repoPath, cleanup := createTestRepo(t)
	defer cleanup()

	manager := NewManager(repoPath)

	// Create a worktree root directory
	wtRootDir, err := os.MkdirTemp("", "wt-root-*")
	if err != nil {
		t.Fatalf("Failed to create wt root dir: %v", err)
	}
	defer os.RemoveAll(wtRootDir)

	// Create a proper worktree in wtRootDir with new branch
	properWtPath := filepath.Join(wtRootDir, "proper-wt")
	if err := manager.CreateNewBranch(properWtPath, "proper-branch", "main"); err != nil {
		t.Fatalf("Failed to create proper worktree: %v", err)
	}

	// Create an orphaned directory (not a real worktree)
	orphanedPath := filepath.Join(wtRootDir, "orphaned-dir")
	if err := os.MkdirAll(orphanedPath, 0755); err != nil {
		t.Fatalf("Failed to create orphaned directory: %v", err)
	}

	// Create a file (should be ignored)
	filePath := filepath.Join(wtRootDir, "somefile.txt")
	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	// Verify both directories exist before cleanup
	if _, err := os.Stat(properWtPath); os.IsNotExist(err) {
		t.Error("Proper worktree should exist before cleanup")
	}
	if _, err := os.Stat(orphanedPath); os.IsNotExist(err) {
		t.Error("Orphaned directory should exist before cleanup")
	}

	// Run cleanup
	removed, err := CleanupOrphaned(wtRootDir, manager)
	if err != nil {
		t.Fatalf("CleanupOrphaned failed: %v", err)
	}

	// Should have removed exactly one directory (orphaned)
	if len(removed) != 1 {
		t.Errorf("Expected to remove 1 directory, removed %d: %v", len(removed), removed)
	}

	// Verify proper worktree still exists
	if _, err := os.Stat(properWtPath); os.IsNotExist(err) {
		t.Error("Proper worktree should not be removed")
	}

	// Verify orphaned directory was removed
	if _, err := os.Stat(orphanedPath); !os.IsNotExist(err) {
		t.Error("Orphaned directory should be removed")
	}

	// Verify file was not removed
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("File should not be removed")
	}
}

func TestWorktreeInfoParsing(t *testing.T) {
	repoPath, cleanup := createTestRepo(t)
	defer cleanup()

	manager := NewManager(repoPath)

	// Create worktree with new branch
	wtPath := filepath.Join(repoPath, "wt-info")
	branchName := "test-branch"
	if err := manager.CreateNewBranch(wtPath, branchName, "main"); err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}

	// List worktrees and check info
	worktrees, err := manager.List()
	if err != nil {
		t.Fatalf("Failed to list worktrees: %v", err)
	}

	// Find our worktree
	var foundWt *WorktreeInfo
	for _, wt := range worktrees {
		absWt, _ := filepath.Abs(wt.Path)
		absWtPath, _ := filepath.Abs(wtPath)
		evalWt, _ := filepath.EvalSymlinks(absWt)
		evalWtPath, _ := filepath.EvalSymlinks(absWtPath)
		if evalWt == evalWtPath {
			foundWt = &wt
			break
		}
	}

	if foundWt == nil {
		t.Fatal("Created worktree not found in list")
	}

	// Verify WorktreeInfo fields
	if foundWt.Branch != branchName {
		t.Errorf("Expected branch %s, got %s", branchName, foundWt.Branch)
	}

	if foundWt.Commit == "" {
		t.Error("Commit should not be empty")
	}

	// Compare paths with symlink resolution
	evalFoundPath, _ := filepath.EvalSymlinks(foundWt.Path)
	evalWtPath, _ := filepath.EvalSymlinks(wtPath)
	if evalFoundPath != evalWtPath {
		t.Errorf("Expected path %s, got %s", evalWtPath, evalFoundPath)
	}
}

func TestMultipleWorktreesFromSameBranch(t *testing.T) {
	repoPath, cleanup := createTestRepo(t)
	defer cleanup()

	manager := NewManager(repoPath)

	// Create a test branch and check it out in first worktree
	createBranch(t, repoPath, "test-branch")
	wt1Path := filepath.Join(repoPath, "wt1")
	if err := manager.Create(wt1Path, "test-branch"); err != nil {
		t.Fatalf("Failed to create wt1: %v", err)
	}

	// Try to create second worktree from same branch (should fail - branch is checked out)
	wt2Path := filepath.Join(repoPath, "wt2")
	err := manager.Create(wt2Path, "test-branch")
	if err == nil {
		t.Error("Should not be able to create multiple worktrees from same branch")
	}
}

func TestWorktreeWithExistingBranch(t *testing.T) {
	repoPath, cleanup := createTestRepo(t)
	defer cleanup()

	manager := NewManager(repoPath)

	// Create a branch
	branchName := "existing-branch"
	createBranch(t, repoPath, branchName)

	// Create worktree from existing branch
	wtPath := filepath.Join(repoPath, "wt-existing")
	if err := manager.Create(wtPath, branchName); err != nil {
		t.Fatalf("Failed to create worktree from existing branch: %v", err)
	}

	// Verify correct branch is checked out
	currentBranch, err := GetCurrentBranch(wtPath)
	if err != nil {
		t.Fatalf("Failed to get current branch: %v", err)
	}
	if currentBranch != branchName {
		t.Errorf("Expected branch %s, got %s", branchName, currentBranch)
	}
}

func TestConcurrentWorktreeOperations(t *testing.T) {
	repoPath, cleanup := createTestRepo(t)
	defer cleanup()

	manager := NewManager(repoPath)

	// Use fewer concurrent operations to reduce git race condition likelihood
	// Git worktree operations access shared lock files and .git/worktrees/ structure
	const numWorktrees = 3

	// Create multiple branches
	for i := 0; i < numWorktrees; i++ {
		createBranch(t, repoPath, fmt.Sprintf("branch-%d", i))
	}

	// Create worktrees with staggered starts and retry logic to handle
	// transient git race conditions (e.g., "failed to read .git/worktrees/*/commondir")
	done := make(chan error, numWorktrees)
	for i := 0; i < numWorktrees; i++ {
		i := i // capture loop variable
		go func() {
			wtPath := filepath.Join(repoPath, fmt.Sprintf("wt-%d", i))
			branchName := fmt.Sprintf("branch-%d", i)

			// Retry with exponential backoff for transient git race conditions
			var lastErr error
			for attempt := 0; attempt < 5; attempt++ {
				if attempt > 0 {
					// Exponential backoff: 50ms, 100ms, 200ms, 400ms
					backoff := time.Duration(50<<attempt) * time.Millisecond
					time.Sleep(backoff)
				}
				lastErr = manager.Create(wtPath, branchName)
				if lastErr == nil {
					done <- nil
					return
				}
				// Only retry on race condition errors, not on permanent failures
				if !strings.Contains(lastErr.Error(), "commondir") &&
					!strings.Contains(lastErr.Error(), "index.lock") {
					break
				}
			}
			done <- lastErr
		}()
	}

	// Wait for all to complete
	for i := 0; i < numWorktrees; i++ {
		if err := <-done; err != nil {
			t.Errorf("Failed to create worktree: %v", err)
		}
	}

	// Verify all worktrees were created
	worktrees, err := manager.List()
	if err != nil {
		t.Fatalf("Failed to list worktrees: %v", err)
	}

	// Should have at least numWorktrees+1 worktrees (main repo + created ones)
	expectedMin := numWorktrees + 1
	if len(worktrees) < expectedMin {
		t.Errorf("Expected at least %d worktrees, got %d", expectedMin, len(worktrees))
	}
}

func TestWorktreeErrorHandling(t *testing.T) {
	// Test with non-existent repo
	manager := NewManager("/nonexistent/repo")

	err := manager.Create("/tmp/wt", "main")
	if err == nil {
		t.Error("Should fail when creating worktree in non-existent repo")
	}

	_, err = manager.List()
	if err == nil {
		t.Error("Should fail when listing worktrees in non-existent repo")
	}

	// Test with valid repo but invalid branch
	repoPath, cleanup := createTestRepo(t)
	defer cleanup()

	manager = NewManager(repoPath)
	wtPath := filepath.Join(repoPath, "wt-invalid")

	err = manager.Create(wtPath, "nonexistent-branch")
	if err == nil {
		t.Error("Should fail when creating worktree from non-existent branch")
	}
}

func TestBranchExists(t *testing.T) {
	repoPath, cleanup := createTestRepo(t)
	defer cleanup()

	manager := NewManager(repoPath)

	// main branch should exist (created in createTestRepo)
	exists, err := manager.BranchExists("main")
	if err != nil {
		t.Fatalf("Failed to check branch existence: %v", err)
	}
	if !exists {
		t.Error("main branch should exist")
	}

	// non-existent branch should not exist
	exists, err = manager.BranchExists("nonexistent-branch")
	if err != nil {
		t.Fatalf("Failed to check branch existence: %v", err)
	}
	if exists {
		t.Error("nonexistent-branch should not exist")
	}

	// Create a new branch and verify it exists
	createBranch(t, repoPath, "test-branch")
	exists, err = manager.BranchExists("test-branch")
	if err != nil {
		t.Fatalf("Failed to check branch existence: %v", err)
	}
	if !exists {
		t.Error("test-branch should exist after creation")
	}
}

func TestRenameBranch(t *testing.T) {
	repoPath, cleanup := createTestRepo(t)
	defer cleanup()

	manager := NewManager(repoPath)

	// Create a branch to rename
	createBranch(t, repoPath, "old-name")

	// Verify old name exists
	exists, err := manager.BranchExists("old-name")
	if err != nil {
		t.Fatalf("Failed to check branch existence: %v", err)
	}
	if !exists {
		t.Error("old-name branch should exist")
	}

	// Rename the branch
	if err := manager.RenameBranch("old-name", "new-name"); err != nil {
		t.Fatalf("Failed to rename branch: %v", err)
	}

	// Verify old name no longer exists
	exists, err = manager.BranchExists("old-name")
	if err != nil {
		t.Fatalf("Failed to check branch existence: %v", err)
	}
	if exists {
		t.Error("old-name branch should not exist after rename")
	}

	// Verify new name exists
	exists, err = manager.BranchExists("new-name")
	if err != nil {
		t.Fatalf("Failed to check branch existence: %v", err)
	}
	if !exists {
		t.Error("new-name branch should exist after rename")
	}
}

func TestMigrateLegacyWorkspaceBranch(t *testing.T) {
	t.Run("no legacy branch", func(t *testing.T) {
		repoPath, cleanup := createTestRepo(t)
		defer cleanup()

		manager := NewManager(repoPath)

		// No legacy workspace branch exists
		migrated, err := manager.MigrateLegacyWorkspaceBranch()
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if migrated {
			t.Error("Should not have migrated when no legacy branch exists")
		}
	})

	t.Run("legacy branch exists", func(t *testing.T) {
		repoPath, cleanup := createTestRepo(t)
		defer cleanup()

		manager := NewManager(repoPath)

		// Create legacy "workspace" branch
		createBranch(t, repoPath, "workspace")

		// Migrate should succeed
		migrated, err := manager.MigrateLegacyWorkspaceBranch()
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !migrated {
			t.Error("Should have migrated legacy branch")
		}

		// Verify old branch no longer exists
		exists, _ := manager.BranchExists("workspace")
		if exists {
			t.Error("Legacy 'workspace' branch should not exist after migration")
		}

		// Verify new branch exists
		exists, _ = manager.BranchExists("workspace/default")
		if !exists {
			t.Error("'workspace/default' branch should exist after migration")
		}
	})

	t.Run("both branches exist - conflict", func(t *testing.T) {
		repoPath, cleanup := createTestRepo(t)
		defer cleanup()

		manager := NewManager(repoPath)

		// Note: git prevents creating "workspace/default" when "workspace" exists and vice versa.
		// So this test simulates the conflict by creating workspace/default first,
		// then manually creating a "workspace" ref file to simulate a corrupt state.
		createBranch(t, repoPath, "workspace/default")

		// Manually create a corrupt "workspace" ref (this shouldn't happen in practice
		// but tests our detection logic)
		workspaceRefPath := filepath.Join(repoPath, ".git", "refs", "heads", "workspace")
		mainRef, _ := os.ReadFile(filepath.Join(repoPath, ".git", "refs", "heads", "main"))
		if err := os.WriteFile(workspaceRefPath, mainRef, 0644); err != nil {
			t.Skipf("Cannot create corrupt ref for testing: %v", err)
		}

		// Migrate should fail with conflict error
		_, err := manager.MigrateLegacyWorkspaceBranch()
		if err == nil {
			t.Error("Should have returned error when both branches exist")
		}
		if !strings.Contains(err.Error(), "manual resolution required") {
			t.Errorf("Error should mention manual resolution: %v", err)
		}
	})

	t.Run("workspace/default already exists", func(t *testing.T) {
		repoPath, cleanup := createTestRepo(t)
		defer cleanup()

		manager := NewManager(repoPath)

		// Only create "workspace/default" (no legacy branch)
		createBranch(t, repoPath, "workspace/default")

		// Migrate should return false (no migration needed)
		migrated, err := manager.MigrateLegacyWorkspaceBranch()
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if migrated {
			t.Error("Should not have migrated when only new branch exists")
		}
	})
}

func TestCanCreateBranchWithPrefix(t *testing.T) {
	repoPath, cleanup := createTestRepo(t)
	defer cleanup()

	manager := NewManager(repoPath)

	// Should be able to create workspace/foo when no "workspace" branch exists
	canCreate, conflictingBranch, err := manager.CanCreateBranchWithPrefix("workspace")
	if err != nil {
		t.Fatalf("Failed to check prefix: %v", err)
	}
	if !canCreate {
		t.Error("Should be able to create workspace/* branches when 'workspace' doesn't exist")
	}
	if conflictingBranch != "" {
		t.Errorf("Conflicting branch should be empty, got: %s", conflictingBranch)
	}

	// Create legacy "workspace" branch
	createBranch(t, repoPath, "workspace")

	// Now should NOT be able to create workspace/foo
	canCreate, conflictingBranch, err = manager.CanCreateBranchWithPrefix("workspace")
	if err != nil {
		t.Fatalf("Failed to check prefix: %v", err)
	}
	if canCreate {
		t.Error("Should NOT be able to create workspace/* branches when 'workspace' exists")
	}
	if conflictingBranch != "workspace" {
		t.Errorf("Conflicting branch should be 'workspace', got: %s", conflictingBranch)
	}
}

func TestCheckWorkspaceBranchConflict(t *testing.T) {
	t.Run("no conflict", func(t *testing.T) {
		repoPath, cleanup := createTestRepo(t)
		defer cleanup()

		manager := NewManager(repoPath)

		hasConflict, suggestion, err := manager.CheckWorkspaceBranchConflict()
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if hasConflict {
			t.Error("Should not have conflict when 'workspace' branch doesn't exist")
		}
		if suggestion != "" {
			t.Error("Suggestion should be empty when no conflict")
		}
	})

	t.Run("conflict exists", func(t *testing.T) {
		repoPath, cleanup := createTestRepo(t)
		defer cleanup()

		manager := NewManager(repoPath)

		// Create legacy "workspace" branch
		createBranch(t, repoPath, "workspace")

		hasConflict, suggestion, err := manager.CheckWorkspaceBranchConflict()
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !hasConflict {
			t.Error("Should have conflict when 'workspace' branch exists")
		}
		if !strings.Contains(suggestion, "legacy 'workspace' branch exists") {
			t.Errorf("Suggestion should explain the conflict: %s", suggestion)
		}
		if !strings.Contains(suggestion, "git branch -m workspace workspace/default") {
			t.Errorf("Suggestion should include migration command: %s", suggestion)
		}
	})
}

func TestGetUpstreamRemote(t *testing.T) {
	t.Run("no remotes", func(t *testing.T) {
		repoPath, cleanup := createTestRepo(t)
		defer cleanup()

		manager := NewManager(repoPath)
		_, err := manager.GetUpstreamRemote()
		if err == nil {
			t.Error("Expected error when no remotes exist")
		}
	})

	t.Run("origin only", func(t *testing.T) {
		repoPath, cleanup := createTestRepo(t)
		defer cleanup()

		// Add origin remote (pointing to local path for simplicity)
		cmd := exec.Command("git", "remote", "add", "origin", repoPath)
		cmd.Dir = repoPath
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to add origin remote: %v", err)
		}

		manager := NewManager(repoPath)
		remote, err := manager.GetUpstreamRemote()
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if remote != "origin" {
			t.Errorf("Expected 'origin', got %s", remote)
		}
	})

	t.Run("upstream preferred over origin", func(t *testing.T) {
		repoPath, cleanup := createTestRepo(t)
		defer cleanup()

		// Add both remotes
		cmd := exec.Command("git", "remote", "add", "origin", repoPath)
		cmd.Dir = repoPath
		cmd.Run()

		cmd = exec.Command("git", "remote", "add", "upstream", repoPath)
		cmd.Dir = repoPath
		cmd.Run()

		manager := NewManager(repoPath)
		remote, err := manager.GetUpstreamRemote()
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if remote != "upstream" {
			t.Errorf("Expected 'upstream', got %s", remote)
		}
	})
}

func TestGetDefaultBranch(t *testing.T) {
	t.Run("detects main branch", func(t *testing.T) {
		repoPath, cleanup := createTestRepo(t)
		defer cleanup()

		// Add origin remote and fetch
		cmd := exec.Command("git", "remote", "add", "origin", repoPath)
		cmd.Dir = repoPath
		cmd.Run()

		cmd = exec.Command("git", "fetch", "origin")
		cmd.Dir = repoPath
		cmd.Run()

		manager := NewManager(repoPath)
		branch, err := manager.GetDefaultBranch("origin")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if branch != "main" {
			t.Errorf("Expected 'main', got %s", branch)
		}
	})
}

func TestFindMergedUpstreamBranches(t *testing.T) {
	t.Run("finds merged branches", func(t *testing.T) {
		repoPath, cleanup := createTestRepo(t)
		defer cleanup()

		manager := NewManager(repoPath)

		// Create a branch that is already merged (same as main)
		createBranch(t, repoPath, "work/test-feature")

		// Add origin remote
		cmd := exec.Command("git", "remote", "add", "origin", repoPath)
		cmd.Dir = repoPath
		cmd.Run()

		cmd = exec.Command("git", "fetch", "origin")
		cmd.Dir = repoPath
		cmd.Run()

		// Find merged branches
		merged, err := manager.FindMergedUpstreamBranches("work/")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// The branch should be found since it's at the same commit as main
		found := false
		for _, b := range merged {
			if b == "work/test-feature" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected to find work/test-feature in merged branches, got: %v", merged)
		}
	})

	t.Run("excludes unmerged branches", func(t *testing.T) {
		repoPath, cleanup := createTestRepo(t)
		defer cleanup()

		manager := NewManager(repoPath)

		// Create a branch and add a commit to it
		cmd := exec.Command("git", "checkout", "-b", "work/unmerged-feature")
		cmd.Dir = repoPath
		cmd.Run()

		// Add a file and commit
		testFile := filepath.Join(repoPath, "newfile.txt")
		os.WriteFile(testFile, []byte("new content"), 0644)

		cmd = exec.Command("git", "add", "newfile.txt")
		cmd.Dir = repoPath
		cmd.Run()

		cmd = exec.Command("git", "commit", "-m", "Add new file")
		cmd.Dir = repoPath
		cmd.Run()

		// Go back to main
		cmd = exec.Command("git", "checkout", "main")
		cmd.Dir = repoPath
		cmd.Run()

		// Add origin remote
		cmd = exec.Command("git", "remote", "add", "origin", repoPath)
		cmd.Dir = repoPath
		cmd.Run()

		cmd = exec.Command("git", "fetch", "origin")
		cmd.Dir = repoPath
		cmd.Run()

		// Find merged branches
		merged, err := manager.FindMergedUpstreamBranches("work/")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// The unmerged branch should NOT be found
		for _, b := range merged {
			if b == "work/unmerged-feature" {
				t.Error("Unmerged branch should not be in the merged list")
			}
		}
	})

	t.Run("respects prefix filter", func(t *testing.T) {
		repoPath, cleanup := createTestRepo(t)
		defer cleanup()

		manager := NewManager(repoPath)

		// Create branches with different prefixes
		createBranch(t, repoPath, "work/test")
		createBranch(t, repoPath, "multiclaude/test")
		createBranch(t, repoPath, "feature/test")

		// Add origin remote
		cmd := exec.Command("git", "remote", "add", "origin", repoPath)
		cmd.Dir = repoPath
		cmd.Run()

		cmd = exec.Command("git", "fetch", "origin")
		cmd.Dir = repoPath
		cmd.Run()

		// Find merged branches with work/ prefix
		merged, err := manager.FindMergedUpstreamBranches("work/")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// Should only find work/test
		for _, b := range merged {
			if !strings.HasPrefix(b, "work/") {
				t.Errorf("Branch %s should not be included (wrong prefix)", b)
			}
		}
	})
}

func TestCleanupMergedBranches(t *testing.T) {
	t.Run("deletes merged branches", func(t *testing.T) {
		repoPath, cleanup := createTestRepo(t)
		defer cleanup()

		manager := NewManager(repoPath)

		// Create a merged branch
		createBranch(t, repoPath, "work/merged-test")

		// Verify branch exists
		exists, _ := manager.BranchExists("work/merged-test")
		if !exists {
			t.Fatal("Branch should exist before cleanup")
		}

		// Add origin remote
		cmd := exec.Command("git", "remote", "add", "origin", repoPath)
		cmd.Dir = repoPath
		cmd.Run()

		cmd = exec.Command("git", "fetch", "origin")
		cmd.Dir = repoPath
		cmd.Run()

		// Clean up merged branches
		deleted, err := manager.CleanupMergedBranches("work/", false)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(deleted) == 0 {
			t.Error("Expected at least one branch to be deleted")
		}

		// Verify branch is deleted
		exists, _ = manager.BranchExists("work/merged-test")
		if exists {
			t.Error("Branch should be deleted after cleanup")
		}
	})

	t.Run("skips branches in active worktrees", func(t *testing.T) {
		repoPath, cleanup := createTestRepo(t)
		defer cleanup()

		manager := NewManager(repoPath)

		// Create a branch and a worktree for it
		createBranch(t, repoPath, "work/active-branch")

		wtPath := filepath.Join(repoPath, "worktrees", "active")
		os.MkdirAll(filepath.Dir(wtPath), 0755)

		err := manager.Create(wtPath, "work/active-branch")
		if err != nil {
			t.Fatalf("Failed to create worktree: %v", err)
		}
		defer manager.Remove(wtPath, true)

		// Add origin remote
		cmd := exec.Command("git", "remote", "add", "origin", repoPath)
		cmd.Dir = repoPath
		cmd.Run()

		cmd = exec.Command("git", "fetch", "origin")
		cmd.Dir = repoPath
		cmd.Run()

		// Clean up merged branches
		deleted, err := manager.CleanupMergedBranches("work/", false)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// The active branch should NOT be deleted
		for _, b := range deleted {
			if b == "work/active-branch" {
				t.Error("Active branch should not be deleted")
			}
		}

		// Verify branch still exists
		exists, _ := manager.BranchExists("work/active-branch")
		if !exists {
			t.Error("Active branch should still exist")
		}
	})
}
