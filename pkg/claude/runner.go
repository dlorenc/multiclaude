// Package claude provides utilities for programmatically running Claude Code CLI.
//
// This package abstracts the details of launching and interacting with Claude Code
// instances running in terminal emulators like tmux. It handles:
//
//   - CLI flag construction
//   - Session ID generation
//   - Startup timing quirks
//   - Terminal integration via the TerminalRunner interface
//
// # Quick Start
//
//	import (
//	    "github.com/dlorenc/multiclaude/pkg/claude"
//	    "github.com/dlorenc/multiclaude/pkg/tmux"
//	)
//
//	// Create a tmux client and claude runner
//	tmuxClient := tmux.NewClient()
//	runner := claude.NewRunner(claude.WithTerminal(tmuxClient))
//
//	// Start Claude in a tmux session
//	config := claude.Config{
//	    WorkDir:      "/path/to/workspace",
//	    SystemPrompt: "You are a helpful coding assistant.",
//	}
//	pid, err := runner.Start("my-session", "claude-window", config)
package claude

import (
	"crypto/rand"
	"fmt"
	"os/exec"
	"time"
)

// TerminalRunner abstracts terminal interaction for running Claude.
// The tmux.Client implements this interface.
type TerminalRunner interface {
	// SendKeys sends text followed by Enter to submit.
	SendKeys(session, window, text string) error

	// SendKeysLiteral sends text without pressing Enter (supports multiline via paste-buffer).
	SendKeysLiteral(session, window, text string) error

	// SendEnter sends just the Enter key.
	SendEnter(session, window string) error

	// GetPanePID gets the process ID running in a pane.
	GetPanePID(session, window string) (int, error)

	// StartPipePane starts capturing pane output to a file.
	StartPipePane(session, window, outputFile string) error

	// StopPipePane stops capturing pane output.
	StopPipePane(session, window string) error
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

// Config contains configuration for starting a Claude instance.
type Config struct {
	// SessionID is the unique identifier for this Claude session.
	// If empty, a new UUID will be generated.
	SessionID string

	// Resume indicates this is resuming an existing session.
	// When true, uses --resume instead of --session-id.
	Resume bool

	// WorkDir is the working directory for Claude.
	// If empty, uses the current directory.
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
func (r *Runner) Start(session, window string, cfg Config) (*StartResult, error) {
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
		if err := r.Terminal.StartPipePane(session, window, cfg.OutputFile); err != nil {
			return nil, fmt.Errorf("failed to start output capture: %w", err)
		}
	}

	// Print MOTD before starting Claude - this will be visible when Claude exits
	motd := fmt.Sprintf(`echo "
================================================================================
  multiclaude agent: %s
  session: %s
--------------------------------------------------------------------------------
  If Claude exits, run:  multiclaude claude
  To restart with the same session and context.
================================================================================
"`, window, sessionID)
	if err := r.Terminal.SendKeys(session, window, motd); err != nil {
		// Non-fatal - just log and continue
	}

	// Send the command to start Claude
	if err := r.Terminal.SendKeys(session, window, cmd); err != nil {
		return nil, fmt.Errorf("failed to send claude command: %w", err)
	}

	// Wait for Claude to start
	time.Sleep(r.StartupDelay)

	// Get the PID
	pid, err := r.Terminal.GetPanePID(session, window)
	if err != nil {
		return nil, fmt.Errorf("failed to get Claude PID: %w", err)
	}

	// Send initial message if configured
	if cfg.InitialMessage != "" {
		time.Sleep(r.MessageDelay)
		if err := r.Terminal.SendKeysLiteral(session, window, cfg.InitialMessage); err != nil {
			return nil, fmt.Errorf("failed to send initial message: %w", err)
		}
		if err := r.Terminal.SendEnter(session, window); err != nil {
			return nil, fmt.Errorf("failed to submit initial message: %w", err)
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
	cmd := r.BinaryPath

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
// This properly handles multiline messages using paste-buffer.
func (r *Runner) SendMessage(session, window, message string) error {
	if r.Terminal == nil {
		return fmt.Errorf("terminal runner not configured")
	}

	// Send the message text
	if err := r.Terminal.SendKeysLiteral(session, window, message); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	// Press Enter to submit
	if err := r.Terminal.SendEnter(session, window); err != nil {
		return fmt.Errorf("failed to submit message: %w", err)
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
