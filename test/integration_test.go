package test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/dlorenc/multiclaude/internal/cli"
	"github.com/dlorenc/multiclaude/internal/daemon"
	"github.com/dlorenc/multiclaude/internal/socket"
	"github.com/dlorenc/multiclaude/internal/state"
	"github.com/dlorenc/multiclaude/internal/worktree"
	"github.com/dlorenc/multiclaude/pkg/config"
	"github.com/dlorenc/multiclaude/pkg/tmux"
)

// setupIntegrationTest creates a complete test environment with daemon, paths, and tmux.
// Returns CLI, daemon, tmux session name, and cleanup function.
func setupIntegrationTest(t *testing.T, repoName string) (*cli.CLI, *daemon.Daemon, string, func()) {
	t.Helper()

	// Set test mode to skip Claude startup
	os.Setenv("MULTICLAUDE_TEST_MODE", "1")

	tmuxClient := tmux.NewClient()
	if !tmuxClient.IsTmuxAvailable() {
		t.Fatal("tmux is required for this test but not available")
	}

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "integration-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Resolve symlinks (macOS /tmp -> /private/tmp)
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
		ArchiveDir:      filepath.Join(tmpDir, "archive"),
	}

	if err := paths.EnsureDirectories(); err != nil {
		t.Fatalf("Failed to create directories: %v", err)
	}

	// Create prompts directory
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

	// Create tmux session
	tmuxSession := "mc-" + repoName
	if err := tmuxClient.CreateSession(context.Background(), tmuxSession, true); err != nil {
		d.Stop()
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create tmux session: %v", err)
	}

	// Create CLI with test paths
	c := cli.NewWithPaths(paths)

	cleanup := func() {
		tmuxClient.KillSession(context.Background(), tmuxSession)
		d.Stop()
		os.RemoveAll(tmpDir)
		os.Unsetenv("MULTICLAUDE_TEST_MODE")
	}

	return c, d, tmuxSession, cleanup
}

// setupTestGitRepo creates a test git repository with initial commit
func setupTestGitRepo(t *testing.T, repoPath string) {
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

// TestWorkerCreationIntegration tests the full worker creation flow via CLI.
// This would have caught the provider bug in PR #132.
func TestWorkerCreationIntegration(t *testing.T) {
	repoName := "worker-creation-test"
	c, d, tmuxSession, cleanup := setupIntegrationTest(t, repoName)
	defer cleanup()

	paths := d.GetPaths()
	repoPath := paths.RepoDir(repoName)

	// Setup test git repo
	setupTestGitRepo(t, repoPath)

	// Register repo with daemon (simulating what init would do)
	repo := &state.Repository{
		GithubURL:        "https://github.com/test/repo",
		TmuxSession:      tmuxSession,
		Agents:           make(map[string]state.Agent),
		MergeQueueConfig: state.DefaultMergeQueueConfig(),
	}
	if err := d.GetState().AddRepo(repoName, repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Change to repo directory (CLI needs to resolve repo from cwd or --repo flag)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	if err := os.Chdir(repoPath); err != nil {
		t.Fatalf("Failed to change to repo dir: %v", err)
	}

	// Create worker via CLI with explicit name and repo flag
	workerName := "test-worker"
	taskDescription := "Implement feature X for testing"
	err := c.Execute([]string{"work", taskDescription, "--name", workerName, "--repo", repoName})
	if err != nil {
		t.Fatalf("Worker creation failed: %v", err)
	}

	// Verification 1: Worker appears in state
	agent, exists := d.GetState().GetAgent(repoName, workerName)
	if !exists {
		t.Error("Worker should exist in state after creation")
	}
	if agent.Type != state.AgentTypeWorker {
		t.Errorf("Agent type = %s, want worker", agent.Type)
	}
	if agent.Task != taskDescription {
		t.Errorf("Agent task = %q, want %q", agent.Task, taskDescription)
	}
	if agent.WorktreePath == "" {
		t.Error("Agent worktree path should not be empty")
	}
	if agent.TmuxWindow != workerName {
		t.Errorf("Agent tmux window = %s, want %s", agent.TmuxWindow, workerName)
	}

	// Verification 2: Worktree exists on disk
	wtPath := paths.AgentWorktree(repoName, workerName)
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		t.Error("Worker worktree should exist on disk")
	}

	// Verify worktree is a valid git worktree
	wt := worktree.NewManager(repoPath)
	wtExists, err := wt.Exists(wtPath)
	if err != nil {
		t.Fatalf("Failed to check worktree existence: %v", err)
	}
	if !wtExists {
		t.Error("Worktree should be recognized as a git worktree")
	}

	// Verification 3: Tmux window was created
	tmuxClient := tmux.NewClient()
	hasWindow, err := tmuxClient.HasWindow(context.Background(), tmuxSession, workerName)
	if err != nil {
		t.Fatalf("Failed to check tmux window: %v", err)
	}
	if !hasWindow {
		t.Error("Worker tmux window should exist")
	}

	// Verification 4: Worker appears in list command via daemon
	client := socket.NewClient(paths.DaemonSock)
	resp, err := client.Send(socket.Request{
		Command: "list_agents",
		Args: map[string]interface{}{
			"repo": repoName,
		},
	})
	if err != nil {
		t.Fatalf("Failed to list agents: %v", err)
	}
	if !resp.Success {
		t.Fatalf("list_agents failed: %s", resp.Error)
	}

	agents, ok := resp.Data.([]interface{})
	if !ok {
		t.Fatal("list_agents data should be an array")
	}

	foundWorker := false
	for _, a := range agents {
		if agentMap, ok := a.(map[string]interface{}); ok {
			if name, _ := agentMap["name"].(string); name == workerName {
				foundWorker = true
				if agentType, _ := agentMap["type"].(string); agentType != "worker" {
					t.Errorf("Listed agent type = %s, want worker", agentType)
				}
				break
			}
		}
	}
	if !foundWorker {
		t.Error("Worker should appear in agent list")
	}
}

// TestRepoInitializationIntegration tests the full repo initialization flow.
// Uses a local git repo instead of cloning from GitHub.
func TestRepoInitializationIntegration(t *testing.T) {
	// Set test mode to skip Claude startup
	os.Setenv("MULTICLAUDE_TEST_MODE", "1")
	defer os.Unsetenv("MULTICLAUDE_TEST_MODE")

	tmuxClient := tmux.NewClient()
	if !tmuxClient.IsTmuxAvailable() {
		t.Fatal("tmux is required for this test but not available")
	}

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "repo-init-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Resolve symlinks
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
		ArchiveDir:      filepath.Join(tmpDir, "archive"),
	}

	if err := paths.EnsureDirectories(); err != nil {
		t.Fatalf("Failed to create directories: %v", err)
	}

	// Create prompts directory
	if err := os.MkdirAll(filepath.Join(tmpDir, "prompts"), 0755); err != nil {
		t.Fatalf("Failed to create prompts dir: %v", err)
	}

	// Create a "bare" git repo to serve as the remote
	remoteRepoPath := filepath.Join(tmpDir, "remote-repo.git")
	cmds := [][]string{
		{"git", "init", "--bare", remoteRepoPath},
	}
	for _, cmdArgs := range cmds {
		cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to run %v: %v", cmdArgs, err)
		}
	}

	// Create a working repo and push to bare repo
	sourceRepo := filepath.Join(tmpDir, "source-repo")
	setupTestGitRepo(t, sourceRepo)

	// Add remote and push
	pushCmds := [][]string{
		{"git", "remote", "add", "origin", remoteRepoPath},
		{"git", "branch", "-M", "main"},
		{"git", "push", "-u", "origin", "main"},
	}
	for _, cmdArgs := range pushCmds {
		cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
		cmd.Dir = sourceRepo
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to run %v: %v", cmdArgs, err)
		}
	}

	// Update bare repo HEAD to point to main (git init --bare defaults to master/main based on config)
	cmd := exec.Command("git", "symbolic-ref", "HEAD", "refs/heads/main")
	cmd.Dir = remoteRepoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to update bare repo HEAD: %v", err)
	}

	// Create and start daemon
	d, err := daemon.New(paths)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}
	if err := d.Start(); err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}
	defer d.Stop()

	time.Sleep(100 * time.Millisecond)

	// Create CLI
	c := cli.NewWithPaths(paths)

	// Initialize repo using local bare repo as "GitHub URL"
	repoName := "init-test-repo"
	err = c.Execute([]string{"init", remoteRepoPath, repoName})
	if err != nil {
		t.Fatalf("Repo initialization failed: %v", err)
	}

	// Clean up tmux session created by init
	tmuxSession := "mc-" + repoName
	defer tmuxClient.KillSession(context.Background(), tmuxSession)

	// Verification 1: Repo is registered with daemon
	repo, exists := d.GetState().GetRepo(repoName)
	if !exists {
		t.Fatal("Repo should be registered after init")
	}
	if repo.TmuxSession != tmuxSession {
		t.Errorf("Repo tmux session = %s, want %s", repo.TmuxSession, tmuxSession)
	}
	if !repo.MergeQueueConfig.Enabled {
		t.Error("Merge queue should be enabled by default")
	}

	// Verification 2: Repo directory exists
	repoPath := paths.RepoDir(repoName)
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		t.Error("Repo directory should exist")
	}

	// Verification 3: Tmux session was created
	hasSession, err := tmuxClient.HasSession(context.Background(), tmuxSession)
	if err != nil {
		t.Fatalf("Failed to check tmux session: %v", err)
	}
	if !hasSession {
		t.Error("Tmux session should exist")
	}

	// Verification 4: Supervisor agent was created
	supervisor, exists := d.GetState().GetAgent(repoName, "supervisor")
	if !exists {
		t.Error("Supervisor agent should exist")
	}
	if supervisor.Type != state.AgentTypeSupervisor {
		t.Errorf("Supervisor type = %s, want supervisor", supervisor.Type)
	}

	// Verification 5: Merge-queue agent was created (since enabled by default)
	mq, exists := d.GetState().GetAgent(repoName, "merge-queue")
	if !exists {
		t.Error("Merge-queue agent should exist when enabled")
	}
	if mq.Type != state.AgentTypeMergeQueue {
		t.Errorf("Merge-queue type = %s, want merge-queue", mq.Type)
	}

	// Verification 6: Check that supervisor and merge-queue windows exist
	for _, windowName := range []string{"supervisor", "merge-queue"} {
		hasWindow, err := tmuxClient.HasWindow(context.Background(), tmuxSession, windowName)
		if err != nil {
			t.Fatalf("Failed to check %s window: %v", windowName, err)
		}
		if !hasWindow {
			t.Errorf("%s tmux window should exist", windowName)
		}
	}
}

// TestRepoInitializationWithMergeQueueDisabled tests init with --no-merge-queue flag
func TestRepoInitializationWithMergeQueueDisabled(t *testing.T) {
	os.Setenv("MULTICLAUDE_TEST_MODE", "1")
	defer os.Unsetenv("MULTICLAUDE_TEST_MODE")

	tmuxClient := tmux.NewClient()
	if !tmuxClient.IsTmuxAvailable() {
		t.Fatal("tmux is required for this test but not available")
	}

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "repo-init-nomq-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tmpDir, _ = filepath.EvalSymlinks(tmpDir)

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

	if err := paths.EnsureDirectories(); err != nil {
		t.Fatalf("Failed to create directories: %v", err)
	}
	os.MkdirAll(filepath.Join(tmpDir, "prompts"), 0755)

	// Create bare repo for cloning
	remoteRepoPath := filepath.Join(tmpDir, "remote-repo.git")
	exec.Command("git", "init", "--bare", remoteRepoPath).Run()

	sourceRepo := filepath.Join(tmpDir, "source-repo")
	setupTestGitRepo(t, sourceRepo)
	cmd := exec.Command("git", "remote", "add", "origin", remoteRepoPath)
	cmd.Dir = sourceRepo
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to add remote: %v", err)
	}
	cmd = exec.Command("git", "branch", "-M", "main")
	cmd.Dir = sourceRepo
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to rename branch: %v", err)
	}
	cmd = exec.Command("git", "push", "-u", "origin", "main")
	cmd.Dir = sourceRepo
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to push: %v", err)
	}

	// Update bare repo HEAD to point to main (git init --bare defaults to master/main based on config)
	cmd = exec.Command("git", "symbolic-ref", "HEAD", "refs/heads/main")
	cmd.Dir = remoteRepoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to update bare repo HEAD: %v", err)
	}

	d, _ := daemon.New(paths)
	d.Start()
	defer d.Stop()
	time.Sleep(100 * time.Millisecond)

	c := cli.NewWithPaths(paths)

	repoName := "nomq-repo"
	err = c.Execute([]string{"init", remoteRepoPath, repoName, "--no-merge-queue"})
	if err != nil {
		t.Fatalf("Repo initialization failed: %v", err)
	}

	tmuxSession := "mc-" + repoName
	defer tmuxClient.KillSession(context.Background(), tmuxSession)

	// Verify merge queue is disabled
	repo, _ := d.GetState().GetRepo(repoName)
	if repo.MergeQueueConfig.Enabled {
		t.Error("Merge queue should be disabled with --no-merge-queue")
	}

	// Verify merge-queue agent was NOT created
	_, mqExists := d.GetState().GetAgent(repoName, "merge-queue")
	if mqExists {
		t.Error("Merge-queue agent should NOT exist when disabled")
	}

	// Supervisor should still exist
	_, supExists := d.GetState().GetAgent(repoName, "supervisor")
	if !supExists {
		t.Error("Supervisor should still exist even with --no-merge-queue")
	}
}

// TestDaemonCommunicationRoundTrip tests that CLI->daemon->CLI communication works correctly.
// This tests the exact flow that was broken by the provider bug.
func TestDaemonCommunicationRoundTrip(t *testing.T) {
	repoName := "roundtrip-test"
	_, d, tmuxSession, cleanup := setupIntegrationTest(t, repoName)
	defer cleanup()

	paths := d.GetPaths()
	client := socket.NewClient(paths.DaemonSock)

	// Test 1: add_repo round-trip
	t.Run("AddRepoRoundTrip", func(t *testing.T) {
		resp, err := client.Send(socket.Request{
			Command: "add_repo",
			Args: map[string]interface{}{
				"name":          repoName,
				"github_url":    "https://github.com/test/repo",
				"tmux_session":  tmuxSession,
				"mq_enabled":    true,
				"mq_track_mode": "all",
			},
		})
		if err != nil {
			t.Fatalf("add_repo request failed: %v", err)
		}
		if !resp.Success {
			t.Fatalf("add_repo should succeed, got error: %s", resp.Error)
		}

		// Verify via get_repo_config (returns merge queue config only)
		resp, err = client.Send(socket.Request{
			Command: "get_repo_config",
			Args: map[string]interface{}{
				"name": repoName,
			},
		})
		if err != nil {
			t.Fatalf("get_repo_config request failed: %v", err)
		}
		if !resp.Success {
			t.Fatalf("get_repo_config should succeed, got error: %s", resp.Error)
		}

		configData, ok := resp.Data.(map[string]interface{})
		if !ok {
			t.Fatal("get_repo_config should return a map")
		}
		// get_repo_config returns merge queue config, not github_url
		if configData["mq_enabled"] != true {
			t.Errorf("mq_enabled = %v, want true", configData["mq_enabled"])
		}
		if configData["mq_track_mode"] != "all" {
			t.Errorf("mq_track_mode = %v, want 'all'", configData["mq_track_mode"])
		}

		// Verify repo exists in state via list_repos
		resp, err = client.Send(socket.Request{
			Command: "list_repos",
		})
		if err != nil {
			t.Fatalf("list_repos request failed: %v", err)
		}
		if !resp.Success {
			t.Fatalf("list_repos should succeed, got error: %s", resp.Error)
		}

		repos, ok := resp.Data.([]interface{})
		if !ok {
			t.Fatal("list_repos should return an array")
		}
		foundRepo := false
		for _, r := range repos {
			if r == repoName {
				foundRepo = true
				break
			}
		}
		if !foundRepo {
			t.Errorf("Repo %s should appear in list_repos", repoName)
		}
	})

	// Test 2: add_agent round-trip
	t.Run("AddAgentRoundTrip", func(t *testing.T) {
		resp, err := client.Send(socket.Request{
			Command: "add_agent",
			Args: map[string]interface{}{
				"repo":          repoName,
				"agent":         "test-worker",
				"type":          "worker",
				"worktree_path": "/tmp/test-worktree",
				"tmux_window":   "test-worker",
				"task":          "Test task for round-trip",
			},
		})
		if err != nil {
			t.Fatalf("add_agent request failed: %v", err)
		}
		if !resp.Success {
			t.Fatalf("add_agent should succeed, got error: %s", resp.Error)
		}

		// Verify via list_agents
		resp, err = client.Send(socket.Request{
			Command: "list_agents",
			Args: map[string]interface{}{
				"repo": repoName,
			},
		})
		if err != nil {
			t.Fatalf("list_agents request failed: %v", err)
		}
		if !resp.Success {
			t.Fatalf("list_agents should succeed, got error: %s", resp.Error)
		}

		agents, ok := resp.Data.([]interface{})
		if !ok {
			t.Fatal("list_agents should return an array")
		}

		foundAgent := false
		for _, a := range agents {
			if agentMap, ok := a.(map[string]interface{}); ok {
				if name, _ := agentMap["name"].(string); name == "test-worker" {
					foundAgent = true
					if task, _ := agentMap["task"].(string); task != "Test task for round-trip" {
						t.Errorf("task = %q, want 'Test task for round-trip'", task)
					}
					break
				}
			}
		}
		if !foundAgent {
			t.Error("Agent should appear in list_agents response")
		}
	})

	// Test 3: complete_agent round-trip
	t.Run("CompleteAgentRoundTrip", func(t *testing.T) {
		resp, err := client.Send(socket.Request{
			Command: "complete_agent",
			Args: map[string]interface{}{
				"repo":  repoName,
				"agent": "test-worker",
			},
		})
		if err != nil {
			t.Fatalf("complete_agent request failed: %v", err)
		}
		if !resp.Success {
			t.Fatalf("complete_agent should succeed, got error: %s", resp.Error)
		}

		// Verify agent is marked for cleanup
		agent, exists := d.GetState().GetAgent(repoName, "test-worker")
		if !exists {
			t.Fatal("Agent should still exist after complete")
		}
		if !agent.ReadyForCleanup {
			t.Error("Agent should be marked for cleanup after complete")
		}
	})

	// Test 4: update_repo_config round-trip
	t.Run("UpdateRepoConfigRoundTrip", func(t *testing.T) {
		resp, err := client.Send(socket.Request{
			Command: "update_repo_config",
			Args: map[string]interface{}{
				"name":          repoName,
				"mq_enabled":    false,
				"mq_track_mode": "author",
			},
		})
		if err != nil {
			t.Fatalf("update_repo_config request failed: %v", err)
		}
		if !resp.Success {
			t.Fatalf("update_repo_config should succeed, got error: %s", resp.Error)
		}

		// Verify via state
		repo, _ := d.GetState().GetRepo(repoName)
		if repo.MergeQueueConfig.Enabled {
			t.Error("Merge queue should be disabled after update")
		}
		if repo.MergeQueueConfig.TrackMode != state.TrackModeAuthor {
			t.Errorf("Track mode = %s, want author", repo.MergeQueueConfig.TrackMode)
		}
	})

	// Test 5: set_current_repo and get_current_repo round-trip
	t.Run("CurrentRepoRoundTrip", func(t *testing.T) {
		resp, err := client.Send(socket.Request{
			Command: "set_current_repo",
			Args: map[string]interface{}{
				"name": repoName,
			},
		})
		if err != nil {
			t.Fatalf("set_current_repo request failed: %v", err)
		}
		if !resp.Success {
			t.Fatalf("set_current_repo should succeed, got error: %s", resp.Error)
		}

		resp, err = client.Send(socket.Request{
			Command: "get_current_repo",
		})
		if err != nil {
			t.Fatalf("get_current_repo request failed: %v", err)
		}
		if !resp.Success {
			t.Fatalf("get_current_repo should succeed, got error: %s", resp.Error)
		}

		if resp.Data != repoName {
			t.Errorf("current repo = %v, want %s", resp.Data, repoName)
		}
	})
}

// TestWorkerCreationWithCustomBranch tests creating a worker from a specific branch
func TestWorkerCreationWithCustomBranch(t *testing.T) {
	repoName := "custom-branch-test"
	c, d, _, cleanup := setupIntegrationTest(t, repoName)
	defer cleanup()

	paths := d.GetPaths()
	repoPath := paths.RepoDir(repoName)

	// Setup test git repo with custom branch
	setupTestGitRepo(t, repoPath)

	// Create a feature branch
	cmds := [][]string{
		{"git", "checkout", "-b", "feature-branch"},
		{"git", "commit", "--allow-empty", "-m", "Feature commit"},
		{"git", "checkout", "master"},
	}
	for _, cmdArgs := range cmds {
		cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
		cmd.Dir = repoPath
		cmd.Run()
	}

	// Register repo
	repo := &state.Repository{
		GithubURL:        "https://github.com/test/repo",
		TmuxSession:      "mc-" + repoName,
		Agents:           make(map[string]state.Agent),
		MergeQueueConfig: state.DefaultMergeQueueConfig(),
	}
	d.GetState().AddRepo(repoName, repo)

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(repoPath)

	// Create worker from feature branch
	workerName := "branch-worker"
	err := c.Execute([]string{"work", "Work on feature", "--name", workerName, "--repo", repoName, "--branch", "feature-branch"})
	if err != nil {
		t.Fatalf("Worker creation failed: %v", err)
	}

	// Verify worker exists
	_, exists := d.GetState().GetAgent(repoName, workerName)
	if !exists {
		t.Error("Worker should exist in state")
	}

	// Verify worktree was created
	wtPath := paths.AgentWorktree(repoName, workerName)
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		t.Error("Worktree should exist on disk")
	}
}

// TestMultipleWorkersInSameRepo tests creating multiple workers in the same repo
func TestMultipleWorkersInSameRepo(t *testing.T) {
	repoName := "multi-worker-test"
	c, d, _, cleanup := setupIntegrationTest(t, repoName)
	defer cleanup()

	paths := d.GetPaths()
	repoPath := paths.RepoDir(repoName)

	setupTestGitRepo(t, repoPath)

	repo := &state.Repository{
		GithubURL:        "https://github.com/test/repo",
		TmuxSession:      "mc-" + repoName,
		Agents:           make(map[string]state.Agent),
		MergeQueueConfig: state.DefaultMergeQueueConfig(),
	}
	d.GetState().AddRepo(repoName, repo)

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(repoPath)

	// Create multiple workers
	workers := []struct {
		name string
		task string
	}{
		{"worker-1", "Task one"},
		{"worker-2", "Task two"},
		{"worker-3", "Task three"},
	}

	for _, w := range workers {
		err := c.Execute([]string{"work", w.task, "--name", w.name, "--repo", repoName})
		if err != nil {
			t.Fatalf("Failed to create worker %s: %v", w.name, err)
		}
	}

	// Verify all workers exist
	for _, w := range workers {
		agent, exists := d.GetState().GetAgent(repoName, w.name)
		if !exists {
			t.Errorf("Worker %s should exist", w.name)
			continue
		}
		if agent.Task != w.task {
			t.Errorf("Worker %s task = %q, want %q", w.name, agent.Task, w.task)
		}

		// Verify worktree exists
		wtPath := paths.AgentWorktree(repoName, w.name)
		if _, err := os.Stat(wtPath); os.IsNotExist(err) {
			t.Errorf("Worker %s worktree should exist", w.name)
		}
	}

	// Verify list_agents returns all workers
	client := socket.NewClient(paths.DaemonSock)
	resp, _ := client.Send(socket.Request{
		Command: "list_agents",
		Args: map[string]interface{}{
			"repo": repoName,
		},
	})

	agents, _ := resp.Data.([]interface{})
	if len(agents) != 3 {
		t.Errorf("Expected 3 workers, got %d", len(agents))
	}
}

// TestWorkerRemovalCleanup tests that worker removal cleans up properly
func TestWorkerRemovalCleanup(t *testing.T) {
	repoName := "removal-test"
	c, d, tmuxSession, cleanup := setupIntegrationTest(t, repoName)
	defer cleanup()

	paths := d.GetPaths()
	repoPath := paths.RepoDir(repoName)

	setupTestGitRepo(t, repoPath)

	repo := &state.Repository{
		GithubURL:        "https://github.com/test/repo",
		TmuxSession:      tmuxSession,
		Agents:           make(map[string]state.Agent),
		MergeQueueConfig: state.DefaultMergeQueueConfig(),
	}
	d.GetState().AddRepo(repoName, repo)

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(repoPath)

	// Create a worker
	workerName := "removable-worker"
	c.Execute([]string{"work", "Task to remove", "--name", workerName, "--repo", repoName})

	// Verify worker exists
	_, exists := d.GetState().GetAgent(repoName, workerName)
	if !exists {
		t.Fatal("Worker should exist before removal")
	}

	wtPath := paths.AgentWorktree(repoName, workerName)
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		t.Fatal("Worktree should exist before removal")
	}

	// Remove worker
	err := c.Execute([]string{"work", "rm", workerName, "--repo", repoName})
	if err != nil {
		t.Fatalf("Worker removal failed: %v", err)
	}

	// Verify worker is removed from state
	_, exists = d.GetState().GetAgent(repoName, workerName)
	if exists {
		t.Error("Worker should not exist after removal")
	}

	// Note: Worktree cleanup happens asynchronously via daemon health check
	// So we don't verify worktree is gone here
}
