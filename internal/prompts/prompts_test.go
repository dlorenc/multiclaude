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
	if !strings.Contains(workspacePrompt, "NOT receive messages from the supervisor") {
		t.Error("workspace prompt should clarify that it doesn't receive supervisor messages")
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
