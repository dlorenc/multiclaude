// Package coordination provides multi-machine coordination for distributed
// multiclaude deployments. It enables agents running on different machines
// to work together on shared repositories through a central coordinator.
package coordination

import (
	"time"
)

// NodeStatus represents the health status of a registered node
type NodeStatus string

const (
	NodeStatusOnline  NodeStatus = "online"
	NodeStatusOffline NodeStatus = "offline"
	NodeStatusDraining NodeStatus = "draining" // not accepting new tasks
)

// TaskStatus represents the lifecycle state of a task
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusClaimed   TaskStatus = "claimed"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusOrphaned  TaskStatus = "orphaned" // node went offline
)

// Priority defines task priority levels
type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityMedium Priority = "medium"
	PriorityHigh   Priority = "high"
	PriorityCritical Priority = "critical"
)

// AgentStatus represents the state of an agent
type AgentStatus string

const (
	AgentStatusRunning   AgentStatus = "running"
	AgentStatusIdle      AgentStatus = "idle"
	AgentStatusCompleted AgentStatus = "completed"
	AgentStatusFailed    AgentStatus = "failed"
)

// EventType identifies the type of coordination event
type EventType string

const (
	EventTypeNodeRegistered   EventType = "node.registered"
	EventTypeNodeOffline      EventType = "node.offline"
	EventTypeNodeOnline       EventType = "node.online"
	EventTypeTaskCreated      EventType = "task.created"
	EventTypeTaskClaimed      EventType = "task.claimed"
	EventTypeTaskCompleted    EventType = "task.completed"
	EventTypeTaskFailed       EventType = "task.failed"
	EventTypeTaskOrphaned     EventType = "task.orphaned"
	EventTypeAgentSpawned     EventType = "agent.spawned"
	EventTypeAgentCompleted   EventType = "agent.completed"
	EventTypeMessageRouted    EventType = "message.routed"
)

// NodeCapacity describes the resource limits of a node
type NodeCapacity struct {
	MaxWorkers     int `json:"max_workers"`
	CurrentWorkers int `json:"current_workers"`
}

// AgentSummary provides a brief view of an agent for coordination purposes
type AgentSummary struct {
	Name      string      `json:"name"`
	Type      string      `json:"type"`
	Status    AgentStatus `json:"status"`
	Task      string      `json:"task,omitempty"`
	StartedAt time.Time   `json:"started_at,omitempty"`
}

// NodeMetrics contains resource utilization metrics from a node
type NodeMetrics struct {
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryPercent float64 `json:"memory_percent"`
	DiskPercent   float64 `json:"disk_percent,omitempty"`
	LoadAverage   float64 `json:"load_average,omitempty"`
}

// Node represents a registered multiclaude instance
type Node struct {
	ID       string            `json:"id"`
	Hostname string            `json:"hostname"`
	Capacity NodeCapacity      `json:"capacity"`
	Labels   map[string]string `json:"labels,omitempty"`
	Status   NodeStatus        `json:"status"`
	LastSeen time.Time         `json:"last_seen"`
	Agents   []AgentSummary    `json:"agents,omitempty"`
	Metrics  *NodeMetrics      `json:"metrics,omitempty"`
}

// Task represents a distributable work item
type Task struct {
	ID          string            `json:"id"`
	Repo        string            `json:"repo"`
	Description string            `json:"description"`
	Priority    Priority          `json:"priority"`
	Labels      map[string]string `json:"labels,omitempty"`
	ClaimedBy   string            `json:"claimed_by,omitempty"`
	ClaimedAt   time.Time         `json:"claimed_at,omitempty"`
	Status      TaskStatus        `json:"status"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	Result      *TaskResult       `json:"result,omitempty"`
}

// TaskResult contains the outcome of a completed task
type TaskResult struct {
	Success   bool      `json:"success"`
	PRNumber  int       `json:"pr_number,omitempty"`
	PRURL     string    `json:"pr_url,omitempty"`
	Error     string    `json:"error,omitempty"`
	Duration  time.Duration `json:"duration"`
	CompletedAt time.Time `json:"completed_at"`
}

// Message represents an inter-agent message in distributed mode
type Message struct {
	ID        string    `json:"id"`
	From      string    `json:"from"`       // "agent@node"
	To        string    `json:"to"`         // "agent@node" or "agent" (local)
	Repo      string    `json:"repo"`
	Body      string    `json:"body"`
	Timestamp time.Time `json:"timestamp"`
	Status    string    `json:"status"`     // pending, delivered, read
}

// Event represents a coordination event for SSE streaming
type Event struct {
	ID        string            `json:"id"`
	Type      EventType         `json:"type"`
	Timestamp time.Time         `json:"timestamp"`
	Repo      string            `json:"repo,omitempty"`
	NodeID    string            `json:"node_id,omitempty"`
	AgentName string            `json:"agent_name,omitempty"`
	TaskID    string            `json:"task_id,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// RegisterRequest is sent when a node registers with the coordinator
type RegisterRequest struct {
	NodeID   string            `json:"node_id"`
	Hostname string            `json:"hostname"`
	Capacity NodeCapacity      `json:"capacity"`
	Labels   map[string]string `json:"labels,omitempty"`
}

// RegisterResponse is returned after successful registration
type RegisterResponse struct {
	RegistrationID          string `json:"registration_id"`
	HeartbeatIntervalSeconds int    `json:"heartbeat_interval_seconds"`
}

// HeartbeatRequest is sent periodically to maintain registration
type HeartbeatRequest struct {
	RegistrationID string         `json:"registration_id"`
	Status         NodeStatus     `json:"status"`
	Agents         []AgentSummary `json:"agents,omitempty"`
	Metrics        *NodeMetrics   `json:"metrics,omitempty"`
}

// HeartbeatResponse acknowledges a heartbeat
type HeartbeatResponse struct {
	Acknowledged bool   `json:"acknowledged"`
	Message      string `json:"message,omitempty"`
}

// TaskClaimRequest is sent to claim a pending task
type TaskClaimRequest struct {
	RegistrationID string `json:"registration_id"`
	WorkerName     string `json:"worker_name"`
}

// TaskClaimResponse confirms a task claim
type TaskClaimResponse struct {
	Claimed bool   `json:"claimed"`
	Task    *Task  `json:"task,omitempty"`
	Error   string `json:"error,omitempty"`
}

// TaskUpdateRequest reports progress or completion of a task
type TaskUpdateRequest struct {
	RegistrationID string      `json:"registration_id"`
	WorkerName     string      `json:"worker_name"`
	Status         TaskStatus  `json:"status"`
	Result         *TaskResult `json:"result,omitempty"`
}

// CreateTaskRequest is sent to create a new distributable task
type CreateTaskRequest struct {
	Repo        string            `json:"repo"`
	Description string            `json:"description"`
	Priority    Priority          `json:"priority,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// StateResponse contains the full coordination state for a repository
type StateResponse struct {
	Repo         string                   `json:"repo"`
	Agents       map[string]*AgentState   `json:"agents"`
	PendingTasks int                      `json:"pending_tasks"`
	ActivePRs    int                      `json:"active_prs"`
	Nodes        []string                 `json:"nodes"`
}

// AgentState represents agent state in coordination response
type AgentState struct {
	Node   string `json:"node"`
	Status string `json:"status"`
	Task   string `json:"task,omitempty"`
}

// APIResponse is the standard wrapper for all API responses
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	Code    string      `json:"code,omitempty"`
}

// Config holds coordination configuration
type Config struct {
	// Server settings
	Enabled    bool   `yaml:"enabled" json:"enabled"`
	ListenAddr string `yaml:"listen_addr" json:"listen_addr"`

	// TLS settings
	TLS *TLSConfig `yaml:"tls" json:"tls,omitempty"`

	// Authentication
	Auth *AuthConfig `yaml:"auth" json:"auth,omitempty"`

	// Registration settings
	HeartbeatInterval time.Duration `yaml:"heartbeat_interval" json:"heartbeat_interval"`
	OfflineThreshold  time.Duration `yaml:"offline_threshold" json:"offline_threshold"`

	// Task settings
	DefaultPriority Priority      `yaml:"default_priority" json:"default_priority"`
	ClaimTimeout    time.Duration `yaml:"claim_timeout" json:"claim_timeout"`
}

// TLSConfig holds TLS settings
type TLSConfig struct {
	Enabled  bool   `yaml:"enabled" json:"enabled"`
	CertFile string `yaml:"cert_file" json:"cert_file"`
	KeyFile  string `yaml:"key_file" json:"key_file"`
}

// AuthConfig holds authentication settings
type AuthConfig struct {
	Tokens      []string `yaml:"tokens" json:"tokens"`
	RequireAuth bool     `yaml:"require_auth" json:"require_auth"`
}

// ClientConfig holds client-side coordination settings
type ClientConfig struct {
	Enabled    bool              `yaml:"enabled" json:"enabled"`
	ServerURL  string            `yaml:"server_url" json:"server_url"`
	Token      string            `yaml:"token" json:"token"`
	NodeID     string            `yaml:"node_id" json:"node_id"`
	Labels     map[string]string `yaml:"labels" json:"labels,omitempty"`
	MaxWorkers int               `yaml:"max_workers" json:"max_workers"`
}

// DefaultConfig returns sensible default configuration
func DefaultConfig() *Config {
	return &Config{
		Enabled:           false,
		ListenAddr:        ":7331",
		HeartbeatInterval: 30 * time.Second,
		OfflineThreshold:  90 * time.Second,
		DefaultPriority:   PriorityMedium,
		ClaimTimeout:      5 * time.Minute,
	}
}

// DefaultClientConfig returns sensible default client configuration
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		Enabled:    false,
		MaxWorkers: 5,
	}
}
