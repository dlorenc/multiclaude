// Package coordination provides types and client for the multiclaude coordination API.
// This enables hybrid deployment where agents can run locally or remotely.
package coordination

import "time"

// AgentLocation indicates where an agent is running
type AgentLocation string

const (
	// LocationLocal indicates an agent running on a developer's machine
	LocationLocal AgentLocation = "local"
	// LocationRemote indicates an agent running in cloud infrastructure
	LocationRemote AgentLocation = "remote"
)

// AgentStatus indicates the current state of an agent
type AgentStatus string

const (
	// StatusActive means the agent is running and processing
	StatusActive AgentStatus = "active"
	// StatusIdle means the agent is running but not currently processing
	StatusIdle AgentStatus = "idle"
	// StatusStopped means the agent has been stopped
	StatusStopped AgentStatus = "stopped"
)

// Agent represents a registered agent in the coordination system
type Agent struct {
	// Name is the unique identifier for this agent within the repo
	Name string `json:"name"`
	// Type is the agent type (supervisor, worker, merge-queue, etc.)
	Type string `json:"type"`
	// Location indicates where the agent is running
	Location AgentLocation `json:"location"`
	// Owner is the user or service that owns this agent
	Owner string `json:"owner"`
	// Repo is the repository this agent belongs to (format: owner/repo)
	Repo string `json:"repo"`
	// Status is the current operational status
	Status AgentStatus `json:"status"`
	// Endpoint is the webhook URL for message delivery (empty for polling)
	Endpoint string `json:"endpoint,omitempty"`
	// LastSeen is when the agent last reported its status
	LastSeen time.Time `json:"last_seen"`
	// CreatedAt is when the agent was registered
	CreatedAt time.Time `json:"created_at"`
	// Metadata contains flexible key-value pairs for extension
	Metadata map[string]string `json:"metadata,omitempty"`
}

// MessageStatus indicates the delivery state of a message
type MessageStatus string

const (
	// MessagePending means the message has not been delivered yet
	MessagePending MessageStatus = "pending"
	// MessageDelivered means the message was sent to the target agent
	MessageDelivered MessageStatus = "delivered"
	// MessageAcked means the target agent acknowledged the message
	MessageAcked MessageStatus = "acked"
)

// Message represents a message between agents
type Message struct {
	// ID is the unique identifier for this message
	ID string `json:"id"`
	// From is the sender agent name
	From string `json:"from"`
	// To is the recipient agent name
	To string `json:"to"`
	// Repo is the repository context (format: owner/repo)
	Repo string `json:"repo"`
	// Body is the message content
	Body string `json:"body"`
	// Status is the delivery status
	Status MessageStatus `json:"status"`
	// CreatedAt is when the message was created
	CreatedAt time.Time `json:"created_at"`
	// AckedAt is when the message was acknowledged (nil if not acked)
	AckedAt *time.Time `json:"acked_at,omitempty"`
}

// SpawnStatus indicates the state of a worker spawn request
type SpawnStatus string

const (
	// SpawnPending means the request is waiting to be processed
	SpawnPending SpawnStatus = "pending"
	// SpawnInProgress means a worker is being created
	SpawnInProgress SpawnStatus = "in_progress"
	// SpawnCompleted means the worker was successfully created
	SpawnCompleted SpawnStatus = "completed"
	// SpawnFailed means the spawn request failed
	SpawnFailed SpawnStatus = "failed"
)

// SpawnRequest represents a request to spawn a remote worker
type SpawnRequest struct {
	// ID is the unique identifier for this request
	ID string `json:"id"`
	// Repo is the repository context (format: owner/repo)
	Repo string `json:"repo"`
	// Task is the task description for the worker
	Task string `json:"task"`
	// SpawnedBy identifies who requested the spawn (e.g., "workspace:dan")
	SpawnedBy string `json:"spawned_by"`
	// Priority affects spawn order (higher = more urgent)
	Priority int `json:"priority"`
	// Status is the current state of the request
	Status SpawnStatus `json:"status"`
	// WorkerName is set once the worker is spawned
	WorkerName string `json:"worker_name,omitempty"`
	// CreatedAt is when the request was created
	CreatedAt time.Time `json:"created_at"`
	// Metadata contains flexible key-value pairs
	Metadata map[string]string `json:"metadata,omitempty"`
}

// HybridConfig contains per-repo configuration for hybrid mode
type HybridConfig struct {
	// Repo is the repository (format: owner/repo)
	Repo string `json:"repo"`
	// Enabled indicates whether hybrid mode is active
	Enabled bool `json:"enabled"`
	// RemoteAgents lists agent types that should run remotely
	RemoteAgents []string `json:"remote_agents,omitempty"`
	// SpawnEndpoint is where spawn requests are sent
	SpawnEndpoint string `json:"spawn_endpoint,omitempty"`
	// WebhookSecret is used to validate incoming webhooks
	WebhookSecret string `json:"webhook_secret,omitempty"`
}

// RegisterAgentRequest is the request body for registering an agent
type RegisterAgentRequest struct {
	Name     string            `json:"name"`
	Type     string            `json:"type"`
	Location AgentLocation     `json:"location"`
	Endpoint string            `json:"endpoint,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// SendMessageRequest is the request body for sending a message
type SendMessageRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
	Body string `json:"body"`
}

// SpawnWorkerRequest is the request body for spawning a worker
type SpawnWorkerRequest struct {
	Task      string            `json:"task"`
	SpawnedBy string            `json:"spawned_by"`
	Priority  int               `json:"priority,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// UpdateAgentRequest is the request body for updating an agent
type UpdateAgentRequest struct {
	Status   *AgentStatus `json:"status,omitempty"`
	Endpoint *string      `json:"endpoint,omitempty"`
}

// APIError represents an error response from the coordination API
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *APIError) Error() string {
	return e.Message
}
