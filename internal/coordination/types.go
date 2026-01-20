// Package coordination provides types and interfaces for hybrid agent deployment.
// It enables local and remote agents to coordinate through a shared registry and
// message routing system.
package coordination

import (
	"time"
)

// Location represents where an agent is running
type Location string

const (
	// LocationLocal indicates an agent running on the developer's machine
	LocationLocal Location = "local"
	// LocationRemote indicates an agent running on remote infrastructure
	LocationRemote Location = "remote"
)

// OwnershipLevel defines the ownership scope of an agent
type OwnershipLevel string

const (
	// OwnershipRepo indicates agents shared by the whole team (supervisor, merge-queue, review)
	OwnershipRepo OwnershipLevel = "repo"
	// OwnershipUser indicates agents owned by individual developers (workspace)
	OwnershipUser OwnershipLevel = "user"
	// OwnershipTask indicates ephemeral agents for specific tasks (workers)
	OwnershipTask OwnershipLevel = "task"
)

// AgentInfo represents an agent's registration in the coordination system
type AgentInfo struct {
	// Name is the unique identifier for this agent within the repo
	Name string `json:"name"`
	// Type is the agent type (supervisor, worker, merge-queue, workspace, review)
	Type string `json:"type"`
	// Location indicates whether the agent runs locally or remotely
	Location Location `json:"location"`
	// Ownership indicates the ownership level of this agent
	Ownership OwnershipLevel `json:"ownership"`
	// RepoName is the repository this agent is associated with
	RepoName string `json:"repo_name"`
	// Owner is the user/entity that owns this agent (for user/task level)
	Owner string `json:"owner,omitempty"`
	// Endpoint is the API endpoint for remote agents (empty for local)
	Endpoint string `json:"endpoint,omitempty"`
	// RegisteredAt is when this agent was registered
	RegisteredAt time.Time `json:"registered_at"`
	// LastHeartbeat is the last time this agent reported healthy
	LastHeartbeat time.Time `json:"last_heartbeat"`
	// Status indicates the current agent status
	Status AgentStatus `json:"status"`
	// Metadata contains additional agent-specific data
	Metadata map[string]string `json:"metadata,omitempty"`
}

// AgentStatus represents the current state of an agent
type AgentStatus string

const (
	// StatusActive indicates the agent is running and healthy
	StatusActive AgentStatus = "active"
	// StatusIdle indicates the agent is running but not working on anything
	StatusIdle AgentStatus = "idle"
	// StatusBusy indicates the agent is currently working on a task
	StatusBusy AgentStatus = "busy"
	// StatusUnreachable indicates the agent has not reported in recently
	StatusUnreachable AgentStatus = "unreachable"
	// StatusTerminated indicates the agent has been shut down
	StatusTerminated AgentStatus = "terminated"
)

// RoutedMessage represents a message being routed through the coordination system
type RoutedMessage struct {
	// ID is the unique message identifier
	ID string `json:"id"`
	// From is the sender agent name
	From string `json:"from"`
	// To is the recipient agent name
	To string `json:"to"`
	// RepoName is the repository context
	RepoName string `json:"repo_name"`
	// Body is the message content
	Body string `json:"body"`
	// Timestamp is when the message was created
	Timestamp time.Time `json:"timestamp"`
	// RouteInfo contains routing metadata
	RouteInfo *RouteInfo `json:"route_info,omitempty"`
}

// RouteInfo contains information about how a message was routed
type RouteInfo struct {
	// SourceLocation is where the sender is running
	SourceLocation Location `json:"source_location"`
	// DestLocation is where the recipient is running
	DestLocation Location `json:"dest_location"`
	// RoutedVia indicates if the message was routed through the API
	RoutedVia string `json:"routed_via,omitempty"`
	// RoutedAt is when the message was routed
	RoutedAt time.Time `json:"routed_at"`
}

// SpawnRequest represents a request to spawn a new worker agent
type SpawnRequest struct {
	// RepoName is the repository to spawn the worker for
	RepoName string `json:"repo_name"`
	// Task is the task description for the worker
	Task string `json:"task"`
	// SpawnedBy is the agent or user requesting the spawn
	SpawnedBy string `json:"spawned_by"`
	// PreferLocation is the preferred location for the worker (optional)
	PreferLocation Location `json:"prefer_location,omitempty"`
	// Metadata contains additional spawn parameters
	Metadata map[string]string `json:"metadata,omitempty"`
}

// SpawnResponse represents the result of a spawn request
type SpawnResponse struct {
	// WorkerName is the assigned name for the spawned worker
	WorkerName string `json:"worker_name"`
	// Location is where the worker was spawned
	Location Location `json:"location"`
	// Endpoint is the endpoint to communicate with the worker (for remote)
	Endpoint string `json:"endpoint,omitempty"`
	// Error is set if the spawn failed
	Error string `json:"error,omitempty"`
}

// HybridConfig holds configuration for hybrid deployment mode
type HybridConfig struct {
	// Enabled indicates whether hybrid mode is active
	Enabled bool `json:"enabled"`
	// CoordinationAPIURL is the URL of the coordination API service
	CoordinationAPIURL string `json:"coordination_api_url,omitempty"`
	// APIToken is the authentication token for the coordination API
	APIToken string `json:"api_token,omitempty"`
	// LocalAgentTypes specifies which agent types should run locally
	LocalAgentTypes []string `json:"local_agent_types,omitempty"`
	// RemoteAgentTypes specifies which agent types should run remotely
	RemoteAgentTypes []string `json:"remote_agent_types,omitempty"`
	// FallbackToLocal indicates whether to fall back to local if remote is unavailable
	FallbackToLocal bool `json:"fallback_to_local"`
}

// DefaultHybridConfig returns the default hybrid configuration (disabled)
func DefaultHybridConfig() HybridConfig {
	return HybridConfig{
		Enabled:         false,
		FallbackToLocal: true,
		LocalAgentTypes: []string{"workspace"},
		RemoteAgentTypes: []string{
			"supervisor",
			"merge-queue",
			"worker",
		},
	}
}

// GetOwnershipLevel returns the ownership level for a given agent type
func GetOwnershipLevel(agentType string) OwnershipLevel {
	switch agentType {
	case "supervisor", "merge-queue", "review":
		return OwnershipRepo
	case "workspace":
		return OwnershipUser
	case "worker":
		return OwnershipTask
	default:
		return OwnershipTask
	}
}
