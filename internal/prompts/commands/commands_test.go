package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetCommand(t *testing.T) {
	tests := []struct {
		name    string
		want    string // Check for substring in content
		wantErr bool
	}{
		{
			name:    "refresh",
			want:    "Sync worktree with main branch",
			wantErr: false,
		},
		{
			name:    "status",
			want:    "system status",
			wantErr: false,
		},
		{
			name:    "workers",
			want:    "List active workers",
			wantErr: false,
		},
		{
			name:    "messages",
			want:    "inter-agent messages",
			wantErr: false,
		},
		{
			name:    "nonexistent",
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := GetCommand(tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetCommand(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
				return
			}
			if !tt.wantErr && content == "" {
				t.Errorf("GetCommand(%q) returned empty content", tt.name)
			}
			if tt.want != "" && !contains(content, tt.want) {
				t.Errorf("GetCommand(%q) content does not contain %q", tt.name, tt.want)
			}
		})
	}
}

func TestAvailableCommands(t *testing.T) {
	expectedCommands := []string{"refresh", "status", "workers", "messages"}

	if len(AvailableCommands) != len(expectedCommands) {
		t.Errorf("Expected %d commands, got %d", len(expectedCommands), len(AvailableCommands))
	}

	for _, expected := range expectedCommands {
		found := false
		for _, cmd := range AvailableCommands {
			if cmd.Name == expected {
				found = true
				if cmd.Filename == "" {
					t.Errorf("Command %q has empty filename", expected)
				}
				if cmd.Description == "" {
					t.Errorf("Command %q has empty description", expected)
				}
				break
			}
		}
		if !found {
			t.Errorf("Command %q not found in AvailableCommands", expected)
		}
	}
}

func TestGenerateCommandsDir(t *testing.T) {
	tmpDir := t.TempDir()
	commandsDir := filepath.Join(tmpDir, "commands")

	err := GenerateCommandsDir(commandsDir)
	if err != nil {
		t.Fatalf("GenerateCommandsDir failed: %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(commandsDir); os.IsNotExist(err) {
		t.Error("Commands directory was not created")
	}

	// Verify all command files were created
	for _, cmd := range AvailableCommands {
		filePath := filepath.Join(commandsDir, cmd.Filename)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Errorf("Command file %q was not created", cmd.Filename)
		}

		// Verify content is not empty
		content, err := os.ReadFile(filePath)
		if err != nil {
			t.Errorf("Failed to read command file %q: %v", cmd.Filename, err)
		}
		if len(content) == 0 {
			t.Errorf("Command file %q is empty", cmd.Filename)
		}
	}
}

func TestSetupAgentCommands(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "agent-config")

	err := SetupAgentCommands(configDir)
	if err != nil {
		t.Fatalf("SetupAgentCommands failed: %v", err)
	}

	// Verify config directory was created
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		t.Error("Config directory was not created")
	}

	// Verify commands subdirectory was created
	commandsDir := filepath.Join(configDir, "commands")
	if _, err := os.Stat(commandsDir); os.IsNotExist(err) {
		t.Error("Commands subdirectory was not created")
	}

	// Verify command files exist
	for _, cmd := range AvailableCommands {
		filePath := filepath.Join(commandsDir, cmd.Filename)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Errorf("Command file %q was not created", cmd.Filename)
		}
	}
}

func TestSetupAgentCommandsIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "agent-config")

	// First call
	if err := SetupAgentCommands(configDir); err != nil {
		t.Fatalf("First SetupAgentCommands failed: %v", err)
	}

	// Second call should not fail
	if err := SetupAgentCommands(configDir); err != nil {
		t.Fatalf("Second SetupAgentCommands failed: %v", err)
	}

	// Verify files still exist
	commandsDir := filepath.Join(configDir, "commands")
	for _, cmd := range AvailableCommands {
		filePath := filepath.Join(commandsDir, cmd.Filename)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Errorf("Command file %q missing after second setup", cmd.Filename)
		}
	}
}

func TestLinkGlobalCredentials(t *testing.T) {
	// Save original HOME and restore after test
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)

	// Create fake global credentials
	globalClaudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(globalClaudeDir, 0755); err != nil {
		t.Fatalf("Failed to create .claude dir: %v", err)
	}
	globalCredFile := filepath.Join(globalClaudeDir, ".credentials.json")
	if err := os.WriteFile(globalCredFile, []byte(`{"token":"test"}`), 0644); err != nil {
		t.Fatalf("Failed to create credentials file: %v", err)
	}

	// Create agent config directory
	configDir := filepath.Join(tmpDir, "agent-config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	// Link credentials
	if err := LinkGlobalCredentials(configDir); err != nil {
		t.Fatalf("LinkGlobalCredentials failed: %v", err)
	}

	// Verify symlink was created
	localCredFile := filepath.Join(configDir, ".credentials.json")
	linkTarget, err := os.Readlink(localCredFile)
	if err != nil {
		t.Fatalf("Failed to read symlink: %v", err)
	}
	if linkTarget != globalCredFile {
		t.Errorf("Symlink target = %q, want %q", linkTarget, globalCredFile)
	}
}

func TestLinkGlobalCredentialsNoGlobalCreds(t *testing.T) {
	// Save original HOME and restore after test
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)

	// Don't create global credentials - simulates API key user

	configDir := filepath.Join(tmpDir, "agent-config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	// Should succeed without creating symlink
	if err := LinkGlobalCredentials(configDir); err != nil {
		t.Fatalf("LinkGlobalCredentials failed: %v", err)
	}

	// Verify no symlink was created
	localCredFile := filepath.Join(configDir, ".credentials.json")
	if _, err := os.Stat(localCredFile); !os.IsNotExist(err) {
		t.Error("Symlink should not exist when global credentials are missing")
	}
}

func TestLinkGlobalCredentialsIdempotent(t *testing.T) {
	// Save original HOME and restore after test
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)

	// Create fake global credentials
	globalClaudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(globalClaudeDir, 0755); err != nil {
		t.Fatalf("Failed to create .claude dir: %v", err)
	}
	globalCredFile := filepath.Join(globalClaudeDir, ".credentials.json")
	if err := os.WriteFile(globalCredFile, []byte(`{"token":"test"}`), 0644); err != nil {
		t.Fatalf("Failed to create credentials file: %v", err)
	}

	configDir := filepath.Join(tmpDir, "agent-config")

	// First call
	if err := LinkGlobalCredentials(configDir); err != nil {
		t.Fatalf("First LinkGlobalCredentials failed: %v", err)
	}

	// Second call should not fail
	if err := LinkGlobalCredentials(configDir); err != nil {
		t.Fatalf("Second LinkGlobalCredentials failed: %v", err)
	}

	// Verify symlink still correct
	localCredFile := filepath.Join(configDir, ".credentials.json")
	linkTarget, err := os.Readlink(localCredFile)
	if err != nil {
		t.Fatalf("Failed to read symlink: %v", err)
	}
	if linkTarget != globalCredFile {
		t.Errorf("Symlink target = %q, want %q", linkTarget, globalCredFile)
	}
}

func TestLinkGlobalCredentialsReplacesInvalidSymlink(t *testing.T) {
	// Save original HOME and restore after test
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)

	// Create fake global credentials
	globalClaudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(globalClaudeDir, 0755); err != nil {
		t.Fatalf("Failed to create .claude dir: %v", err)
	}
	globalCredFile := filepath.Join(globalClaudeDir, ".credentials.json")
	if err := os.WriteFile(globalCredFile, []byte(`{"token":"test"}`), 0644); err != nil {
		t.Fatalf("Failed to create credentials file: %v", err)
	}

	configDir := filepath.Join(tmpDir, "agent-config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	// Create an invalid symlink pointing elsewhere
	localCredFile := filepath.Join(configDir, ".credentials.json")
	if err := os.Symlink("/some/invalid/path", localCredFile); err != nil {
		t.Fatalf("Failed to create invalid symlink: %v", err)
	}

	// LinkGlobalCredentials should fix the invalid symlink
	if err := LinkGlobalCredentials(configDir); err != nil {
		t.Fatalf("LinkGlobalCredentials failed: %v", err)
	}

	// Verify symlink now points to correct location
	linkTarget, err := os.Readlink(localCredFile)
	if err != nil {
		t.Fatalf("Failed to read symlink: %v", err)
	}
	if linkTarget != globalCredFile {
		t.Errorf("Symlink target = %q, want %q", linkTarget, globalCredFile)
	}
}

func TestLinkGlobalCredentialsReplacesRegularFile(t *testing.T) {
	// Save original HOME and restore after test
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)

	// Create fake global credentials
	globalClaudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(globalClaudeDir, 0755); err != nil {
		t.Fatalf("Failed to create .claude dir: %v", err)
	}
	globalCredFile := filepath.Join(globalClaudeDir, ".credentials.json")
	if err := os.WriteFile(globalCredFile, []byte(`{"token":"test"}`), 0644); err != nil {
		t.Fatalf("Failed to create credentials file: %v", err)
	}

	configDir := filepath.Join(tmpDir, "agent-config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	// Create a regular file instead of symlink
	localCredFile := filepath.Join(configDir, ".credentials.json")
	if err := os.WriteFile(localCredFile, []byte(`{"old":"data"}`), 0644); err != nil {
		t.Fatalf("Failed to create regular file: %v", err)
	}

	// LinkGlobalCredentials should replace the regular file with symlink
	if err := LinkGlobalCredentials(configDir); err != nil {
		t.Fatalf("LinkGlobalCredentials failed: %v", err)
	}

	// Verify it's now a symlink to the correct location
	linkTarget, err := os.Readlink(localCredFile)
	if err != nil {
		t.Fatalf("Failed to read symlink (file may not have been replaced): %v", err)
	}
	if linkTarget != globalCredFile {
		t.Errorf("Symlink target = %q, want %q", linkTarget, globalCredFile)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
