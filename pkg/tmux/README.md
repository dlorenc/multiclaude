# pkg/tmux

A Go client for programmatic interaction with tmux terminal multiplexer.

## Why This Package?

Existing Go tmux libraries ([gotmux](https://github.com/GianlucaP106/gotmux), [go-tmux](https://github.com/jubnzv/go-tmux), [gomux](https://github.com/wricardo/gomux)) focus on workspace setup (session/window/pane creation) but lack features needed for **programmatic interaction with running CLI applications**:

| Feature | Existing libraries | This package |
|---------|-------------------|--------------|
| Multiline text via paste-buffer | No | **Yes** |
| Pane PID extraction | No | **Yes** |
| pipe-pane output capture | No | **Yes** |
| Context support for cancellation | No | **Yes** |
| Custom error types | No | **Yes** |
| Atomic text + Enter send | No | **Yes** |

## Installation

```bash
go get github.com/dlorenc/multiclaude/pkg/tmux
```

## Quick Start

```go
package main

import (
    "context"
    "log"
    "github.com/dlorenc/multiclaude/pkg/tmux"
)

func main() {
    ctx := context.Background()
    client := tmux.NewClient()

    // Check if tmux is available
    if !client.IsTmuxAvailable() {
        log.Fatal("tmux is not installed")
    }

    // Create a detached session
    if err := client.CreateSession(ctx, "my-session", true); err != nil {
        log.Fatal(err)
    }
    defer client.KillSession(ctx, "my-session")

    // Send a command
    if err := client.SendKeys(ctx, "my-session", "0", "echo hello"); err != nil {
        log.Fatal(err)
    }
}
```

## Key Features

### Context Support

All I/O methods accept a `context.Context` as the first parameter, enabling:

- Request cancellation
- Timeouts
- Graceful shutdown

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

// This will fail if it takes longer than 5 seconds
if err := client.CreateSession(ctx, "my-session", true); err != nil {
    if errors.Is(err, context.DeadlineExceeded) {
        log.Println("Operation timed out")
    }
}
```

### Custom Error Types

The package provides custom error types for programmatic error handling:

```go
import "errors"

err := client.CreateWindow(ctx, "nonexistent", "window")
if err != nil {
    var cmdErr *tmux.CommandError
    if errors.As(err, &cmdErr) {
        log.Printf("tmux command %s failed: %v", cmdErr.Op, cmdErr.Err)
    }
}

// Helper functions for common checks
if tmux.IsSessionNotFound(err) {
    // Handle missing session
}
if tmux.IsWindowNotFound(err) {
    // Handle missing window
}
```

### Multiline Text Input

The killer feature of this package. When interacting with CLI applications that process input on Enter, you need a way to send multiline text without triggering on each line.

```go
// Send multiline text without triggering intermediate processing
message := `This is line 1
This is line 2
This is line 3`

// SendKeysLiteral uses tmux's paste-buffer for multiline text
if err := client.SendKeysLiteral(ctx, "session", "window", message); err != nil {
    log.Fatal(err)
}

// Now send Enter to submit
if err := client.SendEnter(ctx, "session", "window"); err != nil {
    log.Fatal(err)
}

// Or use the atomic version to avoid race conditions
if err := client.SendKeysLiteralWithEnter(ctx, "session", "window", message); err != nil {
    log.Fatal(err)
}
```

**How it works:** For multiline text, the package uses tmux's paste-buffer mechanism:
1. `tmux set-buffer "..."` - stores the entire text
2. `tmux paste-buffer -t target` - pastes it atomically

This ensures the application receives the complete text before any processing is triggered.

### Atomic Text + Enter

`SendKeysLiteralWithEnter` sends text and Enter in a single shell command, preventing race conditions where Enter might be lost between separate exec calls:

```go
// Atomic: text + Enter in one operation
if err := client.SendKeysLiteralWithEnter(ctx, "session", "window", "echo hello"); err != nil {
    log.Fatal(err)
}
```

### Process PID Extraction

Monitor whether a process running in a tmux pane is still alive:

```go
pid, err := client.GetPanePID(ctx, "session", "window")
if err != nil {
    log.Fatal(err)
}

// Check if process is alive
process, err := os.FindProcess(pid)
if err != nil {
    log.Printf("Process %d not found", pid)
}
```

### Output Capture with pipe-pane

Capture all output from a tmux pane to a file:

```go
// Start capturing output
if err := client.StartPipePane(ctx, "session", "window", "/tmp/output.log"); err != nil {
    log.Fatal(err)
}

// ... run commands in the pane ...

// Stop capturing
if err := client.StopPipePane(ctx, "session", "window"); err != nil {
    log.Fatal(err)
}
```

## API Reference

### Session Management

```go
HasSession(ctx context.Context, name string) (bool, error)      // Check if session exists
CreateSession(ctx context.Context, name string, detached bool) error  // Create new session
KillSession(ctx context.Context, name string) error             // Terminate session
ListSessions(ctx context.Context) ([]string, error)           // List all sessions
```

### Window Management

```go
CreateWindow(ctx context.Context, session, name string) error   // Create window in session
HasWindow(ctx context.Context, session, name string) (bool, error)  // Check if window exists (exact match)
KillWindow(ctx context.Context, session, name string) error     // Terminate window
ListWindows(ctx context.Context, session string) ([]string, error)  // List windows in session
```

### Text Input

```go
SendKeys(ctx context.Context, session, window, text string) error     // Send text + Enter
SendKeysLiteral(ctx context.Context, session, window, text string) error  // Send text (paste-buffer for multiline)
SendEnter(ctx context.Context, session, window string) error          // Send just Enter
SendKeysLiteralWithEnter(ctx context.Context, session, window, text string) error  // Atomic text + Enter
```

### Process Monitoring

```go
GetPanePID(ctx context.Context, session, window string) (int, error)  // Get process PID in pane
```

### Output Capture

```go
StartPipePane(ctx context.Context, session, window, outputFile string) error  // Start capturing
StopPipePane(ctx context.Context, session, window string) error               // Stop capturing
```

### Error Types

```go
type SessionNotFoundError struct { Name string }
type WindowNotFoundError struct { Session, Window string }
type CommandError struct { Op, Session, Window string; Err error }

func IsSessionNotFound(err error) bool
func IsWindowNotFound(err error) bool
```

### Configuration

```go
// Use a custom tmux binary path
client := tmux.NewClient(tmux.WithTmuxPath("/usr/local/bin/tmux"))
```

## Use Cases

This package was designed for orchestrating multiple Claude Code agents, but is useful for any scenario requiring programmatic control of CLI applications:

- Running multiple AI assistants in parallel
- Automated testing of interactive CLI tools
- CI/CD pipelines that need to interact with terminal applications
- DevOps automation with interactive prompts

## Requirements

- tmux 2.0 or later (uses paste-buffer and pipe-pane)
- Go 1.21 or later

## License

See the main project LICENSE file.
