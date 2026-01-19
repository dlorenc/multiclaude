package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// WebhookConfig configures the webhook adapter
type WebhookConfig struct {
	AdapterConfig `yaml:",inline"`
	// URL is the webhook endpoint
	URL string `yaml:"url" json:"url"`
	// Headers are additional HTTP headers to send
	Headers map[string]string `yaml:"headers" json:"headers"`
	// Secret is used to sign webhook payloads (optional)
	Secret string `yaml:"secret" json:"secret"`
	// Timeout for HTTP requests (default: 10s)
	Timeout time.Duration `yaml:"timeout" json:"timeout"`
	// RetryCount is the number of retries on failure (default: 3)
	RetryCount int `yaml:"retry_count" json:"retry_count"`
	// RetryDelay is the delay between retries (default: 1s)
	RetryDelay time.Duration `yaml:"retry_delay" json:"retry_delay"`
}

// WebhookPayload is the JSON structure sent to webhook endpoints
type WebhookPayload struct {
	// Version is the payload format version
	Version string `json:"version"`
	// Event contains the notification event
	Event *Event `json:"event"`
	// Signature is the HMAC signature (if secret is configured)
	Signature string `json:"signature,omitempty"`
	// Timestamp is when the payload was created
	Timestamp time.Time `json:"timestamp"`
}

// WebhookAdapter sends notifications to HTTP endpoints
type WebhookAdapter struct {
	config     *WebhookConfig
	client     *http.Client
	payloadVer string
}

// NewWebhookAdapter creates a new webhook adapter
func NewWebhookAdapter(config *WebhookConfig) (*WebhookAdapter, error) {
	if config.URL == "" {
		return nil, fmt.Errorf("webhook URL is required")
	}

	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}
	if config.RetryCount == 0 {
		config.RetryCount = 3
	}
	if config.RetryDelay == 0 {
		config.RetryDelay = time.Second
	}

	return &WebhookAdapter{
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
		},
		payloadVer: "1.0",
	}, nil
}

// Name returns the adapter name
func (w *WebhookAdapter) Name() string {
	return w.config.Name
}

// Type returns "webhook"
func (w *WebhookAdapter) Type() string {
	return "webhook"
}

// Send sends an event to the webhook endpoint
func (w *WebhookAdapter) Send(ctx context.Context, event *Event) error {
	payload := &WebhookPayload{
		Version:   w.payloadVer,
		Event:     event,
		Timestamp: time.Now(),
	}

	// Marshal payload
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Sign if secret is configured
	if w.config.Secret != "" {
		payload.Signature = w.sign(data)
		// Re-marshal with signature
		data, _ = json.Marshal(payload)
	}

	// Retry loop
	var lastErr error
	for i := 0; i <= w.config.RetryCount; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(w.config.RetryDelay):
			}
		}

		if err := w.sendRequest(ctx, data); err != nil {
			lastErr = err
			continue
		}
		return nil
	}

	return fmt.Errorf("webhook failed after %d retries: %w", w.config.RetryCount, lastErr)
}

// sendRequest sends a single HTTP request
func (w *WebhookAdapter) sendRequest(ctx context.Context, data []byte) error {
	req, err := http.NewRequestWithContext(ctx, "POST", w.config.URL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "multiclaude-notify/1.0")

	// Add custom headers
	for key, value := range w.config.Headers {
		req.Header.Set(key, value)
	}

	// Add signature header if present
	if w.config.Secret != "" {
		req.Header.Set("X-Multiclaude-Signature", w.sign(data))
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body for error messages
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// sign creates an HMAC-SHA256 signature
func (w *WebhookAdapter) sign(data []byte) string {
	mac := hmac.New(sha256.New, []byte(w.config.Secret))
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil))
}

// SupportsResponses returns false for basic webhooks
func (w *WebhookAdapter) SupportsResponses() bool {
	return false
}

// Close cleans up resources
func (w *WebhookAdapter) Close() error {
	w.client.CloseIdleConnections()
	return nil
}

// VerifySignature verifies a webhook signature (for incoming webhooks)
func VerifySignature(secret, signature string, payload []byte) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(expected))
}

// InteractiveWebhookConfig extends WebhookConfig for bi-directional webhooks
type InteractiveWebhookConfig struct {
	WebhookConfig `yaml:",inline"`
	// ResponseURL is where responses should be sent (for server mode)
	ResponseURL string `yaml:"response_url" json:"response_url"`
	// ListenAddr is the address to listen on for incoming responses
	ListenAddr string `yaml:"listen_addr" json:"listen_addr"`
	// ResponsePath is the path for the response endpoint (default: /response)
	ResponsePath string `yaml:"response_path" json:"response_path"`
}

// InteractiveWebhookAdapter supports bi-directional webhook communication
type InteractiveWebhookAdapter struct {
	*WebhookAdapter
	interactiveConfig *InteractiveWebhookConfig
	server            *http.Server
	responseHandler   ResponseHandler
}

// NewInteractiveWebhookAdapter creates an interactive webhook adapter
func NewInteractiveWebhookAdapter(config *InteractiveWebhookConfig) (*InteractiveWebhookAdapter, error) {
	base, err := NewWebhookAdapter(&config.WebhookConfig)
	if err != nil {
		return nil, err
	}

	if config.ResponsePath == "" {
		config.ResponsePath = "/response"
	}

	return &InteractiveWebhookAdapter{
		WebhookAdapter:    base,
		interactiveConfig: config,
	}, nil
}

// SupportsResponses returns true
func (w *InteractiveWebhookAdapter) SupportsResponses() bool {
	return true
}

// SetResponseHandler sets the callback for incoming responses
func (w *InteractiveWebhookAdapter) SetResponseHandler(handler ResponseHandler) {
	w.responseHandler = handler
}

// Start starts the HTTP server for incoming responses
func (w *InteractiveWebhookAdapter) Start(ctx context.Context) error {
	if w.interactiveConfig.ListenAddr == "" {
		return nil // No server mode configured
	}

	mux := http.NewServeMux()
	mux.HandleFunc(w.interactiveConfig.ResponsePath, w.handleResponse)

	w.server = &http.Server{
		Addr:    w.interactiveConfig.ListenAddr,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		w.server.Shutdown(shutdownCtx)
	}()

	if err := w.server.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// handleResponse processes incoming response webhooks
func (w *InteractiveWebhookAdapter) handleResponse(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(rw, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		http.Error(rw, "failed to read body", http.StatusBadRequest)
		return
	}

	// Verify signature if secret is configured
	if w.config.Secret != "" {
		sig := r.Header.Get("X-Multiclaude-Signature")
		if !VerifySignature(w.config.Secret, sig, body) {
			http.Error(rw, "invalid signature", http.StatusUnauthorized)
			return
		}
	}

	var response Response
	if err := json.Unmarshal(body, &response); err != nil {
		http.Error(rw, "invalid JSON", http.StatusBadRequest)
		return
	}

	response.Source = w.Name()
	response.Timestamp = time.Now()

	if w.responseHandler != nil {
		w.responseHandler(&response)
	}

	rw.WriteHeader(http.StatusOK)
	rw.Write([]byte(`{"status":"ok"}`))
}

// Close stops the server and cleans up
func (w *InteractiveWebhookAdapter) Close() error {
	if w.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		w.server.Shutdown(ctx)
	}
	return w.WebhookAdapter.Close()
}
