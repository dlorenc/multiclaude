package coordination

import (
	"testing"
)

func TestGetOwnershipLevel(t *testing.T) {
	tests := []struct {
		agentType string
		expected  OwnershipLevel
	}{
		{"supervisor", OwnershipRepo},
		{"merge-queue", OwnershipRepo},
		{"review", OwnershipRepo},
		{"workspace", OwnershipUser},
		{"worker", OwnershipTask},
		{"unknown", OwnershipTask},
	}

	for _, tt := range tests {
		t.Run(tt.agentType, func(t *testing.T) {
			got := GetOwnershipLevel(tt.agentType)
			if got != tt.expected {
				t.Errorf("GetOwnershipLevel(%q) = %q, want %q", tt.agentType, got, tt.expected)
			}
		})
	}
}

func TestDefaultHybridConfig(t *testing.T) {
	config := DefaultHybridConfig()

	if config.Enabled {
		t.Error("default hybrid config should be disabled")
	}

	if !config.FallbackToLocal {
		t.Error("default should fall back to local")
	}

	// Check local agent types
	foundWorkspace := false
	for _, at := range config.LocalAgentTypes {
		if at == "workspace" {
			foundWorkspace = true
		}
	}
	if !foundWorkspace {
		t.Error("workspace should be in local agent types")
	}

	// Check remote agent types
	expectedRemote := map[string]bool{
		"supervisor":  true,
		"merge-queue": true,
		"worker":      true,
	}
	for _, at := range config.RemoteAgentTypes {
		if !expectedRemote[at] {
			t.Errorf("unexpected remote agent type: %s", at)
		}
		delete(expectedRemote, at)
	}
	if len(expectedRemote) > 0 {
		t.Errorf("missing remote agent types: %v", expectedRemote)
	}
}

func TestLocationConstants(t *testing.T) {
	if LocationLocal != "local" {
		t.Errorf("LocationLocal = %q, want %q", LocationLocal, "local")
	}
	if LocationRemote != "remote" {
		t.Errorf("LocationRemote = %q, want %q", LocationRemote, "remote")
	}
}

func TestAgentStatusConstants(t *testing.T) {
	statuses := []struct {
		status   AgentStatus
		expected string
	}{
		{StatusActive, "active"},
		{StatusIdle, "idle"},
		{StatusBusy, "busy"},
		{StatusUnreachable, "unreachable"},
		{StatusTerminated, "terminated"},
	}

	for _, tt := range statuses {
		if string(tt.status) != tt.expected {
			t.Errorf("status %v = %q, want %q", tt.status, tt.status, tt.expected)
		}
	}
}
