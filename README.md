# multiclaude

A repo-centric orchestrator for managing multiple autonomous Claude Code instances working collaboratively on GitHub repositories.

## Project Status

**Phase 1: COMPLETE ✅**

All core infrastructure libraries have been implemented and thoroughly tested with **67 comprehensive test cases**, including real tmux and git integration tests.

See [SPEC.md](./SPEC.md) for the complete architecture specification and implementation roadmap.

## Usage

### multiclaude start

Starts the controller daemon.

### multiclaude init \<github url\> [path] [name]

* Sets up tracking for that github repo.
* Clones the repo into $HOME/.multiclaude
* Sets up a tmux session for agents
* Sets up a supervisor agent in that tmux session

### multiclaude work -t \<task\> [name or github url]

Detects the repo from the current directory if you're inside it, or uses the provided name/URL.
Creates a worker agent with its own worktree and tmux window in the repo session, with claude open.

### multiclaude work list

Lists all open work agents and allows the user to attach.

### multiclaude work rm \<name\>

Cleans up the tmux window and worktree. Warns if there is uncommitted/unmerged work not present on the remote.

## State

All state is stored in $HOME/.multiclaude

Structure:

### repos/<repo>
All git repos are stored here

### wts/\<repo\>/
Worktrees are stored here

### messages/\<repo\>/\<agent\>

Messages for an agent are stored here

## Agents

There are three kinds of agents:

* The repo supervisor agent
* Worker agents.
* The merge queue agent

All agents are controlled via the supervisor daemon.

Agents can pass messages to each other via the messages directory.
The supervisor agent should be notified via stdin anytime a message is sent by any of the workers via the controller daemon.
The controller daemon notifies workers when they have received a message.

The controller daemon periodically checks in on workers and nudges them along to complete their task.

All tasks are completed when a PR is sent to the remote repo.

The merge queue agent exists to merge items sent as PRs. It can create worker agents to address issues - workers die when they initially send the PR.

## Development Status

### Phase 1: Core Infrastructure - ✅ COMPLETE

All foundational libraries implemented with comprehensive testing:

- **`pkg/config`** - Path configuration and directory management
- **`internal/daemon`** - PID file management for daemon process control
- **`internal/state`** - JSON state persistence with atomic saves and recovery
- **`internal/tmux`** - Complete tmux session/window management (14 integration tests)
- **`internal/worktree`** - Git worktree operations (15 integration tests)
- **`internal/messages`** - Message filesystem operations for agent communication
- **`internal/socket`** - Unix socket client/server for CLI-daemon communication
- **`internal/logging`** - Structured logging infrastructure
- **`internal/cli`** - Command routing framework

### Test Coverage

```bash
$ go test ./...
# All 67 tests passing across 7 packages
# Includes real tmux session tests and real git worktree tests
# No mocking - true end-to-end integration testing
```

### Key Implementation Details

- ✅ Real tmux integration testing (creates actual tmux sessions)
- ✅ Real git worktree integration testing (creates actual git repos)
- ✅ Symlink-aware path resolution (macOS `/var/folders` compatible)
- ✅ Atomic state persistence and recovery
- ✅ Unix socket-based daemon communication
- ✅ Process lifecycle management with PID tracking
- ✅ Message queue system with lifecycle tracking

### Phase 2: Running Daemon & Infrastructure (NEXT)

Implement the actual daemon and wire up infrastructure WITHOUT Claude yet:

**Daemon Implementation:**
- [ ] Daemon main loop with goroutines
- [ ] Start/stop/status commands
- [ ] Health check loop (monitor tmux windows/PIDs)
- [ ] Message router loop (deliver messages via tmux send-keys)
- [ ] State persistence and recovery

**Repository & Worker Management:**
- [ ] `multiclaude init` - clone repo, create tmux session (plain shells)
- [ ] `multiclaude work` - create worktree + tmux window (plain shells)
- [ ] `multiclaude work list/rm` - manage workers
- [ ] `multiclaude attach` - attach to tmux windows
- [ ] Message passing between windows

**Goal:** Fully functional daemon tracking repos/worktrees/tmux - running plain shells before adding Claude.

### Phase 3: Claude Code Integration

Replace plain shells with Claude Code:
- [ ] Start Claude in tmux windows with session tracking
- [ ] Role-specific prompts (supervisor, worker, merge-queue)
- [ ] Agent intelligence and coordination
- [ ] GitHub integration (PR creation/management)

## Building and Testing

```bash
# Build
go build ./cmd/multiclaude

# Run all tests
go test ./...

# Run tests with verbose output
go test ./... -v

# Run specific package tests
go test ./internal/tmux -v
go test ./internal/worktree -v
```

## Prerequisites

- Go 1.21+
- tmux
- git
- GitHub CLI (`gh`)