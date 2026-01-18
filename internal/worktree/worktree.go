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
