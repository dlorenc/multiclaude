package cli

import (
	"testing"
)

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
