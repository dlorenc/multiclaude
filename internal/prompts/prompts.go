package prompts

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dlorenc/multiclaude/internal/prompts/commands"
)

// AgentType represents the type of agent
type AgentType string

const (
	TypeSupervisor AgentType = "supervisor"
	TypeWorker     AgentType = "worker"
	TypeMergeQueue AgentType = "merge-queue"
	TypeWorkspace  AgentType = "workspace"
	TypeReview     AgentType = "review"
)

// Embedded default prompts
//
//go:embed supervisor.md
var defaultSupervisorPrompt string

//go:embed worker.md
var defaultWorkerPrompt string

//go:embed merge-queue.md
var defaultMergeQueuePrompt string

//go:embed workspace.md
var defaultWorkspacePrompt string

//go:embed review.md
var defaultReviewPrompt string

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
	case TypeReview:
		return defaultReviewPrompt
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
	case TypeReview:
		filename = "REVIEW.md"
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

// GetPrompt returns the complete prompt for an agent, combining default, custom prompts, CLI docs, and slash commands
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

	// Add slash commands section
	slashCommands := GetSlashCommandsPrompt()
	if slashCommands != "" {
		result += fmt.Sprintf("\n\n---\n\n%s", slashCommands)
	}

	// Add custom prompt if it exists
	if customPrompt != "" {
		result += fmt.Sprintf("\n\n---\n\nRepository-specific instructions:\n\n%s", customPrompt)
	}

	return result, nil
}

// GenerateTrackingModePrompt generates prompt text explaining which PRs to track
// based on the tracking mode. The trackMode parameter should be "all", "author", or "assigned".
func GenerateTrackingModePrompt(trackMode string) string {
	switch trackMode {
	case "author":
		return `## PR Tracking Mode: Author Only

**IMPORTANT**: This repository is configured to track only PRs where you (or the multiclaude system) are the author.

When listing and monitoring PRs, use:
` + "```bash" + `
gh pr list --author @me --label multiclaude
` + "```" + `

Do NOT process or attempt to merge PRs authored by others. Focus only on PRs created by multiclaude workers.`

	case "assigned":
		return `## PR Tracking Mode: Assigned Only

**IMPORTANT**: This repository is configured to track only PRs where you (or the multiclaude system) are assigned.

When listing and monitoring PRs, use:
` + "```bash" + `
gh pr list --assignee @me --label multiclaude
` + "```" + `

Do NOT process or attempt to merge PRs unless they are assigned to you. Focus only on PRs explicitly assigned to multiclaude.`

	default: // "all"
		return `## PR Tracking Mode: All PRs

This repository is configured to track all PRs with the multiclaude label.

When listing and monitoring PRs, use:
` + "```bash" + `
gh pr list --label multiclaude
` + "```" + `

Monitor and process all multiclaude-labeled PRs regardless of author or assignee.`
	}
}

// GetSlashCommandsPrompt returns a formatted prompt section containing all available
// slash commands. This can be included in agent prompts to document the available
// commands.
func GetSlashCommandsPrompt() string {
	var builder strings.Builder

	builder.WriteString("## Slash Commands\n\n")
	builder.WriteString("The following slash commands are available for use:\n\n")

	for _, cmd := range commands.AvailableCommands {
		content, err := commands.GetCommand(cmd.Name)
		if err != nil {
			continue
		}
		builder.WriteString(content)
		builder.WriteString("\n---\n\n")
	}

	return builder.String()
}
