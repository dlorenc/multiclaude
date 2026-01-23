package cli

import (
	"context"
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
	"github.com/dlorenc/multiclaude/pkg/config"
	"github.com/dlorenc/multiclaude/pkg/tmux"
)

func TestParseFlags(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantFlags      map[string]string
		wantPositional []string
	}{
		{
			name:           "empty args",
			args:           []string{},
			wantFlags:      map[string]string{},
			wantPositional: nil,
		},
		{
			name:           "positional only",
			args:           []string{"arg1", "arg2", "arg3"},
			wantFlags:      map[string]string{},
			wantPositional: []string{"arg1", "arg2", "arg3"},
		},
		{
			name:           "long flag with value",
			args:           []string{"--repo", "myrepo"},
			wantFlags:      map[string]string{"repo": "myrepo"},
			wantPositional: nil,
		},
		{
			name:           "long flag boolean",
			args:           []string{"--verbose"},
			wantFlags:      map[string]string{"verbose": "true"},
			wantPositional: nil,
		},
		{
			name:           "short flag with value",
			args:           []string{"-r", "myrepo"},
			wantFlags:      map[string]string{"r": "myrepo"},
			wantPositional: nil,
		},
		{
			name:           "short flag boolean",
			args:           []string{"-v"},
			wantFlags:      map[string]string{"v": "true"},
			wantPositional: nil,
		},
		{
			name:           "mixed flags and positional",
			args:           []string{"--repo", "myrepo", "task", "description", "-v"},
			wantFlags:      map[string]string{"repo": "myrepo", "v": "true"},
			wantPositional: []string{"task", "description"},
		},
		{
			name:           "multiple long flags",
			args:           []string{"--name", "worker1", "--branch", "main", "--dry-run"},
			wantFlags:      map[string]string{"name": "worker1", "branch": "main", "dry-run": "true"},
			wantPositional: nil,
		},
		{
			name:           "flag followed by flag (boolean)",
			args:           []string{"--verbose", "--debug"},
			wantFlags:      map[string]string{"verbose": "true", "debug": "true"},
			wantPositional: nil,
		},
		{
			name:           "positional before flags",
			args:           []string{"command", "--flag", "value"},
			wantFlags:      map[string]string{"flag": "value"},
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

// TestGenerateSessionID is now in pkg/claude/claude_test.go

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

	// Resolve symlinks to handle macOS /var -> /private/var symlink
	// This ensures paths match when compared with os.Getwd() results
	tmpDir, err = filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatalf("Failed to resolve symlinks: %v", err)
	}

	// Create paths
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
	cli := NewWithPaths(paths)

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

func TestCLISendMessageTriggersImmediateRouting(t *testing.T) {
	cli, d, cleanup := setupTestEnvironment(t)
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
		WorktreePath: filepath.Join(paths.WorktreesDir, repoName, "test-worker"),
		TmuxWindow:   "test-worker",
		Task:         "Test task",
		CreatedAt:    time.Now(),
	}
	if err := d.GetState().AddAgent(repoName, "test-worker", worker); err != nil {
		t.Fatalf("Failed to add worker: %v", err)
	}

	// Create the worktree directory structure so inferAgentContext works
	worktreeDir := filepath.Join(paths.WorktreesDir, repoName, "test-worker")
	if err := os.MkdirAll(worktreeDir, 0755); err != nil {
		t.Fatalf("Failed to create worktree dir: %v", err)
	}

	// Save current directory and change to worktree
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	if err := os.Chdir(worktreeDir); err != nil {
		t.Fatalf("Failed to change to worktree: %v", err)
	}

	// Test that sendMessage can be called and triggers route_messages
	// The route_messages call is best-effort, so we verify:
	// 1. Message is created successfully
	// 2. Socket call doesn't cause errors (it's ignored if it fails)

	err := cli.sendMessage([]string{"supervisor", "Test message for immediate routing"})
	if err != nil {
		t.Fatalf("sendMessage failed: %v", err)
	}

	// Verify message was created
	msgMgr := messages.NewManager(paths.MessagesDir)
	msgs, err := msgMgr.List(repoName, "supervisor")
	if err != nil {
		t.Fatalf("Failed to list messages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(msgs))
	}
	if msgs[0].From != "test-worker" {
		t.Errorf("Message from = %s, want test-worker", msgs[0].From)
	}
	if msgs[0].Body != "Test message for immediate routing" {
		t.Errorf("Message body = %s, want 'Test message for immediate routing'", msgs[0].Body)
	}

	// Verify the route_messages socket command works (daemon should be running)
	client := socket.NewClient(paths.DaemonSock)
	resp, err := client.Send(socket.Request{Command: "route_messages"})
	if err != nil {
		t.Fatalf("Failed to send route_messages: %v", err)
	}
	if !resp.Success {
		t.Errorf("route_messages failed: %s", resp.Error)
	}
}

func TestCLISendMessageFallbackWhenDaemonUnavailable(t *testing.T) {
	// This test verifies that send-message works even when the daemon
	// socket is unavailable (the socket call is best-effort)

	// Create a temp directory for test paths
	tmpDir, err := os.MkdirTemp("", "cli-sendmessage-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Resolve symlinks to handle macOS /var -> /private/var symlink
	tmpDir, err = filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatalf("Failed to resolve symlinks: %v", err)
	}

	// Create paths pointing to non-existent socket
	paths := &config.Paths{
		Root:            tmpDir,
		DaemonPID:       filepath.Join(tmpDir, "daemon.pid"),
		DaemonSock:      filepath.Join(tmpDir, "nonexistent.sock"), // Socket doesn't exist
		DaemonLog:       filepath.Join(tmpDir, "daemon.log"),
		StateFile:       filepath.Join(tmpDir, "state.json"),
		ReposDir:        filepath.Join(tmpDir, "repos"),
		WorktreesDir:    filepath.Join(tmpDir, "wts"),
		MessagesDir:     filepath.Join(tmpDir, "messages"),
		OutputDir:       filepath.Join(tmpDir, "output"),
		ClaudeConfigDir: filepath.Join(tmpDir, "claude-config"),
	}

	if err := paths.EnsureDirectories(); err != nil {
		t.Fatalf("Failed to create directories: %v", err)
	}

	// Create state with repo and agent
	st := state.New(paths.StateFile)

	repoName := "fallback-repo"
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-fallback-repo",
		Agents:      make(map[string]state.Agent),
	}
	if err := st.AddRepo(repoName, repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Create worktree directory for agent context
	worktreeDir := filepath.Join(paths.WorktreesDir, repoName, "sender-agent")
	if err := os.MkdirAll(worktreeDir, 0755); err != nil {
		t.Fatalf("Failed to create worktree dir: %v", err)
	}

	// Create CLI
	cli := NewWithPaths(paths)

	// Change to worktree directory
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	if err := os.Chdir(worktreeDir); err != nil {
		t.Fatalf("Failed to change to worktree: %v", err)
	}

	// Send message - should succeed even though daemon is not running
	// The socket call will fail silently (best-effort)
	err = cli.sendMessage([]string{"supervisor", "Fallback test message"})
	if err != nil {
		t.Fatalf("sendMessage failed when daemon unavailable: %v", err)
	}

	// Verify message was created (fallback to polling works)
	msgMgr := messages.NewManager(paths.MessagesDir)
	msgs, err := msgMgr.List(repoName, "supervisor")
	if err != nil {
		t.Fatalf("Failed to list messages: %v", err)
	}
	if len(msgs) != 1 {
		t.Errorf("Expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Body != "Fallback test message" {
		t.Errorf("Message body = %s, want 'Fallback test message'", msgs[0].Body)
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
		t.Fatal("tmux is required for this test but not available")
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
	if err := tmuxClient.CreateSession(context.Background(), tmuxSession, true); err != nil {
		t.Fatalf("Failed to create tmux session: %v", err)
	}
	defer tmuxClient.KillSession(context.Background(), tmuxSession)

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
	hasWindow, err := tmuxClient.HasWindow(context.Background(), tmuxSession, "test-worker")
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
	}

	// Test CLI creation
	cli := NewWithPaths(paths)
	if cli == nil {
		t.Fatal("CLI should not be nil")
	}

	// Verify commands are registered
	if cli.rootCmd == nil {
		t.Fatal("rootCmd should not be nil")
	}
	if len(cli.rootCmd.Subcommands) == 0 {
		t.Error("CLI should have subcommands registered")
	}

	// Check specific commands exist
	expectedCommands := []string{"start", "daemon", "init", "list", "work", "agent", "agents", "attach", "cleanup", "repair", "docs"}
	for _, cmd := range expectedCommands {
		if _, exists := cli.rootCmd.Subcommands[cmd]; !exists {
			t.Errorf("Expected command %s to be registered", cmd)
		}
	}

	// Check agents subcommands
	agentsCmd, exists := cli.rootCmd.Subcommands["agents"]
	if !exists {
		t.Fatal("agents command should be registered")
	}
	if _, exists := agentsCmd.Subcommands["list"]; !exists {
		t.Error("agents list subcommand should be registered")
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

func TestHasPathPrefix(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		prefix string
		want   bool
	}{
		{
			name:   "exact match",
			path:   "/foo/bar",
			prefix: "/foo/bar",
			want:   true,
		},
		{
			name:   "child directory",
			path:   "/foo/bar/baz",
			prefix: "/foo/bar",
			want:   true,
		},
		{
			name:   "similar prefix not matching",
			path:   "/foo/bar-extra/baz",
			prefix: "/foo/bar",
			want:   false,
		},
		{
			name:   "wts-backup should not match wts",
			path:   "/home/user/.multiclaude/wts-backup/repo/agent",
			prefix: "/home/user/.multiclaude/wts",
			want:   false,
		},
		{
			name:   "wts should match wts",
			path:   "/home/user/.multiclaude/wts/repo/agent",
			prefix: "/home/user/.multiclaude/wts",
			want:   true,
		},
		{
			name:   "prefix with trailing slash",
			path:   "/foo/bar/baz",
			prefix: "/foo/bar/",
			want:   true,
		},
		{
			name:   "unrelated paths",
			path:   "/completely/different",
			prefix: "/foo/bar",
			want:   false,
		},
		{
			name:   "root prefix",
			path:   "/foo/bar",
			prefix: "/",
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasPathPrefix(tt.path, tt.prefix)
			if got != tt.want {
				t.Errorf("hasPathPrefix(%q, %q) = %v, want %v", tt.path, tt.prefix, got, tt.want)
			}
		})
	}
}

func TestInferRepoFromCwdWithSymlinks(t *testing.T) {
	// Create temp directories to simulate multiclaude structure
	tmpDir := t.TempDir()

	// Resolve symlinks (macOS /tmp -> /private/tmp)
	resolvedTmpDir, err := filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatalf("failed to resolve tmpDir symlinks: %v", err)
	}

	worktreesDir := filepath.Join(resolvedTmpDir, "wts")
	reposDir := filepath.Join(resolvedTmpDir, "repos")

	// Create test directory structure including a similar-named directory
	testDirs := []string{
		filepath.Join(worktreesDir, "myrepo", "workspace"),
		filepath.Join(resolvedTmpDir, "wts-backup", "myrepo", "workspace"), // Similar name, should NOT match
	}
	for _, d := range testDirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatalf("failed to create test dir %s: %v", d, err)
		}
	}

	cli := &CLI{
		paths: &config.Paths{
			Root:         resolvedTmpDir,
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
			name:      "worktree via resolved path",
			cwd:       filepath.Join(worktreesDir, "myrepo", "workspace"),
			wantRepo:  "myrepo",
			wantError: false,
		},
		{
			name:      "wts-backup should not match wts",
			cwd:       filepath.Join(resolvedTmpDir, "wts-backup", "myrepo", "workspace"),
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

func TestInferAgentContext(t *testing.T) {
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
	testDirs := []string{
		filepath.Join(worktreesDir, "myrepo", "worker1"),
		filepath.Join(worktreesDir, "myrepo", "workspace"),
		filepath.Join(worktreesDir, "myrepo"), // Just repo level
		filepath.Join(reposDir, "myrepo"),
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

	// Test worktree cases - these should work reliably
	tests := []struct {
		name      string
		cwd       string
		wantRepo  string
		wantAgent string
		wantError bool
	}{
		{
			name:      "in worker worktree",
			cwd:       filepath.Join(worktreesDir, "myrepo", "worker1"),
			wantRepo:  "myrepo",
			wantAgent: "worker1",
			wantError: false,
		},
		{
			name:      "in workspace worktree",
			cwd:       filepath.Join(worktreesDir, "myrepo", "workspace"),
			wantRepo:  "myrepo",
			wantAgent: "workspace",
			wantError: false,
		},
		{
			name:      "in repo worktree dir only",
			cwd:       filepath.Join(worktreesDir, "myrepo"),
			wantRepo:  "myrepo",
			wantAgent: "",
			wantError: true, // Can't determine agent
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.Chdir(tt.cwd); err != nil {
				t.Fatalf("failed to change to test directory %s: %v", tt.cwd, err)
			}

			gotRepo, gotAgent, err := cli.inferAgentContext()

			if tt.wantError {
				if err == nil {
					t.Errorf("inferAgentContext() expected error, got repo=%q agent=%q", gotRepo, gotAgent)
				}
			} else {
				if err != nil {
					t.Errorf("inferAgentContext() unexpected error: %v", err)
				}
				if gotRepo != tt.wantRepo {
					t.Errorf("inferAgentContext() repo = %q, want %q", gotRepo, tt.wantRepo)
				}
				if gotAgent != tt.wantAgent {
					t.Errorf("inferAgentContext() agent = %q, want %q", gotAgent, tt.wantAgent)
				}
			}
		})
	}

	// Test main repo dir - agent name depends on tmux context, so just verify repo is found
	t.Run("in main repo dir returns repo", func(t *testing.T) {
		if err := os.Chdir(filepath.Join(reposDir, "myrepo")); err != nil {
			t.Fatalf("failed to chdir: %v", err)
		}

		gotRepo, _, err := cli.inferAgentContext()
		if err != nil {
			t.Errorf("inferAgentContext() unexpected error: %v", err)
		}
		if gotRepo != "myrepo" {
			t.Errorf("inferAgentContext() repo = %q, want %q", gotRepo, "myrepo")
		}
		// Agent name may vary based on tmux context - don't assert specific value
	})
}

// Workspace command tests

func TestValidateWorkspaceName(t *testing.T) {
	tests := []struct {
		name      string
		workspace string
		wantError bool
	}{
		{
			name:      "valid simple name",
			workspace: "default",
			wantError: false,
		},
		{
			name:      "valid name with dash",
			workspace: "my-workspace",
			wantError: false,
		},
		{
			name:      "valid name with numbers",
			workspace: "workspace123",
			wantError: false,
		},
		{
			name:      "valid name with underscore",
			workspace: "my_workspace",
			wantError: false,
		},
		{
			name:      "empty name",
			workspace: "",
			wantError: true,
		},
		{
			name:      "dot only",
			workspace: ".",
			wantError: true,
		},
		{
			name:      "double dot",
			workspace: "..",
			wantError: true,
		},
		{
			name:      "starts with dot",
			workspace: ".hidden",
			wantError: true,
		},
		{
			name:      "starts with dash",
			workspace: "-invalid",
			wantError: true,
		},
		{
			name:      "ends with dot",
			workspace: "invalid.",
			wantError: true,
		},
		{
			name:      "ends with slash",
			workspace: "invalid/",
			wantError: true,
		},
		{
			name:      "contains double dots",
			workspace: "invalid..name",
			wantError: true,
		},
		{
			name:      "contains space",
			workspace: "invalid name",
			wantError: true,
		},
		{
			name:      "contains tilde",
			workspace: "invalid~name",
			wantError: true,
		},
		{
			name:      "contains caret",
			workspace: "invalid^name",
			wantError: true,
		},
		{
			name:      "contains colon",
			workspace: "invalid:name",
			wantError: true,
		},
		{
			name:      "contains question mark",
			workspace: "invalid?name",
			wantError: true,
		},
		{
			name:      "contains asterisk",
			workspace: "invalid*name",
			wantError: true,
		},
		{
			name:      "contains bracket",
			workspace: "invalid[name",
			wantError: true,
		},
		{
			name:      "contains at sign",
			workspace: "invalid@name",
			wantError: true,
		},
		{
			name:      "contains backslash",
			workspace: "invalid\\name",
			wantError: true,
		},
		{
			name:      "contains curly brace",
			workspace: "invalid{name",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWorkspaceName(tt.workspace)
			if (err != nil) != tt.wantError {
				t.Errorf("validateWorkspaceName(%q) error = %v, wantError %v", tt.workspace, err, tt.wantError)
			}
		})
	}
}

func TestCLIWorkspaceListEmpty(t *testing.T) {
	cli, d, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Add a repo first via daemon so we can list workspaces
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test-repo",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.GetState().AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// List workspaces - should work even when empty
	err := cli.Execute([]string{"workspace", "list", "--repo", "test-repo"})
	if err != nil {
		t.Errorf("workspace list failed: %v", err)
	}
}

func TestCLIWorkspaceListWithWorkspaces(t *testing.T) {
	cli, d, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Add a repo and workspace via daemon
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test-repo",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.GetState().AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	agent := state.Agent{
		Type:         state.AgentTypeWorkspace,
		WorktreePath: "/tmp/test-workspace",
		TmuxWindow:   "default",
		CreatedAt:    time.Now(),
	}
	if err := d.GetState().AddAgent("test-repo", "default", agent); err != nil {
		t.Fatalf("Failed to add workspace agent: %v", err)
	}

	// List workspaces - should show the workspace
	err := cli.Execute([]string{"workspace", "list", "--repo", "test-repo"})
	if err != nil {
		t.Errorf("workspace list with workspaces failed: %v", err)
	}
}

func TestCLIWorkspaceDefaultAction(t *testing.T) {
	cli, d, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Add a repo via daemon
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test-repo",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.GetState().AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// workspace with no args should list (same as workspace list)
	err := cli.Execute([]string{"workspace", "--repo", "test-repo"})
	if err != nil {
		t.Errorf("workspace (no args) failed: %v", err)
	}
}

func TestCLIWorkspaceAddValidation(t *testing.T) {
	cli, d, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Add a repo
	repoPath := cli.paths.RepoDir("test-repo")
	setupTestRepo(t, repoPath)

	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test-repo",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.GetState().AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// workspace add with invalid name should fail
	err := cli.Execute([]string{"workspace", "add", ".invalid", "--repo", "test-repo"})
	if err == nil {
		t.Error("workspace add with invalid name should fail")
	}
}

func TestCLIWorkspaceAddMissingName(t *testing.T) {
	cli, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// workspace add without name should fail
	err := cli.Execute([]string{"workspace", "add"})
	if err == nil {
		t.Error("workspace add without name should fail")
	}
}

func TestCLIWorkspaceRmMissingName(t *testing.T) {
	cli, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// workspace rm without name should fail
	err := cli.Execute([]string{"workspace", "rm"})
	if err == nil {
		t.Error("workspace rm without name should fail")
	}
}

func TestCLIWorkspaceConnectMissingName(t *testing.T) {
	cli, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// workspace connect without name should fail
	err := cli.Execute([]string{"workspace", "connect"})
	if err == nil {
		t.Error("workspace connect without name should fail")
	}
}

// Config and additional tests from PR #81

func TestCLIConfigRepoNoArgs(t *testing.T) {
	cli, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Config with no args should show help/error
	err := cli.Execute([]string{"config"})
	if err == nil {
		t.Error("config with no args should fail")
	}
}

func TestCLIConfigRepoShow(t *testing.T) {
	cli, d, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Add a repo with specific config
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test-repo",
		Agents:      make(map[string]state.Agent),
		MergeQueueConfig: state.MergeQueueConfig{
			Enabled:   true,
			TrackMode: state.TrackModeAuthor,
		},
	}
	if err := d.GetState().AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Show config should work
	err := cli.Execute([]string{"config", "test-repo"})
	if err != nil {
		t.Errorf("config show failed: %v", err)
	}
}

func TestCLIConfigRepoUpdateViaSocket(t *testing.T) {
	_, d, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Add a repo
	repo := &state.Repository{
		GithubURL:        "https://github.com/test/repo",
		TmuxSession:      "mc-test-repo",
		Agents:           make(map[string]state.Agent),
		MergeQueueConfig: state.DefaultMergeQueueConfig(),
	}
	if err := d.GetState().AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Update config via socket directly (tests the actual update mechanism)
	client := socket.NewClient(d.GetPaths().DaemonSock)
	resp, err := client.Send(socket.Request{
		Command: "update_repo_config",
		Args: map[string]interface{}{
			"name":       "test-repo",
			"mq_enabled": false,
		},
	})
	if err != nil {
		t.Fatalf("Failed to send update_repo_config: %v", err)
	}
	if !resp.Success {
		t.Errorf("update_repo_config failed: %s", resp.Error)
	}

	// Verify the update took effect
	updatedRepo, _ := d.GetState().GetRepo("test-repo")
	if updatedRepo.MergeQueueConfig.Enabled != false {
		t.Error("MergeQueueConfig.Enabled should be false after update")
	}
}

func TestCLIConfigRepoNonexistent(t *testing.T) {
	cli, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Config for nonexistent repo should fail
	err := cli.Execute([]string{"config", "nonexistent-repo"})
	if err == nil {
		t.Error("config for nonexistent repo should fail")
	}
}

func TestCLIRemoveWorkerNonexistent(t *testing.T) {
	cli, d, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Add a repo
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test-repo",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.GetState().AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Remove nonexistent worker should fail
	err := cli.Execute([]string{"work", "rm", "nonexistent-worker", "--repo", "test-repo"})
	if err == nil {
		t.Error("removing nonexistent worker should fail")
	}
}

func TestCLIRemoveWorkerWithRealTmux(t *testing.T) {
	tmuxClient := tmux.NewClient()
	if !tmuxClient.IsTmuxAvailable() {
		t.Fatal("tmux is required for this test but not available")
	}

	cli, d, cleanup := setupTestEnvironment(t)
	defer cleanup()

	paths := d.GetPaths()
	repoName := "test-repo"

	// Create tmux session
	tmuxSession := "mc-test-rm"
	if err := tmuxClient.CreateSession(context.Background(), tmuxSession, true); err != nil {
		t.Fatalf("Failed to create tmux session: %v", err)
	}
	defer tmuxClient.KillSession(context.Background(), tmuxSession)

	// Create worker window
	if err := tmuxClient.CreateWindow(context.Background(), tmuxSession, "test-worker"); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// Add repo to daemon
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: tmuxSession,
		Agents:      make(map[string]state.Agent),
	}
	if err := d.GetState().AddRepo(repoName, repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Add agent to state
	agent := state.Agent{
		Type:         state.AgentTypeWorker,
		WorktreePath: paths.AgentWorktree(repoName, "test-worker"),
		TmuxWindow:   "test-worker",
		Task:         "Test task",
		CreatedAt:    time.Now(),
	}
	if err := d.GetState().AddAgent(repoName, "test-worker", agent); err != nil {
		t.Fatalf("Failed to add agent: %v", err)
	}

	// Verify agent exists
	_, exists := d.GetState().GetAgent(repoName, "test-worker")
	if !exists {
		t.Fatal("Agent should exist before removal")
	}

	// Remove worker
	err := cli.Execute([]string{"work", "rm", "test-worker", "--repo", repoName})
	if err != nil {
		t.Errorf("work rm failed: %v", err)
	}

	// Verify agent was removed from state
	_, exists = d.GetState().GetAgent(repoName, "test-worker")
	if exists {
		t.Error("Agent should not exist after removal")
	}
}

func TestCLIAgentCompleteViaSocket(t *testing.T) {
	_, d, cleanup := setupTestEnvironment(t)
	defer cleanup()

	repoName := "test-repo"
	paths := d.GetPaths()

	// Add a repo
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test-repo",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.GetState().AddRepo(repoName, repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Add agent
	agent := state.Agent{
		Type:         state.AgentTypeWorker,
		WorktreePath: paths.AgentWorktree(repoName, "test-worker"),
		TmuxWindow:   "test-worker",
		CreatedAt:    time.Now(),
	}
	if err := d.GetState().AddAgent(repoName, "test-worker", agent); err != nil {
		t.Fatalf("Failed to add agent: %v", err)
	}

	// Test complete_agent directly via socket (the core functionality)
	client := socket.NewClient(paths.DaemonSock)
	resp, err := client.Send(socket.Request{
		Command: "complete_agent",
		Args: map[string]interface{}{
			"repo":  repoName,
			"agent": "test-worker",
		},
	})
	if err != nil {
		t.Fatalf("Failed to send complete_agent: %v", err)
	}
	if !resp.Success {
		t.Errorf("complete_agent failed: %s", resp.Error)
	}

	// Verify agent is marked for cleanup
	updatedAgent, _ := d.GetState().GetAgent(repoName, "test-worker")
	if !updatedAgent.ReadyForCleanup {
		t.Error("Agent should be marked for cleanup")
	}
}

func TestCLIReviewInvalidURL(t *testing.T) {
	cli, d, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Add a repo
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test-repo",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.GetState().AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Invalid URL format should fail
	err := cli.Execute([]string{"review", "not-a-url"})
	if err == nil {
		t.Error("review with invalid URL should fail")
	}
}

func TestCLIGetReposList(t *testing.T) {
	cli, d, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Initially empty
	repos := cli.getReposList()
	if len(repos) != 0 {
		t.Errorf("Expected 0 repos, got %d", len(repos))
	}

	// Add some repos
	for _, name := range []string{"repo1", "repo2", "repo3"} {
		repo := &state.Repository{
			GithubURL:   "https://github.com/test/" + name,
			TmuxSession: "mc-" + name,
			Agents:      make(map[string]state.Agent),
		}
		if err := d.GetState().AddRepo(name, repo); err != nil {
			t.Fatalf("Failed to add repo: %v", err)
		}
	}

	// Should now have 3 repos
	repos = cli.getReposList()
	if len(repos) != 3 {
		t.Errorf("Expected 3 repos, got %d", len(repos))
	}
}

func TestCLIBugCommand(t *testing.T) {
	cli, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Bug command should work (generates diagnostic report)
	// Note: This might print to stdout but shouldn't fail
	err := cli.Execute([]string{"bug", "test description"})
	// Bug command may or may not be implemented, just verify it doesn't panic
	_ = err
}

func TestCLIAgentListMessages(t *testing.T) {
	_, d, cleanup := setupTestEnvironment(t)
	defer cleanup()

	repoName := "test-repo"
	paths := d.GetPaths()

	// Add a repo
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test-repo",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.GetState().AddRepo(repoName, repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Add agents
	for _, name := range []string{"supervisor", "worker1"} {
		agent := state.Agent{
			Type:       state.AgentTypeSupervisor,
			TmuxWindow: name,
			CreatedAt:  time.Now(),
		}
		if err := d.GetState().AddAgent(repoName, name, agent); err != nil {
			t.Fatalf("Failed to add agent: %v", err)
		}
	}

	// Create and send a message using message manager
	msgMgr := messages.NewManager(paths.MessagesDir)
	msg, err := msgMgr.Send(repoName, "supervisor", "worker1", "Test message content")
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Verify message was created
	if msg.ID == "" {
		t.Error("Message ID should not be empty")
	}
	if msg.Status != messages.StatusPending {
		t.Errorf("Message status = %s, want pending", msg.Status)
	}

	// List messages for worker1
	msgs, err := msgMgr.List(repoName, "worker1")
	if err != nil {
		t.Fatalf("Failed to list messages: %v", err)
	}
	if len(msgs) != 1 {
		t.Errorf("Expected 1 message, got %d", len(msgs))
	}

	// Get specific message
	retrieved, err := msgMgr.Get(repoName, "worker1", msg.ID)
	if err != nil {
		t.Fatalf("Failed to get message: %v", err)
	}
	if retrieved.Body != "Test message content" {
		t.Errorf("Message body = %q, want %q", retrieved.Body, "Test message content")
	}

	// Ack message
	if err := msgMgr.Ack(repoName, "worker1", msg.ID); err != nil {
		t.Fatalf("Failed to ack message: %v", err)
	}

	// Verify status changed
	acked, _ := msgMgr.Get(repoName, "worker1", msg.ID)
	if acked.Status != messages.StatusAcked {
		t.Errorf("Message status = %s, want acked", acked.Status)
	}
}

func TestCLIRepoRm(t *testing.T) {
	cli, d, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Add a repo
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test-repo",
		Agents:      make(map[string]state.Agent),
	}
	if err := d.GetState().AddRepo("test-repo", repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Verify repo exists
	_, exists := d.GetState().GetRepo("test-repo")
	if !exists {
		t.Fatal("Repo should exist before removal")
	}

	// Remove repo
	err := cli.Execute([]string{"repo", "rm", "test-repo"})
	if err != nil {
		t.Errorf("repo rm failed: %v", err)
	}

	// Verify repo was removed
	_, exists = d.GetState().GetRepo("test-repo")
	if exists {
		t.Error("Repo should not exist after removal")
	}
}

func TestCLIRepoRmNonexistent(t *testing.T) {
	cli, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Remove nonexistent repo should fail
	err := cli.Execute([]string{"repo", "rm", "nonexistent"})
	if err == nil {
		t.Error("removing nonexistent repo should fail")
	}
}

func TestInitRepoNameParsing(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		wantError   bool
		errContains string
	}{
		{
			name:      "normal URL",
			url:       "https://github.com/user/repo",
			wantError: false, // Will fail later, but name parsing succeeds
		},
		{
			name:      "URL with .git suffix",
			url:       "https://github.com/user/repo.git",
			wantError: false,
		},
		{
			name:      "URL with trailing slash",
			url:       "https://github.com/user/repo/",
			wantError: false, // Should work - trailing slash is trimmed
		},
		{
			name:      "URL with multiple trailing slashes",
			url:       "https://github.com/user/repo///",
			wantError: false, // TrimRight removes all trailing slashes
		},
		{
			name:        "URL that is just slashes",
			url:         "///",
			wantError:   true,
			errContains: "could not determine repository name",
		},
		{
			name:        "domain only URL with trailing slash",
			url:         "https://github.com/",
			wantError:   true,
			errContains: "could not determine repository name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cli, _, cleanup := setupTestEnvironment(t)
			defer cleanup()

			err := cli.Execute([]string{"init", tt.url})

			if tt.wantError {
				if err == nil {
					t.Error("expected error but got none")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
			} else {
				// For valid URLs, we expect the error to be about something other than the name
				// (e.g., git clone failing because the repo doesn't exist)
				if err != nil && strings.Contains(err.Error(), "could not determine repository name") {
					t.Errorf("unexpected name parsing error: %v", err)
				}
			}
		})
	}
}

func TestSanitizeTmuxSessionName(t *testing.T) {
	tests := []struct {
		repoName string
		want     string
	}{
		{"my-repo", "mc-my-repo"},
		{"demos.expanso.io", "mc-demos-expanso-io"},
		{"repo.with.many.dots", "mc-repo-with-many-dots"},
		{"repo:with:colons", "mc-repo-with-colons"},
		{"repo with spaces", "mc-repo-with-spaces"},
		{"simple", "mc-simple"},
		{"repo/with/slashes", "mc-repo-with-slashes"},
		{"path/to/repo.git", "mc-path-to-repo-git"},
		{"repo\x00with\x1fnull", "mc-repowithnull"}, // control characters stripped
	}

	for _, tt := range tests {
		t.Run(tt.repoName, func(t *testing.T) {
			got := sanitizeTmuxSessionName(tt.repoName)
			if got != tt.want {
				t.Errorf("sanitizeTmuxSessionName(%q) = %q, want %q", tt.repoName, got, tt.want)
			}
		})
	}
}

func TestNormalizeGitHubURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "SSH format",
			url:  "git@github.com:user/repo.git",
			want: "github.com/user/repo",
		},
		{
			name: "SSH format without .git",
			url:  "git@github.com:user/repo",
			want: "github.com/user/repo",
		},
		{
			name: "HTTPS format",
			url:  "https://github.com/user/repo",
			want: "github.com/user/repo",
		},
		{
			name: "HTTPS format with .git",
			url:  "https://github.com/user/repo.git",
			want: "github.com/user/repo",
		},
		{
			name: "HTTP format",
			url:  "http://github.com/user/repo",
			want: "github.com/user/repo",
		},
		{
			name: "git:// protocol",
			url:  "git://github.com/user/repo.git",
			want: "github.com/user/repo",
		},
		{
			name: "mixed case",
			url:  "https://GitHub.com/User/Repo",
			want: "github.com/user/repo",
		},
		{
			name: "whitespace trimmed",
			url:  "  https://github.com/user/repo  ",
			want: "github.com/user/repo",
		},
		{
			name: "non-GitHub URL",
			url:  "https://gitlab.com/user/repo",
			want: "",
		},
		{
			name: "empty string",
			url:  "",
			want: "",
		},
		{
			name: "organization repo SSH",
			url:  "git@github.com:myorg/myproject.git",
			want: "github.com/myorg/myproject",
		},
		{
			name: "nested path SSH",
			url:  "git@github.com:user/nested/path.git",
			want: "github.com/user/nested/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeGitHubURL(tt.url)
			if got != tt.want {
				t.Errorf("normalizeGitHubURL(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestFindRepoFromGitRemote(t *testing.T) {
	// Save original working directory
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get original working directory: %v", err)
	}
	defer os.Chdir(origWd)

	t.Run("matches HTTPS URL in state with SSH remote", func(t *testing.T) {
		tmpDir := t.TempDir()
		stateFile := filepath.Join(tmpDir, "state.json")

		// Create state with HTTPS URL
		st := state.New(stateFile)
		st.AddRepo("my-repo", &state.Repository{
			GithubURL:   "https://github.com/myuser/myrepo",
			TmuxSession: "mc-my-repo",
			Agents:      make(map[string]state.Agent),
		})

		// Create a git repo with SSH remote
		gitDir := filepath.Join(tmpDir, "git-test")
		if err := os.MkdirAll(gitDir, 0755); err != nil {
			t.Fatalf("failed to create git dir: %v", err)
		}
		if err := os.Chdir(gitDir); err != nil {
			t.Fatalf("failed to chdir: %v", err)
		}

		// Initialize git repo and set remote
		if err := exec.Command("git", "init").Run(); err != nil {
			t.Fatalf("failed to init git: %v", err)
		}
		if err := exec.Command("git", "remote", "add", "origin", "git@github.com:myuser/myrepo.git").Run(); err != nil {
			t.Fatalf("failed to add remote: %v", err)
		}

		cli := &CLI{
			paths: &config.Paths{
				StateFile: stateFile,
			},
		}

		repoName, err := cli.findRepoFromGitRemote()
		if err != nil {
			t.Fatalf("findRepoFromGitRemote() error: %v", err)
		}
		if repoName != "my-repo" {
			t.Errorf("findRepoFromGitRemote() = %q, want %q", repoName, "my-repo")
		}
	})

	t.Run("matches SSH URL in state with HTTPS remote", func(t *testing.T) {
		tmpDir := t.TempDir()
		stateFile := filepath.Join(tmpDir, "state.json")

		// Create state with SSH URL
		st := state.New(stateFile)
		st.AddRepo("another-repo", &state.Repository{
			GithubURL:   "git@github.com:anotheruser/anotherrepo.git",
			TmuxSession: "mc-another-repo",
			Agents:      make(map[string]state.Agent),
		})

		// Create a git repo with HTTPS remote
		gitDir := filepath.Join(tmpDir, "git-test")
		if err := os.MkdirAll(gitDir, 0755); err != nil {
			t.Fatalf("failed to create git dir: %v", err)
		}
		if err := os.Chdir(gitDir); err != nil {
			t.Fatalf("failed to chdir: %v", err)
		}

		// Initialize git repo and set remote
		if err := exec.Command("git", "init").Run(); err != nil {
			t.Fatalf("failed to init git: %v", err)
		}
		if err := exec.Command("git", "remote", "add", "origin", "https://github.com/anotheruser/anotherrepo").Run(); err != nil {
			t.Fatalf("failed to add remote: %v", err)
		}

		cli := &CLI{
			paths: &config.Paths{
				StateFile: stateFile,
			},
		}

		repoName, err := cli.findRepoFromGitRemote()
		if err != nil {
			t.Fatalf("findRepoFromGitRemote() error: %v", err)
		}
		if repoName != "another-repo" {
			t.Errorf("findRepoFromGitRemote() = %q, want %q", repoName, "another-repo")
		}
	})

	t.Run("no match returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		stateFile := filepath.Join(tmpDir, "state.json")

		// Create state with different URL
		st := state.New(stateFile)
		st.AddRepo("other-repo", &state.Repository{
			GithubURL:   "https://github.com/different/repo",
			TmuxSession: "mc-other-repo",
			Agents:      make(map[string]state.Agent),
		})

		// Create a git repo with non-matching remote
		gitDir := filepath.Join(tmpDir, "git-test")
		if err := os.MkdirAll(gitDir, 0755); err != nil {
			t.Fatalf("failed to create git dir: %v", err)
		}
		if err := os.Chdir(gitDir); err != nil {
			t.Fatalf("failed to chdir: %v", err)
		}

		// Initialize git repo and set remote
		if err := exec.Command("git", "init").Run(); err != nil {
			t.Fatalf("failed to init git: %v", err)
		}
		if err := exec.Command("git", "remote", "add", "origin", "git@github.com:nomatch/norepo.git").Run(); err != nil {
			t.Fatalf("failed to add remote: %v", err)
		}

		cli := &CLI{
			paths: &config.Paths{
				StateFile: stateFile,
			},
		}

		_, err := cli.findRepoFromGitRemote()
		if err == nil {
			t.Error("findRepoFromGitRemote() expected error for non-matching remote")
		}
	})

	t.Run("not in git repo returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		stateFile := filepath.Join(tmpDir, "state.json")

		// Create empty state
		_ = state.New(stateFile)

		// Not a git directory
		if err := os.Chdir(tmpDir); err != nil {
			t.Fatalf("failed to chdir: %v", err)
		}

		cli := &CLI{
			paths: &config.Paths{
				StateFile: stateFile,
			},
		}

		_, err := cli.findRepoFromGitRemote()
		if err == nil {
			t.Error("findRepoFromGitRemote() expected error when not in git repo")
		}
	})

	t.Run("case insensitive matching", func(t *testing.T) {
		tmpDir := t.TempDir()
		stateFile := filepath.Join(tmpDir, "state.json")

		// Create state with mixed case URL
		st := state.New(stateFile)
		st.AddRepo("case-test", &state.Repository{
			GithubURL:   "https://GitHub.com/MyUser/MyRepo",
			TmuxSession: "mc-case-test",
			Agents:      make(map[string]state.Agent),
		})

		// Create a git repo with different case remote
		gitDir := filepath.Join(tmpDir, "git-test")
		if err := os.MkdirAll(gitDir, 0755); err != nil {
			t.Fatalf("failed to create git dir: %v", err)
		}
		if err := os.Chdir(gitDir); err != nil {
			t.Fatalf("failed to chdir: %v", err)
		}

		// Initialize git repo and set remote with different case
		if err := exec.Command("git", "init").Run(); err != nil {
			t.Fatalf("failed to init git: %v", err)
		}
		if err := exec.Command("git", "remote", "add", "origin", "git@github.com:myuser/myrepo.git").Run(); err != nil {
			t.Fatalf("failed to add remote: %v", err)
		}

		cli := &CLI{
			paths: &config.Paths{
				StateFile: stateFile,
			},
		}

		repoName, err := cli.findRepoFromGitRemote()
		if err != nil {
			t.Fatalf("findRepoFromGitRemote() error: %v", err)
		}
		if repoName != "case-test" {
			t.Errorf("findRepoFromGitRemote() = %q, want %q", repoName, "case-test")
		}
	})

	t.Run("multiple repos selects correct one", func(t *testing.T) {
		tmpDir := t.TempDir()
		stateFile := filepath.Join(tmpDir, "state.json")

		// Create state with multiple repos
		st := state.New(stateFile)
		st.AddRepo("repo-a", &state.Repository{
			GithubURL:   "https://github.com/user/repo-a",
			TmuxSession: "mc-repo-a",
			Agents:      make(map[string]state.Agent),
		})
		st.AddRepo("repo-b", &state.Repository{
			GithubURL:   "https://github.com/user/repo-b",
			TmuxSession: "mc-repo-b",
			Agents:      make(map[string]state.Agent),
		})
		st.AddRepo("repo-c", &state.Repository{
			GithubURL:   "https://github.com/user/repo-c",
			TmuxSession: "mc-repo-c",
			Agents:      make(map[string]state.Agent),
		})

		// Create a git repo pointing to repo-b
		gitDir := filepath.Join(tmpDir, "git-test")
		if err := os.MkdirAll(gitDir, 0755); err != nil {
			t.Fatalf("failed to create git dir: %v", err)
		}
		if err := os.Chdir(gitDir); err != nil {
			t.Fatalf("failed to chdir: %v", err)
		}

		// Initialize git repo and set remote
		if err := exec.Command("git", "init").Run(); err != nil {
			t.Fatalf("failed to init git: %v", err)
		}
		if err := exec.Command("git", "remote", "add", "origin", "git@github.com:user/repo-b.git").Run(); err != nil {
			t.Fatalf("failed to add remote: %v", err)
		}

		cli := &CLI{
			paths: &config.Paths{
				StateFile: stateFile,
			},
		}

		repoName, err := cli.findRepoFromGitRemote()
		if err != nil {
			t.Fatalf("findRepoFromGitRemote() error: %v", err)
		}
		if repoName != "repo-b" {
			t.Errorf("findRepoFromGitRemote() = %q, want %q", repoName, "repo-b")
		}
	})
}

func TestGetVersion(t *testing.T) {
	// Save original version
	originalVersion := Version
	defer func() { Version = originalVersion }()

	tests := []struct {
		name        string
		version     string
		wantPrefix  string
		wantSuffix  string
		wantContain string
	}{
		{
			name:       "release version unchanged",
			version:    "v1.2.3",
			wantPrefix: "v1.2.3",
		},
		{
			name:       "semver without v prefix unchanged",
			version:    "1.0.0",
			wantPrefix: "1.0.0",
		},
		{
			name:        "dev version gets semver format",
			version:     "dev",
			wantPrefix:  "0.0.0",
			wantContain: "dev",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Version = tt.version
			got := GetVersion()

			if tt.wantPrefix != "" && !strings.HasPrefix(got, tt.wantPrefix) {
				t.Errorf("GetVersion() = %q, want prefix %q", got, tt.wantPrefix)
			}
			if tt.wantSuffix != "" && !strings.HasSuffix(got, tt.wantSuffix) {
				t.Errorf("GetVersion() = %q, want suffix %q", got, tt.wantSuffix)
			}
			if tt.wantContain != "" && !strings.Contains(got, tt.wantContain) {
				t.Errorf("GetVersion() = %q, want to contain %q", got, tt.wantContain)
			}
		})
	}
}

func TestIsDevVersion(t *testing.T) {
	// Save original version
	originalVersion := Version
	defer func() { Version = originalVersion }()

	tests := []struct {
		name    string
		version string
		want    bool
	}{
		{
			name:    "dev is dev version",
			version: "dev",
			want:    true,
		},
		{
			name:    "release version is not dev",
			version: "v1.2.3",
			want:    false,
		},
		{
			name:    "semver is not dev",
			version: "1.0.0",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Version = tt.version
			if got := IsDevVersion(); got != tt.want {
				t.Errorf("IsDevVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestListAgentDefinitions(t *testing.T) {
	// Create temp directory structure
	tmpDir, err := os.MkdirTemp("", "cli-agents-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	paths := config.NewTestPaths(tmpDir)

	// Create necessary directories
	if err := paths.EnsureDirectories(); err != nil {
		t.Fatal(err)
	}

	repoName := "test-repo"

	// Create local agents directory
	localAgentsDir := paths.RepoAgentsDir(repoName)
	if err := os.MkdirAll(localAgentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a local agent definition
	workerContent := `# Worker Agent

A task-based worker.
`
	if err := os.WriteFile(filepath.Join(localAgentsDir, "worker.md"), []byte(workerContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create state file with test repo
	st := state.New(paths.StateFile)
	if err := st.AddRepo(repoName, &state.Repository{
		GithubURL:   "https://github.com/test/test-repo",
		TmuxSession: "mc-test-repo",
		Agents:      make(map[string]state.Agent),
	}); err != nil {
		t.Fatal(err)
	}

	// Create repo directory (for checked-in definitions lookup)
	repoPath := paths.RepoDir(repoName)
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatal(err)
	}

	// Create checked-in agent definition directory
	repoAgentsDir := filepath.Join(repoPath, ".multiclaude", "agents")
	if err := os.MkdirAll(repoAgentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create checked-in agent definition that should override local
	workerRepoContent := `# Worker Agent (Team Version)

A team-customized worker.
`
	if err := os.WriteFile(filepath.Join(repoAgentsDir, "worker.md"), []byte(workerRepoContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a unique checked-in definition
	customContent := `# Custom Bot

A team-specific bot.
`
	if err := os.WriteFile(filepath.Join(repoAgentsDir, "custom-bot.md"), []byte(customContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create CLI
	cli := NewWithPaths(paths)

	// Change to repo directory to allow resolveRepo to work
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	if err := os.Chdir(repoPath); err != nil {
		t.Fatalf("Failed to change to repo: %v", err)
	}

	// Run list agents definitions (this doesn't require daemon)
	err = cli.listAgentDefinitions([]string{"--repo", repoName})
	if err != nil {
		t.Errorf("listAgentDefinitions failed: %v", err)
	}
}

func TestGetClaudeBinaryReturnsValue(t *testing.T) {
	cli, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	binary, err := cli.getClaudeBinary()
	// May fail if claude not installed, but shouldn't panic
	if err == nil && binary == "" {
		t.Error("getClaudeBinary() returned empty string without error")
	}
}

func TestShowVersionNoPanic(t *testing.T) {
	cli, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Test that showVersion doesn't panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("showVersion() panicked: %v", r)
		}
	}()

	cli.showVersion()
}

func TestVersionCommandBasic(t *testing.T) {
	cli, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Test version command with no flags
	err := cli.versionCommand([]string{})
	if err != nil {
		t.Errorf("versionCommand() failed: %v", err)
	}
}

func TestVersionCommandJSON(t *testing.T) {
	cli, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Test version command with --json flag
	err := cli.versionCommand([]string{"--json"})
	if err != nil {
		t.Errorf("versionCommand(--json) failed: %v", err)
	}
}

func TestShowHelpNoPanic(t *testing.T) {
	cli, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Test that showHelp doesn't panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("showHelp() panicked: %v", r)
		}
	}()

	cli.showHelp()
}

func TestExecuteEmptyArgs(t *testing.T) {
	cli, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Test Execute with empty args (should show help)
	err := cli.Execute([]string{})
	// Should not panic, may or may not error
	_ = err
}

func TestExecuteUnknownCommand(t *testing.T) {
	cli, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Test Execute with unknown command
	err := cli.Execute([]string{"nonexistent-command-xyz"})
	if err == nil {
		t.Error("Execute should fail with unknown command")
	}
}

func TestSpawnAgentFromFile(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantError string
	}{
		{
			name:      "missing name flag",
			args:      []string{"--class", "ephemeral", "--prompt-file", "/tmp/prompt.md"},
			wantError: "--name is required",
		},
		{
			name:      "missing class flag",
			args:      []string{"--name", "test-agent", "--prompt-file", "/tmp/prompt.md"},
			wantError: "--class is required",
		},
		{
			name:      "missing prompt-file flag",
			args:      []string{"--name", "test-agent", "--class", "ephemeral"},
			wantError: "--prompt-file is required",
		},
		{
			name:      "invalid class value",
			args:      []string{"--name", "test-agent", "--class", "invalid", "--prompt-file", "/tmp/prompt.md"},
			wantError: "--class must be 'persistent' or 'ephemeral'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cli, _, cleanup := setupTestEnvironment(t)
			defer cleanup()

			err := cli.spawnAgentFromFile(tt.args)
			if err == nil {
				t.Fatalf("spawnAgentFromFile() should fail with error containing %q", tt.wantError)
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Errorf("spawnAgentFromFile() error = %q, want to contain %q", err.Error(), tt.wantError)
			}
		})
	}
}

func TestSpawnAgentFromFilePromptNotFound(t *testing.T) {
	cli, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Use a non-existent prompt file path
	nonExistentPath := filepath.Join(cli.paths.Root, "nonexistent", "prompt.md")

	// Create a test repo to satisfy the repo resolution
	repoName := "test-repo"
	repoPath := cli.paths.RepoDir(repoName)
	setupTestRepo(t, repoPath)

	// Add repo to state
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test-repo",
		Agents:      make(map[string]state.Agent),
	}
	st, _ := state.Load(cli.paths.StateFile)
	st.AddRepo(repoName, repo)

	args := []string{
		"--name", "test-agent",
		"--class", "ephemeral",
		"--prompt-file", nonExistentPath,
		"--repo", repoName,
	}

	err := cli.spawnAgentFromFile(args)
	if err == nil {
		t.Fatal("spawnAgentFromFile() should fail when prompt file doesn't exist")
	}
	if !strings.Contains(err.Error(), "failed to read prompt file") {
		t.Errorf("spawnAgentFromFile() error = %q, want to contain 'failed to read prompt file'", err.Error())
	}
}

func TestResetAgentDefinitions(t *testing.T) {
	cli, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Create a test repo
	repoName := "test-repo"
	repoPath := cli.paths.RepoDir(repoName)
	setupTestRepo(t, repoPath)

	// Add repo to state
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test-repo",
		Agents:      make(map[string]state.Agent),
	}
	st, _ := state.Load(cli.paths.StateFile)
	st.AddRepo(repoName, repo)

	t.Run("creates fresh when agents dir does not exist", func(t *testing.T) {
		agentsDir := cli.paths.RepoAgentsDir(repoName)

		// Ensure agents dir doesn't exist
		os.RemoveAll(agentsDir)

		// Run reset
		err := cli.resetAgentDefinitions([]string{"--repo", repoName})
		if err != nil {
			t.Fatalf("resetAgentDefinitions() error = %v", err)
		}

		// Verify agents dir was created
		if _, err := os.Stat(agentsDir); os.IsNotExist(err) {
			t.Error("agents directory should exist after reset")
		}

		// Verify some templates were copied
		entries, err := os.ReadDir(agentsDir)
		if err != nil {
			t.Fatalf("Failed to read agents dir: %v", err)
		}
		if len(entries) == 0 {
			t.Error("agents directory should contain template files")
		}
	})

	t.Run("removes and re-copies when agents dir exists", func(t *testing.T) {
		agentsDir := cli.paths.RepoAgentsDir(repoName)

		// Create agents dir with a custom file
		if err := os.MkdirAll(agentsDir, 0755); err != nil {
			t.Fatalf("Failed to create agents dir: %v", err)
		}
		customFile := filepath.Join(agentsDir, "custom-file.md")
		if err := os.WriteFile(customFile, []byte("custom content"), 0644); err != nil {
			t.Fatalf("Failed to write custom file: %v", err)
		}

		// Run reset
		err := cli.resetAgentDefinitions([]string{"--repo", repoName})
		if err != nil {
			t.Fatalf("resetAgentDefinitions() error = %v", err)
		}

		// Verify custom file was removed
		if _, err := os.Stat(customFile); !os.IsNotExist(err) {
			t.Error("custom file should be removed after reset")
		}

		// Verify templates were copied
		entries, err := os.ReadDir(agentsDir)
		if err != nil {
			t.Fatalf("Failed to read agents dir: %v", err)
		}
		if len(entries) == 0 {
			t.Error("agents directory should contain template files after reset")
		}
	})
}

// TestCLISetCurrentRepo tests the setCurrentRepo command
func TestCLISetCurrentRepo(t *testing.T) {
	cli, d, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Add a test repo to daemon's state via socket
	client := socket.NewClient(d.GetPaths().DaemonSock)
	_, err := client.Send(socket.Request{
		Command: "add_repo",
		Args: map[string]interface{}{
			"name":         "test-repo",
			"github_url":   "https://github.com/test/repo",
			"tmux_session": "mc-test-repo",
		},
	})
	if err != nil {
		t.Fatalf("Failed to add repo via socket: %v", err)
	}

	t.Run("sets current repo successfully", func(t *testing.T) {
		err := cli.setCurrentRepo([]string{"test-repo"})
		if err != nil {
			t.Fatalf("setCurrentRepo() error = %v", err)
		}

		// Verify it was set via daemon
		resp, err := client.Send(socket.Request{Command: "get_current_repo"})
		if err != nil {
			t.Fatalf("Failed to get current repo: %v", err)
		}
		if currentRepo, ok := resp.Data.(string); !ok || currentRepo != "test-repo" {
			t.Errorf("CurrentRepo = %v, want test-repo", resp.Data)
		}
	})

	t.Run("returns error when no repo name provided", func(t *testing.T) {
		err := cli.setCurrentRepo([]string{})
		if err == nil {
			t.Error("setCurrentRepo() should return error when no repo name provided")
		}
	})

	t.Run("returns error for nonexistent repo", func(t *testing.T) {
		err := cli.setCurrentRepo([]string{"nonexistent-repo"})
		if err == nil {
			t.Error("setCurrentRepo() should return error for nonexistent repo")
		}
	})
}

// TestCLIGetCurrentRepo tests the getCurrentRepo command
func TestCLIGetCurrentRepo(t *testing.T) {
	cli, d, cleanup := setupTestEnvironment(t)
	defer cleanup()

	t.Run("shows message when no repo set", func(t *testing.T) {
		// Ensure no current repo is set - this should not error,
		// just show a message
		err := cli.getCurrentRepo([]string{})
		// This command prints output but doesn't return an error
		// when no repo is set, so we just check it doesn't panic
		_ = err // Ignore error as it may or may not error depending on daemon state
	})

	t.Run("shows current repo when set", func(t *testing.T) {
		// Add and set a current repo via daemon
		client := socket.NewClient(d.GetPaths().DaemonSock)
		_, err := client.Send(socket.Request{
			Command: "add_repo",
			Args: map[string]interface{}{
				"name":         "test-repo",
				"github_url":   "https://github.com/test/repo",
				"tmux_session": "mc-test-repo",
			},
		})
		if err != nil {
			t.Fatalf("Failed to add repo: %v", err)
		}

		// Set it as current
		_, err = client.Send(socket.Request{
			Command: "set_current_repo",
			Args:    map[string]interface{}{"name": "test-repo"},
		})
		if err != nil {
			t.Fatalf("Failed to set current repo: %v", err)
		}

		err = cli.getCurrentRepo([]string{})
		if err != nil {
			t.Fatalf("getCurrentRepo() error = %v", err)
		}
	})
}

// TestCLIClearCurrentRepo tests the clearCurrentRepo command
func TestCLIClearCurrentRepo(t *testing.T) {
	cli, d, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Set a current repo first
	st, _ := state.Load(d.GetPaths().StateFile)
	repo := &state.Repository{
		GithubURL:   "https://github.com/test/repo",
		TmuxSession: "mc-test-repo",
		Agents:      make(map[string]state.Agent),
	}
	st.AddRepo("test-repo", repo)
	st.CurrentRepo = "test-repo"
	st.Save()

	t.Run("clears current repo", func(t *testing.T) {
		err := cli.clearCurrentRepo([]string{})
		if err != nil {
			t.Fatalf("clearCurrentRepo() error = %v", err)
		}

		// Verify it was cleared
		st, _ := state.Load(d.GetPaths().StateFile)
		if st.CurrentRepo != "" {
			t.Errorf("CurrentRepo = %v, want empty string", st.CurrentRepo)
		}
	})
}

// TestRemoveDirectoryIfExists tests the removeDirectoryIfExists helper
func TestRemoveDirectoryIfExists(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-remove-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	t.Run("removes existing directory", func(t *testing.T) {
		testDir := filepath.Join(tmpDir, "test-dir")
		if err := os.Mkdir(testDir, 0755); err != nil {
			t.Fatalf("Failed to create test dir: %v", err)
		}

		removeDirectoryIfExists(testDir, "test directory")

		if _, err := os.Stat(testDir); !os.IsNotExist(err) {
			t.Error("Directory should be removed")
		}
	})

	t.Run("handles nonexistent directory gracefully", func(t *testing.T) {
		nonexistentDir := filepath.Join(tmpDir, "nonexistent")
		// Should not panic or error
		removeDirectoryIfExists(nonexistentDir, "nonexistent directory")
	})
}

// TestCLIAddWorkspace tests the addWorkspace command
func TestCLIAddWorkspace(t *testing.T) {
	cli, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Create and add a test repo
	repoName := "workspace-test-repo"

	t.Run("returns error for invalid workspace name", func(t *testing.T) {
		err := cli.addWorkspace([]string{"invalid name with spaces", "--repo", repoName})
		if err == nil {
			t.Error("addWorkspace() should return error for invalid name")
		}
	})

	t.Run("returns error when no name provided", func(t *testing.T) {
		err := cli.addWorkspace([]string{"--repo", repoName})
		if err == nil {
			t.Error("addWorkspace() should return error when no name provided")
		}
	})

	// Note: Full workspace creation requires tmux and proper daemon state,
	// which is tested in integration tests. Here we test the validation logic.
}

// TestCLIRemoveWorkspace tests the removeWorkspace command
func TestCLIRemoveWorkspace(t *testing.T) {
	cli, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	repoName := "remove-workspace-test"

	t.Run("returns error when no name provided", func(t *testing.T) {
		err := cli.removeWorkspace([]string{"--repo", repoName})
		if err == nil {
			t.Error("removeWorkspace() should return error when no name provided")
		}
	})

	t.Run("returns error for nonexistent workspace", func(t *testing.T) {
		err := cli.removeWorkspace([]string{"nonexistent-workspace", "--repo", repoName})
		if err == nil {
			t.Error("removeWorkspace() should return error for nonexistent workspace")
		}
	})

	// Note: Full workspace removal requires tmux and proper daemon state,
	// which is tested in integration tests. Here we test the validation logic.
}

// TestCLIShowHistory tests the showHistory command
func TestCLIShowHistory(t *testing.T) {
	cli, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	repoName := "history-test-repo"

	t.Run("returns error for invalid status filter", func(t *testing.T) {
		err := cli.showHistory([]string{"--repo", repoName, "--status", "invalid"})
		if err == nil {
			t.Error("showHistory() should return error for invalid status filter")
		}
	})

	// Note: Full history display requires daemon state with task history,
	// which is tested in integration tests. Here we test the validation logic.
}

// TestCLIGetPRStatusForBranch tests the getPRStatusForBranch helper
func TestCLIGetPRStatusForBranch(t *testing.T) {
	cli, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Create a test repo
	repoPath := cli.paths.RepoDir("pr-status-test")
	setupTestRepo(t, repoPath)

	t.Run("returns existing PR URL when provided", func(t *testing.T) {
		status, link := cli.getPRStatusForBranch(repoPath, "test-branch", "https://github.com/test/repo/pull/123")
		if status != "unknown" {
			t.Errorf("status = %v, want unknown", status)
		}
		if link != "#123" {
			t.Errorf("link = %v, want #123", link)
		}
	})

	t.Run("returns no-pr when branch is empty", func(t *testing.T) {
		status, link := cli.getPRStatusForBranch(repoPath, "", "")
		if status != "no-pr" {
			t.Errorf("status = %v, want no-pr", status)
		}
		if link != "" {
			t.Errorf("link = %v, want empty", link)
		}
	})

	t.Run("handles branch with no PR", func(t *testing.T) {
		status, link := cli.getPRStatusForBranch(repoPath, "nonexistent-branch", "")
		if status != "no-pr" {
			t.Errorf("status = %v, want no-pr", status)
		}
		if link != "" {
			t.Errorf("link = %v, want empty", link)
		}
	})
}
