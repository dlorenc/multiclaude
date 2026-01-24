# AGENTS.md

This file documents the agent system in multiclaude for contributors working on agent-related code or extending the system.

## The Brownian Ratchet Model

multiclaude operates on a physics-inspired principle: **multiple autonomous agents create apparent chaos, but CI serves as a ratchet that converts chaos into permanent forward progress**.

```
Agent Activity (Chaotic)          CI Gate (Ratchet)          Main Branch (Progress)
┌─────────────────────────┐       ┌─────────────┐           ┌─────────────────────┐
│ Worker A: auth feature  │──PR──▶│             │           │                     │
│ Worker B: auth feature  │──PR──▶│  CI Passes? │──merge──▶ │  ████████████████   │
│ Worker C: bugfix #42    │──PR──▶│             │           │  (irreversible)     │
└─────────────────────────┘       └─────────────┘           └─────────────────────┘
       ▲                                │
       │                                │ fail
       └────────────────────────────────┘
              (retry or spawn fixup)
```

**Key implications for agent design:**
- Redundant work is acceptable and expected
- Failed attempts are not wasted - they eliminate paths
- Any PR that passes CI represents valid forward progress
- The merge-queue agent is the critical "ratchet mechanism"

## Agent Types

### 1. Supervisor (`internal/prompts/supervisor.md`)

**Role**: Orchestration and coordination
**Worktree**: Main repository (no isolated branch)
**Lifecycle**: Persistent (runs as long as repo is tracked)

The supervisor monitors all other agents and nudges them toward progress. It:
- Receives automatic notifications when workers complete
- Sends guidance to stuck agents via inter-agent messaging
- Reports status when humans ask "what's everyone up to?"
- Never directly merges or modifies PRs (that's merge-queue's job)

**Key constraint**: The supervisor coordinates but doesn't execute. It communicates through `multiclaude message send` rather than taking direct action on PRs.

### 2. Merge-Queue (`internal/templates/agent-templates/merge-queue.md`)

**Role**: The ratchet mechanism - converts passing PRs into permanent progress
**Worktree**: Main repository
**Lifecycle**: Persistent

This is the most complex agent with multiple responsibilities:

| Responsibility | Commands Used |
|----------------|---------------|
| Monitor PRs | `gh pr list --label multiclaude` |
| Check CI | `gh run list --branch main`, `gh pr checks <n>` |
| Verify reviews | `gh pr view <n> --json reviews,reviewRequests` |
| Merge PRs | `gh pr merge <n> --squash` |
| Spawn fix workers | `multiclaude worker create "Fix CI for PR #N"` |
| Handle emergencies | Enter "emergency fix mode" when main is broken |

**Critical behaviors:**
- Never merges PRs with unaddressed review feedback
- Enters emergency mode and halts all merges when main branch CI fails
- Tracks PRs needing human input with `needs-human-input` label
- Can close unsalvageable PRs but must preserve learnings in issues

### 3. Worker (`internal/templates/agent-templates/worker.md`)

**Role**: Execute specific tasks and create PRs
**Worktree**: Isolated branch (`work/<worker-name>`)
**Lifecycle**: Ephemeral (cleaned up after completion)

Workers are the "muscle" of the system. They:
- Receive a task assignment at spawn time
- Work in isolation on their own branch
- Create PRs with detailed summaries (so other agents can continue if needed)
- Signal completion with `multiclaude agent complete`
- Can ask supervisor for help via messaging

**Completion flow:**
```
Worker creates PR → Worker runs `multiclaude agent complete`
                         ↓
              Daemon marks agent for cleanup
                         ↓
              Daemon notifies supervisor + merge-queue
                         ↓
              Health check cleans up worktree + window
```

### 4. Workspace (`internal/prompts/workspace.md`)

**Role**: User's persistent interactive session
**Worktree**: Own branch (`workspace/<name>`)
**Lifecycle**: Persistent (user's home base)

The workspace is unique - it's the only agent that:
- Receives direct human input
- Is NOT part of the automated nudge/wake cycle
- Can spawn workers on behalf of the user
- Persists conversation history across sessions

### 5. Review (`internal/templates/agent-templates/reviewer.md`)

**Role**: Code review and quality gate
**Worktree**: PR branch (ephemeral)
**Lifecycle**: Ephemeral (spawned by merge-queue)

Review agents are spawned by merge-queue to evaluate PRs before merge. They:
- Post blocking (`[BLOCKING]`) or non-blocking suggestions as PR comments
- Report summary to merge-queue for merge decision
- Default to non-blocking suggestions unless security/correctness issues

## Agent Communication

Agents communicate via filesystem-based messaging in `~/.multiclaude/messages/<repo>/<agent>/`.

### Message Lifecycle

```
pending → delivered → read → acked
```

| Status | Meaning |
|--------|---------|
| `pending` | Written to disk, not yet sent to agent |
| `delivered` | Sent to agent's tmux window |
| `read` | Agent has seen it (implicit) |
| `acked` | Agent explicitly acknowledged |

### Message Commands

```bash
# From any agent:
multiclaude message send <target> "<message>"
multiclaude message send --all "<broadcast>"
multiclaude message list
multiclaude message ack <id>
multiclaude message read <id>
```

Note: The old `agent send-message`, `agent list-messages`, `agent read-message`, and `agent ack-message` commands are still available as aliases for backward compatibility.

### Implementation Details

Messages are JSON files in `~/.multiclaude/messages/<repo>/<agent>/<msg-id>.json`:

```json
{
  "id": "msg-abc123",
  "from": "worker-clever-fox",
  "to": "supervisor",
  "timestamp": "2024-01-15T10:30:00Z",
  "body": "I need clarification on the auth requirements",
  "status": "pending"
}
```

The daemon routes messages every 2 minutes via `SendKeysLiteralWithEnter()` - this atomically sends text + Enter to avoid race conditions (see `pkg/tmux/client.go:264`).

## Agent Slash Commands

Each agent has access to multiclaude-specific slash commands via `CLAUDE_CONFIG_DIR`. These are automatically set up when agents spawn.

### Available Commands

| Command | Description |
|---------|-------------|
| `/refresh` | Sync worktree with main branch (fetch, rebase) |
| `/status` | Show system status, git status, and pending messages |
| `/workers` | List active workers for the repository |
| `/messages` | Check and manage inter-agent messages |

### Implementation

Slash commands are embedded in `internal/prompts/commands/` and deployed per-agent:

```
~/.multiclaude/claude-config/<repo>/<agent>/
└── commands/
    ├── refresh.md
    ├── status.md
    ├── workers.md
    └── messages.md
```

The daemon sets `CLAUDE_CONFIG_DIR` when starting Claude, which tells Claude Code where to find the custom commands.

### Adding New Commands

1. Create `internal/prompts/commands/<name>.md` with instructions
2. Add to `AvailableCommands` in `commands.go`
3. Rebuild: `go build ./cmd/multiclaude`

## Agent Prompts System

### Embedded Prompts

Default prompts are embedded at compile time via `//go:embed`:

```go
// internal/prompts/prompts.go
//go:embed supervisor.md
var defaultSupervisorPrompt string
```

### Custom Prompts (Configurable Agents System)

Repositories can customize agent behavior by creating markdown files in `.multiclaude/agents/`:

| Agent Type | Definition File |
|------------|-----------------|
| worker | `.multiclaude/agents/worker.md` |
| merge-queue | `.multiclaude/agents/merge-queue.md` |
| review | `.multiclaude/agents/review.md` |

**Precedence order:**
1. `<repo>/.multiclaude/agents/<agent>.md` (checked into repo, highest priority)
2. `~/.multiclaude/repos/<repo>/agents/<agent>.md` (local overrides)
3. Built-in templates (fallback)

Note: Supervisor and workspace agents use embedded prompts only and cannot be customized via this system.

**Deprecated:** The old system using `SUPERVISOR.md`, `WORKER.md`, `REVIEWER.md`, etc. directly in `.multiclaude/` is deprecated. Migrate your custom prompts to the new `.multiclaude/agents/` directory structure.

### Managing Agent Definitions via CLI

```bash
# List agent definitions for the current repo
multiclaude agents list

# Reset definitions to built-in defaults
multiclaude agents reset

# Spawn a custom agent from a prompt file
multiclaude agents spawn --name my-agent --class worker --prompt-file ./custom.md
```

### Example: Customizing Worker Behavior

To customize how workers operate for your project:

1. First, ensure the default templates are copied to your local definitions:
   ```bash
   multiclaude agents reset
   ```

2. Edit the worker definition:
   ```bash
   $EDITOR ~/.multiclaude/repos/my-repo/agents/worker.md
   ```

3. Add project-specific instructions at the end:
   ```markdown
   ## Project-Specific Guidelines

   ### Commit Conventions
   - Use conventional commits: feat:, fix:, docs:, refactor:, test:
   - Reference issue numbers in commit messages

   ### Code Style
   - Follow the patterns in existing code
   - Run `make lint` before creating PRs
   - All new public functions need docstrings

   ### Testing Requirements
   - Add tests for all new functionality
   - Ensure `make test` passes before marking complete
   ```

4. To share with your team, move the customization to the repo:
   ```bash
   mkdir -p .multiclaude/agents
   cp ~/.multiclaude/repos/my-repo/agents/worker.md .multiclaude/agents/
   git add .multiclaude/agents/worker.md
   git commit -m "docs: Add worker agent conventions"
   ```

### Prompt Assembly

```
Final Prompt = Default Prompt + CLI Documentation + Custom Prompt
```

CLI docs are auto-generated via `go generate ./pkg/config`.

## Agent Lifecycle Management

### Spawn Flow (Worker Example)

```
CLI: multiclaude worker create "task description"
         ↓
1. Generate unique name (adjective-animal pattern)
2. Create git worktree at ~/.multiclaude/wts/<repo>/<name>
3. Create tmux window in session mc-<repo>
4. Write prompt file with task embedded
5. Start Claude with --append-system-prompt-file
6. Register agent in state.json
7. Start output capture via pipe-pane
```

### Cleanup Flow

```
Agent runs: multiclaude agent complete
         ↓
1. Agent marked with ReadyForCleanup=true in state
2. Daemon notifies supervisor + merge-queue
3. Health check (every 2 min) finds marked agents
4. Kill tmux window
5. Remove from state.json
6. Delete worktree
7. Clean up messages directory
```

### Health Check Cycle

The daemon runs health checks every 2 minutes with **self-healing behavior**:

1. Check if tmux session exists for each repo
2. If tmux session is missing, **attempt restoration** before cleanup:
   - Recreate the tmux session
   - Restart supervisor, merge-queue, and any tracked agents
   - Only mark agents for cleanup if restoration fails
3. For each agent, verify tmux window exists
4. Clean up any agents with `ReadyForCleanup=true`
5. Prune orphaned worktrees (disk but not in git)
6. Prune orphaned message directories

This self-healing makes the daemon resilient to tmux server restarts, manual session kills, or other unexpected session losses.

## State Management

All agent state lives in `~/.multiclaude/state.json`:

```json
{
  "repos": {
    "my-repo": {
      "github_url": "https://github.com/owner/repo",
      "tmux_session": "mc-my-repo",
      "agents": {
        "supervisor": {
          "type": "supervisor",
          "worktree_path": "/path/to/repo",
          "tmux_window": "supervisor",
          "session_id": "uuid-v4",
          "pid": 12345,
          "created_at": "2024-01-15T10:00:00Z"
        },
        "clever-fox": {
          "type": "worker",
          "worktree_path": "~/.multiclaude/wts/my-repo/clever-fox",
          "task": "Implement auth feature",
          "ready_for_cleanup": false
        }
      }
    }
  }
}
```

State updates use atomic write pattern: write to `.tmp`, then `rename()`.

## Wake/Nudge System

The daemon periodically "nudges" agents to keep them active:

| Agent Type | Nudge Message |
|------------|---------------|
| supervisor | "Status check: Review worker progress and check merge queue." |
| merge-queue | "Status check: Review open PRs and check CI status." |
| worker | "Status check: Update on your progress?" |
| review | "Status check: Update on your review progress?" |
| workspace | **Not nudged** (user-driven only) |

Nudges are sent every 2 minutes, but agents are skipped if nudged within the last 2 minutes.

## Testing Agents

### Unit Tests

Agent-related tests are in:
- `internal/messages/messages_test.go` - Message system
- `internal/state/state_test.go` - State management
- `internal/prompts/prompts_test.go` - Prompt loading

### Integration Tests

E2E tests require tmux and use `MULTICLAUDE_TEST_MODE=1` to skip actual Claude startup:

```bash
# Run integration tests
go test ./test/...

# Test specific recovery scenarios
go test ./test/ -run TestDaemonCrashRecovery
```

### Recovery Tests

`test/recovery_test.go` covers:
- Corrupted state file recovery
- Orphaned tmux session cleanup
- Orphaned worktree cleanup
- Stale socket cleanup
- Orphaned message directory cleanup
- Daemon crash recovery
- Concurrent state access

## Adding a New Agent Type

1. **Define the type** in `internal/state/state.go`:
   ```go
   const AgentTypeMyAgent AgentType = "my-agent"
   ```

2. **Create the prompt template** at `internal/templates/agent-templates/my-agent.md`
   - Note: Only supervisor and workspace prompts are embedded directly in `internal/prompts/`
   - Other agent types (worker, merge-queue, review) use templates that can be customized

3. **Add the template** to `internal/templates/templates.go` for embedding

4. **Add prompt loading** in `GetDefaultPrompt()` if needed (for embedded prompts only)

5. **Add wake message** in `daemon.go:wakeAgents()` if needed

6. **Add CLI commands** if the agent needs special handling

7. **Write tests** for the new agent's behavior
