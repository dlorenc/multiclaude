// Package provider handles CLI provider resolution and validation.
// It supports multiple CLI backends (claude, happy) with per-repository configuration.
package provider

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/dlorenc/multiclaude/internal/state"
)

const (
	// HappyAuthFile is the path to happy's auth file relative to home directory
	HappyAuthFile = ".happy/access.key"
	// EnvProvider is the environment variable to override provider
	EnvProvider = "MULTICLAUDE_PROVIDER"
)

// Info contains resolved provider information
type Info struct {
	Type       state.ProviderType
	BinaryPath string
}

// Resolve resolves the binary path for a given provider type.
// It checks the MULTICLAUDE_PROVIDER environment override first, then uses the provided type.
// For happy provider, it also validates that authentication is configured.
func Resolve(providerType state.ProviderType) (*Info, error) {
	// Check environment override
	if envProvider := os.Getenv(EnvProvider); envProvider != "" {
		providerType = state.ProviderType(envProvider)
	}

	// Default to claude if empty
	if providerType == "" {
		providerType = state.ProviderClaude
	}

	// Validate provider type
	if providerType != state.ProviderClaude && providerType != state.ProviderHappy {
		return nil, &InvalidProviderError{Provider: string(providerType)}
	}

	binaryName := string(providerType)

	// Resolve binary path
	binaryPath, err := exec.LookPath(binaryName)
	if err != nil {
		return nil, &NotFoundError{Provider: providerType, Cause: err}
	}

	// For happy, verify auth exists
	if providerType == state.ProviderHappy {
		if err := ValidateHappyAuth(); err != nil {
			return nil, err
		}
	}

	return &Info{
		Type:       providerType,
		BinaryPath: binaryPath,
	}, nil
}

// ValidateHappyAuth checks if happy authentication is configured.
// Returns nil if auth is configured, error otherwise.
func ValidateHappyAuth() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	authPath := filepath.Join(home, HappyAuthFile)
	if _, err := os.Stat(authPath); os.IsNotExist(err) {
		return &AuthNotConfiguredError{Provider: state.ProviderHappy}
	}

	return nil
}

// NotFoundError indicates the provider binary was not found in PATH
type NotFoundError struct {
	Provider state.ProviderType
	Cause    error
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s binary not found in PATH", e.Provider)
}

func (e *NotFoundError) Unwrap() error {
	return e.Cause
}

// AuthNotConfiguredError indicates the provider auth is not configured
type AuthNotConfiguredError struct {
	Provider state.ProviderType
}

func (e *AuthNotConfiguredError) Error() string {
	return fmt.Sprintf("%s authentication not configured", e.Provider)
}

// InvalidProviderError indicates an invalid provider type was specified
type InvalidProviderError struct {
	Provider string
}

func (e *InvalidProviderError) Error() string {
	return fmt.Sprintf("invalid provider: %s (must be 'claude' or 'happy')", e.Provider)
}
