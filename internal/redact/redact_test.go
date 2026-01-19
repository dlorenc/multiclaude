package redact

import (
	"strings"
	"testing"
)

func TestRedactor_RepoName(t *testing.T) {
	r := New()

	// First repo should get repo-1
	name1 := r.RepoName("my-private-repo")
	if name1 != "repo-1" {
		t.Errorf("expected repo-1, got %s", name1)
	}

	// Same repo should get same redacted name
	name1Again := r.RepoName("my-private-repo")
	if name1Again != "repo-1" {
		t.Errorf("expected repo-1 again, got %s", name1Again)
	}

	// Second repo should get repo-2
	name2 := r.RepoName("another-secret-repo")
	if name2 != "repo-2" {
		t.Errorf("expected repo-2, got %s", name2)
	}
}

func TestRedactor_AgentName(t *testing.T) {
	r := New()

	// First worker should get worker-1
	worker1 := r.AgentName("jolly-tiger", "worker")
	if worker1 != "worker-1" {
		t.Errorf("expected worker-1, got %s", worker1)
	}

	// Same worker should get same name
	worker1Again := r.AgentName("jolly-tiger", "worker")
	if worker1Again != "worker-1" {
		t.Errorf("expected worker-1 again, got %s", worker1Again)
	}

	// Second worker should get worker-2
	worker2 := r.AgentName("happy-panda", "worker")
	if worker2 != "worker-2" {
		t.Errorf("expected worker-2, got %s", worker2)
	}

	// Supervisor should get supervisor-1 (separate counter)
	supervisor1 := r.AgentName("main-supervisor", "supervisor")
	if supervisor1 != "supervisor-1" {
		t.Errorf("expected supervisor-1, got %s", supervisor1)
	}
}

func TestRedactor_GitHubURL(t *testing.T) {
	r := New()

	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "https://github.com/owner/repo",
			expected: "https://github.com/<owner>/<repo>",
		},
		{
			input:    "https://github.com/my-org/my-private-repo.git",
			expected: "https://github.com/<owner>/<repo>",
		},
		{
			input:    "git@github.com:owner/repo",
			expected: "git@github.com:<owner>/<repo>",
		},
		{
			input:    "git@github.com:my-org/my-private-repo.git",
			expected: "git@github.com:<owner>/<repo>",
		},
		{
			input:    "no url here",
			expected: "no url here",
		},
	}

	for _, tc := range tests {
		result := r.GitHubURL(tc.input)
		if result != tc.expected {
			t.Errorf("GitHubURL(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestRedactor_Path(t *testing.T) {
	r := New()
	// Register a repo first so it gets redacted in paths
	r.RepoName("my-repo")

	// Test home directory redaction (this test depends on having a home dir)
	if r.homeDir != "" {
		path := r.homeDir + "/.multiclaude/repos/my-repo"
		result := r.Path(path)
		if !strings.HasPrefix(result, "/Users/<user>") {
			t.Errorf("expected path to start with /Users/<user>, got %s", result)
		}
		if !strings.Contains(result, "repo-1") {
			t.Errorf("expected path to contain repo-1 (redacted repo name), got %s", result)
		}
	}
}

func TestRedactor_Text(t *testing.T) {
	r := New()
	// Register repos for consistent redaction
	r.RepoName("private-project")

	text := `Error in repository private-project:
Clone URL: https://github.com/secret-org/private-project
SSH URL: git@github.com:secret-org/private-project.git`

	result := r.Text(text)

	// Should redact GitHub URLs
	if strings.Contains(result, "secret-org") {
		t.Errorf("text still contains 'secret-org': %s", result)
	}
	if !strings.Contains(result, "<owner>") {
		t.Errorf("expected <owner> placeholder in result: %s", result)
	}

	// Should redact repo names
	if strings.Contains(result, "private-project") {
		t.Errorf("text still contains 'private-project': %s", result)
	}
	if !strings.Contains(result, "repo-1") {
		t.Errorf("expected repo-1 in result: %s", result)
	}
}

func TestRedactor_ConsistentMapping(t *testing.T) {
	r := New()

	// Add multiple repos and agents
	r.RepoName("alpha")
	r.RepoName("beta")
	r.AgentName("agent-a", "worker")
	r.AgentName("agent-b", "worker")

	// Verify consistent mappings
	if r.RepoName("alpha") != "repo-1" {
		t.Error("inconsistent repo mapping for alpha")
	}
	if r.RepoName("beta") != "repo-2" {
		t.Error("inconsistent repo mapping for beta")
	}
	if r.AgentName("agent-a", "worker") != "worker-1" {
		t.Error("inconsistent agent mapping for agent-a")
	}
	if r.AgentName("agent-b", "worker") != "worker-2" {
		t.Error("inconsistent agent mapping for agent-b")
	}
}

func TestItoa(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{10, "10"},
		{123, "123"},
		{9999, "9999"},
	}

	for _, tc := range tests {
		result := itoa(tc.input)
		if result != tc.expected {
			t.Errorf("itoa(%d) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}
