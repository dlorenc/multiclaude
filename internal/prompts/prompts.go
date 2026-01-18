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
	TypeWorkspace  AgentType = "workspace"
)

// Embedded default prompts
//go:embed supervisor.md
var defaultSupervisorPrompt string

//go:embed worker.md
var defaultWorkerPrompt string

//go:embed merge-queue.md
var defaultMergeQueuePrompt string

//go:embed workspace.md
var defaultWorkspacePrompt string

// GetDefaultPrompt returns the default prompt for the given agent type
func GetDefaultPrompt(agentType AgentType) string {
	switch agentType {
	case TypeSupervisor:
		return defaultSupervisorPrompt
	case TypeWorker:
		return defaultWorkerPrompt
	case TypeMergeQueue:
		return defaultMergeQueuePrompt
	case TypeWorkspace:
		return defaultWorkspacePrompt
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
	case TypeWorkspace:
		filename = "WORKSPACE.md"
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

// GetPrompt returns the complete prompt for an agent, combining default, custom prompts, and CLI docs
func GetPrompt(repoPath string, agentType AgentType, cliDocs string) (string, error) {
	defaultPrompt := GetDefaultPrompt(agentType)

	customPrompt, err := LoadCustomPrompt(repoPath, agentType)
	if err != nil {
		return "", err
	}

	// Build the complete prompt
	var result string
	result = defaultPrompt

	// Add CLI documentation
	if cliDocs != "" {
		result += fmt.Sprintf("\n\n---\n\n%s", cliDocs)
	}

	// Add custom prompt if it exists
	if customPrompt != "" {
		result += fmt.Sprintf("\n\n---\n\nRepository-specific instructions:\n\n%s", customPrompt)
	}

	return result, nil
}
