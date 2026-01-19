package provider

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dlorenc/multiclaude/internal/state"
)

func TestResolve_DefaultClaude(t *testing.T) {
	// Empty provider should default to claude
	info, err := Resolve("")
	if err != nil {
		// Skip if claude not installed
		if _, ok := err.(*NotFoundError); ok {
			t.Skip("claude binary not installed")
		}
		t.Fatalf("unexpected error: %v", err)
	}

	if info.Type != state.ProviderClaude {
		t.Errorf("expected provider type %q, got %q", state.ProviderClaude, info.Type)
	}
	if info.BinaryPath == "" {
		t.Error("expected non-empty binary path")
	}
}

func TestResolve_ExplicitClaude(t *testing.T) {
	info, err := Resolve(state.ProviderClaude)
	if err != nil {
		if _, ok := err.(*NotFoundError); ok {
			t.Skip("claude binary not installed")
		}
		t.Fatalf("unexpected error: %v", err)
	}

	if info.Type != state.ProviderClaude {
		t.Errorf("expected provider type %q, got %q", state.ProviderClaude, info.Type)
	}
}

func TestResolve_Happy(t *testing.T) {
	info, err := Resolve(state.ProviderHappy)
	if err != nil {
		// Could be not found or auth not configured
		if _, ok := err.(*NotFoundError); ok {
			t.Skip("happy binary not installed")
		}
		if _, ok := err.(*AuthNotConfiguredError); ok {
			t.Skip("happy auth not configured")
		}
		t.Fatalf("unexpected error: %v", err)
	}

	if info.Type != state.ProviderHappy {
		t.Errorf("expected provider type %q, got %q", state.ProviderHappy, info.Type)
	}
}

func TestResolve_EnvOverride(t *testing.T) {
	// Set env var to override
	os.Setenv(EnvProvider, "claude")
	defer os.Unsetenv(EnvProvider)

	// Even if we pass happy, env should override to claude
	info, err := Resolve(state.ProviderHappy)
	if err != nil {
		if _, ok := err.(*NotFoundError); ok {
			t.Skip("claude binary not installed")
		}
		t.Fatalf("unexpected error: %v", err)
	}

	if info.Type != state.ProviderClaude {
		t.Errorf("expected provider type %q (from env), got %q", state.ProviderClaude, info.Type)
	}
}

func TestResolve_InvalidProvider(t *testing.T) {
	_, err := Resolve("invalid-provider")
	if err == nil {
		t.Fatal("expected error for invalid provider")
	}

	if _, ok := err.(*InvalidProviderError); !ok {
		t.Errorf("expected InvalidProviderError, got %T: %v", err, err)
	}
}

func TestValidateHappyAuth_Missing(t *testing.T) {
	// Create a temp home directory without auth file
	tmpHome, err := os.MkdirTemp("", "happy-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpHome)

	// Save and restore HOME
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	err = ValidateHappyAuth()
	if err == nil {
		t.Fatal("expected error for missing auth file")
	}

	if _, ok := err.(*AuthNotConfiguredError); !ok {
		t.Errorf("expected AuthNotConfiguredError, got %T: %v", err, err)
	}
}

func TestValidateHappyAuth_Present(t *testing.T) {
	// Create a temp home directory with auth file
	tmpHome, err := os.MkdirTemp("", "happy-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpHome)

	// Create .happy/access.key
	happyDir := filepath.Join(tmpHome, ".happy")
	if err := os.MkdirAll(happyDir, 0755); err != nil {
		t.Fatal(err)
	}
	authFile := filepath.Join(happyDir, "access.key")
	if err := os.WriteFile(authFile, []byte("test-key"), 0644); err != nil {
		t.Fatal(err)
	}

	// Save and restore HOME
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	err = ValidateHappyAuth()
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestErrorMessages(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "NotFoundError",
			err:      &NotFoundError{Provider: state.ProviderHappy},
			expected: "happy binary not found in PATH",
		},
		{
			name:     "AuthNotConfiguredError",
			err:      &AuthNotConfiguredError{Provider: state.ProviderHappy},
			expected: "happy authentication not configured",
		},
		{
			name:     "InvalidProviderError",
			err:      &InvalidProviderError{Provider: "foobar"},
			expected: "invalid provider: foobar (must be 'claude' or 'happy')",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}
