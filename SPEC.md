# multiclaude Architecture Specification

## Overview

multiclaude orchestrates multiple Claude Code instances working autonomously on GitHub repositories. It uses tmux for session management, git worktrees for isolation, and a daemon for coordination.

## Core Concepts

### Agents

An **agent** is a Claude Code instance with:
- A **tmux window** for visibility and interaction
- A **git worktree** for isolated file access
- A **role** defining its responsibilities (supervisor, worker, or merge-queue)
- A **session ID** (UUID) for context persistence

Agents run autonomously. Humans can attach via tmux to observe or intervene at any time.

### Agent Types

**Supervisor**
- One per repository
- Monitors all other agents
- Answers status questions ("what's everyone up to?")
- Nudges stuck workers
- Makes high-level decisions about task coordination
- Keeps worktree synced with main branch

**Worker**
- Executes a specific task
- Creates a PR when work is complete
- Signals completion, then gets cleaned up
- Named with Docker-style names (e.g., "happy-platypus")
- Multiple workers can run concurrently

**Merge Queue**
- One per repository
- Monitors open PRs from workers
- Merges when CI passes
- Spawns fixup workers for CI failures
- Never weakens CI without human approval

### Daemon

A single daemon process coordinates everything:
- Manages state (repos, agents, messages)
- Routes messages between agents
- Periodically nudges agents
- Monitors agent health
- Handles CLI requests via Unix socket

## Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                         multiclaude daemon                        │
│                                                                   │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐              │
│  │ Health Loop │  │ Message Loop│  │ Nudge Loop  │              │
│  │  (2 min)    │  │   (2 min)   │  │  (2 min)    │              │
│  └─────────────┘  └─────────────┘  └─────────────┘              │
│                                                                   │
│  ┌─────────────────────────────────────────────────────────────┐ │
│  │                     State Manager                           │ │
│  │  repos, agents, messages, PIDs, worktrees                   │ │
│  └─────────────────────────────────────────────────────────────┘ │
│                                                                   │
│  ┌─────────────────────────────────────────────────────────────┐ │
│  │                    Unix Socket Server                       │ │
│  │  CLI commands: init, work, list, send-message, etc.        │ │
│  └─────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────┘
              │                    │                    │
              ▼                    ▼                    ▼
    ┌─────────────────────────────────────────────────────────────┐
    │                    tmux: mc-<repo>                          │
    │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐    │
    │  │supervisor│  │merge-queue│  │worker-1  │  │worker-2  │    │
    │  │(Claude)  │  │(Claude)   │  │(Claude)  │  │(Claude)  │    │
    │  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘    │
    └───────┼─────────────┼────────────┼────────────┼─────────────┘
            │             │            │            │
            ▼             ▼            ▼            ▼
    ┌─────────────────────────────────────────────────────────────┐
    │                    Git Worktrees                            │
    │  ~/.multiclaude/wts/<repo>/                                 │
    │    supervisor/  merge-queue/  worker-1/  worker-2/          │
    └─────────────────────────────────────────────────────────────┘
```

## Directory Structure

```
~/.multiclaude/
├── daemon.pid              # Daemon process ID
├── daemon.sock             # Unix socket for CLI
├── daemon.log              # Daemon activity log
├── state.json              # Persisted state
├── prompts/                # Generated prompt files
│   └── <repo>-<agent>.md
├── repos/                  # Cloned repositories
│   └── <repo>/             # Main clone (bare or regular)
├── wts/                    # Git worktrees
│   └── <repo>/
│       ├── supervisor/
│       ├── merge-queue/
│       └── <worker-name>/
└── messages/               # Inter-agent messages
    └── <repo>/
        └── <agent>/
            └── <msg-id>.json
```

## State Model

State is stored in memory and persisted to `state.json`:

```json
{
  "repos": {
    "my-repo": {
      "github_url": "https://github.com/org/my-repo",
      "local_path": "/Users/user/.multiclaude/repos/my-repo",
      "tmux_session": "mc-my-repo",
      "agents": {
        "supervisor": {
          "type": "supervisor",
          "worktree_path": "...",
          "tmux_window": "supervisor",
          "session_id": "550e8400-e29b-41d4-a716-446655440000",
          "pid": 12345,
          "created_at": "2026-01-18T10:00:00Z",
          "last_nudge": "2026-01-18T10:30:00Z"
        },
        "happy-platypus": {
          "type": "worker",
          "task": "Add authentication tests",
          "worktree_path": "...",
          "tmux_window": "happy-platypus",
          "session_id": "...",
          "pid": 12347,
          "created_at": "...",
          "ready_for_cleanup": false
        }
      }
    }
  }
}
```

## Daemon Loops

**Health Check Loop (every 2 minutes)**
- Verify tmux windows exist
- Check Claude process PIDs are alive
- Mark dead agents for cleanup
- Remove orphaned resources

**Message Router Loop (every 2 minutes)**
- Scan `messages/` for pending messages
- Deliver via `tmux send-keys`
- Update message status

**Nudge Loop (every 2 minutes)**
- Send status check to agents that haven't been active
- Exponential backoff to avoid spam
- Wake supervisor and merge-queue periodically

## Message System

Agents communicate via JSON files in `~/.multiclaude/messages/<repo>/<agent>/`:

```json
{
  "id": "msg-abc123",
  "from": "supervisor",
  "to": "happy-platypus",
  "timestamp": "2026-01-18T10:30:00Z",
  "body": "How's the authentication work going?",
  "status": "pending"
}
```

**Status progression:** `pending` → `delivered` → `read` → `acked` → deleted

**Delivery:** Daemon uses `tmux send-keys -t session:window "message" C-m` to inject messages into the agent's stdin.

## CLI Protocol

CLI communicates with daemon via Unix socket at `~/.multiclaude/daemon.sock`.

**Request format:**
```json
{
  "command": "add_agent",
  "args": {
    "repo": "my-repo",
    "name": "happy-platypus",
    "type": "worker",
    "task": "Add tests"
  }
}
```

**Response format:**
```json
{
  "success": true,
  "data": { ... },
  "error": ""
}
```

## Agent Lifecycle

### Repository Initialization

`multiclaude init <github-url>`

1. Clone repo to `~/.multiclaude/repos/<name>`
2. Create tmux session `mc-<name>`
3. Create supervisor worktree and window, start Claude
4. Create merge-queue worktree and window, start Claude
5. Register in daemon state

### Worker Creation

`multiclaude work "task description"`

1. Generate Docker-style name
2. Create worktree from main (or `--branch`)
3. Create tmux window
4. Start Claude with worker prompt and task
5. Register in daemon state

### Worker Completion

1. Worker creates PR via `gh pr create`
2. Worker runs `multiclaude agent complete`
3. Daemon marks `ready_for_cleanup: true`
4. Daemon cleans up worktree and tmux window

### Worker Removal

`multiclaude work rm <name>`

1. Check for uncommitted changes (warn if found)
2. Check for unpushed commits (warn if found)
3. Remove tmux window
4. Remove git worktree
5. Update state

## Claude Integration

### Startup Command

```bash
claude --session-id "<uuid>" \
  --append-system-prompt-file "<prompt-file>" \
  --dangerously-skip-permissions
```

- `--session-id` enables context persistence across restarts
- `--append-system-prompt-file` loads role-specific instructions
- `--dangerously-skip-permissions` enables autonomous operation

### Role Prompts

Default prompts are embedded in the binary. Repositories can customize agents with:
- `.multiclaude/agents/worker.md` - Worker agent definition
- `.multiclaude/agents/merge-queue.md` - Merge-queue agent definition
- `.multiclaude/agents/review.md` - Review agent definition

**Deprecated:** The old files (`SUPERVISOR.md`, `WORKER.md`, `REVIEWER.md` directly in `.multiclaude/`) are deprecated.

### Hooks Configuration

If `.multiclaude/hooks.json` exists, it's copied to the worktree's `.claude/settings.json` for Claude Code hooks integration.

## Error Handling

**Daemon not running:**
```
Error: multiclaude daemon is not running
Start it with: multiclaude start
```

**Uncommitted work on cleanup:**
```
Warning: Worker 'happy-platypus' has uncommitted changes:
  M src/auth.ts
Continue with cleanup? [y/N]
```

**Agent crash:**
- Health loop detects dead PID
- Logs warning to daemon.log
- Marks agent for potential restart

## Design Principles

1. **Transparency** - All work visible via tmux
2. **Isolation** - Each agent has its own worktree
3. **Recovery** - State persists, daemon recovers from crashes
4. **Safety** - CI is sacred, never weakened without human approval
5. **Simplicity** - Filesystem for messages, tmux for visibility, git for isolation

## Golden Rules

These rules are embedded in agent prompts:

1. **If CI passes, the code can go in.** Never reduce or weaken CI without explicit human approval.

2. **Forward progress trumps all.** Any incremental progress is good. A reviewable PR is progress. The only failure is an agent that doesn't move forward at all.

## Package Structure

```
cmd/multiclaude/        # CLI entry point
internal/
  cli/                  # Command routing
  daemon/               # Daemon process and loops
  logging/              # Structured logging
  messages/             # Message filesystem operations
  names/                # Docker-style name generation
  prompts/              # Role-specific prompts
  socket/               # Unix socket client/server
  state/                # JSON state persistence
  tmux/                 # tmux session management
  worktree/             # Git worktree operations
pkg/
  config/               # Path configuration
```

## Implementation Status

- **Phase 1: Core Infrastructure** - Complete
- **Phase 2: Daemon & Management** - Complete
- **Phase 3: Claude Integration** - Complete
- **Phase 4: Polish & Refinement** - In Progress

## Future Considerations

Not in current scope:
- Web dashboard
- Cross-repo coordination
- Multi-user support
- Remote daemon
- Automatic agent respawning

Possible enhancements:
- Cost tracking
- Agent templates
- Issue tracker integration
- Budget controls
