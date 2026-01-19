package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// APIServer provides a REST API for dashboard integration
type APIServer struct {
	config    *APIConfig
	hub       *Hub
	server    *http.Server

	// SSE subscribers
	sseClients   map[string]chan *Event
	sseMu        sync.RWMutex

	// Recent events cache for polling clients
	recentEvents []*Event
	eventsMu     sync.RWMutex
	maxEvents    int

	// Status providers for /status endpoint
	statusProviders []StatusProvider
	providerMu      sync.RWMutex
}

// StatusProvider provides status information for the API
type StatusProvider interface {
	// GetStatus returns current status for a repository
	GetStatus(repoName string) (*StatusSummary, error)
	// ListRepos returns all tracked repositories
	ListRepos() []string
}

// NewAPIServer creates a new API server
func NewAPIServer(config *APIConfig, hub *Hub) *APIServer {
	return &APIServer{
		config:       config,
		hub:          hub,
		sseClients:   make(map[string]chan *Event),
		recentEvents: make([]*Event, 0),
		maxEvents:    100,
	}
}

// RegisterStatusProvider adds a status provider
func (a *APIServer) RegisterStatusProvider(provider StatusProvider) {
	a.providerMu.Lock()
	defer a.providerMu.Unlock()
	a.statusProviders = append(a.statusProviders, provider)
}

// Start starts the API server
func (a *APIServer) Start(ctx context.Context) error {
	if a.config == nil || !a.config.Enabled {
		return nil
	}

	mux := http.NewServeMux()

	// Public endpoints
	mux.HandleFunc("/api/v1/health", a.handleHealth)

	// Protected endpoints
	mux.HandleFunc("/api/v1/events", a.withAuth(a.handleEvents))
	mux.HandleFunc("/api/v1/events/stream", a.withAuth(a.handleSSE))
	mux.HandleFunc("/api/v1/status", a.withAuth(a.handleStatus))
	mux.HandleFunc("/api/v1/status/", a.withAuth(a.handleRepoStatus))
	mux.HandleFunc("/api/v1/respond", a.withAuth(a.handleRespond))
	mux.HandleFunc("/api/v1/adapters", a.withAuth(a.handleAdapters))
	mux.HandleFunc("/api/v1/stats", a.withAuth(a.handleStats))

	// Apply CORS if configured
	handler := a.corsMiddleware(mux)

	a.server = &http.Server{
		Addr:    a.config.ListenAddr,
		Handler: handler,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		a.server.Shutdown(shutdownCtx)
	}()

	if err := a.server.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Stop stops the API server
func (a *APIServer) Stop() error {
	if a.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return a.server.Shutdown(ctx)
	}
	return nil
}

// BroadcastEvent sends an event to all SSE clients and caches it
func (a *APIServer) BroadcastEvent(event *Event) {
	// Add to recent events cache
	a.eventsMu.Lock()
	a.recentEvents = append(a.recentEvents, event)
	if len(a.recentEvents) > a.maxEvents {
		a.recentEvents = a.recentEvents[1:]
	}
	a.eventsMu.Unlock()

	// Broadcast to SSE clients
	a.sseMu.RLock()
	defer a.sseMu.RUnlock()

	for _, ch := range a.sseClients {
		select {
		case ch <- event:
		default:
			// Client channel full, skip
		}
	}
}

// corsMiddleware adds CORS headers
func (a *APIServer) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(a.config.CORSOrigins) > 0 {
			origin := r.Header.Get("Origin")
			for _, allowed := range a.config.CORSOrigins {
				if allowed == "*" || allowed == origin {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
					break
				}
			}
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// withAuth wraps a handler with authentication
func (a *APIServer) withAuth(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.config.AuthToken != "" {
			auth := r.Header.Get("Authorization")
			expected := "Bearer " + a.config.AuthToken
			if auth != expected {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		handler(w, r)
	}
}

// handleHealth returns server health status
func (a *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}

// handleEvents returns recent events
func (a *APIServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	a.eventsMu.RLock()
	events := make([]*Event, len(a.recentEvents))
	copy(events, a.recentEvents)
	a.eventsMu.RUnlock()

	// Filter by query params
	typeFilter := r.URL.Query().Get("type")
	repoFilter := r.URL.Query().Get("repo")

	if typeFilter != "" || repoFilter != "" {
		filtered := make([]*Event, 0)
		for _, event := range events {
			if typeFilter != "" && string(event.Type) != typeFilter {
				continue
			}
			if repoFilter != "" && event.RepoName != repoFilter {
				continue
			}
			filtered = append(filtered, event)
		}
		events = filtered
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"events": events,
		"count":  len(events),
	})
}

// handleSSE provides server-sent events stream
func (a *APIServer) handleSSE(w http.ResponseWriter, r *http.Request) {
	if !a.config.EnableSSE {
		http.Error(w, "SSE not enabled", http.StatusNotFound)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Create client channel
	clientID := fmt.Sprintf("client-%d", time.Now().UnixNano())
	eventCh := make(chan *Event, 10)

	a.sseMu.Lock()
	a.sseClients[clientID] = eventCh
	a.sseMu.Unlock()

	defer func() {
		a.sseMu.Lock()
		delete(a.sseClients, clientID)
		close(eventCh)
		a.sseMu.Unlock()
	}()

	// Send initial connection event
	fmt.Fprintf(w, "event: connected\ndata: {\"client_id\":\"%s\"}\n\n", clientID)
	flusher.Flush()

	// Stream events
	for {
		select {
		case event := <-eventCh:
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "event: notification\ndata: %s\n\n", string(data))
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// handleStatus returns overall status
func (a *APIServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	a.providerMu.RLock()
	providers := a.statusProviders
	a.providerMu.RUnlock()

	response := map[string]interface{}{
		"time":   time.Now().Format(time.RFC3339),
		"repos":  []interface{}{},
	}

	var repos []interface{}
	for _, provider := range providers {
		for _, repoName := range provider.ListRepos() {
			status, err := provider.GetStatus(repoName)
			if err != nil {
				continue
			}
			repos = append(repos, status)
		}
	}
	response["repos"] = repos

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleRepoStatus returns status for a specific repository
func (a *APIServer) handleRepoStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract repo name from path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/status/")
	repoName := strings.TrimSuffix(path, "/")

	if repoName == "" {
		http.Error(w, "repo name required", http.StatusBadRequest)
		return
	}

	a.providerMu.RLock()
	providers := a.statusProviders
	a.providerMu.RUnlock()

	for _, provider := range providers {
		status, err := provider.GetStatus(repoName)
		if err != nil {
			continue
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
		return
	}

	http.Error(w, "repo not found", http.StatusNotFound)
}

// handleRespond handles responses to events
func (a *APIServer) handleRespond(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ResponseID string `json:"response_id"`
		Message    string `json:"message"`
		Action     string `json:"action,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if req.ResponseID == "" {
		http.Error(w, "response_id is required", http.StatusBadRequest)
		return
	}

	// Verify the event exists
	event := a.hub.GetPendingEvent(req.ResponseID)
	if event == nil {
		http.Error(w, "event not found or expired", http.StatusNotFound)
		return
	}

	// Create and dispatch response
	response := &Response{
		EventID:    event.ID,
		ResponseID: req.ResponseID,
		Message:    req.Message,
		Action:     req.Action,
		Source:     "api",
		Timestamp:  time.Now(),
	}

	a.hub.handleResponse(response)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"event_id": event.ID,
	})
}

// handleAdapters returns information about configured adapters
func (a *APIServer) handleAdapters(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	adapters := a.hub.GetAdapters()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"adapters": adapters,
		"count":    len(adapters),
	})
}

// handleStats returns notification statistics
func (a *APIServer) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats := a.hub.GetStats()

	a.sseMu.RLock()
	stats["sse_clients"] = len(a.sseClients)
	a.sseMu.RUnlock()

	a.eventsMu.RLock()
	stats["cached_events"] = len(a.recentEvents)
	a.eventsMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// APIResponse represents a standard API response
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// SendAPIResponse sends a JSON API response
func SendAPIResponse(w http.ResponseWriter, status int, data interface{}, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	response := APIResponse{Success: err == nil, Data: data}
	if err != nil {
		response.Error = err.Error()
	}

	json.NewEncoder(w).Encode(response)
}
