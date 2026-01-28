//go:generate go run ../../cmd/generate-docs

package config

import (
	"os"
	"path/filepath"
)

// Paths holds all the directory and file paths used by multiclaude
type Paths struct {
	Root            string // $HOME/.multiclaude/
	DaemonPID       string // daemon.pid
	DaemonSock      string // daemon.sock
	DaemonLog       string // daemon.log
	StateFile       string // state.json
	ReposDir        string // repos/
	WorktreesDir    string // wts/
	MessagesDir     string // messages/
	OutputDir       string // output/
	ClaudeConfigDir string // claude-config/
	ArchiveDir      string // archive/ (for paused work)
}

// DefaultPaths returns the default paths for multiclaude
func DefaultPaths() (*Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	root := filepath.Join(home, ".multiclaude")

	return &Paths{
		Root:            root,
		DaemonPID:       filepath.Join(root, "daemon.pid"),
		DaemonSock:      filepath.Join(root, "daemon.sock"),
		DaemonLog:       filepath.Join(root, "daemon.log"),
		StateFile:       filepath.Join(root, "state.json"),
		ReposDir:        filepath.Join(root, "repos"),
		WorktreesDir:    filepath.Join(root, "wts"),
		MessagesDir:     filepath.Join(root, "messages"),
		OutputDir:       filepath.Join(root, "output"),
		ClaudeConfigDir: filepath.Join(root, "claude-config"),
		ArchiveDir:      filepath.Join(root, "archive"),
	}, nil
}

// EnsureDirectories creates all necessary directories if they don't exist
func (p *Paths) EnsureDirectories() error {
	dirs := []string{
		p.Root,
		p.ReposDir,
		p.WorktreesDir,
		p.MessagesDir,
		p.OutputDir,
		p.ClaudeConfigDir,
		p.ArchiveDir,
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

// RepoAgentsDir returns the path for a repository's agent definitions
// These are the per-repo agent templates that define configurable agents
func (p *Paths) RepoAgentsDir(repoName string) string {
	return filepath.Join(p.ReposDir, repoName, "agents")
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

// RepoOutputDir returns the path for a repository's output logs
func (p *Paths) RepoOutputDir(repoName string) string {
	return filepath.Join(p.OutputDir, repoName)
}

// WorkersOutputDir returns the path for worker agent output logs
func (p *Paths) WorkersOutputDir(repoName string) string {
	return filepath.Join(p.RepoOutputDir(repoName), "workers")
}

// AgentLogFile returns the path to an agent's log file
func (p *Paths) AgentLogFile(repoName, agentName string, isWorker bool) string {
	if isWorker {
		return filepath.Join(p.WorkersOutputDir(repoName), agentName+".log")
	}
	return filepath.Join(p.RepoOutputDir(repoName), agentName+".log")
}

// AgentClaudeConfigDir returns the path for a specific agent's Claude config directory
// This is used to set CLAUDE_CONFIG_DIR for per-agent slash commands
func (p *Paths) AgentClaudeConfigDir(repoName, agentName string) string {
	return filepath.Join(p.ClaudeConfigDir, repoName, agentName)
}

// AgentCommandsDir returns the path for a specific agent's slash commands directory
func (p *Paths) AgentCommandsDir(repoName, agentName string) string {
	return filepath.Join(p.AgentClaudeConfigDir(repoName, agentName), "commands")
}

// NewTestPaths creates a Paths instance for testing with all paths under tmpDir.
// This eliminates duplicate test setup code and ensures consistent path configuration.
func NewTestPaths(tmpDir string) *Paths {
	return &Paths{
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
}

// RepoArchiveDir returns the path for a repository's archived work
func (p *Paths) RepoArchiveDir(repoName string) string {
	return filepath.Join(p.ArchiveDir, repoName)
}
