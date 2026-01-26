# Change: Add Repo Lifecycle Management Commands

## Why

Currently, multiclaude has fragmented commands for managing repositories:
- `repo init/list/rm` for basic repo tracking
- `daemon start/stop` for the global daemon
- `worker create/list/rm` for individual workers
- `agents spawn` for persistent agents

There's no unified way to:
1. **Start** a full repo session (supervisor + merge-queue + workspace)
2. Get **comprehensive status** of all repo activity
3. **Hibernate** a repo (pause agents without losing state)
4. **Refresh** all worktrees atomically
5. **Clean** orphaned resources for a specific repo

Users must manually orchestrate these operations, leading to inconsistent states.

## What Changes

Add new `repo` subcommands for complete lifecycle management:

| Command | Purpose |
|---------|---------|
| `repo start [name]` | Start all agents (supervisor, merge-queue, workspace) |
| `repo status [name]` | Comprehensive status with agents, PRs, messages, health |
| `repo hibernate [name]` | Pause all agents, preserve state |
| `repo wake [name]` | Resume hibernated repo |
| `repo refresh [name]` | Sync all worktrees with main branch |
| `repo clean [name]` | Clean orphaned resources for repo |

Add output format options to all commands:
- `--format=text` (default) - Human-readable
- `--format=json` - Machine-readable JSON
- `--format=yaml` - YAML output
- `--tui` - Interactive terminal UI
- `--websocket` - Stream to WebSocket server

## Impact

- **Affected specs**: New capability (repo-lifecycle)
- **Affected code**:
  - `multiclaude/internal/cli/cli.go` - New commands
  - `multiclaude/internal/daemon/daemon.go` - Hibernate/wake support
  - `multiclaude/internal/state/state.go` - Hibernation state
- **Breaking changes**: None (additive only)
- **Dependencies**: TUI requires new dependency (bubbletea or similar)
