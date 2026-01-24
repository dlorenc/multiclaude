# Architecture Deep Dive

This document provides detailed technical documentation of multiclaude's architecture, data flows, and implementation details.

## System Overview

multiclaude is a daemon-based orchestration system built in Go. It manages multiple Claude Code instances running in tmux windows, each with isolated git worktrees.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              User's Machine                                  │
│                                                                              │
│  ┌──────────────┐     Unix Socket      ┌──────────────────────────────────┐ │
│  │  CLI Client  │ ◄──────────────────► │         Daemon Process           │ │
│  │              │                       │                                  │ │
│  │ multiclaude  │                       │  ┌────────────────────────────┐ │ │
│  │   work       │                       │  │      Goroutine Pool        │ │ │
│  │   init       │                       │  │                            │ │ │
│  │   list       │                       │  │  • Server Loop             │ │ │
│  │   ...        │                       │  │  • Health Check Loop       │ │ │
│  └──────────────┘                       │  │  • Message Router Loop     │ │ │
│                                         │  │  • Wake/Nudge Loop         │ │ │
│                                         │  └────────────────────────────┘ │ │
│                                         │                                  │ │
│                                         │  ┌────────────────────────────┐ │ │
│                                         │  │     State Manager          │ │ │
│                                         │  │  (thread-safe, persisted)  │ │ │
│                                         │  └────────────────────────────┘ │ │
│                                         └──────────────────────────────────┘ │
│                                                         │                    │
│                                                         ▼                    │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                         tmux Sessions                                  │  │
│  │                                                                        │  │
│  │  mc-repo-a                              mc-repo-b                      │  │
│  │  ┌──────────┬──────────┬──────────┐    ┌──────────┬──────────┐        │  │
│  │  │supervisor│merge-q   │worker-1  │    │supervisor│worker-1  │        │  │
│  │  │(Claude)  │(Claude)  │(Claude)  │    │(Claude)  │(Claude)  │        │  │
│  │  └────┬─────┴────┬─────┴────┬─────┘    └────┬─────┴────┬─────┘        │  │
│  │       │          │          │               │          │               │  │
│  └───────┼──────────┼──────────┼───────────────┼──────────┼───────────────┘  │
│          │          │          │               │          │                  │
│          ▼          ▼          ▼               ▼          ▼                  │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                        Git Worktrees                                   │  │
│  │  ~/.multiclaude/wts/repo-a/          ~/.multiclaude/wts/repo-b/       │  │
│  │    supervisor/  merge-queue/           supervisor/  worker-1/         │  │
│  │    worker-1/                                                          │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Component Details

### Daemon (`internal/daemon/daemon.go`)

The daemon is the central coordinator. It runs as a background process and manages all state.

**Structure:**
```go
type Daemon struct {
    paths   *config.Paths      // File system paths
    state   *state.State       // Thread-safe state manager
    tmux    *tmux.Client       // tmux operations
    logger  *logging.Logger    // Structured logging
    server  *socket.Server     // Unix socket server
    pidFile *PIDFile           // Process ID management

    ctx    context.Context     // For graceful shutdown
    cancel context.CancelFunc
    wg     sync.WaitGroup      // Track goroutines
}
```

**Lifecycle:**
1. `New()` - Initialize daemon, load state from disk
2. `Start()` - Claim PID file, start socket server, launch goroutines
3. `Wait()` - Block until shutdown
4. `Stop()` - Cancel context, wait for goroutines, save state, cleanup

**Goroutines:**

| Loop | Interval | Purpose |
|------|----------|---------|
| `serverLoop` | Continuous | Handle incoming socket requests |
| `healthCheckLoop` | 2 min | Verify agents are alive, cleanup dead ones |
| `messageRouterLoop` | 2 min | Deliver pending messages to agents |
| `wakeLoop` | 2 min | Nudge idle agents with status checks |

### State Management (`internal/state/state.go`)

State is stored in memory with automatic persistence to disk.

**Data Model:**
```go
type State struct {
    Repos map[string]*Repository  // repo name -> repository
    mu    sync.RWMutex            // Thread safety
    path  string                  // Path to state.json
}

type Repository struct {
    GithubURL   string
    TmuxSession string
    Agents      map[string]Agent
}

type Agent struct {
    Type            AgentType  // supervisor, worker, merge-queue
    WorktreePath    string
    TmuxWindow      string
    SessionID       string     // UUID for Claude --session-id
    PID             int        // Claude process ID
    Task            string     // Worker task description
    CreatedAt       time.Time
    LastNudge       time.Time
    ReadyForCleanup bool
}
```

**Persistence:**
- Atomic writes using temp file + rename
- Auto-save after every state change
- Load on daemon startup with recovery

**Thread Safety:**
- `sync.RWMutex` protects all operations
- Read operations use `RLock()`
- Write operations use `Lock()` + auto-save
- `GetAllRepos()` returns deep copy for iteration

### Socket Communication (`internal/socket/socket.go`)

CLI and daemon communicate via Unix domain socket.

**Protocol:**
```
Request:  JSON { command: string, args: map[string]any }
Response: JSON { success: bool, data: any, error: string }
```

**Commands:**
| Command | Args | Description |
|---------|------|-------------|
| `ping` | - | Health check |
| `status` | - | Get daemon status |
| `stop` | - | Stop daemon |
| `list_repos` | - | List repositories |
| `add_repo` | name, github_url, tmux_session | Register repo |
| `add_agent` | repo, agent, type, worktree_path, ... | Register agent |
| `remove_agent` | repo, agent | Unregister agent |
| `list_agents` | repo | List agents in repo |
| `complete_agent` | repo, agent | Mark ready for cleanup |
| `trigger_cleanup` | - | Force cleanup run |
| `repair_state` | - | Fix state inconsistencies |

### tmux Integration (`internal/tmux/tmux.go`)

All tmux operations are encapsulated in a client wrapper.

**Key Operations:**
```go
// Session management
CreateSession(name, startDir string) error
KillSession(name string) error
HasSession(name string) (bool, error)
ListSessions() ([]string, error)

// Window management
CreateWindow(session, name, startDir string) error
KillWindow(session, window string) error
HasWindow(session, window string) (bool, error)

// Agent interaction
SendKeysLiteral(session, window, text string) error  // Paste mode
SendEnter(session, window string) error               // Submit
GetPanePID(session, window string) (int, error)       // Process tracking
```

**Message Delivery:**
Messages are sent using tmux's literal mode to avoid shell interpretation issues:
```bash
tmux set-buffer -b mc_msg "message text" \; paste-buffer -b mc_msg -t session:window \; delete-buffer -b mc_msg
```

### Git Worktree Management (`internal/worktree/worktree.go`)

Each agent gets an isolated working directory via git worktrees.

**Operations:**
```go
// Create worktree with new branch
CreateNewBranch(path, branch, startPoint string) error

// Remove worktree
Remove(path string, force bool) error

// Cleanup orphaned directories
CleanupOrphaned(wtRootDir string, manager *Manager) ([]string, error)

// Git state checks
HasUncommittedChanges(path string) (bool, error)
HasUnpushedCommits(path string) (bool, error)
GetCurrentBranch(path string) (string, error)
```

**Worktree Layout:**
```
~/.multiclaude/wts/<repo>/
├── supervisor/        # Shared with main clone initially
├── merge-queue/       # Shared with main clone initially
├── happy-platypus/    # Worker worktree
│   └── (full repo)
└── clever-fox/        # Another worker worktree
    └── (full repo)
```

### Message System (`internal/messages/messages.go`)

Agents communicate via JSON files on the filesystem.

**Message Structure:**
```go
type Message struct {
    ID        string    `json:"id"`        // msg-<uuid>
    From      string    `json:"from"`      // sender agent name
    To        string    `json:"to"`        // recipient agent name
    Timestamp time.Time `json:"timestamp"`
    Body      string    `json:"body"`      // markdown text
    Status    Status    `json:"status"`    // pending/delivered/read/acked
    AckedAt   *time.Time `json:"acked_at"`
}
```

**Status Flow:**
```
pending → delivered → read → acked → (deleted)
   │          │         │       │
   │          │         │       └── Agent acknowledged
   │          │         └── Agent read via CLI
   │          └── Daemon sent via tmux
   └── Written to filesystem
```

**File Layout:**
```
~/.multiclaude/messages/<repo>/<agent>/
├── msg-abc123.json
├── msg-def456.json
└── ...
```

### Prompt System (`internal/prompts/prompts.go`)

Role-specific prompts are embedded in the binary and can be extended by repositories.

**Embedded Prompts:**
```go
//go:embed supervisor.md
var supervisorPrompt string

//go:embed worker.md
var workerPrompt string

//go:embed merge-queue.md
var mergeQueuePrompt string
```

**Prompt Assembly:**
1. Load default embedded prompt for role
2. Append custom prompt from `.multiclaude/<ROLE>.md` if exists
3. Append auto-generated CLI documentation
4. Write to `~/.multiclaude/prompts/<agent>.md`
5. Pass to Claude via `--append-system-prompt-file`

### CLI (`internal/cli/cli.go`)

The CLI handles user commands and communicates with the daemon.

**Command Tree:**
```
multiclaude
├── start                    # Start daemon (alias for daemon start)
├── stop-all [--clean]       # Stop everything
├── repo
│   ├── init <url>           # Initialize repo
│   ├── list                 # List repos
│   ├── rm <name>            # Remove repo
│   ├── use <name>           # Set default repo
│   ├── current              # Show default repo
│   ├── unset                # Clear default repo
│   └── history              # Show task history
├── worker
│   ├── create <task>        # Create worker
│   ├── list                 # List workers
│   └── rm <name>            # Remove worker
├── workspace
│   ├── add <name>           # Add workspace
│   ├── list                 # List workspaces
│   ├── connect <name>       # Connect to workspace
│   └── rm <name>            # Remove workspace
├── agent
│   ├── attach <name>        # Attach to tmux window
│   ├── complete             # Signal completion
│   ├── restart <name>       # Restart crashed agent
│   ├── send-message         # (alias for message send)
│   ├── list-messages        # (alias for message list)
│   ├── read-message         # (alias for message read)
│   └── ack-message          # (alias for message ack)
├── message
│   ├── send <to> <msg>      # Send message
│   ├── list                 # List messages
│   ├── read <id>            # Read message
│   └── ack <id>             # Acknowledge message
├── agents
│   ├── list                 # List agent definitions
│   ├── spawn                # Spawn from prompt file
│   └── reset                # Reset to defaults
├── cleanup [--dry-run]      # Clean orphaned resources
├── repair                   # Fix state
├── review <pr-url>          # Spawn review agent
├── config                   # View/modify repo config
├── logs                     # View agent logs
├── bug                      # Generate diagnostic report
├── version                  # Show version
└── daemon
    ├── start
    ├── stop
    ├── status
    └── logs [-f]
```

## Data Flows

### Repository Initialization

```
User: multiclaude repo init https://github.com/org/repo

CLI                          Daemon                       System
 │                              │                            │
 │──── ping ───────────────────►│                            │
 │◄─── pong ────────────────────│                            │
 │                              │                            │
 │ git clone ─────────────────────────────────────────────► │
 │                              │                            │
 │ tmux new-session ──────────────────────────────────────► │
 │ tmux new-window ───────────────────────────────────────► │
 │                              │                            │
 │ write prompt files ────────────────────────────────────► │
 │                              │                            │
 │ tmux send-keys (start Claude) ─────────────────────────► │
 │                              │                            │
 │──── add_repo ───────────────►│                            │
 │◄─── success ─────────────────│                            │
 │                              │──── save state ──────────► │
 │──── add_agent (supervisor) ─►│                            │
 │◄─── success ─────────────────│                            │
 │                              │──── save state ──────────► │
 │──── add_agent (merge-queue) ►│                            │
 │◄─── success ─────────────────│                            │
 │                              │──── save state ──────────► │
```

### Worker Creation

```
User: multiclaude worker create "Add unit tests"

CLI                          Daemon                       System
 │                              │                            │
 │──── list_agents ────────────►│                            │
 │◄─── agent list ──────────────│                            │
 │                              │                            │
 │ git worktree add ──────────────────────────────────────► │
 │ tmux new-window ───────────────────────────────────────► │
 │ write prompt file ─────────────────────────────────────► │
 │ tmux send-keys (start Claude) ─────────────────────────► │
 │ tmux send-keys (task message) ─────────────────────────► │
 │                              │                            │
 │──── add_agent (worker) ─────►│                            │
 │◄─── success ─────────────────│                            │
 │                              │──── save state ──────────► │
```

### Message Delivery

```
Agent A: multiclaude message send agent-b "Hello"

CLI                          Daemon                       System
 │                              │                            │
 │ write message.json ────────────────────────────────────► │
 │                              │                            │
 ═══════════════════════════════════════════════════════════

[2 minutes later, message router loop runs]

                             Daemon                       tmux
                                │                            │
                                │──── scan messages dir ────►│
                                │◄─── pending messages ──────│
                                │                            │
                                │──── send-keys (literal) ──►│
                                │                            │ (paste to Agent B)
                                │──── update status ────────►│
                                │     (delivered)            │
```

### Worker Completion

```
Worker: multiclaude agent complete

CLI                          Daemon                       System
 │                              │                            │
 │──── complete_agent ─────────►│                            │
 │                              │──── mark ready_for_cleanup │
 │                              │                            │
 │                              │──── send completion msg ──►│
 │                              │     to supervisor          │
 │                              │                            │
 │◄─── success ─────────────────│                            │

[Health check loop runs]

                             Daemon                       System
                                │                            │
                                │──── kill tmux window ─────►│
                                │──── git worktree remove ──►│
                                │──── remove from state ────►│
                                │──── cleanup messages ─────►│
```

## Concurrency Model

The daemon uses Go's standard concurrency primitives:

**Goroutine Management:**
- `sync.WaitGroup` tracks all background goroutines
- `context.Context` enables graceful shutdown
- Each loop checks `ctx.Done()` before sleeping

**State Thread Safety:**
- `sync.RWMutex` in State struct
- Read operations use `RLock()` for parallel reads
- Write operations use `Lock()` for exclusive access
- `GetAllRepos()` returns deep copy to avoid iteration issues

**Shutdown Sequence:**
```go
func (d *Daemon) Stop() error {
    d.cancel()           // Signal all goroutines to stop
    d.wg.Wait()          // Wait for them to finish
    d.server.Stop()      // Close socket server
    d.state.Save()       // Persist final state
    d.pidFile.Remove()   // Cleanup PID file
}
```

## Error Handling

**Recovery Patterns:**
- State loaded from disk on startup
- Orphaned resources cleaned up by health check
- Dead agents detected via PID checks and tmux queries
- `repair` command fixes state/resource mismatches

**Failure Modes:**

| Failure | Detection | Recovery |
|---------|-----------|----------|
| Agent process dies | Health check PID check | Log warning, keep tmux window |
| tmux window killed | Health check window check | Remove agent from state |
| tmux session killed | Health check session check | Remove all agents for repo |
| Daemon crashes | N/A | Reload state on restart |
| Orphan worktree | Cleanup loop | Remove directory |
| Orphan message dir | Cleanup loop | Remove directory |

## Security Considerations

**Process Isolation:**
- Each agent runs in isolated git worktree
- No shared mutable state between agents
- Agents communicate only via filesystem messages

**Permission Model:**
- Claude runs with `--dangerously-skip-permissions`
- This is intentional for autonomous operation
- Agents are isolated to their worktree directories

**CI Safety:**
- Agents are instructed to never weaken CI
- Merge queue requires human approval for CI changes
- This is a soft constraint enforced via prompts

## File System Layout

```
~/.multiclaude/
├── daemon.pid              # Contains daemon PID
├── daemon.sock             # Unix socket (mode 0600)
├── daemon.log              # Append-only log file
├── state.json              # JSON state (atomically updated)
├── state.json.tmp          # Temp file during atomic write
│
├── prompts/                # Generated prompt files
│   ├── supervisor.md
│   ├── merge-queue.md
│   └── happy-platypus.md
│
├── repos/                  # Cloned repositories
│   └── my-repo/
│       └── (git repository)
│
├── wts/                    # Git worktrees
│   └── my-repo/
│       ├── supervisor/     # (may be symlink to repos/my-repo)
│       ├── merge-queue/
│       └── happy-platypus/
│
└── messages/               # Inter-agent messages
    └── my-repo/
        ├── supervisor/
        │   └── msg-abc.json
        ├── merge-queue/
        └── happy-platypus/
```

## Testing Strategy

**Unit Tests:**
- Each package has `*_test.go` files
- Mock-free where possible (real tmux, real git)
- `MULTICLAUDE_TEST_MODE=1` skips Claude startup

**Integration Tests:**
- `test/e2e_test.go` tests full workflows
- Creates real tmux sessions
- Creates real git repos and worktrees
- Verifies state persistence

**Test Commands:**
```bash
go test ./...                    # All tests
go test ./internal/daemon -v     # Daemon tests
go test ./internal/tmux -v       # tmux integration tests
go test ./test -v                # E2E tests
```
