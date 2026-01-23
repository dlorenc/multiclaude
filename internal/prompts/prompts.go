package prompts

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dlorenc/multiclaude/internal/prompts/commands"
	"github.com/dlorenc/multiclaude/internal/state"
)

// AgentType is an alias for state.AgentType.
// Deprecated: Use state.AgentType directly instead.
type AgentType = state.AgentType

// Deprecated: Use state.AgentTypeSupervisor directly.
const TypeSupervisor = state.AgentTypeSupervisor

// Deprecated: Use state.AgentTypeWorker directly.
const TypeWorker = state.AgentTypeWorker

// Deprecated: Use state.AgentTypeMergeQueue directly.
const TypeMergeQueue = state.AgentTypeMergeQueue

// Deprecated: Use state.AgentTypeWorkspace directly.
const TypeWorkspace = state.AgentTypeWorkspace

// Deprecated: Use state.AgentTypeReview directly.
const TypeReview = state.AgentTypeReview

// Embedded default prompts
// Only supervisor and workspace are "hardcoded" - other agent types (worker, merge-queue, review)
// should come from configurable agent definitions in agent-templates.
//
//go:embed supervisor.md
var defaultSupervisorPrompt string

//go:embed workspace.md
var defaultWorkspacePrompt string

// GetDefaultPrompt returns the default prompt for the given agent type.
// Only supervisor and workspace have embedded default prompts.
// Worker, merge-queue, and review prompts should come from agent definitions.
func GetDefaultPrompt(agentType state.AgentType) string {
	switch agentType {
	case state.AgentTypeSupervisor:
		return defaultSupervisorPrompt
	case state.AgentTypeWorkspace:
		return defaultWorkspacePrompt
	case state.AgentTypeWorker, state.AgentTypeMergeQueue, state.AgentTypeReview:
		// These agent types should use configurable agent definitions
		// from ~/.multiclaude/repos/<repo>/agents/ or <repo>/.multiclaude/agents/
		return ""
	default:
		return ""
	}
}

// LoadCustomPrompt loads a custom prompt from the repository's .multiclaude directory.
// Returns empty string if the file doesn't exist.
//
// Deprecated: This function is deprecated. Use the configurable agent system instead:
// - Agent definitions: <repo>/.multiclaude/agents/<agent-name>.md
// - Local overrides: ~/.multiclaude/repos/<repo>/agents/<agent-name>.md
// See internal/agents package for the new system.
func LoadCustomPrompt(repoPath string, agentType state.AgentType) (string, error) {
	var filename string
	switch agentType {
	case state.AgentTypeSupervisor:
		filename = "SUPERVISOR.md"
	case state.AgentTypeWorker:
		filename = "WORKER.md"
	case state.AgentTypeMergeQueue:
		filename = "MERGE-QUEUE.md"
	case state.AgentTypeWorkspace:
		filename = "WORKSPACE.md"
	case state.AgentTypeReview:
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
func GetPrompt(repoPath string, agentType state.AgentType, cliDocs string) (string, error) {
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
