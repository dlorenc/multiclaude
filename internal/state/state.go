package state

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// AgentType represents the type of agent
type AgentType string

const (
	AgentTypeSupervisor AgentType = "supervisor"
	AgentTypeWorker     AgentType = "worker"
	AgentTypeMergeQueue AgentType = "merge-queue"
	AgentTypeWorkspace  AgentType = "workspace"
)

// Agent represents an agent's state
type Agent struct {
	Type            AgentType `json:"type"`
	WorktreePath    string    `json:"worktree_path"`
	TmuxWindow      string    `json:"tmux_window"`
	SessionID       string    `json:"session_id"`
	PID             int       `json:"pid"`
	Task            string    `json:"task,omitempty"`             // Only for workers
	CreatedAt       time.Time `json:"created_at"`
	LastNudge       time.Time `json:"last_nudge,omitempty"`
	ReadyForCleanup bool      `json:"ready_for_cleanup,omitempty"` // Only for workers
}

// Repository represents a tracked repository's state
type Repository struct {
	GithubURL    string           `json:"github_url"`
	TmuxSession  string           `json:"tmux_session"`
	Agents       map[string]Agent `json:"agents"`
}

// State represents the entire daemon state
type State struct {
	Repos map[string]*Repository `json:"repos"`
	mu    sync.RWMutex
	path  string
}

// New creates a new empty state
func New(path string) *State {
	return &State{
		Repos: make(map[string]*Repository),
		path:  path,
	}
}

// Load loads state from disk
func Load(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// No state file, return empty state
			return New(path), nil
		}
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("failed to parse state file: %w", err)
	}

	s.path = path

	// Initialize map if nil
	if s.Repos == nil {
		s.Repos = make(map[string]*Repository)
	}

	return &s, nil
}

// Save persists state to disk
func (s *State) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// Write to temp file first, then rename for atomicity
	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("failed to rename state file: %w", err)
	}

	return nil
}

// AddRepo adds a new repository to the state
func (s *State) AddRepo(name string, repo *Repository) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.Repos[name]; exists {
		return fmt.Errorf("repository %q already exists", name)
	}

	if repo.Agents == nil {
		repo.Agents = make(map[string]Agent)
	}

	s.Repos[name] = repo
	return s.saveUnlocked()
}

// GetRepo returns a repository by name
func (s *State) GetRepo(name string) (*Repository, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	repo, exists := s.Repos[name]
	return repo, exists
}

// ListRepos returns all repository names
func (s *State) ListRepos() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	repos := make([]string, 0, len(s.Repos))
	for name := range s.Repos {
		repos = append(repos, name)
	}
	return repos
}

// GetAllRepos returns a snapshot of all repositories
// This is safe for iteration and won't cause concurrent map access issues
func (s *State) GetAllRepos() map[string]*Repository {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Create a deep copy to avoid concurrent access issues
	repos := make(map[string]*Repository, len(s.Repos))
	for name, repo := range s.Repos {
		// Copy the repository
		repoCopy := &Repository{
			GithubURL:   repo.GithubURL,
			TmuxSession: repo.TmuxSession,
			Agents:      make(map[string]Agent, len(repo.Agents)),
		}
		// Copy agents
		for agentName, agent := range repo.Agents {
			repoCopy.Agents[agentName] = agent
		}
		repos[name] = repoCopy
	}
	return repos
}

// AddAgent adds a new agent to a repository
func (s *State) AddAgent(repoName, agentName string, agent Agent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	repo, exists := s.Repos[repoName]
	if !exists {
		return fmt.Errorf("repository %q not found", repoName)
	}

	if _, exists := repo.Agents[agentName]; exists {
		return fmt.Errorf("agent %q already exists in repository %q", agentName, repoName)
	}

	repo.Agents[agentName] = agent
	return s.saveUnlocked()
}

// UpdateAgent updates an existing agent
func (s *State) UpdateAgent(repoName, agentName string, agent Agent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	repo, exists := s.Repos[repoName]
	if !exists {
		return fmt.Errorf("repository %q not found", repoName)
	}

	if _, exists := repo.Agents[agentName]; !exists {
		return fmt.Errorf("agent %q not found in repository %q", agentName, repoName)
	}

	repo.Agents[agentName] = agent
	return s.saveUnlocked()
}

// RemoveAgent removes an agent from a repository
func (s *State) RemoveAgent(repoName, agentName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	repo, exists := s.Repos[repoName]
	if !exists {
		return fmt.Errorf("repository %q not found", repoName)
	}

	delete(repo.Agents, agentName)
	return s.saveUnlocked()
}

// GetAgent returns an agent by name
func (s *State) GetAgent(repoName, agentName string) (Agent, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	repo, exists := s.Repos[repoName]
	if !exists {
		return Agent{}, false
	}

	agent, exists := repo.Agents[agentName]
	return agent, exists
}

// ListAgents returns all agent names for a repository
func (s *State) ListAgents(repoName string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	repo, exists := s.Repos[repoName]
	if !exists {
		return nil, fmt.Errorf("repository %q not found", repoName)
	}

	agents := make([]string, 0, len(repo.Agents))
	for name := range repo.Agents {
		agents = append(agents, name)
	}
	return agents, nil
}

// saveUnlocked saves state without acquiring lock (caller must hold lock)
func (s *State) saveUnlocked() error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("failed to rename state file: %w", err)
	}

	return nil
}
