package coordination

import (
	"fmt"
	"sync"
	"time"
)

// Registry provides access to the agent registry.
// It tracks all agents (local and remote) and their current state.
type Registry interface {
	// Register adds or updates an agent in the registry
	Register(agent *AgentInfo) error
	// Unregister removes an agent from the registry
	Unregister(repoName, agentName string) error
	// Get retrieves an agent by name
	Get(repoName, agentName string) (*AgentInfo, error)
	// List returns all agents for a repository
	List(repoName string) ([]*AgentInfo, error)
	// ListByType returns agents of a specific type
	ListByType(repoName, agentType string) ([]*AgentInfo, error)
	// ListByLocation returns agents at a specific location
	ListByLocation(repoName string, location Location) ([]*AgentInfo, error)
	// UpdateHeartbeat updates the last heartbeat time for an agent
	UpdateHeartbeat(repoName, agentName string) error
	// UpdateStatus updates the status of an agent
	UpdateStatus(repoName, agentName string, status AgentStatus) error
}

// LocalRegistry implements Registry using in-memory storage.
// This is used when hybrid mode is disabled or as a local cache.
type LocalRegistry struct {
	mu     sync.RWMutex
	agents map[string]map[string]*AgentInfo // repo -> agent name -> info
}

// NewLocalRegistry creates a new local registry
func NewLocalRegistry() *LocalRegistry {
	return &LocalRegistry{
		agents: make(map[string]map[string]*AgentInfo),
	}
}

// Register adds or updates an agent in the registry
func (r *LocalRegistry) Register(agent *AgentInfo) error {
	if agent.Name == "" {
		return fmt.Errorf("agent name is required")
	}
	if agent.RepoName == "" {
		return fmt.Errorf("repo name is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.agents[agent.RepoName] == nil {
		r.agents[agent.RepoName] = make(map[string]*AgentInfo)
	}

	// Set registration time if not already set
	if agent.RegisteredAt.IsZero() {
		agent.RegisteredAt = time.Now()
	}
	agent.LastHeartbeat = time.Now()

	// Set ownership if not specified
	if agent.Ownership == "" {
		agent.Ownership = GetOwnershipLevel(agent.Type)
	}

	r.agents[agent.RepoName][agent.Name] = agent
	return nil
}

// Unregister removes an agent from the registry
func (r *LocalRegistry) Unregister(repoName, agentName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.agents[repoName] == nil {
		return fmt.Errorf("repository %q not found in registry", repoName)
	}

	if _, exists := r.agents[repoName][agentName]; !exists {
		return fmt.Errorf("agent %q not found in repository %q", agentName, repoName)
	}

	delete(r.agents[repoName], agentName)
	return nil
}

// Get retrieves an agent by name
func (r *LocalRegistry) Get(repoName, agentName string) (*AgentInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.agents[repoName] == nil {
		return nil, fmt.Errorf("repository %q not found in registry", repoName)
	}

	agent, exists := r.agents[repoName][agentName]
	if !exists {
		return nil, fmt.Errorf("agent %q not found in repository %q", agentName, repoName)
	}

	// Return a copy to prevent mutation
	copy := *agent
	return &copy, nil
}

// List returns all agents for a repository
func (r *LocalRegistry) List(repoName string) ([]*AgentInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.agents[repoName] == nil {
		return []*AgentInfo{}, nil
	}

	result := make([]*AgentInfo, 0, len(r.agents[repoName]))
	for _, agent := range r.agents[repoName] {
		copy := *agent
		result = append(result, &copy)
	}
	return result, nil
}

// ListByType returns agents of a specific type
func (r *LocalRegistry) ListByType(repoName, agentType string) ([]*AgentInfo, error) {
	agents, err := r.List(repoName)
	if err != nil {
		return nil, err
	}

	var result []*AgentInfo
	for _, agent := range agents {
		if agent.Type == agentType {
			result = append(result, agent)
		}
	}
	return result, nil
}

// ListByLocation returns agents at a specific location
func (r *LocalRegistry) ListByLocation(repoName string, location Location) ([]*AgentInfo, error) {
	agents, err := r.List(repoName)
	if err != nil {
		return nil, err
	}

	var result []*AgentInfo
	for _, agent := range agents {
		if agent.Location == location {
			result = append(result, agent)
		}
	}
	return result, nil
}

// UpdateHeartbeat updates the last heartbeat time for an agent
func (r *LocalRegistry) UpdateHeartbeat(repoName, agentName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.agents[repoName] == nil {
		return fmt.Errorf("repository %q not found in registry", repoName)
	}

	agent, exists := r.agents[repoName][agentName]
	if !exists {
		return fmt.Errorf("agent %q not found in repository %q", agentName, repoName)
	}

	agent.LastHeartbeat = time.Now()
	return nil
}

// UpdateStatus updates the status of an agent
func (r *LocalRegistry) UpdateStatus(repoName, agentName string, status AgentStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.agents[repoName] == nil {
		return fmt.Errorf("repository %q not found in registry", repoName)
	}

	agent, exists := r.agents[repoName][agentName]
	if !exists {
		return fmt.Errorf("agent %q not found in repository %q", agentName, repoName)
	}

	agent.Status = status
	agent.LastHeartbeat = time.Now()
	return nil
}

// GetStaleAgents returns agents that haven't sent a heartbeat within the threshold
func (r *LocalRegistry) GetStaleAgents(repoName string, threshold time.Duration) ([]*AgentInfo, error) {
	agents, err := r.List(repoName)
	if err != nil {
		return nil, err
	}

	cutoff := time.Now().Add(-threshold)
	var stale []*AgentInfo
	for _, agent := range agents {
		if agent.LastHeartbeat.Before(cutoff) && agent.Status != StatusTerminated {
			result := *agent
			result.Status = StatusUnreachable
			stale = append(stale, &result)
		}
	}
	return stale, nil
}

// Clear removes all agents for a repository
func (r *LocalRegistry) Clear(repoName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.agents, repoName)
}
