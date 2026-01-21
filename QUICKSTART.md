# Quickstart

Get multiclaude running in 5 minutes.

## Prerequisites

- Go 1.21+
- tmux
- git
- GitHub CLI (`gh`) authenticated: `gh auth login`

## Install

```bash
go install github.com/dlorenc/multiclaude/cmd/multiclaude@latest
```

## Start

```bash
# Start the daemon (runs in background)
multiclaude start

# Initialize a repository
multiclaude init https://github.com/your/repo

# Spawn a worker to do a task
multiclaude work "Add unit tests for the auth module"
```

## Watch

```bash
# Attach to the tmux session
tmux attach -t mc-repo
```

Use tmux keys to navigate:
- `Ctrl-b n` - Next window
- `Ctrl-b p` - Previous window
- `Ctrl-b d` - Detach (agents keep running)

## What's Running

After `init`, you'll have three agents:

| Agent | Purpose |
|-------|---------|
| supervisor | Coordinates all agents |
| merge-queue | Merges PRs when CI passes |
| (your workers) | Execute specific tasks |

## Common Tasks

```bash
# Spawn more workers
multiclaude work "Fix the login bug"
multiclaude work "Add logging to the API"

# List active workers
multiclaude work list

# Watch a specific agent
multiclaude attach supervisor --read-only

# Check daemon status
multiclaude daemon status

# View daemon logs
multiclaude daemon logs -f

# Stop everything
multiclaude stop-all
```

## What Happens Next

1. Workers create PRs when their task is done
2. Merge-queue monitors PRs and merges when CI passes
3. Workers clean up automatically after completion

For the full guide, see [README.md](README.md).
