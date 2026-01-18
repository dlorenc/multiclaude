// Command generate-docs generates documentation from code constants.
// This ensures documentation stays in sync with the actual implementation.
//
// Usage:
//
//	go run ./cmd/generate-docs
//	go generate ./pkg/config/...
package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dlorenc/multiclaude/pkg/config"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	content := generateDirectoryStructure()

	// Determine output path
	outPath := "docs/DIRECTORY_STRUCTURE.md"
	if len(os.Args) > 1 {
		outPath = os.Args[1]
	}

	// If path is relative, make it relative to the project root (where go.mod is)
	if !filepath.IsAbs(outPath) {
		root, err := findProjectRoot()
		if err != nil {
			return fmt.Errorf("failed to find project root: %w", err)
		}
		outPath = filepath.Join(root, outPath)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write the file
	if err := os.WriteFile(outPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	fmt.Printf("Generated %s\n", outPath)
	return nil
}

// findProjectRoot walks up the directory tree to find go.mod
func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found")
		}
		dir = parent
	}
}

func generateDirectoryStructure() string {
	var buf bytes.Buffer

	// Header
	buf.WriteString("# Multiclaude Directory Structure\n\n")
	buf.WriteString("This document describes the directory structure used by multiclaude in `~/.multiclaude/`.\n")
	buf.WriteString("It is intended to help with debugging and understanding how multiclaude organizes its data.\n\n")
	buf.WriteString("> **Note**: This file is auto-generated from code constants in `pkg/config/doc.go`.\n")
	buf.WriteString("> Do not edit manually. Run `go generate ./pkg/config/...` to regenerate.\n\n")

	// Directory layout
	buf.WriteString("## Directory Layout\n\n")
	buf.WriteString("```\n")
	buf.WriteString("~/.multiclaude/\n")
	buf.WriteString("├── daemon.pid          # Daemon process ID\n")
	buf.WriteString("├── daemon.sock         # Unix socket for CLI communication\n")
	buf.WriteString("├── daemon.log          # Daemon activity log\n")
	buf.WriteString("├── state.json          # Persistent daemon state\n")
	buf.WriteString("│\n")
	buf.WriteString("├── repos/              # Cloned repositories\n")
	buf.WriteString("│   └── <repo-name>/    # Git clone of tracked repo\n")
	buf.WriteString("│\n")
	buf.WriteString("├── wts/                # Git worktrees\n")
	buf.WriteString("│   └── <repo-name>/\n")
	buf.WriteString("│       ├── supervisor/     # Supervisor's worktree\n")
	buf.WriteString("│       ├── merge-queue/    # Merge queue's worktree\n")
	buf.WriteString("│       └── <worker-name>/  # Worker worktrees\n")
	buf.WriteString("│\n")
	buf.WriteString("├── messages/           # Inter-agent messages\n")
	buf.WriteString("│   └── <repo-name>/\n")
	buf.WriteString("│       └── <agent-name>/\n")
	buf.WriteString("│           └── msg-<uuid>.json\n")
	buf.WriteString("│\n")
	buf.WriteString("└── prompts/            # Generated agent prompts\n")
	buf.WriteString("    └── <agent-name>.md\n")
	buf.WriteString("```\n\n")

	// Generate detailed descriptions
	docs := config.DirectoryDocs()
	buf.WriteString("## Path Descriptions\n\n")
	for _, doc := range docs {
		typeEmoji := "📄"
		if doc.Type == "directory" {
			typeEmoji = "📁"
		}
		buf.WriteString(fmt.Sprintf("### %s `%s`\n\n", typeEmoji, doc.Path))
		buf.WriteString(fmt.Sprintf("**Type**: %s\n\n", doc.Type))
		buf.WriteString(fmt.Sprintf("%s\n\n", doc.Description))
		if doc.Notes != "" {
			buf.WriteString(fmt.Sprintf("**Notes**: %s\n\n", doc.Notes))
		}
	}

	// Generate state.json documentation
	buf.WriteString("## state.json Format\n\n")
	buf.WriteString("The `state.json` file contains the daemon's persistent state. It is written atomically\n")
	buf.WriteString("(write to temp file, then rename) to prevent corruption.\n\n")
	buf.WriteString("### Schema\n\n")
	buf.WriteString("```json\n")
	buf.WriteString(`{
  "repos": {
    "<repo-name>": {
      "github_url": "https://github.com/owner/repo",
      "tmux_session": "multiclaude-repo",
      "agents": {
        "<agent-name>": {
          "type": "supervisor|worker|merge-queue|workspace",
          "worktree_path": "/path/to/worktree",
          "tmux_window": "window-name",
          "session_id": "uuid",
          "pid": 12345,
          "task": "task description (workers only)",
          "created_at": "2025-01-01T00:00:00Z",
          "last_nudge": "2025-01-01T00:00:00Z",
          "ready_for_cleanup": false
        }
      }
    }
  }
}
`)
	buf.WriteString("```\n\n")

	buf.WriteString("### Field Reference\n\n")
	buf.WriteString("| Field | Type | Description |\n")
	buf.WriteString("|-------|------|-------------|\n")
	for _, doc := range config.StateDocs() {
		buf.WriteString(fmt.Sprintf("| `%s` | `%s` | %s |\n", doc.Field, doc.Type, doc.Description))
	}
	buf.WriteString("\n")

	// Generate message file documentation
	buf.WriteString("## Message File Format\n\n")
	buf.WriteString("Message files are stored in `messages/<repo>/<agent>/msg-<uuid>.json`.\n")
	buf.WriteString("They are used for inter-agent communication.\n\n")
	buf.WriteString("### Schema\n\n")
	buf.WriteString("```json\n")
	buf.WriteString(`{
  "id": "msg-abc123def456",
  "from": "supervisor",
  "to": "happy-platypus",
  "timestamp": "2025-01-01T00:00:00Z",
  "body": "Please review PR #42",
  "status": "pending",
  "acked_at": null
}
`)
	buf.WriteString("```\n\n")

	buf.WriteString("### Field Reference\n\n")
	buf.WriteString("| Field | Type | Description |\n")
	buf.WriteString("|-------|------|-------------|\n")
	for _, doc := range config.MessageDocs() {
		buf.WriteString(fmt.Sprintf("| `%s` | `%s` | %s |\n", doc.Field, doc.Type, doc.Description))
	}
	buf.WriteString("\n")

	// Add debugging tips section
	buf.WriteString("## Debugging Tips\n\n")
	buf.WriteString("### Check daemon status\n\n")
	buf.WriteString("```bash\n")
	buf.WriteString("# Is the daemon running?\n")
	buf.WriteString("cat ~/.multiclaude/daemon.pid && ps -p $(cat ~/.multiclaude/daemon.pid)\n\n")
	buf.WriteString("# View daemon logs\n")
	buf.WriteString("tail -f ~/.multiclaude/daemon.log\n")
	buf.WriteString("```\n\n")

	buf.WriteString("### Inspect state\n\n")
	buf.WriteString("```bash\n")
	buf.WriteString("# Pretty-print current state\n")
	buf.WriteString("cat ~/.multiclaude/state.json | jq .\n\n")
	buf.WriteString("# List all agents for a repo\n")
	buf.WriteString("cat ~/.multiclaude/state.json | jq '.repos[\"my-repo\"].agents | keys'\n")
	buf.WriteString("```\n\n")

	buf.WriteString("### Check agent worktrees\n\n")
	buf.WriteString("```bash\n")
	buf.WriteString("# List all worktrees for a repo\n")
	buf.WriteString("ls ~/.multiclaude/wts/my-repo/\n\n")
	buf.WriteString("# Check git status in an agent's worktree\n")
	buf.WriteString("git -C ~/.multiclaude/wts/my-repo/supervisor status\n")
	buf.WriteString("```\n\n")

	buf.WriteString("### View messages\n\n")
	buf.WriteString("```bash\n")
	buf.WriteString("# List all messages for an agent\n")
	buf.WriteString("ls ~/.multiclaude/messages/my-repo/supervisor/\n\n")
	buf.WriteString("# Read a specific message\n")
	buf.WriteString("cat ~/.multiclaude/messages/my-repo/supervisor/msg-*.json | jq .\n")
	buf.WriteString("```\n\n")

	buf.WriteString("### Clean up stale state\n\n")
	buf.WriteString("```bash\n")
	buf.WriteString("# Use the built-in repair command\n")
	buf.WriteString("multiclaude repair\n\n")
	buf.WriteString("# Or manually clean up orphaned resources\n")
	buf.WriteString("multiclaude cleanup\n")
	buf.WriteString("```\n")

	return buf.String()
}
