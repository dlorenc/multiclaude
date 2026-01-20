package notify

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// HubConfig configures the NotificationHub
type HubConfig struct {
	// RateLimit is the maximum notifications per minute per adapter
	RateLimit int `yaml:"rate_limit" json:"rate_limit"`
	// CooldownAfterBurst is seconds to wait after hitting rate limit
	CooldownAfterBurst int `yaml:"cooldown_after_burst" json:"cooldown_after_burst"`
	// DedupeWindow is how long to deduplicate identical events
	DedupeWindow time.Duration `yaml:"dedupe_window" json:"dedupe_window"`
	// QuietHours configures notification quiet periods
	QuietHours *QuietHours `yaml:"quiet_hours" json:"quiet_hours,omitempty"`
}

// QuietHours defines when notifications should be suppressed
type QuietHours struct {
	Enabled  bool   `yaml:"enabled" json:"enabled"`
	Start    string `yaml:"start" json:"start"`       // "22:00"
	End      string `yaml:"end" json:"end"`           // "08:00"
	Timezone string `yaml:"timezone" json:"timezone"` // "America/Los_Angeles"
}

// DefaultHubConfig returns sensible defaults
func DefaultHubConfig() *HubConfig {
	return &HubConfig{
		RateLimit:          10,
		CooldownAfterBurst: 60,
		DedupeWindow:       5 * time.Minute,
	}
}

// Hub is the central notification coordinator
type Hub struct {
	config   *HubConfig
	adapters map[string]adapterEntry
	filters  map[string]*EventFilter // adapter name -> filter

	// Rate limiting state
	rateLimits map[string]*rateLimitState
	rateMu     sync.Mutex

	// Deduplication state
	recentEvents map[string]time.Time // event fingerprint -> timestamp
	dedupMu      sync.Mutex

	// Response handling
	responseHandler ResponseHandler
	pendingEvents   map[string]*Event // responseID -> event
	pendingMu       sync.RWMutex

	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

type adapterEntry struct {
	adapter Adapter
	filter  *EventFilter
}

type rateLimitState struct {
	count     int
	window    time.Time
	cooldown  time.Time
}

// NewHub creates a new NotificationHub
func NewHub(config *HubConfig) *Hub {
	if config == nil {
		config = DefaultHubConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Hub{
		config:        config,
		adapters:      make(map[string]adapterEntry),
		filters:       make(map[string]*EventFilter),
		rateLimits:    make(map[string]*rateLimitState),
		recentEvents:  make(map[string]time.Time),
		pendingEvents: make(map[string]*Event),
		ctx:           ctx,
		cancel:        cancel,
	}
}

// RegisterAdapter adds an adapter to the hub
func (h *Hub) RegisterAdapter(adapter Adapter, filter *EventFilter) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	name := adapter.Name()
	if _, exists := h.adapters[name]; exists {
		return fmt.Errorf("adapter %q already registered", name)
	}

	h.adapters[name] = adapterEntry{
		adapter: adapter,
		filter:  filter,
	}

	// Set up response handler for interactive adapters
	if ia, ok := adapter.(InteractiveAdapter); ok {
		ia.SetResponseHandler(h.handleResponse)
	}

	return nil
}

// UnregisterAdapter removes an adapter from the hub
func (h *Hub) UnregisterAdapter(name string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	entry, exists := h.adapters[name]
	if !exists {
		return fmt.Errorf("adapter %q not found", name)
	}

	if err := entry.adapter.Close(); err != nil {
		return fmt.Errorf("failed to close adapter: %w", err)
	}

	delete(h.adapters, name)
	return nil
}

// Start starts the hub and all interactive adapters
func (h *Hub) Start() error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for name, entry := range h.adapters {
		if ia, ok := entry.adapter.(InteractiveAdapter); ok {
			h.wg.Add(1)
			go func(name string, ia InteractiveAdapter) {
				defer h.wg.Done()
				if err := ia.Start(h.ctx); err != nil {
					// Log error but don't fail the whole hub
					fmt.Printf("adapter %s start error: %v\n", name, err)
				}
			}(name, ia)
		}
	}

	// Start cleanup goroutine for dedup cache
	h.wg.Add(1)
	go h.cleanupLoop()

	return nil
}

// Stop stops the hub and all adapters
func (h *Hub) Stop() error {
	h.cancel()
	h.wg.Wait()

	h.mu.Lock()
	defer h.mu.Unlock()

	var errs []error
	for _, entry := range h.adapters {
		if err := entry.adapter.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing adapters: %v", errs)
	}
	return nil
}

// Notify sends an event to all matching adapters
func (h *Hub) Notify(event *Event) error {
	// Ensure event has an ID
	if event.ID == "" {
		event.ID = fmt.Sprintf("evt-%s", uuid.New().String()[:13])
	}

	// Set timestamp if not set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Set priority if not set
	if event.Priority == "" {
		event.Priority = EventPriority(event.Type)
	}

	// Check quiet hours
	if h.isQuietHours() {
		return nil
	}

	// Check deduplication
	if h.isDuplicate(event) {
		return nil
	}

	// Store for response handling if needed
	if event.ActionRequired && event.ResponseID == "" {
		event.ResponseID = fmt.Sprintf("resp-%s", uuid.New().String()[:13])
		h.pendingMu.Lock()
		h.pendingEvents[event.ResponseID] = event
		h.pendingMu.Unlock()
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	var errs []error
	for name, entry := range h.adapters {
		// Check filter
		if entry.filter != nil && !entry.filter.Matches(event) {
			continue
		}

		// Check rate limit
		if !h.checkRateLimit(name) {
			continue
		}

		// Send notification
		ctx, cancel := context.WithTimeout(h.ctx, 10*time.Second)
		if err := entry.adapter.Send(ctx, event); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
		cancel()
	}

	if len(errs) > 0 {
		return fmt.Errorf("notification errors: %v", errs)
	}
	return nil
}

// SetResponseHandler sets the callback for responses
func (h *Hub) SetResponseHandler(handler ResponseHandler) {
	h.responseHandler = handler
}

// GetPendingEvent retrieves a pending event by response ID
func (h *Hub) GetPendingEvent(responseID string) *Event {
	h.pendingMu.RLock()
	defer h.pendingMu.RUnlock()
	return h.pendingEvents[responseID]
}

// handleResponse processes responses from adapters
func (h *Hub) handleResponse(response *Response) {
	if h.responseHandler != nil {
		h.responseHandler(response)
	}
}

// isDuplicate checks if we've seen this event recently
func (h *Hub) isDuplicate(event *Event) bool {
	fingerprint := h.eventFingerprint(event)

	h.dedupMu.Lock()
	defer h.dedupMu.Unlock()

	if lastSeen, exists := h.recentEvents[fingerprint]; exists {
		if time.Since(lastSeen) < h.config.DedupeWindow {
			return true
		}
	}

	h.recentEvents[fingerprint] = time.Now()
	return false
}

// eventFingerprint creates a unique identifier for deduplication
func (h *Hub) eventFingerprint(event *Event) string {
	return fmt.Sprintf("%s:%s:%s:%s", event.Type, event.RepoName, event.AgentName, event.Title)
}

// checkRateLimit checks and updates rate limit for an adapter
func (h *Hub) checkRateLimit(adapterName string) bool {
	h.rateMu.Lock()
	defer h.rateMu.Unlock()

	state, exists := h.rateLimits[adapterName]
	if !exists {
		state = &rateLimitState{
			window: time.Now(),
		}
		h.rateLimits[adapterName] = state
	}

	// Check cooldown
	if !state.cooldown.IsZero() && time.Now().Before(state.cooldown) {
		return false
	}
	state.cooldown = time.Time{}

	// Reset window if expired
	if time.Since(state.window) > time.Minute {
		state.count = 0
		state.window = time.Now()
	}

	// Check rate limit
	if state.count >= h.config.RateLimit {
		state.cooldown = time.Now().Add(time.Duration(h.config.CooldownAfterBurst) * time.Second)
		return false
	}

	state.count++
	return true
}

// isQuietHours checks if we're currently in quiet hours
func (h *Hub) isQuietHours() bool {
	if h.config.QuietHours == nil || !h.config.QuietHours.Enabled {
		return false
	}

	qh := h.config.QuietHours
	now := time.Now()

	// Parse timezone
	loc, err := time.LoadLocation(qh.Timezone)
	if err != nil {
		loc = time.Local
	}
	now = now.In(loc)

	// Parse start and end times
	startTime, err := time.Parse("15:04", qh.Start)
	if err != nil {
		return false
	}
	endTime, err := time.Parse("15:04", qh.End)
	if err != nil {
		return false
	}

	// Create times for today
	start := time.Date(now.Year(), now.Month(), now.Day(),
		startTime.Hour(), startTime.Minute(), 0, 0, loc)
	end := time.Date(now.Year(), now.Month(), now.Day(),
		endTime.Hour(), endTime.Minute(), 0, 0, loc)

	// Handle overnight quiet hours (e.g., 22:00 - 08:00)
	if end.Before(start) {
		// We're in overnight mode
		if now.After(start) || now.Before(end) {
			return true
		}
	} else {
		// Same day quiet hours
		if now.After(start) && now.Before(end) {
			return true
		}
	}

	return false
}

// cleanupLoop periodically cleans up old dedup entries
func (h *Hub) cleanupLoop() {
	defer h.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.cleanupDedupCache()
			h.cleanupPendingEvents()
		case <-h.ctx.Done():
			return
		}
	}
}

// cleanupDedupCache removes old entries from the dedup cache
func (h *Hub) cleanupDedupCache() {
	h.dedupMu.Lock()
	defer h.dedupMu.Unlock()

	cutoff := time.Now().Add(-h.config.DedupeWindow)
	for fp, ts := range h.recentEvents {
		if ts.Before(cutoff) {
			delete(h.recentEvents, fp)
		}
	}
}

// cleanupPendingEvents removes old pending events
func (h *Hub) cleanupPendingEvents() {
	h.pendingMu.Lock()
	defer h.pendingMu.Unlock()

	cutoff := time.Now().Add(-24 * time.Hour)
	for id, event := range h.pendingEvents {
		if event.Timestamp.Before(cutoff) {
			delete(h.pendingEvents, id)
		}
	}
}

// GetAdapters returns a list of registered adapter names
func (h *Hub) GetAdapters() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	names := make([]string, 0, len(h.adapters))
	for name := range h.adapters {
		names = append(names, name)
	}
	return names
}

// GetStats returns notification statistics
func (h *Hub) GetStats() map[string]interface{} {
	h.rateMu.Lock()
	defer h.rateMu.Unlock()

	h.dedupMu.Lock()
	defer h.dedupMu.Unlock()

	h.pendingMu.RLock()
	defer h.pendingMu.RUnlock()

	stats := make(map[string]interface{})
	stats["adapters"] = len(h.adapters)
	stats["pending_events"] = len(h.pendingEvents)
	stats["dedup_cache_size"] = len(h.recentEvents)

	adapterStats := make(map[string]interface{})
	for name, state := range h.rateLimits {
		adapterStats[name] = map[string]interface{}{
			"count_this_minute": state.count,
			"in_cooldown":       !state.cooldown.IsZero() && time.Now().Before(state.cooldown),
		}
	}
	stats["rate_limits"] = adapterStats

	return stats
}
