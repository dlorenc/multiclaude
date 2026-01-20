// Package hooks provides utilities for managing Claude hooks configuration.
package hooks

import (
	"fmt"
	"os"
	"path/filepath"
)

// CopyConfig copies hooks configuration from repo to workdir if it exists.
// The hooks.json file in .multiclaude directory is copied to .claude/settings.json
// in the target directory, allowing Claude to use custom hooks in worktrees.
func CopyConfig(repoPath, workDir string) error {
	hooksPath := filepath.Join(repoPath, ".multiclaude", "hooks.json")

	// Check if hooks.json exists
	if _, err := os.Stat(hooksPath); os.IsNotExist(err) {
		return nil // No hooks config, that's fine
	} else if err != nil {
		return fmt.Errorf("failed to check hooks config: %w", err)
	}

	// Create .claude directory in workdir
	claudeDir := filepath.Join(workDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return fmt.Errorf("failed to create .claude directory: %w", err)
	}

	// Copy hooks.json to .claude/settings.json
	hooksData, err := os.ReadFile(hooksPath)
	if err != nil {
		return fmt.Errorf("failed to read hooks config: %w", err)
	}

	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, hooksData, 0644); err != nil {
		return fmt.Errorf("failed to write settings.json: %w", err)
	}

	return nil
}
