// Package templates provides embedded agent templates that are copied to
// per-repository agent directories during initialization.
package templates

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Embed the agent-templates directory from the repository root
//
//go:embed all:agent-templates
var agentTemplates embed.FS

// CopyAgentTemplates copies all agent template files from the embedded
// agent-templates directory to the specified destination directory.
// The destination directory will be created if it doesn't exist.
func CopyAgentTemplates(destDir string) error {
	// Create the destination directory
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create agents directory: %w", err)
	}

	// Walk the embedded filesystem and copy all .md files
	err := fs.WalkDir(agentTemplates, "agent-templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip the root directory
		if path == "agent-templates" {
			return nil
		}

		// Only copy .md files
		if d.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}

		// Read the embedded file
		content, err := agentTemplates.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read template %s: %w", path, err)
		}

		// Get the filename (strip the "agent-templates/" prefix)
		filename := filepath.Base(path)
		destPath := filepath.Join(destDir, filename)

		// Write to destination
		if err := os.WriteFile(destPath, content, 0644); err != nil {
			return fmt.Errorf("failed to write template %s: %w", destPath, err)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to copy agent templates: %w", err)
	}

	return nil
}

// ListAgentTemplates returns the names of all available agent templates.
func ListAgentTemplates() ([]string, error) {
	var templates []string

	entries, err := agentTemplates.ReadDir("agent-templates")
	if err != nil {
		return nil, fmt.Errorf("failed to read agent templates: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".md" {
			templates = append(templates, entry.Name())
		}
	}

	return templates, nil
}
