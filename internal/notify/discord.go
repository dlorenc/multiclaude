package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DiscordConfig configures the Discord adapter
type DiscordConfig struct {
	AdapterConfig `yaml:",inline"`
	// WebhookURL is the Discord webhook URL
	WebhookURL string `yaml:"webhook_url" json:"webhook_url"`
	// Username is the display name for the webhook (optional)
	Username string `yaml:"username" json:"username"`
	// AvatarURL is the avatar image URL (optional)
	AvatarURL string `yaml:"avatar_url" json:"avatar_url"`
	// Timeout for HTTP requests
	Timeout time.Duration `yaml:"timeout" json:"timeout"`
}

// DiscordAdapter sends notifications via Discord webhooks
type DiscordAdapter struct {
	config *DiscordConfig
	client *http.Client
}

// NewDiscordAdapter creates a new Discord adapter
func NewDiscordAdapter(config *DiscordConfig) (*DiscordAdapter, error) {
	if config.WebhookURL == "" {
		return nil, fmt.Errorf("webhook_url is required")
	}

	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}
	if config.Username == "" {
		config.Username = "Multiclaude"
	}

	return &DiscordAdapter{
		config: config,
		client: &http.Client{Timeout: config.Timeout},
	}, nil
}

// Name returns the adapter name
func (d *DiscordAdapter) Name() string {
	return d.config.Name
}

// Type returns "discord"
func (d *DiscordAdapter) Type() string {
	return "discord"
}

// Send sends an event to Discord
func (d *DiscordAdapter) Send(ctx context.Context, event *Event) error {
	embed := d.formatEmbed(event)

	payload := map[string]interface{}{
		"embeds": []interface{}{embed},
	}

	if d.config.Username != "" {
		payload["username"] = d.config.Username
	}
	if d.config.AvatarURL != "" {
		payload["avatar_url"] = d.config.AvatarURL
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", d.config.WebhookURL, bytes.NewReader(data))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("discord returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// formatEmbed creates a Discord embed for an event
func (d *DiscordAdapter) formatEmbed(event *Event) map[string]interface{} {
	embed := map[string]interface{}{
		"title":       fmt.Sprintf("%s %s", d.eventEmoji(event.Type), event.Title),
		"description": event.Message,
		"color":       d.eventColor(event),
		"timestamp":   event.Timestamp.Format(time.RFC3339),
	}

	// Add fields for metadata
	fields := []map[string]interface{}{}

	fields = append(fields, map[string]interface{}{
		"name":   "Repository",
		"value":  event.RepoName,
		"inline": true,
	})

	if event.AgentName != "" {
		fields = append(fields, map[string]interface{}{
			"name":   "Agent",
			"value":  fmt.Sprintf("%s (%s)", event.AgentName, event.AgentType),
			"inline": true,
		})
	}

	fields = append(fields, map[string]interface{}{
		"name":   "Priority",
		"value":  string(event.Priority),
		"inline": true,
	})

	// Add context fields
	for key, value := range event.Context {
		fields = append(fields, map[string]interface{}{
			"name":   key,
			"value":  value,
			"inline": false,
		})
	}

	embed["fields"] = fields

	// Add footer
	embed["footer"] = map[string]interface{}{
		"text": fmt.Sprintf("Event ID: %s", event.ID),
	}

	return embed
}

// eventEmoji returns an appropriate emoji for the event type
func (d *DiscordAdapter) eventEmoji(eventType EventType) string {
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
		return "\U0001F500" // Shuffle
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

// eventColor returns the embed color based on priority/type
func (d *DiscordAdapter) eventColor(event *Event) int {
	// Discord colors are decimal RGB values
	switch event.Priority {
	case PriorityHigh:
		return 15158332 // Red (#E74C3C)
	case PriorityMedium:
		return 15105570 // Orange (#E67E22)
	default:
		return 3447003 // Blue (#3498DB)
	}
}

// SupportsResponses returns false for Discord webhooks
func (d *DiscordAdapter) SupportsResponses() bool {
	return false
}

// Close cleans up resources
func (d *DiscordAdapter) Close() error {
	d.client.CloseIdleConnections()
	return nil
}

// FormatDiscordStatusSummary creates a Discord embed for a status summary
func FormatDiscordStatusSummary(summary *StatusSummary) map[string]interface{} {
	embed := map[string]interface{}{
		"title":     fmt.Sprintf("\U0001F4CA Status Update: %s", summary.RepoName),
		"color":     3447003, // Blue
		"timestamp": summary.GeneratedAt.Format(time.RFC3339),
	}

	fields := []map[string]interface{}{
		{
			"name":   "Active Workers",
			"value":  fmt.Sprintf("%d", summary.ActiveWorkers),
			"inline": true,
		},
		{
			"name":   "Pending Questions",
			"value":  fmt.Sprintf("%d", summary.PendingQuestions),
			"inline": true,
		},
		{
			"name":   "Completed Tasks",
			"value":  fmt.Sprintf("%d", summary.CompletedTasks),
			"inline": true,
		},
	}

	// Add agent details
	if len(summary.Agents) > 0 {
		var agentList string
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

			agentList += fmt.Sprintf("%s **%s** (%s)", statusEmoji, agent.Name, agent.Type)
			if agent.Task != "" {
				agentList += fmt.Sprintf("\n  _%s_", agent.Task)
			}
			agentList += "\n"
		}

		fields = append(fields, map[string]interface{}{
			"name":   "Agents",
			"value":  agentList,
			"inline": false,
		})
	}

	embed["fields"] = fields

	return embed
}
