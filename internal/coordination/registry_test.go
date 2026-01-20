package coordination

import (
	"testing"
	"time"
)

func TestNewRegistry(t *testing.T) {
	config := DefaultConfig()
	r := NewRegistry(config)

	if r == nil {
		t.Fatal("expected registry")
	}
	if r.nodes == nil {
		t.Error("expected nodes map to be initialized")
	}
	if r.registrations == nil {
		t.Error("expected registrations map to be initialized")
	}
}

func TestRegistry_Register(t *testing.T) {
	config := DefaultConfig()
	r := NewRegistry(config)

	req := &RegisterRequest{
		NodeID:   "node-1",
		Hostname: "localhost",
		Capacity: NodeCapacity{MaxWorkers: 5},
		Labels:   map[string]string{"pool": "gpu"},
	}

	resp, err := r.Register(req)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if resp.RegistrationID == "" {
		t.Error("expected registration ID")
	}
	if resp.HeartbeatIntervalSeconds != 30 {
		t.Errorf("expected heartbeat interval 30, got %d", resp.HeartbeatIntervalSeconds)
	}

	// Verify node was added
	node, err := r.GetNode("node-1")
	if err != nil {
		t.Fatalf("GetNode failed: %v", err)
	}
	if node.Hostname != "localhost" {
		t.Errorf("expected hostname 'localhost', got '%s'", node.Hostname)
	}
	if node.Capacity.MaxWorkers != 5 {
		t.Errorf("expected max_workers 5, got %d", node.Capacity.MaxWorkers)
	}
	if node.Labels["pool"] != "gpu" {
		t.Errorf("expected label pool=gpu, got '%s'", node.Labels["pool"])
	}
	if node.Status != NodeStatusOnline {
		t.Errorf("expected status online, got '%s'", node.Status)
	}
}

func TestRegistry_RegisterDuplicate(t *testing.T) {
	config := DefaultConfig()
	r := NewRegistry(config)

	req := &RegisterRequest{
		NodeID:   "node-1",
		Hostname: "localhost",
	}

	resp1, err := r.Register(req)
	if err != nil {
		t.Fatalf("First register failed: %v", err)
	}

	// Register again - should update existing
	req.Hostname = "updated-host"
	resp2, err := r.Register(req)
	if err != nil {
		t.Fatalf("Second register failed: %v", err)
	}

	// Should return same registration ID
	if resp1.RegistrationID != resp2.RegistrationID {
		t.Errorf("expected same registration ID, got '%s' and '%s'", resp1.RegistrationID, resp2.RegistrationID)
	}

	// Hostname should be updated
	node, _ := r.GetNode("node-1")
	if node.Hostname != "updated-host" {
		t.Errorf("expected hostname 'updated-host', got '%s'", node.Hostname)
	}
}

func TestRegistry_Unregister(t *testing.T) {
	config := DefaultConfig()
	r := NewRegistry(config)

	req := &RegisterRequest{
		NodeID:   "node-1",
		Hostname: "localhost",
	}

	resp, _ := r.Register(req)

	err := r.Unregister(resp.RegistrationID)
	if err != nil {
		t.Fatalf("Unregister failed: %v", err)
	}

	// Node should be gone
	_, err = r.GetNode("node-1")
	if err == nil {
		t.Error("expected error getting unregistered node")
	}
}

func TestRegistry_UnregisterNotFound(t *testing.T) {
	config := DefaultConfig()
	r := NewRegistry(config)

	err := r.Unregister("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent registration")
	}
}

func TestRegistry_Heartbeat(t *testing.T) {
	config := DefaultConfig()
	r := NewRegistry(config)

	req := &RegisterRequest{
		NodeID:   "node-1",
		Hostname: "localhost",
	}

	resp, _ := r.Register(req)

	hb := &HeartbeatRequest{
		RegistrationID: resp.RegistrationID,
		Status:         NodeStatusOnline,
		Agents: []AgentSummary{
			{Name: "worker-1", Type: "worker", Status: AgentStatusRunning},
		},
		Metrics: &NodeMetrics{CPUPercent: 50.0},
	}

	err := r.Heartbeat(hb)
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}

	node, _ := r.GetNode("node-1")
	if len(node.Agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(node.Agents))
	}
	if node.Metrics.CPUPercent != 50.0 {
		t.Errorf("expected CPU 50%%, got %f", node.Metrics.CPUPercent)
	}
}

func TestRegistry_HeartbeatNotRegistered(t *testing.T) {
	config := DefaultConfig()
	r := NewRegistry(config)

	hb := &HeartbeatRequest{
		RegistrationID: "nonexistent",
		Status:         NodeStatusOnline,
	}

	err := r.Heartbeat(hb)
	if err == nil {
		t.Error("expected error for nonexistent registration")
	}
}

func TestRegistry_ListNodes(t *testing.T) {
	config := DefaultConfig()
	r := NewRegistry(config)

	r.Register(&RegisterRequest{NodeID: "node-1", Hostname: "host1"})
	r.Register(&RegisterRequest{NodeID: "node-2", Hostname: "host2"})

	nodes := r.ListNodes()
	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(nodes))
	}
}

func TestRegistry_ListOnlineNodes(t *testing.T) {
	config := DefaultConfig()
	r := NewRegistry(config)

	resp1, _ := r.Register(&RegisterRequest{NodeID: "node-1", Hostname: "host1"})
	r.Register(&RegisterRequest{NodeID: "node-2", Hostname: "host2"})

	// Mark node-1 as offline
	r.Heartbeat(&HeartbeatRequest{
		RegistrationID: resp1.RegistrationID,
		Status:         NodeStatusOffline,
	})

	nodes := r.ListOnlineNodes()
	if len(nodes) != 1 {
		t.Errorf("expected 1 online node, got %d", len(nodes))
	}
	if nodes[0].ID != "node-2" {
		t.Errorf("expected node-2, got '%s'", nodes[0].ID)
	}
}

func TestRegistry_ListAvailableNodes(t *testing.T) {
	config := DefaultConfig()
	r := NewRegistry(config)

	r.Register(&RegisterRequest{
		NodeID:   "node-1",
		Hostname: "host1",
		Capacity: NodeCapacity{MaxWorkers: 5, CurrentWorkers: 5}, // Full
	})
	r.Register(&RegisterRequest{
		NodeID:   "node-2",
		Hostname: "host2",
		Capacity: NodeCapacity{MaxWorkers: 5, CurrentWorkers: 2}, // Has capacity
	})

	nodes := r.ListAvailableNodes()
	if len(nodes) != 1 {
		t.Errorf("expected 1 available node, got %d", len(nodes))
	}
}

func TestRegistry_FindNodesByLabel(t *testing.T) {
	config := DefaultConfig()
	r := NewRegistry(config)

	r.Register(&RegisterRequest{
		NodeID:   "node-1",
		Hostname: "host1",
		Labels:   map[string]string{"pool": "gpu", "region": "us-west"},
	})
	r.Register(&RegisterRequest{
		NodeID:   "node-2",
		Hostname: "host2",
		Labels:   map[string]string{"pool": "cpu", "region": "us-west"},
	})

	// Find GPU nodes
	nodes := r.FindNodesByLabel(map[string]string{"pool": "gpu"})
	if len(nodes) != 1 {
		t.Errorf("expected 1 GPU node, got %d", len(nodes))
	}
	if nodes[0].ID != "node-1" {
		t.Errorf("expected node-1, got '%s'", nodes[0].ID)
	}

	// Find all US West nodes
	nodes = r.FindNodesByLabel(map[string]string{"region": "us-west"})
	if len(nodes) != 2 {
		t.Errorf("expected 2 US West nodes, got %d", len(nodes))
	}

	// Find nodes with multiple labels
	nodes = r.FindNodesByLabel(map[string]string{"pool": "cpu", "region": "us-west"})
	if len(nodes) != 1 {
		t.Errorf("expected 1 matching node, got %d", len(nodes))
	}
}

func TestRegistry_GetNodeByRegistration(t *testing.T) {
	config := DefaultConfig()
	r := NewRegistry(config)

	resp, _ := r.Register(&RegisterRequest{
		NodeID:   "node-1",
		Hostname: "localhost",
	})

	node, err := r.GetNodeByRegistration(resp.RegistrationID)
	if err != nil {
		t.Fatalf("GetNodeByRegistration failed: %v", err)
	}
	if node.ID != "node-1" {
		t.Errorf("expected node-1, got '%s'", node.ID)
	}
}

func TestRegistry_GetStats(t *testing.T) {
	config := DefaultConfig()
	r := NewRegistry(config)

	r.Register(&RegisterRequest{
		NodeID:   "node-1",
		Capacity: NodeCapacity{MaxWorkers: 5, CurrentWorkers: 2},
	})
	r.Register(&RegisterRequest{
		NodeID:   "node-2",
		Capacity: NodeCapacity{MaxWorkers: 3, CurrentWorkers: 1},
	})

	stats := r.GetStats()

	if stats["total_nodes"].(int) != 2 {
		t.Errorf("expected total_nodes 2, got %v", stats["total_nodes"])
	}
	if stats["online"].(int) != 2 {
		t.Errorf("expected online 2, got %v", stats["online"])
	}
	if stats["total_capacity"].(int) != 8 {
		t.Errorf("expected total_capacity 8, got %v", stats["total_capacity"])
	}
	if stats["used_capacity"].(int) != 3 {
		t.Errorf("expected used_capacity 3, got %v", stats["used_capacity"])
	}
	if stats["available"].(int) != 5 {
		t.Errorf("expected available 5, got %v", stats["available"])
	}
}

func TestMatchesLabels(t *testing.T) {
	tests := []struct {
		name       string
		nodeLabels map[string]string
		required   map[string]string
		want       bool
	}{
		{
			name:       "empty required",
			nodeLabels: map[string]string{"pool": "gpu"},
			required:   nil,
			want:       true,
		},
		{
			name:       "matching single",
			nodeLabels: map[string]string{"pool": "gpu"},
			required:   map[string]string{"pool": "gpu"},
			want:       true,
		},
		{
			name:       "non-matching",
			nodeLabels: map[string]string{"pool": "cpu"},
			required:   map[string]string{"pool": "gpu"},
			want:       false,
		},
		{
			name:       "partial match",
			nodeLabels: map[string]string{"pool": "gpu"},
			required:   map[string]string{"pool": "gpu", "region": "us-west"},
			want:       false,
		},
		{
			name:       "superset match",
			nodeLabels: map[string]string{"pool": "gpu", "region": "us-west", "env": "prod"},
			required:   map[string]string{"pool": "gpu", "region": "us-west"},
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesLabels(tt.nodeLabels, tt.required)
			if got != tt.want {
				t.Errorf("matchesLabels() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRegistry_OfflineTimeout(t *testing.T) {
	config := DefaultConfig()
	config.OfflineThreshold = 100 * time.Millisecond
	r := NewRegistry(config)

	r.Register(&RegisterRequest{
		NodeID:   "node-1",
		Hostname: "localhost",
	})

	// Force the LastSeen to be in the past
	r.mu.Lock()
	r.nodes["node-1"].LastSeen = time.Now().Add(-200 * time.Millisecond)
	r.mu.Unlock()

	// Run the offline check
	r.markOfflineNodes(config.OfflineThreshold)

	node, _ := r.GetNode("node-1")
	if node.Status != NodeStatusOffline {
		t.Errorf("expected status offline, got '%s'", node.Status)
	}
}
