package notify

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if !config.Enabled {
		t.Error("default config should be enabled")
	}

	if config.RateLimit == nil {
		t.Error("default config should have rate limit")
	}

	if config.RateLimit.MaxPerMinute != 10 {
		t.Errorf("default max_per_minute = %d, want 10", config.RateLimit.MaxPerMinute)
	}

	// Check default events
	expectedEvents := []string{
		"agent.question", "agent.completed", "agent.stuck",
		"agent.error", "pr.created", "pr.merged", "ci.failed",
	}
	for _, event := range expectedEvents {
		if _, ok := config.Events[event]; !ok {
			t.Errorf("default config missing event: %s", event)
		}
	}
}

func TestLoadConfig(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Test loading non-existent file returns default config
	config, err := LoadConfig(filepath.Join(tmpDir, "nonexistent.yaml"))
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if !config.Enabled {
		t.Error("default config should be enabled")
	}

	// Create a test config file
	configContent := `
enabled: true
channels:
  - type: webhook
    name: test-webhook
    enabled: true
    url: https://example.com/webhook
  - type: slack
    name: test-slack
    enabled: true
    webhook_url: https://hooks.slack.com/services/xxx
events:
  agent.question:
    enabled: true
    channels: ["test-webhook", "test-slack"]
  agent.completed:
    enabled: false
rate_limit:
  max_per_minute: 20
  cooldown_after_burst: 120
`
	configPath := filepath.Join(tmpDir, "notifications.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	config, err = LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if !config.Enabled {
		t.Error("config should be enabled")
	}
	if len(config.Channels) != 2 {
		t.Errorf("expected 2 channels, got %d", len(config.Channels))
	}
	if config.RateLimit.MaxPerMinute != 20 {
		t.Errorf("max_per_minute = %d, want 20", config.RateLimit.MaxPerMinute)
	}
}

func TestExpandEnvVars(t *testing.T) {
	// Set test environment variables
	os.Setenv("TEST_VAR", "test-value")
	os.Setenv("TEST_TOKEN", "secret-token")
	defer func() {
		os.Unsetenv("TEST_VAR")
		os.Unsetenv("TEST_TOKEN")
	}()

	tests := []struct {
		input    string
		expected string
	}{
		// Simple ${VAR} syntax
		{"${TEST_VAR}", "test-value"},
		{"prefix-${TEST_VAR}-suffix", "prefix-test-value-suffix"},
		// $VAR syntax
		{"$TEST_VAR", "test-value"},
		{"prefix-$TEST_VAR-suffix", "prefix-test-value-suffix"},
		// Default value syntax
		{"${NONEXISTENT:-default}", "default"},
		{"${TEST_VAR:-default}", "test-value"},
		// Multiple variables
		{"${TEST_VAR} and ${TEST_TOKEN}", "test-value and secret-token"},
		// No variables
		{"no variables here", "no variables here"},
		// Empty result for missing var without default
		{"$NONEXISTENT_VAR", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := expandEnvVars(tt.input)
			if result != tt.expected {
				t.Errorf("expandEnvVars(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestConfig_Validate(t *testing.T) {
	// Valid config
	config := &Config{
		Enabled: true,
		Channels: []ChannelConfig{
			{Type: "webhook", Name: "test", Enabled: true, URL: "https://example.com"},
		},
		Events: map[string]EventConfig{
			"agent.question": {Enabled: true, Channels: []string{"test"}},
		},
	}

	if err := config.Validate(); err != nil {
		t.Errorf("valid config should pass validation: %v", err)
	}

	// Duplicate channel name
	config2 := &Config{
		Enabled: true,
		Channels: []ChannelConfig{
			{Type: "webhook", Name: "test", Enabled: true, URL: "https://example.com"},
			{Type: "webhook", Name: "test", Enabled: true, URL: "https://example2.com"},
		},
	}

	if err := config2.Validate(); err == nil {
		t.Error("config with duplicate channel names should fail validation")
	}

	// Unknown channel in event
	config3 := &Config{
		Enabled: true,
		Channels: []ChannelConfig{
			{Type: "webhook", Name: "test", Enabled: true, URL: "https://example.com"},
		},
		Events: map[string]EventConfig{
			"agent.question": {Enabled: true, Channels: []string{"unknown-channel"}},
		},
	}

	if err := config3.Validate(); err == nil {
		t.Error("config with unknown channel reference should fail validation")
	}

	// Missing required fields
	config4 := &Config{
		Enabled: true,
		Channels: []ChannelConfig{
			{Type: "webhook", Name: "test", Enabled: true}, // Missing URL
		},
	}

	if err := config4.Validate(); err == nil {
		t.Error("webhook without URL should fail validation")
	}

	// Invalid adapter type
	config5 := &Config{
		Enabled: true,
		Channels: []ChannelConfig{
			{Type: "invalid", Name: "test", Enabled: true},
		},
	}

	if err := config5.Validate(); err == nil {
		t.Error("config with invalid adapter type should fail validation")
	}
}

func TestConfig_CreateAdapters(t *testing.T) {
	config := &Config{
		Enabled: true,
		Channels: []ChannelConfig{
			{Type: "webhook", Name: "webhook1", Enabled: true, URL: "https://example.com/webhook"},
			{Type: "webhook", Name: "webhook2", Enabled: false, URL: "https://example.com/webhook2"},
		},
	}

	adapters, err := config.CreateAdapters()
	if err != nil {
		t.Fatalf("CreateAdapters failed: %v", err)
	}

	// Should only create enabled adapters
	if len(adapters) != 1 {
		t.Errorf("expected 1 enabled adapter, got %d", len(adapters))
	}

	if adapters[0].Name() != "webhook1" {
		t.Errorf("adapter name = %s, want webhook1", adapters[0].Name())
	}
}

func TestConfig_GetChannelsForEvent(t *testing.T) {
	config := &Config{
		Enabled: true,
		Channels: []ChannelConfig{
			{Type: "webhook", Name: "ch1", Enabled: true, URL: "https://example.com"},
			{Type: "webhook", Name: "ch2", Enabled: true, URL: "https://example.com"},
			{Type: "webhook", Name: "ch3", Enabled: false, URL: "https://example.com"},
		},
		Events: map[string]EventConfig{
			"agent.question": {Enabled: true, Channels: []string{"ch1"}},
			"agent.error":    {Enabled: true}, // No channels specified
			"agent.stuck":    {Enabled: false, Channels: []string{"ch1", "ch2"}},
		},
	}

	// Event with specific channels
	channels := config.GetChannelsForEvent(EventAgentQuestion)
	if len(channels) != 1 || channels[0] != "ch1" {
		t.Errorf("GetChannelsForEvent(agent.question) = %v, want [ch1]", channels)
	}

	// Event with no channels specified (should use all enabled)
	channels = config.GetChannelsForEvent(EventAgentError)
	if len(channels) != 2 {
		t.Errorf("GetChannelsForEvent(agent.error) should return all enabled channels, got %d", len(channels))
	}

	// Disabled event
	channels = config.GetChannelsForEvent(EventAgentStuck)
	if channels != nil {
		t.Error("disabled event should return nil channels")
	}

	// Unknown event
	channels = config.GetChannelsForEvent(EventType("unknown"))
	if channels != nil {
		t.Error("unknown event should return nil channels")
	}
}

func TestConfig_GetHubConfig(t *testing.T) {
	config := &Config{
		Enabled: true,
		RateLimit: &RateLimitConfig{
			MaxPerMinute:       50,
			CooldownAfterBurst: 300,
		},
		QuietHours: &QuietHours{
			Enabled:  true,
			Start:    "22:00",
			End:      "08:00",
			Timezone: "America/New_York",
		},
	}

	hubConfig := config.GetHubConfig()

	if hubConfig.RateLimit != 50 {
		t.Errorf("RateLimit = %d, want 50", hubConfig.RateLimit)
	}
	if hubConfig.CooldownAfterBurst != 300 {
		t.Errorf("CooldownAfterBurst = %d, want 300", hubConfig.CooldownAfterBurst)
	}
	if hubConfig.QuietHours == nil || !hubConfig.QuietHours.Enabled {
		t.Error("QuietHours should be set and enabled")
	}
}

func TestSaveConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "notifications.yaml")

	config := &Config{
		Enabled: true,
		Channels: []ChannelConfig{
			{Type: "webhook", Name: "test", Enabled: true, URL: "https://example.com"},
		},
	}

	if err := SaveConfig(config, configPath); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// Load it back
	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if !loaded.Enabled {
		t.Error("loaded config should be enabled")
	}
	if len(loaded.Channels) != 1 {
		t.Errorf("loaded config should have 1 channel, got %d", len(loaded.Channels))
	}
}
