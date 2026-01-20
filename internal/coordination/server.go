package coordination

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Server provides the HTTP coordination API.
type Server struct {
	config     *Config
	registry   *Registry
	tasks      *TaskManager
	events     *EventBus
	httpServer *http.Server

	mu sync.RWMutex
}

// NewServer creates a new coordination server.
func NewServer(config *Config) *Server {
	if config == nil {
		config = DefaultConfig()
	}

	s := &Server{
		config:   config,
		registry: NewRegistry(config),
		tasks:    NewTaskManager(config),
		events:   NewEventBus(),
	}

	return s
}

// Start starts the coordination server.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Health (unauthenticated)
	mux.HandleFunc("/api/v1/health", s.handleHealth)

	// Registration
	mux.HandleFunc("/api/v1/register", s.withAuth(s.handleRegister))
	mux.HandleFunc("/api/v1/unregister", s.withAuth(s.handleUnregister))
	mux.HandleFunc("/api/v1/heartbeat", s.withAuth(s.handleHeartbeat))

	// Nodes
	mux.HandleFunc("/api/v1/nodes", s.withAuth(s.handleNodes))
	mux.HandleFunc("/api/v1/nodes/", s.withAuth(s.handleNodeByID))

	// Tasks
	mux.HandleFunc("/api/v1/tasks", s.withAuth(s.handleTasks))
	mux.HandleFunc("/api/v1/tasks/pending", s.withAuth(s.handlePendingTasks))
	mux.HandleFunc("/api/v1/tasks/", s.withAuth(s.handleTaskByID))

	// Messages
	mux.HandleFunc("/api/v1/messages", s.withAuth(s.handleMessages))

	// State
	mux.HandleFunc("/api/v1/state/", s.withAuth(s.handleState))

	// Events
	mux.HandleFunc("/api/v1/events/stream", s.withAuth(s.handleEventStream))

	// Apply CORS
	handler := s.corsMiddleware(mux)

	s.httpServer = &http.Server{
		Addr:    s.config.ListenAddr,
		Handler: handler,
	}

	// Start background goroutines
	go s.registry.StartCleanup(ctx, s.config.OfflineThreshold)
	go s.tasks.StartCleanup(ctx, s.config.ClaimTimeout)

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.httpServer.Shutdown(shutdownCtx)
	}()

	if err := s.httpServer.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Stop stops the coordination server.
func (s *Server) Stop() error {
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// corsMiddleware adds CORS headers for browser-based clients.
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// withAuth wraps a handler with authentication.
func (s *Server) withAuth(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.config.Auth != nil && s.config.Auth.RequireAuth {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				s.sendError(w, http.StatusUnauthorized, "missing authorization", "UNAUTHORIZED")
				return
			}

			token := strings.TrimPrefix(auth, "Bearer ")
			valid := false
			for _, t := range s.config.Auth.Tokens {
				if t == token {
					valid = true
					break
				}
			}

			if !valid {
				s.sendError(w, http.StatusUnauthorized, "invalid token", "UNAUTHORIZED")
				return
			}
		}
		handler(w, r)
	}
}

// handleHealth returns server health status.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.sendSuccess(w, map[string]interface{}{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}

// handleRegister handles node registration.
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	if req.NodeID == "" {
		s.sendError(w, http.StatusBadRequest, "node_id is required", "MISSING_FIELD")
		return
	}

	resp, err := s.registry.Register(&req)
	if err != nil {
		s.sendError(w, http.StatusConflict, err.Error(), "REGISTRATION_FAILED")
		return
	}

	// Emit event
	s.events.Publish(Event{
		Type:      EventTypeNodeRegistered,
		Timestamp: time.Now(),
		NodeID:    req.NodeID,
		Data: map[string]interface{}{
			"hostname": req.Hostname,
			"capacity": req.Capacity,
		},
	})

	s.sendSuccess(w, resp)
}

// handleUnregister handles node unregistration.
func (s *Server) handleUnregister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	var req struct {
		RegistrationID string `json:"registration_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	if err := s.registry.Unregister(req.RegistrationID); err != nil {
		s.sendError(w, http.StatusNotFound, err.Error(), "NOT_FOUND")
		return
	}

	s.sendSuccess(w, map[string]bool{"unregistered": true})
}

// handleHeartbeat handles node heartbeats.
func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	var req HeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	if err := s.registry.Heartbeat(&req); err != nil {
		s.sendError(w, http.StatusNotFound, err.Error(), "NOT_REGISTERED")
		return
	}

	s.sendSuccess(w, HeartbeatResponse{
		Acknowledged: true,
	})
}

// handleNodes handles node listing.
func (s *Server) handleNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	nodes := s.registry.ListNodes()
	s.sendSuccess(w, map[string]interface{}{
		"nodes": nodes,
		"count": len(nodes),
	})
}

// handleNodeByID handles operations on a specific node.
func (s *Server) handleNodeByID(w http.ResponseWriter, r *http.Request) {
	nodeID := strings.TrimPrefix(r.URL.Path, "/api/v1/nodes/")
	nodeID = strings.TrimSuffix(nodeID, "/")

	if nodeID == "" {
		s.sendError(w, http.StatusBadRequest, "node_id required", "MISSING_FIELD")
		return
	}

	switch r.Method {
	case http.MethodGet:
		node, err := s.registry.GetNode(nodeID)
		if err != nil {
			s.sendError(w, http.StatusNotFound, err.Error(), "NOT_FOUND")
			return
		}
		s.sendSuccess(w, node)

	default:
		s.sendError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
	}
}

// handleTasks handles task operations.
func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		status := TaskStatus(r.URL.Query().Get("status"))
		tasks := s.tasks.List(status)
		s.sendSuccess(w, map[string]interface{}{
			"tasks": tasks,
			"count": len(tasks),
		})

	case http.MethodPost:
		var req CreateTaskRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.sendError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
			return
		}

		task, err := s.tasks.Create(&req)
		if err != nil {
			s.sendError(w, http.StatusBadRequest, err.Error(), "CREATE_FAILED")
			return
		}

		s.events.Publish(Event{
			Type:      EventTypeTaskCreated,
			Timestamp: time.Now(),
			Repo:      task.Repo,
			TaskID:    task.ID,
			Data: map[string]interface{}{
				"description": task.Description,
				"priority":    task.Priority,
			},
		})

		s.sendSuccess(w, task)

	default:
		s.sendError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
	}
}

// handlePendingTasks handles pending task queries.
func (s *Server) handlePendingTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	// Parse label filters
	labels := make(map[string]string)
	for _, l := range r.URL.Query()["label"] {
		parts := strings.SplitN(l, ":", 2)
		if len(parts) == 2 {
			labels[parts[0]] = parts[1]
		}
	}

	tasks := s.tasks.GetPending(labels)
	s.sendSuccess(w, map[string]interface{}{
		"tasks": tasks,
		"count": len(tasks),
	})
}

// handleTaskByID handles operations on a specific task.
func (s *Server) handleTaskByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/tasks/")
	parts := strings.Split(path, "/")

	if len(parts) == 0 || parts[0] == "" {
		s.sendError(w, http.StatusBadRequest, "task_id required", "MISSING_FIELD")
		return
	}

	taskID := parts[0]

	// Check for claim endpoint
	if len(parts) > 1 && parts[1] == "claim" {
		s.handleTaskClaim(w, r, taskID)
		return
	}

	switch r.Method {
	case http.MethodGet:
		task, err := s.tasks.Get(taskID)
		if err != nil {
			s.sendError(w, http.StatusNotFound, err.Error(), "NOT_FOUND")
			return
		}
		s.sendSuccess(w, task)

	case http.MethodPut:
		var req TaskUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.sendError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
			return
		}

		if err := s.tasks.Update(taskID, &req); err != nil {
			s.sendError(w, http.StatusBadRequest, err.Error(), "UPDATE_FAILED")
			return
		}

		// Emit appropriate event
		eventType := EventTypeTaskCompleted
		if req.Status == TaskStatusFailed {
			eventType = EventTypeTaskFailed
		}
		s.events.Publish(Event{
			Type:      eventType,
			Timestamp: time.Now(),
			TaskID:    taskID,
			Data: map[string]interface{}{
				"status": req.Status,
			},
		})

		s.sendSuccess(w, map[string]bool{"updated": true})

	default:
		s.sendError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
	}
}

// handleTaskClaim handles task claiming.
func (s *Server) handleTaskClaim(w http.ResponseWriter, r *http.Request, taskID string) {
	if r.Method != http.MethodPost {
		s.sendError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	var req TaskClaimRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	// Verify registration
	node, err := s.registry.GetNodeByRegistration(req.RegistrationID)
	if err != nil {
		s.sendError(w, http.StatusUnauthorized, "invalid registration", "NOT_REGISTERED")
		return
	}

	task, err := s.tasks.Claim(taskID, node.ID, req.WorkerName)
	if err != nil {
		s.sendSuccess(w, TaskClaimResponse{
			Claimed: false,
			Error:   err.Error(),
		})
		return
	}

	// Emit event
	s.events.Publish(Event{
		Type:      EventTypeTaskClaimed,
		Timestamp: time.Now(),
		NodeID:    node.ID,
		TaskID:    taskID,
		Data: map[string]interface{}{
			"worker": req.WorkerName,
		},
	})

	s.sendSuccess(w, TaskClaimResponse{
		Claimed: true,
		Task:    task,
	})
}

// handleMessages handles message operations.
func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Get messages for an agent
		agent := r.URL.Query().Get("agent")
		if agent == "" {
			s.sendError(w, http.StatusBadRequest, "agent parameter required", "MISSING_FIELD")
			return
		}
		// For now, return empty list - message storage would need additional implementation
		s.sendSuccess(w, map[string]interface{}{
			"messages": []Message{},
		})

	case http.MethodPost:
		var msg Message
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			s.sendError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
			return
		}

		// Emit event for message routing
		s.events.Publish(Event{
			Type:      EventTypeMessageRouted,
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"from": msg.From,
				"to":   msg.To,
				"repo": msg.Repo,
			},
		})

		s.sendSuccess(w, map[string]string{
			"message_id": msg.ID,
			"status":     "routed",
		})

	default:
		s.sendError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
	}
}

// handleState handles state queries.
func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	repo := strings.TrimPrefix(r.URL.Path, "/api/v1/state/")
	repo = strings.TrimSuffix(repo, "/")

	if repo == "" {
		s.sendError(w, http.StatusBadRequest, "repo name required", "MISSING_FIELD")
		return
	}

	// Build state response from registry and tasks
	state := &StateResponse{
		Repo:   repo,
		Agents: make(map[string]*AgentState),
		Nodes:  []string{},
	}

	// Collect agents from all nodes
	for _, node := range s.registry.ListNodes() {
		state.Nodes = append(state.Nodes, node.ID)
		for _, agent := range node.Agents {
			state.Agents[agent.Name] = &AgentState{
				Node:   node.ID,
				Status: string(agent.Status),
				Task:   agent.Task,
			}
		}
	}

	// Count pending tasks for this repo
	for _, task := range s.tasks.List(TaskStatusPending) {
		if task.Repo == repo {
			state.PendingTasks++
		}
	}

	s.sendSuccess(w, state)
}

// handleEventStream handles SSE event streaming.
func (s *Server) handleEventStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.sendError(w, http.StatusInternalServerError, "SSE not supported", "SSE_UNSUPPORTED")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Subscribe to events
	events := s.events.Subscribe()
	defer s.events.Unsubscribe(events)

	// Send initial connected event
	fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"ok\"}\n\n")
	flusher.Flush()

	for {
		select {
		case event := <-events:
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, string(data))
			flusher.Flush()

		case <-r.Context().Done():
			return
		}
	}
}

// sendSuccess sends a successful API response.
func (s *Server) sendSuccess(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    data,
	})
}

// sendError sends an error API response.
func (s *Server) sendError(w http.ResponseWriter, status int, message, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(APIResponse{
		Success: false,
		Error:   message,
		Code:    code,
	})
}
