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
