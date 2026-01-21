// Package commands provides embedded slash command templates for Claude Code agents.
//
// These commands are injected per-agent via CLAUDE_CONFIG_DIR to provide
// multiclaude-specific functionality within Claude Code sessions.
package commands

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

// Embedded command templates
//
//go:embed refresh.md status.md workers.md messages.md
var commandFS embed.FS

// CommandInfo describes a slash command
type CommandInfo struct {
	Name        string // Command name (without /)
	Filename    string // Source filename
	Description string // Brief description
}

// AvailableCommands lists all available slash commands
var AvailableCommands = []CommandInfo{
	{Name: "refresh", Filename: "refresh.md", Description: "Sync worktree with main branch"},
	{Name: "status", Filename: "status.md", Description: "Show system status"},
	{Name: "workers", Filename: "workers.md", Description: "List active workers"},
	{Name: "messages", Filename: "messages.md", Description: "Check inter-agent messages"},
}

// GetCommand returns the content of a specific command template
func GetCommand(name string) (string, error) {
	filename := name + ".md"
	content, err := commandFS.ReadFile(filename)
	if err != nil {
		return "", fmt.Errorf("command %q not found: %w", name, err)
	}
	return string(content), nil
}

// GenerateCommandsDir creates a commands directory with all slash commands
// at the specified path. Returns the path to the commands directory.
func GenerateCommandsDir(commandsDir string) error {
	// Create the commands directory
	if err := os.MkdirAll(commandsDir, 0755); err != nil {
		return fmt.Errorf("failed to create commands directory: %w", err)
	}

	// Write each command file
	for _, cmd := range AvailableCommands {
		content, err := commandFS.ReadFile(cmd.Filename)
		if err != nil {
			return fmt.Errorf("failed to read embedded command %s: %w", cmd.Name, err)
		}

		destPath := filepath.Join(commandsDir, cmd.Filename)
		if err := os.WriteFile(destPath, content, 0644); err != nil {
			return fmt.Errorf("failed to write command file %s: %w", cmd.Name, err)
		}
	}

	return nil
}

// SetupAgentCommands creates the Claude config directory structure for an agent
// and populates it with slash commands. Returns the path to the config directory
// that should be set as CLAUDE_CONFIG_DIR.
func SetupAgentCommands(configDir string) error {
	// Create the config directory
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Create and populate the commands subdirectory
	commandsDir := filepath.Join(configDir, "commands")
	if err := GenerateCommandsDir(commandsDir); err != nil {
		return err
	}

	return nil
}

// LinkGlobalCredentials creates a symlink from the Claude config directory's
// .credentials.json to the global ~/.claude/.credentials.json.
//
// When CLAUDE_CONFIG_DIR is set, Claude looks for credentials there, not in the
// global ~/.claude directory. This symlink ensures agents can access OAuth
// credentials without duplicating sensitive files.
//
// If global credentials don't exist (e.g., user is using API key), this is a no-op.
func LinkGlobalCredentials(configDir string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	globalCredFile := filepath.Join(home, ".claude", ".credentials.json")
	localCredFile := filepath.Join(configDir, ".credentials.json")

	// Check if global credentials exist
	if _, err := os.Stat(globalCredFile); os.IsNotExist(err) {
		// No global credentials - user might be using API key
		return nil
	}

	// Ensure the config directory exists
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Check if symlink already exists and is valid
	if linkTarget, err := os.Readlink(localCredFile); err == nil {
		if linkTarget == globalCredFile {
			// Already correctly linked
			return nil
		}
		// Invalid link, remove it
		os.Remove(localCredFile)
	} else if _, err := os.Stat(localCredFile); err == nil {
		// File exists but is not a symlink, remove it
		os.Remove(localCredFile)
	}

	// Create symlink
	if err := os.Symlink(globalCredFile, localCredFile); err != nil {
		return fmt.Errorf("failed to create credentials symlink: %w", err)
	}

	return nil
}
