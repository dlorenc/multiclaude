// Package claude provides utilities for programmatically running Claude Code CLI.
//
// This package abstracts the details of launching and interacting with Claude Code
// instances running in terminal emulators like tmux. It handles:
//
//   - CLI flag construction
//   - Session ID generation
//   - Startup timing quirks
//   - Terminal integration via the TerminalRunner interface
//   - Context support for cancellation and timeouts
//
// # Quick Start
//
//	import (
//	    "context"
//	    "github.com/dlorenc/multiclaude/pkg/claude"
//	    "github.com/dlorenc/multiclaude/pkg/tmux"
//	)
//
//	// Create a tmux client and claude runner
//	tmuxClient := tmux.NewClient()
//	runner := claude.NewRunner(claude.WithTerminal(tmuxClient))
//
//	// Start Claude in a tmux session
//	ctx := context.Background()
//	config := claude.Config{
//	    WorkDir:      "/path/to/workspace",
//	    SystemPrompt: "You are a helpful coding assistant.",
//	}
//	result, err := runner.Start(ctx, "my-session", "claude-window", config)
//
// # Sending Messages
//
//	// Send a message to a running Claude instance
//	err := runner.SendMessage(ctx, "my-session", "claude-window", "Hello, Claude!")
package claude

import (
	"context"
	"crypto/rand"
	"fmt"
	"os/exec"
	"time"
)

// TerminalRunner abstracts terminal interaction for running Claude.
// The tmux.Client implements this interface.
type TerminalRunner interface {
	// SendKeys sends text followed by Enter to submit.
	SendKeys(ctx context.Context, session, window, text string) error

	// SendKeysLiteral sends text without pressing Enter (supports multiline via paste-buffer).
	SendKeysLiteral(ctx context.Context, session, window, text string) error

	// SendEnter sends just the Enter key.
	SendEnter(ctx context.Context, session, window string) error

	// SendKeysLiteralWithEnter sends text + Enter atomically.
	// This prevents race conditions where Enter might be lost between separate calls.
	SendKeysLiteralWithEnter(ctx context.Context, session, window, text string) error

	// GetPanePID gets the process ID running in a pane.
	GetPanePID(ctx context.Context, session, window string) (int, error)

	// StartPipePane starts capturing pane output to a file.
	StartPipePane(ctx context.Context, session, window, outputFile string) error

	// StopPipePane stops capturing pane output.
	StopPipePane(ctx context.Context, session, window string) error
}

// Runner manages Claude Code instances.
type Runner struct {
	// BinaryPath is the path to the claude binary.
	// Defaults to "claude" (relies on PATH).
	BinaryPath string

	// Terminal is the terminal runner for sending commands.
	Terminal TerminalRunner

	// StartupDelay is how long to wait after starting Claude before
	// attempting to get the PID. Defaults to 500ms.
	StartupDelay time.Duration

	// MessageDelay is how long to wait after startup before sending
	// the first message. Defaults to 1s.
	MessageDelay time.Duration

	// SkipPermissions controls whether to pass --dangerously-skip-permissions.
	// This is required for non-interactive use. Defaults to true.
	SkipPermissions bool
}

// RunnerOption is a functional option for configuring a Runner.
type RunnerOption func(*Runner)

// WithBinaryPath sets a custom path to the claude binary.
func WithBinaryPath(path string) RunnerOption {
	return func(r *Runner) {
		r.BinaryPath = path
	}
}

// WithTerminal sets the terminal runner.
func WithTerminal(t TerminalRunner) RunnerOption {
	return func(r *Runner) {
		r.Terminal = t
	}
}

// WithStartupDelay sets the startup delay.
func WithStartupDelay(d time.Duration) RunnerOption {
	return func(r *Runner) {
		r.StartupDelay = d
	}
}

// WithMessageDelay sets the message delay.
func WithMessageDelay(d time.Duration) RunnerOption {
	return func(r *Runner) {
		r.MessageDelay = d
	}
}

// WithPermissions controls whether to skip permission checks.
// Set to false to require interactive permission prompts.
func WithPermissions(skip bool) RunnerOption {
	return func(r *Runner) {
		r.SkipPermissions = skip
	}
}

// NewRunner creates a new Claude runner with the given options.
func NewRunner(opts ...RunnerOption) *Runner {
	r := &Runner{
		BinaryPath:      "claude",
		StartupDelay:    500 * time.Millisecond,
		MessageDelay:    1 * time.Second,
		SkipPermissions: true,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// ResolveBinaryPath attempts to find the claude binary in PATH.
// Returns the full path if found, otherwise returns "claude".
func ResolveBinaryPath() string {
	if path, err := exec.LookPath("claude"); err == nil {
		return path
	}
	return "claude"
}

// IsBinaryAvailable checks if the Claude CLI is installed and available.
// This is useful for verifying prerequisites before attempting to use the Runner.
// Similar to tmux.Client.IsTmuxAvailable().
func (r *Runner) IsBinaryAvailable() bool {
	cmd := exec.Command(r.BinaryPath, "--version")
	return cmd.Run() == nil
}

// Config contains configuration for starting a Claude instance.
type Config struct {
	// SessionID is the unique identifier for this Claude session.
	// If empty, a new UUID will be generated.
	//
	// Session IDs allow resuming conversations across process restarts.
	// They correlate logs with specific sessions and track concurrent instances.
	SessionID string

	// Resume indicates this is resuming an existing session rather than starting fresh.
	// When true, uses --resume instead of --session-id.
	//
	// Use Resume=true when:
	//   - Restarting an agent after a crash
	//   - Continuing a conversation from a previous run
	//   - The session state was previously saved by Claude
	//
	// Use Resume=false (default) when:
	//   - Starting a new conversation
	//   - The session has never been started before
	Resume bool

	// WorkDir is the working directory for Claude.
	// If non-empty, the command will cd to this directory before launching Claude.
	WorkDir string

	// SystemPromptFile is the path to a file containing the system prompt.
	// This is passed via --append-system-prompt-file.
	SystemPromptFile string

	// InitialMessage is an optional message to send to Claude after startup.
	// If non-empty, sent after MessageDelay.
	InitialMessage string

	// OutputFile is the path to capture Claude's output.
	// If non-empty, StartPipePane is called with this file.
	OutputFile string

	// MOTD is an optional message of the day to display before starting Claude.
	// This is useful for showing restart instructions or other information.
	// If empty, no MOTD is displayed.
	MOTD string
}

// StartResult contains information about a started Claude instance.
type StartResult struct {
	// SessionID is the session ID used for this Claude instance.
	SessionID string

	// PID is the process ID of the Claude process.
	PID int

	// Command is the full command that was executed.
	Command string
}

// Start launches Claude in the specified tmux session/window.
func (r *Runner) Start(ctx context.Context, session, window string, cfg Config) (*StartResult, error) {
	if r.Terminal == nil {
		return nil, fmt.Errorf("terminal runner not configured")
	}

	// Generate session ID if not provided
	sessionID := cfg.SessionID
	if sessionID == "" {
		var err error
		sessionID, err = GenerateSessionID()
		if err != nil {
			return nil, fmt.Errorf("failed to generate session ID: %w", err)
		}
	}

	// Build the command
	cmd := r.buildCommand(sessionID, cfg)

	// Start output capture if configured
	if cfg.OutputFile != "" {
		if err := r.Terminal.StartPipePane(ctx, session, window, cfg.OutputFile); err != nil {
			return nil, fmt.Errorf("failed to start output capture: %w", err)
		}
	}

	// Print MOTD before starting Claude if configured
	if cfg.MOTD != "" {
		motd := fmt.Sprintf("echo %q", cfg.MOTD)
		if err := r.Terminal.SendKeys(ctx, session, window, motd); err != nil {
			// Non-fatal - just continue
		}
	}

	// Send the command to start Claude
	if err := r.Terminal.SendKeys(ctx, session, window, cmd); err != nil {
		return nil, fmt.Errorf("failed to send claude command: %w", err)
	}

	// Wait for Claude to start (respecting context)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(r.StartupDelay):
	}

	// Get the PID
	pid, err := r.Terminal.GetPanePID(ctx, session, window)
	if err != nil {
		return nil, fmt.Errorf("failed to get Claude PID: %w", err)
	}

	// Send initial message if configured
	if cfg.InitialMessage != "" {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(r.MessageDelay):
		}
		if err := r.Terminal.SendKeysLiteralWithEnter(ctx, session, window, cfg.InitialMessage); err != nil {
			return nil, fmt.Errorf("failed to send initial message: %w", err)
		}
	}

	return &StartResult{
		SessionID: sessionID,
		PID:       pid,
		Command:   cmd,
	}, nil
}

// buildCommand constructs the claude CLI command string.
func (r *Runner) buildCommand(sessionID string, cfg Config) string {
	var cmd string

	// If WorkDir is specified, cd to that directory first
	if cfg.WorkDir != "" {
		cmd = fmt.Sprintf("cd %q && ", cfg.WorkDir)
	}

	// Note: CLAUDE_CONFIG_DIR and CLAUDE_CODE_OAUTH_TOKEN are not used because
	// Claude Code only reads credentials from ~/.claude/.credentials.json
	// regardless of CLAUDE_CONFIG_DIR setting. Slash commands go in ~/.claude/commands/.

	cmd += r.BinaryPath

	// Add session ID or resume
	if cfg.Resume {
		cmd += fmt.Sprintf(" --resume %s", sessionID)
	} else {
		cmd += fmt.Sprintf(" --session-id %s", sessionID)
	}

	// Add skip permissions flag
	if r.SkipPermissions {
		cmd += " --dangerously-skip-permissions"
	}

	// Add system prompt file
	if cfg.SystemPromptFile != "" {
		cmd += fmt.Sprintf(" --append-system-prompt-file %s", cfg.SystemPromptFile)
	}

	return cmd
}

// SendMessage sends a message to a running Claude instance.
// This properly handles multiline messages using paste-buffer and sends
// text + Enter atomically to prevent race conditions.
func (r *Runner) SendMessage(ctx context.Context, session, window, message string) error {
	if r.Terminal == nil {
		return fmt.Errorf("terminal runner not configured")
	}

	// Use atomic send for reliability
	if err := r.Terminal.SendKeysLiteralWithEnter(ctx, session, window, message); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}

// GenerateSessionID generates a UUID v4 session ID.
func GenerateSessionID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate session ID: %w", err)
	}

	// Set version (4) and variant bits for UUID v4
	bytes[6] = (bytes[6] & 0x0f) | 0x40 // Version 4
	bytes[8] = (bytes[8] & 0x3f) | 0x80 // Variant 10

	return fmt.Sprintf("%x-%x-%x-%x-%x",
		bytes[0:4],
		bytes[4:6],
		bytes[6:8],
		bytes[8:10],
		bytes[10:16],
	), nil
}
