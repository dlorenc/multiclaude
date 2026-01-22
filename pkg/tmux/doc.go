// Package tmux provides a Go client for programmatic interaction with tmux.
//
// This package focuses on features needed for interacting with running CLI
// applications in tmux, which are not covered by existing Go tmux libraries:
//
//   - Multiline text input using paste-buffer (see [Client.SendKeysLiteral])
//   - Process PID extraction from panes (see [Client.GetPanePID])
//   - Output capture via pipe-pane (see [Client.StartPipePane], [Client.StopPipePane])
//
// # Installation
//
//	go get github.com/dlorenc/multiclaude/pkg/tmux
//
// # Requirements
//
// This package requires tmux to be installed and available in PATH.
// Use [Client.IsTmuxAvailable] to check availability at runtime.
//
// # Example Usage
//
//	package main
//
//	import (
//	    "context"
//	    "log"
//	    "github.com/dlorenc/multiclaude/pkg/tmux"
//	)
//
//	func main() {
//	    ctx := context.Background()
//	    client := tmux.NewClient()
//
//	    // Verify tmux is available
//	    if !client.IsTmuxAvailable() {
//	        log.Fatal("tmux is not installed")
//	    }
//
//	    // Create a detached session
//	    if err := client.CreateSession(ctx, "demo", true); err != nil {
//	        log.Fatal(err)
//	    }
//	    defer client.KillSession(ctx, "demo")
//
//	    // Create a named window
//	    if err := client.CreateWindow(ctx, "demo", "worker"); err != nil {
//	        log.Fatal(err)
//	    }
//
//	    // Start capturing output
//	    if err := client.StartPipePane(ctx, "demo", "worker", "/tmp/demo.log"); err != nil {
//	        log.Fatal(err)
//	    }
//
//	    // Send multiline text without triggering intermediate processing
//	    multilineMessage := `This is a
//	multiline message
//	that won't trigger on each newline`
//	    if err := client.SendKeysLiteral(ctx, "demo", "worker", multilineMessage); err != nil {
//	        log.Fatal(err)
//	    }
//
//	    // Now send Enter to submit
//	    if err := client.SendEnter(ctx, "demo", "worker"); err != nil {
//	        log.Fatal(err)
//	    }
//
//	    // Get the PID of the process in the pane
//	    pid, err := client.GetPanePID(ctx, "demo", "worker")
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//	    log.Printf("Process PID: %d", pid)
//	}
//
// # The Paste-Buffer Technique
//
// When sending multiline text to a CLI application, naive approaches using
// send-keys fail because the application may interpret each newline as a
// command submission. This package uses tmux's paste-buffer to send the
// entire text atomically:
//
//  1. Set the buffer with the full text: tmux set-buffer "..."
//  2. Paste the buffer to the pane: tmux paste-buffer -t session:window
//
// This ensures the application receives the complete multiline text before
// any processing is triggered.
//
// # Comparison to Other Libraries
//
// | Feature                    | gotmux | go-tmux | gomux | this package |
// |----------------------------|--------|---------|-------|--------------|
// | Session/window creation    | Yes    | Yes     | Yes   | Yes          |
// | Multiline paste-buffer     | No     | No      | No    | Yes          |
// | Pane PID extraction        | No     | No      | No    | Yes          |
// | pipe-pane output capture   | No     | No      | No    | Yes          |
package tmux
