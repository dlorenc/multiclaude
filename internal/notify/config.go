package notify

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the notifications configuration
type Config struct {
	// Enabled controls whether notifications are active
	Enabled bool `yaml:"enabled" json:"enabled"`
	// Channels defines the notification adapters
	Channels []ChannelConfig `yaml:"channels" json:"channels"`
	// Events configures which events trigger notifications
	Events map[string]EventConfig `yaml:"events" json:"events"`
	// QuietHours configures notification suppression periods
	QuietHours *QuietHours `yaml:"quiet_hours" json:"quiet_hours,omitempty"`
	// RateLimit configures rate limiting
	RateLimit *RateLimitConfig `yaml:"rate_limit" json:"rate_limit,omitempty"`
	// API configures the REST API endpoint
	API *APIConfig `yaml:"api" json:"api,omitempty"`
}

// ChannelConfig represents a notification channel configuration
type ChannelConfig struct {
	// Type is the adapter type (slack, telegram, discord, webhook)
	Type string `yaml:"type" json:"type"`
	// Name is a unique identifier for this channel
	Name string `yaml:"name" json:"name"`
	// Enabled controls whether this channel is active
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Webhook-specific fields
	URL     string            `yaml:"url,omitempty" json:"url,omitempty"`
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	Secret  string            `yaml:"secret,omitempty" json:"secret,omitempty"`

	// Slack-specific fields
	WebhookURL    string `yaml:"webhook_url,omitempty" json:"webhook_url,omitempty"`
	BotToken      string `yaml:"bot_token,omitempty" json:"bot_token,omitempty"`
	Channel       string `yaml:"channel,omitempty" json:"channel,omitempty"`
	SigningSecret string `yaml:"signing_secret,omitempty" json:"signing_secret,omitempty"`

	// Telegram-specific fields
	ChatID string `yaml:"chat_id,omitempty" json:"chat_id,omitempty"`

	// Discord-specific fields (uses URL for webhook_url)
	Username  string `yaml:"username,omitempty" json:"username,omitempty"`
	AvatarURL string `yaml:"avatar_url,omitempty" json:"avatar_url,omitempty"`

	// Common interactive fields
	ListenAddr   string `yaml:"listen_addr,omitempty" json:"listen_addr,omitempty"`
	ResponseURL  string `yaml:"response_url,omitempty" json:"response_url,omitempty"`
	ResponsePath string `yaml:"response_path,omitempty" json:"response_path,omitempty"`
}

// EventConfig configures notification behavior for an event type
type EventConfig struct {
	// Enabled controls whether this event triggers notifications
	Enabled bool `yaml:"enabled" json:"enabled"`
	// Channels lists which channels receive this event
	Channels []string `yaml:"channels" json:"channels"`
	// DelayMinutes delays notification (for events like agent.stuck)
	DelayMinutes int `yaml:"delay_minutes,omitempty" json:"delay_minutes,omitempty"`
	// MinPriority filters events below this priority
	MinPriority Priority `yaml:"min_priority,omitempty" json:"min_priority,omitempty"`
}

// RateLimitConfig configures rate limiting
type RateLimitConfig struct {
	// MaxPerMinute is the maximum notifications per minute per adapter
	MaxPerMinute int `yaml:"max_per_minute" json:"max_per_minute"`
	// CooldownAfterBurst is seconds to wait after hitting rate limit
	CooldownAfterBurst int `yaml:"cooldown_after_burst" json:"cooldown_after_burst"`
}

// APIConfig configures the REST API endpoint for dashboard integration
type APIConfig struct {
	// Enabled controls whether the API is active
	Enabled bool `yaml:"enabled" json:"enabled"`
	// ListenAddr is the address to listen on (e.g., ":8080")
	ListenAddr string `yaml:"listen_addr" json:"listen_addr"`
	// AuthToken is a bearer token for API authentication (optional)
	AuthToken string `yaml:"auth_token,omitempty" json:"auth_token,omitempty"`
	// CORSOrigins is a list of allowed CORS origins
	CORSOrigins []string `yaml:"cors_origins,omitempty" json:"cors_origins,omitempty"`
	// EnableSSE enables server-sent events for real-time updates
	EnableSSE bool `yaml:"enable_sse" json:"enable_sse"`
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		Enabled:  true,
		Channels: []ChannelConfig{},
		Events: map[string]EventConfig{
			"agent.question": {
				Enabled:  true,
				Channels: []string{},
			},
			"agent.completed": {
				Enabled:  true,
				Channels: []string{},
			},
			"agent.stuck": {
				Enabled:      true,
				DelayMinutes: 10,
				Channels:     []string{},
			},
			"agent.error": {
				Enabled:  true,
				Channels: []string{},
			},
			"pr.created": {
				Enabled:  true,
				Channels: []string{},
			},
			"pr.merged": {
				Enabled:  true,
				Channels: []string{},
			},
			"ci.failed": {
				Enabled:  true,
				Channels: []string{},
			},
		},
		RateLimit: &RateLimitConfig{
			MaxPerMinute:       10,
			CooldownAfterBurst: 60,
		},
	}
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Expand environment variables
	expanded := expandEnvVars(string(data))

	var config Config
	if err := yaml.Unmarshal([]byte(expanded), &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}

// LoadConfigFromPaths tries to load config from multiple paths
func LoadConfigFromPaths(paths ...string) (*Config, error) {
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return LoadConfig(path)
		}
	}
	return DefaultConfig(), nil
}

// DefaultConfigPaths returns the default config file paths
func DefaultConfigPaths() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	return []string{
		filepath.Join(home, ".multiclaude", "notifications.yaml"),
		filepath.Join(home, ".multiclaude", "notifications.yml"),
		".multiclaude/notifications.yaml",
		".multiclaude/notifications.yml",
	}, nil
}

// expandEnvVars expands ${VAR} and $VAR patterns in the string
func expandEnvVars(s string) string {
	// Match ${VAR} or ${VAR:-default}
	re := regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?::-([^}]*))?\}`)
	result := re.ReplaceAllStringFunc(s, func(match string) string {
		parts := re.FindStringSubmatch(match)
		if len(parts) >= 2 {
			varName := parts[1]
			defaultVal := ""
			if len(parts) >= 3 {
				defaultVal = parts[2]
			}
			if val := os.Getenv(varName); val != "" {
				return val
			}
			return defaultVal
		}
		return match
	})

	// Also match $VAR (without braces)
	re2 := regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)
	result = re2.ReplaceAllStringFunc(result, func(match string) string {
		varName := strings.TrimPrefix(match, "$")
		if val := os.Getenv(varName); val != "" {
			return val
		}
		return ""
	})

	return result
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil
	}

	channelNames := make(map[string]bool)
	for i, ch := range c.Channels {
		if ch.Name == "" {
			return fmt.Errorf("channel %d: name is required", i)
		}
		if channelNames[ch.Name] {
			return fmt.Errorf("duplicate channel name: %s", ch.Name)
		}
		channelNames[ch.Name] = true

		if err := c.validateChannel(&ch); err != nil {
			return fmt.Errorf("channel %s: %w", ch.Name, err)
		}
	}

	// Validate event channel references
	for eventType, eventConfig := range c.Events {
		for _, chName := range eventConfig.Channels {
			if !channelNames[chName] {
				return fmt.Errorf("event %s: unknown channel %q", eventType, chName)
			}
		}
	}

	return nil
}

// validateChannel validates a channel configuration
func (c *Config) validateChannel(ch *ChannelConfig) error {
	switch ch.Type {
	case "webhook":
		if ch.URL == "" {
			return fmt.Errorf("url is required for webhook adapter")
		}
	case "slack":
		if ch.WebhookURL == "" && ch.BotToken == "" {
			return fmt.Errorf("either webhook_url or bot_token is required for slack adapter")
		}
	case "telegram":
		if ch.BotToken == "" {
			return fmt.Errorf("bot_token is required for telegram adapter")
		}
		if ch.ChatID == "" {
			return fmt.Errorf("chat_id is required for telegram adapter")
		}
	case "discord":
		if ch.URL == "" && ch.WebhookURL == "" {
			return fmt.Errorf("url or webhook_url is required for discord adapter")
		}
	default:
		return fmt.Errorf("unknown channel type: %s", ch.Type)
	}
	return nil
}

// CreateAdapters creates adapters from the configuration
func (c *Config) CreateAdapters() ([]Adapter, error) {
	if !c.Enabled {
		return nil, nil
	}

	var adapters []Adapter

	for _, ch := range c.Channels {
		if !ch.Enabled {
			continue
		}

		adapter, err := createAdapter(ch)
		if err != nil {
			return nil, fmt.Errorf("failed to create adapter %s: %w", ch.Name, err)
		}
		adapters = append(adapters, adapter)
	}

	return adapters, nil
}

// createAdapter creates an adapter from channel configuration
func createAdapter(ch ChannelConfig) (Adapter, error) {
	switch ch.Type {
	case "webhook":
		config := &WebhookConfig{
			AdapterConfig: AdapterConfig{
				Name:    ch.Name,
				Type:    ch.Type,
				Enabled: ch.Enabled,
			},
			URL:     ch.URL,
			Headers: ch.Headers,
			Secret:  ch.Secret,
			Timeout: 10 * time.Second,
		}
		if ch.ListenAddr != "" {
			interactiveConfig := &InteractiveWebhookConfig{
				WebhookConfig: *config,
				ResponseURL:   ch.ResponseURL,
				ListenAddr:    ch.ListenAddr,
				ResponsePath:  ch.ResponsePath,
			}
			return NewInteractiveWebhookAdapter(interactiveConfig)
		}
		return NewWebhookAdapter(config)

	case "slack":
		config := &SlackConfig{
			AdapterConfig: AdapterConfig{
				Name:    ch.Name,
				Type:    ch.Type,
				Enabled: ch.Enabled,
			},
			WebhookURL:    ch.WebhookURL,
			BotToken:      ch.BotToken,
			Channel:       ch.Channel,
			SigningSecret: ch.SigningSecret,
			ListenAddr:    ch.ListenAddr,
			Timeout:       10 * time.Second,
		}
		return NewSlackAdapter(config)

	case "telegram":
		// For telegram, use BotToken field or fall back to secret
		botToken := ch.BotToken
		if botToken == "" {
			botToken = ch.Secret
		}
		config := &TelegramConfig{
			AdapterConfig: AdapterConfig{
				Name:    ch.Name,
				Type:    ch.Type,
				Enabled: ch.Enabled,
			},
			BotToken:   botToken,
			ChatID:     ch.ChatID,
			ListenAddr: ch.ListenAddr,
			WebhookURL: ch.ResponseURL,
			Timeout:    10 * time.Second,
		}
		return NewTelegramAdapter(config)

	case "discord":
		webhookURL := ch.URL
		if webhookURL == "" {
			webhookURL = ch.WebhookURL
		}
		config := &DiscordConfig{
			AdapterConfig: AdapterConfig{
				Name:    ch.Name,
				Type:    ch.Type,
				Enabled: ch.Enabled,
			},
			WebhookURL: webhookURL,
			Username:   ch.Username,
			AvatarURL:  ch.AvatarURL,
			Timeout:    10 * time.Second,
		}
		return NewDiscordAdapter(config)

	default:
		return nil, fmt.Errorf("unknown adapter type: %s", ch.Type)
	}
}

// GetEventFilter returns an EventFilter for the specified channels
func (c *Config) GetEventFilter(eventType string) *EventFilter {
	eventConfig, ok := c.Events[eventType]
	if !ok || !eventConfig.Enabled {
		return nil
	}

	return &EventFilter{
		EventTypes:  []EventType{EventType(eventType)},
		MinPriority: eventConfig.MinPriority,
	}
}

// GetChannelsForEvent returns the channel names that should receive an event
func (c *Config) GetChannelsForEvent(eventType EventType) []string {
	eventConfig, ok := c.Events[string(eventType)]
	if !ok || !eventConfig.Enabled {
		return nil
	}

	// If no channels specified, use all enabled channels
	if len(eventConfig.Channels) == 0 {
		var channels []string
		for _, ch := range c.Channels {
			if ch.Enabled {
				channels = append(channels, ch.Name)
			}
		}
		return channels
	}

	return eventConfig.Channels
}

// GetHubConfig returns a HubConfig from this configuration
func (c *Config) GetHubConfig() *HubConfig {
	config := DefaultHubConfig()

	if c.RateLimit != nil {
		config.RateLimit = c.RateLimit.MaxPerMinute
		config.CooldownAfterBurst = c.RateLimit.CooldownAfterBurst
	}

	config.QuietHours = c.QuietHours

	return config
}

// SaveConfig saves configuration to a YAML file
func SaveConfig(config *Config, path string) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
