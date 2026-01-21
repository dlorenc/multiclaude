package config

import (
	"testing"
)

func TestDirectoryDocs(t *testing.T) {
	docs := DirectoryDocs()

	// Verify we have documentation entries
	if len(docs) == 0 {
		t.Fatal("DirectoryDocs() returned empty slice")
	}

	// Verify each doc has required fields
	for i, doc := range docs {
		if doc.Path == "" {
			t.Errorf("DirectoryDocs()[%d].Path is empty", i)
		}
		if doc.Description == "" {
			t.Errorf("DirectoryDocs()[%d].Description is empty for path %q", i, doc.Path)
		}
		if doc.Type != "file" && doc.Type != "directory" {
			t.Errorf("DirectoryDocs()[%d].Type = %q, want 'file' or 'directory' for path %q", i, doc.Type, doc.Path)
		}
	}

	// Verify key paths are documented
	requiredPaths := []string{
		"daemon.pid",
		"daemon.sock",
		"daemon.log",
		"state.json",
		"repos/",
		"wts/",
		"messages/",
	}

	pathSet := make(map[string]bool)
	for _, doc := range docs {
		pathSet[doc.Path] = true
	}

	for _, required := range requiredPaths {
		if !pathSet[required] {
			t.Errorf("DirectoryDocs() missing documentation for required path %q", required)
		}
	}
}

func TestStateDocs(t *testing.T) {
	docs := StateDocs()

	// Verify we have documentation entries
	if len(docs) == 0 {
		t.Fatal("StateDocs() returned empty slice")
	}

	// Verify each doc has required fields
	for i, doc := range docs {
		if doc.Field == "" {
			t.Errorf("StateDocs()[%d].Field is empty", i)
		}
		if doc.Type == "" {
			t.Errorf("StateDocs()[%d].Type is empty for field %q", i, doc.Field)
		}
		if doc.Description == "" {
			t.Errorf("StateDocs()[%d].Description is empty for field %q", i, doc.Field)
		}
	}

	// Verify key fields are documented
	requiredFields := []string{
		"repos",
		"repos.<name>.github_url",
		"repos.<name>.agents",
	}

	fieldSet := make(map[string]bool)
	for _, doc := range docs {
		fieldSet[doc.Field] = true
	}

	for _, required := range requiredFields {
		if !fieldSet[required] {
			t.Errorf("StateDocs() missing documentation for required field %q", required)
		}
	}
}

func TestMessageDocs(t *testing.T) {
	docs := MessageDocs()

	// Verify we have documentation entries
	if len(docs) == 0 {
		t.Fatal("MessageDocs() returned empty slice")
	}

	// Verify each doc has required fields
	for i, doc := range docs {
		if doc.Field == "" {
			t.Errorf("MessageDocs()[%d].Field is empty", i)
		}
		if doc.Type == "" {
			t.Errorf("MessageDocs()[%d].Type is empty for field %q", i, doc.Field)
		}
		if doc.Description == "" {
			t.Errorf("MessageDocs()[%d].Description is empty for field %q", i, doc.Field)
		}
	}

	// Verify key fields are documented
	requiredFields := []string{
		"id",
		"from",
		"to",
		"timestamp",
		"body",
		"status",
	}

	fieldSet := make(map[string]bool)
	for _, doc := range docs {
		fieldSet[doc.Field] = true
	}

	for _, required := range requiredFields {
		if !fieldSet[required] {
			t.Errorf("MessageDocs() missing documentation for required field %q", required)
		}
	}
}
