package coordination

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	// DefaultAPIEndpoint is the default coordination API URL
	DefaultAPIEndpoint = "https://api.multiclaude.dev/v1"
	// DefaultTimeout is the default HTTP client timeout
	DefaultTimeout = 30 * time.Second
)

// Client is the coordination API client
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// ClientOption is a functional option for configuring the client
type ClientOption func(*Client)

// WithBaseURL sets a custom API endpoint
func WithBaseURL(url string) ClientOption {
	return func(c *Client) {
		c.baseURL = url
	}
}

// WithTimeout sets a custom HTTP timeout
func WithTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) {
		c.httpClient.Timeout = timeout
	}
}

// NewClient creates a new coordination API client
func NewClient(token string, opts ...ClientOption) *Client {
	c := &Client{
		baseURL: DefaultAPIEndpoint,
		token:   token,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ---- Agent Registry ----

// RegisterAgent registers a new agent with the coordination API
func (c *Client) RegisterAgent(repo string, req *RegisterAgentRequest) (*Agent, error) {
	path := fmt.Sprintf("/repos/%s/agents", url.PathEscape(repo))
	var agent Agent
	if err := c.post(path, req, &agent); err != nil {
		return nil, err
	}
	return &agent, nil
}

// ListAgents lists all agents for a repository
func (c *Client) ListAgents(repo string) ([]*Agent, error) {
	path := fmt.Sprintf("/repos/%s/agents", url.PathEscape(repo))
	var agents []*Agent
	if err := c.get(path, &agents); err != nil {
		return nil, err
	}
	return agents, nil
}

// GetAgent retrieves a specific agent
func (c *Client) GetAgent(repo, name string) (*Agent, error) {
	path := fmt.Sprintf("/repos/%s/agents/%s", url.PathEscape(repo), url.PathEscape(name))
	var agent Agent
	if err := c.get(path, &agent); err != nil {
		return nil, err
	}
	return &agent, nil
}

// UpdateAgent updates an agent's status or endpoint
func (c *Client) UpdateAgent(repo, name string, req *UpdateAgentRequest) (*Agent, error) {
	path := fmt.Sprintf("/repos/%s/agents/%s", url.PathEscape(repo), url.PathEscape(name))
	var agent Agent
	if err := c.patch(path, req, &agent); err != nil {
		return nil, err
	}
	return &agent, nil
}

// DeregisterAgent removes an agent from the registry
func (c *Client) DeregisterAgent(repo, name string) error {
	path := fmt.Sprintf("/repos/%s/agents/%s", url.PathEscape(repo), url.PathEscape(name))
	return c.delete(path)
}

// Heartbeat updates the agent's last_seen timestamp
func (c *Client) Heartbeat(repo, name string, status AgentStatus) error {
	req := &UpdateAgentRequest{Status: &status}
	_, err := c.UpdateAgent(repo, name, req)
	return err
}

// ---- Messages ----

// SendMessage sends a message to another agent
func (c *Client) SendMessage(repo string, req *SendMessageRequest) (*Message, error) {
	path := fmt.Sprintf("/repos/%s/messages", url.PathEscape(repo))
	var msg Message
	if err := c.post(path, req, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// GetMessages retrieves messages for an agent
func (c *Client) GetMessages(repo, agentName string) ([]*Message, error) {
	path := fmt.Sprintf("/repos/%s/agents/%s/messages", url.PathEscape(repo), url.PathEscape(agentName))
	var messages []*Message
	if err := c.get(path, &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

// GetPendingMessages retrieves only pending messages for an agent
func (c *Client) GetPendingMessages(repo, agentName string) ([]*Message, error) {
	path := fmt.Sprintf("/repos/%s/agents/%s/messages?status=pending", url.PathEscape(repo), url.PathEscape(agentName))
	var messages []*Message
	if err := c.get(path, &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

// AckMessage acknowledges a message
func (c *Client) AckMessage(repo, messageID string) error {
	path := fmt.Sprintf("/repos/%s/messages/%s", url.PathEscape(repo), url.PathEscape(messageID))
	req := map[string]string{"status": string(MessageAcked)}
	return c.patch(path, req, nil)
}

// ---- Worker Spawning ----

// SpawnWorker requests a new remote worker to be spawned
func (c *Client) SpawnWorker(repo string, req *SpawnWorkerRequest) (*SpawnRequest, error) {
	path := fmt.Sprintf("/repos/%s/workers/spawn", url.PathEscape(repo))
	var spawn SpawnRequest
	if err := c.post(path, req, &spawn); err != nil {
		return nil, err
	}
	return &spawn, nil
}

// GetPendingSpawns retrieves pending spawn requests
func (c *Client) GetPendingSpawns(repo string) ([]*SpawnRequest, error) {
	path := fmt.Sprintf("/repos/%s/workers/pending", url.PathEscape(repo))
	var spawns []*SpawnRequest
	if err := c.get(path, &spawns); err != nil {
		return nil, err
	}
	return spawns, nil
}

// ---- Configuration ----

// GetHybridConfig retrieves the hybrid configuration for a repository
func (c *Client) GetHybridConfig(repo string) (*HybridConfig, error) {
	path := fmt.Sprintf("/repos/%s/config", url.PathEscape(repo))
	var config HybridConfig
	if err := c.get(path, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

// UpdateHybridConfig updates the hybrid configuration for a repository
func (c *Client) UpdateHybridConfig(repo string, config *HybridConfig) (*HybridConfig, error) {
	path := fmt.Sprintf("/repos/%s/config", url.PathEscape(repo))
	var result HybridConfig
	if err := c.put(path, config, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ---- HTTP helpers ----

func (c *Client) get(path string, result interface{}) error {
	return c.doRequest(http.MethodGet, path, nil, result)
}

func (c *Client) post(path string, body, result interface{}) error {
	return c.doRequest(http.MethodPost, path, body, result)
}

func (c *Client) put(path string, body, result interface{}) error {
	return c.doRequest(http.MethodPut, path, body, result)
}

func (c *Client) patch(path string, body, result interface{}) error {
	return c.doRequest(http.MethodPatch, path, body, result)
}

func (c *Client) delete(path string) error {
	return c.doRequest(http.MethodDelete, path, nil, nil)
}

func (c *Client) doRequest(method, path string, body, result interface{}) error {
	fullURL := c.baseURL + path

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, fullURL, reqBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var apiErr APIError
		if err := json.Unmarshal(respBody, &apiErr); err != nil {
			return fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
		}
		return &apiErr
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}
	}

	return nil
}
