# multiclaude

A lightweight orchestrator for running multiple Claude Code agents on GitHub repositories.

multiclaude spawns and coordinates autonomous Claude Code instances that work together on your codebase. Each agent runs in its own tmux window with an isolated git worktree, making all work observable and interruptible at any time.

## Table of Contents

- [What It Does](#what-it-does)
- [Philosophy: The Brownian Ratchet](#philosophy-the-brownian-ratchet)
- [Quick Start](#quick-start)
- [Architecture](#architecture)
- [Commands Reference](#commands-reference)
- [Working with multiclaude](#working-with-multiclaude)
- [Public Libraries](#public-libraries)
- [Building](#building)
- [Requirements](#requirements)

## What It Does

multiclaude lets you run multiple Claude Code instances in parallel, each working on different tasks for the same repository. Instead of manually managing separate terminal sessions, multiclaude:

- **Spawns autonomous agents** - Each agent is a full Claude Code instance with its own task
- **Isolates work via git worktrees** - Agents can't interfere with each other's changes
- **Coordinates via tmux** - Watch any agent work in real-time, or let them run unattended
- **Manages the merge lifecycle** - A dedicated merge-queue agent monitors PRs and merges when CI passes

```
┌─────────────────────────────────────────────────────────────┐
│                     tmux session: mc-repo                   │
├───────────────┬───────────────┬───────────────┬─────────────┤
│  supervisor   │  merge-queue  │ happy-platypus│ clever-fox  │
│   (Claude)    │   (Claude)    │   (Claude)    │  (Claude)   │
│               │               │               │             │
│ Coordinates   │ Merges PRs    │ Working on    │ Working on  │
│ all agents    │ when CI green │ task #1       │ task #2     │
└───────────────┴───────────────┴───────────────┴─────────────┘
        │                │               │               │
        └────────────────┴───────────────┴───────────────┘
                    isolated git worktrees
```

### Agent Types

| Agent | Role | Lifecycle |
|-------|------|-----------|
| **Supervisor** | Coordinates agents, answers status questions, nudges stuck workers | Persistent |
| **Merge Queue** | Monitors PRs, merges when CI passes, spawns fixup workers | Persistent |
| **Worker** | Executes a specific task, creates a PR when done | Ephemeral |
| **Workspace** | Your persistent interactive session for spawning workers | Persistent |

## Philosophy: The Brownian Ratchet

multiclaude embraces a counterintuitive design principle: **chaos is fine, as long as we ratchet forward**.

In physics, a Brownian ratchet is a thought experiment where random molecular motion is converted into directed movement through a mechanism that allows motion in only one direction. multiclaude applies this principle to software development.

### The Chaos

Multiple autonomous agents work simultaneously on overlapping concerns. They may duplicate effort, create conflicting changes, or produce suboptimal solutions. This apparent disorder is not a bug—it's a feature. More attempts mean more chances for progress.

### The Ratchet

CI is the arbiter. If it passes, the code goes in. Every merged PR clicks the ratchet forward one notch. Progress is permanent—we never go backward. The merge-queue agent serves as this ratchet mechanism, ensuring that any work meeting the CI bar gets incorporated.

### Why This Works

- **Agents don't need perfect coordination** - Redundant work is cheaper than blocked work
- **Failed attempts cost nothing** - Only successful attempts matter
- **Incremental progress compounds** - Many small PRs beat waiting for one perfect PR
- **The system is antifragile** - More agents mean more chaos but also more forward motion

### Core Beliefs

These aren't configuration options—they're baked into how the system works:

1. **CI is King** - If tests pass, the code can ship. If tests fail, it doesn't. Agents never weaken CI to make work pass.

2. **Forward Progress Over Perfection** - Any incremental progress is good. A reviewable PR is progress. The only failure is an agent that doesn't push the ball forward at all.

3. **Chaos is Expected** - Multiple agents will create conflicts and duplicate work. This is fine. Wasted work is cheap; blocked work is expensive.

4. **Humans Approve, Agents Execute** - Agents create PRs for human review. They don't bypass review requirements or merge without appropriate approval.

## Quick Start

```bash
# Install
go install github.com/dlorenc/multiclaude/cmd/multiclaude@latest

# Prerequisites: tmux, git, gh (GitHub CLI authenticated)

# Start the daemon
multiclaude start

# Initialize a repository
multiclaude init https://github.com/your/repo

# Create a worker to do a task
multiclaude work "Add unit tests for the auth module"

# Watch agents work
tmux attach -t mc-repo
```

### What Happens

1. **`multiclaude start`** - Launches a background daemon that coordinates everything
2. **`multiclaude init`** - Clones the repo, creates a tmux session, spawns supervisor and merge-queue agents
3. **`multiclaude work`** - Creates a new worker agent with its own branch and worktree
4. **`tmux attach`** - Connect to watch agents work (use `Ctrl-b n/p` to switch windows)

## Architecture

### System Overview

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
│  │  CLI commands → daemon → state updates → tmux management    │ │
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

### Design Principles

| Principle | Description |
|-----------|-------------|
| **Observable** | All agent activity visible via tmux. Attach anytime to watch or intervene. |
| **Isolated** | Each agent works in its own git worktree. No interference between tasks. |
| **Recoverable** | State persists to disk. Daemon recovers gracefully from crashes. |
| **Safe** | Agents never weaken CI or bypass checks without human approval. |
| **Simple** | Filesystem for state, tmux for visibility, git for isolation. |

### Directory Structure

```
~/.multiclaude/
├── daemon.pid          # Daemon process ID
├── daemon.sock         # Unix socket for CLI
├── daemon.log          # Daemon logs
├── state.json          # Persisted state
├── repos/<repo>/       # Cloned repositories
├── wts/<repo>/         # Git worktrees (supervisor, merge-queue, workers)
├── messages/<repo>/    # Inter-agent messages
└── claude-config/<repo>/<agent>/  # Per-agent Claude configuration
```

### Agent Communication

Agents communicate via filesystem-based messaging. The daemon routes messages between agents every 2 minutes:

```
supervisor ─────message────▶ worker
     ▲                          │
     │                          │
     └──────────reply───────────┘
```

Messages are JSON files in `~/.multiclaude/messages/<repo>/<agent>/` with status progression: `pending` → `delivered` → `acked`

## Commands Reference

### Daemon

```bash
multiclaude start              # Start the daemon
multiclaude daemon stop        # Stop the daemon
multiclaude daemon status      # Show daemon status
multiclaude daemon logs -f     # Follow daemon logs
multiclaude stop-all           # Stop everything, kill all tmux sessions
multiclaude stop-all --clean   # Stop and remove all state files
```

### Repositories

```bash
multiclaude init <github-url>              # Initialize repository tracking
multiclaude init <github-url> [name]       # With custom name
multiclaude list                           # List tracked repositories
multiclaude repo rm <name>                 # Remove a tracked repository
```

### Workspaces

Workspaces are persistent Claude sessions where you interact with the codebase and spawn workers.

```bash
multiclaude workspace add <name>           # Create a new workspace
multiclaude workspace add <name> --branch main  # Create from specific branch
multiclaude workspace list                 # List all workspaces
multiclaude workspace connect <name>       # Attach to a workspace
multiclaude workspace rm <name>            # Remove workspace
multiclaude workspace <name>               # Connect shorthand
```

### Workers

```bash
multiclaude work "task description"        # Create worker for task
multiclaude work "task" --branch feature   # Start from specific branch
multiclaude work "Fix tests" --branch origin/work/fox --push-to work/fox  # Iterate on existing PR
multiclaude work list                      # List active workers
multiclaude work rm <name>                 # Remove worker
```

### Observing

```bash
multiclaude attach <agent-name>            # Attach to agent's tmux window
multiclaude attach <agent-name> --read-only # Observe without interaction
tmux attach -t mc-<repo>                   # Attach to entire repo session
multiclaude logs <agent-name>              # View agent output logs
multiclaude logs <agent-name> -f           # Follow agent logs
```

### Agent Commands (from within Claude)

```bash
multiclaude agent send-message <to> "msg"  # Send message to another agent
multiclaude agent list-messages            # List incoming messages
multiclaude agent ack-message <id>         # Acknowledge a message
multiclaude agent complete                 # Signal task completion (workers)
```

### Agent Slash Commands

Agents have access to multiclaude-specific slash commands:

- `/refresh` - Sync worktree with main branch
- `/status` - Show system status and pending messages
- `/workers` - List active workers for the repo
- `/messages` - Check inter-agent messages

## Working with multiclaude

### Spawning Workers from Your Workspace

Connect to your workspace and spawn workers for parallel tasks:

```
> Let's tackle issues #44 and #45 in parallel

╭─────────────────────────────────────────────────────────────╮
│ I'll spawn workers for both issues.                         │
│                                                             │
│ > multiclaude work "Implement rich list commands per #44"   │
│ ✓ Worker created: swift-eagle (branch: work/swift-eagle)    │
│                                                             │
│ > multiclaude work "Improve error messages per #45"         │
│ ✓ Worker created: calm-deer (branch: work/calm-deer)        │
│                                                             │
│ Both workers are now running. Check on them with:           │
│   multiclaude work list                                     │
│   multiclaude attach swift-eagle                            │
╰─────────────────────────────────────────────────────────────╯
```

### Watching Agents Work

Use tmux navigation to switch between agent windows:

- `Ctrl-b n` / `Ctrl-b p` — Next/previous window
- `Ctrl-b 0-9` — Jump to window by number
- `Ctrl-b w` — Window picker
- `Ctrl-b d` — Detach (agents keep running)

### The Merge Queue in Action

The merge-queue agent automatically monitors and merges PRs:

```
╭─────────────────────────────────────────────────────────────╮
│ Checking open PRs...                                        │
│                                                             │
│ > gh pr list --author @me                                   │
│ #47  Add rich list commands      swift-eagle                │
│ #48  Improve error messages      calm-deer                  │
│                                                             │
│ Checking CI status for #47...                               │
│ ✓ All checks passed                                         │
│                                                             │
│ PR #47 is ready to merge!                                   │
│ > gh pr merge 47 --squash --auto                            │
│ ✓ Merged #47 into main                                      │
╰─────────────────────────────────────────────────────────────╯
```

When CI fails, the merge-queue spawns fixup workers automatically.

### Repository Configuration

Repositories can include optional configuration in `.multiclaude/`:

```
.multiclaude/
├── SUPERVISOR.md   # Additional instructions for supervisor
├── WORKER.md       # Additional instructions for workers
├── REVIEWER.md     # Additional instructions for merge queue
└── hooks.json      # Claude Code hooks configuration
```

## Public Libraries

multiclaude includes two reusable Go packages:

### pkg/tmux - Programmatic tmux Interaction

```bash
go get github.com/dlorenc/multiclaude/pkg/tmux
```

Features for programmatic interaction with running CLI applications:
- Multiline text via paste-buffer (atomic input without triggering intermediate processing)
- Pane PID extraction for process monitoring
- pipe-pane output capture for logging

```go
client := tmux.NewClient()
client.SendKeysLiteral("session", "window", "multi\nline\ntext")
pid, _ := client.GetPanePID("session", "window")
client.StartPipePane("session", "window", "/tmp/output.log")
```

[Full documentation →](pkg/tmux/README.md)

### pkg/claude - Claude Code Runner

```bash
go get github.com/dlorenc/multiclaude/pkg/claude
```

A library for launching and managing Claude Code instances:
- Terminal abstraction (works with tmux or custom implementations)
- Session management with automatic UUID session IDs
- Output capture and multiline message support

```go
runner := claude.NewRunner(
    claude.WithTerminal(tmuxClient),
    claude.WithBinaryPath(claude.ResolveBinaryPath()),
)
result, _ := runner.Start("session", "window", claude.Config{
    SystemPromptFile: "/path/to/prompt.md",
})
runner.SendMessage("session", "window", "Hello, Claude!")
```

[Full documentation →](pkg/claude/README.md)

## Building

```bash
# Build
go build ./cmd/multiclaude

# Run tests
go test ./...

# Install locally
go install ./cmd/multiclaude
```

## Requirements

- Go 1.21+
- tmux
- git
- GitHub CLI (`gh`) authenticated via `gh auth login`

## Related Projects

multiclaude shares similar goals with [Gastown](https://github.com/steveyegge/gastown), Steve Yegge's multi-agent orchestrator. Both coordinate multiple Claude Code instances using tmux and git worktrees. multiclaude aims to be a simpler, more lightweight alternative.

## License

MIT
