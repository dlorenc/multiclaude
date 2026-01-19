// Package tmux provides a Go client for interacting with tmux terminal multiplexer.
//
// This package differs from existing Go tmux libraries (gotmux, go-tmux, gomux) by
// focusing on programmatic interaction with running CLI applications:
//
//   - Multiline text input via paste-buffer (avoids triggering intermediate processing)
//   - Process PID extraction from panes
//   - Output capture via pipe-pane
//
// # Quick Start
//
//	client := tmux.NewClient()
//
//	// Check if tmux is available
//	if !client.IsTmuxAvailable() {
//	    log.Fatal("tmux is not installed")
//	}
//
//	// Create a detached session
//	if err := client.CreateSession("my-session", true); err != nil {
//	    log.Fatal(err)
//	}
//
//	// Send commands to the session
//	if err := client.SendKeys("my-session", "0", "echo hello"); err != nil {
//	    log.Fatal(err)
//	}
package tmux

import (
	"fmt"
	"os/exec"
	"strings"
)

// Client wraps tmux operations for programmatic control of tmux sessions,
// windows, and panes.
type Client struct {
	// tmuxPath allows overriding the default "tmux" binary path.
	// If empty, "tmux" is used (relies on PATH).
	tmuxPath string
}

// ClientOption is a functional option for configuring a Client.
type ClientOption func(*Client)

// WithTmuxPath sets a custom path to the tmux binary.
func WithTmuxPath(path string) ClientOption {
	return func(c *Client) {
		c.tmuxPath = path
	}
}

// NewClient creates a new tmux client with the given options.
func NewClient(opts ...ClientOption) *Client {
	c := &Client{
		tmuxPath: "tmux",
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// tmuxCmd creates an exec.Cmd for the configured tmux binary.
func (c *Client) tmuxCmd(args ...string) *exec.Cmd {
	return exec.Command(c.tmuxPath, args...)
}

// IsTmuxAvailable checks if tmux is installed and available.
func (c *Client) IsTmuxAvailable() bool {
	cmd := c.tmuxCmd("-V")
	return cmd.Run() == nil
}

// =============================================================================
// Session Management
// =============================================================================

// HasSession checks if a tmux session with the given name exists.
func (c *Client) HasSession(name string) (bool, error) {
	cmd := c.tmuxCmd("has-session", "-t", name)
	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Exit code 1 means session doesn't exist
			if exitErr.ExitCode() == 1 {
				return false, nil
			}
		}
		return false, fmt.Errorf("failed to check session: %w", err)
	}
	return true, nil
}

// CreateSession creates a new tmux session with the given name.
// If detached is true, creates the session in detached mode (-d).
func (c *Client) CreateSession(name string, detached bool) error {
	args := []string{"new-session", "-s", name}
	if detached {
		args = append(args, "-d")
	}

	cmd := c.tmuxCmd(args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	return nil
}

// KillSession terminates a tmux session.
func (c *Client) KillSession(name string) error {
	cmd := c.tmuxCmd("kill-session", "-t", name)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to kill session: %w", err)
	}
	return nil
}

// ListSessions returns a list of all tmux session names.
func (c *Client) ListSessions() ([]string, error) {
	cmd := c.tmuxCmd("list-sessions", "-F", "#{session_name}")
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// No sessions running
			if exitErr.ExitCode() == 1 {
				return []string{}, nil
			}
		}
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}

	sessions := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(sessions) == 1 && sessions[0] == "" {
		return []string{}, nil
	}
	return sessions, nil
}

// =============================================================================
// Window Management
// =============================================================================

// CreateWindow creates a new window in the specified session.
func (c *Client) CreateWindow(session, windowName string) error {
	target := fmt.Sprintf("%s:", session)
	cmd := c.tmuxCmd("new-window", "-t", target, "-n", windowName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create window: %w", err)
	}
	return nil
}

// HasWindow checks if a window with the given name exists in the session.
func (c *Client) HasWindow(session, windowName string) (bool, error) {
	cmd := c.tmuxCmd("list-windows", "-t", session)
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to list windows: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, windowName) {
			return true, nil
		}
	}
	return false, nil
}

// KillWindow terminates a specific window in a session.
func (c *Client) KillWindow(session, windowName string) error {
	target := fmt.Sprintf("%s:%s", session, windowName)
	cmd := c.tmuxCmd("kill-window", "-t", target)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to kill window: %w", err)
	}
	return nil
}

// ListWindows returns a list of window names in the specified session.
func (c *Client) ListWindows(session string) ([]string, error) {
	cmd := c.tmuxCmd("list-windows", "-t", session, "-F", "#{window_name}")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list windows: %w", err)
	}

	windows := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(windows) == 1 && windows[0] == "" {
		return []string{}, nil
	}
	return windows, nil
}

// =============================================================================
// Text Input - The Key Differentiator
// =============================================================================

// SendKeys sends text to a window followed by Enter (C-m).
// This is equivalent to typing the text and pressing Enter.
func (c *Client) SendKeys(session, windowName, text string) error {
	target := fmt.Sprintf("%s:%s", session, windowName)
	cmd := c.tmuxCmd("send-keys", "-t", target, text, "C-m")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to send keys: %w", err)
	}
	return nil
}

// SendKeysLiteral sends text to a window without pressing Enter.
//
// For single-line text, this uses tmux's send-keys with -l (literal mode).
// For multiline text, it uses tmux's paste buffer mechanism to send the
// entire message at once without triggering intermediate command processing.
//
// This is the key differentiator from other tmux libraries - it properly
// handles multiline text when interacting with CLI applications that might
// interpret newlines as command submission.
func (c *Client) SendKeysLiteral(session, windowName, text string) error {
	target := fmt.Sprintf("%s:%s", session, windowName)

	// For multiline text, use paste buffer to avoid triggering processing on each line
	if strings.Contains(text, "\n") {
		// Set the buffer with the text
		setCmd := c.tmuxCmd("set-buffer", text)
		if err := setCmd.Run(); err != nil {
			return fmt.Errorf("failed to set buffer: %w", err)
		}

		// Paste the buffer to the target
		pasteCmd := c.tmuxCmd("paste-buffer", "-t", target)
		if err := pasteCmd.Run(); err != nil {
			return fmt.Errorf("failed to paste buffer: %w", err)
		}
		return nil
	}

	// No newlines, send the text using send-keys with literal mode
	cmd := c.tmuxCmd("send-keys", "-t", target, "-l", text)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to send keys: %w", err)
	}
	return nil
}

// SendEnter sends just the Enter key (C-m) to a window.
// Useful when you want to send text with SendKeysLiteral and then
// separately trigger command execution.
func (c *Client) SendEnter(session, windowName string) error {
	target := fmt.Sprintf("%s:%s", session, windowName)
	cmd := c.tmuxCmd("send-keys", "-t", target, "C-m")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to send enter: %w", err)
	}
	return nil
}

// SendKeysLiteralWithEnter sends text + Enter atomically using shell command chaining.
// This prevents race conditions where Enter might be lost between separate exec calls.
// Uses sh -c with && to chain tmux commands in a single shell execution.
// This approach works reliably for both single-line and multiline messages.
func (c *Client) SendKeysLiteralWithEnter(session, windowName, text string) error {
	target := fmt.Sprintf("%s:%s", session, windowName)

	// Use sh -c to chain tmux commands atomically with &&
	// The text is passed as $1 to avoid shell escaping issues with special characters
	// Commands: set-buffer (load text) -> paste-buffer (insert to pane) -> send-keys Enter (submit)
	cmdStr := fmt.Sprintf("%s set-buffer -- \"$1\" && %s paste-buffer -t %s && %s send-keys -t %s Enter",
		c.tmuxPath, c.tmuxPath, target, c.tmuxPath, target)
	cmd := exec.Command("sh", "-c", cmdStr, "sh", text)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to send keys atomically: %w", err)
	}
	return nil
}

// =============================================================================
// Process Monitoring - Another Differentiator
// =============================================================================

// GetPanePID gets the PID of the process running in the first pane of a window.
// This allows monitoring whether the process in a tmux pane is still alive.
func (c *Client) GetPanePID(session, windowName string) (int, error) {
	target := fmt.Sprintf("%s:%s", session, windowName)
	cmd := c.tmuxCmd("display-message", "-t", target, "-p", "#{pane_pid}")
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to get pane PID: %w", err)
	}

	var pid int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &pid); err != nil {
		return 0, fmt.Errorf("failed to parse PID: %w", err)
	}

	return pid, nil
}

// =============================================================================
// Output Capture - Third Differentiator
// =============================================================================

// StartPipePane starts capturing pane output to a file.
// The output is appended to the file, so it persists across restarts.
//
// Example:
//
//	client.StartPipePane("my-session", "my-window", "/tmp/output.log")
//	// ... run commands in the pane ...
//	client.StopPipePane("my-session", "my-window")
func (c *Client) StartPipePane(session, windowName, outputFile string) error {
	target := fmt.Sprintf("%s:%s", session, windowName)
	// Use -o to open a pipe (output only, not input)
	// cat >> appends to the file so output is preserved
	cmd := c.tmuxCmd("pipe-pane", "-o", "-t", target, fmt.Sprintf("cat >> '%s'", outputFile))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start pipe-pane: %w", err)
	}
	return nil
}

// StopPipePane stops the pipe-pane for a window.
// After calling this, output is no longer captured to the file.
func (c *Client) StopPipePane(session, windowName string) error {
	target := fmt.Sprintf("%s:%s", session, windowName)
	// Running pipe-pane with no command stops any existing pipe
	cmd := c.tmuxCmd("pipe-pane", "-t", target)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to stop pipe-pane: %w", err)
	}
	return nil
}
