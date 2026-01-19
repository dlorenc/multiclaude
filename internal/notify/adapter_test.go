package notify

import (
	"testing"
)

func TestEventFilter_Matches(t *testing.T) {
	tests := []struct {
		name    string
		filter  EventFilter
		event   *Event
		matches bool
	}{
		{
			name:   "empty filter matches all",
			filter: EventFilter{},
			event: &Event{
				Type:     EventAgentQuestion,
				Priority: PriorityHigh,
				RepoName: "test-repo",
			},
			matches: true,
		},
		{
			name: "type filter matches correct type",
			filter: EventFilter{
				EventTypes: []EventType{EventAgentQuestion, EventAgentCompleted},
			},
			event: &Event{
				Type:     EventAgentQuestion,
				RepoName: "test-repo",
			},
			matches: true,
		},
		{
			name: "type filter rejects wrong type",
			filter: EventFilter{
				EventTypes: []EventType{EventAgentQuestion, EventAgentCompleted},
			},
			event: &Event{
				Type:     EventAgentError,
				RepoName: "test-repo",
			},
			matches: false,
		},
		{
			name: "priority filter matches high priority",
			filter: EventFilter{
				MinPriority: PriorityMedium,
			},
			event: &Event{
				Type:     EventAgentQuestion,
				Priority: PriorityHigh,
				RepoName: "test-repo",
			},
			matches: true,
		},
		{
			name: "priority filter rejects low priority",
			filter: EventFilter{
				MinPriority: PriorityHigh,
			},
			event: &Event{
				Type:     EventAgentQuestion,
				Priority: PriorityLow,
				RepoName: "test-repo",
			},
			matches: false,
		},
		{
			name: "repo inclusion matches",
			filter: EventFilter{
				Repos: []string{"repo-a", "repo-b"},
			},
			event: &Event{
				Type:     EventAgentQuestion,
				RepoName: "repo-a",
			},
			matches: true,
		},
		{
			name: "repo inclusion rejects",
			filter: EventFilter{
				Repos: []string{"repo-a", "repo-b"},
			},
			event: &Event{
				Type:     EventAgentQuestion,
				RepoName: "repo-c",
			},
			matches: false,
		},
		{
			name: "repo exclusion rejects",
			filter: EventFilter{
				ExcludeRepos: []string{"repo-excluded"},
			},
			event: &Event{
				Type:     EventAgentQuestion,
				RepoName: "repo-excluded",
			},
			matches: false,
		},
		{
			name: "repo exclusion allows",
			filter: EventFilter{
				ExcludeRepos: []string{"repo-excluded"},
			},
			event: &Event{
				Type:     EventAgentQuestion,
				RepoName: "repo-allowed",
			},
			matches: true,
		},
		{
			name: "combined filters",
			filter: EventFilter{
				EventTypes:  []EventType{EventAgentQuestion},
				MinPriority: PriorityMedium,
				Repos:       []string{"important-repo"},
			},
			event: &Event{
				Type:     EventAgentQuestion,
				Priority: PriorityHigh,
				RepoName: "important-repo",
			},
			matches: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.filter.Matches(tt.event)
			if got != tt.matches {
				t.Errorf("EventFilter.Matches() = %v, want %v", got, tt.matches)
			}
		})
	}
}

func TestPriorityAtLeast(t *testing.T) {
	tests := []struct {
		p1, p2   Priority
		expected bool
	}{
		{PriorityHigh, PriorityHigh, true},
		{PriorityHigh, PriorityMedium, true},
		{PriorityHigh, PriorityLow, true},
		{PriorityMedium, PriorityHigh, false},
		{PriorityMedium, PriorityMedium, true},
		{PriorityMedium, PriorityLow, true},
		{PriorityLow, PriorityHigh, false},
		{PriorityLow, PriorityMedium, false},
		{PriorityLow, PriorityLow, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.p1)+">="+string(tt.p2), func(t *testing.T) {
			got := priorityAtLeast(tt.p1, tt.p2)
			if got != tt.expected {
				t.Errorf("priorityAtLeast(%s, %s) = %v, want %v", tt.p1, tt.p2, got, tt.expected)
			}
		})
	}
}
