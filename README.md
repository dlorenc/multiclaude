# multiclaude

A lightweight orchestrator for running multiple Claude Code agents on GitHub repositories.

multiclaude spawns and coordinates autonomous Claude Code instances that work together on your codebase. Each agent runs in its own tmux window with an isolated git worktree, making all work observable and interruptible at any time.

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

## How It Works

multiclaude creates a tmux session for each repository with three types of agents:

1. **Supervisor** - Coordinates all agents, answers status questions, nudges stuck workers
2. **Workers** - Execute specific tasks, create PRs when done
3. **Merge Queue** - Monitors PRs, merges when CI passes, spawns fixup workers as needed

Agents communicate via a filesystem-based message system. The daemon routes messages and periodically nudges agents to keep work moving forward.

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

## Commands

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
multiclaude init <github-url> [path] [name] # With custom local path or name
multiclaude list                           # List tracked repositories
```

### Workers

```bash
multiclaude work "task description"        # Create worker for task
multiclaude work "task" --branch feature   # Start from specific branch
multiclaude work list                      # List active workers
multiclaude work rm <name>                 # Remove worker (warns if uncommitted work)
```

### Observing

```bash
multiclaude attach <agent-name>            # Attach to agent's tmux window
multiclaude attach <agent-name> --read-only # Observe without interaction
tmux attach -t mc-<repo>                   # Attach to entire repo session
```

### Agent Commands (run from within Claude)

```bash
multiclaude agent send-message <to> "msg"  # Send message to another agent
multiclaude agent send-message --all "msg" # Broadcast to all agents
multiclaude agent list-messages            # List incoming messages
multiclaude agent ack-message <id>         # Acknowledge a message
multiclaude agent complete                 # Signal task completion (workers)
```

## Directory Structure

```
~/.multiclaude/
├── daemon.pid          # Daemon process ID
├── daemon.sock         # Unix socket for CLI
├── daemon.log          # Daemon logs
├── state.json          # Persisted state
├── repos/<repo>/       # Cloned repositories
├── wts/<repo>/         # Git worktrees (supervisor, merge-queue, workers)
└── messages/<repo>/    # Inter-agent messages
```

## Repository Configuration

Repositories can include optional configuration in `.multiclaude/`:

```
.multiclaude/
├── SUPERVISOR.md   # Additional instructions for supervisor
├── WORKER.md       # Additional instructions for workers
├── REVIEWER.md     # Additional instructions for merge queue
└── hooks.json      # Claude Code hooks configuration
```

## Design Principles

1. **Observable** - All agent activity visible via tmux. Attach anytime to watch or intervene.
2. **Isolated** - Each agent works in its own git worktree. No interference between tasks.
3. **Recoverable** - State persists to disk. Daemon recovers gracefully from crashes.
4. **Safe** - Agents never weaken CI or bypass checks without human approval.
5. **Simple** - Minimal abstractions. Filesystem for state, tmux for visibility, git for isolation.

## Golden Rules

Two principles guide all agent behavior:

1. **If CI passes, the code can go in.** CI is the source of truth. Never reduce or weaken CI without explicit human approval.

2. **Forward progress trumps all.** Any incremental progress is good. A reviewable PR is progress. The only failure is an agent that doesn't push the ball forward at all.

## Philosophy: The Brownian Ratchet

multiclaude embraces a counterintuitive design principle: **chaos is fine, as long as we ratchet forward**.

In physics, a Brownian ratchet is a thought experiment where random molecular motion is converted into directed movement through a mechanism that allows motion in only one direction. multiclaude applies this principle to software development.

**The Chaos**: Multiple autonomous agents work simultaneously on overlapping concerns. They may duplicate effort, create conflicting changes, or produce suboptimal solutions. This apparent disorder is not a bug—it's a feature. More attempts mean more chances for progress.

**The Ratchet**: CI is the arbiter. If it passes, the code goes in. Every merged PR clicks the ratchet forward one notch. Progress is permanent—we never go backward. The merge queue agent serves as this ratchet mechanism, ensuring that any work meeting the CI bar gets incorporated.

**Why This Works**:
- Agents don't need perfect coordination. Redundant work is cheaper than blocked work.
- Failed attempts cost nothing. Only successful attempts matter.
- Incremental progress compounds. Many small PRs beat waiting for one perfect PR.
- The system is antifragile. More agents mean more chaos but also more forward motion.

This philosophy means we optimize for throughput of successful changes, not efficiency of individual agents. An agent that produces a mergeable PR has succeeded, even if another agent was working on the same thing.

## Acknowledgements

### Gastown

multiclaude was developed independently but shares similar goals with [Gastown](https://github.com/steveyegge/gastown), Steve Yegge's multi-agent orchestrator for Claude Code released in January 2026.

**Similarities:**
- Both are Go-based orchestrators for multiple Claude Code instances
- Both use tmux for session management and human observability
- Both use git worktrees for isolated agent workspaces
- Both coordinate agents working on GitHub repositories

**Key Differences:**

| Aspect | multiclaude | Gastown |
|--------|-------------|---------|
| Agent model | 3 roles: supervisor, worker, merge-queue | 7 roles: Mayor, Polecats, Refinery, Witness, Deacon, Dogs, Crew |
| State persistence | JSON file + filesystem | Git-backed "hooks" for crash recovery |
| Work tracking | Simple task descriptions | "Beads" framework for structured work units |
| Communication | Filesystem-based messages | Built on Beads framework |
| Philosophy | Minimal, Unix-style simplicity | Comprehensive orchestration system |
| Maturity | Early development | More established, larger feature set |

multiclaude aims to be a simpler, more lightweight alternative. If you need sophisticated orchestration features, work swarming, or built-in crash recovery, Gastown may be a better fit.

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

## License

MIT
