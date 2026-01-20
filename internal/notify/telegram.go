package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// TelegramConfig configures the Telegram adapter
type TelegramConfig struct {
	AdapterConfig `yaml:",inline"`
	// BotToken is the Telegram Bot API token
	BotToken string `yaml:"bot_token" json:"bot_token"`
	// ChatID is the target chat/channel/group ID
	ChatID string `yaml:"chat_id" json:"chat_id"`
	// ParseMode is the message format (HTML or Markdown)
	ParseMode string `yaml:"parse_mode" json:"parse_mode"`
	// DisableNotification sends messages silently
	DisableNotification bool `yaml:"disable_notification" json:"disable_notification"`
	// ListenAddr for receiving updates via webhook (optional)
	ListenAddr string `yaml:"listen_addr" json:"listen_addr"`
	// WebhookPath is the path for the webhook endpoint
	WebhookPath string `yaml:"webhook_path" json:"webhook_path"`
	// WebhookURL is the public URL for Telegram to send updates
	WebhookURL string `yaml:"webhook_url" json:"webhook_url"`
	// Timeout for API requests
	Timeout time.Duration `yaml:"timeout" json:"timeout"`
}

// TelegramAdapter sends notifications via Telegram Bot API
type TelegramAdapter struct {
	config          *TelegramConfig
	client          *http.Client
	apiBase         string
	server          *http.Server
	responseHandler ResponseHandler
}

// NewTelegramAdapter creates a new Telegram adapter
func NewTelegramAdapter(config *TelegramConfig) (*TelegramAdapter, error) {
	if config.BotToken == "" {
		return nil, fmt.Errorf("bot_token is required")
	}
	if config.ChatID == "" {
		return nil, fmt.Errorf("chat_id is required")
	}

	if config.ParseMode == "" {
		config.ParseMode = "HTML"
	}
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}
	if config.WebhookPath == "" {
		config.WebhookPath = "/telegram/webhook"
	}

	return &TelegramAdapter{
		config:  config,
		client:  &http.Client{Timeout: config.Timeout},
		apiBase: fmt.Sprintf("https://api.telegram.org/bot%s", config.BotToken),
	}, nil
}

// Name returns the adapter name
func (t *TelegramAdapter) Name() string {
	return t.config.Name
}

// Type returns "telegram"
func (t *TelegramAdapter) Type() string {
	return "telegram"
}

// Send sends an event to Telegram
func (t *TelegramAdapter) Send(ctx context.Context, event *Event) error {
	message := t.formatMessage(event)

	payload := map[string]interface{}{
		"chat_id":              t.config.ChatID,
		"text":                 message,
		"parse_mode":           t.config.ParseMode,
		"disable_notification": t.config.DisableNotification,
	}

	// Add inline keyboard for interactive events
	if event.ActionRequired && event.ResponseID != "" {
		payload["reply_markup"] = t.buildKeyboard(event)
	}

	return t.sendMessage(ctx, payload)
}

// formatMessage creates a formatted message for Telegram
func (t *TelegramAdapter) formatMessage(event *Event) string {
	var sb strings.Builder

	// Emoji and title
	emoji := t.eventEmoji(event.Type)
	sb.WriteString(fmt.Sprintf("%s <b>%s</b>\n\n", emoji, t.escapeHTML(event.Title)))

	// Metadata
	sb.WriteString(fmt.Sprintf("<b>Repository:</b> %s\n", t.escapeHTML(event.RepoName)))
	if event.AgentName != "" {
		sb.WriteString(fmt.Sprintf("<b>Agent:</b> %s (%s)\n", t.escapeHTML(event.AgentName), event.AgentType))
	}
	sb.WriteString(fmt.Sprintf("<b>Priority:</b> %s\n", event.Priority))
	sb.WriteString("\n")

	// Main message
	if event.Message != "" {
		sb.WriteString(t.escapeHTML(event.Message))
		sb.WriteString("\n\n")
	}

	// Context
	if len(event.Context) > 0 {
		for key, value := range event.Context {
			if key == "pr_url" {
				sb.WriteString(fmt.Sprintf("<b>%s:</b> <a href=\"%s\">View PR</a>\n", key, value))
			} else {
				sb.WriteString(fmt.Sprintf("<b>%s:</b> %s\n", key, t.escapeHTML(value)))
			}
		}
		sb.WriteString("\n")
	}

	// Footer
	sb.WriteString(fmt.Sprintf("<code>Event: %s | %s</code>", event.ID, event.Timestamp.Format("15:04:05")))

	return sb.String()
}

// buildKeyboard creates an inline keyboard for the message
func (t *TelegramAdapter) buildKeyboard(event *Event) map[string]interface{} {
	buttons := [][]map[string]interface{}{}

	// Action buttons row
	actionRow := []map[string]interface{}{
		{
			"text":          "Respond",
			"callback_data": fmt.Sprintf("respond:%s", event.ResponseID),
		},
		{
			"text":          "Dismiss",
			"callback_data": fmt.Sprintf("dismiss:%s", event.ResponseID),
		},
	}
	buttons = append(buttons, actionRow)

	// URL buttons row (if applicable)
	if prURL, ok := event.Context["pr_url"]; ok {
		urlRow := []map[string]interface{}{
			{
				"text": "View Pull Request",
				"url":  prURL,
			},
		}
		buttons = append(buttons, urlRow)
	}

	return map[string]interface{}{
		"inline_keyboard": buttons,
	}
}

// sendMessage sends a message via the Bot API
func (t *TelegramAdapter) sendMessage(ctx context.Context, payload map[string]interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	url := fmt.Sprintf("%s/sendMessage", t.apiBase)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.OK {
		return fmt.Errorf("telegram API error: %s", result.Description)
	}

	return nil
}

// eventEmoji returns an appropriate emoji for the event type
func (t *TelegramAdapter) eventEmoji(eventType EventType) string {
	switch eventType {
	case EventAgentQuestion:
		return "\U0001F64B" // Raising hand
	case EventAgentCompleted:
		return "\u2705" // Check mark
	case EventAgentStuck:
		return "\u26A0\uFE0F" // Warning
	case EventAgentError:
		return "\u274C" // Cross mark
	case EventPRCreated:
		return "\U0001F500" // Shuffle arrows
	case EventPRMerged:
		return "\U0001F7E2" // Green circle
	case EventCIFailed:
		return "\U0001F6A8" // Rotating light
	case EventStatusUpdate:
		return "\U0001F4CA" // Bar chart
	default:
		return "\U0001F514" // Bell
	}
}

// escapeHTML escapes special characters for Telegram HTML mode
func (t *TelegramAdapter) escapeHTML(text string) string {
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	return text
}

// SupportsResponses returns true if webhook is configured
func (t *TelegramAdapter) SupportsResponses() bool {
	return t.config.ListenAddr != "" && t.config.WebhookURL != ""
}

// SetResponseHandler sets the callback for responses
func (t *TelegramAdapter) SetResponseHandler(handler ResponseHandler) {
	t.responseHandler = handler
}

// Start starts the webhook server and registers with Telegram
func (t *TelegramAdapter) Start(ctx context.Context) error {
	if t.config.ListenAddr == "" {
		return nil
	}

	// Register webhook with Telegram
	if t.config.WebhookURL != "" {
		if err := t.setWebhook(ctx); err != nil {
			return fmt.Errorf("failed to set webhook: %w", err)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc(t.config.WebhookPath, t.handleWebhook)

	t.server = &http.Server{
		Addr:    t.config.ListenAddr,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// Delete webhook on shutdown
		t.deleteWebhook(shutdownCtx)
		t.server.Shutdown(shutdownCtx)
	}()

	if err := t.server.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// setWebhook registers the webhook URL with Telegram
func (t *TelegramAdapter) setWebhook(ctx context.Context) error {
	url := fmt.Sprintf("%s/setWebhook", t.apiBase)
	payload := map[string]interface{}{
		"url":             t.config.WebhookURL + t.config.WebhookPath,
		"allowed_updates": []string{"callback_query", "message"},
	}

	data, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if !result.OK {
		return fmt.Errorf("failed to set webhook: %s", result.Description)
	}

	return nil
}

// deleteWebhook removes the webhook from Telegram
func (t *TelegramAdapter) deleteWebhook(ctx context.Context) error {
	url := fmt.Sprintf("%s/deleteWebhook", t.apiBase)
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return err
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// handleWebhook processes incoming Telegram updates
func (t *TelegramAdapter) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	var update struct {
		CallbackQuery *struct {
			ID   string `json:"id"`
			From struct {
				ID       int64  `json:"id"`
				Username string `json:"username"`
			} `json:"from"`
			Data string `json:"data"`
		} `json:"callback_query"`
		Message *struct {
			From struct {
				ID       int64  `json:"id"`
				Username string `json:"username"`
			} `json:"from"`
			Text           string `json:"text"`
			ReplyToMessage *struct {
				Text string `json:"text"`
			} `json:"reply_to_message"`
		} `json:"message"`
	}

	if err := json.Unmarshal(body, &update); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Handle callback query (button press)
	if update.CallbackQuery != nil {
		parts := strings.SplitN(update.CallbackQuery.Data, ":", 2)
		if len(parts) == 2 {
			response := &Response{
				ResponseID: parts[1],
				Action:     parts[0],
				Source:     t.Name(),
				UserID:     fmt.Sprintf("%d", update.CallbackQuery.From.ID),
				Timestamp:  time.Now(),
			}

			if t.responseHandler != nil {
				t.responseHandler(response)
			}

			// Answer the callback query
			t.answerCallbackQuery(r.Context(), update.CallbackQuery.ID)
		}
	}

	// Handle reply message
	if update.Message != nil && update.Message.ReplyToMessage != nil {
		// Extract event ID from the original message if possible
		response := &Response{
			Message:   update.Message.Text,
			Source:    t.Name(),
			UserID:    fmt.Sprintf("%d", update.Message.From.ID),
			Timestamp: time.Now(),
		}

		if t.responseHandler != nil {
			t.responseHandler(response)
		}
	}

	w.WriteHeader(http.StatusOK)
}

// answerCallbackQuery acknowledges a button press
func (t *TelegramAdapter) answerCallbackQuery(ctx context.Context, queryID string) {
	url := fmt.Sprintf("%s/answerCallbackQuery", t.apiBase)
	payload := map[string]interface{}{
		"callback_query_id": queryID,
		"text":              "Response received",
	}

	data, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := t.client.Do(req)
	if resp != nil {
		resp.Body.Close()
	}
}

// Close cleans up resources
func (t *TelegramAdapter) Close() error {
	if t.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		t.deleteWebhook(ctx)
		t.server.Shutdown(ctx)
	}
	t.client.CloseIdleConnections()
	return nil
}

// FormatTelegramStatusSummary creates a Telegram message for a status summary
func FormatTelegramStatusSummary(summary *StatusSummary) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("\U0001F4CA <b>Status Update: %s</b>\n\n", summary.RepoName))

	sb.WriteString(fmt.Sprintf(
		"<b>Active Workers:</b> %d\n<b>Pending Questions:</b> %d\n<b>Completed:</b> %d\n\n",
		summary.ActiveWorkers,
		summary.PendingQuestions,
		summary.CompletedTasks,
	))

	if len(summary.Agents) > 0 {
		sb.WriteString("<b>Agents:</b>\n")
		for _, agent := range summary.Agents {
			statusEmoji := "\u26AA" // White circle
			switch agent.Status {
			case "working":
				statusEmoji = "\U0001F7E2" // Green circle
			case "waiting":
				statusEmoji = "\U0001F7E1" // Yellow circle
			case "stuck":
				statusEmoji = "\U0001F534" // Red circle
			case "completed":
				statusEmoji = "\u2705" // Check mark
			}

			sb.WriteString(fmt.Sprintf("%s <b>%s</b> (%s)", statusEmoji, agent.Name, agent.Type))
			if agent.Task != "" {
				sb.WriteString(fmt.Sprintf("\n  <i>%s</i>", agent.Task))
			}
			if agent.Duration != "" {
				sb.WriteString(fmt.Sprintf(" (%s)", agent.Duration))
			}
			sb.WriteString("\n")
		}
	}

	sb.WriteString(fmt.Sprintf("\n<code>Generated: %s</code>", summary.GeneratedAt.Format("15:04:05")))

	return sb.String()
}
