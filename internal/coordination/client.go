package coordination

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Client provides access to a remote coordination server.
// It handles registration, heartbeats, task claiming, and event streaming.
type Client struct {
	baseURL        string
	httpClient     *http.Client
	token          string
	nodeID         string
	registrationID string

	// For heartbeat management
	heartbeatCtx    context.Context
	heartbeatCancel context.CancelFunc
	heartbeatWg     sync.WaitGroup

	mu sync.RWMutex
}

// NewClient creates a new coordination client.
// The baseURL should be the coordination server address (e.g., "https://coordinator:7331").
// The token is used for bearer authentication.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// NewClientWithConfig creates a new coordination client from configuration.
func NewClientWithConfig(config *ClientConfig) *Client {
	c := NewClient(config.ServerURL, config.Token)
	c.nodeID = config.NodeID
	return c
}

// SetNodeID sets the node identifier for this client.
func (c *Client) SetNodeID(nodeID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nodeID = nodeID
}

// GetNodeID returns the current node ID.
func (c *Client) GetNodeID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.nodeID
}

// GetRegistrationID returns the current registration ID.
func (c *Client) GetRegistrationID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.registrationID
}

// IsRegistered returns true if the client is currently registered.
func (c *Client) IsRegistered() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.registrationID != ""
}

// Register registers this node with the coordination server.
// It returns registration details including the required heartbeat interval.
func (c *Client) Register(ctx context.Context, req *RegisterRequest) (*RegisterResponse, error) {
	c.mu.Lock()
	if req.NodeID == "" {
		if c.nodeID != "" {
			req.NodeID = c.nodeID
		} else {
			req.NodeID = fmt.Sprintf("node-%s", uuid.New().String()[:8])
			c.nodeID = req.NodeID
		}
	}
	c.mu.Unlock()

	var resp RegisterResponse
	if err := c.doRequest(ctx, http.MethodPost, "/api/v1/register", req, &resp); err != nil {
		return nil, fmt.Errorf("registration failed: %w", err)
	}

	c.mu.Lock()
	c.registrationID = resp.RegistrationID
	c.mu.Unlock()

	return &resp, nil
}

// Unregister removes this node's registration from the coordinator.
func (c *Client) Unregister(ctx context.Context) error {
	c.mu.RLock()
	regID := c.registrationID
	c.mu.RUnlock()

	if regID == "" {
		return nil // not registered
	}

	req := map[string]string{"registration_id": regID}
	if err := c.doRequest(ctx, http.MethodPost, "/api/v1/unregister", req, nil); err != nil {
		return fmt.Errorf("unregister failed: %w", err)
	}

	c.mu.Lock()
	c.registrationID = ""
	c.mu.Unlock()

	return nil
}

// Heartbeat sends a heartbeat to the coordination server.
// This must be called periodically to maintain registration.
func (c *Client) Heartbeat(ctx context.Context, status *HeartbeatRequest) error {
	c.mu.RLock()
	regID := c.registrationID
	c.mu.RUnlock()

	if regID == "" {
		return fmt.Errorf("not registered")
	}

	status.RegistrationID = regID

	var resp HeartbeatResponse
	if err := c.doRequest(ctx, http.MethodPost, "/api/v1/heartbeat", status, &resp); err != nil {
		return fmt.Errorf("heartbeat failed: %w", err)
	}

	if !resp.Acknowledged {
		return fmt.Errorf("heartbeat not acknowledged: %s", resp.Message)
	}

	return nil
}

// StartHeartbeat starts automatic heartbeat sending at the specified interval.
// It also reports the current agent status via the statusFunc callback.
func (c *Client) StartHeartbeat(interval time.Duration, statusFunc func() *HeartbeatRequest) {
	c.mu.Lock()
	if c.heartbeatCancel != nil {
		c.heartbeatCancel()
	}
	c.heartbeatCtx, c.heartbeatCancel = context.WithCancel(context.Background())
	c.mu.Unlock()

	c.heartbeatWg.Add(1)
	go func() {
		defer c.heartbeatWg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				status := statusFunc()
				if status == nil {
					status = &HeartbeatRequest{Status: NodeStatusOnline}
				}
				ctx, cancel := context.WithTimeout(c.heartbeatCtx, 10*time.Second)
				if err := c.Heartbeat(ctx, status); err != nil {
					// Log error but don't stop heartbeating
					fmt.Printf("heartbeat error: %v\n", err)
				}
				cancel()
			case <-c.heartbeatCtx.Done():
				return
			}
		}
	}()
}

// StopHeartbeat stops the automatic heartbeat goroutine.
func (c *Client) StopHeartbeat() {
	c.mu.Lock()
	if c.heartbeatCancel != nil {
		c.heartbeatCancel()
		c.heartbeatCancel = nil
	}
	c.mu.Unlock()
	c.heartbeatWg.Wait()
}

// GetPendingTasks fetches tasks that are pending and match the given labels.
// If labels is nil, all pending tasks are returned.
func (c *Client) GetPendingTasks(ctx context.Context, labels map[string]string) ([]Task, error) {
	path := "/api/v1/tasks/pending"
	if len(labels) > 0 {
		params := url.Values{}
		for k, v := range labels {
			params.Add("label", fmt.Sprintf("%s:%s", k, v))
		}
		path += "?" + params.Encode()
	}

	var resp struct {
		Tasks []Task `json:"tasks"`
	}
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, fmt.Errorf("failed to get pending tasks: %w", err)
	}

	return resp.Tasks, nil
}

// ClaimTask attempts to claim a task for execution.
// Returns the claimed task or an error if the claim failed.
func (c *Client) ClaimTask(ctx context.Context, taskID, workerName string) (*Task, error) {
	c.mu.RLock()
	regID := c.registrationID
	c.mu.RUnlock()

	if regID == "" {
		return nil, fmt.Errorf("not registered")
	}

	req := &TaskClaimRequest{
		RegistrationID: regID,
		WorkerName:     workerName,
	}

	path := fmt.Sprintf("/api/v1/tasks/%s/claim", taskID)
	var resp TaskClaimResponse
	if err := c.doRequest(ctx, http.MethodPost, path, req, &resp); err != nil {
		return nil, fmt.Errorf("failed to claim task: %w", err)
	}

	if !resp.Claimed {
		return nil, fmt.Errorf("task claim rejected: %s", resp.Error)
	}

	return resp.Task, nil
}

// UpdateTask updates the status of a claimed task.
func (c *Client) UpdateTask(ctx context.Context, taskID string, update *TaskUpdateRequest) error {
	c.mu.RLock()
	regID := c.registrationID
	c.mu.RUnlock()

	if regID == "" {
		return fmt.Errorf("not registered")
	}

	update.RegistrationID = regID

	path := fmt.Sprintf("/api/v1/tasks/%s", taskID)
	if err := c.doRequest(ctx, http.MethodPut, path, update, nil); err != nil {
		return fmt.Errorf("failed to update task: %w", err)
	}

	return nil
}

// CreateTask creates a new distributable task.
func (c *Client) CreateTask(ctx context.Context, req *CreateTaskRequest) (*Task, error) {
	var task Task
	if err := c.doRequest(ctx, http.MethodPost, "/api/v1/tasks", req, &task); err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}
	return &task, nil
}

// GetTask retrieves a specific task by ID.
func (c *Client) GetTask(ctx context.Context, taskID string) (*Task, error) {
	path := fmt.Sprintf("/api/v1/tasks/%s", taskID)
	var task Task
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &task); err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}
	return &task, nil
}

// ListTasks retrieves all tasks, optionally filtered by status.
func (c *Client) ListTasks(ctx context.Context, status TaskStatus) ([]Task, error) {
	path := "/api/v1/tasks"
	if status != "" {
		path += "?status=" + string(status)
	}

	var resp struct {
		Tasks []Task `json:"tasks"`
	}
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, fmt.Errorf("failed to list tasks: %w", err)
	}

	return resp.Tasks, nil
}

// SendMessage sends an inter-agent message through the coordinator.
func (c *Client) SendMessage(ctx context.Context, msg *Message) error {
	if msg.ID == "" {
		msg.ID = fmt.Sprintf("msg-%s", uuid.New().String()[:8])
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	if err := c.doRequest(ctx, http.MethodPost, "/api/v1/messages", msg, nil); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}

// GetMessages retrieves pending messages for an agent.
func (c *Client) GetMessages(ctx context.Context, agentName string) ([]Message, error) {
	path := fmt.Sprintf("/api/v1/messages?agent=%s", url.QueryEscape(agentName))

	var resp struct {
		Messages []Message `json:"messages"`
	}
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}

	return resp.Messages, nil
}

// GetState retrieves the coordination state for a repository.
func (c *Client) GetState(ctx context.Context, repo string) (*StateResponse, error) {
	path := fmt.Sprintf("/api/v1/state/%s", url.PathEscape(repo))

	var state StateResponse
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &state); err != nil {
		return nil, fmt.Errorf("failed to get state: %w", err)
	}

	return &state, nil
}

// ListNodes retrieves all registered nodes.
func (c *Client) ListNodes(ctx context.Context) ([]Node, error) {
	var resp struct {
		Nodes []Node `json:"nodes"`
	}
	if err := c.doRequest(ctx, http.MethodGet, "/api/v1/nodes", nil, &resp); err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	return resp.Nodes, nil
}

// GetNode retrieves a specific node by ID.
func (c *Client) GetNode(ctx context.Context, nodeID string) (*Node, error) {
	path := fmt.Sprintf("/api/v1/nodes/%s", url.PathEscape(nodeID))

	var node Node
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &node); err != nil {
		return nil, fmt.Errorf("failed to get node: %w", err)
	}

	return &node, nil
}

// Health checks the coordinator health.
func (c *Client) Health(ctx context.Context) error {
	if err := c.doRequest(ctx, http.MethodGet, "/api/v1/health", nil, nil); err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	return nil
}

// StreamEvents opens an SSE connection for real-time events.
// Returns a channel that receives events until the context is cancelled.
func (c *Client) StreamEvents(ctx context.Context) (<-chan Event, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/events/stream", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "text/event-stream")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to event stream: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("event stream returned status %d", resp.StatusCode)
	}

	events := make(chan Event, 100)

	go func() {
		defer close(events)
		defer resp.Body.Close()

		reader := bufio.NewReader(resp.Body)
		var eventType string
		var data strings.Builder

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					fmt.Printf("event stream error: %v\n", err)
				}
				return
			}

			line = strings.TrimSpace(line)

			if line == "" {
				// End of event
				if data.Len() > 0 && eventType != "" {
					var event Event
					if err := json.Unmarshal([]byte(data.String()), &event); err == nil {
						select {
						case events <- event:
						default:
							// Channel full, drop event
						}
					}
				}
				eventType = ""
				data.Reset()
				continue
			}

			if strings.HasPrefix(line, "event:") {
				eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			} else if strings.HasPrefix(line, "data:") {
				data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}
	}()

	return events, nil
}

// Close stops heartbeats and cleans up resources.
func (c *Client) Close() error {
	c.StopHeartbeat()
	return nil
}

// doRequest performs an HTTP request with JSON encoding/decoding.
func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Check for error response
	if resp.StatusCode >= 400 {
		var apiResp APIResponse
		if err := json.Unmarshal(respBody, &apiResp); err == nil && apiResp.Error != "" {
			return fmt.Errorf("%s (code: %s)", apiResp.Error, apiResp.Code)
		}
		return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	// Decode successful response
	if result != nil && len(respBody) > 0 {
		// Try to unwrap from APIResponse
		var apiResp APIResponse
		if err := json.Unmarshal(respBody, &apiResp); err == nil && apiResp.Success {
			// Re-marshal the Data field and unmarshal into result
			if apiResp.Data != nil {
				dataBytes, err := json.Marshal(apiResp.Data)
				if err != nil {
					return fmt.Errorf("failed to re-marshal response data: %w", err)
				}
				if err := json.Unmarshal(dataBytes, result); err != nil {
					return fmt.Errorf("failed to decode response data: %w", err)
				}
				return nil
			}
		}

		// Try direct unmarshaling
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}
