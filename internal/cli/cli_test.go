package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dlorenc/multiclaude/internal/daemon"
	"github.com/dlorenc/multiclaude/internal/messages"
	"github.com/dlorenc/multiclaude/internal/socket"
	"github.com/dlorenc/multiclaude/internal/state"
	"github.com/dlorenc/multiclaude/internal/tmux"
	"github.com/dlorenc/multiclaude/pkg/config"
)

func TestParseFlags(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantFlags    map[string]string
		wantPositional []string
	}{
		{
			name:         "empty args",
			args:         []string{},
			wantFlags:    map[string]string{},
			wantPositional: nil,
		},
		{
			name:         "positional only",
			args:         []string{"arg1", "arg2", "arg3"},
			wantFlags:    map[string]string{},
			wantPositional: []string{"arg1", "arg2", "arg3"},
		},
		{
			name:         "long flag with value",
			args:         []string{"--repo", "myrepo"},
			wantFlags:    map[string]string{"repo": "myrepo"},
			wantPositional: nil,
		},
		{
			name:         "long flag boolean",
			args:         []string{"--verbose"},
			wantFlags:    map[string]string{"verbose": "true"},
			wantPositional: nil,
		},
		{
			name:         "short flag with value",
			args:         []string{"-r", "myrepo"},
			wantFlags:    map[string]string{"r": "myrepo"},
			wantPositional: nil,
		},
		{
			name:         "short flag boolean",
			args:         []string{"-v"},
			wantFlags:    map[string]string{"v": "true"},
			wantPositional: nil,
		},
		{
			name:         "mixed flags and positional",
			args:         []string{"--repo", "myrepo", "task", "description", "-v"},
			wantFlags:    map[string]string{"repo": "myrepo", "v": "true"},
			wantPositional: []string{"task", "description"},
		},
		{
			name:         "multiple long flags",
			args:         []string{"--name", "worker1", "--branch", "main", "--dry-run"},
			wantFlags:    map[string]string{"name": "worker1", "branch": "main", "dry-run": "true"},
			wantPositional: nil,
		},
		{
			name:         "flag followed by flag (boolean)",
			args:         []string{"--verbose", "--debug"},
			wantFlags:    map[string]string{"verbose": "true", "debug": "true"},
			wantPositional: nil,
		},
		{
			name:         "positional before flags",
			args:         []string{"command", "--flag", "value"},
			wantFlags:    map[string]string{"flag": "value"},
			wantPositional: []string{"command"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFlags, gotPositional := ParseFlags(tt.args)

			// Check flags
			if len(gotFlags) != len(tt.wantFlags) {
				t.Errorf("ParseFlags() flags len = %d, want %d", len(gotFlags), len(tt.wantFlags))
			}
			for k, v := range tt.wantFlags {
				if gotFlags[k] != v {
					t.Errorf("ParseFlags() flags[%q] = %q, want %q", k, gotFlags[k], v)
				}
			}

			// Check positional
			if len(gotPositional) != len(tt.wantPositional) {
				t.Errorf("ParseFlags() positional len = %d, want %d", len(gotPositional), len(tt.wantPositional))
			}
			for i, v := range tt.wantPositional {
				if i < len(gotPositional) && gotPositional[i] != v {
					t.Errorf("ParseFlags() positional[%d] = %q, want %q", i, gotPositional[i], v)
				}
			}
		})
	}
}

func TestFormatTime(t *testing.T) {
	tests := []struct {
		name     string
		time     time.Time
		wantType string // "time" for HH:MM:SS format, "date" for date format
	}{
		{
			name:     "recent time (today)",
			time:     time.Now().Add(-1 * time.Hour),
			wantType: "time",
		},
		{
			name:     "old time (yesterday)",
			time:     time.Now().Add(-25 * time.Hour),
			wantType: "date",
		},
		{
			name:     "old time (last week)",
			time:     time.Now().Add(-7 * 24 * time.Hour),
			wantType: "date",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatTime(tt.time)

			if tt.wantType == "time" {
				// Should contain colons (HH:MM:SS format)
				if !strings.Contains(got, ":") {
					t.Errorf("formatTime() = %q, expected time format with colons", got)
				}
				// Should not contain month abbreviation
				months := []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
				for _, m := range months {
					if strings.Contains(got, m) {
						t.Errorf("formatTime() = %q, expected time-only format without month", got)
					}
				}
			} else {
				// Should contain month abbreviation
				hasMonth := false
				months := []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
				for _, m := range months {
					if strings.Contains(got, m) {
						hasMonth = true
						break
					}
				}
				if !hasMonth {
					t.Errorf("formatTime() = %q, expected date format with month", got)
				}
			}
		})
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		maxLen int
		want   string
	}{
		{
			name:   "short string",
			s:      "hello",
			maxLen: 10,
			want:   "hello",
		},
		{
			name:   "exact length",
			s:      "hello",
			maxLen: 5,
			want:   "hello",
		},
		{
			name:   "long string",
			s:      "hello world this is a long string",
			maxLen: 15,
			want:   "hello world ...",
		},
		{
			name:   "empty string",
			s:      "",
			maxLen: 10,
			want:   "",
		},
		{
			name:   "truncate to minimum",
			s:      "abcdefgh",
			maxLen: 4,
			want:   "a...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateString(tt.s, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateString() = %q, want %q", got, tt.want)
			}
			if len(got) > tt.maxLen {
				t.Errorf("truncateString() len = %d, exceeds maxLen %d", len(got), tt.maxLen)
			}
		})
	}
}

func TestGenerateSessionID(t *testing.T) {
	// Generate multiple session IDs and verify uniqueness
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id, err := generateSessionID()
		if err != nil {
			t.Fatalf("generateSessionID() error = %v", err)
		}

		// Check format: UUID v4 format xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
		parts := strings.Split(id, "-")
		if len(parts) != 5 {
			t.Errorf("generateSessionID() = %q, expected 5 parts separated by dashes", id)
		}

		// Check part lengths: 8-4-4-4-12
		expectedLens := []int{8, 4, 4, 4, 12}
		for j, part := range parts {
			if len(part) != expectedLens[j] {
				t.Errorf("generateSessionID() part %d len = %d, want %d", j, len(part), expectedLens[j])
			}
		}

		// Check uniqueness
		if ids[id] {
			t.Errorf("generateSessionID() generated duplicate ID: %q", id)
		}
		ids[id] = true
	}
}

func TestGenerateDocumentation(t *testing.T) {
	// Create a minimal CLI with commands registered
	cli := &CLI{
		paths: nil, // Not needed for doc generation
		rootCmd: &Command{
			Name:        "test",
			Description: "test cli",
			Subcommands: make(map[string]*Command),
		},
	}

	// Add some test commands
	cli.rootCmd.Subcommands["start"] = &Command{
		Name:        "start",
		Description: "Start the daemon",
		Usage:       "test start",
	}
	cli.rootCmd.Subcommands["stop"] = &Command{
		Name:        "stop",
		Description: "Stop the daemon",
	}
	cli.rootCmd.Subcommands["work"] = &Command{
		Name:        "work",
		Description: "Worker commands",
		Subcommands: map[string]*Command{
			"list": {
				Name:        "list",
				Description: "List workers",
			},
			"rm": {
				Name:        "rm",
				Description: "Remove a worker",
				Usage:       "test work rm <name>",
			},
		},
	}

	docs := cli.GenerateDocumentation()

	// Verify documentation contains expected content
	if !strings.Contains(docs, "# Multiclaude CLI Reference") {
		t.Error("GenerateDocumentation() missing header")
	}
	if !strings.Contains(docs, "## start") {
		t.Error("GenerateDocumentation() missing start command")
	}
	if !strings.Contains(docs, "Start the daemon") {
		t.Error("GenerateDocumentation() missing start description")
	}
	if !strings.Contains(docs, "## work") {
		t.Error("GenerateDocumentation() missing work command")
	}
	if !strings.Contains(docs, "**Subcommands:**") {
		t.Error("GenerateDocumentation() missing subcommands section")
	}
	if !strings.Contains(docs, "**Usage:**") {
		t.Error("GenerateDocumentation() missing usage section")
	}
}

// setupTestEnvironment creates a test environment with daemon and paths
func setupTestEnvironment(t *testing.T) (*CLI, *daemon.Daemon, func()) {
	t.Helper()

	// Set test mode to skip Claude startup
	os.Setenv("MULTICLAUDE_TEST_MODE", "1")

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "cli-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Create paths
	paths := &config.Paths{
		Root:         tmpDir,
		DaemonPID:    filepath.Join(tmpDir, "daemon.pid"),
		DaemonSock:   filepath.Join(tmpDir, "daemon.sock"),
		DaemonLog:    filepath.Join(tmpDir, "daemon.log"),
		StateFile:    filepath.Join(tmpDir, "state.json"),
		ReposDir:     filepath.Join(tmpDir, "repos"),
		WorktreesDir: filepath.Join(tmpDir, "wts"),
		MessagesDir:  filepath.Join(tmpDir, "messages"),
		OutputDir:    filepath.Join(tmpDir, "output"),
	}

	if err := paths.EnsureDirectories(); err != nil {
		t.Fatalf("Failed to create directories: %v", err)
	}

	// Create prompts directory (it's under root)
	if err := os.MkdirAll(filepath.Join(tmpDir, "prompts"), 0755); err != nil {
		t.Fatalf("Failed to create prompts dir: %v", err)
	}

	// Create daemon
	d, err := daemon.New(paths)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	// Start daemon
	if err := d.Start(); err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}

	// Wait for daemon to be ready
	time.Sleep(100 * time.Millisecond)

	// Create CLI with test paths
	cli := NewWithPaths(paths, "")

	cleanup := func() {
		d.Stop()
		os.RemoveAll(tmpDir)
		os.Unsetenv("MULTICLAUDE_TEST_MODE")
	}

	return cli, d, cleanup
}

// setupTestRepo creates a test git repository
func setupTestRepo(t *testing.T, repoPath string) {
	t.Helper()

	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatalf("Failed to create repo dir: %v", err)
	}

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@example.com"},
		{"git", "config", "user.name", "Test User"},
		{"git", "commit", "--allow-empty", "-m", "Initial commit"},
	}

	for _, cmdArgs := range cmds {
		cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
		cmd.Dir = repoPath
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to run %v: %v", cmdArgs, err)
		}
	}
}

func TestCLIListReposEmpty(t *testing.T) {
	cli, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// List repos when empty - should not error
	err := cli.Execute([]string{"list"})
	if err != nil {
		t.Errorf("list repos failed: %v", err)
	}
}

func TestCLIDaemonStatus(t *testing.T) {
	cli, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Check daemon status
	err := cli.Execute([]string{"daemon", "status"})
	if err != nil {
		t.Errorf("daemon status failed: %v", err)
	}
}

func TestCLIWorkListEmpty(t *testing.T) {
	cli, d, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Add a repo first via daemon so we can list workers
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test-repo",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.GetState().AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// List workers - should work even when empty
	err := cli.Execute([]string{"work", "list", "--repo", "test-repo"})
	if err != nil {
		t.Errorf("work list failed: %v", err)
	}
}

func TestCLIWorkListWithWorkers(t *testing.T) {
	cli, d, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Add a repo and worker via daemon
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test-repo",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.GetState().AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	agent := state.Agent{
		Type:         state.AgentTypeWorker,
		WorktreePath: "/tmp/test",
		TmuxWindow:   "test-worker",
		Task:         "Test task description",
		CreatedAt:    time.Now(),
	}
	if err := d.GetState().AddAgent("test-repo", "test-worker", agent); err != nil {
		t.Fatalf("Failed to add agent: %v", err)
	}

	// List workers - should show the worker
	err := cli.Execute([]string{"work", "list", "--repo", "test-repo"})
	if err != nil {
		t.Errorf("work list with workers failed: %v", err)
	}
}

func TestCLIAgentMessaging(t *testing.T) {
	_, d, cleanup := setupTestEnvironment(t)
	defer cleanup()

	repoName := "test-repo"
	paths := d.GetPaths()

	// Add a repo and agents
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test-repo",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.GetState().AddRepo(repoName, repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	supervisor := state.Agent{
		Type:         state.AgentTypeSupervisor,
		WorktreePath: paths.RepoDir(repoName),
		TmuxWindow:   "supervisor",
		CreatedAt:    time.Now(),
	}
	if err := d.GetState().AddAgent(repoName, "supervisor", supervisor); err != nil {
		t.Fatalf("Failed to add supervisor: %v", err)
	}

	worker := state.Agent{
		Type:         state.AgentTypeWorker,
		WorktreePath: "/tmp/worker",
		TmuxWindow:   "test-worker",
		Task:         "Test task",
		CreatedAt:    time.Now(),
	}
	if err := d.GetState().AddAgent(repoName, "test-worker", worker); err != nil {
		t.Fatalf("Failed to add worker: %v", err)
	}

	// Test message sending via manager directly (CLI requires being in worktree)
	msgMgr := messages.NewManager(paths.MessagesDir)

	// Send message from supervisor to worker
	msg, err := msgMgr.Send(repoName, "supervisor", "test-worker", "Hello from supervisor")
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Verify message was created
	if msg.Status != messages.StatusPending {
		t.Errorf("Message status = %s, want pending", msg.Status)
	}
	if msg.From != "supervisor" {
		t.Errorf("Message from = %s, want supervisor", msg.From)
	}
	if msg.To != "test-worker" {
		t.Errorf("Message to = %s, want test-worker", msg.To)
	}

	// List messages for worker
	msgs, err := msgMgr.List(repoName, "test-worker")
	if err != nil {
		t.Fatalf("Failed to list messages: %v", err)
	}
	if len(msgs) != 1 {
		t.Errorf("Expected 1 message, got %d", len(msgs))
	}

	// Read message
	readMsg, err := msgMgr.Get(repoName, "test-worker", msg.ID)
	if err != nil {
		t.Fatalf("Failed to read message: %v", err)
	}
	if readMsg.Body != "Hello from supervisor" {
		t.Errorf("Message body = %s, want 'Hello from supervisor'", readMsg.Body)
	}

	// Update status to read
	if err := msgMgr.UpdateStatus(repoName, "test-worker", msg.ID, messages.StatusRead); err != nil {
		t.Fatalf("Failed to update status: %v", err)
	}

	// Ack message
	if err := msgMgr.Ack(repoName, "test-worker", msg.ID); err != nil {
		t.Fatalf("Failed to ack message: %v", err)
	}

	// Verify acked
	ackedMsg, _ := msgMgr.Get(repoName, "test-worker", msg.ID)
	if ackedMsg.Status != messages.StatusAcked {
		t.Errorf("Message status = %s, want acked", ackedMsg.Status)
	}
}

func TestCLISocketCommunication(t *testing.T) {
	_, d, cleanup := setupTestEnvironment(t)
	defer cleanup()

	paths := d.GetPaths()
	client := socket.NewClient(paths.DaemonSock)

	// Test ping
	resp, err := client.Send(socket.Request{Command: "ping"})
	if err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
	if !resp.Success {
		t.Error("Ping should succeed")
	}
	if resp.Data != "pong" {
		t.Errorf("Ping response = %v, want pong", resp.Data)
	}

	// Test status
	resp, err = client.Send(socket.Request{Command: "status"})
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if !resp.Success {
		t.Error("Status should succeed")
	}
	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Status data should be a map")
	}
	if running, ok := data["running"].(bool); !ok || !running {
		t.Error("Status should show running=true")
	}

	// Test add_repo
	resp, err = client.Send(socket.Request{
		Command: "add_repo",
		Args: map[string]interface{}{
			"name":         "test-repo",
			"github_url":   "https://github.com/test/repo",
			"tmux_session": "mc-test-repo",
		},
	})
	if err != nil {
		t.Fatalf("Add repo failed: %v", err)
	}
	if !resp.Success {
		t.Errorf("Add repo failed: %s", resp.Error)
	}

	// Test list_repos
	resp, err = client.Send(socket.Request{Command: "list_repos"})
	if err != nil {
		t.Fatalf("List repos failed: %v", err)
	}
	if !resp.Success {
		t.Error("List repos should succeed")
	}
	repos, ok := resp.Data.([]interface{})
	if !ok {
		t.Fatal("List repos data should be an array")
	}
	if len(repos) != 1 {
		t.Errorf("Expected 1 repo, got %d", len(repos))
	}

	// Test add_agent
	resp, err = client.Send(socket.Request{
		Command: "add_agent",
		Args: map[string]interface{}{
			"repo":          "test-repo",
			"agent":         "test-worker",
			"type":          "worker",
			"worktree_path": "/tmp/test",
			"tmux_window":   "test-window",
			"task":          "Test task",
		},
	})
	if err != nil {
		t.Fatalf("Add agent failed: %v", err)
	}
	if !resp.Success {
		t.Errorf("Add agent failed: %s", resp.Error)
	}

	// Test list_agents
	resp, err = client.Send(socket.Request{
		Command: "list_agents",
		Args: map[string]interface{}{
			"repo": "test-repo",
		},
	})
	if err != nil {
		t.Fatalf("List agents failed: %v", err)
	}
	if !resp.Success {
		t.Error("List agents should succeed")
	}
	agents, ok := resp.Data.([]interface{})
	if !ok {
		t.Fatal("List agents data should be an array")
	}
	if len(agents) != 1 {
		t.Errorf("Expected 1 agent, got %d", len(agents))
	}

	// Test complete_agent
	resp, err = client.Send(socket.Request{
		Command: "complete_agent",
		Args: map[string]interface{}{
			"repo":  "test-repo",
			"agent": "test-worker",
		},
	})
	if err != nil {
		t.Fatalf("Complete agent failed: %v", err)
	}
	if !resp.Success {
		t.Errorf("Complete agent failed: %s", resp.Error)
	}

	// Verify agent is marked for cleanup
	st := d.GetState()
	agent, exists := st.GetAgent("test-repo", "test-worker")
	if !exists {
		t.Fatal("Agent should exist")
	}
	if !agent.ReadyForCleanup {
		t.Error("Agent should be marked for cleanup")
	}

	// Test remove_agent
	resp, err = client.Send(socket.Request{
		Command: "remove_agent",
		Args: map[string]interface{}{
			"repo":  "test-repo",
			"agent": "test-worker",
		},
	})
	if err != nil {
		t.Fatalf("Remove agent failed: %v", err)
	}
	if !resp.Success {
		t.Errorf("Remove agent failed: %s", resp.Error)
	}

	// Verify agent is removed
	_, exists = st.GetAgent("test-repo", "test-worker")
	if exists {
		t.Error("Agent should be removed")
	}
}

func TestCLIWorkCreateWithRealTmux(t *testing.T) {
	tmuxClient := tmux.NewClient()
	if !tmuxClient.IsTmuxAvailable() {
		t.Skip("tmux not available, skipping test")
	}

	cli, d, cleanup := setupTestEnvironment(t)
	defer cleanup()

	paths := d.GetPaths()
	repoName := "test-repo"
	repoPath := paths.RepoDir(repoName)

	// Create a test git repo
	setupTestRepo(t, repoPath)

	// Create tmux session
	tmuxSession := "mc-test-repo"
	if err := tmuxClient.CreateSession(tmuxSession, true); err != nil {
		t.Fatalf("Failed to create tmux session: %v", err)
	}
	defer tmuxClient.KillSession(tmuxSession)

	// Add repo to daemon
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: tmuxSession,
		Agents:      make(map[string]state.Agent),
	}
	if err := d.GetState().AddRepo(repoName, repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Change to repo directory for worktree operations
	oldDir, _ := os.Getwd()
	if err := os.Chdir(repoPath); err != nil {
		t.Fatalf("Failed to change to repo dir: %v", err)
	}
	defer os.Chdir(oldDir)

	// Create worker using CLI (explicitly pass --repo since directory inference
	// doesn't work with test paths)
	err := cli.Execute([]string{"work", "Test task description", "--name", "test-worker", "--repo", repoName})
	if err != nil {
		t.Errorf("work create failed: %v", err)
	}

	// Verify worker was created in state
	agent, exists := d.GetState().GetAgent(repoName, "test-worker")
	if !exists {
		t.Error("Worker should exist in state")
	}
	if agent.Type != state.AgentTypeWorker {
		t.Errorf("Agent type = %s, want worker", agent.Type)
	}
	if agent.Task != "Test task description" {
		t.Errorf("Agent task = %s, want 'Test task description'", agent.Task)
	}

	// Verify tmux window was created
	hasWindow, err := tmuxClient.HasWindow(tmuxSession, "test-worker")
	if err != nil {
		t.Fatalf("Failed to check window: %v", err)
	}
	if !hasWindow {
		t.Error("Worker tmux window should exist")
	}

	// Verify worktree was created
	wtPath := paths.AgentWorktree(repoName, "test-worker")
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		t.Error("Worker worktree should exist")
	}

	// Test work list shows the worker
	err = cli.Execute([]string{"work", "list", "--repo", repoName})
	if err != nil {
		t.Errorf("work list failed: %v", err)
	}
}

func TestCLICleanupCommand(t *testing.T) {
	cli, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Test cleanup with dry-run (should not error even with no resources)
	err := cli.Execute([]string{"cleanup", "--dry-run"})
	if err != nil {
		t.Errorf("cleanup --dry-run failed: %v", err)
	}

	// Test cleanup with verbose
	err = cli.Execute([]string{"cleanup", "--verbose"})
	if err != nil {
		t.Errorf("cleanup --verbose failed: %v", err)
	}
}

func TestCLIRepairCommand(t *testing.T) {
	cli, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Test repair command
	err := cli.Execute([]string{"repair"})
	if err != nil {
		t.Errorf("repair failed: %v", err)
	}

	// Test repair with verbose
	err = cli.Execute([]string{"repair", "--verbose"})
	if err != nil {
		t.Errorf("repair --verbose failed: %v", err)
	}
}

func TestCLIDocsCommand(t *testing.T) {
	cli, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Test docs command
	err := cli.Execute([]string{"docs"})
	if err != nil {
		t.Errorf("docs failed: %v", err)
	}
}

func TestCLIHelpCommand(t *testing.T) {
	cli, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Test help
	err := cli.Execute([]string{"--help"})
	if err != nil {
		t.Errorf("--help failed: %v", err)
	}

	// Test subcommand help
	err = cli.Execute([]string{"work", "--help"})
	if err != nil {
		t.Errorf("work --help failed: %v", err)
	}

	err = cli.Execute([]string{"agent", "--help"})
	if err != nil {
		t.Errorf("agent --help failed: %v", err)
	}
}

func TestCLIUnknownCommand(t *testing.T) {
	cli, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Test unknown command should fail
	err := cli.Execute([]string{"nonexistent"})
	if err == nil {
		t.Error("unknown command should fail")
	}
}

func TestNewWithPaths(t *testing.T) {
	tmpDir := t.TempDir()
	paths := &config.Paths{
		Root:         tmpDir,
		DaemonPID:    filepath.Join(tmpDir, "daemon.pid"),
		DaemonSock:   filepath.Join(tmpDir, "daemon.sock"),
		DaemonLog:    filepath.Join(tmpDir, "daemon.log"),
		StateFile:    filepath.Join(tmpDir, "state.json"),
		ReposDir:     filepath.Join(tmpDir, "repos"),
		WorktreesDir: filepath.Join(tmpDir, "wts"),
		MessagesDir:  filepath.Join(tmpDir, "messages"),
		OutputDir:    filepath.Join(tmpDir, "output"),
	}

	// Test with empty claude path
	cli := NewWithPaths(paths, "")
	if cli == nil {
		t.Fatal("CLI should not be nil")
	}
	if cli.claudeBinaryPath != "claude" {
		t.Errorf("claudeBinaryPath = %s, want 'claude'", cli.claudeBinaryPath)
	}

	// Test with custom claude path
	cli = NewWithPaths(paths, "/custom/path/claude")
	if cli.claudeBinaryPath != "/custom/path/claude" {
		t.Errorf("claudeBinaryPath = %s, want '/custom/path/claude'", cli.claudeBinaryPath)
	}

	// Verify commands are registered
	if cli.rootCmd == nil {
		t.Fatal("rootCmd should not be nil")
	}
	if len(cli.rootCmd.Subcommands) == 0 {
		t.Error("CLI should have subcommands registered")
	}

	// Check specific commands exist
	expectedCommands := []string{"start", "daemon", "init", "list", "work", "agent", "attach", "cleanup", "repair", "docs"}
	for _, cmd := range expectedCommands {
		if _, exists := cli.rootCmd.Subcommands[cmd]; !exists {
			t.Errorf("Expected command %s to be registered", cmd)
		}
	}
}

func TestInferRepoFromCwd(t *testing.T) {
	// Create temp directories to simulate multiclaude structure
	tmpDir := t.TempDir()

	// Resolve symlinks (macOS /tmp -> /private/tmp)
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatalf("failed to resolve tmpDir symlinks: %v", err)
	}

	worktreesDir := filepath.Join(tmpDir, "wts")
	reposDir := filepath.Join(tmpDir, "repos")

	// Create test directory structure
	// Worktree: wts/myrepo/workspace
	// Worktree: wts/otherrepo/worker1
	// Repo: repos/myrepo
	testDirs := []string{
		filepath.Join(worktreesDir, "myrepo", "workspace"),
		filepath.Join(worktreesDir, "myrepo", "worker1"),
		filepath.Join(worktreesDir, "otherrepo", "agent1"),
		filepath.Join(reposDir, "myrepo"),
		filepath.Join(reposDir, "otherrepo"),
	}
	for _, d := range testDirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatalf("failed to create test dir %s: %v", d, err)
		}
	}

	cli := &CLI{
		paths: &config.Paths{
			Root:         tmpDir,
			WorktreesDir: worktreesDir,
			ReposDir:     reposDir,
		},
	}

	// Save original working directory
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get original working directory: %v", err)
	}
	defer os.Chdir(origWd)

	tests := []struct {
		name      string
		cwd       string
		wantRepo  string
		wantError bool
	}{
		{
			name:      "worktree workspace",
			cwd:       filepath.Join(worktreesDir, "myrepo", "workspace"),
			wantRepo:  "myrepo",
			wantError: false,
		},
		{
			name:      "worktree worker",
			cwd:       filepath.Join(worktreesDir, "myrepo", "worker1"),
			wantRepo:  "myrepo",
			wantError: false,
		},
		{
			name:      "worktree other repo",
			cwd:       filepath.Join(worktreesDir, "otherrepo", "agent1"),
			wantRepo:  "otherrepo",
			wantError: false,
		},
		{
			name:      "main repo dir",
			cwd:       filepath.Join(reposDir, "myrepo"),
			wantRepo:  "myrepo",
			wantError: false,
		},
		{
			name:      "outside multiclaude",
			cwd:       os.TempDir(),
			wantRepo:  "",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.Chdir(tt.cwd); err != nil {
				t.Fatalf("failed to change to test directory %s: %v", tt.cwd, err)
			}

			gotRepo, err := cli.inferRepoFromCwd()

			if tt.wantError {
				if err == nil {
					t.Errorf("inferRepoFromCwd() expected error, got repo=%q", gotRepo)
				}
			} else {
				if err != nil {
					t.Errorf("inferRepoFromCwd() unexpected error: %v", err)
				}
				if gotRepo != tt.wantRepo {
					t.Errorf("inferRepoFromCwd() = %q, want %q", gotRepo, tt.wantRepo)
				}
			}
		})
	}
}
