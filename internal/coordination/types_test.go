package coordination

import (
	"testing"
	"time"
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

func TestOwnershipLevelConstants(t *testing.T) {
	levels := []struct {
		level    OwnershipLevel
		expected string
	}{
		{OwnershipRepo, "repo"},
		{OwnershipUser, "user"},
		{OwnershipTask, "task"},
	}

	for _, tt := range levels {
		if string(tt.level) != tt.expected {
			t.Errorf("ownership level %v = %q, want %q", tt.level, tt.level, tt.expected)
		}
	}
}

func TestRoutedMessage(t *testing.T) {
	now := time.Now()
	msg := RoutedMessage{
		ID:        "msg-123",
		From:      "supervisor",
		To:        "worker-1",
		RepoName:  "test-repo",
		Body:      "Please work on task X",
		Timestamp: now,
		RouteInfo: &RouteInfo{
			SourceLocation: LocationLocal,
			DestLocation:   LocationRemote,
			RoutedVia:      "coordination-api",
			RoutedAt:       now,
		},
	}

	if msg.ID != "msg-123" {
		t.Errorf("ID = %q, want %q", msg.ID, "msg-123")
	}
	if msg.From != "supervisor" {
		t.Errorf("From = %q, want %q", msg.From, "supervisor")
	}
	if msg.To != "worker-1" {
		t.Errorf("To = %q, want %q", msg.To, "worker-1")
	}
	if msg.RepoName != "test-repo" {
		t.Errorf("RepoName = %q, want %q", msg.RepoName, "test-repo")
	}
	if msg.Body != "Please work on task X" {
		t.Errorf("Body = %q, want %q", msg.Body, "Please work on task X")
	}
	if msg.RouteInfo == nil {
		t.Fatal("RouteInfo should not be nil")
	}
	if msg.RouteInfo.SourceLocation != LocationLocal {
		t.Errorf("SourceLocation = %q, want %q", msg.RouteInfo.SourceLocation, LocationLocal)
	}
	if msg.RouteInfo.DestLocation != LocationRemote {
		t.Errorf("DestLocation = %q, want %q", msg.RouteInfo.DestLocation, LocationRemote)
	}
}

func TestRoutedMessageWithoutRouteInfo(t *testing.T) {
	msg := RoutedMessage{
		ID:        "msg-456",
		From:      "worker-1",
		To:        "supervisor",
		RepoName:  "test-repo",
		Body:      "Task completed",
		Timestamp: time.Now(),
	}

	if msg.RouteInfo != nil {
		t.Error("RouteInfo should be nil when not set")
	}
}

func TestSpawnRequest(t *testing.T) {
	req := SpawnRequest{
		RepoName:       "test-repo",
		Task:           "Implement feature X",
		SpawnedBy:      "supervisor",
		PreferLocation: LocationRemote,
		Metadata: map[string]string{
			"priority": "high",
			"branch":   "feature/x",
		},
	}

	if req.RepoName != "test-repo" {
		t.Errorf("RepoName = %q, want %q", req.RepoName, "test-repo")
	}
	if req.Task != "Implement feature X" {
		t.Errorf("Task = %q, want %q", req.Task, "Implement feature X")
	}
	if req.SpawnedBy != "supervisor" {
		t.Errorf("SpawnedBy = %q, want %q", req.SpawnedBy, "supervisor")
	}
	if req.PreferLocation != LocationRemote {
		t.Errorf("PreferLocation = %q, want %q", req.PreferLocation, LocationRemote)
	}
	if req.Metadata["priority"] != "high" {
		t.Errorf("Metadata[priority] = %q, want %q", req.Metadata["priority"], "high")
	}
}

func TestSpawnRequestMinimal(t *testing.T) {
	req := SpawnRequest{
		RepoName:  "test-repo",
		Task:      "Simple task",
		SpawnedBy: "user",
	}

	if req.PreferLocation != "" {
		t.Errorf("PreferLocation should be empty, got %q", req.PreferLocation)
	}
	if req.Metadata != nil {
		t.Errorf("Metadata should be nil, got %v", req.Metadata)
	}
}

func TestSpawnResponse(t *testing.T) {
	resp := SpawnResponse{
		WorkerName: "happy-penguin",
		Location:   LocationRemote,
		Endpoint:   "https://api.example.com/agents/happy-penguin",
	}

	if resp.WorkerName != "happy-penguin" {
		t.Errorf("WorkerName = %q, want %q", resp.WorkerName, "happy-penguin")
	}
	if resp.Location != LocationRemote {
		t.Errorf("Location = %q, want %q", resp.Location, LocationRemote)
	}
	if resp.Endpoint != "https://api.example.com/agents/happy-penguin" {
		t.Errorf("Endpoint = %q, want expected value", resp.Endpoint)
	}
	if resp.Error != "" {
		t.Errorf("Error should be empty, got %q", resp.Error)
	}
}

func TestSpawnResponseWithError(t *testing.T) {
	resp := SpawnResponse{
		Error: "no remote capacity available",
	}

	if resp.WorkerName != "" {
		t.Errorf("WorkerName should be empty on error, got %q", resp.WorkerName)
	}
	if resp.Error != "no remote capacity available" {
		t.Errorf("Error = %q, want %q", resp.Error, "no remote capacity available")
	}
}

func TestAgentInfo(t *testing.T) {
	now := time.Now()
	agent := AgentInfo{
		Name:          "test-worker",
		Type:          "worker",
		Location:      LocationLocal,
		Ownership:     OwnershipTask,
		RepoName:      "test-repo",
		Owner:         "user@example.com",
		Endpoint:      "",
		RegisteredAt:  now,
		LastHeartbeat: now,
		Status:        StatusActive,
		Metadata: map[string]string{
			"task_id": "task-123",
		},
	}

	if agent.Name != "test-worker" {
		t.Errorf("Name = %q, want %q", agent.Name, "test-worker")
	}
	if agent.Type != "worker" {
		t.Errorf("Type = %q, want %q", agent.Type, "worker")
	}
	if agent.Location != LocationLocal {
		t.Errorf("Location = %q, want %q", agent.Location, LocationLocal)
	}
	if agent.Ownership != OwnershipTask {
		t.Errorf("Ownership = %q, want %q", agent.Ownership, OwnershipTask)
	}
	if agent.Owner != "user@example.com" {
		t.Errorf("Owner = %q, want %q", agent.Owner, "user@example.com")
	}
	if agent.Status != StatusActive {
		t.Errorf("Status = %q, want %q", agent.Status, StatusActive)
	}
	if agent.Metadata["task_id"] != "task-123" {
		t.Errorf("Metadata[task_id] = %q, want %q", agent.Metadata["task_id"], "task-123")
	}
}

func TestHybridConfigEnabled(t *testing.T) {
	config := HybridConfig{
		Enabled:            true,
		CoordinationAPIURL: "https://api.example.com",
		APIToken:           "secret-token",
		LocalAgentTypes:    []string{"workspace"},
		RemoteAgentTypes:   []string{"supervisor", "worker"},
		FallbackToLocal:    true,
	}

	if !config.Enabled {
		t.Error("Enabled should be true")
	}
	if config.CoordinationAPIURL != "https://api.example.com" {
		t.Errorf("CoordinationAPIURL = %q, want %q", config.CoordinationAPIURL, "https://api.example.com")
	}
	if config.APIToken != "secret-token" {
		t.Errorf("APIToken = %q, want %q", config.APIToken, "secret-token")
	}
	if len(config.LocalAgentTypes) != 1 || config.LocalAgentTypes[0] != "workspace" {
		t.Errorf("LocalAgentTypes = %v, want [workspace]", config.LocalAgentTypes)
	}
	if len(config.RemoteAgentTypes) != 2 {
		t.Errorf("RemoteAgentTypes length = %d, want 2", len(config.RemoteAgentTypes))
	}
}

func TestHybridConfigDisabled(t *testing.T) {
	config := HybridConfig{
		Enabled: false,
	}

	if config.Enabled {
		t.Error("Enabled should be false")
	}
	if config.CoordinationAPIURL != "" {
		t.Errorf("CoordinationAPIURL should be empty when disabled, got %q", config.CoordinationAPIURL)
	}
}

func TestHybridConfigEmptyAgentTypes(t *testing.T) {
	config := HybridConfig{
		Enabled:          true,
		LocalAgentTypes:  []string{},
		RemoteAgentTypes: []string{},
	}

	if len(config.LocalAgentTypes) != 0 {
		t.Errorf("LocalAgentTypes should be empty, got %v", config.LocalAgentTypes)
	}
	if len(config.RemoteAgentTypes) != 0 {
		t.Errorf("RemoteAgentTypes should be empty, got %v", config.RemoteAgentTypes)
	}
}
