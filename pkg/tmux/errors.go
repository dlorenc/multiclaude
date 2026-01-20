package tmux

import "fmt"

// SessionNotFoundError indicates that a tmux session does not exist.
type SessionNotFoundError struct {
	Name string
}

func (e *SessionNotFoundError) Error() string {
	return fmt.Sprintf("tmux session not found: %s", e.Name)
}

// Is returns true if target is a *SessionNotFoundError.
func (e *SessionNotFoundError) Is(target error) bool {
	_, ok := target.(*SessionNotFoundError)
	return ok
}

// WindowNotFoundError indicates that a tmux window does not exist within a session.
type WindowNotFoundError struct {
	Session string
	Window  string
}

func (e *WindowNotFoundError) Error() string {
	return fmt.Sprintf("tmux window not found: %s in session %s", e.Window, e.Session)
}

// Is returns true if target is a *WindowNotFoundError.
func (e *WindowNotFoundError) Is(target error) bool {
	_, ok := target.(*WindowNotFoundError)
	return ok
}

// CommandError wraps errors from tmux command execution with additional context.
type CommandError struct {
	Op      string // Operation that failed (e.g., "create-session", "send-keys")
	Session string // Session name, if applicable
	Window  string // Window name, if applicable
	Err     error  // Underlying error
}

func (e *CommandError) Error() string {
	if e.Window != "" {
		return fmt.Sprintf("tmux %s failed for %s:%s: %v", e.Op, e.Session, e.Window, e.Err)
	}
	if e.Session != "" {
		return fmt.Sprintf("tmux %s failed for session %s: %v", e.Op, e.Session, e.Err)
	}
	return fmt.Sprintf("tmux %s failed: %v", e.Op, e.Err)
}

func (e *CommandError) Unwrap() error {
	return e.Err
}

// IsSessionNotFound returns true if the error indicates a session was not found.
func IsSessionNotFound(err error) bool {
	_, ok := err.(*SessionNotFoundError)
	return ok
}

// IsWindowNotFound returns true if the error indicates a window was not found.
func IsWindowNotFound(err error) bool {
	_, ok := err.(*WindowNotFoundError)
	return ok
}
