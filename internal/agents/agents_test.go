package agents

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadLocalDefinitions(t *testing.T) {
	// Create temp directory structure
	tmpDir, err := os.MkdirTemp("", "agents-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	localAgentsDir := filepath.Join(tmpDir, "local", "agents")
	if err := os.MkdirAll(localAgentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create test agent definitions
	workerContent := `# Worker Agent

A task-based worker that completes assigned work.

## Your Role

Complete the assigned task.
`
	if err := os.WriteFile(filepath.Join(localAgentsDir, "worker.md"), []byte(workerContent), 0644); err != nil {
		t.Fatal(err)
	}

	reviewerContent := `# Code Reviewer

Reviews pull requests.
`
	if err := os.WriteFile(filepath.Join(localAgentsDir, "reviewer.md"), []byte(reviewerContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a non-.md file that should be ignored
	if err := os.WriteFile(filepath.Join(localAgentsDir, "readme.txt"), []byte("ignore me"), 0644); err != nil {
		t.Fatal(err)
	}

	reader := NewReader(localAgentsDir, "")
	defs, err := reader.ReadLocalDefinitions()
	if err != nil {
		t.Fatalf("ReadLocalDefinitions failed: %v", err)
	}

	if len(defs) != 2 {
		t.Fatalf("expected 2 definitions, got %d", len(defs))
	}

	// Check that we got the expected definitions
	defMap := make(map[string]Definition)
	for _, def := range defs {
		defMap[def.Name] = def
	}

	worker, ok := defMap["worker"]
	if !ok {
		t.Fatal("worker definition not found")
	}
	if worker.Source != SourceLocal {
		t.Errorf("expected source local, got %s", worker.Source)
	}
	if worker.Content != workerContent {
		t.Error("worker content mismatch")
	}

	reviewer, ok := defMap["reviewer"]
	if !ok {
		t.Fatal("reviewer definition not found")
	}
	if reviewer.Source != SourceLocal {
		t.Errorf("expected source local, got %s", reviewer.Source)
	}
}

func TestReadRepoDefinitions(t *testing.T) {
	// Create temp directory structure
	tmpDir, err := os.MkdirTemp("", "agents-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	repoPath := filepath.Join(tmpDir, "repo")
	repoAgentsDir := filepath.Join(repoPath, ".multiclaude", "agents")
	if err := os.MkdirAll(repoAgentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a checked-in agent definition
	customContent := `# Custom Bot

A team-specific automation bot.
`
	if err := os.WriteFile(filepath.Join(repoAgentsDir, "custom-bot.md"), []byte(customContent), 0644); err != nil {
		t.Fatal(err)
	}

	reader := NewReader("", repoPath)
	defs, err := reader.ReadRepoDefinitions()
	if err != nil {
		t.Fatalf("ReadRepoDefinitions failed: %v", err)
	}

	if len(defs) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(defs))
	}

	if defs[0].Name != "custom-bot" {
		t.Errorf("expected name custom-bot, got %s", defs[0].Name)
	}
	if defs[0].Source != SourceRepo {
		t.Errorf("expected source repo, got %s", defs[0].Source)
	}
}

func TestReadRepoDefinitionsNonExistent(t *testing.T) {
	// When the repo agents directory doesn't exist, should return empty slice, not error
	tmpDir, err := os.MkdirTemp("", "agents-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	reader := NewReader("", tmpDir)
	defs, err := reader.ReadRepoDefinitions()
	if err != nil {
		t.Fatalf("ReadRepoDefinitions should not fail for non-existent directory: %v", err)
	}

	if len(defs) != 0 {
		t.Fatalf("expected 0 definitions, got %d", len(defs))
	}
}

func TestMergeDefinitions(t *testing.T) {
	local := []Definition{
		{Name: "worker", Content: "local worker", Source: SourceLocal},
		{Name: "reviewer", Content: "local reviewer", Source: SourceLocal},
		{Name: "local-only", Content: "only in local", Source: SourceLocal},
	}

	repo := []Definition{
		{Name: "worker", Content: "repo worker", Source: SourceRepo},
		{Name: "repo-only", Content: "only in repo", Source: SourceRepo},
	}

	merged := MergeDefinitions(local, repo)

	if len(merged) != 4 {
		t.Fatalf("expected 4 definitions, got %d", len(merged))
	}

	// Convert to map for easier checking
	defMap := make(map[string]Definition)
	for _, def := range merged {
		defMap[def.Name] = def
	}

	// Check that repo definition wins for worker
	worker, ok := defMap["worker"]
	if !ok {
		t.Fatal("worker not found in merged")
	}
	if worker.Content != "repo worker" {
		t.Errorf("expected repo worker content, got %s", worker.Content)
	}
	if worker.Source != SourceRepo {
		t.Errorf("expected source repo, got %s", worker.Source)
	}

	// Check that local-only definition is preserved
	localOnly, ok := defMap["local-only"]
	if !ok {
		t.Fatal("local-only not found in merged")
	}
	if localOnly.Source != SourceLocal {
		t.Errorf("expected source local, got %s", localOnly.Source)
	}

	// Check that repo-only definition is included
	repoOnly, ok := defMap["repo-only"]
	if !ok {
		t.Fatal("repo-only not found in merged")
	}
	if repoOnly.Source != SourceRepo {
		t.Errorf("expected source repo, got %s", repoOnly.Source)
	}

	// Check that reviewer is preserved from local
	reviewer, ok := defMap["reviewer"]
	if !ok {
		t.Fatal("reviewer not found in merged")
	}
	if reviewer.Source != SourceLocal {
		t.Errorf("expected source local, got %s", reviewer.Source)
	}
}

func TestReadAllDefinitions(t *testing.T) {
	// Create temp directory structure
	tmpDir, err := os.MkdirTemp("", "agents-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	localAgentsDir := filepath.Join(tmpDir, "local", "agents")
	if err := os.MkdirAll(localAgentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	repoPath := filepath.Join(tmpDir, "repo")
	repoAgentsDir := filepath.Join(repoPath, ".multiclaude", "agents")
	if err := os.MkdirAll(repoAgentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Local worker
	if err := os.WriteFile(filepath.Join(localAgentsDir, "worker.md"), []byte("local worker"), 0644); err != nil {
		t.Fatal(err)
	}

	// Local reviewer
	if err := os.WriteFile(filepath.Join(localAgentsDir, "reviewer.md"), []byte("local reviewer"), 0644); err != nil {
		t.Fatal(err)
	}

	// Repo worker (should win)
	if err := os.WriteFile(filepath.Join(repoAgentsDir, "worker.md"), []byte("repo worker"), 0644); err != nil {
		t.Fatal(err)
	}

	// Repo custom-bot (unique)
	if err := os.WriteFile(filepath.Join(repoAgentsDir, "custom-bot.md"), []byte("repo custom"), 0644); err != nil {
		t.Fatal(err)
	}

	reader := NewReader(localAgentsDir, repoPath)
	defs, err := reader.ReadAllDefinitions()
	if err != nil {
		t.Fatalf("ReadAllDefinitions failed: %v", err)
	}

	if len(defs) != 3 {
		t.Fatalf("expected 3 definitions, got %d", len(defs))
	}

	// Verify sorted order
	expectedOrder := []string{"custom-bot", "reviewer", "worker"}
	for i, def := range defs {
		if def.Name != expectedOrder[i] {
			t.Errorf("expected %s at position %d, got %s", expectedOrder[i], i, def.Name)
		}
	}

	// Verify worker is from repo
	for _, def := range defs {
		if def.Name == "worker" && def.Source != SourceRepo {
			t.Errorf("expected worker to be from repo, got %s", def.Source)
		}
	}
}

func TestParseTitle(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "with title",
			content:  "# Worker Agent\n\nSome description",
			expected: "Worker Agent",
		},
		{
			name:     "with leading whitespace",
			content:  "  \n# Worker Agent\n\nSome description",
			expected: "Worker Agent",
		},
		{
			name:     "no title",
			content:  "Some content without title",
			expected: "fallback",
		},
		{
			name:     "h2 only",
			content:  "## Section\n\nContent",
			expected: "fallback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := Definition{Name: "fallback", Content: tt.content}
			title := def.ParseTitle()
			if title != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, title)
			}
		})
	}
}

func TestParseDescription(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "simple description",
			content:  "# Worker Agent\n\nA task-based worker.\n\n## Section",
			expected: "A task-based worker.",
		},
		{
			name:     "multi-line description",
			content:  "# Worker Agent\n\nFirst line of description.\nSecond line.\n\n## Section",
			expected: "First line of description. Second line.",
		},
		{
			name:     "no description",
			content:  "# Worker Agent\n\n## Section",
			expected: "",
		},
		{
			name:     "description with no following section",
			content:  "# Worker Agent\n\nJust a description.",
			expected: "Just a description.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := Definition{Content: tt.content}
			desc := def.ParseDescription()
			if desc != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, desc)
			}
		})
	}
}

func TestEmptyLocalDir(t *testing.T) {
	reader := NewReader("", "")
	defs, err := reader.ReadLocalDefinitions()
	if err != nil {
		t.Fatalf("ReadLocalDefinitions should not fail for empty path: %v", err)
	}
	if len(defs) != 0 {
		t.Fatalf("expected 0 definitions, got %d", len(defs))
	}
}

func TestEmptyRepoPath(t *testing.T) {
	reader := NewReader("", "")
	defs, err := reader.ReadRepoDefinitions()
	if err != nil {
		t.Fatalf("ReadRepoDefinitions should not fail for empty path: %v", err)
	}
	if len(defs) != 0 {
		t.Fatalf("expected 0 definitions, got %d", len(defs))
	}
}
