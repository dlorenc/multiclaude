package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AgentType represents the type of agent
type AgentType string

const (
	AgentTypeSupervisor        AgentType = "supervisor"
	AgentTypeWorker            AgentType = "worker"
	AgentTypeMergeQueue        AgentType = "merge-queue"
	AgentTypePRShepherd        AgentType = "pr-shepherd"
	AgentTypeWorkspace         AgentType = "workspace"
	AgentTypeReview            AgentType = "review"
	AgentTypeGenericPersistent AgentType = "generic-persistent"
)

// IsPersistent returns true if this agent type represents a persistent agent
// that should be auto-restarted when dead. Persistent agents include supervisor,
// merge-queue, pr-shepherd, workspace, and generic-persistent. Transient agents
// (worker, review) are not auto-restarted.
func (t AgentType) IsPersistent() bool {
	switch t {
	case AgentTypeSupervisor, AgentTypeMergeQueue, AgentTypePRShepherd, AgentTypeWorkspace, AgentTypeGenericPersistent:
		return true
	default:
		return false
	}
}

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

// ParseTrackMode parses a string into a TrackMode.
// Returns an error if the string is not a valid track mode.
func ParseTrackMode(s string) (TrackMode, error) {
	switch s {
	case "all":
		return TrackModeAll, nil
	case "author":
		return TrackModeAuthor, nil
	case "assigned":
		return TrackModeAssigned, nil
	default:
		return "", fmt.Errorf("invalid track mode: %q (valid modes: all, author, assigned)", s)
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

// PRShepherdConfig holds configuration for the PR shepherd agent (used in fork mode)
type PRShepherdConfig struct {
	// Enabled determines whether the PR shepherd agent should run (default: true in fork mode)
	Enabled bool `json:"enabled"`
	// TrackMode determines which PRs to track: "all", "author", or "assigned" (default: "author")
	TrackMode TrackMode `json:"track_mode"`
}

// DefaultPRShepherdConfig returns the default PR shepherd configuration
func DefaultPRShepherdConfig() PRShepherdConfig {
	return PRShepherdConfig{
		Enabled:   true,
		TrackMode: TrackModeAuthor, // In fork mode, default to tracking only author's PRs
	}
}

// ForkConfig holds fork-related configuration for a repository
type ForkConfig struct {
	// IsFork is true if the repository is detected as a fork
	IsFork bool `json:"is_fork"`
	// UpstreamURL is the URL of the upstream repository (if fork)
	UpstreamURL string `json:"upstream_url,omitempty"`
	// UpstreamOwner is the owner of the upstream repository (if fork)
	UpstreamOwner string `json:"upstream_owner,omitempty"`
	// UpstreamRepo is the name of the upstream repository (if fork)
	UpstreamRepo string `json:"upstream_repo,omitempty"`
	// ForceForkMode forces fork mode even for non-forks (edge case)
	ForceForkMode bool `json:"force_fork_mode,omitempty"`
}

// TaskStatus represents the status of a completed task
type TaskStatus string

const (
	// TaskStatusOpen means a PR was created but not yet merged or closed
	TaskStatusOpen TaskStatus = "open"
	// TaskStatusMerged means the PR was merged
	TaskStatusMerged TaskStatus = "merged"
	// TaskStatusClosed means the PR was closed without merging
	TaskStatusClosed TaskStatus = "closed"
	// TaskStatusNoPR means no PR was created
	TaskStatusNoPR TaskStatus = "no-pr"
	// TaskStatusFailed means the task failed without creating a PR
	TaskStatusFailed TaskStatus = "failed"
	// TaskStatusUnknown means the status couldn't be determined
	TaskStatusUnknown TaskStatus = "unknown"
)

// TaskHistoryEntry represents a completed task in the history
type TaskHistoryEntry struct {
	Name          string     `json:"name"`                     // Worker name
	Task          string     `json:"task"`                     // Task description
	Branch        string     `json:"branch"`                   // Git branch
	PRURL         string     `json:"pr_url,omitempty"`         // Pull request URL if created
	PRNumber      int        `json:"pr_number,omitempty"`      // PR number for quick lookup
	Status        TaskStatus `json:"status"`                   // Current status
	Summary       string     `json:"summary,omitempty"`        // Brief summary of what was accomplished
	FailureReason string     `json:"failure_reason,omitempty"` // Why the task failed (if applicable)
	CreatedAt     time.Time  `json:"created_at"`               // When the task was started
	CompletedAt   time.Time  `json:"completed_at,omitempty"`   // When the task was completed
}

// Agent represents an agent's state
type Agent struct {
	Type            AgentType `json:"type"`
	WorktreePath    string    `json:"worktree_path"`
	TmuxWindow      string    `json:"tmux_window"`
	SessionID       string    `json:"session_id"`
	PID             int       `json:"pid"`
	Task            string    `json:"task,omitempty"`           // Only for workers
	Summary         string    `json:"summary,omitempty"`        // Brief summary of work done (workers only)
	FailureReason   string    `json:"failure_reason,omitempty"` // Why the task failed (workers only)
	CreatedAt       time.Time `json:"created_at"`
	LastNudge       time.Time `json:"last_nudge,omitempty"`
	ReadyForCleanup bool      `json:"ready_for_cleanup,omitempty"` // Only for workers
}

// Repository represents a tracked repository's state
type Repository struct {
	GithubURL        string             `json:"github_url"`
	TmuxSession      string             `json:"tmux_session"`
	Agents           map[string]Agent   `json:"agents"`
	TaskHistory      []TaskHistoryEntry `json:"task_history,omitempty"`
	MergeQueueConfig MergeQueueConfig   `json:"merge_queue_config,omitempty"`
	PRShepherdConfig PRShepherdConfig   `json:"pr_shepherd_config,omitempty"`
	ForkConfig       ForkConfig         `json:"fork_config,omitempty"`
	TargetBranch     string             `json:"target_branch,omitempty"` // Default branch for PRs (usually "main")
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

// atomicWrite writes data to a file atomically using a temp file and rename.
// This prevents corruption if the process crashes during writing.
func atomicWrite(path string, data []byte) error {
	// Use a unique temp file to avoid races between concurrent saves.
	// CreateTemp creates a file with a unique name in the same directory.
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, ".state-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Write data and close the file
	_, writeErr := tmpFile.Write(data)
	closeErr := tmpFile.Close()

	// Check for write or close errors
	if writeErr != nil {
		os.Remove(tmpPath) // Clean up temp file on error
		return fmt.Errorf("failed to write state file: %w", writeErr)
	}
	if closeErr != nil {
		os.Remove(tmpPath) // Clean up temp file on error
		return fmt.Errorf("failed to close temp file: %w", closeErr)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath) // Clean up temp file on error
		return fmt.Errorf("failed to rename state file: %w", err)
	}

	return nil
}

// Save persists state to disk
func (s *State) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	return atomicWrite(s.path, data)
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

// ClearAllAgents removes all agents from all repositories
// but preserves the repository entries themselves
func (s *State) ClearAllAgents() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, repo := range s.Repos {
		repo.Agents = make(map[string]Agent)
	}
	return s.saveUnlocked()
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
			PRShepherdConfig: repo.PRShepherdConfig,
			ForkConfig:       repo.ForkConfig,
			TargetBranch:     repo.TargetBranch,
		}
		// Copy agents
		for agentName, agent := range repo.Agents {
			repoCopy.Agents[agentName] = agent
		}
		// Copy task history
		if repo.TaskHistory != nil {
			repoCopy.TaskHistory = make([]TaskHistoryEntry, len(repo.TaskHistory))
			copy(repoCopy.TaskHistory, repo.TaskHistory)
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

// UpdateAgentPID updates just the PID of an agent
func (s *State) UpdateAgentPID(repoName, agentName string, pid int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	repo, exists := s.Repos[repoName]
	if !exists {
		return fmt.Errorf("repository %q not found", repoName)
	}

	agent, exists := repo.Agents[agentName]
	if !exists {
		return fmt.Errorf("agent %q not found in repository %q", agentName, repoName)
	}

	agent.PID = pid
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

// GetPRShepherdConfig returns the PR shepherd config for a repository
func (s *State) GetPRShepherdConfig(repoName string) (PRShepherdConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	repo, exists := s.Repos[repoName]
	if !exists {
		return PRShepherdConfig{}, fmt.Errorf("repository %q not found", repoName)
	}

	// Return default config if not set (for backward compatibility)
	if repo.PRShepherdConfig.TrackMode == "" {
		return DefaultPRShepherdConfig(), nil
	}
	return repo.PRShepherdConfig, nil
}

// UpdatePRShepherdConfig updates the PR shepherd config for a repository
func (s *State) UpdatePRShepherdConfig(repoName string, config PRShepherdConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	repo, exists := s.Repos[repoName]
	if !exists {
		return fmt.Errorf("repository %q not found", repoName)
	}

	repo.PRShepherdConfig = config
	return s.saveUnlocked()
}

// GetForkConfig returns the fork config for a repository
func (s *State) GetForkConfig(repoName string) (ForkConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	repo, exists := s.Repos[repoName]
	if !exists {
		return ForkConfig{}, fmt.Errorf("repository %q not found", repoName)
	}

	return repo.ForkConfig, nil
}

// UpdateForkConfig updates the fork config for a repository
func (s *State) UpdateForkConfig(repoName string, config ForkConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	repo, exists := s.Repos[repoName]
	if !exists {
		return fmt.Errorf("repository %q not found", repoName)
	}

	repo.ForkConfig = config
	return s.saveUnlocked()
}

// IsForkMode returns true if the repository should operate in fork mode.
// This is true if the repository is detected as a fork OR if force_fork_mode is enabled.
func (s *State) IsForkMode(repoName string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	repo, exists := s.Repos[repoName]
	if !exists {
		return false
	}

	return repo.ForkConfig.IsFork || repo.ForkConfig.ForceForkMode
}

// AddTaskHistory adds a completed task to the repository's history
func (s *State) AddTaskHistory(repoName string, entry TaskHistoryEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	repo, exists := s.Repos[repoName]
	if !exists {
		return fmt.Errorf("repository %q not found", repoName)
	}

	repo.TaskHistory = append(repo.TaskHistory, entry)
	return s.saveUnlocked()
}

// GetTaskHistory returns the task history for a repository, optionally limited to N entries
func (s *State) GetTaskHistory(repoName string, limit int) ([]TaskHistoryEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	repo, exists := s.Repos[repoName]
	if !exists {
		return nil, fmt.Errorf("repository %q not found", repoName)
	}

	history := repo.TaskHistory
	if history == nil {
		return []TaskHistoryEntry{}, nil
	}

	// Return most recent first
	result := make([]TaskHistoryEntry, len(history))
	for i, entry := range history {
		result[len(history)-1-i] = entry
	}

	// Apply limit if specified
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}

	return result, nil
}

// UpdateTaskHistoryStatus updates the status and PR info for a task by name
func (s *State) UpdateTaskHistoryStatus(repoName, taskName string, status TaskStatus, prURL string, prNumber int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	repo, exists := s.Repos[repoName]
	if !exists {
		return fmt.Errorf("repository %q not found", repoName)
	}

	// Find the most recent entry with this name and update it
	for i := len(repo.TaskHistory) - 1; i >= 0; i-- {
		if repo.TaskHistory[i].Name == taskName {
			repo.TaskHistory[i].Status = status
			if prURL != "" {
				repo.TaskHistory[i].PRURL = prURL
			}
			if prNumber > 0 {
				repo.TaskHistory[i].PRNumber = prNumber
			}
			return s.saveUnlocked()
		}
	}

	return fmt.Errorf("task %q not found in history", taskName)
}

// UpdateTaskHistorySummary updates the summary and failure reason for a task by name
func (s *State) UpdateTaskHistorySummary(repoName, taskName, summary, failureReason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	repo, exists := s.Repos[repoName]
	if !exists {
		return fmt.Errorf("repository %q not found", repoName)
	}

	// Find the most recent entry with this name and update it
	for i := len(repo.TaskHistory) - 1; i >= 0; i-- {
		if repo.TaskHistory[i].Name == taskName {
			if summary != "" {
				repo.TaskHistory[i].Summary = summary
			}
			if failureReason != "" {
				repo.TaskHistory[i].FailureReason = failureReason
				// Also update status to failed if a failure reason is provided
				repo.TaskHistory[i].Status = TaskStatusFailed
			}
			return s.saveUnlocked()
		}
	}

	return fmt.Errorf("task %q not found in history", taskName)
}

// saveUnlocked saves state without acquiring lock (caller must hold lock)
func (s *State) saveUnlocked() error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	return atomicWrite(s.path, data)
}
