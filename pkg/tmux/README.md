# pkg/tmux

A Go client for programmatic interaction with tmux terminal multiplexer.

## Why This Package?

Existing Go tmux libraries ([gotmux](https://github.com/GianlucaP106/gotmux), [go-tmux](https://github.com/jubnzv/go-tmux), [gomux](https://github.com/wricardo/gomux)) focus on workspace setup (session/window/pane creation) but lack features needed for **programmatic interaction with running CLI applications**:

| Feature | Existing libraries | This package |
|---------|-------------------|--------------|
| Multiline text via paste-buffer | No | **Yes** |
| Pane PID extraction | No | **Yes** |
| pipe-pane output capture | No | **Yes** |

## Installation

```bash
go get github.com/dlorenc/multiclaude/pkg/tmux
```

## Quick Start

```go
package main

import (
    "log"
    "github.com/dlorenc/multiclaude/pkg/tmux"
)

func main() {
    client := tmux.NewClient()

    // Check if tmux is available
    if !client.IsTmuxAvailable() {
        log.Fatal("tmux is not installed")
    }

    // Create a detached session
    if err := client.CreateSession("my-session", true); err != nil {
        log.Fatal(err)
    }
    defer client.KillSession("my-session")

    // Send a command
    if err := client.SendKeys("my-session", "0", "echo hello"); err != nil {
        log.Fatal(err)
    }
}
```

## Key Features

### Multiline Text Input

The killer feature of this package. When interacting with CLI applications that process input on Enter, you need a way to send multiline text without triggering on each line.

```go
// Send multiline text without triggering intermediate processing
message := `This is line 1
This is line 2
This is line 3`

// SendKeysLiteral uses tmux's paste-buffer for multiline text
if err := client.SendKeysLiteral("session", "window", message); err != nil {
    log.Fatal(err)
}

// Now send Enter to submit
if err := client.SendEnter("session", "window"); err != nil {
    log.Fatal(err)
}
```

**How it works:** For multiline text, the package uses tmux's paste-buffer mechanism:
1. `tmux set-buffer "..."` - stores the entire text
2. `tmux paste-buffer -t target` - pastes it atomically

This ensures the application receives the complete text before any processing is triggered.

### Process PID Extraction

Monitor whether a process running in a tmux pane is still alive:

```go
pid, err := client.GetPanePID("session", "window")
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
if err := client.StartPipePane("session", "window", "/tmp/output.log"); err != nil {
    log.Fatal(err)
}

// ... run commands in the pane ...

// Stop capturing
if err := client.StopPipePane("session", "window"); err != nil {
    log.Fatal(err)
}
```

## API Reference

### Session Management

```go
HasSession(name string) (bool, error)      // Check if session exists
CreateSession(name string, detached bool) error  // Create new session
KillSession(name string) error             // Terminate session
ListSessions() ([]string, error)           // List all sessions
```

### Window Management

```go
CreateWindow(session, name string) error   // Create window in session
HasWindow(session, name string) (bool, error)  // Check if window exists
KillWindow(session, name string) error     // Terminate window
ListWindows(session string) ([]string, error)  // List windows in session
```

### Text Input

```go
SendKeys(session, window, text string) error     // Send text + Enter
SendKeysLiteral(session, window, text string) error  // Send text (paste-buffer for multiline)
SendEnter(session, window string) error          // Send just Enter
```

### Process Monitoring

```go
GetPanePID(session, window string) (int, error)  // Get process PID in pane
```

### Output Capture

```go
StartPipePane(session, window, outputFile string) error  // Start capturing
StopPipePane(session, window string) error               // Stop capturing
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
