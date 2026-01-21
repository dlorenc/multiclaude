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
    "context"
    "log"
    "github.com/dlorenc/multiclaude/pkg/claude"
    "github.com/dlorenc/multiclaude/pkg/tmux"
)

func main() {
    ctx := context.Background()

    // Create terminal runner (tmux)
    tmuxClient := tmux.NewClient()

    // Create Claude runner
    runner := claude.NewRunner(
        claude.WithTerminal(tmuxClient),
        claude.WithBinaryPath(claude.ResolveBinaryPath()),
    )

    // Create tmux session and window
    tmuxClient.CreateSession(ctx, "my-session", true)
    defer tmuxClient.KillSession(ctx, "my-session")
    tmuxClient.CreateWindow(ctx, "my-session", "claude")

    // Start Claude
    result, err := runner.Start(ctx, "my-session", "claude", claude.Config{
        SystemPromptFile: "/path/to/prompt.md",
        WorkDir:          "/path/to/workspace",
    })
    if err != nil {
        log.Fatal(err)
    }

    log.Printf("Claude started: session=%s, pid=%d", result.SessionID, result.PID)

    // Send a message
    if err := runner.SendMessage(ctx, "my-session", "claude", "Hello, Claude!"); err != nil {
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
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

result, err := runner.Start(ctx, "session", "window", claude.Config{})
if errors.Is(err, context.DeadlineExceeded) {
    log.Println("Claude startup timed out")
}
```

### TerminalRunner Interface

The package uses the `TerminalRunner` interface to abstract terminal operations:

```go
type TerminalRunner interface {
    SendKeys(ctx context.Context, session, window, text string) error
    SendKeysLiteral(ctx context.Context, session, window, text string) error
    SendEnter(ctx context.Context, session, window string) error
    SendKeysLiteralWithEnter(ctx context.Context, session, window, text string) error
    GetPanePID(ctx context.Context, session, window string) (int, error)
    StartPipePane(ctx context.Context, session, window, outputFile string) error
    StopPipePane(ctx context.Context, session, window string) error
}
```

The `pkg/tmux.Client` implements this interface, but you can create custom implementations for other terminal emulators.

### Working Directory Support

Specify the working directory where Claude should run:

```go
result, err := runner.Start(ctx, "session", "window", claude.Config{
    WorkDir: "/path/to/project",  // Claude will cd here before starting
})
```

### Message of the Day (MOTD)

Display a custom message before Claude starts (useful for showing restart instructions or context):

```go
result, err := runner.Start(ctx, "session", "window", claude.Config{
    MOTD: "Restarting Claude session after crash...",
})
```

### Session ID Management

Each Claude instance gets a unique UUID v4 session ID:

```go
// Generate a new session ID
sessionID, err := claude.GenerateSessionID()

// Or let Start() generate one
result, _ := runner.Start(ctx, "session", "window", claude.Config{})
fmt.Println(result.SessionID)

// Or provide your own
result, _ := runner.Start(ctx, "session", "window", claude.Config{
    SessionID: "my-custom-id",
})

// Resume an existing session
result, _ := runner.Start(ctx, "session", "window", claude.Config{
    SessionID: existingID,
    Resume:    true,  // Uses --resume instead of --session-id
})
```

### Output Capture

Capture Claude's output to a file:

```go
result, err := runner.Start(ctx, "session", "window", claude.Config{
    OutputFile: "/tmp/claude-output.log",
})
```

### Multiline Messages

The `SendMessage` method uses atomic sends to properly handle multiline text:

```go
message := `Please review this code:

func hello() {
    fmt.Println("Hello, World!")
}

What improvements would you suggest?`

runner.SendMessage(ctx, "session", "window", message)
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

## Config Fields

| Field | Description |
|-------|-------------|
| `SessionID` | Unique session identifier (auto-generated if empty) |
| `Resume` | If true, uses `--resume` instead of `--session-id` |
| `WorkDir` | Working directory to cd into before starting |
| `SystemPromptFile` | Path to system prompt file |
| `InitialMessage` | Optional message to send after startup |
| `OutputFile` | Path to capture output via pipe-pane |
| `MOTD` | Message to display before starting Claude |

## CLI Flags

The runner constructs Claude commands with these flags:

| Flag | Description |
|------|-------------|
| `--session-id <uuid>` | Unique session identifier |
| `--resume <uuid>` | Resume existing session |
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
