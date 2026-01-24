# Multiclaude Directory Structure

This document describes the directory structure used by multiclaude in `~/.multiclaude/`.
It is intended to help with debugging and understanding how multiclaude organizes its data.

> **Note**: This file is auto-generated from code constants in `pkg/config/doc.go`.
> Do not edit manually. Run `go generate ./pkg/config/...` to regenerate.

## Directory Layout

```
~/.multiclaude/
├── daemon.pid          # Daemon process ID
├── daemon.sock         # Unix socket for CLI communication
├── daemon.log          # Daemon activity log
├── state.json          # Persistent daemon state
│
├── repos/              # Cloned repositories
│   └── <repo-name>/    # Git clone of tracked repo
│
├── wts/                # Git worktrees
│   └── <repo-name>/
│       ├── supervisor/     # Supervisor's worktree
│       ├── merge-queue/    # Merge queue's worktree
│       └── <worker-name>/  # Worker worktrees
│
├── messages/           # Inter-agent messages
│   └── <repo-name>/
│       └── <agent-name>/
│           └── msg-<uuid>.json
│
└── prompts/            # Generated agent prompts
    └── <agent-name>.md
```

## Path Descriptions

### 📄 `daemon.pid`

**Type**: file

Contains the process ID of the running multiclaude daemon

**Notes**: Text file with a single integer. Deleted on clean daemon shutdown.

### 📄 `daemon.sock`

**Type**: file

Unix domain socket for CLI-to-daemon communication

**Notes**: Created with mode 0600 for security. The CLI connects here to send commands.

### 📄 `daemon.log`

**Type**: file

Append-only log of daemon activity

**Notes**: Useful for debugging daemon issues. Check this when agents behave unexpectedly.

### 📄 `state.json`

**Type**: file

Central state file containing all tracked repositories and agents

**Notes**: Written atomically via temp file + rename. See StateDoc() for format details.

### 📁 `repos/`

**Type**: directory

Contains cloned git repositories (bare or working)

**Notes**: Each repository is stored in repos/<repo-name>/

### 📁 `repos/<repo-name>/`

**Type**: directory

A cloned git repository

**Notes**: Full git clone of the tracked repository.

### 📁 `wts/`

**Type**: directory

Git worktrees for isolated agent working directories

**Notes**: Each agent gets its own worktree to work independently.

### 📁 `wts/<repo-name>/`

**Type**: directory

Worktrees directory for a specific repository

**Notes**: Contains subdirectories for each agent working on this repo.

### 📁 `wts/<repo-name>/<agent-name>/`

**Type**: directory

An agent's isolated git worktree

**Notes**: Agent types: supervisor, merge-queue, or worker names like happy-platypus.

### 📁 `messages/`

**Type**: directory

Inter-agent message files for coordination

**Notes**: Agents communicate via JSON message files in this directory.

### 📁 `messages/<repo-name>/`

**Type**: directory

Messages directory for a specific repository

**Notes**: Contains subdirectories for each agent that can receive messages.

### 📁 `messages/<repo-name>/<agent-name>/`

**Type**: directory

Inbox directory for a specific agent

**Notes**: Contains msg-<uuid>.json files addressed to this agent.

### 📁 `prompts/`

**Type**: directory

Generated prompt files for agents

**Notes**: Created on-demand. Contains <agent-name>.md prompt files.

## state.json Format

The `state.json` file contains the daemon's persistent state. It is written atomically
(write to temp file, then rename) to prevent corruption.

### Schema

```json
{
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
```

### Field Reference

| Field | Type | Description |
|-------|------|-------------|
| `repos` | `map[string]*Repository` | Map of repository name to repository state |
| `repos.<name>.github_url` | `string` | GitHub URL of the repository |
| `repos.<name>.tmux_session` | `string` | Name of the tmux session for this repo |
| `repos.<name>.agents` | `map[string]Agent` | Map of agent name to agent state |
| `repos.<name>.agents.<name>.type` | `string` | Agent type: supervisor, worker, merge-queue, or workspace |
| `repos.<name>.agents.<name>.worktree_path` | `string` | Absolute path to the agent's git worktree |
| `repos.<name>.agents.<name>.tmux_window` | `string` | Tmux window name for this agent |
| `repos.<name>.agents.<name>.session_id` | `string` | UUID for Claude session context |
| `repos.<name>.agents.<name>.pid` | `int` | Process ID of the Claude process |
| `repos.<name>.agents.<name>.task` | `string` | Task description (workers only, omitempty) |
| `repos.<name>.agents.<name>.created_at` | `time.Time` | When the agent was created |
| `repos.<name>.agents.<name>.last_nudge` | `time.Time` | Last time agent was nudged (omitempty) |
| `repos.<name>.agents.<name>.ready_for_cleanup` | `bool` | Whether worker is ready to be cleaned up (workers only, omitempty) |

## Message File Format

Message files are stored in `messages/<repo>/<agent>/msg-<uuid>.json`.
They are used for inter-agent communication.

### Schema

```json
{
  "id": "msg-abc123def456",
  "from": "supervisor",
  "to": "happy-platypus",
  "timestamp": "2025-01-01T00:00:00Z",
  "body": "Please review PR #42",
  "status": "pending",
  "acked_at": null
}
```

### Field Reference

| Field | Type | Description |
|-------|------|-------------|
| `id` | `string` | Message ID in format msg-<uuid> |
| `from` | `string` | Sender agent name |
| `to` | `string` | Recipient agent name |
| `timestamp` | `time.Time` | When the message was sent |
| `body` | `string` | Message content (markdown text) |
| `status` | `string` | Message status: pending, delivered, read, or acked |
| `acked_at` | `time.Time` | When the message was acknowledged (omitempty) |

## Debugging Tips

### Check daemon status

```bash
# Is the daemon running?
cat ~/.multiclaude/daemon.pid && ps -p $(cat ~/.multiclaude/daemon.pid)

# View daemon logs
tail -f ~/.multiclaude/daemon.log
```

### Inspect state

```bash
# Pretty-print current state
cat ~/.multiclaude/state.json | jq .

# List all agents for a repo
cat ~/.multiclaude/state.json | jq '.repos["my-repo"].agents | keys'
```

### Check agent worktrees

```bash
# List all worktrees for a repo
ls ~/.multiclaude/wts/my-repo/

# Check git status in an agent's worktree
git -C ~/.multiclaude/wts/my-repo/supervisor status
```

### View messages

```bash
# List all messages for an agent
ls ~/.multiclaude/messages/my-repo/supervisor/

# Read a specific message
cat ~/.multiclaude/messages/my-repo/supervisor/msg-*.json | jq .
```

### Clean up stale state

```bash
# Use the built-in repair command
multiclaude repair

# Or manually clean up orphaned resources
multiclaude cleanup
```
