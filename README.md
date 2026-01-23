# multiclaude

A lightweight orchestrator for running multiple Claude Code agents on GitHub repositories.

multiclaude spawns and coordinates autonomous Claude Code instances that work together on your codebase. Each agent runs in its own tmux window with an isolated git worktree, making all work observable and interruptible at any time.

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

## Our Opinions

multiclaude is intentionally opinionated. These aren't configuration options—they're core beliefs baked into how the system works:

### CI is King

CI is the source of truth. Period. If tests pass, the code can ship. If tests fail, the code doesn't ship. There's no "but the change looks right" or "I'm pretty sure it's fine." The automation decides.

Agents are forbidden from weakening CI to make their work pass. No skipping tests, no reducing coverage requirements, no "temporary" workarounds. If an agent can't pass CI, it asks for help or tries a different approach.

### Forward Progress Over Perfection

Any incremental progress is good. A reviewable PR is progress. A partial implementation with tests is progress. The only failure is an agent that doesn't push the ball forward at all.

This means we'd rather have three okay PRs than wait for one perfect PR. We'd rather merge working code now and improve it later than block on getting everything right the first time. Small, frequent commits beat large, infrequent ones.

### Chaos is Expected

Multiple agents working simultaneously will create conflicts, duplicate work, and occasionally step on each other's toes. This is fine. This is the plan.

Trying to perfectly coordinate agent work is both expensive and fragile. Instead, we let chaos happen and use CI as the ratchet that captures forward progress. Wasted work is cheap; blocked work is expensive.

### Humans Approve, Agents Execute

Agents do the work. Humans set the direction and approve the results. Agents should never make decisions that require human judgment—they should ask.

This means agents create PRs for human review. Agents ask the supervisor when they're stuck. Agents don't bypass review requirements or merge without appropriate approval. The merge queue agent can auto-merge, but only when CI passes and review requirements are met.

## Gastown and multiclaude

multiclaude was developed independently but shares similar goals with [Gastown](https://github.com/steveyegge/gastown), Steve Yegge's multi-agent orchestrator for Claude Code released in January 2026.

Both projects solve the same fundamental problem: coordinating multiple Claude Code instances working on a shared codebase. Both use Go, tmux for observability, and git worktrees for isolation. If you're evaluating multi-agent orchestrators, you should look at both.

**Where they differ:**

| Aspect | multiclaude | Gastown |
|--------|-------------|---------|
| Agent model | 3 roles: supervisor, worker, merge-queue | 7 roles: Mayor, Polecats, Refinery, Witness, Deacon, Dogs, Crew |
| State persistence | JSON file + filesystem | Git-backed "hooks" for crash recovery |
| Work tracking | Simple task descriptions | "Beads" framework for structured work units |
| Communication | Filesystem-based messages | Built on Beads framework |
| Philosophy | Minimal, Unix-style simplicity | Comprehensive orchestration system |
| Maturity | Early development | More established, larger feature set |

multiclaude aims to be a simpler, more lightweight alternative—the "worse is better" approach. If you need sophisticated orchestration features, work swarming, or built-in crash recovery, Gastown may be a better fit.

### Remote-First: Software is an MMORPG

The biggest philosophical difference: **multiclaude is designed for remote-first collaboration**.

Gastown treats agents as NPCs in a single-player game. You're the player, agents are your minions. This works great for solo development where you want to parallelize your own work.

multiclaude treats software engineering as an **MMORPG**. You're one player among many—some human, some AI. The workspace agent is your character, but other humans have their own workspaces. Workers are party members you spawn for quests. The supervisor coordinates the guild. The merge queue is the raid boss that decides what loot (code) makes it into the vault (main branch).

This means:
- **Your workspace persists**. It's your home base, not a temporary session.
- **You interact with workers, not control them**. Spawn them with a task, check on them later.
- **Other humans can have their own workspaces** on the same repo.
- **The system keeps running when you're away**. Agents work, PRs merge, CI runs.

The workspace is where you hop in to spawn agents, check on progress, review what landed, and plan the next sprint—then hop out and let the system work while you sleep.

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
multiclaude repo rm <name>                 # Remove a tracked repository
```

### Workspaces

Workspaces are persistent Claude sessions where you interact with the codebase, spawn workers, and manage your development flow. Each workspace has its own git worktree, tmux window, and Claude instance.

```bash
multiclaude workspace add <name>           # Create a new workspace
multiclaude workspace add <name> --branch main  # Create from specific branch
multiclaude workspace list                 # List all workspaces
multiclaude workspace connect <name>       # Attach to a workspace
multiclaude workspace rm <name>            # Remove workspace (warns if uncommitted work)
multiclaude workspace                      # List workspaces (shorthand)
multiclaude workspace <name>               # Connect to workspace (shorthand)
```

**Notes:**
- Workspaces use the branch naming convention `workspace/<name>`
- Workspace names follow git branch naming rules (no spaces, special characters, etc.)
- A "default" workspace is created automatically when you run `multiclaude init`
- Use `multiclaude attach <workspace-name>` as an alternative to `workspace connect`

### Workers

```bash
multiclaude work "task description"        # Create worker for task
multiclaude work "task" --branch feature   # Start from specific branch
multiclaude work "Fix tests" --branch origin/work/fox --push-to work/fox  # Iterate on existing PR
multiclaude work list                      # List active workers
multiclaude work rm <name>                 # Remove worker (warns if uncommitted work)
```

The `--push-to` flag creates a worker that pushes to an existing branch instead of creating a new PR. Use this when you want to iterate on an existing PR.

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

### Agent Slash Commands (available within Claude sessions)

Agents have access to multiclaude-specific slash commands:

- `/refresh` - Sync worktree with main branch
- `/status` - Show system status and pending messages
- `/workers` - List active workers for the repo
- `/messages` - Check inter-agent messages

### Agent Definitions

Manage configurable agent definitions:

```bash
multiclaude agents list                    # List available agent definitions
multiclaude agents reset                   # Reset to built-in templates
multiclaude agents spawn --name <n> --class <c> --prompt-file <f>  # Spawn custom agent
```

Agent definitions in `~/.multiclaude/repos/<repo>/agents/` customize agent behavior.
Definitions checked into `<repo>/.multiclaude/agents/` are shared with your team.

## Working with multiclaude

### What the tmux Session Looks Like

When you attach to a repo's tmux session, you'll see multiple windows—one per agent:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ mc-myrepo: supervisor | merge-queue | workspace | swift-eagle | calm-deer   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  $ claude                                                                   │
│                                                                             │
│  ╭─────────────────────────────────────────────────────────────────────────╮│
│  │ I'll check on the current workers and see if anyone needs help.        ││
│  │                                                                         ││
│  │ > multiclaude work list                                                 ││
│  │ Workers (2):                                                            ││
│  │   - swift-eagle: working on issue #44                                   ││
│  │   - calm-deer: working on issue #24                                     ││
│  │                                                                         ││
│  │ Both workers are making progress. swift-eagle just pushed a commit.    ││
│  │ I'll check back in a few minutes.                                       ││
│  ╰─────────────────────────────────────────────────────────────────────────╯│
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  > What would you like to do?                                               │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

Use standard tmux navigation:
- `Ctrl-b n` / `Ctrl-b p` — Next/previous window
- `Ctrl-b 0-9` — Jump to window by number
- `Ctrl-b w` — Window picker
- `Ctrl-b d` — Detach (agents keep running)

### Workflow: Spawning Workers from Your Workspace

Your workspace is a persistent Claude session where you can spawn and manage workers:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ You (in workspace):                                                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  > Let's tackle issues #44 and #45 in parallel                              │
│                                                                             │
│  ╭─────────────────────────────────────────────────────────────────────────╮│
│  │ I'll spawn workers for both issues.                                     ││
│  │                                                                         ││
│  │ > multiclaude work "Implement rich list commands per issue #44"         ││
│  │ ✓ Worker created: swift-eagle (branch: work/swift-eagle)                ││
│  │                                                                         ││
│  │ > multiclaude work "Improve error messages per issue #45"               ││
│  │ ✓ Worker created: calm-deer (branch: work/calm-deer)                    ││
│  │                                                                         ││
│  │ Both workers are now running. You can check on them with:               ││
│  │   multiclaude work list                                                 ││
│  │   multiclaude attach swift-eagle                                        ││
│  ╰─────────────────────────────────────────────────────────────────────────╯│
│                                                                             │
│  > Great, let me know when they finish. I'm going to grab lunch.            │
│                                                                             │
│  ╭─────────────────────────────────────────────────────────────────────────╮│
│  │ Will do! I'll get notified when they complete and can summarize         ││
│  │ their PRs when you're back. The workers will keep running while         ││
│  │ you're away.                                                            ││
│  ╰─────────────────────────────────────────────────────────────────────────╯│
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

Later, when you return:

```
│  > I'm back. What happened while I was gone?                                │
│                                                                             │
│  ╭─────────────────────────────────────────────────────────────────────────╮│
│  │ Welcome back! Here's the update:                                        ││
│  │                                                                         ││
│  │ ✓ swift-eagle completed - PR #47 created for rich list commands         ││
│  │ ✓ calm-deer completed - PR #48 created for error messages               ││
│  │                                                                         ││
│  │ Both PRs are passing CI. The merge queue is monitoring them.            ││
│  ╰─────────────────────────────────────────────────────────────────────────╯│
```

### Watching the Supervisor

The supervisor coordinates agents and provides status updates. Attach to watch it work:

```bash
multiclaude attach supervisor --read-only
```

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ Supervisor:                                                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ╭─────────────────────────────────────────────────────────────────────────╮│
│  │ [Periodic check - 14:32]                                                ││
│  │                                                                         ││
│  │ Checking agent status...                                                ││
│  │                                                                         ││
│  │ Agents:                                                                 ││
│  │   supervisor: healthy (me)                                              ││
│  │   merge-queue: healthy, monitoring 2 PRs                                ││
│  │   workspace: healthy, user attached                                     ││
│  │   swift-eagle: healthy, working on #44                                  ││
│  │   calm-deer: needs attention - stuck on test failure                    ││
│  │                                                                         ││
│  │ Sending help to calm-deer...                                            ││
│  │                                                                         ││
│  │ > multiclaude agent send-message calm-deer "I see you're stuck on a     ││
│  │   test failure. The flaky test in auth_test.go sometimes fails due to   ││
│  │   timing. Try adding a retry or mocking the clock."                     ││
│  ╰─────────────────────────────────────────────────────────────────────────╯│
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Watching the Merge Queue

The merge queue monitors PRs and merges them when CI passes:

```bash
multiclaude attach merge-queue --read-only
```

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ Merge Queue:                                                                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ╭─────────────────────────────────────────────────────────────────────────╮│
│  │ [PR Check - 14:45]                                                      ││
│  │                                                                         ││
│  │ Checking open PRs...                                                    ││
│  │                                                                         ││
│  │ > gh pr list --author @me                                               ││
│  │ #47  Add rich list commands      swift-eagle   work/swift-eagle         ││
│  │ #48  Improve error messages      calm-deer     work/calm-deer           ││
│  │                                                                         ││
│  │ Checking CI status for #47...                                           ││
│  │ > gh pr checks 47                                                       ││
│  │ ✓ All checks passed                                                     ││
│  │                                                                         ││
│  │ PR #47 is ready to merge!                                               ││
│  │ > gh pr merge 47 --squash --auto                                        ││
│  │ ✓ Merged #47 into main                                                  ││
│  │                                                                         ││
│  │ Notifying supervisor of merge...                                        ││
│  │ > multiclaude agent send-message supervisor "Merged PR #47: Add rich    ││
│  │   list commands"                                                        ││
│  ╰─────────────────────────────────────────────────────────────────────────╯│
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

When CI fails, the merge queue can spawn workers to fix it:

```
│  │ Checking CI status for #48...                                           ││
│  │ ✗ Tests failed: 2 failures in error_test.go                             ││
│  │                                                                         ││
│  │ Spawning fixup worker for #48...                                        ││
│  │ > multiclaude work "Fix test failures in PR #48" --branch work/calm-deer││
│  │ ✓ Worker created: quick-fox                                             ││
│  │                                                                         ││
│  │ I'll check back on #48 after quick-fox pushes a fix.                    ││
```

## Configurable Agents

multiclaude allows you to customize agent behavior through agent definitions - markdown files that define how workers, merge-queue, and review agents operate.

### What Can Be Customized

You can customize:
- **Worker** behavior - coding style, commit conventions, testing requirements
- **Merge-queue** behavior - merge policies, PR handling rules
- **Review** behavior - code review focus areas, comment style

Note: Supervisor and workspace agents use embedded prompts and cannot be customized.

### How to Customize

Agent definitions are stored in `~/.multiclaude/repos/<repo>/agents/`:

```bash
# List current agent definitions
multiclaude agents list

# Reset to built-in defaults (useful after upgrading)
multiclaude agents reset
```

Edit the definition files to customize behavior:
- `~/.multiclaude/repos/<repo>/agents/worker.md`
- `~/.multiclaude/repos/<repo>/agents/merge-queue.md`
- `~/.multiclaude/repos/<repo>/agents/reviewer.md`

### Sharing with Your Team

Check agent definitions into your repository to share them:

```
<repo>/.multiclaude/agents/
├── worker.md        # Team's worker conventions
├── merge-queue.md   # Team's merge policies
└── review.md        # Team's review guidelines
```

These take precedence over local definitions, ensuring all team members use consistent agent behavior.

### Precedence Order

1. `<repo>/.multiclaude/agents/<agent>.md` (checked into repo, highest priority)
2. `~/.multiclaude/repos/<repo>/agents/<agent>.md` (local overrides)
3. Built-in templates (fallback)

### Example: Customizing Worker Conventions

To make workers follow your project's coding conventions, edit the worker definition:

```bash
# Open the worker definition
$EDITOR ~/.multiclaude/repos/my-repo/agents/worker.md
```

Add project-specific instructions:
```markdown
## Project-Specific Guidelines

- Use conventional commits (feat:, fix:, docs:, etc.)
- All new functions require unit tests
- Follow the existing code style in src/
- Run `npm run lint` before creating PRs
```

## Architecture

### Design Principles

1. **Observable** - All agent activity visible via tmux. Attach anytime to watch or intervene.
2. **Isolated** - Each agent works in its own git worktree. No interference between tasks.
3. **Recoverable** - State persists to disk. Daemon recovers gracefully from crashes.
4. **Safe** - Agents never weaken CI or bypass checks without human approval.
5. **Simple** - Minimal abstractions. Filesystem for state, tmux for visibility, git for isolation.

### Directory Structure

```
~/.multiclaude/
├── daemon.pid          # Daemon process ID
├── daemon.sock         # Unix socket for CLI
├── daemon.log          # Daemon logs
├── state.json          # Persisted state
├── repos/<repo>/       # Cloned repositories
│   └── agents/         # Per-repo agent definitions (local overrides)
├── wts/<repo>/         # Git worktrees (supervisor, merge-queue, workers)
├── messages/<repo>/    # Inter-agent messages
└── claude-config/<repo>/<agent>/  # Per-agent Claude configuration (slash commands)
```

Repository-checked agent definitions in `<repo>/.multiclaude/agents/` take precedence over local definitions.

### Repository Configuration

Repositories can include optional configuration in `.multiclaude/`:

```
.multiclaude/
├── agents/              # Agent customization (recommended)
│   ├── worker.md        # Worker agent definition
│   ├── merge-queue.md   # Merge-queue agent definition
│   └── review.md        # Review agent definition
└── hooks.json           # Claude Code hooks configuration
```

Agent definitions in `.multiclaude/agents/` take precedence over local definitions in `~/.multiclaude/repos/<repo>/agents/` and built-in templates.

**Deprecated:** The old system using `SUPERVISOR.md`, `WORKER.md`, `REVIEWER.md` directly in `.multiclaude/` is deprecated. Migrate to the new `agents/` directory structure.

## Public Libraries

multiclaude includes two reusable Go packages that can be used independently of the orchestrator:

### pkg/tmux - Programmatic tmux Interaction

```bash
go get github.com/dlorenc/multiclaude/pkg/tmux
```

Unlike existing Go tmux libraries ([gotmux](https://github.com/GianlucaP106/gotmux), [go-tmux](https://github.com/jubnzv/go-tmux)) that focus on workspace setup, this package provides features for **programmatic interaction with running CLI applications**:

- **Multiline text via paste-buffer** - Send multi-line input atomically without triggering intermediate processing
- **Pane PID extraction** - Monitor whether processes in panes are still alive
- **pipe-pane output capture** - Capture all pane output to files for logging/analysis

```go
client := tmux.NewClient()
client.SendKeysLiteral("session", "window", "multi\nline\ntext")  // Uses paste-buffer
pid, _ := client.GetPanePID("session", "window")
client.StartPipePane("session", "window", "/tmp/output.log")
```

[Full documentation →](pkg/tmux/README.md)

### pkg/claude - Claude Code Runner

```bash
go get github.com/dlorenc/multiclaude/pkg/claude
```

A library for launching and interacting with Claude Code instances in terminals:

- **Terminal abstraction** - Works with tmux or custom terminal implementations
- **Session management** - Automatic UUID session IDs and process tracking
- **Output capture** - Route Claude output to files
- **Multiline support** - Properly handles multi-line messages to Claude

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

## License

MIT
