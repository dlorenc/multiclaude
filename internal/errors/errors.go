// Package errors provides enhanced error handling utilities for better CLI UX.
package errors

import (
	"fmt"
	"strings"
)

// Category represents the type of error for consistent formatting
type Category int

const (
	// CategoryUsage indicates incorrect command usage
	CategoryUsage Category = iota
	// CategoryConfig indicates configuration or setup issues
	CategoryConfig
	// CategoryRuntime indicates operational failures
	CategoryRuntime
	// CategoryConnection indicates daemon/IPC communication issues
	CategoryConnection
	// CategoryNotFound indicates a resource was not found
	CategoryNotFound
)

// CLIError represents an error with additional context for CLI display
type CLIError struct {
	Category   Category
	Message    string
	Suggestion string // Optional hint for how to fix the error
	Cause      error  // Wrapped error
}

// Error implements the error interface
func (e *CLIError) Error() string {
	return e.Message
}

// Unwrap returns the underlying error
func (e *CLIError) Unwrap() error {
	return e.Cause
}

// New creates a new CLIError
func New(category Category, message string) *CLIError {
	return &CLIError{
		Category: category,
		Message:  message,
	}
}

// Wrap wraps an existing error with CLI context
func Wrap(category Category, message string, cause error) *CLIError {
	return &CLIError{
		Category: category,
		Message:  message,
		Cause:    cause,
	}
}

// WithSuggestion adds a suggestion to the error
func (e *CLIError) WithSuggestion(suggestion string) *CLIError {
	e.Suggestion = suggestion
	return e
}

// Format returns a user-friendly formatted error message
func Format(err error) string {
	if err == nil {
		return ""
	}

	var sb strings.Builder

	// Check if it's a CLIError
	if cliErr, ok := err.(*CLIError); ok {
		// Add category prefix
		prefix := categoryPrefix(cliErr.Category)
		sb.WriteString(prefix)
		sb.WriteString(cliErr.Message)

		// Add cause if present
		if cliErr.Cause != nil {
			sb.WriteString(": ")
			sb.WriteString(cliErr.Cause.Error())
		}

		// Add suggestion if present
		if cliErr.Suggestion != "" {
			sb.WriteString("\n\nTry: ")
			sb.WriteString(cliErr.Suggestion)
		}
	} else {
		// Regular error - format with generic prefix
		sb.WriteString("Error: ")
		sb.WriteString(err.Error())
	}

	return sb.String()
}

// categoryPrefix returns the prefix for each error category
func categoryPrefix(cat Category) string {
	switch cat {
	case CategoryUsage:
		return "Usage error: "
	case CategoryConfig:
		return "Configuration error: "
	case CategoryRuntime:
		return "Error: "
	case CategoryConnection:
		return "Connection error: "
	case CategoryNotFound:
		return "Not found: "
	default:
		return "Error: "
	}
}

// Common error constructors for frequently used patterns

// DaemonNotRunning creates an error for when the daemon is not running
func DaemonNotRunning() *CLIError {
	return &CLIError{
		Category:   CategoryConnection,
		Message:    "daemon is not running",
		Suggestion: "multiclaude start",
	}
}

// DaemonCommunicationFailed creates an error for daemon communication failures
func DaemonCommunicationFailed(operation string, cause error) *CLIError {
	return &CLIError{
		Category:   CategoryConnection,
		Message:    fmt.Sprintf("failed to communicate with daemon while %s", operation),
		Cause:      cause,
		Suggestion: "multiclaude daemon status",
	}
}

// InvalidUsage creates an error for invalid command usage
func InvalidUsage(usage string) *CLIError {
	return &CLIError{
		Category: CategoryUsage,
		Message:  usage,
	}
}

// NotInRepo creates an error for when user is not in a tracked repository
func NotInRepo() *CLIError {
	return &CLIError{
		Category:   CategoryConfig,
		Message:    "not in a tracked repository",
		Suggestion: "multiclaude init <github-url> to track a repository, or use --repo flag",
	}
}

// MultipleRepos creates an error for when multiple repos exist and none specified
func MultipleRepos() *CLIError {
	return &CLIError{
		Category:   CategoryUsage,
		Message:    "multiple repositories are tracked",
		Suggestion: "use --repo flag to specify which repository",
	}
}

// AgentNotFound creates an error for when an agent is not found
func AgentNotFound(agentType, name, repo string) *CLIError {
	return &CLIError{
		Category:   CategoryNotFound,
		Message:    fmt.Sprintf("%s '%s' not found in repository '%s'", agentType, name, repo),
		Suggestion: fmt.Sprintf("multiclaude work list --repo %s", repo),
	}
}

// InvalidPRURL creates an error for invalid PR URLs
func InvalidPRURL() *CLIError {
	return &CLIError{
		Category:   CategoryUsage,
		Message:    "invalid PR URL format",
		Suggestion: "use format: https://github.com/owner/repo/pull/123",
	}
}

// GitOperationFailed creates an error for git operation failures
func GitOperationFailed(operation string, cause error) *CLIError {
	return &CLIError{
		Category:   CategoryRuntime,
		Message:    fmt.Sprintf("git %s failed", operation),
		Cause:      cause,
		Suggestion: "check git status and ensure the repository is in a clean state",
	}
}

// TmuxOperationFailed creates an error for tmux operation failures with specific suggestions
func TmuxOperationFailed(operation string, cause error) *CLIError {
	suggestion := tmuxSuggestionForOperation(operation, cause)
	return &CLIError{
		Category:   CategoryRuntime,
		Message:    fmt.Sprintf("tmux %s failed", operation),
		Cause:      cause,
		Suggestion: suggestion,
	}
}

// tmuxSuggestionForOperation provides specific suggestions based on the operation and error
func tmuxSuggestionForOperation(operation string, cause error) string {
	errMsg := ""
	if cause != nil {
		errMsg = cause.Error()
	}

	// tmux binary not found
	if strings.Contains(errMsg, "executable file not found") || strings.Contains(errMsg, "not found in") {
		return "could not find 'tmux' binary in PATH"
	}

	// Session already exists
	if strings.Contains(errMsg, "duplicate session") || strings.Contains(errMsg, "already exists") {
		return "a tmux session with this name already exists; kill it with: tmux kill-session -t <session-name>"
	}

	// Default: no specific suggestion
	return ""
}

// WorktreeCreationFailed creates an error for worktree creation failures
func WorktreeCreationFailed(cause error) *CLIError {
	return &CLIError{
		Category:   CategoryRuntime,
		Message:    "failed to create git worktree",
		Cause:      cause,
		Suggestion: "check disk space and git repository state",
	}
}

// ClaudeNotFound creates an error for when Claude binary is not found
func ClaudeNotFound(cause error) *CLIError {
	return &CLIError{
		Category:   CategoryConfig,
		Message:    "claude binary not found in PATH",
		Cause:      cause,
		Suggestion: "install Claude Code CLI: https://docs.anthropic.com/claude-code",
	}
}

// MissingArgument creates an error for missing required arguments
func MissingArgument(argName, expectedType string) *CLIError {
	msg := fmt.Sprintf("missing required argument: %s", argName)
	if expectedType != "" {
		msg = fmt.Sprintf("missing required argument: %s (%s)", argName, expectedType)
	}
	return &CLIError{
		Category: CategoryUsage,
		Message:  msg,
	}
}

// InvalidArgument creates an error for invalid argument values
func InvalidArgument(argName, value, expected string) *CLIError {
	return &CLIError{
		Category: CategoryUsage,
		Message:  fmt.Sprintf("invalid value for '%s': got '%s', expected %s", argName, value, expected),
	}
}

// NotInAgentContext creates an error for commands run outside agent context
func NotInAgentContext() *CLIError {
	return &CLIError{
		Category:   CategoryConfig,
		Message:    "not in a multiclaude agent directory",
		Suggestion: "run this command from within an agent's tmux window",
	}
}

// UnknownCommand creates an error for unknown commands
func UnknownCommand(cmd string) *CLIError {
	return &CLIError{
		Category:   CategoryUsage,
		Message:    fmt.Sprintf("unknown command: %s", cmd),
		Suggestion: "multiclaude --help",
	}
}
