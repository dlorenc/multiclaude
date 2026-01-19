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
	AgentTypeReview     AgentType = "review"
)

// TrackMode defines which PRs the merge queue should track
type TrackMode string

const (
	// TrackModeAll tracks all PRs (default)
	TrackModeAll TrackMode = "all"
	// TrackModeAuthor tracks only PRs where the multiclaude user is the author
	TrackModeAuthor TrackMode = "author"
	// TrackModeAssigned tracks only PRs where the multiclaude user is assigned
	TrackModeAssigned TrackMode = "assigned"
)

// ProviderType defines which CLI backend to use
type ProviderType string

const (
	// ProviderClaude uses the claude CLI (default)
	ProviderClaude ProviderType = "claude"
	// ProviderHappy uses the happy CLI from happy.engineering
	ProviderHappy ProviderType = "happy"
)

// ProviderConfig holds configuration for the CLI provider
type ProviderConfig struct {
	// Provider is the CLI backend to use: "claude" or "happy" (default: "claude")
	Provider ProviderType `json:"provider"`
}

// DefaultProviderConfig returns the default provider configuration
func DefaultProviderConfig() ProviderConfig {
	return ProviderConfig{
		Provider: ProviderClaude,
	}
}

// MergeQueueConfig holds configuration for the merge queue agent
type MergeQueueConfig struct {
	// Enabled determines whether the merge queue agent should run (default: true)
	Enabled bool `json:"enabled"`
	// TrackMode determines which PRs to track: "all", "author", or "assigned" (default: "all")
	TrackMode TrackMode `json:"track_mode"`
}

// DefaultMergeQueueConfig returns the default merge queue configuration
func DefaultMergeQueueConfig() MergeQueueConfig {
	return MergeQueueConfig{
		Enabled:   true,
		TrackMode: TrackModeAll,
	}
}

// Agent represents an agent's state
type Agent struct {
	Type            AgentType `json:"type"`
	WorktreePath    string    `json:"worktree_path"`
	TmuxWindow      string    `json:"tmux_window"`
	SessionID       string    `json:"session_id"`
	PID             int       `json:"pid"`
	Task            string    `json:"task,omitempty"` // Only for workers
	CreatedAt       time.Time `json:"created_at"`
	LastNudge       time.Time `json:"last_nudge,omitempty"`
	ReadyForCleanup bool      `json:"ready_for_cleanup,omitempty"` // Only for workers
}

// Repository represents a tracked repository's state
type Repository struct {
	GithubURL        string           `json:"github_url"`
	TmuxSession      string           `json:"tmux_session"`
	Agents           map[string]Agent `json:"agents"`
	MergeQueueConfig MergeQueueConfig `json:"merge_queue_config,omitempty"`
	ProviderConfig   ProviderConfig   `json:"provider_config,omitempty"`
}

// State represents the entire daemon state
type State struct {
	Repos       map[string]*Repository `json:"repos"`
	CurrentRepo string                 `json:"current_repo,omitempty"`
	mu          sync.RWMutex
	path        string
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

// RemoveRepo removes a repository from the state
func (s *State) RemoveRepo(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.Repos[name]; !exists {
		return fmt.Errorf("repository %q not found", name)
	}

	delete(s.Repos, name)
	return s.saveUnlocked()
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

// SetCurrentRepo sets the current/default repository
func (s *State) SetCurrentRepo(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Verify the repo exists
	if _, exists := s.Repos[name]; !exists {
		return fmt.Errorf("repository %q not found", name)
	}

	s.CurrentRepo = name
	return s.saveUnlocked()
}

// GetCurrentRepo returns the current/default repository name
func (s *State) GetCurrentRepo() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.CurrentRepo
}

// ClearCurrentRepo clears the current/default repository
func (s *State) ClearCurrentRepo() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.CurrentRepo = ""
	return s.saveUnlocked()
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
			GithubURL:        repo.GithubURL,
			TmuxSession:      repo.TmuxSession,
			Agents:           make(map[string]Agent, len(repo.Agents)),
			MergeQueueConfig: repo.MergeQueueConfig,
			ProviderConfig:   repo.ProviderConfig,
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

// GetMergeQueueConfig returns the merge queue config for a repository
func (s *State) GetMergeQueueConfig(repoName string) (MergeQueueConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	repo, exists := s.Repos[repoName]
	if !exists {
		return MergeQueueConfig{}, fmt.Errorf("repository %q not found", repoName)
	}

	// Return default config if not set (for backward compatibility)
	if repo.MergeQueueConfig.TrackMode == "" {
		return DefaultMergeQueueConfig(), nil
	}
	return repo.MergeQueueConfig, nil
}

// UpdateMergeQueueConfig updates the merge queue config for a repository
func (s *State) UpdateMergeQueueConfig(repoName string, config MergeQueueConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	repo, exists := s.Repos[repoName]
	if !exists {
		return fmt.Errorf("repository %q not found", repoName)
	}

	repo.MergeQueueConfig = config
	return s.saveUnlocked()
}

// GetProviderConfig returns the provider config for a repository
func (s *State) GetProviderConfig(repoName string) (ProviderConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	repo, exists := s.Repos[repoName]
	if !exists {
		return ProviderConfig{}, fmt.Errorf("repository %q not found", repoName)
	}

	// Return default config if not set (for backward compatibility)
	if repo.ProviderConfig.Provider == "" {
		return DefaultProviderConfig(), nil
	}
	return repo.ProviderConfig, nil
}

// UpdateProviderConfig updates the provider config for a repository
func (s *State) UpdateProviderConfig(repoName string, config ProviderConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	repo, exists := s.Repos[repoName]
	if !exists {
		return fmt.Errorf("repository %q not found", repoName)
	}

	repo.ProviderConfig = config
	return s.saveUnlocked()
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
