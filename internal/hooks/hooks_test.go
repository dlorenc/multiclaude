package hooks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyConfig(t *testing.T) {
	t.Run("no hooks config", func(t *testing.T) {
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		workDir := filepath.Join(tmpDir, "workdir")

		if err := os.MkdirAll(repoPath, 0755); err != nil {
			t.Fatalf("Failed to create repo dir: %v", err)
		}
		if err := os.MkdirAll(workDir, 0755); err != nil {
			t.Fatalf("Failed to create work dir: %v", err)
		}

		// Should succeed with no hooks config
		if err := CopyConfig(repoPath, workDir); err != nil {
			t.Errorf("CopyConfig() error = %v, want nil", err)
		}

		// .claude directory should not exist
		claudeDir := filepath.Join(workDir, ".claude")
		if _, err := os.Stat(claudeDir); !os.IsNotExist(err) {
			t.Error(".claude directory should not be created when no hooks config exists")
		}
	})

	t.Run("with hooks config", func(t *testing.T) {
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		workDir := filepath.Join(tmpDir, "workdir")

		// Create repo with hooks config
		hooksDir := filepath.Join(repoPath, ".multiclaude")
		if err := os.MkdirAll(hooksDir, 0755); err != nil {
			t.Fatalf("Failed to create hooks dir: %v", err)
		}
		if err := os.MkdirAll(workDir, 0755); err != nil {
			t.Fatalf("Failed to create work dir: %v", err)
		}

		// Write hooks config
		hooksContent := `{"hooks": {"test": "echo test"}}`
		hooksPath := filepath.Join(hooksDir, "hooks.json")
		if err := os.WriteFile(hooksPath, []byte(hooksContent), 0644); err != nil {
			t.Fatalf("Failed to write hooks config: %v", err)
		}

		// Copy config
		if err := CopyConfig(repoPath, workDir); err != nil {
			t.Errorf("CopyConfig() error = %v, want nil", err)
		}

		// Verify .claude/settings.json was created
		settingsPath := filepath.Join(workDir, ".claude", "settings.json")
		data, err := os.ReadFile(settingsPath)
		if err != nil {
			t.Fatalf("Failed to read settings.json: %v", err)
		}

		if string(data) != hooksContent {
			t.Errorf("settings.json content = %q, want %q", string(data), hooksContent)
		}
	})

	t.Run("existing .claude directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		workDir := filepath.Join(tmpDir, "workdir")

		// Create repo with hooks config
		hooksDir := filepath.Join(repoPath, ".multiclaude")
		if err := os.MkdirAll(hooksDir, 0755); err != nil {
			t.Fatalf("Failed to create hooks dir: %v", err)
		}

		// Create workdir with existing .claude directory
		claudeDir := filepath.Join(workDir, ".claude")
		if err := os.MkdirAll(claudeDir, 0755); err != nil {
			t.Fatalf("Failed to create claude dir: %v", err)
		}

		// Write hooks config
		hooksContent := `{"hooks": {"test": "echo test"}}`
		hooksPath := filepath.Join(hooksDir, "hooks.json")
		if err := os.WriteFile(hooksPath, []byte(hooksContent), 0644); err != nil {
			t.Fatalf("Failed to write hooks config: %v", err)
		}

		// Copy config - should succeed even with existing directory
		if err := CopyConfig(repoPath, workDir); err != nil {
			t.Errorf("CopyConfig() error = %v, want nil", err)
		}
	})
}
