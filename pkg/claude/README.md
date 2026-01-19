# pkg/claude

A Go library for programmatically running and interacting with Claude Code CLI.

## Installation

```bash
go get github.com/dlorenc/multiclaude/pkg/claude
```

## Quick Start

```go
package main

import (
    "log"
    "github.com/dlorenc/multiclaude/pkg/claude"
    "github.com/dlorenc/multiclaude/pkg/tmux"
)

func main() {
    // Create terminal runner (tmux)
    tmuxClient := tmux.NewClient()

    // Create Claude runner
    runner := claude.NewRunner(
        claude.WithTerminal(tmuxClient),
        claude.WithBinaryPath(claude.ResolveBinaryPath()),
    )

    // Create tmux session and window
    tmuxClient.CreateSession("my-session", true)
    defer tmuxClient.KillSession("my-session")
    tmuxClient.CreateWindow("my-session", "claude")

    // Start Claude
    result, err := runner.Start("my-session", "claude", claude.Config{
        SystemPromptFile: "/path/to/prompt.md",
    })
    if err != nil {
        log.Fatal(err)
    }

    log.Printf("Claude started: session=%s, pid=%d", result.SessionID, result.PID)

    // Send a message
    if err := runner.SendMessage("my-session", "claude", "Hello, Claude!"); err != nil {
        log.Fatal(err)
    }
}
```

## Key Features

### TerminalRunner Interface

The package uses the `TerminalRunner` interface to abstract terminal operations:

```go
type TerminalRunner interface {
    SendKeys(session, window, text string) error
    SendKeysLiteral(session, window, text string) error
    SendEnter(session, window string) error
    GetPanePID(session, window string) (int, error)
    StartPipePane(session, window, outputFile string) error
    StopPipePane(session, window string) error
}
```

The `pkg/tmux.Client` implements this interface, but you can create custom implementations for other terminal emulators.

### Session ID Management

Each Claude instance gets a unique UUID v4 session ID:

```go
// Generate a new session ID
sessionID, err := claude.GenerateSessionID()

// Or let Start() generate one
result, _ := runner.Start("session", "window", claude.Config{})
fmt.Println(result.SessionID)

// Or provide your own
result, _ := runner.Start("session", "window", claude.Config{
    SessionID: "my-custom-id",
})
```

### Output Capture

Capture Claude's output to a file:

```go
result, err := runner.Start("session", "window", claude.Config{
    OutputFile: "/tmp/claude-output.log",
})
```

### Multiline Messages

The `SendMessage` method properly handles multiline text:

```go
message := `Please review this code:

func hello() {
    fmt.Println("Hello, World!")
}

What improvements would you suggest?`

runner.SendMessage("session", "window", message)
```

## Configuration Options

```go
runner := claude.NewRunner(
    // Path to claude binary (default: "claude")
    claude.WithBinaryPath("/usr/local/bin/claude"),

    // Terminal runner (required for Start/SendMessage)
    claude.WithTerminal(tmuxClient),

    // Time to wait after starting before getting PID (default: 500ms)
    claude.WithStartupDelay(1 * time.Second),

    // Time to wait before sending initial message (default: 1s)
    claude.WithMessageDelay(2 * time.Second),

    // Whether to skip permission prompts (default: true)
    claude.WithPermissions(true),
)
```

## CLI Flags

The runner constructs Claude commands with these flags:

| Flag | Description |
|------|-------------|
| `--session-id <uuid>` | Unique session identifier |
| `--dangerously-skip-permissions` | Skip interactive permission prompts |
| `--append-system-prompt-file <path>` | Path to system prompt file |

## Prompt Building

For building complex prompts, see the `pkg/claude/prompt` subpackage:

```go
import "github.com/dlorenc/multiclaude/pkg/claude/prompt"

builder := prompt.NewBuilder()
builder.AddSection("Role", "You are a helpful coding assistant.")
builder.AddSection("Context", "Working on a Go project.")

promptText := builder.Build()
```

## Use Cases

- Running multiple Claude instances in parallel
- Automated code review with Claude
- CI/CD integration
- Interactive development assistants
- Pair programming automation

## Requirements

- Claude Code CLI installed and in PATH
- tmux (if using tmux as terminal runner)
- Go 1.21 or later

## License

See the main project LICENSE file.
