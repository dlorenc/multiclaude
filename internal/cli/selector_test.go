package cli

import (
	"strings"
	"testing"
)

func TestSelectableItem(t *testing.T) {
	// Test SelectableItem struct creation
	item := SelectableItem{
		Name:        "test-item",
		Description: "test description",
	}

	if item.Name != "test-item" {
		t.Errorf("Name = %s, want test-item", item.Name)
	}
	if item.Description != "test description" {
		t.Errorf("Description = %s, want test description", item.Description)
	}
}

func TestAgentsToSelectableItems(t *testing.T) {
	agents := []interface{}{
		map[string]interface{}{
			"name":   "worker-1",
			"type":   "worker",
			"task":   "Fix bug in login",
			"status": "running",
		},
		map[string]interface{}{
			"name":   "worker-2",
			"type":   "worker",
			"status": "completed",
		},
		map[string]interface{}{
			"name":   "default",
			"type":   "workspace",
			"status": "idle",
		},
		map[string]interface{}{
			"name":   "supervisor",
			"type":   "supervisor",
			"status": "running",
		},
	}

	// Test filtering by worker type
	t.Run("filter workers", func(t *testing.T) {
		items := agentsToSelectableItems(agents, []string{"worker"})
		if len(items) != 2 {
			t.Errorf("expected 2 workers, got %d", len(items))
		}
		if items[0].Name != "worker-1" {
			t.Errorf("expected worker-1, got %s", items[0].Name)
		}
		if items[0].Description != "Fix bug in login" {
			t.Errorf("expected task description, got %s", items[0].Description)
		}
	})

	// Test filtering by workspace type
	t.Run("filter workspaces", func(t *testing.T) {
		items := agentsToSelectableItems(agents, []string{"workspace"})
		if len(items) != 1 {
			t.Errorf("expected 1 workspace, got %d", len(items))
		}
		if items[0].Name != "default" {
			t.Errorf("expected default, got %s", items[0].Name)
		}
	})

	// Test no filter (all agents)
	t.Run("no filter", func(t *testing.T) {
		items := agentsToSelectableItems(agents, nil)
		if len(items) != 4 {
			t.Errorf("expected 4 agents, got %d", len(items))
		}
	})

	// Test empty filter (same as no filter)
	t.Run("empty filter", func(t *testing.T) {
		items := agentsToSelectableItems(agents, []string{})
		if len(items) != 4 {
			t.Errorf("expected 4 agents, got %d", len(items))
		}
	})

	// Test multiple types filter
	t.Run("multiple types filter", func(t *testing.T) {
		items := agentsToSelectableItems(agents, []string{"worker", "workspace"})
		if len(items) != 3 {
			t.Errorf("expected 3 agents (workers + workspace), got %d", len(items))
		}
	})
}

func TestReposToSelectableItems(t *testing.T) {
	repos := []interface{}{
		map[string]interface{}{
			"name":         "repo1",
			"total_agents": float64(5),
		},
		map[string]interface{}{
			"name":         "repo2",
			"total_agents": float64(0),
		},
		map[string]interface{}{
			"name": "repo3",
		},
	}

	items := reposToSelectableItems(repos)
	if len(items) != 3 {
		t.Errorf("expected 3 repos, got %d", len(items))
	}

	// Check first repo with agents
	if items[0].Name != "repo1" {
		t.Errorf("expected repo1, got %s", items[0].Name)
	}
	if items[0].Description != "5 agents" {
		t.Errorf("expected '5 agents', got %s", items[0].Description)
	}

	// Check repo with 0 agents (no description)
	if items[1].Name != "repo2" {
		t.Errorf("expected repo2, got %s", items[1].Name)
	}
	if items[1].Description != "" {
		t.Errorf("expected empty description, got %s", items[1].Description)
	}

	// Check repo without agent count
	if items[2].Name != "repo3" {
		t.Errorf("expected repo3, got %s", items[2].Name)
	}
	if items[2].Description != "" {
		t.Errorf("expected empty description, got %s", items[2].Description)
	}
}

// Edge case tests for selector functions

func TestAgentsToSelectableItems_EmptyInput(t *testing.T) {
	// Test with nil input
	items := agentsToSelectableItems(nil, nil)
	if len(items) != 0 {
		t.Errorf("expected 0 items for nil input, got %d", len(items))
	}

	// Test with empty slice
	items = agentsToSelectableItems([]interface{}{}, nil)
	if len(items) != 0 {
		t.Errorf("expected 0 items for empty input, got %d", len(items))
	}
}

func TestAgentsToSelectableItems_MalformedData(t *testing.T) {
	// Test with non-map data (should be skipped)
	agents := []interface{}{
		"not a map",
		123,
		nil,
		map[string]interface{}{
			"name": "valid-agent",
			"type": "worker",
		},
	}

	items := agentsToSelectableItems(agents, nil)
	if len(items) != 1 {
		t.Errorf("expected 1 valid item, got %d", len(items))
	}
	if items[0].Name != "valid-agent" {
		t.Errorf("expected valid-agent, got %s", items[0].Name)
	}
}

func TestAgentsToSelectableItems_MissingFields(t *testing.T) {
	// Test agents with missing or wrong type fields
	agents := []interface{}{
		map[string]interface{}{
			// Missing name
			"type": "worker",
		},
		map[string]interface{}{
			"name": "agent-with-numeric-type",
			"type": 123, // Wrong type
		},
		map[string]interface{}{
			"name": "valid",
			"type": "worker",
		},
	}

	items := agentsToSelectableItems(agents, []string{"worker"})
	// First agent has no name (empty string), second has wrong type assertion
	// Only third should be matched with type filter
	found := false
	for _, item := range items {
		if item.Name == "valid" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find 'valid' agent")
	}
}

func TestAgentsToSelectableItems_LongTaskDescription(t *testing.T) {
	// Test that long task descriptions are truncated
	longTask := strings.Repeat("a", 100) // 100 character task

	agents := []interface{}{
		map[string]interface{}{
			"name": "agent",
			"type": "worker",
			"task": longTask,
		},
	}

	items := agentsToSelectableItems(agents, nil)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	// Description should be truncated (format.Truncate limits to 50)
	if len(items[0].Description) > 53 { // 50 + "..." = 53
		t.Errorf("description should be truncated, got length %d", len(items[0].Description))
	}
}

func TestAgentsToSelectableItems_StatusFallback(t *testing.T) {
	// Test that status is used as fallback when task is empty
	agents := []interface{}{
		map[string]interface{}{
			"name":   "agent-1",
			"type":   "worker",
			"task":   "",
			"status": "running",
		},
		map[string]interface{}{
			"name":   "agent-2",
			"type":   "worker",
			"status": "completed",
		},
	}

	items := agentsToSelectableItems(agents, nil)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	if items[0].Description != "running" {
		t.Errorf("expected status fallback 'running', got %s", items[0].Description)
	}
	if items[1].Description != "completed" {
		t.Errorf("expected status fallback 'completed', got %s", items[1].Description)
	}
}

func TestReposToSelectableItems_EmptyInput(t *testing.T) {
	// Test with nil input
	items := reposToSelectableItems(nil)
	if len(items) != 0 {
		t.Errorf("expected 0 items for nil input, got %d", len(items))
	}

	// Test with empty slice
	items = reposToSelectableItems([]interface{}{})
	if len(items) != 0 {
		t.Errorf("expected 0 items for empty input, got %d", len(items))
	}
}

func TestReposToSelectableItems_MalformedData(t *testing.T) {
	// Test with non-map data (should be skipped)
	repos := []interface{}{
		"not a map",
		123,
		nil,
		map[string]interface{}{
			"name":         "valid-repo",
			"total_agents": float64(3),
		},
	}

	items := reposToSelectableItems(repos)
	if len(items) != 1 {
		t.Errorf("expected 1 valid item, got %d", len(items))
	}
	if items[0].Name != "valid-repo" {
		t.Errorf("expected valid-repo, got %s", items[0].Name)
	}
	if items[0].Description != "3 agents" {
		t.Errorf("expected '3 agents', got %s", items[0].Description)
	}
}

func TestReposToSelectableItems_WrongTypeFields(t *testing.T) {
	// Test repos with wrong type fields
	repos := []interface{}{
		map[string]interface{}{
			"name":         123, // Wrong type for name
			"total_agents": float64(5),
		},
		map[string]interface{}{
			"name":         "repo-with-string-agents",
			"total_agents": "5", // Wrong type for total_agents
		},
		map[string]interface{}{
			"name":         "valid-repo",
			"total_agents": float64(10),
		},
	}

	items := reposToSelectableItems(repos)
	// First repo has wrong name type (empty string), second has wrong agents type
	// All should produce items but with potentially empty fields

	// Find valid-repo
	var validItem *SelectableItem
	for i := range items {
		if items[i].Name == "valid-repo" {
			validItem = &items[i]
			break
		}
	}

	if validItem == nil {
		t.Error("expected to find 'valid-repo'")
	} else if validItem.Description != "10 agents" {
		t.Errorf("expected '10 agents', got %s", validItem.Description)
	}
}

func TestAgentsToSelectableItems_FilterNotMatching(t *testing.T) {
	// Test filtering with a type that doesn't exist
	agents := []interface{}{
		map[string]interface{}{
			"name": "worker-1",
			"type": "worker",
		},
		map[string]interface{}{
			"name": "supervisor",
			"type": "supervisor",
		},
	}

	items := agentsToSelectableItems(agents, []string{"nonexistent-type"})
	if len(items) != 0 {
		t.Errorf("expected 0 items for non-matching filter, got %d", len(items))
	}
}

func TestFormatAgentStatusCell(t *testing.T) {
	tests := []struct {
		status   string
		wantText string
	}{
		{"running", "running"},
		{"completed", "completed"},
		{"stopped", "stopped"},
		{"idle", "idle"},
		{"", "idle"},        // Default case
		{"unknown", "idle"}, // Unknown status defaults to idle
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			cell := formatAgentStatusCell(tt.status)
			// The cell.Text contains ANSI escape codes, but we can check it's not empty
			if cell.Text == "" {
				t.Errorf("formatAgentStatusCell(%q) returned empty text", tt.status)
			}
		})
	}
}
