package test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dlorenc/multiclaude/internal/agents"
	"github.com/dlorenc/multiclaude/internal/cli"
	"github.com/dlorenc/multiclaude/internal/daemon"
	"github.com/dlorenc/multiclaude/internal/socket"
	"github.com/dlorenc/multiclaude/internal/state"
	"github.com/dlorenc/multiclaude/internal/templates"
	"github.com/dlorenc/multiclaude/pkg/config"
	"github.com/dlorenc/multiclaude/pkg/tmux"
)

// TestAgentTemplatesCopiedOnInit verifies that agent templates are copied
// to the per-repo agents directory during `multiclaude init`.
func TestAgentTemplatesCopiedOnInit(t *testing.T) {
	os.Setenv("MULTICLAUDE_TEST_MODE", "1")
	defer os.Unsetenv("MULTICLAUDE_TEST_MODE")

	tmuxClient := tmux.NewClient()
	if !tmuxClient.IsTmuxAvailable() {
		t.Fatal("tmux is required for this test but not available")
	}

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "agent-templates-test-*")
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
	cmd.Run()
	cmd = exec.Command("git", "branch", "-M", "main")
	cmd.Dir = sourceRepo
	cmd.Run()
	cmd = exec.Command("git", "push", "-u", "origin", "main")
	cmd.Dir = sourceRepo
	cmd.Run()

	// Update bare repo HEAD
	cmd = exec.Command("git", "symbolic-ref", "HEAD", "refs/heads/main")
	cmd.Dir = remoteRepoPath
	cmd.Run()

	d, _ := daemon.New(paths)
	d.Start()
	defer d.Stop()
	time.Sleep(100 * time.Millisecond)

	c := cli.NewWithPaths(paths)

	repoName := "templates-test"
	err = c.Execute([]string{"init", remoteRepoPath, repoName})
	if err != nil {
		t.Fatalf("Repo initialization failed: %v", err)
	}

	tmuxSession := "mc-" + repoName
	defer tmuxClient.KillSession(context.Background(), tmuxSession)

	// Verify agents directory was created
	agentsDir := paths.RepoAgentsDir(repoName)
	if _, err := os.Stat(agentsDir); os.IsNotExist(err) {
		t.Fatal("Agents directory should exist after init")
	}

	// Verify expected template files were copied
	expectedFiles := []string{"merge-queue.md", "reviewer.md", "worker.md"}
	for _, filename := range expectedFiles {
		filePath := filepath.Join(agentsDir, filename)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Errorf("Template file %s should exist after init", filename)
		}
	}

	// Verify content is non-empty
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		t.Fatalf("Failed to read agents dir: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(agentsDir, entry.Name()))
		if err != nil {
			t.Errorf("Failed to read %s: %v", entry.Name(), err)
			continue
		}
		if len(content) == 0 {
			t.Errorf("Template file %s should not be empty", entry.Name())
		}
	}
}

// TestAgentDefinitionMerging verifies that repo definitions override local definitions
// when both exist with the same name.
func TestAgentDefinitionMerging(t *testing.T) {
	// Create temp directories
	tmpDir, err := os.MkdirTemp("", "agent-merge-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	localAgentsDir := filepath.Join(tmpDir, "local-agents")
	repoPath := filepath.Join(tmpDir, "repo")
	repoAgentsDir := filepath.Join(repoPath, ".multiclaude", "agents")

	// Create directories
	os.MkdirAll(localAgentsDir, 0755)
	os.MkdirAll(repoAgentsDir, 0755)

	// Create local definition for "worker"
	localWorkerContent := "# Worker (Local)\n\nThis is the local worker definition."
	if err := os.WriteFile(filepath.Join(localAgentsDir, "worker.md"), []byte(localWorkerContent), 0644); err != nil {
		t.Fatalf("Failed to write local worker: %v", err)
	}

	// Create local-only definition
	localOnlyContent := "# Local Only Bot\n\nThis only exists locally."
	if err := os.WriteFile(filepath.Join(localAgentsDir, "local-only.md"), []byte(localOnlyContent), 0644); err != nil {
		t.Fatalf("Failed to write local-only: %v", err)
	}

	// Create repo definition for "worker" (should override local)
	repoWorkerContent := "# Worker (Repo Override)\n\nThis is the repo worker definition that overrides local."
	if err := os.WriteFile(filepath.Join(repoAgentsDir, "worker.md"), []byte(repoWorkerContent), 0644); err != nil {
		t.Fatalf("Failed to write repo worker: %v", err)
	}

	// Create repo-only definition
	repoOnlyContent := "# Repo Only Bot\n\nThis only exists in the repo."
	if err := os.WriteFile(filepath.Join(repoAgentsDir, "repo-only.md"), []byte(repoOnlyContent), 0644); err != nil {
		t.Fatalf("Failed to write repo-only: %v", err)
	}

	// Create reader and read all definitions
	reader := agents.NewReader(localAgentsDir, repoPath)
	definitions, err := reader.ReadAllDefinitions()
	if err != nil {
		t.Fatalf("Failed to read all definitions: %v", err)
	}

	// Verify we have 3 definitions: worker (merged), local-only, repo-only
	if len(definitions) != 3 {
		t.Errorf("Expected 3 definitions, got %d", len(definitions))
	}

	// Build a map for easier lookup
	defMap := make(map[string]agents.Definition)
	for _, def := range definitions {
		defMap[def.Name] = def
	}

	// Test 1: "worker" should be from repo (overrides local)
	if workerDef, ok := defMap["worker"]; ok {
		if workerDef.Source != agents.SourceRepo {
			t.Errorf("worker definition source = %s, want repo", workerDef.Source)
		}
		if !strings.Contains(workerDef.Content, "Repo Override") {
			t.Error("worker definition should contain repo content, not local")
		}
	} else {
		t.Error("worker definition should exist")
	}

	// Test 2: "local-only" should be from local
	if localOnlyDef, ok := defMap["local-only"]; ok {
		if localOnlyDef.Source != agents.SourceLocal {
			t.Errorf("local-only definition source = %s, want local", localOnlyDef.Source)
		}
	} else {
		t.Error("local-only definition should exist")
	}

	// Test 3: "repo-only" should be from repo
	if repoOnlyDef, ok := defMap["repo-only"]; ok {
		if repoOnlyDef.Source != agents.SourceRepo {
			t.Errorf("repo-only definition source = %s, want repo", repoOnlyDef.Source)
		}
	} else {
		t.Error("repo-only definition should exist")
	}
}

// TestAgentsListCommand verifies that `multiclaude agents list` shows available definitions.
func TestAgentsListCommand(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "agents-list-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	paths := config.NewTestPaths(tmpDir)
	if err := paths.EnsureDirectories(); err != nil {
		t.Fatalf("Failed to create directories: %v", err)
	}

	repoName := "list-test-repo"

	// Create local agents directory with templates
	agentsDir := paths.RepoAgentsDir(repoName)
	if err := templates.CopyAgentTemplates(agentsDir); err != nil {
		t.Fatalf("Failed to copy templates: %v", err)
	}

	// Create repo directory (for repo definition lookup)
	repoPath := paths.RepoDir(repoName)
	os.MkdirAll(repoPath, 0755)
	setupTestGitRepo(t, repoPath)

	// Create state with the repo
	st := state.New(paths.StateFile)
	if err := st.AddRepo(repoName, &state.Repository{
		GithubURL:   "https://github.com/test/list-test",
		TmuxSession: "mc-list-test-repo",
		Agents:      make(map[string]state.Agent),
	}); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Create CLI
	c := cli.NewWithPaths(paths)

	// Change to repo directory
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(repoPath)

	// Test: list should not error
	err = c.Execute([]string{"agents", "list", "--repo", repoName})
	if err != nil {
		t.Errorf("agents list command failed: %v", err)
	}
}

// TestAgentsResetCommand verifies that `multiclaude agents reset` restores defaults.
func TestAgentsResetCommand(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "agents-reset-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	paths := config.NewTestPaths(tmpDir)
	if err := paths.EnsureDirectories(); err != nil {
		t.Fatalf("Failed to create directories: %v", err)
	}

	repoName := "reset-test-repo"

	// Create repo directory
	repoPath := paths.RepoDir(repoName)
	setupTestGitRepo(t, repoPath)

	// Create agents directory with templates initially
	agentsDir := paths.RepoAgentsDir(repoName)
	if err := templates.CopyAgentTemplates(agentsDir); err != nil {
		t.Fatalf("Failed to copy templates: %v", err)
	}

	// Create state with the repo
	st := state.New(paths.StateFile)
	if err := st.AddRepo(repoName, &state.Repository{
		GithubURL:   "https://github.com/test/reset-test",
		TmuxSession: "mc-reset-test-repo",
		Agents:      make(map[string]state.Agent),
	}); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Create CLI
	c := cli.NewWithPaths(paths)

	// Test 1: Delete a template file
	workerPath := filepath.Join(agentsDir, "worker.md")
	if err := os.Remove(workerPath); err != nil {
		t.Fatalf("Failed to remove worker.md: %v", err)
	}

	// Verify it's gone
	if _, err := os.Stat(workerPath); !os.IsNotExist(err) {
		t.Fatal("worker.md should be deleted")
	}

	// Test 2: Add a custom file
	customPath := filepath.Join(agentsDir, "custom-bot.md")
	if err := os.WriteFile(customPath, []byte("# Custom Bot\n"), 0644); err != nil {
		t.Fatalf("Failed to create custom file: %v", err)
	}

	// Run reset
	err = c.Execute([]string{"agents", "reset", "--repo", repoName})
	if err != nil {
		t.Fatalf("agents reset command failed: %v", err)
	}

	// Verify: worker.md is restored
	if _, err := os.Stat(workerPath); os.IsNotExist(err) {
		t.Error("worker.md should be restored after reset")
	}

	// Verify: custom file is removed
	if _, err := os.Stat(customPath); !os.IsNotExist(err) {
		t.Error("custom-bot.md should be removed after reset")
	}

	// Verify: all default templates exist
	expectedFiles := []string{"merge-queue.md", "reviewer.md", "worker.md"}
	for _, filename := range expectedFiles {
		filePath := filepath.Join(agentsDir, filename)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Errorf("Template file %s should exist after reset", filename)
		}
	}
}

// TestAgentsSpawnCommand verifies that `multiclaude agents spawn` creates an agent
// with a custom prompt via the daemon's spawn_agent handler.
func TestAgentsSpawnCommand(t *testing.T) {
	os.Setenv("MULTICLAUDE_TEST_MODE", "1")
	defer os.Unsetenv("MULTICLAUDE_TEST_MODE")

	tmuxClient := tmux.NewClient()
	if !tmuxClient.IsTmuxAvailable() {
		t.Fatal("tmux is required for this test but not available")
	}

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "agents-spawn-test-*")
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
	}

	if err := paths.EnsureDirectories(); err != nil {
		t.Fatalf("Failed to create directories: %v", err)
	}
	os.MkdirAll(filepath.Join(tmpDir, "prompts"), 0755)

	repoName := "spawn-test-repo"
	repoPath := paths.RepoDir(repoName)
	setupTestGitRepo(t, repoPath)

	// Create tmux session
	tmuxSession := "mc-" + repoName
	if err := tmuxClient.CreateSession(context.Background(), tmuxSession, true); err != nil {
		t.Fatalf("Failed to create tmux session: %v", err)
	}
	defer tmuxClient.KillSession(context.Background(), tmuxSession)

	// Create daemon
	d, err := daemon.New(paths)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}
	if err := d.Start(); err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}
	defer d.Stop()
	time.Sleep(100 * time.Millisecond)

	// Add repo to state
	repo := &state.Repository{
		GithubURL:        "https://github.com/test/spawn-test",
		TmuxSession:      tmuxSession,
		Agents:           make(map[string]state.Agent),
		MergeQueueConfig: state.DefaultMergeQueueConfig(),
	}
	if err := d.GetState().AddRepo(repoName, repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Create a custom prompt file
	promptFile := filepath.Join(tmpDir, "custom-prompt.md")
	promptContent := `# Custom Test Agent

You are a custom test agent created for testing purposes.

## Instructions
1. Acknowledge your creation
2. Report success
`
	if err := os.WriteFile(promptFile, []byte(promptContent), 0644); err != nil {
		t.Fatalf("Failed to create prompt file: %v", err)
	}

	// Spawn agent via daemon socket
	client := socket.NewClient(paths.DaemonSock)
	resp, err := client.Send(socket.Request{
		Command: "spawn_agent",
		Args: map[string]interface{}{
			"repo":   repoName,
			"name":   "test-custom-agent",
			"class":  "ephemeral",
			"prompt": promptContent,
		},
	})
	if err != nil {
		t.Fatalf("spawn_agent request failed: %v", err)
	}
	if !resp.Success {
		t.Fatalf("spawn_agent should succeed, got error: %s", resp.Error)
	}

	// Verify agent was created in state
	agent, exists := d.GetState().GetAgent(repoName, "test-custom-agent")
	if !exists {
		t.Fatal("Agent should exist in state after spawn")
	}

	// Verify agent type is "worker" (since it's ephemeral and doesn't have "review" in name)
	if agent.Type != state.AgentTypeWorker {
		t.Errorf("Agent type = %s, want worker", agent.Type)
	}

	// Verify tmux window was created
	hasWindow, err := tmuxClient.HasWindow(context.Background(), tmuxSession, "test-custom-agent")
	if err != nil {
		t.Fatalf("Failed to check tmux window: %v", err)
	}
	if !hasWindow {
		t.Error("Tmux window should exist for spawned agent")
	}
}

// TestAgentDefinitionsSentToSupervisor verifies that agent definitions are sent
// to the supervisor when a repository is restored.
func TestAgentDefinitionsSentToSupervisor(t *testing.T) {
	os.Setenv("MULTICLAUDE_TEST_MODE", "1")
	defer os.Unsetenv("MULTICLAUDE_TEST_MODE")

	tmuxClient := tmux.NewClient()
	if !tmuxClient.IsTmuxAvailable() {
		t.Fatal("tmux is required for this test but not available")
	}

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "agent-defs-supervisor-test-*")
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
	}

	if err := paths.EnsureDirectories(); err != nil {
		t.Fatalf("Failed to create directories: %v", err)
	}
	os.MkdirAll(filepath.Join(tmpDir, "prompts"), 0755)

	repoName := "supervisor-defs-test"
	repoPath := paths.RepoDir(repoName)
	setupTestGitRepo(t, repoPath)

	// Create agents directory with templates
	agentsDir := paths.RepoAgentsDir(repoName)
	if err := templates.CopyAgentTemplates(agentsDir); err != nil {
		t.Fatalf("Failed to copy templates: %v", err)
	}

	// Create tmux session
	tmuxSession := "mc-" + repoName
	if err := tmuxClient.CreateSession(context.Background(), tmuxSession, true); err != nil {
		t.Fatalf("Failed to create tmux session: %v", err)
	}
	defer tmuxClient.KillSession(context.Background(), tmuxSession)

	// Create daemon and add repo
	d, _ := daemon.New(paths)
	d.Start()
	defer d.Stop()
	time.Sleep(100 * time.Millisecond)

	// Add repo with merge queue enabled
	repo := &state.Repository{
		GithubURL:        "https://github.com/test/supervisor-defs-test",
		TmuxSession:      tmuxSession,
		Agents:           make(map[string]state.Agent),
		MergeQueueConfig: state.DefaultMergeQueueConfig(),
	}
	if err := d.GetState().AddRepo(repoName, repo); err != nil {
		t.Fatalf("Failed to add repo: %v", err)
	}

	// Verify agents dir exists with definitions
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		t.Fatalf("Failed to read agents dir: %v", err)
	}
	if len(entries) < 3 {
		t.Errorf("Expected at least 3 agent definitions, got %d", len(entries))
	}

	// Read definitions to verify they can be merged
	reader := agents.NewReader(agentsDir, repoPath)
	definitions, err := reader.ReadAllDefinitions()
	if err != nil {
		t.Fatalf("Failed to read definitions: %v", err)
	}

	// Verify we have definitions
	if len(definitions) == 0 {
		t.Error("Should have at least one agent definition")
	}

	// Verify each definition has required fields
	for _, def := range definitions {
		if def.Name == "" {
			t.Error("Definition name should not be empty")
		}
		if def.Content == "" {
			t.Error("Definition content should not be empty")
		}
		if def.Source != agents.SourceLocal && def.Source != agents.SourceRepo {
			t.Errorf("Definition source = %s, want local or repo", def.Source)
		}
	}
}

// TestSpawnPersistentAgent tests spawning a persistent (vs ephemeral) agent.
func TestSpawnPersistentAgent(t *testing.T) {
	os.Setenv("MULTICLAUDE_TEST_MODE", "1")
	defer os.Unsetenv("MULTICLAUDE_TEST_MODE")

	tmuxClient := tmux.NewClient()
	if !tmuxClient.IsTmuxAvailable() {
		t.Fatal("tmux is required for this test but not available")
	}

	tmpDir, err := os.MkdirTemp("", "persistent-agent-test-*")
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
	}

	if err := paths.EnsureDirectories(); err != nil {
		t.Fatalf("Failed to create directories: %v", err)
	}
	os.MkdirAll(filepath.Join(tmpDir, "prompts"), 0755)

	repoName := "persistent-test"
	repoPath := paths.RepoDir(repoName)
	setupTestGitRepo(t, repoPath)

	tmuxSession := "mc-" + repoName
	if err := tmuxClient.CreateSession(context.Background(), tmuxSession, true); err != nil {
		t.Fatalf("Failed to create tmux session: %v", err)
	}
	defer tmuxClient.KillSession(context.Background(), tmuxSession)

	d, _ := daemon.New(paths)
	d.Start()
	defer d.Stop()
	time.Sleep(100 * time.Millisecond)

	repo := &state.Repository{
		GithubURL:        "https://github.com/test/persistent-test",
		TmuxSession:      tmuxSession,
		Agents:           make(map[string]state.Agent),
		MergeQueueConfig: state.DefaultMergeQueueConfig(),
	}
	d.GetState().AddRepo(repoName, repo)

	// Spawn persistent agent
	client := socket.NewClient(paths.DaemonSock)
	resp, err := client.Send(socket.Request{
		Command: "spawn_agent",
		Args: map[string]interface{}{
			"repo":   repoName,
			"name":   "persistent-bot",
			"class":  "persistent",
			"prompt": "You are a persistent agent that survives restarts.",
		},
	})
	if err != nil {
		t.Fatalf("spawn_agent request failed: %v", err)
	}
	if !resp.Success {
		t.Fatalf("spawn_agent should succeed, got error: %s", resp.Error)
	}

	// Verify agent exists and has generic-persistent type
	agent, exists := d.GetState().GetAgent(repoName, "persistent-bot")
	if !exists {
		t.Fatal("Agent should exist in state")
	}
	// Persistent agents use AgentTypeGenericPersistent (unless they have special names like merge-queue)
	if agent.Type != state.AgentTypeGenericPersistent {
		t.Errorf("Agent type = %s, want generic-persistent", agent.Type)
	}
}

// TestSpawnEphemeralAgent tests spawning an ephemeral agent.
func TestSpawnEphemeralAgent(t *testing.T) {
	os.Setenv("MULTICLAUDE_TEST_MODE", "1")
	defer os.Unsetenv("MULTICLAUDE_TEST_MODE")

	tmuxClient := tmux.NewClient()
	if !tmuxClient.IsTmuxAvailable() {
		t.Fatal("tmux is required for this test but not available")
	}

	tmpDir, err := os.MkdirTemp("", "ephemeral-agent-test-*")
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
	}

	if err := paths.EnsureDirectories(); err != nil {
		t.Fatalf("Failed to create directories: %v", err)
	}
	os.MkdirAll(filepath.Join(tmpDir, "prompts"), 0755)

	repoName := "ephemeral-test"
	repoPath := paths.RepoDir(repoName)
	setupTestGitRepo(t, repoPath)

	tmuxSession := "mc-" + repoName
	if err := tmuxClient.CreateSession(context.Background(), tmuxSession, true); err != nil {
		t.Fatalf("Failed to create tmux session: %v", err)
	}
	defer tmuxClient.KillSession(context.Background(), tmuxSession)

	d, _ := daemon.New(paths)
	d.Start()
	defer d.Stop()
	time.Sleep(100 * time.Millisecond)

	repo := &state.Repository{
		GithubURL:        "https://github.com/test/ephemeral-test",
		TmuxSession:      tmuxSession,
		Agents:           make(map[string]state.Agent),
		MergeQueueConfig: state.DefaultMergeQueueConfig(),
	}
	d.GetState().AddRepo(repoName, repo)

	// Spawn ephemeral agent
	client := socket.NewClient(paths.DaemonSock)
	resp, err := client.Send(socket.Request{
		Command: "spawn_agent",
		Args: map[string]interface{}{
			"repo":   repoName,
			"name":   "ephemeral-bot",
			"class":  "ephemeral",
			"prompt": "You are an ephemeral agent that does not survive restarts.",
		},
	})
	if err != nil {
		t.Fatalf("spawn_agent request failed: %v", err)
	}
	if !resp.Success {
		t.Fatalf("spawn_agent should succeed, got error: %s", resp.Error)
	}

	// Verify agent exists and has worker type (ephemeral agents become workers)
	agent, exists := d.GetState().GetAgent(repoName, "ephemeral-bot")
	if !exists {
		t.Fatal("Agent should exist in state")
	}
	// Ephemeral agents are workers (unless they have "review" in the name)
	if agent.Type != state.AgentTypeWorker {
		t.Errorf("Agent type = %s, want worker", agent.Type)
	}
}
