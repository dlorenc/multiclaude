package bugreport

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dlorenc/multiclaude/internal/state"
	"github.com/dlorenc/multiclaude/pkg/config"
)

func TestCollector_Collect(t *testing.T) {
	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "bugreport-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Set up paths
	paths := &config.Paths{
		Root:            tmpDir,
		DaemonPID:       filepath.Join(tmpDir, "daemon.pid"),
		DaemonSock:      filepath.Join(tmpDir, "daemon.sock"),
		DaemonLog:       filepath.Join(tmpDir, "daemon.log"),
		StateFile:       filepath.Join(tmpDir, "state.json"),
		ReposDir:        filepath.Join(tmpDir, "repos"),
		WorktreesDir:    filepath.Join(tmpDir, "wts"),
		MessagesDir:     filepath.Join(tmpDir, "messages"),
		OutputDir:       filepath.Join(tmpDir, "output"),
		ClaudeConfigDir: filepath.Join(tmpDir, "claude-config"),
		ArchiveDir:      filepath.Join(tmpDir, "archive"),
	}

	// Create a test state file
	testState := struct {
		Repos map[string]*state.Repository `json:"repos"`
	}{
		Repos: map[string]*state.Repository{
			"test-repo": {
				GithubURL:   "https://github.com/test-owner/test-repo",
				TmuxSession: "test-session",
				Agents: map[string]state.Agent{
					"supervisor":  {Type: state.AgentTypeSupervisor},
					"worker-1":    {Type: state.AgentTypeWorker},
					"worker-2":    {Type: state.AgentTypeWorker},
					"merge-queue": {Type: state.AgentTypeMergeQueue},
				},
			},
		},
	}
	stateData, _ := json.Marshal(testState)
	os.WriteFile(paths.StateFile, stateData, 0644)

	// Create a test daemon log
	logContent := `2024-01-01 10:00:00 Starting daemon
2024-01-01 10:00:01 Repository test-repo initialized
2024-01-01 10:00:02 Worker jolly-tiger started`
	os.WriteFile(paths.DaemonLog, []byte(logContent), 0644)

	// Create collector and collect report
	collector := NewCollector(paths, "1.0.0-test")
	report, err := collector.Collect("Test bug description", false)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	// Verify basic fields
	if report.Description != "Test bug description" {
		t.Errorf("expected description 'Test bug description', got %q", report.Description)
	}
	if report.Version != "1.0.0-test" {
		t.Errorf("expected version '1.0.0-test', got %q", report.Version)
	}
	if report.GoVersion == "" {
		t.Error("expected non-empty GoVersion")
	}
	if report.OS == "" {
		t.Error("expected non-empty OS")
	}
	if report.Arch == "" {
		t.Error("expected non-empty Arch")
	}

	// Verify agent counts
	if report.RepoCount != 1 {
		t.Errorf("expected RepoCount 1, got %d", report.RepoCount)
	}
	if report.WorkerCount != 2 {
		t.Errorf("expected WorkerCount 2, got %d", report.WorkerCount)
	}
	if report.SupervisorCount != 1 {
		t.Errorf("expected SupervisorCount 1, got %d", report.SupervisorCount)
	}
	if report.MergeQueueCount != 1 {
		t.Errorf("expected MergeQueueCount 1, got %d", report.MergeQueueCount)
	}

	// Verify daemon log was collected
	if report.DaemonLogTail == "" || report.DaemonLogTail == "(no log file found)" {
		t.Error("expected daemon log tail to be collected")
	}
}

func TestCollector_CollectVerbose(t *testing.T) {
	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "bugreport-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Set up paths
	paths := &config.Paths{
		Root:            tmpDir,
		DaemonPID:       filepath.Join(tmpDir, "daemon.pid"),
		DaemonSock:      filepath.Join(tmpDir, "daemon.sock"),
		DaemonLog:       filepath.Join(tmpDir, "daemon.log"),
		StateFile:       filepath.Join(tmpDir, "state.json"),
		ReposDir:        filepath.Join(tmpDir, "repos"),
		WorktreesDir:    filepath.Join(tmpDir, "wts"),
		MessagesDir:     filepath.Join(tmpDir, "messages"),
		OutputDir:       filepath.Join(tmpDir, "output"),
		ClaudeConfigDir: filepath.Join(tmpDir, "claude-config"),
		ArchiveDir:      filepath.Join(tmpDir, "archive"),
	}

	// Create a test state file with multiple repos
	testState := struct {
		Repos map[string]*state.Repository `json:"repos"`
	}{
		Repos: map[string]*state.Repository{
			"repo-alpha": {
				GithubURL:   "https://github.com/owner/alpha",
				TmuxSession: "alpha-session",
				Agents: map[string]state.Agent{
					"supervisor": {Type: state.AgentTypeSupervisor},
					"worker-1":   {Type: state.AgentTypeWorker},
				},
			},
			"repo-beta": {
				GithubURL:   "https://github.com/owner/beta",
				TmuxSession: "beta-session",
				Agents: map[string]state.Agent{
					"worker-1": {Type: state.AgentTypeWorker},
					"worker-2": {Type: state.AgentTypeWorker},
					"worker-3": {Type: state.AgentTypeWorker},
				},
			},
		},
	}
	stateData, _ := json.Marshal(testState)
	os.WriteFile(paths.StateFile, stateData, 0644)

	// Create collector and collect verbose report
	collector := NewCollector(paths, "1.0.0-test")
	report, err := collector.Collect("", true)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	// Verify verbose mode includes repo stats
	if !report.Verbose {
		t.Error("expected Verbose to be true")
	}
	if len(report.RepoStats) != 2 {
		t.Errorf("expected 2 repo stats, got %d", len(report.RepoStats))
	}

	// Verify repo names are redacted
	for _, stat := range report.RepoStats {
		if !strings.HasPrefix(stat.Name, "repo-") {
			t.Errorf("expected redacted repo name starting with 'repo-', got %q", stat.Name)
		}
		if strings.Contains(stat.Name, "alpha") || strings.Contains(stat.Name, "beta") {
			t.Errorf("repo name should be redacted, got %q", stat.Name)
		}
	}
}

func TestFormatMarkdown(t *testing.T) {
	report := &Report{
		Description:      "Test bug",
		Version:          "1.0.0",
		GoVersion:        "go1.21.0",
		OS:               "darwin",
		Arch:             "arm64",
		TmuxVersion:      "tmux 3.3a",
		GitVersion:       "git version 2.40.0",
		ClaudeExists:     true,
		DaemonRunning:    true,
		DaemonPID:        12345,
		RepoCount:        2,
		WorkerCount:      5,
		SupervisorCount:  2,
		MergeQueueCount:  1,
		WorkspaceCount:   1,
		ReviewAgentCount: 0,
		DaemonLogTail:    "test log content\n",
	}

	markdown := FormatMarkdown(report)

	// Verify sections exist
	if !strings.Contains(markdown, "# Multiclaude Bug Report") {
		t.Error("missing title")
	}
	if !strings.Contains(markdown, "## Description") {
		t.Error("missing description section")
	}
	if !strings.Contains(markdown, "Test bug") {
		t.Error("missing description content")
	}
	if !strings.Contains(markdown, "## Environment") {
		t.Error("missing environment section")
	}
	if !strings.Contains(markdown, "## Tool Versions") {
		t.Error("missing tool versions section")
	}
	if !strings.Contains(markdown, "## Daemon Status") {
		t.Error("missing daemon status section")
	}
	if !strings.Contains(markdown, "Running (PID: 12345)") {
		t.Error("missing daemon PID")
	}
	if !strings.Contains(markdown, "## Statistics") {
		t.Error("missing statistics section")
	}
	if !strings.Contains(markdown, "## Daemon Log") {
		t.Error("missing daemon log section")
	}
}

func TestFormatMarkdown_Verbose(t *testing.T) {
	report := &Report{
		Verbose:         true,
		Version:         "1.0.0",
		GoVersion:       "go1.21.0",
		OS:              "linux",
		Arch:            "amd64",
		TmuxVersion:     "tmux 3.2",
		GitVersion:      "git version 2.39.0",
		ClaudeExists:    true,
		DaemonRunning:   true,
		DaemonPID:       1234,
		RepoCount:       2,
		WorkerCount:     3,
		SupervisorCount: 2,
		RepoStats: []RepoStat{
			{Name: "repo-1", WorkerCount: 2, HasSupervisor: true, HasMergeQueue: true},
			{Name: "repo-2", WorkerCount: 1, HasSupervisor: true, HasMergeQueue: false},
		},
		DaemonLogTail: "log content",
	}

	markdown := FormatMarkdown(report)

	// Verify verbose section exists
	if !strings.Contains(markdown, "### Per-Repository Breakdown") {
		t.Error("missing per-repository breakdown section")
	}
	if !strings.Contains(markdown, "repo-1") {
		t.Error("missing repo-1 in breakdown")
	}
	if !strings.Contains(markdown, "repo-2") {
		t.Error("missing repo-2 in breakdown")
	}
}

func TestFormatMarkdown_NoDescription(t *testing.T) {
	report := &Report{
		Description:   "",
		Version:       "1.0.0",
		GoVersion:     "go1.21.0",
		OS:            "darwin",
		Arch:          "arm64",
		DaemonLogTail: "log",
	}

	markdown := FormatMarkdown(report)

	// Should not have description section when empty
	if strings.Contains(markdown, "## Description") {
		t.Error("should not have description section when description is empty")
	}
}

func TestFormatMarkdown_DaemonNotRunning(t *testing.T) {
	report := &Report{
		Version:       "1.0.0",
		GoVersion:     "go1.21.0",
		OS:            "darwin",
		Arch:          "arm64",
		DaemonRunning: false,
		DaemonPID:     0,
		DaemonLogTail: "log",
	}

	markdown := FormatMarkdown(report)

	if !strings.Contains(markdown, "Not running") {
		t.Error("should show 'Not running' when daemon is not running")
	}
}

func TestFormatMarkdown_StalePID(t *testing.T) {
	report := &Report{
		Version:       "1.0.0",
		GoVersion:     "go1.21.0",
		OS:            "darwin",
		Arch:          "arm64",
		DaemonRunning: false,
		DaemonPID:     9999,
		DaemonLogTail: "log",
	}

	markdown := FormatMarkdown(report)

	if !strings.Contains(markdown, "stale PID: 9999") {
		t.Error("should show stale PID when daemon is not running but PID exists")
	}
}
