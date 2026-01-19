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
	"strings"
	"time"
)

// SlackConfig configures the Slack adapter
type SlackConfig struct {
	AdapterConfig `yaml:",inline"`
	// WebhookURL is the Slack Incoming Webhook URL
	WebhookURL string `yaml:"webhook_url" json:"webhook_url"`
	// BotToken is for interactive features (optional)
	BotToken string `yaml:"bot_token" json:"bot_token"`
	// Channel is the default channel for messages
	Channel string `yaml:"channel" json:"channel"`
	// SigningSecret is for verifying Slack requests (optional)
	SigningSecret string `yaml:"signing_secret" json:"signing_secret"`
	// ListenAddr for interactive components (optional)
	ListenAddr string `yaml:"listen_addr" json:"listen_addr"`
	// Timeout for HTTP requests
	Timeout time.Duration `yaml:"timeout" json:"timeout"`
}

// SlackAdapter sends notifications to Slack
type SlackAdapter struct {
	config          *SlackConfig
	client          *http.Client
	server          *http.Server
	responseHandler ResponseHandler
}

// NewSlackAdapter creates a new Slack adapter
func NewSlackAdapter(config *SlackConfig) (*SlackAdapter, error) {
	if config.WebhookURL == "" && config.BotToken == "" {
		return nil, fmt.Errorf("either webhook_url or bot_token is required")
	}

	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}

	return &SlackAdapter{
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
		},
	}, nil
}

// Name returns the adapter name
func (s *SlackAdapter) Name() string {
	return s.config.Name
}

// Type returns "slack"
func (s *SlackAdapter) Type() string {
	return "slack"
}

// Send sends an event to Slack
func (s *SlackAdapter) Send(ctx context.Context, event *Event) error {
	blocks := s.formatBlocks(event)
	payload := s.buildPayload(event, blocks)

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Use webhook URL for simple notifications
	if s.config.WebhookURL != "" {
		return s.sendWebhook(ctx, data)
	}

	// Use Bot API for more features
	return s.sendBotMessage(ctx, data)
}

// sendWebhook sends via Incoming Webhook
func (s *SlackAdapter) sendWebhook(ctx context.Context, data []byte) error {
	req, err := http.NewRequestWithContext(ctx, "POST", s.config.WebhookURL, bytes.NewReader(data))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("slack returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// sendBotMessage sends via Bot API
func (s *SlackAdapter) sendBotMessage(ctx context.Context, data []byte) error {
	req, err := http.NewRequestWithContext(ctx, "POST", "https://slack.com/api/chat.postMessage", bytes.NewReader(data))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.config.BotToken)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("bot API request failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.OK {
		return fmt.Errorf("slack API error: %s", result.Error)
	}

	return nil
}

// buildPayload creates the Slack message payload
func (s *SlackAdapter) buildPayload(event *Event, blocks []map[string]interface{}) map[string]interface{} {
	payload := map[string]interface{}{
		"blocks": blocks,
		"text":   event.Title, // Fallback for notifications
	}

	if s.config.Channel != "" && s.config.BotToken != "" {
		payload["channel"] = s.config.Channel
	}

	return payload
}

// formatBlocks creates Slack Block Kit blocks for an event
func (s *SlackAdapter) formatBlocks(event *Event) []map[string]interface{} {
	var blocks []map[string]interface{}

	// Header section with emoji
	emoji := s.eventEmoji(event.Type)
	header := fmt.Sprintf("%s %s", emoji, event.Title)

	blocks = append(blocks, map[string]interface{}{
		"type": "header",
		"text": map[string]interface{}{
			"type": "plain_text",
			"text": header,
		},
	})

	// Context with repo and agent info
	contextElements := []map[string]interface{}{}
	if event.RepoName != "" {
		contextElements = append(contextElements, map[string]interface{}{
			"type": "mrkdwn",
			"text": fmt.Sprintf("*Repo:* %s", event.RepoName),
		})
	}
	if event.AgentName != "" {
		contextElements = append(contextElements, map[string]interface{}{
			"type": "mrkdwn",
			"text": fmt.Sprintf("*Agent:* %s (%s)", event.AgentName, event.AgentType),
		})
	}
	contextElements = append(contextElements, map[string]interface{}{
		"type": "mrkdwn",
		"text": fmt.Sprintf("*Priority:* %s", event.Priority),
	})

	blocks = append(blocks, map[string]interface{}{
		"type":     "context",
		"elements": contextElements,
	})

	// Main message content
	if event.Message != "" {
		// Escape message for Slack
		message := s.escapeSlack(event.Message)
		blocks = append(blocks, map[string]interface{}{
			"type": "section",
			"text": map[string]interface{}{
				"type": "mrkdwn",
				"text": message,
			},
		})
	}

	// Add context fields if present
	if len(event.Context) > 0 {
		fields := []map[string]interface{}{}
		for key, value := range event.Context {
			fields = append(fields, map[string]interface{}{
				"type": "mrkdwn",
				"text": fmt.Sprintf("*%s:*\n%s", key, value),
			})
		}
		blocks = append(blocks, map[string]interface{}{
			"type":   "section",
			"fields": fields,
		})
	}

	// Add action buttons if response is supported
	if event.ActionRequired && event.ResponseID != "" {
		blocks = append(blocks, map[string]interface{}{
			"type": "divider",
		})

		actions := []map[string]interface{}{
			{
				"type": "button",
				"text": map[string]interface{}{
					"type": "plain_text",
					"text": "Respond",
				},
				"style":     "primary",
				"action_id": "respond",
				"value":     event.ResponseID,
			},
			{
				"type": "button",
				"text": map[string]interface{}{
					"type": "plain_text",
					"text": "Dismiss",
				},
				"action_id": "dismiss",
				"value":     event.ResponseID,
			},
		}

		// Add PR link if present
		if prURL, ok := event.Context["pr_url"]; ok {
			actions = append(actions, map[string]interface{}{
				"type": "button",
				"text": map[string]interface{}{
					"type": "plain_text",
					"text": "View PR",
				},
				"url": prURL,
			})
		}

		blocks = append(blocks, map[string]interface{}{
			"type":     "actions",
			"elements": actions,
		})
	}

	// Footer with timestamp
	blocks = append(blocks, map[string]interface{}{
		"type": "context",
		"elements": []map[string]interface{}{
			{
				"type": "mrkdwn",
				"text": fmt.Sprintf("Event ID: %s | %s", event.ID, event.Timestamp.Format(time.RFC3339)),
			},
		},
	})

	return blocks
}

// eventEmoji returns an appropriate emoji for the event type
func (s *SlackAdapter) eventEmoji(eventType EventType) string {
	switch eventType {
	case EventAgentQuestion:
		return ":raising_hand:"
	case EventAgentCompleted:
		return ":white_check_mark:"
	case EventAgentStuck:
		return ":warning:"
	case EventAgentError:
		return ":x:"
	case EventPRCreated:
		return ":git:"
	case EventPRMerged:
		return ":merged:"
	case EventCIFailed:
		return ":rotating_light:"
	case EventStatusUpdate:
		return ":chart_with_upwards_trend:"
	default:
		return ":bell:"
	}
}

// escapeSlack escapes special characters for Slack mrkdwn
func (s *SlackAdapter) escapeSlack(text string) string {
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	return text
}

// SupportsResponses returns true if bot token is configured
func (s *SlackAdapter) SupportsResponses() bool {
	return s.config.BotToken != "" && s.config.ListenAddr != ""
}

// SetResponseHandler sets the callback for interactive responses
func (s *SlackAdapter) SetResponseHandler(handler ResponseHandler) {
	s.responseHandler = handler
}

// Start starts the interactive components server
func (s *SlackAdapter) Start(ctx context.Context) error {
	if s.config.ListenAddr == "" {
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/slack/interactions", s.handleInteraction)

	s.server = &http.Server{
		Addr:    s.config.ListenAddr,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.server.Shutdown(shutdownCtx)
	}()

	if err := s.server.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// handleInteraction processes Slack interactive component callbacks
func (s *SlackAdapter) handleInteraction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	// Verify Slack signature
	if s.config.SigningSecret != "" {
		timestamp := r.Header.Get("X-Slack-Request-Timestamp")
		signature := r.Header.Get("X-Slack-Signature")
		if !s.verifySlackSignature(timestamp, signature, body) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
	}

	// Parse the payload
	r.ParseForm()
	payloadStr := r.FormValue("payload")

	var payload struct {
		Type    string `json:"type"`
		User    struct {
			ID       string `json:"id"`
			Username string `json:"username"`
		} `json:"user"`
		Actions []struct {
			ActionID string `json:"action_id"`
			Value    string `json:"value"`
		} `json:"actions"`
		Message struct {
			Ts string `json:"ts"`
		} `json:"message"`
		ResponseURL string `json:"response_url"`
	}

	if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	// Process actions
	for _, action := range payload.Actions {
		response := &Response{
			ResponseID: action.Value,
			Action:     action.ActionID,
			Source:     s.Name(),
			UserID:     payload.User.ID,
			Timestamp:  time.Now(),
		}

		if s.responseHandler != nil {
			s.responseHandler(response)
		}
	}

	// Acknowledge the interaction
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"response_type":"in_channel"}`))
}

// verifySlackSignature verifies the Slack request signature
func (s *SlackAdapter) verifySlackSignature(timestamp, signature string, body []byte) bool {
	baseString := fmt.Sprintf("v0:%s:%s", timestamp, string(body))
	mac := hmac.New(sha256.New, []byte(s.config.SigningSecret))
	mac.Write([]byte(baseString))
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(expected))
}

// Close cleans up resources
func (s *SlackAdapter) Close() error {
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.server.Shutdown(ctx)
	}
	s.client.CloseIdleConnections()
	return nil
}

// FormatStatusSummary creates a Slack message for a status summary
func FormatSlackStatusSummary(summary *StatusSummary) []map[string]interface{} {
	var blocks []map[string]interface{}

	// Header
	blocks = append(blocks, map[string]interface{}{
		"type": "header",
		"text": map[string]interface{}{
			"type": "plain_text",
			"text": fmt.Sprintf(":chart_with_upwards_trend: Status Update: %s", summary.RepoName),
		},
	})

	// Overview stats
	statsText := fmt.Sprintf(
		"*Active Workers:* %d | *Pending Questions:* %d | *Completed:* %d",
		summary.ActiveWorkers,
		summary.PendingQuestions,
		summary.CompletedTasks,
	)
	blocks = append(blocks, map[string]interface{}{
		"type": "section",
		"text": map[string]interface{}{
			"type": "mrkdwn",
			"text": statsText,
		},
	})

	// Agent list
	if len(summary.Agents) > 0 {
		blocks = append(blocks, map[string]interface{}{
			"type": "divider",
		})

		for _, agent := range summary.Agents {
			statusEmoji := ":white_circle:"
			switch agent.Status {
			case "working":
				statusEmoji = ":large_green_circle:"
			case "waiting":
				statusEmoji = ":large_yellow_circle:"
			case "stuck":
				statusEmoji = ":red_circle:"
			case "completed":
				statusEmoji = ":white_check_mark:"
			}

			agentText := fmt.Sprintf("%s *%s* (%s)", statusEmoji, agent.Name, agent.Type)
			if agent.Task != "" {
				agentText += fmt.Sprintf("\n_%s_", agent.Task)
			}
			if agent.Duration != "" {
				agentText += fmt.Sprintf(" (%s)", agent.Duration)
			}

			blocks = append(blocks, map[string]interface{}{
				"type": "section",
				"text": map[string]interface{}{
					"type": "mrkdwn",
					"text": agentText,
				},
			})
		}
	}

	// Footer
	blocks = append(blocks, map[string]interface{}{
		"type": "context",
		"elements": []map[string]interface{}{
			{
				"type": "mrkdwn",
				"text": fmt.Sprintf("Generated at %s", summary.GeneratedAt.Format(time.RFC3339)),
			},
		},
	})

	return blocks
}
