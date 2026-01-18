package prompts

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

// AgentType represents the type of agent
type AgentType string

const (
	TypeSupervisor AgentType = "supervisor"
	TypeWorker     AgentType = "worker"
	TypeMergeQueue AgentType = "merge-queue"
)

// Embedded default prompts
//go:embed supervisor.md
var defaultSupervisorPrompt string

//go:embed worker.md
var defaultWorkerPrompt string

//go:embed merge-queue.md
var defaultMergeQueuePrompt string

// GetDefaultPrompt returns the default prompt for the given agent type
func GetDefaultPrompt(agentType AgentType) string {
	switch agentType {
	case TypeSupervisor:
		return defaultSupervisorPrompt
	case TypeWorker:
		return defaultWorkerPrompt
	case TypeMergeQueue:
		return defaultMergeQueuePrompt
	default:
		return ""
	}
}

// LoadCustomPrompt loads a custom prompt from the repository's .multiclaude directory
// Returns empty string if the file doesn't exist
func LoadCustomPrompt(repoPath string, agentType AgentType) (string, error) {
	var filename string
	switch agentType {
	case TypeSupervisor:
		filename = "SUPERVISOR.md"
	case TypeWorker:
		filename = "WORKER.md"
	case TypeMergeQueue:
		filename = "REVIEWER.md"
	default:
		return "", fmt.Errorf("unknown agent type: %s", agentType)
	}

	promptPath := filepath.Join(repoPath, ".multiclaude", filename)

	// Check if file exists
	if _, err := os.Stat(promptPath); os.IsNotExist(err) {
		return "", nil // File doesn't exist, return empty string (not an error)
	}

	// Read the file
	content, err := os.ReadFile(promptPath)
	if err != nil {
		return "", fmt.Errorf("failed to read custom prompt: %w", err)
	}

	return string(content), nil
}

// GetPrompt returns the complete prompt for an agent, combining default and custom prompts
func GetPrompt(repoPath string, agentType AgentType) (string, error) {
	defaultPrompt := GetDefaultPrompt(agentType)

	customPrompt, err := LoadCustomPrompt(repoPath, agentType)
	if err != nil {
		return "", err
	}

	if customPrompt == "" {
		// No custom prompt, return default only
		return defaultPrompt, nil
	}

	// Combine default and custom prompts
	return fmt.Sprintf("%s\n\n---\n\nRepository-specific instructions:\n\n%s", defaultPrompt, customPrompt), nil
}
