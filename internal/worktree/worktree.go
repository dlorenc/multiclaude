package worktree

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Manager handles git worktree operations
type Manager struct {
	repoPath string
}

// NewManager creates a new worktree manager for a repository
func NewManager(repoPath string) *Manager {
	return &Manager{repoPath: repoPath}
}

// Create creates a new git worktree
func (m *Manager) Create(path, branch string) error {
	cmd := exec.Command("git", "worktree", "add", path, branch)
	cmd.Dir = m.repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create worktree: %w\nOutput: %s", err, output)
	}
	return nil
}

// CreateNewBranch creates a new worktree with a new branch
func (m *Manager) CreateNewBranch(path, newBranch, startPoint string) error {
	cmd := exec.Command("git", "worktree", "add", "-b", newBranch, path, startPoint)
	cmd.Dir = m.repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create worktree with new branch: %w\nOutput: %s", err, output)
	}
	return nil
}

// Remove removes a git worktree
func (m *Manager) Remove(path string, force bool) error {
	args := []string{"worktree", "remove", path}
	if force {
		args = append(args, "--force")
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = m.repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to remove worktree: %w\nOutput: %s", err, output)
	}
	return nil
}

// List returns a list of all worktrees
func (m *Manager) List() ([]WorktreeInfo, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = m.repoPath
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}

	return parseWorktreeList(string(output)), nil
}

// Exists checks if a worktree exists at the given path
func (m *Manager) Exists(path string) (bool, error) {
	worktrees, err := m.List()
	if err != nil {
		return false, err
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return false, err
	}
	// Resolve symlinks for accurate comparison (important on macOS)
	evalPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		// Path might not exist yet, use absPath
		evalPath = absPath
	}

	for _, wt := range worktrees {
		wtAbs, err := filepath.Abs(wt.Path)
		if err != nil {
			continue
		}
		wtEval, err := filepath.EvalSymlinks(wtAbs)
		if err != nil {
			wtEval = wtAbs
		}
		if wtEval == evalPath {
			return true, nil
		}
	}

	return false, nil
}

// Prune removes worktree information for missing paths
func (m *Manager) Prune() error {
	cmd := exec.Command("git", "worktree", "prune")
	cmd.Dir = m.repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to prune worktrees: %w\nOutput: %s", err, output)
	}
	return nil
}

// HasUncommittedChanges checks if a worktree has uncommitted changes
func HasUncommittedChanges(path string) (bool, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to check git status: %w", err)
	}

	return len(strings.TrimSpace(string(output))) > 0, nil
}

// HasUnpushedCommits checks if a worktree has unpushed commits
func HasUnpushedCommits(path string) (bool, error) {
	// First check if there's a tracking branch
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	cmd.Dir = path
	if err := cmd.Run(); err != nil {
		// No tracking branch, so no unpushed commits
		return false, nil
	}

	// Check for commits ahead of upstream
	cmd = exec.Command("git", "rev-list", "--count", "@{u}..")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to check unpushed commits: %w", err)
	}

	count := strings.TrimSpace(string(output))
	return count != "0", nil
}

// GetCurrentBranch returns the current branch name for a worktree
func GetCurrentBranch(path string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// WorktreeInfo contains information about a worktree
type WorktreeInfo struct {
	Path   string
	Commit string
	Branch string
}

// parseWorktreeList parses the output of `git worktree list --porcelain`
func parseWorktreeList(output string) []WorktreeInfo {
	var worktrees []WorktreeInfo
	var current WorktreeInfo

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if current.Path != "" {
				worktrees = append(worktrees, current)
				current = WorktreeInfo{}
			}
			continue
		}

		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}

		switch parts[0] {
		case "worktree":
			current.Path = parts[1]
		case "HEAD":
			current.Commit = parts[1]
		case "branch":
			current.Branch = strings.TrimPrefix(parts[1], "refs/heads/")
		}
	}

	// Add last worktree if exists
	if current.Path != "" {
		worktrees = append(worktrees, current)
	}

	return worktrees
}

// BranchExists checks if a branch exists in the repository
func (m *Manager) BranchExists(branchName string) (bool, error) {
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+branchName)
	cmd.Dir = m.repoPath
	err := cmd.Run()
	if err != nil {
		// Exit code 1 means branch doesn't exist
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, fmt.Errorf("failed to check branch existence: %w", err)
	}
	return true, nil
}

// RenameBranch renames a branch from oldName to newName
func (m *Manager) RenameBranch(oldName, newName string) error {
	cmd := exec.Command("git", "branch", "-m", oldName, newName)
	cmd.Dir = m.repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to rename branch: %w\nOutput: %s", err, output)
	}
	return nil
}

// DeleteBranch force deletes a branch (git branch -D)
func (m *Manager) DeleteBranch(branchName string) error {
	cmd := exec.Command("git", "branch", "-D", branchName)
	cmd.Dir = m.repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete branch: %w\nOutput: %s", err, output)
	}
	return nil
}

// ListBranchesWithPrefix lists all branches that start with the given prefix
func (m *Manager) ListBranchesWithPrefix(prefix string) ([]string, error) {
	cmd := exec.Command("git", "for-each-ref", "--format=%(refname:short)", "refs/heads/"+prefix)
	cmd.Dir = m.repoPath
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list branches: %w", err)
	}

	var branches []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line != "" {
			branches = append(branches, line)
		}
	}
	return branches, nil
}

// FindOrphanedBranches finds branches with the given prefix that don't have corresponding worktrees
func (m *Manager) FindOrphanedBranches(prefix string) ([]string, error) {
	// Get all branches with the prefix
	branches, err := m.ListBranchesWithPrefix(prefix)
	if err != nil {
		return nil, err
	}

	// Get all worktrees
	worktrees, err := m.List()
	if err != nil {
		return nil, err
	}

	// Build a set of branches that have worktrees
	activeBranches := make(map[string]bool)
	for _, wt := range worktrees {
		if wt.Branch != "" {
			activeBranches[wt.Branch] = true
		}
	}

	// Find orphaned branches
	var orphaned []string
	for _, branch := range branches {
		if !activeBranches[branch] {
			orphaned = append(orphaned, branch)
		}
	}

	return orphaned, nil
}

// CanCreateBranchWithPrefix checks if a branch can be created with a given prefix.
// Returns false if there's a conflicting branch (e.g., "workspace" exists and
// we're trying to create "workspace/foo").
func (m *Manager) CanCreateBranchWithPrefix(prefix string) (bool, string, error) {
	// Check if a branch with the exact prefix name exists (e.g., "workspace")
	// This would prevent creating "workspace/foo" due to git ref limitations
	exists, err := m.BranchExists(prefix)
	if err != nil {
		return false, "", err
	}
	if exists {
		return false, prefix, nil
	}
	return true, "", nil
}

// MigrateLegacyWorkspaceBranch checks for a legacy "workspace" branch and renames it
// to "workspace/default" to allow the new workspace/<name> naming convention.
// Returns:
//   - migrated: true if migration was performed
//   - error: any error that occurred
func (m *Manager) MigrateLegacyWorkspaceBranch() (bool, error) {
	// Check if legacy "workspace" branch exists
	legacyExists, err := m.BranchExists("workspace")
	if err != nil {
		return false, fmt.Errorf("failed to check for legacy workspace branch: %w", err)
	}

	if !legacyExists {
		// No legacy branch, nothing to migrate
		return false, nil
	}

	// Check if the new naming convention is already in use (workspace/default exists)
	newExists, err := m.BranchExists("workspace/default")
	if err != nil {
		return false, fmt.Errorf("failed to check for workspace/default branch: %w", err)
	}

	if newExists {
		// Both exist - this is a conflict state that shouldn't happen in normal usage
		return false, fmt.Errorf("both 'workspace' and 'workspace/default' branches exist; manual resolution required")
	}

	// Rename workspace -> workspace/default
	if err := m.RenameBranch("workspace", "workspace/default"); err != nil {
		return false, fmt.Errorf("failed to migrate workspace branch: %w", err)
	}

	return true, nil
}

// CheckWorkspaceBranchConflict checks if there's a potential conflict between
// legacy "workspace" branch and the new workspace/<name> naming convention.
// Returns:
//   - hasConflict: true if there's a blocking "workspace" branch
//   - suggestion: a suggested fix for the user
func (m *Manager) CheckWorkspaceBranchConflict() (bool, string, error) {
	exists, err := m.BranchExists("workspace")
	if err != nil {
		return false, "", err
	}

	if exists {
		suggestion := `A legacy 'workspace' branch exists which conflicts with the new workspace/<name> naming convention.

To fix this, you can either:
1. Let multiclaude migrate the branch automatically by running:
   cd ` + m.repoPath + ` && git branch -m workspace workspace/default

2. Or manually rename/delete the legacy branch:
   cd ` + m.repoPath + ` && git branch -m workspace <new-name>
   cd ` + m.repoPath + ` && git branch -d workspace`
		return true, suggestion, nil
	}

	return false, "", nil
}

// GetUpstreamRemote returns the name of the upstream remote, typically "upstream" or "origin"
// It prefers "upstream" if it exists, otherwise falls back to "origin"
func (m *Manager) GetUpstreamRemote() (string, error) {
	// Check if "upstream" remote exists
	cmd := exec.Command("git", "remote", "get-url", "upstream")
	cmd.Dir = m.repoPath
	if err := cmd.Run(); err == nil {
		return "upstream", nil
	}

	// Fall back to "origin"
	cmd = exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = m.repoPath
	if err := cmd.Run(); err == nil {
		return "origin", nil
	}

	return "", fmt.Errorf("no upstream or origin remote found")
}

// GetDefaultBranch returns the default branch name for a remote (e.g., "main" or "master")
func (m *Manager) GetDefaultBranch(remote string) (string, error) {
	// Try to get the default branch from the remote's HEAD
	cmd := exec.Command("git", "symbolic-ref", fmt.Sprintf("refs/remotes/%s/HEAD", remote))
	cmd.Dir = m.repoPath
	output, err := cmd.Output()
	if err == nil {
		// Output is like "refs/remotes/origin/main" - extract the branch name
		refPath := strings.TrimSpace(string(output))
		parts := strings.Split(refPath, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1], nil
		}
	}

	// Fallback: check for common branch names
	for _, branch := range []string{"main", "master"} {
		cmd := exec.Command("git", "rev-parse", "--verify", fmt.Sprintf("refs/remotes/%s/%s", remote, branch))
		cmd.Dir = m.repoPath
		if err := cmd.Run(); err == nil {
			return branch, nil
		}
	}

	return "", fmt.Errorf("could not determine default branch for remote %s", remote)
}

// FetchRemote fetches updates from a remote
func (m *Manager) FetchRemote(remote string) error {
	cmd := exec.Command("git", "fetch", remote)
	cmd.Dir = m.repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to fetch from %s: %w\nOutput: %s", remote, err, output)
	}
	return nil
}

// FindMergedUpstreamBranches finds local branches that have been merged into the upstream default branch.
// It fetches from the upstream remote first to ensure we have the latest state.
// The branchPrefix filters which branches to check (e.g., "multiclaude/" or "work/").
// Returns a list of branch names that can be safely deleted.
func (m *Manager) FindMergedUpstreamBranches(branchPrefix string) ([]string, error) {
	// Get the upstream remote name
	remote, err := m.GetUpstreamRemote()
	if err != nil {
		return nil, fmt.Errorf("failed to get upstream remote: %w", err)
	}

	// Fetch from upstream to get the latest state
	if err := m.FetchRemote(remote); err != nil {
		return nil, fmt.Errorf("failed to fetch from upstream: %w", err)
	}

	// Get the default branch name
	defaultBranch, err := m.GetDefaultBranch(remote)
	if err != nil {
		return nil, fmt.Errorf("failed to get default branch: %w", err)
	}

	// Get branches merged into upstream's default branch
	upstreamRef := fmt.Sprintf("%s/%s", remote, defaultBranch)
	cmd := exec.Command("git", "branch", "--merged", upstreamRef, "--format=%(refname:short)")
	cmd.Dir = m.repoPath
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list merged branches: %w", err)
	}

	// Filter branches by prefix
	var mergedBranches []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		branch := strings.TrimSpace(line)
		if branch == "" {
			continue
		}
		// Skip the default branches themselves
		if branch == "main" || branch == "master" {
			continue
		}
		// Only include branches matching the prefix
		if branchPrefix != "" && !strings.HasPrefix(branch, branchPrefix) {
			continue
		}
		mergedBranches = append(mergedBranches, branch)
	}

	return mergedBranches, nil
}

// DeleteRemoteBranch deletes a branch from a remote
func (m *Manager) DeleteRemoteBranch(remote, branchName string) error {
	cmd := exec.Command("git", "push", remote, "--delete", branchName)
	cmd.Dir = m.repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete remote branch: %w\nOutput: %s", err, output)
	}
	return nil
}

// CleanupMergedBranches finds and deletes local branches that have been merged upstream.
// If deleteRemote is true, it also deletes the corresponding remote branches from origin.
// Returns the list of deleted branch names.
func (m *Manager) CleanupMergedBranches(branchPrefix string, deleteRemote bool) ([]string, error) {
	// Find merged branches
	mergedBranches, err := m.FindMergedUpstreamBranches(branchPrefix)
	if err != nil {
		return nil, err
	}

	if len(mergedBranches) == 0 {
		return nil, nil
	}

	// Get worktrees to avoid deleting branches that are still checked out
	worktrees, err := m.List()
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}

	activeBranches := make(map[string]bool)
	for _, wt := range worktrees {
		if wt.Branch != "" {
			activeBranches[wt.Branch] = true
		}
	}

	var deleted []string
	for _, branch := range mergedBranches {
		// Skip branches that are currently checked out in worktrees
		if activeBranches[branch] {
			continue
		}

		// Delete local branch
		if err := m.DeleteBranch(branch); err != nil {
			// Log but continue with other branches
			continue
		}
		deleted = append(deleted, branch)

		// Delete remote branch if requested
		if deleteRemote {
			// Try to delete from origin (the fork)
			_ = m.DeleteRemoteBranch("origin", branch)
		}
	}

	return deleted, nil
}

// CleanupOrphaned removes worktree directories that exist on disk but not in git
func CleanupOrphaned(wtRootDir string, manager *Manager) ([]string, error) {
	// Get all worktrees from git
	gitWorktrees, err := manager.List()
	if err != nil {
		return nil, err
	}

	gitPaths := make(map[string]bool)
	for _, wt := range gitWorktrees {
		absPath, err := filepath.Abs(wt.Path)
		if err != nil {
			continue
		}
		// Resolve symlinks for accurate comparison (important on macOS)
		evalPath, err := filepath.EvalSymlinks(absPath)
		if err != nil {
			evalPath = absPath
		}
		gitPaths[evalPath] = true
	}

	// Find directories in wtRootDir that aren't in git worktrees
	var removed []string
	entries, err := os.ReadDir(wtRootDir)
	if err != nil {
		if os.IsNotExist(err) {
			return removed, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		path := filepath.Join(wtRootDir, entry.Name())
		absPath, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		// Resolve symlinks for accurate comparison (important on macOS)
		evalPath, err := filepath.EvalSymlinks(absPath)
		if err != nil {
			evalPath = absPath
		}

		if !gitPaths[evalPath] {
			// This is an orphaned directory
			if err := os.RemoveAll(path); err == nil {
				removed = append(removed, path)
			}
		}
	}

	return removed, nil
}
