package prompts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetDefaultPrompt(t *testing.T) {
	tests := []struct {
		name      string
		agentType AgentType
		wantEmpty bool
	}{
		{"supervisor", TypeSupervisor, false},
		{"worker", TypeWorker, false},
		{"merge-queue", TypeMergeQueue, false},
		{"workspace", TypeWorkspace, false},
		{"review", TypeReview, false},
		{"unknown", AgentType("unknown"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := GetDefaultPrompt(tt.agentType)
			if tt.wantEmpty && prompt != "" {
				t.Errorf("expected empty prompt for %s, got %s", tt.agentType, prompt)
			}
			if !tt.wantEmpty && prompt == "" {
				t.Errorf("expected non-empty prompt for %s", tt.agentType)
			}
		})
	}
}

func TestGetDefaultPromptContent(t *testing.T) {
	// Verify supervisor prompt
	supervisorPrompt := GetDefaultPrompt(TypeSupervisor)
	if !strings.Contains(supervisorPrompt, "supervisor agent") {
		t.Error("supervisor prompt should mention 'supervisor agent'")
	}
	if !strings.Contains(supervisorPrompt, "multiclaude agent send-message") {
		t.Error("supervisor prompt should mention message commands")
	}

	// Verify worker prompt
	workerPrompt := GetDefaultPrompt(TypeWorker)
	if !strings.Contains(workerPrompt, "worker agent") {
		t.Error("worker prompt should mention 'worker agent'")
	}
	if !strings.Contains(workerPrompt, "multiclaude agent complete") {
		t.Error("worker prompt should mention complete command")
	}

	// Verify merge queue prompt
	mergePrompt := GetDefaultPrompt(TypeMergeQueue)
	if !strings.Contains(mergePrompt, "merge queue agent") {
		t.Error("merge queue prompt should mention 'merge queue agent'")
	}
	if !strings.Contains(mergePrompt, "CRITICAL CONSTRAINT") {
		t.Error("merge queue prompt should have critical constraint about CI")
	}

	// Verify workspace prompt
	workspacePrompt := GetDefaultPrompt(TypeWorkspace)
	if !strings.Contains(workspacePrompt, "user workspace") {
		t.Error("workspace prompt should mention 'user workspace'")
	}
	if !strings.Contains(workspacePrompt, "multiclaude agent send-message") {
		t.Error("workspace prompt should document inter-agent messaging capabilities")
	}
	if !strings.Contains(workspacePrompt, "Spawn and manage worker agents") {
		t.Error("workspace prompt should document worker spawning capabilities")
	}

	// Verify review prompt
	reviewPrompt := GetDefaultPrompt(TypeReview)
	if !strings.Contains(reviewPrompt, "code review agent") {
		t.Error("review prompt should mention 'code review agent'")
	}
	if !strings.Contains(reviewPrompt, "Forward progress is forward") {
		t.Error("review prompt should mention the philosophy 'Forward progress is forward'")
	}
	if !strings.Contains(reviewPrompt, "[BLOCKING]") {
		t.Error("review prompt should mention [BLOCKING] comment format")
	}
	if !strings.Contains(reviewPrompt, "multiclaude agent complete") {
		t.Error("review prompt should mention complete command")
	}
}

func TestLoadCustomPrompt(t *testing.T) {
	// Create temporary repo directory
	tmpDir, err := os.MkdirTemp("", "multiclaude-prompts-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create .multiclaude directory
	multiclaudeDir := filepath.Join(tmpDir, ".multiclaude")
	if err := os.MkdirAll(multiclaudeDir, 0755); err != nil {
		t.Fatalf("failed to create .multiclaude dir: %v", err)
	}

	t.Run("no custom prompt", func(t *testing.T) {
		prompt, err := LoadCustomPrompt(tmpDir, TypeSupervisor)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if prompt != "" {
			t.Errorf("expected empty prompt when file doesn't exist, got: %s", prompt)
		}
	})

	t.Run("with custom supervisor prompt", func(t *testing.T) {
		customContent := "Custom supervisor instructions here"
		promptPath := filepath.Join(multiclaudeDir, "SUPERVISOR.md")
		if err := os.WriteFile(promptPath, []byte(customContent), 0644); err != nil {
			t.Fatalf("failed to write custom prompt: %v", err)
		}

		prompt, err := LoadCustomPrompt(tmpDir, TypeSupervisor)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if prompt != customContent {
			t.Errorf("expected %q, got %q", customContent, prompt)
		}
	})

	t.Run("with custom worker prompt", func(t *testing.T) {
		customContent := "Custom worker instructions"
		promptPath := filepath.Join(multiclaudeDir, "WORKER.md")
		if err := os.WriteFile(promptPath, []byte(customContent), 0644); err != nil {
			t.Fatalf("failed to write custom prompt: %v", err)
		}

		prompt, err := LoadCustomPrompt(tmpDir, TypeWorker)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if prompt != customContent {
			t.Errorf("expected %q, got %q", customContent, prompt)
		}
	})

	t.Run("with custom reviewer prompt", func(t *testing.T) {
		customContent := "Custom reviewer instructions"
		promptPath := filepath.Join(multiclaudeDir, "REVIEWER.md")
		if err := os.WriteFile(promptPath, []byte(customContent), 0644); err != nil {
			t.Fatalf("failed to write custom prompt: %v", err)
		}

		prompt, err := LoadCustomPrompt(tmpDir, TypeMergeQueue)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if prompt != customContent {
			t.Errorf("expected %q, got %q", customContent, prompt)
		}
	})

	t.Run("with custom workspace prompt", func(t *testing.T) {
		customContent := "Custom workspace instructions"
		promptPath := filepath.Join(multiclaudeDir, "WORKSPACE.md")
		if err := os.WriteFile(promptPath, []byte(customContent), 0644); err != nil {
			t.Fatalf("failed to write custom prompt: %v", err)
		}

		prompt, err := LoadCustomPrompt(tmpDir, TypeWorkspace)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if prompt != customContent {
			t.Errorf("expected %q, got %q", customContent, prompt)
		}
	})

	t.Run("with custom review prompt", func(t *testing.T) {
		customContent := "Custom review instructions"
		promptPath := filepath.Join(multiclaudeDir, "REVIEW.md")
		if err := os.WriteFile(promptPath, []byte(customContent), 0644); err != nil {
			t.Fatalf("failed to write custom prompt: %v", err)
		}

		prompt, err := LoadCustomPrompt(tmpDir, TypeReview)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if prompt != customContent {
			t.Errorf("expected %q, got %q", customContent, prompt)
		}
	})
}

func TestGenerateTrackingModePrompt(t *testing.T) {
	tests := []struct {
		name       string
		trackMode  string
		wantPrefix string
		wantCmd    string
	}{
		{
			name:       "author mode",
			trackMode:  "author",
			wantPrefix: "## PR Tracking Mode: Author Only",
			wantCmd:    "--author @me",
		},
		{
			name:       "assigned mode",
			trackMode:  "assigned",
			wantPrefix: "## PR Tracking Mode: Assigned Only",
			wantCmd:    "--assignee @me",
		},
		{
			name:       "all mode",
			trackMode:  "all",
			wantPrefix: "## PR Tracking Mode: All PRs",
			wantCmd:    "--label multiclaude",
		},
		{
			name:       "default (empty)",
			trackMode:  "",
			wantPrefix: "## PR Tracking Mode: All PRs",
			wantCmd:    "--label multiclaude",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateTrackingModePrompt(tt.trackMode)

			if !strings.HasPrefix(result, tt.wantPrefix) {
				t.Errorf("GenerateTrackingModePrompt(%q) should start with %q, got %q",
					tt.trackMode, tt.wantPrefix, result[:min(len(result), 50)])
			}

			if !strings.Contains(result, tt.wantCmd) {
				t.Errorf("GenerateTrackingModePrompt(%q) should contain %q",
					tt.trackMode, tt.wantCmd)
			}
		})
	}
}

func TestGetPrompt(t *testing.T) {
	// Create temporary repo directory
	tmpDir, err := os.MkdirTemp("", "multiclaude-prompts-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	t.Run("default only", func(t *testing.T) {
		prompt, err := GetPrompt(tmpDir, TypeSupervisor, "")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if prompt == "" {
			t.Error("expected non-empty prompt")
		}
		if !strings.Contains(prompt, "supervisor agent") {
			t.Error("prompt should contain default supervisor text")
		}
	})

	t.Run("default + custom", func(t *testing.T) {
		// Create .multiclaude directory
		multiclaudeDir := filepath.Join(tmpDir, ".multiclaude")
		if err := os.MkdirAll(multiclaudeDir, 0755); err != nil {
			t.Fatalf("failed to create .multiclaude dir: %v", err)
		}

		// Write custom prompt
		customContent := "Use emojis in all messages! 🎉"
		promptPath := filepath.Join(multiclaudeDir, "WORKER.md")
		if err := os.WriteFile(promptPath, []byte(customContent), 0644); err != nil {
			t.Fatalf("failed to write custom prompt: %v", err)
		}

		prompt, err := GetPrompt(tmpDir, TypeWorker, "")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !strings.Contains(prompt, "worker agent") {
			t.Error("prompt should contain default worker text")
		}
		if !strings.Contains(prompt, "Use emojis") {
			t.Error("prompt should contain custom text")
		}
		if !strings.Contains(prompt, "Repository-specific instructions") {
			t.Error("prompt should have separator between default and custom")
		}
	})

	t.Run("with CLI docs", func(t *testing.T) {
		cliDocs := "# CLI Documentation\n\n## Commands\n\n- test command"
		prompt, err := GetPrompt(tmpDir, TypeSupervisor, cliDocs)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !strings.Contains(prompt, "supervisor agent") {
			t.Error("prompt should contain default supervisor text")
		}
		if !strings.Contains(prompt, "CLI Documentation") {
			t.Error("prompt should contain CLI docs")
		}
	})
}

// TestGetSlashCommandsPromptContainsAllCommands verifies that GetSlashCommandsPrompt()
// includes all expected slash commands.
func TestGetSlashCommandsPromptContainsAllCommands(t *testing.T) {
	prompt := GetSlashCommandsPrompt()

	expectedCommands := []string{
		"/status",
		"/refresh",
		"/workers",
		"/messages",
	}

	for _, cmd := range expectedCommands {
		if !strings.Contains(prompt, cmd) {
			t.Errorf("GetSlashCommandsPrompt() should contain %q", cmd)
		}
	}
}

// TestGetSlashCommandsPromptContainsCLICommands verifies that GetSlashCommandsPrompt()
// contains the actual CLI commands that should be run for each slash command.
func TestGetSlashCommandsPromptContainsCLICommands(t *testing.T) {
	prompt := GetSlashCommandsPrompt()

	// Commands expected in /status
	statusCommands := []struct {
		command     string
		description string
	}{
		{"multiclaude daemon status", "/status should include daemon status check"},
		{"git status", "/status should include git status"},
	}

	// Commands expected in /refresh
	refreshCommands := []struct {
		command     string
		description string
	}{
		{"git fetch origin main", "/refresh should include fetch from origin/main"},
		{"git rebase origin/main", "/refresh should include rebase onto origin/main"},
	}

	// Commands expected in /workers
	workersCommands := []struct {
		command     string
		description string
	}{
		{"multiclaude work list", "/workers should include work list command"},
	}

	// Commands expected in /messages
	messagesCommands := []struct {
		command     string
		description string
	}{
		{"multiclaude agent list-messages", "/messages should include list-messages command"},
	}

	allCommands := [][]struct {
		command     string
		description string
	}{
		statusCommands,
		refreshCommands,
		workersCommands,
		messagesCommands,
	}

	for _, cmdGroup := range allCommands {
		for _, cmd := range cmdGroup {
			if !strings.Contains(prompt, cmd.command) {
				t.Errorf("GetSlashCommandsPrompt(): %s (expected command: %q)", cmd.description, cmd.command)
			}
		}
	}
}

// TestGetPromptIncludesSlashCommandsForAllAgentTypes verifies that GetPrompt()
// includes the slash commands section for every agent type.
func TestGetPromptIncludesSlashCommandsForAllAgentTypes(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "multiclaude-prompts-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	agentTypes := []AgentType{
		TypeWorker,
		TypeSupervisor,
		TypeMergeQueue,
		TypeWorkspace,
		TypeReview,
	}

	for _, agentType := range agentTypes {
		t.Run(string(agentType), func(t *testing.T) {
			prompt, err := GetPrompt(tmpDir, agentType, "")
			if err != nil {
				t.Fatalf("GetPrompt failed for %s: %v", agentType, err)
			}

			// Verify slash commands section header is present
			if !strings.Contains(prompt, "## Slash Commands") {
				t.Errorf("GetPrompt(%s) should contain slash commands section header", agentType)
			}

			// Verify all slash commands are present
			expectedCommands := []string{"/status", "/refresh", "/workers", "/messages"}
			for _, cmd := range expectedCommands {
				if !strings.Contains(prompt, cmd) {
					t.Errorf("GetPrompt(%s) should contain slash command %q", agentType, cmd)
				}
			}
		})
	}
}

// TestGetSlashCommandsPromptFormatting verifies that the slash commands section
// is properly formatted with headers, code blocks, etc.
func TestGetSlashCommandsPromptFormatting(t *testing.T) {
	prompt := GetSlashCommandsPrompt()

	// Check for main section header
	if !strings.Contains(prompt, "## Slash Commands") {
		t.Error("GetSlashCommandsPrompt() should have '## Slash Commands' header")
	}

	// Check for introductory text
	if !strings.Contains(prompt, "The following slash commands are available") {
		t.Error("GetSlashCommandsPrompt() should have introductory text")
	}

	// Check for code block markers (bash code blocks in command files)
	if !strings.Contains(prompt, "```bash") {
		t.Error("GetSlashCommandsPrompt() should contain bash code blocks")
	}

	// Check for section separators
	if !strings.Contains(prompt, "---") {
		t.Error("GetSlashCommandsPrompt() should contain section separators between commands")
	}

	// Check that each command has a header (# /command format)
	commandHeaders := []string{
		"# /status",
		"# /refresh",
		"# /workers",
		"# /messages",
	}
	for _, header := range commandHeaders {
		if !strings.Contains(prompt, header) {
			t.Errorf("GetSlashCommandsPrompt() should contain command header %q", header)
		}
	}

	// Check for instructions sections
	if !strings.Contains(prompt, "## Instructions") {
		t.Error("GetSlashCommandsPrompt() should contain instruction headers")
	}
}

// TestGetSlashCommandsPromptNonEmpty verifies that GetSlashCommandsPrompt()
// returns a non-empty result.
func TestGetSlashCommandsPromptNonEmpty(t *testing.T) {
	prompt := GetSlashCommandsPrompt()

	if prompt == "" {
		t.Error("GetSlashCommandsPrompt() should not return empty string")
	}

	// Should be substantial content (at least include all 4 commands with descriptions)
	if len(prompt) < 500 {
		t.Errorf("GetSlashCommandsPrompt() seems too short (got %d bytes), expected substantial content", len(prompt))
	}
}

// TestParseGitHubURL tests the parseGitHubURL helper function
func TestParseGitHubURL(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantOwner string
		wantRepo  string
	}{
		{
			name:      "HTTPS URL",
			url:       "https://github.com/dlorenc/multiclaude.git",
			wantOwner: "dlorenc",
			wantRepo:  "multiclaude",
		},
		{
			name:      "HTTPS URL without .git",
			url:       "https://github.com/aronchick/multiclaude",
			wantOwner: "aronchick",
			wantRepo:  "multiclaude",
		},
		{
			name:      "SSH URL",
			url:       "git@github.com:dlorenc/multiclaude.git",
			wantOwner: "dlorenc",
			wantRepo:  "multiclaude",
		},
		{
			name:      "SSH URL without .git",
			url:       "git@github.com:aronchick/multiclaude",
			wantOwner: "aronchick",
			wantRepo:  "multiclaude",
		},
		{
			name:      "Invalid URL",
			url:       "not-a-github-url",
			wantOwner: "",
			wantRepo:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo := parseGitHubURL(tt.url)
			if owner != tt.wantOwner {
				t.Errorf("parseGitHubURL(%q) owner = %q, want %q", tt.url, owner, tt.wantOwner)
			}
			if repo != tt.wantRepo {
				t.Errorf("parseGitHubURL(%q) repo = %q, want %q", tt.url, repo, tt.wantRepo)
			}
		})
	}
}

// TestDetectFork tests fork detection in various git repository configurations
func TestDetectFork(t *testing.T) {
	// This test requires git to be installed
	// We'll create temporary git repos with different remote configurations

	t.Run("non-git directory returns non-fork", func(t *testing.T) {
		// Create a temporary directory
		tmpDir, err := os.MkdirTemp("", "multiclaude-fork-test-*")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		// Note: DetectFork will return IsFork=false if git command fails
		// This is expected behavior - we gracefully handle non-git directories
		info, err := DetectFork(tmpDir)
		if err != nil {
			t.Errorf("DetectFork() unexpected error: %v", err)
		}
		if info == nil {
			t.Fatal("DetectFork() returned nil info")
		}
		// For non-git directory, should return IsFork=false
		if info.IsFork {
			t.Error("DetectFork() for non-git directory should return IsFork=false")
		}
	})

	t.Run("actual git repo without upstream", func(t *testing.T) {
		// We can test against the actual repo we're in (if it's not a fork)
		// This is a real integration test
		currentDir, err := os.Getwd()
		if err != nil {
			t.Fatalf("failed to get current dir: %v", err)
		}

		// Find the repo root (go up until we find .git)
		repoRoot := currentDir
		for {
			if _, err := os.Stat(filepath.Join(repoRoot, ".git")); err == nil {
				break
			}
			parent := filepath.Dir(repoRoot)
			if parent == repoRoot {
				t.Skip("not in a git repository")
			}
			repoRoot = parent
		}

		info, err := DetectFork(repoRoot)
		if err != nil {
			t.Errorf("DetectFork() unexpected error: %v", err)
		}
		if info == nil {
			t.Fatal("DetectFork() returned nil info")
		}

		// The actual result depends on whether we're in a fork or not
		// Just verify the function runs without error
		t.Logf("Fork detection result: IsFork=%v, Upstream=%s/%s, Fork=%s/%s",
			info.IsFork, info.UpstreamOwner, info.UpstreamRepo, info.ForkOwner, info.ForkRepo)
	})
}

// TestGenerateForkWorkflowPrompt tests fork workflow prompt generation
func TestGenerateForkWorkflowPrompt(t *testing.T) {
	t.Run("non-fork returns empty string", func(t *testing.T) {
		info := &ForkInfo{
			IsFork: false,
		}
		prompt := GenerateForkWorkflowPrompt(info)
		if prompt != "" {
			t.Error("GenerateForkWorkflowPrompt() should return empty string for non-fork")
		}
	})

	t.Run("fork returns guidance", func(t *testing.T) {
		info := &ForkInfo{
			IsFork:         true,
			UpstreamRemote: "upstream",
			UpstreamOwner:  "dlorenc",
			UpstreamRepo:   "multiclaude",
			ForkOwner:      "aronchick",
			ForkRepo:       "multiclaude",
		}
		prompt := GenerateForkWorkflowPrompt(info)

		// Verify key elements are present
		if !strings.Contains(prompt, "Fork Workflow") {
			t.Error("prompt should contain 'Fork Workflow' header")
		}
		if !strings.Contains(prompt, "dlorenc/multiclaude") {
			t.Error("prompt should mention upstream repo")
		}
		if !strings.Contains(prompt, "aronchick/multiclaude") {
			t.Error("prompt should mention fork repo")
		}
		if !strings.Contains(prompt, "feature branch") {
			t.Error("prompt should mention feature branches")
		}
		if !strings.Contains(prompt, "gh pr create --repo") {
			t.Error("prompt should show upstream PR creation command")
		}
		if !strings.Contains(prompt, "CRITICAL") {
			t.Error("prompt should emphasize critical points")
		}
	})
}

// TestGetPromptIncludesForkGuidance verifies fork guidance is included when in a fork
func TestGetPromptIncludesForkGuidance(t *testing.T) {
	// Get the actual repo we're testing in
	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current dir: %v", err)
	}

	// Find repo root
	repoRoot := currentDir
	for {
		if _, err := os.Stat(filepath.Join(repoRoot, ".git")); err == nil {
			break
		}
		parent := filepath.Dir(repoRoot)
		if parent == repoRoot {
			t.Skip("not in a git repository")
		}
		repoRoot = parent
	}

	// Test GetPrompt with the actual repo
	prompt, err := GetPrompt(repoRoot, TypeWorker, "")
	if err != nil {
		t.Errorf("GetPrompt() unexpected error: %v", err)
	}

	// Check if we're in a fork
	forkInfo, err := DetectFork(repoRoot)
	if err != nil {
		t.Fatalf("DetectFork() failed: %v", err)
	}

	if forkInfo.IsFork {
		// If we're in a fork, prompt should include fork guidance
		if !strings.Contains(prompt, "Fork Workflow") {
			t.Error("GetPrompt() should include fork workflow guidance when in a fork")
		}
		t.Logf("Detected fork: %s/%s (upstream: %s/%s)",
			forkInfo.ForkOwner, forkInfo.ForkRepo,
			forkInfo.UpstreamOwner, forkInfo.UpstreamRepo)
	} else {
		// If not in a fork, should not include fork guidance
		if strings.Contains(prompt, "Fork Workflow") {
			t.Error("GetPrompt() should not include fork workflow guidance when not in a fork")
		}
		t.Log("Not in a fork, no fork guidance included (expected)")
	}
}
