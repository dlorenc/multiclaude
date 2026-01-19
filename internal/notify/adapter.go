package notify

import (
	"context"
)

// Adapter defines the interface for notification adapters
type Adapter interface {
	// Name returns the adapter's configured name
	Name() string

	// Type returns the adapter type (slack, telegram, discord, webhook)
	Type() string

	// Send sends a notification event
	Send(ctx context.Context, event *Event) error

	// SupportsResponses returns true if the adapter can handle responses
	SupportsResponses() bool

	// Close cleans up any resources
	Close() error
}

// ResponseHandler is called when a response is received from an adapter
type ResponseHandler func(response *Response)

// InteractiveAdapter extends Adapter with response handling capabilities
type InteractiveAdapter interface {
	Adapter

	// SetResponseHandler sets the callback for incoming responses
	SetResponseHandler(handler ResponseHandler)

	// Start starts listening for responses (if applicable)
	Start(ctx context.Context) error
}

// AdapterConfig is the base configuration for all adapters
type AdapterConfig struct {
	// Name is a unique identifier for this adapter instance
	Name string `yaml:"name" json:"name"`
	// Type is the adapter type (slack, telegram, discord, webhook)
	Type string `yaml:"type" json:"type"`
	// Enabled controls whether this adapter is active
	Enabled bool `yaml:"enabled" json:"enabled"`
}

// EventFilter determines which events an adapter should receive
type EventFilter struct {
	// EventTypes lists allowed event types (empty means all)
	EventTypes []EventType `yaml:"event_types" json:"event_types"`
	// MinPriority is the minimum priority to notify on
	MinPriority Priority `yaml:"min_priority" json:"min_priority"`
	// Repos lists specific repos to monitor (empty means all)
	Repos []string `yaml:"repos" json:"repos"`
	// ExcludeRepos lists repos to exclude
	ExcludeRepos []string `yaml:"exclude_repos" json:"exclude_repos"`
}

// Matches returns true if the event matches this filter
func (f *EventFilter) Matches(event *Event) bool {
	// Check event type
	if len(f.EventTypes) > 0 {
		found := false
		for _, t := range f.EventTypes {
			if t == event.Type {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check priority
	if f.MinPriority != "" {
		if !priorityAtLeast(event.Priority, f.MinPriority) {
			return false
		}
	}

	// Check repo inclusion
	if len(f.Repos) > 0 {
		found := false
		for _, r := range f.Repos {
			if r == event.RepoName {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check repo exclusion
	for _, r := range f.ExcludeRepos {
		if r == event.RepoName {
			return false
		}
	}

	return true
}

// priorityAtLeast returns true if p1 >= p2
func priorityAtLeast(p1, p2 Priority) bool {
	priorities := map[Priority]int{
		PriorityLow:    1,
		PriorityMedium: 2,
		PriorityHigh:   3,
	}
	return priorities[p1] >= priorities[p2]
}
