// Package notify provides a notification system for multiclaude events.
// It supports multiple adapters (Slack, Telegram, Discord, webhooks) and
// enables remote monitoring and interaction with agents.
package notify

import (
	"time"
)

// EventType represents the type of notification event
type EventType string

const (
	// EventAgentQuestion is triggered when an agent asks a question
	EventAgentQuestion EventType = "agent.question"
	// EventAgentCompleted is triggered when a worker signals completion
	EventAgentCompleted EventType = "agent.completed"
	// EventAgentStuck is triggered when an agent hasn't made progress
	EventAgentStuck EventType = "agent.stuck"
	// EventAgentError is triggered when an agent process dies or crashes
	EventAgentError EventType = "agent.error"
	// EventPRCreated is triggered when an agent creates a pull request
	EventPRCreated EventType = "pr.created"
	// EventPRMerged is triggered when merge queue merges a PR
	EventPRMerged EventType = "pr.merged"
	// EventCIFailed is triggered when CI fails on an agent's PR
	EventCIFailed EventType = "ci.failed"
	// EventStatusUpdate is triggered for periodic status summaries
	EventStatusUpdate EventType = "status.update"
)

// Priority represents notification priority level
type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityMedium Priority = "medium"
	PriorityHigh   Priority = "high"
)

// Event represents a notification event
type Event struct {
	// ID is a unique identifier for this event
	ID string `json:"id"`
	// Type is the event type
	Type EventType `json:"type"`
	// Priority indicates urgency
	Priority Priority `json:"priority"`
	// Timestamp when the event occurred
	Timestamp time.Time `json:"timestamp"`
	// RepoName is the repository this event relates to
	RepoName string `json:"repo_name"`
	// AgentName is the agent that triggered the event (if applicable)
	AgentName string `json:"agent_name,omitempty"`
	// AgentType is the type of agent (supervisor, worker, etc)
	AgentType string `json:"agent_type,omitempty"`
	// Title is a short summary of the event
	Title string `json:"title"`
	// Message is the detailed event content
	Message string `json:"message"`
	// Context contains additional event-specific data
	Context map[string]string `json:"context,omitempty"`
	// ActionRequired indicates if user action is needed
	ActionRequired bool `json:"action_required"`
	// ResponseID is set when this event can receive a response
	ResponseID string `json:"response_id,omitempty"`
}

// EventPriority returns the default priority for an event type
func EventPriority(t EventType) Priority {
	switch t {
	case EventAgentQuestion, EventAgentStuck, EventAgentError, EventCIFailed:
		return PriorityHigh
	case EventAgentCompleted, EventPRCreated:
		return PriorityMedium
	case EventPRMerged, EventStatusUpdate:
		return PriorityLow
	default:
		return PriorityMedium
	}
}

// Response represents a user response to a notification
type Response struct {
	// EventID is the ID of the event being responded to
	EventID string `json:"event_id"`
	// ResponseID links back to the original event's response ID
	ResponseID string `json:"response_id"`
	// Message is the user's response message
	Message string `json:"message"`
	// Action is a predefined action (e.g., "dismiss", "retry")
	Action string `json:"action,omitempty"`
	// Source indicates which adapter received the response
	Source string `json:"source"`
	// UserID identifies who responded (adapter-specific)
	UserID string `json:"user_id,omitempty"`
	// Timestamp when the response was received
	Timestamp time.Time `json:"timestamp"`
}

// StatusSummary represents a summary of current agent status
type StatusSummary struct {
	// RepoName is the repository
	RepoName string `json:"repo_name"`
	// TotalAgents is the number of active agents
	TotalAgents int `json:"total_agents"`
	// ActiveWorkers is the number of working agents
	ActiveWorkers int `json:"active_workers"`
	// PendingQuestions is the number of unanswered questions
	PendingQuestions int `json:"pending_questions"`
	// CompletedTasks is the number of completed tasks since last summary
	CompletedTasks int `json:"completed_tasks"`
	// Agents is a list of agent statuses
	Agents []AgentStatus `json:"agents"`
	// GeneratedAt is when this summary was created
	GeneratedAt time.Time `json:"generated_at"`
}

// AgentStatus represents the status of a single agent
type AgentStatus struct {
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Task      string    `json:"task,omitempty"`
	Status    string    `json:"status"` // "working", "waiting", "stuck", "completed"
	Duration  string    `json:"duration,omitempty"`
	LastEvent time.Time `json:"last_event,omitempty"`
}
