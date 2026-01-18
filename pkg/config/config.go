//go:generate go run ../../cmd/generate-docs

package config

import (
	"os"
	"path/filepath"
)

// Paths holds all the directory and file paths used by multiclaude
type Paths struct {
	Root         string // $HOME/.multiclaude/
	DaemonPID    string // daemon.pid
	DaemonSock   string // daemon.sock
	DaemonLog    string // daemon.log
	StateFile    string // state.json
	ReposDir     string // repos/
	WorktreesDir string // wts/
	MessagesDir  string // messages/
}

// DefaultPaths returns the default paths for multiclaude
func DefaultPaths() (*Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	root := filepath.Join(home, ".multiclaude")

	return &Paths{
		Root:         root,
		DaemonPID:    filepath.Join(root, "daemon.pid"),
		DaemonSock:   filepath.Join(root, "daemon.sock"),
		DaemonLog:    filepath.Join(root, "daemon.log"),
		StateFile:    filepath.Join(root, "state.json"),
		ReposDir:     filepath.Join(root, "repos"),
		WorktreesDir: filepath.Join(root, "wts"),
		MessagesDir:  filepath.Join(root, "messages"),
	}, nil
}

// EnsureDirectories creates all necessary directories if they don't exist
func (p *Paths) EnsureDirectories() error {
	dirs := []string{
		p.Root,
		p.ReposDir,
		p.WorktreesDir,
		p.MessagesDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	return nil
}

// RepoDir returns the path for a specific repository
func (p *Paths) RepoDir(repoName string) string {
	return filepath.Join(p.ReposDir, repoName)
}

// WorktreeDir returns the path for a repository's worktrees
func (p *Paths) WorktreeDir(repoName string) string {
	return filepath.Join(p.WorktreesDir, repoName)
}

// AgentWorktree returns the path for a specific agent's worktree
func (p *Paths) AgentWorktree(repoName, agentName string) string {
	return filepath.Join(p.WorktreeDir(repoName), agentName)
}

// MessagesDir returns the path for a repository's messages
func (p *Paths) RepoMessagesDir(repoName string) string {
	return filepath.Join(p.MessagesDir, repoName)
}

// AgentMessagesDir returns the path for a specific agent's messages
func (p *Paths) AgentMessagesDir(repoName, agentName string) string {
	return filepath.Join(p.RepoMessagesDir(repoName), agentName)
}
