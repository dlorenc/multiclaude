package coordination

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Registry manages node registration and health tracking.
type Registry struct {
	config *Config

	// nodes maps node ID to Node
	nodes map[string]*Node
	// registrations maps registration ID to node ID
	registrations map[string]string

	mu sync.RWMutex
}

// NewRegistry creates a new node registry.
func NewRegistry(config *Config) *Registry {
	return &Registry{
		config:        config,
		nodes:         make(map[string]*Node),
		registrations: make(map[string]string),
	}
}

// Register registers a new node or updates an existing registration.
func (r *Registry) Register(req *RegisterRequest) (*RegisterResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if node already registered
	if existing, exists := r.nodes[req.NodeID]; exists {
		// Update existing registration
		existing.Hostname = req.Hostname
		existing.Capacity = req.Capacity
		existing.Labels = req.Labels
		existing.Status = NodeStatusOnline
		existing.LastSeen = time.Now()

		// Find existing registration ID
		for regID, nodeID := range r.registrations {
			if nodeID == req.NodeID {
				return &RegisterResponse{
					RegistrationID:           regID,
					HeartbeatIntervalSeconds: int(r.config.HeartbeatInterval.Seconds()),
				}, nil
			}
		}
	}

	// Create new registration
	regID := fmt.Sprintf("reg-%s", uuid.New().String()[:8])

	node := &Node{
		ID:       req.NodeID,
		Hostname: req.Hostname,
		Capacity: req.Capacity,
		Labels:   req.Labels,
		Status:   NodeStatusOnline,
		LastSeen: time.Now(),
	}

	r.nodes[req.NodeID] = node
	r.registrations[regID] = req.NodeID

	return &RegisterResponse{
		RegistrationID:           regID,
		HeartbeatIntervalSeconds: int(r.config.HeartbeatInterval.Seconds()),
	}, nil
}

// Unregister removes a node's registration.
func (r *Registry) Unregister(registrationID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	nodeID, exists := r.registrations[registrationID]
	if !exists {
		return fmt.Errorf("registration %q not found", registrationID)
	}

	delete(r.registrations, registrationID)
	delete(r.nodes, nodeID)

	return nil
}

// Heartbeat updates a node's health status.
func (r *Registry) Heartbeat(req *HeartbeatRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	nodeID, exists := r.registrations[req.RegistrationID]
	if !exists {
		return fmt.Errorf("registration %q not found", req.RegistrationID)
	}

	node, exists := r.nodes[nodeID]
	if !exists {
		return fmt.Errorf("node %q not found", nodeID)
	}

	node.Status = req.Status
	node.LastSeen = time.Now()
	node.Agents = req.Agents
	node.Metrics = req.Metrics

	// Update capacity based on current agents
	if req.Agents != nil {
		node.Capacity.CurrentWorkers = len(req.Agents)
	}

	return nil
}

// GetNode retrieves a node by ID.
func (r *Registry) GetNode(nodeID string) (*Node, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	node, exists := r.nodes[nodeID]
	if !exists {
		return nil, fmt.Errorf("node %q not found", nodeID)
	}

	// Return a copy to prevent mutation
	nodeCopy := *node
	return &nodeCopy, nil
}

// GetNodeByRegistration retrieves a node by registration ID.
func (r *Registry) GetNodeByRegistration(registrationID string) (*Node, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	nodeID, exists := r.registrations[registrationID]
	if !exists {
		return nil, fmt.Errorf("registration %q not found", registrationID)
	}

	node, exists := r.nodes[nodeID]
	if !exists {
		return nil, fmt.Errorf("node %q not found", nodeID)
	}

	nodeCopy := *node
	return &nodeCopy, nil
}

// ListNodes returns all registered nodes.
func (r *Registry) ListNodes() []Node {
	r.mu.RLock()
	defer r.mu.RUnlock()

	nodes := make([]Node, 0, len(r.nodes))
	for _, node := range r.nodes {
		nodes = append(nodes, *node)
	}

	return nodes
}

// ListOnlineNodes returns only online nodes.
func (r *Registry) ListOnlineNodes() []Node {
	r.mu.RLock()
	defer r.mu.RUnlock()

	nodes := make([]Node, 0)
	for _, node := range r.nodes {
		if node.Status == NodeStatusOnline {
			nodes = append(nodes, *node)
		}
	}

	return nodes
}

// ListAvailableNodes returns nodes with available worker capacity.
func (r *Registry) ListAvailableNodes() []Node {
	r.mu.RLock()
	defer r.mu.RUnlock()

	nodes := make([]Node, 0)
	for _, node := range r.nodes {
		if node.Status == NodeStatusOnline &&
			node.Capacity.CurrentWorkers < node.Capacity.MaxWorkers {
			nodes = append(nodes, *node)
		}
	}

	return nodes
}

// FindNodesByLabel returns nodes matching the given labels.
func (r *Registry) FindNodesByLabel(labels map[string]string) []Node {
	r.mu.RLock()
	defer r.mu.RUnlock()

	nodes := make([]Node, 0)
	for _, node := range r.nodes {
		if node.Status != NodeStatusOnline {
			continue
		}
		if matchesLabels(node.Labels, labels) {
			nodes = append(nodes, *node)
		}
	}

	return nodes
}

// StartCleanup starts the background cleanup goroutine.
func (r *Registry) StartCleanup(ctx context.Context, offlineThreshold time.Duration) {
	ticker := time.NewTicker(offlineThreshold / 3)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.markOfflineNodes(offlineThreshold)
		case <-ctx.Done():
			return
		}
	}
}

// markOfflineNodes marks nodes as offline if they haven't sent a heartbeat recently.
func (r *Registry) markOfflineNodes(threshold time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	cutoff := time.Now().Add(-threshold)

	for _, node := range r.nodes {
		if node.Status == NodeStatusOnline && node.LastSeen.Before(cutoff) {
			node.Status = NodeStatusOffline
		}
	}
}

// GetStats returns registry statistics.
func (r *Registry) GetStats() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	online := 0
	offline := 0
	draining := 0
	totalCapacity := 0
	usedCapacity := 0

	for _, node := range r.nodes {
		switch node.Status {
		case NodeStatusOnline:
			online++
			totalCapacity += node.Capacity.MaxWorkers
			usedCapacity += node.Capacity.CurrentWorkers
		case NodeStatusOffline:
			offline++
		case NodeStatusDraining:
			draining++
		}
	}

	return map[string]interface{}{
		"total_nodes":    len(r.nodes),
		"online":         online,
		"offline":        offline,
		"draining":       draining,
		"total_capacity": totalCapacity,
		"used_capacity":  usedCapacity,
		"available":      totalCapacity - usedCapacity,
	}
}

// matchesLabels checks if node labels match the required labels.
func matchesLabels(nodeLabels, required map[string]string) bool {
	if len(required) == 0 {
		return true
	}

	for k, v := range required {
		if nodeLabels[k] != v {
			return false
		}
	}

	return true
}
