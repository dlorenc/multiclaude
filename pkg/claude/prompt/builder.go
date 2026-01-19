// Package prompt provides utilities for building Claude system prompts.
//
// This package helps construct layered prompts by combining:
//   - Default prompts (embedded in code)
//   - Custom prompts (from repository configuration)
//   - Dynamic sections (CLI docs, context, etc.)
//
// # Quick Start
//
//	builder := prompt.NewBuilder()
//	builder.AddSection("Role", "You are a helpful coding assistant.")
//	builder.AddSection("Guidelines", "Write clean, readable code.")
//	text := builder.Build()
package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Builder constructs layered system prompts for Claude.
type Builder struct {
	sections []section
}

type section struct {
	header  string
	content string
}

// NewBuilder creates a new prompt builder.
func NewBuilder() *Builder {
	return &Builder{}
}

// AddSection adds a named section to the prompt.
// The section header will be formatted as a markdown header (## header).
func (b *Builder) AddSection(header, content string) *Builder {
	if content != "" {
		b.sections = append(b.sections, section{header: header, content: content})
	}
	return b
}

// AddRaw adds raw content without a section header.
func (b *Builder) AddRaw(content string) *Builder {
	if content != "" {
		b.sections = append(b.sections, section{content: content})
	}
	return b
}

// Build constructs the final prompt string.
func (b *Builder) Build() string {
	var parts []string
	for _, s := range b.sections {
		if s.header != "" {
			parts = append(parts, fmt.Sprintf("## %s\n\n%s", s.header, s.content))
		} else {
			parts = append(parts, s.content)
		}
	}
	return strings.Join(parts, "\n\n---\n\n")
}

// Len returns the number of sections in the builder.
func (b *Builder) Len() int {
	return len(b.sections)
}

// Clear removes all sections from the builder.
func (b *Builder) Clear() *Builder {
	b.sections = nil
	return b
}

// AgentType represents the type of agent for prompt loading.
type AgentType string

// Standard agent types.
const (
	TypeSupervisor AgentType = "supervisor"
	TypeWorker     AgentType = "worker"
	TypeMergeQueue AgentType = "merge-queue"
	TypeWorkspace  AgentType = "workspace"
	TypeReview     AgentType = "review"
)

// Loader loads prompts from the filesystem.
type Loader struct {
	// DefaultPrompts maps agent types to their default prompts.
	// These are typically embedded in the application.
	DefaultPrompts map[AgentType]string

	// CustomPromptDir is the directory to look for custom prompts.
	// Custom prompts are loaded from <dir>/<AGENT_TYPE>.md
	CustomPromptDir string
}

// NewLoader creates a new prompt loader.
func NewLoader() *Loader {
	return &Loader{
		DefaultPrompts: make(map[AgentType]string),
	}
}

// SetDefault sets the default prompt for an agent type.
func (l *Loader) SetDefault(agentType AgentType, content string) *Loader {
	l.DefaultPrompts[agentType] = content
	return l
}

// SetCustomDir sets the directory for custom prompts.
func (l *Loader) SetCustomDir(dir string) *Loader {
	l.CustomPromptDir = dir
	return l
}

// customPromptFilename returns the filename for a custom prompt.
func customPromptFilename(agentType AgentType) string {
	switch agentType {
	case TypeSupervisor:
		return "SUPERVISOR.md"
	case TypeWorker:
		return "WORKER.md"
	case TypeMergeQueue:
		return "REVIEWER.md"
	case TypeWorkspace:
		return "WORKSPACE.md"
	case TypeReview:
		return "REVIEW.md"
	default:
		return ""
	}
}

// LoadCustom loads a custom prompt from the configured directory.
// Returns empty string if the file doesn't exist.
func (l *Loader) LoadCustom(agentType AgentType) (string, error) {
	if l.CustomPromptDir == "" {
		return "", nil
	}

	filename := customPromptFilename(agentType)
	if filename == "" {
		return "", fmt.Errorf("unknown agent type: %s", agentType)
	}

	path := filepath.Join(l.CustomPromptDir, filename)

	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read custom prompt: %w", err)
	}

	return string(content), nil
}

// Load loads the complete prompt for an agent type.
// It combines the default prompt with any custom prompt from the configured directory.
func (l *Loader) Load(agentType AgentType) (string, error) {
	builder := NewBuilder()

	// Add default prompt
	if defaultPrompt, ok := l.DefaultPrompts[agentType]; ok {
		builder.AddRaw(defaultPrompt)
	}

	// Add custom prompt if it exists
	customPrompt, err := l.LoadCustom(agentType)
	if err != nil {
		return "", err
	}
	if customPrompt != "" {
		builder.AddSection("Repository-specific instructions", customPrompt)
	}

	return builder.Build(), nil
}

// LoadWithExtras loads a prompt with additional sections.
// extras is a map of section headers to content.
func (l *Loader) LoadWithExtras(agentType AgentType, extras map[string]string) (string, error) {
	builder := NewBuilder()

	// Add default prompt
	if defaultPrompt, ok := l.DefaultPrompts[agentType]; ok {
		builder.AddRaw(defaultPrompt)
	}

	// Add extra sections
	for header, content := range extras {
		builder.AddSection(header, content)
	}

	// Add custom prompt if it exists
	customPrompt, err := l.LoadCustom(agentType)
	if err != nil {
		return "", err
	}
	if customPrompt != "" {
		builder.AddSection("Repository-specific instructions", customPrompt)
	}

	return builder.Build(), nil
}

// WriteToFile writes the prompt to a file.
func WriteToFile(path, content string) error {
	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create prompt directory: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write prompt file: %w", err)
	}

	return nil
}
