package templates

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListAgentTemplates(t *testing.T) {
	templates, err := ListAgentTemplates()
	if err != nil {
		t.Fatalf("ListAgentTemplates failed: %v", err)
	}

	// Check that we have the expected templates
	expected := map[string]bool{
		"merge-queue.md": true,
		"worker.md":      true,
		"reviewer.md":    true,
	}

	if len(templates) != len(expected) {
		t.Errorf("Expected %d templates, got %d: %v", len(expected), len(templates), templates)
	}

	for _, tmpl := range templates {
		if !expected[tmpl] {
			t.Errorf("Unexpected template: %s", tmpl)
		}
	}
}

func TestCopyAgentTemplates(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "templates-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	destDir := filepath.Join(tmpDir, "agents")

	// Copy templates
	if err := CopyAgentTemplates(destDir); err != nil {
		t.Fatalf("CopyAgentTemplates failed: %v", err)
	}

	// Verify the destination directory was created
	if _, err := os.Stat(destDir); os.IsNotExist(err) {
		t.Error("Destination directory was not created")
	}

	// Verify all expected files exist and have content
	expectedFiles := []string{"merge-queue.md", "worker.md", "reviewer.md"}
	for _, filename := range expectedFiles {
		path := filepath.Join(destDir, filename)
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			t.Errorf("Expected file %s does not exist", filename)
			continue
		}
		if err != nil {
			t.Errorf("Error checking file %s: %v", filename, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("File %s is empty", filename)
		}
	}
}

func TestCopyAgentTemplatesIdempotent(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "templates-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	destDir := filepath.Join(tmpDir, "agents")

	// Copy templates twice - should not error
	if err := CopyAgentTemplates(destDir); err != nil {
		t.Fatalf("First CopyAgentTemplates failed: %v", err)
	}
	if err := CopyAgentTemplates(destDir); err != nil {
		t.Fatalf("Second CopyAgentTemplates failed: %v", err)
	}
}

func TestCopyAgentTemplatesErrorHandling(t *testing.T) {
	t.Run("errors when destination is read-only", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "templates-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		// Create a read-only directory
		destDir := filepath.Join(tmpDir, "readonly")
		if err := os.MkdirAll(destDir, 0755); err != nil {
			t.Fatalf("Failed to create readonly dir: %v", err)
		}

		// Make directory read-only
		if err := os.Chmod(destDir, 0444); err != nil {
			t.Fatalf("Failed to chmod: %v", err)
		}
		defer os.Chmod(destDir, 0755) // Restore permissions for cleanup

		// Attempt to copy should fail when trying to write files
		err = CopyAgentTemplates(destDir)
		if err == nil {
			t.Error("Expected error when writing to read-only directory")
		}
	})

	t.Run("handles nested directory creation", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "templates-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		// Use a nested path that doesn't exist
		destDir := filepath.Join(tmpDir, "level1", "level2", "agents")

		// Should create all parent directories
		if err := CopyAgentTemplates(destDir); err != nil {
			t.Fatalf("CopyAgentTemplates failed with nested path: %v", err)
		}

		// Verify directory was created
		if _, err := os.Stat(destDir); os.IsNotExist(err) {
			t.Error("Nested destination directory was not created")
		}

		// Verify files were copied
		expectedFiles := []string{"merge-queue.md", "worker.md", "reviewer.md"}
		for _, filename := range expectedFiles {
			path := filepath.Join(destDir, filename)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Errorf("Expected file %s does not exist in nested directory", filename)
			}
		}
	})

	t.Run("handles empty destination path", func(t *testing.T) {
		// While empty string is technically valid (current directory),
		// the function should handle it gracefully
		tmpDir, err := os.MkdirTemp("", "templates-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		// Change to temp directory
		oldDir, err := os.Getwd()
		if err != nil {
			t.Fatalf("Failed to get working directory: %v", err)
		}
		defer os.Chdir(oldDir)

		if err := os.Chdir(tmpDir); err != nil {
			t.Fatalf("Failed to change directory: %v", err)
		}

		// Use "." as destination
		if err := CopyAgentTemplates("."); err != nil {
			t.Fatalf("CopyAgentTemplates failed with '.' path: %v", err)
		}

		// Verify files were copied to current directory
		expectedFiles := []string{"merge-queue.md", "worker.md", "reviewer.md"}
		for _, filename := range expectedFiles {
			if _, err := os.Stat(filename); os.IsNotExist(err) {
				t.Errorf("Expected file %s does not exist", filename)
			}
		}
	})
}

func TestListAgentTemplatesConsistency(t *testing.T) {
	// List templates
	templates, err := ListAgentTemplates()
	if err != nil {
		t.Fatalf("ListAgentTemplates failed: %v", err)
	}

	// Copy to a temp directory
	tmpDir, err := os.MkdirTemp("", "templates-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := CopyAgentTemplates(tmpDir); err != nil {
		t.Fatalf("CopyAgentTemplates failed: %v", err)
	}

	// Read what was actually copied
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read copied directory: %v", err)
	}

	var copiedFiles []string
	for _, entry := range entries {
		if !entry.IsDir() {
			copiedFiles = append(copiedFiles, entry.Name())
		}
	}

	// Lists should match
	if len(templates) != len(copiedFiles) {
		t.Errorf("ListAgentTemplates returned %d files but %d were copied", len(templates), len(copiedFiles))
	}

	templateMap := make(map[string]bool)
	for _, tmpl := range templates {
		templateMap[tmpl] = true
	}

	for _, copied := range copiedFiles {
		if !templateMap[copied] {
			t.Errorf("File %s was copied but not in ListAgentTemplates result", copied)
		}
	}
}
