# multiclaude Architecture Specification

Version: 1.0
Last Updated: 2026-01-18

## Overview

multiclaude is a repo-centric orchestrator for managing multiple autonomous Claude Code instances working collaboratively on GitHub repositories. It uses tmux for session management, git worktrees for isolation, and a daemon-based architecture for coordination.

## Core Concepts

### Agent Model

An **agent** consists of three components:
1. **Git worktree** - Isolated working directory for the agent
2. **Claude Code instance** - Running in interactive mode with session tracking
3. **tmux window** - In a repo-specific session, allowing human observation/interaction

Agents run autonomously but humans can attach via tmux to observe or intervene at any time.

### Agent Types

There are three types of agents per repository:

1. **Supervisor Agent**
   - Manages overall repository coordination
   - Checks on all agents and nudges them when needed
   - Human interface to the controller daemon
   - Keeps worktree synced with main branch
   - Broad scope: task-level, project-level, and orchestration-level decisions

2. **Worker Agents**
   - Execute specific tasks (features, bug fixes, etc.)
   - Create PRs when work is complete
   - Signal completion and wait for daemon cleanup
   - Start from main branch by default (unless `--branch` specified)
   - Named with Docker-style generated names (e.g., "happy-platypus")

3. **Merge Queue Agent**
   - Monitors open PRs and drives them toward merge
   - Decides priority, parallelization, and strategy
   - Can spawn new workers to fix CI failures or address review feedback
   - **Hard constraint**: Never removes or weakens CI checks without human approval
   - Requests human help via PR labels/comments when needed
   - Always running but woken periodically by supervisor

### Controller Daemon

A single global daemon process that:
- Orchestrates all repos and agents
- Runs core loops (health checks, message routing, cleanup)
- Manages state (agents, worktrees, tmux sessions, messages)
- Drives the supervisor agents with notifications
- Supervisor agents provide intelligence and decision-making

**Daemon ↔ Supervisor relationship:**
- Daemon informs supervisor of events (messages, agent status, etc.)
- Supervisor instructs daemon on actions to take
- Users interact with supervisor via tmux to influence daemon behavior

## System Architecture

### Directory Structure

```
$HOME/.multiclaude/
├── daemon.pid              # Daemon process ID
├── daemon.sock             # Unix socket for CLI communication
├── daemon.log              # Daemon logs
├── state.json              # Persisted daemon state
├── repos/                  # Cloned repositories
│   └── <repo-name>/        # Main repo clone
├── wts/                    # Git worktrees
│   └── <repo-name>/
│       ├── supervisor/     # Supervisor worktree
│       ├── merge-queue/    # Merge queue worktree
│       └── <worker-name>/  # Worker worktrees
└── messages/               # Inter-agent messages
    └── <repo-name>/
        └── <agent-name>/   # Messages for specific agent
            └── *.json      # Message files
```

### State Management

**State Storage:**
- Primary: In-memory state in daemon process
- Backup: `state.json` synced to disk after every change and at shutdown
- Format: JSON

**State Contents:**
```json
{
  "repos": {
    "my-repo": {
      "github_url": "https://github.com/org/my-repo",
      "tmux_session": "multiclaude-my-repo",
      "agents": {
        "supervisor": {
          "type": "supervisor",
          "worktree_path": "/Users/user/.multiclaude/wts/my-repo/supervisor",
          "tmux_window": "supervisor",
          "session_id": "550e8400-...",
          "pid": 12345,
          "created_at": "2026-01-18T10:00:00Z",
          "last_nudge": "2026-01-18T10:30:00Z"
        },
        "merge-queue": {
          "type": "merge-queue",
          "worktree_path": "/Users/user/.multiclaude/wts/my-repo/merge-queue",
          "tmux_window": "merge-queue",
          "session_id": "660e8400-...",
          "pid": 12346,
          "created_at": "2026-01-18T10:00:00Z",
          "last_nudge": "2026-01-18T10:30:00Z"
        },
        "happy-platypus": {
          "type": "worker",
          "worktree_path": "/Users/user/.multiclaude/wts/my-repo/happy-platypus",
          "tmux_window": "happy-platypus",
          "session_id": "770e8400-...",
          "pid": 12347,
          "task": "Fix authentication bug in login flow",
          "created_at": "2026-01-18T10:15:00Z",
          "last_nudge": "2026-01-18T10:30:00Z",
          "ready_for_cleanup": false
        }
      }
    }
  }
}
```

**Recovery:**
- On daemon restart, load `state.json`
- Verify tmux sessions/windows still exist
- Check if Claude processes are still running (via PID)
- Clean up orphaned resources
- Warn about any inconsistencies

### Daemon Implementation

**Technology:** Go with PID file management

**Core Loops:**

1. **Health Check Loop** (every 5 minutes)
   - Check if agents are still alive (PID exists, tmux window exists)
   - Mark dead agents for cleanup
   - Clean up orphaned worktrees and tmux windows

2. **Nudge Loop** (every 10 minutes per agent)
   - Send status check messages to agents via `tmux send-keys`
   - Track last nudge time to avoid spam
   - Adjust cadence based on agent activity

3. **Message Router Loop** (event-driven)
   - Watch `messages/` directory for new message files
   - Route messages to recipients via `tmux send-keys`
   - Track message delivery state (pending → delivered → acked → deleted)

4. **Supervisor Wake Loop** (every 15 minutes)
   - Wake merge queue agent to check PRs
   - Wake supervisor to check on workers

**Communication:**
- CLI commands → Daemon: Unix socket at `~/.multiclaude/daemon.sock`
- Protocol: JSON over socket
- Daemon → Agents: `tmux send-keys` to inject messages into stdin

**Logging:**
- All daemon activity logged to `~/.multiclaude/daemon.log`
- Includes: agent lifecycle, message routing, errors, cleanup actions

## Message Passing

### Message CLI Commands

Agents communicate via CLI commands that handle filesystem and notifications:

```bash
# Send message (writes to FS, notifies daemon)
multiclaude agent send-message <recipient> <message>
multiclaude agent send-message --all <message>
multiclaude agent send-message recipient1,recipient2 <message>

# Read messages
multiclaude agent list-messages        # Show unread messages
multiclaude agent read-message <id>    # Read specific message

# Acknowledge message (marks as read, eventually deleted)
multiclaude agent ack-message <id>
```

### Message Format

Messages stored as JSON files in `~/.multiclaude/messages/<repo>/<agent>/<message-id>.json`:

```json
{
  "id": "msg-123e4567-e89b",
  "from": "supervisor",
  "to": "happy-platypus",
  "timestamp": "2026-01-18T10:30:00Z",
  "body": "How's the authentication fix going? Need any help?",
  "status": "delivered",
  "acked_at": null
}
```

**Message Lifecycle:**
1. `pending` - Written to filesystem, daemon not yet notified
2. `delivered` - Daemon notified, message sent to agent via tmux
3. `read` - Agent has read the message
4. `acked` - Agent has acknowledged the message
5. *deleted* - Removed from filesystem after ack (eventually)

**Body Format:** Markdown text, injected into agent's stdin

### Message Delivery

When daemon delivers a message:
```bash
# Daemon executes:
tmux send-keys -t session:window "Message from supervisor: How's it going?" C-m
```

Agent role prompts instruct them to:
- Read messages when they appear
- Acknowledge with `multiclaude agent ack-message <id>` after processing
- Respond via `multiclaude agent send-message` if needed

## Agent Lifecycle

### Repository Initialization

Command: `multiclaude init <github-url> [path] [name]`

**Sequence:**
1. Check if daemon is running (fail if not)
2. Generate repo name from URL if not provided
3. Check if repo already exists (error if it does)
4. Clone repo to `~/.multiclaude/repos/<repo-name>`
5. Create tmux session named `multiclaude-<repo-name>`
6. Create supervisor agent:
   - Worktree at `~/.multiclaude/wts/<repo-name>/supervisor` from main
   - tmux window named "supervisor"
   - Start Claude Code with supervisor role prompt
7. Create merge queue agent:
   - Worktree at `~/.multiclaude/wts/<repo-name>/merge-queue` from main
   - tmux window named "merge-queue"
   - Start Claude Code with merge queue role prompt
8. Register repo and agents in daemon state
9. Output tmux session name for user

### Worker Creation

Command: `multiclaude work -t <task> [name-or-url]`

**Sequence:**
1. Detect repo (from current directory or provided name)
2. Generate worker name (Docker-style) or use provided override
3. Create worktree from main at `~/.multiclaude/wts/<repo>/<worker-name>`
4. Create tmux window in repo session named `<worker-name>`
5. Start Claude Code in interactive mode:
   ```bash
   claude --session-id "<worker-id>" \
     --allowedTools "Bash,Read,Write,Edit,Grep,Glob"
   ```
6. Load worker role prompt from `.multiclaude/WORKER.md` if exists
7. Send initial task via tmux send-keys:
   ```bash
   tmux send-keys -t session:worker-name "Task: <task>" C-m
   ```
8. Register worker in daemon state
9. Output worker name and how to attach

**With Branch Flag:**
```bash
multiclaude work -t "Fix review comments" --branch my-feature-branch
```
Creates worktree from specified branch instead of main.

### Worker Completion

When worker finishes:
1. Worker creates PR using `gh pr create`
2. Worker signals completion: `multiclaude agent complete`
3. Daemon marks worker as `ready_for_cleanup: true`
4. Daemon cleanup loop eventually:
   - Checks if PR exists and is tracked
   - Removes tmux window
   - Removes worktree: `git worktree remove <path>`
   - Warns if uncommitted/unmerged work exists
   - Updates state

### Worker Cleanup Command

Command: `multiclaude work rm <name>`

**Behavior:**
- Checks for uncommitted changes (warns if found)
- Checks for unpushed commits (warns if found)
- Checks if PR exists but not merged (warns if found)
- Removes tmux window
- Removes worktree
- Updates daemon state

### Listing Workers

Command: `multiclaude work list`

**Output:**
```
Active workers for my-repo:

  happy-platypus    Fix authentication bug             10m ago
  clever-elephant   Add dark mode toggle               25m ago

Attach with: multiclaude attach <name>
Or: tmux attach -t multiclaude-my-repo:<name>
```

## Role-Specific Prompts

### System Prompt Files

Repositories can include optional configuration in `.multiclaude/`:

- **SUPERVISOR.md** - Additional instructions for supervisor agent
- **REVIEWER.md** - Instructions for merge queue agent
- **WORKER.md** - Instructions for worker agents

**Loading:**
- Checked at agent startup
- If present, appended to agent's system prompt via `--append-system-prompt-file`
- If absent, agent uses default role prompt

### Default Role Prompts

**Supervisor (built-in):**
```markdown
You are the supervisor agent for this repository. Your responsibilities:

- Monitor all worker agents and the merge queue agent
- Nudge agents when they seem stuck or need guidance
- Answer questions from the controller daemon about agent status
- When humans ask "what's everyone up to?", report on all active agents
- Keep your worktree synced with the main branch

You can communicate with agents using:
- multiclaude agent send-message <agent> <message>
- multiclaude agent list-messages
- multiclaude agent ack-message <id>

You work in coordination with the controller daemon, which handles
routing and scheduling. Ask humans for guidance when uncertain.
```

**Worker (built-in):**
```markdown
You are a worker agent assigned to a specific task. Your responsibilities:

- Complete the task you've been assigned
- Create a PR when your work is ready
- Signal completion with: multiclaude agent complete
- Communicate with the supervisor if you need help
- Acknowledge messages with: multiclaude agent ack-message <id>

Your work starts from the main branch in an isolated worktree.
When you create a PR, use the branch name: multiclaude/<your-agent-name>

After creating your PR, signal completion and wait for cleanup.
```

**Merge Queue (built-in):**
```markdown
You are the merge queue agent for this repository. Your responsibilities:

- Monitor all open PRs created by multiclaude workers
- Decide the best strategy to move PRs toward merge
- Prioritize which PRs to work on first
- Spawn new workers to fix CI failures or address review feedback
- Merge PRs when CI is green and conditions are met

CRITICAL CONSTRAINT: Never remove or weaken CI checks without explicit
human approval. If you need to bypass checks, request human assistance
via PR comments and labels.

Use these commands:
- gh pr list --label multiclaude
- gh pr status
- gh pr checks <pr-number>
- multiclaude work -t "Fix CI for PR #123" --branch <pr-branch>

Check .multiclaude/REVIEWER.md for repository-specific merge criteria.
```

## tmux Session Management

### Session Structure

Each repository has one tmux session:

**Session name:** `multiclaude-<repo-name>`

**Window structure:**
- Window 0: `supervisor` - Supervisor agent
- Window 1: `merge-queue` - Merge queue agent
- Window 2+: `<worker-name>` - Worker agents (dynamic)

**Pane structure:**
- Single pane per window (users can split if they attach)
- Just Claude Code running in each pane

### Attaching to Agents

Command: `multiclaude attach <agent-name> [--read-only]`

**Behavior:**
- Resolves agent name to tmux session:window
- Attaches to tmux window
- With `--read-only`: uses `tmux attach -r` for monitoring without interaction

**Manual attach:**
```bash
# Interactive
tmux attach -t multiclaude-my-repo:happy-platypus

# Read-only
tmux attach -t multiclaude-my-repo:happy-platypus -r
```

## GitHub Integration

### Authentication

Uses `gh` CLI for all GitHub operations.

**Prerequisite:** Users must authenticate `gh` before using multiclaude:
```bash
gh auth login
```

**Verification:** Daemon checks on startup:
```bash
gh auth status
```

### PR Creation

Workers create PRs using:
```bash
gh pr create --title "Fix: authentication bug" \
  --body "Fixes issue #123..." \
  --label "multiclaude" \
  --head "multiclaude/happy-platypus"
```

**Branch naming:** `multiclaude/<worker-name>`

**Labels:** All multiclaude PRs tagged with `multiclaude` label for filtering

### PR Monitoring

Merge queue agent uses:
```bash
# List multiclaude PRs
gh pr list --label multiclaude

# Check specific PR
gh pr status <pr-number>
gh pr checks <pr-number>

# Watch for CI completion
gh pr checks <pr-number> --watch
```

## Claude Code Integration

### Agent Startup

Each agent starts Claude Code in interactive mode:

```bash
# In tmux window, start Claude
claude --session-id "<agent-id>" \
  --allowedTools "Bash,Read,Write,Edit,Grep,Glob" \
  --append-system-prompt-file .multiclaude/WORKER.md  # if exists
```

**Session tracking:**
- `--session-id` persists context across restarts
- If Claude crashes, daemon restarts with same session ID

**Tool allowlist:**
- Pre-approved tools to avoid permission prompts
- Enables autonomous operation
- Can be customized per agent type

### Message Delivery

Daemon sends messages via tmux:
```bash
tmux send-keys -t session:window "Message from supervisor: How's it going?" C-m
```

Claude processes message from stdin and responds in the tmux window.

### Health Monitoring

Daemon monitors:
- **Process PID** - Check if Claude process is alive
- **tmux window** - Verify window still exists
- **Session activity** - Track last interaction time

On crash:
- Log error to daemon.log
- Mark agent for cleanup
- Optionally restart agent with same session ID

### Optional Hooks

If repository includes `.multiclaude/hooks.json`, copy to worktree:

```bash
cp .multiclaude/hooks.json <worktree>/.claude/settings.json
```

**Example hooks configuration:**
```json
{
  "hooks": {
    "SessionStart": [{
      "hooks": [{
        "type": "command",
        "command": "multiclaude agent session-start"
      }]
    }],
    "PreToolUse": [{
      "matcher": "Bash",
      "hooks": [{
        "type": "command",
        "command": "multiclaude agent validate-command"
      }]
    }],
    "PostToolUse": [{
      "matcher": "Write|Edit",
      "hooks": [{
        "type": "command",
        "command": "multiclaude agent log-change"
      }]
    }]
  }
}
```

**Fallback without hooks:**
- Agents manually run `multiclaude agent` commands
- Daemon monitors via git status and tmux activity
- No special interception of Claude actions

## CLI Commands

### Daemon Management

```bash
# Start daemon
multiclaude start
multiclaude daemon start  # alias

# Stop daemon
multiclaude daemon stop

# Check status
multiclaude daemon status

# View logs
multiclaude daemon logs [--follow]
```

### Repository Management

```bash
# Initialize repository
multiclaude init <github-url> [path] [name]

# List tracked repos
multiclaude list
```

### Worker Management

```bash
# Create worker
multiclaude work -t <task> [name-or-url]
multiclaude work -t <task> --branch <branch> [name]

# List workers
multiclaude work list [repo-name]

# Remove worker
multiclaude work rm <worker-name>
```

### Agent Interaction

```bash
# Attach to agent
multiclaude attach <agent-name> [--read-only]

# Agent commands (run from within Claude)
multiclaude agent send-message <recipient> <message>
multiclaude agent send-message --all <message>
multiclaude agent list-messages
multiclaude agent read-message <id>
multiclaude agent ack-message <id>
multiclaude agent complete  # Signal worker completion
```

### Maintenance

```bash
# Clean up orphaned resources
multiclaude cleanup [--dry-run]

# Repair state after crash
multiclaude repair
```

## Error Handling

### Daemon Not Running

Commands that require daemon fail with:
```
Error: multiclaude daemon is not running
Start it with: multiclaude start
```

### Authentication Failures

On daemon startup, check `gh auth status`:
```
Error: GitHub CLI not authenticated
Authenticate with: gh auth login
```

### Resource Cleanup

Warnings when removing workers with uncommitted work:
```
Warning: Worker 'happy-platypus' has uncommitted changes:
  M src/auth.ts
  ?? src/auth.test.ts

Continue with cleanup? [y/N]
```

### Orphaned Resources

`multiclaude cleanup` detects and removes:
- Worktrees without corresponding tmux windows
- tmux windows without corresponding worktrees
- Message files for non-existent agents
- State entries for dead agents

## Future Considerations

### Not in v1.0

These features are explicitly out of scope for the initial implementation:

- Web dashboard for monitoring agents
- Webhook-based PR notifications (polling only)
- Cross-repo coordination
- Agent-to-agent direct communication (must go through supervisor)
- Automatic agent respawning on crash (manual recovery only)
- Resource limits (workers, memory, etc.)
- Multi-user support (single user per daemon)
- Remote daemon (local only)

### Possible Future Enhancements

- Metrics and analytics (cost tracking, success rates)
- Agent templates (predefined task types)
- Integration with issue trackers
- Automatic branch syncing/rebasing
- PR review automation
- Budget controls (API cost limits)
- Agent performance profiling

## Design Principles

1. **Transparency** - Users can observe all agent activity via tmux
2. **Simplicity** - Prefer simple solutions over complex abstractions
3. **Recovery** - System should gracefully handle crashes and orphaned resources
4. **Autonomy** - Agents operate independently but coordinate when needed
5. **Human-in-the-loop** - Users can intervene at any time via tmux or supervisor
6. **Safety** - Never weaken CI or bypass checks without human approval
7. **Filesystem-as-state** - Leverage git worktrees and tmux for resource management

## Implementation Priorities

### Phase 1: Core Infrastructure & Libraries ✅ COMPLETE
Build and thoroughly test the foundational components:
- [x] Daemon process with PID file management
- [x] Unix socket communication (request/response protocol)
- [x] State management (JSON persistence, sync, recovery)
- [x] tmux library (session/window creation, management, send-keys)
- [x] Worktree library (create, remove, cleanup orphaned)
- [x] Message filesystem operations (write, read, list, delete)
- [x] CLI framework (command parsing, daemon communication)
- [x] Error handling and logging infrastructure
- [x] Comprehensive unit and integration tests for all libraries with real tmux sessions and FS operations.

**Goal:** Rock-solid foundation with well-tested primitives.
**Status:** ✅ COMPLETE - All libraries implemented with comprehensive tests passing (67 tests).

### Phase 2: Running Daemon & Infrastructure
Implement the actual daemon process and wire up infrastructure WITHOUT Claude yet:

**Daemon Process:**
- [ ] Implement daemon main loop and goroutines
- [ ] Daemon start/stop commands (check PID, claim, daemonize)
- [ ] Daemon status command (read PID, check if running)
- [ ] Daemon logs command (tail daemon.log)
- [ ] State loading on startup and persistence on changes
- [ ] Graceful shutdown (cleanup, save state)

**Core Daemon Loops:**
- [ ] Health check loop (every 5 min) - verify tmux windows/PIDs exist
- [ ] Message router loop (event-driven) - watch messages dir, deliver via tmux send-keys
- [ ] Cleanup loop - remove orphaned tmux windows, worktrees, dead agents from state
- [ ] Periodic wake/nudge mechanism (configurable intervals)

**Repository Management:**
- [ ] `multiclaude init <github-url>` - clone repo, create tmux session
- [ ] Create supervisor tmux window (run plain shell for now, not Claude)
- [ ] Create merge-queue tmux window (run plain shell for now)
- [ ] Register repo and agents in state
- [ ] `multiclaude list` - show tracked repos

**Worker Management:**
- [ ] `multiclaude work -t <task>` - create worktree, tmux window (plain shell)
- [ ] `multiclaude work -t <task> --branch <branch>` - create from specific branch
- [ ] `multiclaude work list` - show active workers
- [ ] `multiclaude work rm <name>` - cleanup worktree and tmux window
- [ ] Detect uncommitted/unpushed changes on cleanup (warn user)

**Agent Communication:**
- [ ] Wire up `multiclaude agent send-message` to write message files
- [ ] Wire up `multiclaude agent list-messages` to read from messages dir
- [ ] Wire up `multiclaude agent ack-message` to mark messages acknowledged
- [ ] Daemon delivers messages to tmux windows via send-keys

**Testing & Validation:**
- [ ] `multiclaude attach <agent-name>` - attach to tmux window
- [ ] Test daemon can track multiple repos
- [ ] Test worker lifecycle (create, run commands, cleanup)
- [ ] Test message passing between windows
- [ ] Test state persistence and recovery after daemon restart
- [ ] Test orphaned resource cleanup

**Goal:** Fully functional daemon that tracks repos, manages tmux sessions/worktrees, and routes messages - all running plain shells in tmux windows. This proves the infrastructure works before adding Claude.

### Phase 3: Claude Code Integration
Replace plain shells with Claude Code instances:

**Claude in tmux:**
- [ ] Start Claude Code in tmux windows with session tracking
- [ ] Pass role-specific prompts via `--append-system-prompt-file`
- [ ] Monitor Claude process health (PID tracking)
- [ ] Handle Claude crashes and restarts (reuse session ID)
- [ ] Optional hooks configuration loading (.multiclaude/hooks.json)

**Role-Specific Prompts:**
- [ ] Default supervisor prompt (built-in)
- [ ] Default worker prompt (built-in)
- [ ] Default merge queue prompt (built-in)
- [ ] Load custom prompts from .multiclaude/ directory
- [ ] Pass task description to workers on startup

**Agent Intelligence:**
- [ ] Supervisor monitoring and coordination behavior
- [ ] Worker task execution and PR creation behavior
- [ ] Merge queue PR management and CI coordination
- [ ] Agent completion signaling (`multiclaude agent complete`)
- [ ] GitHub integration (gh CLI for PRs)

**Goal:** Autonomous Claude agents working collaboratively on repositories.

### Phase 4: Polish & Refinement
- [ ] Enhanced error messages and user guidance
- [ ] Cleanup and repair commands
- [ ] Better attach command with read-only mode
- [ ] Rich list commands with status information
- [ ] Documentation and usage examples
- [ ] Edge case handling and recovery scenarios
- [ ] Performance optimization
- [ ] GitHub auth verification on startup

**Goal:** Production-ready tool with excellent UX.
