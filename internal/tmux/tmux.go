package tmux

import (
	"fmt"
	"os/exec"
	"strings"
)

// Client wraps tmux operations
type Client struct{}

// NewClient creates a new tmux client
func NewClient() *Client {
	return &Client{}
}

// HasSession checks if a tmux session exists
func (c *Client) HasSession(name string) (bool, error) {
	cmd := exec.Command("tmux", "has-session", "-t", name)
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

// CreateSession creates a new tmux session
// If detached is true, creates session in detached mode
func (c *Client) CreateSession(name string, detached bool) error {
	args := []string{"new-session", "-s", name}
	if detached {
		args = append(args, "-d")
	}

	cmd := exec.Command("tmux", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	return nil
}

// CreateWindow creates a new window in a session
func (c *Client) CreateWindow(session, windowName string) error {
	target := fmt.Sprintf("%s:", session)
	cmd := exec.Command("tmux", "new-window", "-t", target, "-n", windowName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create window: %w", err)
	}
	return nil
}

// HasWindow checks if a window exists in a session
func (c *Client) HasWindow(session, windowName string) (bool, error) {
	cmd := exec.Command("tmux", "list-windows", "-t", session)
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

// KillWindow kills a window in a session
func (c *Client) KillWindow(session, windowName string) error {
	target := fmt.Sprintf("%s:%s", session, windowName)
	cmd := exec.Command("tmux", "kill-window", "-t", target)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to kill window: %w", err)
	}
	return nil
}

// KillSession kills a tmux session
func (c *Client) KillSession(name string) error {
	cmd := exec.Command("tmux", "kill-session", "-t", name)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to kill session: %w", err)
	}
	return nil
}

// SendKeys sends text to a window followed by Enter
func (c *Client) SendKeys(session, windowName, text string) error {
	target := fmt.Sprintf("%s:%s", session, windowName)
	cmd := exec.Command("tmux", "send-keys", "-t", target, text, "C-m")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to send keys: %w", err)
	}
	return nil
}

// SendKeysLiteral sends text to a window without Enter (using -l for literal mode)
// For multiline text, it uses tmux's paste buffer to send the entire message at once
// without triggering intermediate processing.
func (c *Client) SendKeysLiteral(session, windowName, text string) error {
	target := fmt.Sprintf("%s:%s", session, windowName)

	// For multiline text, use paste buffer to avoid triggering processing on each line
	if strings.Contains(text, "\n") {
		// Set the buffer with the text
		setCmd := exec.Command("tmux", "set-buffer", text)
		if err := setCmd.Run(); err != nil {
			return fmt.Errorf("failed to set buffer: %w", err)
		}

		// Paste the buffer to the target
		pasteCmd := exec.Command("tmux", "paste-buffer", "-t", target)
		if err := pasteCmd.Run(); err != nil {
			return fmt.Errorf("failed to paste buffer: %w", err)
		}
		return nil
	}

	// No newlines, send the text using send-keys with literal mode
	cmd := exec.Command("tmux", "send-keys", "-t", target, "-l", text)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to send keys: %w", err)
	}
	return nil
}

// SendEnter sends just the Enter key to a window
func (c *Client) SendEnter(session, windowName string) error {
	target := fmt.Sprintf("%s:%s", session, windowName)
	cmd := exec.Command("tmux", "send-keys", "-t", target, "C-m")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to send enter: %w", err)
	}
	return nil
}

// ListSessions returns a list of all tmux sessions
func (c *Client) ListSessions() ([]string, error) {
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
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

// ListWindows returns a list of windows in a session
func (c *Client) ListWindows(session string) ([]string, error) {
	cmd := exec.Command("tmux", "list-windows", "-t", session, "-F", "#{window_name}")
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

// IsTmuxAvailable checks if tmux is installed and available
func (c *Client) IsTmuxAvailable() bool {
	cmd := exec.Command("tmux", "-V")
	return cmd.Run() == nil
}

// GetPaneInfo returns information about a pane
type PaneInfo struct {
	PID int
}

// GetPanePID gets the PID of the process running in a pane
func (c *Client) GetPanePID(session, windowName string) (int, error) {
	target := fmt.Sprintf("%s:%s", session, windowName)
	cmd := exec.Command("tmux", "display-message", "-t", target, "-p", "#{pane_pid}")
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

// StartPipePane sets up pipe-pane to capture output to a file
// The output is appended to the file, so it persists across daemon restarts
func (c *Client) StartPipePane(session, windowName, outputFile string) error {
	target := fmt.Sprintf("%s:%s", session, windowName)
	// Use -o to open a pipe (output only, not input)
	// cat >> appends to the file so output is preserved
	cmd := exec.Command("tmux", "pipe-pane", "-o", "-t", target, fmt.Sprintf("cat >> '%s'", outputFile))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start pipe-pane: %w", err)
	}
	return nil
}

// StopPipePane stops the pipe-pane for a window
func (c *Client) StopPipePane(session, windowName string) error {
	target := fmt.Sprintf("%s:%s", session, windowName)
	// Running pipe-pane with no command stops any existing pipe
	cmd := exec.Command("tmux", "pipe-pane", "-t", target)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to stop pipe-pane: %w", err)
	}
	return nil
}
