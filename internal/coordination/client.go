package coordination

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	// DefaultAPITimeout is the default timeout for API requests
	DefaultAPITimeout = 10 * time.Second
	// DefaultCacheTTL is the default time-to-live for cached entries
	DefaultCacheTTL = 30 * time.Second
	// DefaultHeartbeatInterval is the default interval for sending heartbeats
	DefaultHeartbeatInterval = 30 * time.Second
	// DefaultMessagePollInterval is the default interval for polling remote messages
	DefaultMessagePollInterval = 5 * time.Second
)

// ClientConfig holds configuration for the coordination client
type ClientConfig struct {
	// BaseURL is the base URL of the Coordination API
	BaseURL string
	// APIToken is the authentication token
	APIToken string
	// Timeout is the HTTP request timeout
	Timeout time.Duration
	// CacheTTL is the time-to-live for cached entries
	CacheTTL time.Duration
	// EnableCache enables local caching of registry data
	EnableCache bool
}

// DefaultClientConfig returns a ClientConfig with sensible defaults
func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		Timeout:     DefaultAPITimeout,
		CacheTTL:    DefaultCacheTTL,
		EnableCache: true,
	}
}

// Client communicates with the remote Coordination API.
// It implements the Registry interface and provides additional
// methods for spawn and message operations.
type Client struct {
	config     ClientConfig
	httpClient *http.Client
	cache      *clientCache
	mu         sync.RWMutex
}

// clientCache provides local caching of agent information
type clientCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	ttl     time.Duration
}

// cacheEntry holds a cached agent with its fetch time
type cacheEntry struct {
	agent     *AgentInfo
	fetchedAt time.Time
}

// newClientCache creates a new client cache
func newClientCache(ttl time.Duration) *clientCache {
	return &clientCache{
		entries: make(map[string]*cacheEntry),
		ttl:     ttl,
	}
}

// cacheKey generates a cache key from repo and agent name
func cacheKey(repoName, agentName string) string {
	return repoName + "/" + agentName
}

// Get retrieves an agent from the cache if it exists and is not expired
func (c *clientCache) Get(repoName, agentName string) (*AgentInfo, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[cacheKey(repoName, agentName)]
	if !ok {
		return nil, false
	}

	if time.Since(entry.fetchedAt) > c.ttl {
		return nil, false
	}

	// Return a copy
	copy := *entry.agent
	return &copy, true
}

// Set stores an agent in the cache
func (c *clientCache) Set(repoName, agentName string, agent *AgentInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()

	copy := *agent
	c.entries[cacheKey(repoName, agentName)] = &cacheEntry{
		agent:     &copy,
		fetchedAt: time.Now(),
	}
}

// Delete removes an agent from the cache
func (c *clientCache) Delete(repoName, agentName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, cacheKey(repoName, agentName))
}

// Clear removes all entries from the cache
func (c *clientCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*cacheEntry)
}

// NewClient creates a new coordination client
func NewClient(config ClientConfig) (*Client, error) {
	if config.BaseURL == "" {
		return nil, fmt.Errorf("coordination API base URL is required")
	}

	// Validate URL format
	_, err := url.Parse(config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid coordination API URL: %w", err)
	}

	// Apply defaults
	if config.Timeout == 0 {
		config.Timeout = DefaultAPITimeout
	}
	if config.CacheTTL == 0 {
		config.CacheTTL = DefaultCacheTTL
	}

	client := &Client{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}

	if config.EnableCache {
		client.cache = newClientCache(config.CacheTTL)
	}

	return client, nil
}

// NewClientFromHybridConfig creates a client from HybridConfig
func NewClientFromHybridConfig(cfg HybridConfig) (*Client, error) {
	if !cfg.Enabled {
		return nil, fmt.Errorf("hybrid mode is not enabled")
	}

	return NewClient(ClientConfig{
		BaseURL:     cfg.CoordinationAPIURL,
		APIToken:    cfg.APIToken,
		Timeout:     DefaultAPITimeout,
		CacheTTL:    DefaultCacheTTL,
		EnableCache: true,
	})
}

// apiResponse represents a generic API response
type apiResponse struct {
	Success bool            `json:"success"`
	Error   string          `json:"error,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// agentResponse is the response for single agent operations
type agentResponse struct {
	Agent *AgentInfo `json:"agent"`
}

// agentsResponse is the response for list operations
type agentsResponse struct {
	Agents []*AgentInfo `json:"agents"`
}

// spawnAPIResponse is the response for spawn operations
type spawnAPIResponse struct {
	Spawn *SpawnResponse `json:"spawn"`
}

// messageResponse is the response for single message operations
type messageResponse struct {
	Message *RoutedMessage `json:"message"`
}

// messagesResponse is the response for list message operations
type messagesResponse struct {
	Messages []*RoutedMessage `json:"messages"`
}

// doRequest performs an HTTP request to the Coordination API
func (c *Client) doRequest(method, path string, body interface{}) (*apiResponse, error) {
	fullURL := c.config.BaseURL + path

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, fullURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	if c.config.APIToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Handle non-2xx status codes
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiResp apiResponse
		if json.Unmarshal(respBody, &apiResp) == nil && apiResp.Error != "" {
			return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, apiResp.Error)
		}
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var apiResp apiResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("API returned error: %s", apiResp.Error)
	}

	return &apiResp, nil
}

// Register adds or updates an agent in the remote registry
func (c *Client) Register(agent *AgentInfo) error {
	if agent.Name == "" {
		return fmt.Errorf("agent name is required")
	}
	if agent.RepoName == "" {
		return fmt.Errorf("repo name is required")
	}

	// Set defaults
	if agent.RegisteredAt.IsZero() {
		agent.RegisteredAt = time.Now()
	}
	agent.LastHeartbeat = time.Now()
	if agent.Ownership == "" {
		agent.Ownership = GetOwnershipLevel(agent.Type)
	}

	resp, err := c.doRequest(http.MethodPost, "/api/v1/agents", agent)
	if err != nil {
		return err
	}

	// Update local cache with response
	if c.cache != nil && resp.Data != nil {
		var agentResp agentResponse
		if json.Unmarshal(resp.Data, &agentResp) == nil && agentResp.Agent != nil {
			c.cache.Set(agent.RepoName, agent.Name, agentResp.Agent)
		}
	}

	return nil
}

// Unregister removes an agent from the remote registry
func (c *Client) Unregister(repoName, agentName string) error {
	path := fmt.Sprintf("/api/v1/agents/%s/%s", url.PathEscape(repoName), url.PathEscape(agentName))
	_, err := c.doRequest(http.MethodDelete, path, nil)
	if err != nil {
		return err
	}

	// Remove from local cache
	if c.cache != nil {
		c.cache.Delete(repoName, agentName)
	}

	return nil
}

// Get retrieves an agent from the remote registry
func (c *Client) Get(repoName, agentName string) (*AgentInfo, error) {
	// Check cache first
	if c.cache != nil {
		if agent, ok := c.cache.Get(repoName, agentName); ok {
			return agent, nil
		}
	}

	path := fmt.Sprintf("/api/v1/agents/%s/%s", url.PathEscape(repoName), url.PathEscape(agentName))
	resp, err := c.doRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var agentResp agentResponse
	if err := json.Unmarshal(resp.Data, &agentResp); err != nil {
		return nil, fmt.Errorf("failed to parse agent response: %w", err)
	}

	if agentResp.Agent == nil {
		return nil, fmt.Errorf("agent %q not found in repository %q", agentName, repoName)
	}

	// Update cache
	if c.cache != nil {
		c.cache.Set(repoName, agentName, agentResp.Agent)
	}

	return agentResp.Agent, nil
}

// List returns all agents for a repository from the remote registry
func (c *Client) List(repoName string) ([]*AgentInfo, error) {
	path := fmt.Sprintf("/api/v1/agents/%s", url.PathEscape(repoName))
	resp, err := c.doRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var agentsResp agentsResponse
	if err := json.Unmarshal(resp.Data, &agentsResp); err != nil {
		return nil, fmt.Errorf("failed to parse agents response: %w", err)
	}

	// Update cache with all returned agents
	if c.cache != nil {
		for _, agent := range agentsResp.Agents {
			c.cache.Set(repoName, agent.Name, agent)
		}
	}

	return agentsResp.Agents, nil
}

// ListByType returns agents of a specific type from the remote registry
func (c *Client) ListByType(repoName, agentType string) ([]*AgentInfo, error) {
	path := fmt.Sprintf("/api/v1/agents/%s?type=%s", url.PathEscape(repoName), url.QueryEscape(agentType))
	resp, err := c.doRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var agentsResp agentsResponse
	if err := json.Unmarshal(resp.Data, &agentsResp); err != nil {
		return nil, fmt.Errorf("failed to parse agents response: %w", err)
	}

	return agentsResp.Agents, nil
}

// ListByLocation returns agents at a specific location from the remote registry
func (c *Client) ListByLocation(repoName string, location Location) ([]*AgentInfo, error) {
	path := fmt.Sprintf("/api/v1/agents/%s?location=%s", url.PathEscape(repoName), url.QueryEscape(string(location)))
	resp, err := c.doRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var agentsResp agentsResponse
	if err := json.Unmarshal(resp.Data, &agentsResp); err != nil {
		return nil, fmt.Errorf("failed to parse agents response: %w", err)
	}

	return agentsResp.Agents, nil
}

// UpdateHeartbeat updates the last heartbeat time for an agent
func (c *Client) UpdateHeartbeat(repoName, agentName string) error {
	path := fmt.Sprintf("/api/v1/agents/%s/%s/heartbeat", url.PathEscape(repoName), url.PathEscape(agentName))
	_, err := c.doRequest(http.MethodPut, path, nil)
	if err != nil {
		return err
	}

	// Update cached heartbeat time
	if c.cache != nil {
		if agent, ok := c.cache.Get(repoName, agentName); ok {
			agent.LastHeartbeat = time.Now()
			c.cache.Set(repoName, agentName, agent)
		}
	}

	return nil
}

// UpdateStatus updates the status of an agent
func (c *Client) UpdateStatus(repoName, agentName string, status AgentStatus) error {
	path := fmt.Sprintf("/api/v1/agents/%s/%s/status", url.PathEscape(repoName), url.PathEscape(agentName))

	body := map[string]string{"status": string(status)}
	_, err := c.doRequest(http.MethodPut, path, body)
	if err != nil {
		return err
	}

	// Update cached status
	if c.cache != nil {
		if agent, ok := c.cache.Get(repoName, agentName); ok {
			agent.Status = status
			agent.LastHeartbeat = time.Now()
			c.cache.Set(repoName, agentName, agent)
		}
	}

	return nil
}

// RequestSpawn requests the creation of a new remote worker
func (c *Client) RequestSpawn(req SpawnRequest) (*SpawnResponse, error) {
	if req.RepoName == "" {
		return nil, fmt.Errorf("repo name is required")
	}
	if req.Task == "" {
		return nil, fmt.Errorf("task description is required")
	}

	resp, err := c.doRequest(http.MethodPost, "/api/v1/spawn", req)
	if err != nil {
		return nil, err
	}

	var spawnResp spawnAPIResponse
	if err := json.Unmarshal(resp.Data, &spawnResp); err != nil {
		return nil, fmt.Errorf("failed to parse spawn response: %w", err)
	}

	return spawnResp.Spawn, nil
}

// GetSpawnStatus retrieves the status of a spawn request
func (c *Client) GetSpawnStatus(spawnID string) (*SpawnResponse, error) {
	path := fmt.Sprintf("/api/v1/spawn/%s", url.PathEscape(spawnID))
	resp, err := c.doRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var spawnResp spawnAPIResponse
	if err := json.Unmarshal(resp.Data, &spawnResp); err != nil {
		return nil, fmt.Errorf("failed to parse spawn response: %w", err)
	}

	return spawnResp.Spawn, nil
}

// CancelSpawn cancels a pending spawn request
func (c *Client) CancelSpawn(spawnID string) error {
	path := fmt.Sprintf("/api/v1/spawn/%s", url.PathEscape(spawnID))
	_, err := c.doRequest(http.MethodDelete, path, nil)
	return err
}

// SendMessage sends a message through the coordination API
func (c *Client) SendMessage(msg *RoutedMessage) error {
	if msg.From == "" {
		return fmt.Errorf("message sender is required")
	}
	if msg.To == "" {
		return fmt.Errorf("message recipient is required")
	}
	if msg.RepoName == "" {
		return fmt.Errorf("repo name is required")
	}

	// Set timestamp if not set
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	_, err := c.doRequest(http.MethodPost, "/api/v1/messages", msg)
	return err
}

// GetMessages retrieves pending messages for an agent
func (c *Client) GetMessages(repoName, agentName string) ([]*RoutedMessage, error) {
	path := fmt.Sprintf("/api/v1/messages/%s/%s", url.PathEscape(repoName), url.PathEscape(agentName))
	resp, err := c.doRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var msgsResp messagesResponse
	if err := json.Unmarshal(resp.Data, &msgsResp); err != nil {
		return nil, fmt.Errorf("failed to parse messages response: %w", err)
	}

	return msgsResp.Messages, nil
}

// AcknowledgeMessage marks a message as received/processed
func (c *Client) AcknowledgeMessage(messageID string) error {
	path := fmt.Sprintf("/api/v1/messages/%s/ack", url.PathEscape(messageID))
	_, err := c.doRequest(http.MethodPut, path, nil)
	return err
}

// Ping checks connectivity to the coordination API
func (c *Client) Ping() error {
	_, err := c.doRequest(http.MethodGet, "/api/v1/health", nil)
	return err
}

// ClearCache clears the local agent cache
func (c *Client) ClearCache() {
	if c.cache != nil {
		c.cache.Clear()
	}
}

// HybridRegistry wraps both local and remote registries,
// providing a unified view with fallback support.
type HybridRegistry struct {
	local      *LocalRegistry
	remote     *Client
	config     HybridConfig
	mu         sync.RWMutex
	lastRemote time.Time
}

// NewHybridRegistry creates a registry that coordinates between local and remote
func NewHybridRegistry(local *LocalRegistry, remote *Client, config HybridConfig) *HybridRegistry {
	return &HybridRegistry{
		local:  local,
		remote: remote,
		config: config,
	}
}

// shouldUseRemote determines if the operation should go to the remote registry
func (h *HybridRegistry) shouldUseRemote() bool {
	return h.config.Enabled && h.remote != nil
}

// Register adds an agent to both local and remote registries
func (h *HybridRegistry) Register(agent *AgentInfo) error {
	// Always register locally first
	if err := h.local.Register(agent); err != nil {
		return err
	}

	// Register remotely if enabled
	if h.shouldUseRemote() {
		if err := h.remote.Register(agent); err != nil {
			// If fallback is enabled, log and continue
			if h.config.FallbackToLocal {
				// Could log warning here
				return nil
			}
			return fmt.Errorf("remote registration failed: %w", err)
		}
	}

	return nil
}

// Unregister removes an agent from both registries
func (h *HybridRegistry) Unregister(repoName, agentName string) error {
	// Always unregister locally
	localErr := h.local.Unregister(repoName, agentName)

	// Unregister remotely if enabled
	if h.shouldUseRemote() {
		if err := h.remote.Unregister(repoName, agentName); err != nil {
			if !h.config.FallbackToLocal {
				return fmt.Errorf("remote unregistration failed: %w", err)
			}
		}
	}

	return localErr
}

// Get retrieves an agent, checking remote first if available
func (h *HybridRegistry) Get(repoName, agentName string) (*AgentInfo, error) {
	// Try remote first if enabled
	if h.shouldUseRemote() {
		agent, err := h.remote.Get(repoName, agentName)
		if err == nil {
			return agent, nil
		}
		// Fall through to local if fallback enabled
		if !h.config.FallbackToLocal {
			return nil, err
		}
	}

	// Fall back to local
	return h.local.Get(repoName, agentName)
}

// List returns all agents, merging local and remote
func (h *HybridRegistry) List(repoName string) ([]*AgentInfo, error) {
	// Get local agents
	localAgents, err := h.local.List(repoName)
	if err != nil {
		return nil, err
	}

	// If remote not enabled, return local only
	if !h.shouldUseRemote() {
		return localAgents, nil
	}

	// Get remote agents
	remoteAgents, err := h.remote.List(repoName)
	if err != nil {
		if h.config.FallbackToLocal {
			return localAgents, nil
		}
		return nil, err
	}

	// Merge, preferring remote data for duplicates
	agentMap := make(map[string]*AgentInfo)
	for _, a := range localAgents {
		agentMap[a.Name] = a
	}
	for _, a := range remoteAgents {
		agentMap[a.Name] = a
	}

	result := make([]*AgentInfo, 0, len(agentMap))
	for _, a := range agentMap {
		result = append(result, a)
	}

	return result, nil
}

// ListByType returns agents of a specific type from both registries
func (h *HybridRegistry) ListByType(repoName, agentType string) ([]*AgentInfo, error) {
	agents, err := h.List(repoName)
	if err != nil {
		return nil, err
	}

	var result []*AgentInfo
	for _, agent := range agents {
		if agent.Type == agentType {
			result = append(result, agent)
		}
	}
	return result, nil
}

// ListByLocation returns agents at a specific location
func (h *HybridRegistry) ListByLocation(repoName string, location Location) ([]*AgentInfo, error) {
	agents, err := h.List(repoName)
	if err != nil {
		return nil, err
	}

	var result []*AgentInfo
	for _, agent := range agents {
		if agent.Location == location {
			result = append(result, agent)
		}
	}
	return result, nil
}

// UpdateHeartbeat updates heartbeat on both registries
func (h *HybridRegistry) UpdateHeartbeat(repoName, agentName string) error {
	// Update local
	if err := h.local.UpdateHeartbeat(repoName, agentName); err != nil {
		// Agent might be remote-only
		if !h.shouldUseRemote() {
			return err
		}
	}

	// Update remote if enabled
	if h.shouldUseRemote() {
		if err := h.remote.UpdateHeartbeat(repoName, agentName); err != nil {
			if !h.config.FallbackToLocal {
				return err
			}
		}
	}

	return nil
}

// UpdateStatus updates status on both registries
func (h *HybridRegistry) UpdateStatus(repoName, agentName string, status AgentStatus) error {
	// Update local
	localErr := h.local.UpdateStatus(repoName, agentName, status)

	// Update remote if enabled
	if h.shouldUseRemote() {
		if err := h.remote.UpdateStatus(repoName, agentName, status); err != nil {
			if !h.config.FallbackToLocal {
				return err
			}
		}
	}

	return localErr
}

// Verify Client implements Registry interface
var _ Registry = (*Client)(nil)

// Verify HybridRegistry implements Registry interface
var _ Registry = (*HybridRegistry)(nil)
