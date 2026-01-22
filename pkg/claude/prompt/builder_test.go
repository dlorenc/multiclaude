package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewBuilder(t *testing.T) {
	b := NewBuilder()
	if b == nil {
		t.Fatal("NewBuilder() returned nil")
	}
	if b.Len() != 0 {
		t.Errorf("expected empty builder, got %d sections", b.Len())
	}
}

func TestBuilderAddSection(t *testing.T) {
	b := NewBuilder()
	b.AddSection("Role", "You are a helpful assistant.")
	b.AddSection("Guidelines", "Write clean code.")

	if b.Len() != 2 {
		t.Errorf("expected 2 sections, got %d", b.Len())
	}

	result := b.Build()
	if !strings.Contains(result, "## Role") {
		t.Error("expected result to contain '## Role'")
	}
	if !strings.Contains(result, "You are a helpful assistant.") {
		t.Error("expected result to contain role content")
	}
	if !strings.Contains(result, "## Guidelines") {
		t.Error("expected result to contain '## Guidelines'")
	}
}

func TestBuilderAddSectionEmpty(t *testing.T) {
	b := NewBuilder()
	b.AddSection("Empty", "")
	b.AddSection("Role", "Content")

	// Empty section should be skipped
	if b.Len() != 1 {
		t.Errorf("expected 1 section (empty skipped), got %d", b.Len())
	}
}

func TestBuilderAddRaw(t *testing.T) {
	b := NewBuilder()
	b.AddRaw("Raw content without header")

	result := b.Build()
	if !strings.Contains(result, "Raw content without header") {
		t.Error("expected result to contain raw content")
	}
	if strings.Contains(result, "## ") {
		t.Error("raw content should not have a header")
	}
}

func TestBuilderAddRawEmpty(t *testing.T) {
	b := NewBuilder()
	b.AddRaw("")

	if b.Len() != 0 {
		t.Errorf("expected 0 sections (empty skipped), got %d", b.Len())
	}
}

func TestBuilderBuild(t *testing.T) {
	b := NewBuilder()
	b.AddSection("First", "First content")
	b.AddSection("Second", "Second content")

	result := b.Build()

	// Sections should be separated by ---
	if !strings.Contains(result, "---") {
		t.Error("expected sections to be separated by ---")
	}

	// Check order
	firstIdx := strings.Index(result, "First content")
	secondIdx := strings.Index(result, "Second content")
	if firstIdx > secondIdx {
		t.Error("expected first section to come before second")
	}
}

func TestBuilderBuildEmpty(t *testing.T) {
	b := NewBuilder()
	result := b.Build()

	if result != "" {
		t.Errorf("expected empty string from empty builder, got %q", result)
	}
}

func TestBuilderChaining(t *testing.T) {
	result := NewBuilder().
		AddSection("A", "Content A").
		AddSection("B", "Content B").
		AddRaw("Raw").
		Build()

	if !strings.Contains(result, "Content A") {
		t.Error("expected result to contain Content A")
	}
	if !strings.Contains(result, "Content B") {
		t.Error("expected result to contain Content B")
	}
	if !strings.Contains(result, "Raw") {
		t.Error("expected result to contain Raw")
	}
}

func TestBuilderClear(t *testing.T) {
	b := NewBuilder()
	b.AddSection("Test", "Content")
	b.Clear()

	if b.Len() != 0 {
		t.Errorf("expected 0 sections after clear, got %d", b.Len())
	}
}

func TestNewLoader(t *testing.T) {
	l := NewLoader()
	if l == nil {
		t.Fatal("NewLoader() returned nil")
	}
	if l.DefaultPrompts == nil {
		t.Error("DefaultPrompts should be initialized")
	}
}

func TestLoaderSetDefault(t *testing.T) {
	l := NewLoader()
	l.SetDefault(TypeSupervisor, "Default supervisor prompt")

	if l.DefaultPrompts[TypeSupervisor] != "Default supervisor prompt" {
		t.Error("expected default prompt to be set")
	}
}

func TestLoaderSetCustomDir(t *testing.T) {
	l := NewLoader()
	l.SetCustomDir("/path/to/prompts")

	if l.CustomPromptDir != "/path/to/prompts" {
		t.Error("expected custom dir to be set")
	}
}

func TestLoaderLoad(t *testing.T) {
	l := NewLoader()
	l.SetDefault(TypeSupervisor, "You are a supervisor.")

	result, err := l.Load(TypeSupervisor)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if !strings.Contains(result, "You are a supervisor.") {
		t.Error("expected result to contain default prompt")
	}
}

func TestLoaderLoadWithCustomPrompt(t *testing.T) {
	// Create temp directory with custom prompt
	tmpDir, err := os.MkdirTemp("", "prompt-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	customContent := "Custom supervisor instructions."
	if err := os.WriteFile(filepath.Join(tmpDir, "SUPERVISOR.md"), []byte(customContent), 0644); err != nil {
		t.Fatalf("failed to write custom prompt: %v", err)
	}

	l := NewLoader()
	l.SetDefault(TypeSupervisor, "Default supervisor prompt")
	l.SetCustomDir(tmpDir)

	result, err := l.Load(TypeSupervisor)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Should contain both default and custom
	if !strings.Contains(result, "Default supervisor prompt") {
		t.Error("expected result to contain default prompt")
	}
	if !strings.Contains(result, "Custom supervisor instructions.") {
		t.Error("expected result to contain custom prompt")
	}
	if !strings.Contains(result, "Repository-specific instructions") {
		t.Error("expected result to have custom prompt section header")
	}
}

func TestLoaderLoadCustom(t *testing.T) {
	// Create temp directory with custom prompt
	tmpDir, err := os.MkdirTemp("", "prompt-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	customContent := "Custom worker instructions."
	if err := os.WriteFile(filepath.Join(tmpDir, "WORKER.md"), []byte(customContent), 0644); err != nil {
		t.Fatalf("failed to write custom prompt: %v", err)
	}

	l := NewLoader()
	l.SetCustomDir(tmpDir)

	result, err := l.LoadCustom(TypeWorker)
	if err != nil {
		t.Fatalf("LoadCustom() failed: %v", err)
	}

	if result != customContent {
		t.Errorf("expected %q, got %q", customContent, result)
	}
}

func TestLoaderLoadCustomMissing(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "prompt-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	l := NewLoader()
	l.SetCustomDir(tmpDir)

	result, err := l.LoadCustom(TypeSupervisor)
	if err != nil {
		t.Fatalf("LoadCustom() failed: %v", err)
	}

	// Should return empty string for missing file
	if result != "" {
		t.Errorf("expected empty string for missing file, got %q", result)
	}
}

func TestLoaderLoadCustomNoDir(t *testing.T) {
	l := NewLoader()
	// No custom dir set

	result, err := l.LoadCustom(TypeSupervisor)
	if err != nil {
		t.Fatalf("LoadCustom() failed: %v", err)
	}

	if result != "" {
		t.Errorf("expected empty string when no custom dir, got %q", result)
	}
}

func TestLoaderLoadCustomUnknownType(t *testing.T) {
	l := NewLoader()
	l.SetCustomDir("/tmp")

	_, err := l.LoadCustom(AgentType("unknown"))
	if err == nil {
		t.Error("expected error for unknown agent type")
	}
}

func TestLoaderLoadWithExtras(t *testing.T) {
	l := NewLoader()
	l.SetDefault(TypeWorker, "Default worker prompt")

	extras := map[string]string{
		"CLI Documentation":  "Available commands: ...",
		"Additional Context": "Some context here.",
	}

	result, err := l.LoadWithExtras(TypeWorker, extras)
	if err != nil {
		t.Fatalf("LoadWithExtras() failed: %v", err)
	}

	if !strings.Contains(result, "Default worker prompt") {
		t.Error("expected result to contain default prompt")
	}
	if !strings.Contains(result, "CLI Documentation") {
		t.Error("expected result to contain extras header")
	}
	if !strings.Contains(result, "Available commands:") {
		t.Error("expected result to contain extras content")
	}
}

func TestCustomPromptFilename(t *testing.T) {
	tests := []struct {
		agentType AgentType
		expected  string
	}{
		{TypeSupervisor, "SUPERVISOR.md"},
		{TypeWorker, "WORKER.md"},
		{TypeMergeQueue, "REVIEWER.md"},
		{TypeWorkspace, "WORKSPACE.md"},
		{TypeReview, "REVIEW.md"},
		{AgentType("unknown"), ""},
	}

	for _, tc := range tests {
		t.Run(string(tc.agentType), func(t *testing.T) {
			result := customPromptFilename(tc.agentType)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestWriteToFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "prompt-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Test writing to nested directory (should create it)
	path := filepath.Join(tmpDir, "nested", "dir", "prompt.md")
	content := "Test prompt content"

	if err := WriteToFile(path, content); err != nil {
		t.Fatalf("WriteToFile() failed: %v", err)
	}

	// Verify file exists and has correct content
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	if string(data) != content {
		t.Errorf("expected %q, got %q", content, string(data))
	}
}

func TestWriteToFileExisting(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "prompt-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	path := filepath.Join(tmpDir, "prompt.md")

	// Write first content
	if err := WriteToFile(path, "First"); err != nil {
		t.Fatalf("WriteToFile() failed: %v", err)
	}

	// Overwrite with new content
	if err := WriteToFile(path, "Second"); err != nil {
		t.Fatalf("WriteToFile() failed: %v", err)
	}

	// Verify new content
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	if string(data) != "Second" {
		t.Errorf("expected 'Second', got %q", string(data))
	}
}

func TestWriteToFileInvalidPath(t *testing.T) {
	// Try to write to a path where we can't create the directory
	// (using /dev/null as a file means we can't create a subdir inside it)
	err := WriteToFile("/dev/null/subdir/prompt.md", "content")
	if err == nil {
		t.Error("expected error when writing to invalid path")
	}
}

func TestLoaderChaining(t *testing.T) {
	l := NewLoader().
		SetDefault(TypeSupervisor, "Default").
		SetCustomDir("/tmp")

	if l.DefaultPrompts[TypeSupervisor] != "Default" {
		t.Error("chained SetDefault failed")
	}
	if l.CustomPromptDir != "/tmp" {
		t.Error("chained SetCustomDir failed")
	}
}

func TestLoaderLoadUnknownAgentType(t *testing.T) {
	l := NewLoader()
	l.SetCustomDir("/tmp")

	// Loading an unknown agent type should error when trying to load custom prompt
	_, err := l.Load(AgentType("unknown"))
	if err == nil {
		t.Error("expected error for unknown agent type")
	}
}

func TestLoaderLoadWithExtrasError(t *testing.T) {
	l := NewLoader()
	l.SetCustomDir("/tmp")

	extras := map[string]string{
		"Extra": "content",
	}

	// Loading an unknown agent type with extras should error
	_, err := l.LoadWithExtras(AgentType("unknown"), extras)
	if err == nil {
		t.Error("expected error for unknown agent type with extras")
	}
}

// BenchmarkBuilderBuild measures the performance of building prompts.
func BenchmarkBuilderBuild(b *testing.B) {
	builder := NewBuilder().
		AddSection("Section 1", strings.Repeat("Content ", 100)).
		AddSection("Section 2", strings.Repeat("Content ", 100)).
		AddSection("Section 3", strings.Repeat("Content ", 100))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		builder.Build()
	}
}
