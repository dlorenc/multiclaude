// Package claude provides utilities for programmatically running Claude Code CLI.
//
// This package abstracts the details of launching and interacting with Claude Code
// instances running in terminal emulators. It handles:
//
//   - CLI flag construction (--session-id, --dangerously-skip-permissions, --append-system-prompt-file)
//   - Session ID generation (UUID v4)
//   - Startup timing quirks
//   - Terminal integration via the [TerminalRunner] interface
//
// # Installation
//
//	go get github.com/dlorenc/multiclaude/pkg/claude
//
// # Requirements
//
// This package requires the Claude Code CLI to be installed. The binary is typically
// named "claude" and should be available in PATH. Use [ResolveBinaryPath] to find it,
// and [Runner.IsBinaryAvailable] to verify it's installed before use.
//
// # Example Usage
//
//	package main
//
//	import (
//	    "log"
//	    "github.com/dlorenc/multiclaude/pkg/claude"
//	    "github.com/dlorenc/multiclaude/pkg/tmux"
//	)
//
//	func main() {
//	    // Create terminal runner (tmux in this case)
//	    tmuxClient := tmux.NewClient()
//
//	    // Create Claude runner with tmux as the terminal
//	    runner := claude.NewRunner(
//	        claude.WithTerminal(tmuxClient),
//	        claude.WithBinaryPath(claude.ResolveBinaryPath()),
//	    )
//
//	    // Verify Claude CLI is available
//	    if !runner.IsBinaryAvailable() {
//	        log.Fatal("Claude CLI is not installed")
//	    }
//
//	    // Prepare a session
//	    tmuxClient.CreateSession("demo", true)
//	    tmuxClient.CreateWindow("demo", "claude")
//
//	    // Start Claude
//	    result, err := runner.Start("demo", "claude", claude.Config{
//	        SystemPromptFile: "/path/to/prompt.md",
//	        OutputFile:       "/tmp/claude-output.log",
//	    })
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//
//	    log.Printf("Claude started with session ID: %s, PID: %d", result.SessionID, result.PID)
//
//	    // Send a message
//	    if err := runner.SendMessage("demo", "claude", "Hello, Claude!"); err != nil {
//	        log.Fatal(err)
//	    }
//	}
//
// # The TerminalRunner Interface
//
// The [TerminalRunner] interface abstracts terminal operations, allowing this package
// to work with any terminal emulator that implements it. The [pkg/tmux.Client] provides
// a ready-to-use implementation for tmux.
//
// # Session Management
//
// Each Claude instance is identified by a session ID (UUID v4). This allows:
//
//   - Resuming sessions across process restarts
//   - Tracking multiple concurrent Claude instances
//   - Correlating logs with specific sessions
//
// Use [GenerateSessionID] to create new session IDs, or provide your own via [Config.SessionID].
//
// # Timing Considerations
//
// Starting Claude and sending messages requires careful timing:
//
//   - [Runner.StartupDelay] (default 500ms): Wait after launching before getting PID
//   - [Runner.MessageDelay] (default 1s): Wait before sending initial message
//
// These can be adjusted via [WithStartupDelay] and [WithMessageDelay] options.
package claude
