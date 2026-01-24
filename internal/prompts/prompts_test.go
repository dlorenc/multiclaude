package prompts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dlorenc/multiclaude/internal/state"
)

func TestGetDefaultPrompt(t *testing.T) {
	tests := []struct {
		name      string
		agentType state.AgentType
		wantEmpty bool
	}{
		{"supervisor", state.AgentTypeSupervisor, false},
		{"workspace", state.AgentTypeWorkspace, false},
		// Worker, merge-queue, and review should return empty - they use configurable agent definitions
		{"worker", state.AgentTypeWorker, true},
		{"merge-queue", state.AgentTypeMergeQueue, true},
		{"review", state.AgentTypeReview, true},
		{"unknown", state.AgentType("unknown"), true},
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
	// Verify supervisor prompt (hardcoded - has embedded content)
	supervisorPrompt := GetDefaultPrompt(state.AgentTypeSupervisor)
	if !strings.Contains(supervisorPrompt, "You are the supervisor") {
		t.Error("supervisor prompt should mention 'You are the supervisor'")
	}
	if !strings.Contains(supervisorPrompt, "multiclaude message send") {
		t.Error("supervisor prompt should mention message commands")
	}

	// Verify workspace prompt (hardcoded - has embedded content)
	workspacePrompt := GetDefaultPrompt(state.AgentTypeWorkspace)
	if !strings.Contains(workspacePrompt, "user's workspace") {
		t.Error("workspace prompt should mention 'user's workspace'")
	}
	if !strings.Contains(workspacePrompt, "multiclaude message send") {
		t.Error("workspace prompt should document inter-agent messaging capabilities")
	}
	if !strings.Contains(workspacePrompt, "Spawning Workers") {
		t.Error("workspace prompt should document worker spawning capabilities")
	}

	// Note: Worker, merge-queue, and review prompts are now configurable
	// and come from agent definitions, not embedded defaults.
	// Their content is tested via the templates package instead.

	// Verify worker, merge-queue, and review return empty (configurable agents)
	if GetDefaultPrompt(state.AgentTypeWorker) != "" {
		t.Error("worker prompt should be empty (configurable agent)")
	}
	if GetDefaultPrompt(state.AgentTypeMergeQueue) != "" {
		t.Error("merge-queue prompt should be empty (configurable agent)")
	}
	if GetDefaultPrompt(state.AgentTypeReview) != "" {
		t.Error("review prompt should be empty (configurable agent)")
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
		prompt, err := LoadCustomPrompt(tmpDir, state.AgentTypeSupervisor)
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

		prompt, err := LoadCustomPrompt(tmpDir, state.AgentTypeSupervisor)
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

		prompt, err := LoadCustomPrompt(tmpDir, state.AgentTypeWorker)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if prompt != customContent {
			t.Errorf("expected %q, got %q", customContent, prompt)
		}
	})

	t.Run("with custom merge-queue prompt", func(t *testing.T) {
		customContent := "Custom merge-queue instructions"
		promptPath := filepath.Join(multiclaudeDir, "MERGE-QUEUE.md")
		if err := os.WriteFile(promptPath, []byte(customContent), 0644); err != nil {
			t.Fatalf("failed to write custom prompt: %v", err)
		}

		prompt, err := LoadCustomPrompt(tmpDir, state.AgentTypeMergeQueue)
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

		prompt, err := LoadCustomPrompt(tmpDir, state.AgentTypeWorkspace)
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

		prompt, err := LoadCustomPrompt(tmpDir, state.AgentTypeReview)
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
		prompt, err := GetPrompt(tmpDir, state.AgentTypeSupervisor, "")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if prompt == "" {
			t.Error("expected non-empty prompt")
		}
		if !strings.Contains(prompt, "You are the supervisor") {
			t.Error("prompt should contain default supervisor text")
		}
	})

	t.Run("default + custom", func(t *testing.T) {
		// Create .multiclaude directory
		multiclaudeDir := filepath.Join(tmpDir, ".multiclaude")
		if err := os.MkdirAll(multiclaudeDir, 0755); err != nil {
			t.Fatalf("failed to create .multiclaude dir: %v", err)
		}

		// Write custom prompt for supervisor (which has embedded default)
		customContent := "Use emojis in all messages! 🎉"
		promptPath := filepath.Join(multiclaudeDir, "SUPERVISOR.md")
		if err := os.WriteFile(promptPath, []byte(customContent), 0644); err != nil {
			t.Fatalf("failed to write custom prompt: %v", err)
		}

		prompt, err := GetPrompt(tmpDir, state.AgentTypeSupervisor, "")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !strings.Contains(prompt, "You are the supervisor") {
			t.Error("prompt should contain default supervisor text")
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
		prompt, err := GetPrompt(tmpDir, state.AgentTypeSupervisor, cliDocs)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !strings.Contains(prompt, "You are the supervisor") {
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
		{"multiclaude worker list", "/workers should include worker list command"},
	}

	// Commands expected in /messages
	messagesCommands := []struct {
		command     string
		description string
	}{
		{"multiclaude message list", "/messages should include list command"},
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

// TestGetPromptIncludesSlashCommandsForHardcodedAgentTypes verifies that GetPrompt()
// includes the slash commands section for hardcoded agent types (supervisor, workspace).
// Worker, merge-queue, and review are configurable agents and their prompts come
// from agent definitions, not GetPrompt().
func TestGetPromptIncludesSlashCommandsForHardcodedAgentTypes(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "multiclaude-prompts-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Only test hardcoded agent types (supervisor, workspace)
	agentTypes := []state.AgentType{
		state.AgentTypeSupervisor,
		state.AgentTypeWorkspace,
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

// TestGetPromptForConfigurableAgentTypesReturnsSlashCommandsOnly verifies that
// configurable agent types (worker, merge-queue, review) only get slash commands
// since they have no embedded default prompt.
func TestGetPromptForConfigurableAgentTypesReturnsSlashCommandsOnly(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "multiclaude-prompts-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Configurable agent types have no embedded prompt
	agentTypes := []state.AgentType{
		state.AgentTypeWorker,
		state.AgentTypeMergeQueue,
		state.AgentTypeReview,
	}

	for _, agentType := range agentTypes {
		t.Run(string(agentType), func(t *testing.T) {
			prompt, err := GetPrompt(tmpDir, agentType, "")
			if err != nil {
				t.Fatalf("GetPrompt failed for %s: %v", agentType, err)
			}

			// Should still include slash commands even with empty default
			if !strings.Contains(prompt, "## Slash Commands") {
				t.Errorf("GetPrompt(%s) should contain slash commands even for configurable agents", agentType)
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
