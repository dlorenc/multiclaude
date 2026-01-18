# Crash Recovery Guide

This document describes what happens when various multiclaude components crash, what resources may become orphaned, and how to recover from each scenario.

## Overview of Components

Multiclaude consists of several components that can fail independently:

| Component | Description | Persistence |
|-----------|-------------|-------------|
| **Daemon** | Central coordinator process | `daemon.pid`, `state.json` |
| **Supervisor** | Claude agent managing workers | tmux window, no special state |
| **Merge-Queue** | Claude agent processing PRs | tmux window, no special state |
| **Workers** | Claude agents executing tasks | tmux windows, git worktrees, branches |
| **Workspace** | User's interactive Claude agent | tmux window, git worktree |
| **Tmux Session** | Container for all agents in a repo | tmux server state |
| **Git Worktrees** | Isolated working directories | filesystem + git metadata |

## Crash Scenarios and Recovery

### 1. Daemon Crash

**What happens:**
- The daemon process (`multiclaude daemon _run`) terminates unexpectedly
- All background loops stop: health checks, message routing, agent waking
- Socket communication becomes unavailable
- CLI commands that need daemon will fail

**What gets orphaned:**
- `daemon.pid` file remains with stale PID
- `daemon.sock` file may remain
- tmux sessions and windows continue running (agents keep working)
- State file (`state.json`) remains valid - last atomic write is preserved

**Automatic recovery:**
- On next `multiclaude start`, daemon detects stale PID file via signal 0 check
- Stale PID file is removed and new daemon takes over
- State is loaded from `state.json`
- First health check runs immediately to verify agents

**Manual recovery:**
```bash
# Check if daemon is actually dead
ps -p $(cat ~/.multiclaude/daemon.pid) 2>/dev/null

# Start daemon (handles stale PID automatically)
multiclaude start

# Verify recovery
multiclaude daemon status
```

**Impact:**
- Agents continue working but won't receive messages
- No periodic status nudges
- Dead agents won't be cleaned up until daemon restarts

---

### 2. Supervisor Crash

**What happens:**
- Claude process in supervisor tmux window exits
- Supervisor stops coordinating workers
- Workers continue independently but without guidance

**What gets orphaned:**
- tmux window remains (empty shell prompt)
- State still shows supervisor as active

**Automatic recovery:**
- Health check detects window exists but process may be dead
- Currently: Warning logged but no automatic restart (by design)
- Window is preserved so user can manually restart

**Manual recovery:**
```bash
# Check supervisor window
multiclaude attach supervisor

# Restart Claude manually in the window
claude --session-id <session-id> --dangerously-skip-permissions \
  --append-system-prompt-file ~/.multiclaude/prompts/supervisor.md

# Or force repair to reset state
multiclaude repair
```

**Impact:**
- New workers won't be supervised
- Existing workers continue but may complete without feedback
- Merge-queue continues independently

---

### 3. Merge-Queue Crash

**What happens:**
- Claude process in merge-queue tmux window exits
- PR processing stops
- PRs may sit in queue without review/merge

**What gets orphaned:**
- tmux window remains (empty shell prompt)
- State still shows merge-queue as active

**Automatic/Manual recovery:**
- Same as supervisor - window preserved for manual restart

**Impact:**
- PRs won't be automatically merged
- CI status won't be monitored
- Workers complete but their PRs wait

---

### 4. Worker Crash

**What happens:**
- Claude process in worker tmux window exits
- Work on task stops mid-progress
- Worker may have:
  - Uncommitted changes
  - Unpushed commits
  - Partial PR

**What gets orphaned:**
- tmux window (empty shell)
- git worktree with potential uncommitted work
- git branch
- Message directory for worker
- State entry for worker

**Automatic recovery:**
- Health check sees window exists (process may be dead)
- If `ReadyForCleanup` is not set, worker is preserved
- Changes are NOT automatically committed or pushed

**Manual recovery:**
```bash
# Check worker status
multiclaude attach <worker-name>

# Option 1: Continue the work manually
cd ~/.multiclaude/wts/<repo>/<worker-name>
git status  # Check for uncommitted work
# Continue or restart Claude

# Option 2: Save work and remove worker
cd ~/.multiclaude/wts/<repo>/<worker-name>
git stash  # or: git add . && git commit -m "WIP"
git push -u origin work/<worker-name>
multiclaude work rm <worker-name>

# Option 3: Force remove (lose uncommitted work)
multiclaude work rm <worker-name>
# Answer 'y' to warnings about uncommitted changes
```

**Impact:**
- Task incomplete
- Uncommitted work at risk
- Branch may have partial work

---

### 5. Workspace Crash

**What happens:**
- User's interactive Claude session exits
- Similar to worker but typically has more user interaction

**Recovery:**
- Manual restart in tmux window
- Workspace worktree and branch preserved

---

### 6. Tmux Session Crash

**What happens:**
- Entire tmux session (`mc-<repo>`) terminates
- All agents in that repo are affected
- Happens if: tmux kill-session, tmux server restart, system crash

**What gets orphaned:**
- All tmux windows gone
- State still shows all agents as active
- All worktrees remain on disk
- All branches remain in git
- All message directories remain

**Automatic recovery:**
- Health check detects missing session
- All agents for that repo marked for cleanup
- Worktrees removed (if workers)
- State updated

**Manual recovery:**
```bash
# Repair will handle session detection
multiclaude repair

# Or reinitialize if needed
multiclaude stop-all
multiclaude start
multiclaude init <github-url>  # Will fail if repo exists
```

**Impact:**
- All work in progress lost (if uncommitted)
- Must reinitialize repo to continue

---

### 7. Git Worktree Corruption

**What happens:**
- Worktree directory deleted manually
- Git metadata out of sync
- Agent can't operate on files

**What gets orphaned:**
- Git worktree metadata (in .git/worktrees)
- State references non-existent path

**Automatic recovery:**
- `cleanupOrphanedWorktrees()` detects directories not in git
- `git worktree prune` cleans up stale metadata

**Manual recovery:**
```bash
# Prune git worktree metadata
git -C ~/.multiclaude/repos/<repo> worktree prune

# Run cleanup
multiclaude cleanup

# Repair state
multiclaude repair
```

---

### 8. System Crash / Power Loss

**What happens:**
- All processes terminate immediately
- No graceful shutdown possible
- State file may be mid-write (but atomic rename protects this)

**What gets orphaned:**
- Stale `daemon.pid`
- Stale `daemon.sock`
- tmux sessions gone (tmux server died)
- All worktrees remain
- All branches remain
- State.json is valid (atomic write via rename)

**Recovery:**
```bash
# Start daemon (handles stale files)
multiclaude start

# Repair state (detects missing sessions)
multiclaude repair

# Check what remains
multiclaude list
multiclaude work list
```

---

## Recovery Commands

### `multiclaude repair`

**When to use:** After crashes, when state seems inconsistent with reality.

**What it does:**
1. Verifies each tmux session exists
2. Verifies each tmux window exists
3. Checks worktree paths exist (for workers)
4. Removes agents with missing resources
5. Cleans up orphaned worktree directories
6. Cleans up orphaned message directories

**Limitations:**
- Requires daemon to be running
- Does not restore lost work
- Does not restart crashed Claude processes

### `multiclaude cleanup`

**When to use:** To clean orphaned files without full state repair.

**What it does:**
- With daemon: Triggers health check and cleanup
- Without daemon: Local cleanup of orphaned worktrees

**Dry-run mode:**
```bash
multiclaude cleanup --dry-run
```

### `multiclaude stop-all`

**When to use:** To completely stop everything and optionally reset state.

**What it does:**
1. Kills all `mc-*` tmux sessions
2. Stops daemon
3. Optionally (`--clean`): Removes state files

**Preserves:**
- Repository clones
- Worktree directories
- Messages

---

## What Cannot Be Recovered

| Lost Resource | Cause | Prevention |
|---------------|-------|------------|
| Uncommitted changes | Worker crash before commit | Workers should commit early and often |
| Unpushed commits | Crash before push | Workers should push regularly |
| In-flight operations | Daemon crash during request | CLI will timeout; retry command |
| Message delivery | Crash during delivery | Message stays pending; delivered on restart |
| Partial PR | Worker crash mid-PR creation | Check GitHub for draft PRs |

---

## Diagnostic Commands

### Check Component Health

```bash
# Daemon status
multiclaude daemon status

# View daemon logs
multiclaude daemon logs
tail -f ~/.multiclaude/daemon.log

# List all tmux sessions
tmux list-sessions | grep mc-

# List windows in a session
tmux list-windows -t mc-<repo>

# Check state file
cat ~/.multiclaude/state.json | jq .
```

### Check for Orphaned Resources

```bash
# Orphaned worktrees (directories without git tracking)
ls ~/.multiclaude/wts/<repo>/
git -C ~/.multiclaude/repos/<repo> worktree list

# Orphaned message directories
ls ~/.multiclaude/messages/<repo>/
# Compare with agents in state.json

# Orphaned branches
git -C ~/.multiclaude/repos/<repo> branch | grep work/
```

### Check Process Status

```bash
# Is daemon running?
ps -p $(cat ~/.multiclaude/daemon.pid) 2>/dev/null

# Find all Claude processes
ps aux | grep claude

# Check agent process in tmux
tmux display-message -t mc-<repo>:<window> -p "#{pane_pid}"
ps -p <pid>
```

---

## Best Practices for Resilience

### For Workers

1. **Commit early and often** - Don't accumulate too many uncommitted changes
2. **Push to remote** - Unpushed commits are only local
3. **Signal completion** - Always run `multiclaude agent complete` when done

### For Operators

1. **Monitor daemon logs** - Watch for repeated errors
2. **Run periodic repair** - `multiclaude repair` is safe to run regularly
3. **Check orphaned resources** - Especially after system crashes

### System Configuration

1. **tmux resurrection** - Consider tmux-resurrect plugin for session persistence
2. **Process supervisor** - Use systemd/launchd to auto-restart daemon
3. **Log rotation** - Daemon log can grow large

---

## Architecture Decisions

### Why tmux windows are not auto-cleaned on crash

When a Claude process dies but the tmux window remains, we intentionally **do not** clean up the window because:

1. User may want to inspect the final state
2. User may want to manually restart Claude
3. The shell retains command history
4. Cleaning up could lose context about what happened

### Why we don't auto-restart Claude processes

1. Restarting could lose conversation context
2. Session state is not trivially restorable
3. Manual intervention allows user to decide next steps
4. Avoids potential restart loops

### Why state.json uses atomic writes

The state file is written atomically (write to temp file, then rename) because:

1. Prevents corruption from mid-write crashes
2. Ensures consistent reads even during writes
3. Rename is atomic on most filesystems

---

## Future Improvements

See GitHub issue #23 for tracking. Potential enhancements:

1. **State backup** - Periodic backups of state.json
2. **Process monitoring** - Detect dead Claude processes, not just missing windows
3. **Work-in-progress protection** - Auto-stash uncommitted changes before cleanup
4. **Graceful worker shutdown** - Allow workers to save state on SIGTERM
5. **Health status API** - Expose detailed health info via CLI
