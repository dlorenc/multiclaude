package prompts

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dlorenc/multiclaude/internal/prompts/commands"
	"github.com/dlorenc/multiclaude/internal/provider"
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

// Deprecated: Use state.AgentTypePRShepherd directly.
const TypePRShepherd = state.AgentTypePRShepherd

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
	case state.AgentTypePRShepherd:
		filename = "PR-SHEPHERD.md"
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
// This version defaults to GitHub commands for backward compatibility.
func GenerateTrackingModePrompt(trackMode string) string {
	return GenerateTrackingModePromptWithProvider(trackMode, state.ProviderGitHub, nil)
}

// GenerateTrackingModePromptWithProvider generates provider-aware prompt text explaining which PRs to track.
func GenerateTrackingModePromptWithProvider(trackMode string, providerType state.ProviderType, provConfig *state.ProviderConfig) string {
	// Get the appropriate provider for command generation
	var prov provider.Provider
	if providerType == state.ProviderAzureDevOps && provConfig != nil {
		prov = provider.NewAzureDevOpsWithConfig(provConfig.Organization, provConfig.Project, provConfig.RepoName)
	} else {
		prov = provider.NewGitHub()
	}

	switch trackMode {
	case "author":
		return fmt.Sprintf(`## PR Tracking Mode: Author Only

**IMPORTANT**: This repository is configured to track only PRs where you (or the multiclaude system) are the author.

When listing and monitoring PRs, use:
`+"```bash"+`
%s
`+"```"+`

Do NOT process or attempt to merge PRs authored by others. Focus only on PRs created by multiclaude workers.`,
			prov.PRListCommand("multiclaude", "@me"))

	case "assigned":
		// Note: ADO doesn't have assignee filter in the same way as GitHub
		if providerType == state.ProviderAzureDevOps {
			return fmt.Sprintf(`## PR Tracking Mode: Assigned Only

**IMPORTANT**: This repository is configured to track only PRs where you (or the multiclaude system) are the reviewer.

When listing and monitoring PRs, filter for PRs where you are a reviewer:
`+"```bash"+`
%s
`+"```"+`

Do NOT process or attempt to complete PRs unless they are assigned to you for review.`,
				prov.PRListCommand("multiclaude", ""))
		}
		// GitHub: Use --assignee flag (not --author) for assigned mode
		return `## PR Tracking Mode: Assigned Only

**IMPORTANT**: This repository is configured to track only PRs where you (or the multiclaude system) are assigned.

When listing and monitoring PRs, use:
` + "```bash" + `
gh pr list --assignee @me --label multiclaude
` + "```" + `

Do NOT process or attempt to merge PRs unless they are assigned to you. Focus only on PRs explicitly assigned to multiclaude.`

	default: // "all"
		return fmt.Sprintf(`## PR Tracking Mode: All PRs

This repository is configured to track all PRs with the multiclaude label.

When listing and monitoring PRs, use:
`+"```bash"+`
%s
`+"```"+`

Monitor and process all multiclaude-labeled PRs regardless of author or assignee.`,
			prov.PRListCommand("multiclaude", ""))
	}
}

// GenerateForkWorkflowPrompt generates prompt text explaining fork-based workflow.
// This is injected into all agent prompts when working in a fork.
// This version defaults to GitHub commands for backward compatibility.
func GenerateForkWorkflowPrompt(upstreamOwner, upstreamRepo, forkOwner string) string {
	return GenerateForkWorkflowPromptWithProvider(upstreamOwner, upstreamRepo, forkOwner, state.ProviderGitHub, nil)
}

// GenerateForkWorkflowPromptWithProvider generates provider-aware prompt text for fork workflow.
func GenerateForkWorkflowPromptWithProvider(upstreamOwner, upstreamRepo, forkOwner string, providerType state.ProviderType, provConfig *state.ProviderConfig) string {
	// Azure DevOps forks work differently - provide different guidance
	if providerType == state.ProviderAzureDevOps {
		return fmt.Sprintf(`## Fork Workflow (Azure DevOps)

You are working in a fork repository.

**Key differences from main repository workflow:**

### Git Remotes
- **origin**: Your fork - push branches here
- **upstream**: Original repo - PRs target this repo

### Creating PRs
PRs should target the upstream repository. In Azure DevOps, you create PRs through the web UI or API:

` + "```bash" + `
# Fetch upstream changes first
git fetch upstream main

# Create your branch and push to origin
git checkout -b feature-branch
# ... make changes ...
git push origin feature-branch

# Then create a PR via the Azure DevOps API or web UI
# The PR should target the upstream repository
` + "```" + `

### Keeping Synced
Keep your fork updated with upstream:
` + "```bash" + `
# Fetch upstream changes
git fetch upstream main

# Rebase your work
git rebase upstream/main

# Update your fork's main
git checkout main && git merge --ff-only upstream/main && git push origin main
` + "```" + `

### Important Notes
- **You cannot complete PRs** - upstream maintainers do that
- Create branches on your fork (origin), target upstream for PRs
- Keep rebasing onto upstream/main to avoid conflicts
- Use the Azure DevOps REST API with AZURE_DEVOPS_PAT for automation
`)
	}

	// GitHub fork workflow (default)
	return fmt.Sprintf(`## Fork Workflow (Auto-detected)

You are working in a fork of **%s/%s**.

**Key differences from upstream workflow:**

### Git Remotes
- **origin**: Your fork (%s/%s) - push branches here
- **upstream**: Original repo (%s/%s) - PRs target this repo

### Creating PRs
PRs should target the upstream repository, not your fork:
`+"```bash"+`
# Create a PR targeting upstream
gh pr create --repo %s/%s --head %s:<branch-name>

# View your PRs on upstream
gh pr list --repo %s/%s --author @me
`+"```"+`

### Keeping Synced
Keep your fork updated with upstream:
`+"```bash"+`
# Fetch upstream changes
git fetch upstream main

# Rebase your work
git rebase upstream/main

# Update your fork's main
git checkout main && git merge --ff-only upstream/main && git push origin main
`+"```"+`

### Important Notes
- **You cannot merge PRs** - upstream maintainers do that
- Create branches on your fork (origin), target upstream for PRs
- Keep rebasing onto upstream/main to avoid conflicts
- The pr-shepherd agent handles getting PRs ready for review
`, upstreamOwner, upstreamRepo,
		forkOwner, upstreamRepo,
		upstreamOwner, upstreamRepo,
		upstreamOwner, upstreamRepo, forkOwner,
		upstreamOwner, upstreamRepo)
}

// GenerateProviderInfoPrompt generates prompt text explaining the git hosting provider.
// This helps agents understand which commands to use.
func GenerateProviderInfoPrompt(providerType state.ProviderType, provConfig *state.ProviderConfig) string {
	if providerType == state.ProviderAzureDevOps && provConfig != nil {
		return fmt.Sprintf(`## Git Hosting Provider: Azure DevOps

This repository is hosted on **Azure DevOps**.

**Organization**: %s
**Project**: %s
**Repository**: %s

### Authentication
All Azure DevOps API calls require the AZURE_DEVOPS_PAT environment variable to be set.

### Key Differences from GitHub
- Use curl with the Azure DevOps REST API instead of the 'gh' CLI
- PRs are called "Pull Requests" and use a different API
- Labels are called "tags" in Azure DevOps
- CI pipelines work differently than GitHub Actions

### Common Commands
`+"```bash"+`
# List open PRs
curl -s -u ":$AZURE_DEVOPS_PAT" "https://dev.azure.com/%s/%s/_apis/git/repositories/%s/pullrequests?api-version=7.0&searchCriteria.status=active"

# View PR details
curl -s -u ":$AZURE_DEVOPS_PAT" "https://dev.azure.com/%s/%s/_apis/git/pullrequests/{PR_NUMBER}?api-version=7.0"

# Add a PR comment
curl -s -u ":$AZURE_DEVOPS_PAT" -X POST "https://dev.azure.com/%s/%s/_apis/git/repositories/%s/pullrequests/{PR_NUMBER}/threads?api-version=7.0" \
  -H "Content-Type: application/json" \
  -d '{"comments":[{"content":"Your comment here"}]}'
`+"```"+`
`,
			provConfig.Organization, provConfig.Project, provConfig.RepoName,
			provConfig.Organization, provConfig.Project, provConfig.RepoName,
			provConfig.Organization, provConfig.Project,
			provConfig.Organization, provConfig.Project, provConfig.RepoName)
	}

	// GitHub is the default - no special prompt needed as agents assume GitHub by default
	return ""
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
