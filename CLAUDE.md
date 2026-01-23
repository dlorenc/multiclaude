# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**multiclaude** is a lightweight orchestrator for running multiple Claude Code agents on GitHub repositories. Each agent runs in its own tmux window with an isolated git worktree, enabling parallel autonomous work on a shared codebase.

### The Brownian Ratchet Philosophy

This project embraces controlled chaos: multiple agents work simultaneously, potentially duplicating effort or creating conflicts. **CI is the ratchet** - if tests pass, the code goes in. Progress is permanent.

**Core Beliefs (hardcoded, not configurable):**
- CI is King: Never weaken CI to make work pass
- Forward Progress > Perfection: Partial working solutions beat perfect incomplete ones
- Chaos is Expected: Redundant work is cheaper than blocked work
- Humans Approve, Agents Execute: Agents create PRs but don't bypass review

## Quick Reference

```bash
# Build & Install
go build ./cmd/multiclaude         # Build binary
go install ./cmd/multiclaude       # Install to $GOPATH/bin

# Test
go test ./...                      # All tests
go test ./internal/daemon          # Single package
go test -v ./test/...              # E2E tests (requires tmux)
go test ./internal/state -run TestSave  # Single test

# Development
go generate ./pkg/config           # Regenerate CLI docs for prompts
MULTICLAUDE_TEST_MODE=1 go test ./test/...  # Skip Claude startup
```

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                         CLI (cmd/multiclaude)                    │
└────────────────────────────────┬────────────────────────────────┘
                                 │ Unix Socket
┌────────────────────────────────▼────────────────────────────────┐
│                          Daemon (internal/daemon)                │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐        │
│  │ Health   │  │ Message  │  │ Wake/    │  │ Socket   │        │
│  │ Check    │  │ Router   │  │ Nudge    │  │ Server   │        │
│  │ (2min)   │  │ (2min)   │  │ (2min)   │  │          │        │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘        │
└────────────────────────────────┬────────────────────────────────┘
                                 │
    ┌────────────────────────────┼────────────────────────────────┐
    │                            │                                │
┌───▼───┐  ┌───────────┐  ┌─────▼─────┐  ┌──────────┐  ┌────────┐
│super- │  │merge-     │  │workspace  │  │worker-N  │  │review  │
│visor  │  │queue      │  │           │  │          │  │        │
└───────┘  └───────────┘  └───────────┘  └──────────┘  └────────┘
    │           │              │              │             │
    └───────────┴──────────────┴──────────────┴─────────────┘
              tmux session: mc-<repo>  (one window per agent)
```

### Package Responsibilities

| Package | Purpose | Key Types |
|---------|---------|-----------|
| `cmd/multiclaude` | Entry point | `main()` |
| `internal/cli` | All CLI commands | `CLI`, `Command` |
| `internal/daemon` | Background process | `Daemon`, daemon loops |
| `internal/state` | Persistence | `State`, `Agent`, `Repository` |
| `internal/messages` | Inter-agent IPC | `Manager`, `Message` |
| `internal/prompts` | Agent system prompts | Embedded `*.md` files, `GetSlashCommandsPrompt()` |
| `internal/prompts/commands` | Slash command templates | `GenerateCommandsDir()`, embedded `*.md` (legacy) |
| `internal/hooks` | Claude hooks config | `CopyConfig()` |
| `internal/worktree` | Git worktree ops | `Manager`, `WorktreeInfo` |
| `internal/tmux` | Internal tmux client | `Client` (internal use) |
| `internal/socket` | Unix socket IPC | `Server`, `Client`, `Request` |
| `internal/errors` | User-friendly errors | `CLIError`, error constructors |
| `internal/names` | Worker name generation | `Generate()` (adjective-animal) |
| `pkg/config` | Path configuration | `Paths`, `NewTestPaths()` |
| `pkg/tmux` | **Public** tmux library | `Client` (multiline support) |
| `pkg/claude` | **Public** Claude runner | `Runner`, `Config` |

### Data Flow

1. **CLI** parses args → sends `Request` via Unix socket
2. **Daemon** handles request → updates `state.json` → manages tmux
3. **Agents** run in tmux windows with embedded prompts and per-agent slash commands (via `CLAUDE_CONFIG_DIR`)
4. **Messages** flow via filesystem JSON files, routed by daemon
5. **Health checks** (every 2 min) attempt self-healing restoration before cleanup of dead agents

## Key Files to Understand

| File | What It Does |
|------|--------------|
| `internal/cli/cli.go` | **Large file** (~3700 lines) with all CLI commands |
| `internal/daemon/daemon.go` | Daemon implementation with all loops |
| `internal/state/state.go` | State struct with mutex-protected operations |
| `internal/prompts/*.md` | Agent system prompts (embedded at compile) |
| `pkg/tmux/client.go` | Public tmux library with `SendKeysLiteralWithEnter` |

## Patterns and Conventions

### Error Handling

Use structured errors from `internal/errors` for user-facing messages:

```go
// Good: User gets helpful message + suggestion
return errors.DaemonNotRunning()  // "daemon is not running" + "Try: multiclaude daemon start"

// Good: Wrap with context
return errors.GitOperationFailed("clone", err)

// Avoid: Raw errors lose context for users
return fmt.Errorf("clone failed: %w", err)
```

### State Mutations

Always use atomic writes for crash safety:

```go
// internal/state/state.go pattern
func (s *State) saveUnlocked() error {
    data, _ := json.MarshalIndent(s, "", "  ")
    tmpPath := s.path + ".tmp"
    os.WriteFile(tmpPath, data, 0644)  // Write temp
    os.Rename(tmpPath, s.path)          // Atomic rename
}
```

### Tmux Text Input

Use `SendKeysLiteralWithEnter` for atomic text + Enter (prevents race conditions):

```go
// Good: Atomic operation
tmux.SendKeysLiteralWithEnter(session, window, message)

// Avoid: Race condition between text and Enter
tmux.SendKeysLiteral(session, window, message)
tmux.SendEnter(session, window)  // Enter might be lost!
```

### Agent Context Detection

Agents infer their context from working directory:

```go
// internal/cli/cli.go:2385
func (c *CLI) inferRepoFromCwd() (string, error) {
    // Checks if cwd is under ~/.multiclaude/wts/<repo>/ or repos/<repo>/
}
```

## Testing

### Test Categories

| Directory | What | Requirements |
|-----------|------|--------------|
| `internal/*/` | Unit tests | None |
| `test/` | E2E integration | tmux installed |
| `test/recovery_test.go` | Crash recovery | tmux installed |

### Test Mode

```bash
# Skip actual Claude startup in tests
MULTICLAUDE_TEST_MODE=1 go test ./test/...
```

### Writing Tests

```go
// Create isolated test environment using the helper
tmpDir, _ := os.MkdirTemp("", "multiclaude-test-*")
paths := config.NewTestPaths(tmpDir)  // Sets up all paths correctly
defer os.RemoveAll(tmpDir)

// Use NewWithPaths for testing
cli := cli.NewWithPaths(paths, "claude")
```

## Agent System

See `AGENTS.md` for detailed agent documentation including:
- Agent types and their roles
- Message routing implementation
- Prompt system and customization
- Agent lifecycle management
- Adding new agent types

## Contributing Checklist

When modifying agent behavior:
- [ ] Update the relevant prompt in `internal/prompts/*.md`
- [ ] Run `go generate ./pkg/config` if CLI changed
- [ ] Test with tmux: `go test ./test/...`
- [ ] Check state persistence: `go test ./internal/state/...`

When adding CLI commands:
- [ ] Add to `registerCommands()` in `internal/cli/cli.go`
- [ ] Use `internal/errors` for user-facing errors
- [ ] Add help text with `Usage` field
- [ ] Regenerate docs: `go generate ./pkg/config`

When modifying daemon loops:
- [ ] Consider interaction with health check (2 min cycle)
- [ ] Test crash recovery: `go test ./test/ -run Recovery`
- [ ] Verify state atomicity with concurrent access tests

## Runtime Directories

```
~/.multiclaude/
├── daemon.pid              # Daemon PID (lock file)
├── daemon.sock             # Unix socket for CLI<->daemon
├── daemon.log              # Daemon logs (rotated at 10MB)
├── state.json              # All state (repos, agents, config)
├── prompts/                # Generated prompt files for agents
├── repos/<repo>/           # Cloned repositories
├── wts/<repo>/<agent>/     # Git worktrees (one per agent)
├── messages/<repo>/<agent>/ # Message JSON files
├── output/<repo>/          # Agent output logs
│   └── workers/            # Worker-specific logs
└── claude-config/<repo>/<agent>/ # Per-agent CLAUDE_CONFIG_DIR
    └── commands/           # Slash command files (*.md)
```

## Common Operations

### Debug a stuck agent

```bash
# Attach to see what it's doing
multiclaude attach <agent-name> --read-only

# Check its messages
multiclaude agent list-messages  # (from agent's tmux window)

# Manually nudge via daemon logs
tail -f ~/.multiclaude/daemon.log
```

### Repair inconsistent state

```bash
# Local repair (no daemon)
multiclaude repair

# Daemon-side repair
multiclaude cleanup --dry-run  # See what would be cleaned
multiclaude cleanup            # Actually clean up
```

### Test prompt changes

```bash
# Prompts are embedded at compile time
vim internal/prompts/worker.md
go build ./cmd/multiclaude
# New workers will use updated prompt
```
