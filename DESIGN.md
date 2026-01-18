# Design Decisions

This document explains the key design decisions in multiclaude and the rationale behind them.

## Philosophy

multiclaude follows these guiding principles:

1. **Simplicity over sophistication** - Use standard tools (tmux, git, filesystem) instead of building custom infrastructure
2. **Observable by default** - Humans can see everything agents do at any time
3. **Recoverable** - System state can survive crashes and be repaired
4. **Minimal abstraction** - Keep concepts close to their implementations
5. **Forward progress** - Any movement toward the goal is good

## Key Decisions

### Why tmux?

**Decision:** Use tmux for all agent session management.

**Alternatives Considered:**
- Custom terminal emulator
- Background processes with log files
- Docker containers
- Screen

**Rationale:**
- **Observability**: Humans can attach to any agent at any time with a single command
- **Familiarity**: Most developers already know tmux
- **Robustness**: tmux sessions survive SSH disconnects and terminal crashes
- **Interactivity**: Users can type directly to agents when needed
- **No new concepts**: tmux windows are well-understood abstractions

**Trade-offs:**
- Requires tmux to be installed
- Limited to local machine (no remote daemon)
- Session names must be unique

### Why git worktrees?

**Decision:** Each agent gets an isolated git worktree instead of working in the same directory.

**Alternatives Considered:**
- Single directory with branch switching
- Full repository clones per agent
- Shared working directory with stashing

**Rationale:**
- **True isolation**: Agents can have different files checked out simultaneously
- **No conflicts**: Branch switching in one agent doesn't affect others
- **Standard git**: No special tooling needed, just `git worktree`
- **Clean cleanup**: `git worktree remove` handles everything
- **Lightweight**: Worktrees share git objects, not full clones

**Trade-offs:**
- More disk usage than shared directory
- Complexity of managing multiple worktrees
- Need to track worktree → agent mapping

### Why filesystem for messages?

**Decision:** Inter-agent messages are JSON files on the filesystem.

**Alternatives Considered:**
- SQLite database
- Redis/message queue
- Unix pipes
- HTTP API

**Rationale:**
- **Debuggability**: Just `cat` the files to see messages
- **Durability**: Files survive daemon restarts
- **Simplicity**: No additional dependencies
- **Inspectable**: Users can manually read/edit messages
- **Recovery**: Easy to repair corrupted state

**Trade-offs:**
- Polling instead of push (2-minute intervals)
- No guaranteed ordering across agents
- Manual cleanup needed for old messages

### Why single daemon?

**Decision:** One daemon process manages all repositories and agents.

**Alternatives Considered:**
- Per-repository daemons
- No daemon (CLI-only)
- Systemd services per repo

**Rationale:**
- **Single state**: One source of truth for all repositories
- **Coordination**: Central point for cross-agent communication
- **Resource management**: Easier to track and clean up
- **Simple operations**: One process to start/stop

**Trade-offs:**
- Single point of failure
- All repos share daemon lifecycle
- More complex internal state management

### Why Docker-style names?

**Decision:** Worker agents get auto-generated names like "happy-platypus" or "clever-fox".

**Alternatives Considered:**
- Sequential numbers (worker-1, worker-2)
- UUID-based names
- User-specified names only
- Branch-based names

**Rationale:**
- **Memorable**: Easier to discuss "clever-fox" than "worker-7"
- **Unique**: Random combination avoids collisions
- **Fun**: Adds personality to the system
- **Short**: Fits nicely in tmux window names

**Trade-offs:**
- Random names don't indicate task
- Possible (rare) collisions
- Subjective aesthetic preference

### Why JSON for state?

**Decision:** All persistent state is stored in a single JSON file.

**Alternatives Considered:**
- SQLite database
- Multiple files per entity
- Protocol buffers
- YAML

**Rationale:**
- **Human readable**: Easy to inspect with any editor
- **Simple recovery**: Can manually edit if corrupted
- **Atomic updates**: Temp file + rename pattern
- **No dependencies**: Standard library only
- **Full state snapshots**: Easy to backup/restore

**Trade-offs:**
- Entire file rewritten on every change
- No partial updates or queries
- Size limits (though unlikely to hit them)

### Why embedded prompts?

**Decision:** Default role prompts are embedded in the binary using Go's `//go:embed`.

**Alternatives Considered:**
- External files shipped with binary
- Fetch from remote URL
- Hardcoded strings in code
- Config files in ~/.multiclaude

**Rationale:**
- **Self-contained**: Binary has everything it needs
- **Version locked**: Prompts match binary version
- **Easy customization**: Repos can override with `.multiclaude/*.md`
- **Maintainable**: Prompts are real markdown files in source

**Trade-offs:**
- Prompts change requires rebuild
- Larger binary size (minimal impact)

### Why --dangerously-skip-permissions?

**Decision:** Agents run Claude with `--dangerously-skip-permissions` flag.

**Alternatives Considered:**
- Interactive permission prompts
- Pre-approved command allowlist
- Sandboxed execution

**Rationale:**
- **Autonomous operation**: Agents work without human intervention
- **Isolation via worktree**: Each agent is confined to its directory
- **Trust model**: We trust Claude's judgment within the repo
- **Speed**: No waiting for permission approvals

**Trade-offs:**
- Reduced safety guardrails
- Agents can execute any command
- Relies on prompt-based constraints for safety

## Constraints

### CI is Sacred

**Constraint:** Agents must never weaken or disable CI checks without explicit human approval.

**Enforcement:**
- Embedded in all agent prompts as a "golden rule"
- Merge queue explicitly prohibited from bypassing checks
- No programmatic enforcement (trust-based)

**Rationale:**
- CI is the source of truth for code quality
- Weakening CI to merge code is always the wrong trade-off
- Humans must approve any CI changes

### Forward Progress Over Perfection

**Constraint:** Any incremental progress is acceptable. The only failure is no movement at all.

**Enforcement:**
- Embedded in agent prompts
- Workers encouraged to create partial PRs
- Supervisor instructed to nudge for progress, not completion

**Rationale:**
- Perfect is the enemy of good
- Other agents can continue work
- Reviewable PRs enable human feedback

## Non-Goals

These are explicitly NOT goals for multiclaude:

### No Web Dashboard
We use tmux for observability. A web UI would add complexity without significant benefit for the target user (developers comfortable with terminal).

### No Remote Daemon
The daemon runs locally. Remote operation would require authentication, networking, and security infrastructure beyond our scope.

### No Cross-Repository Coordination
Each repository is independent. Cross-repo coordination would require complex dependency management.

### No Multi-User Support
Single user per daemon. Multi-user would require user isolation and access control.

### No Automatic Restart on Crash
When Claude crashes, we log it but don't auto-restart. Users can restart manually. Auto-restart risks infinite loops.

## Comparison to Alternatives

### vs. Running Claude Manually

| Aspect | multiclaude | Manual |
|--------|-------------|--------|
| Multiple tasks | Parallel agents | Sequential |
| Observability | tmux attach | N/A |
| Isolation | Git worktrees | Branch switching |
| Coordination | Message system | None |
| Persistence | Session IDs | Lost on restart |

### vs. Gastown

| Aspect | multiclaude | Gastown |
|--------|-------------|---------|
| Agent roles | 3 (supervisor, worker, merge-queue) | 7+ specialized roles |
| State persistence | JSON file | Git-backed hooks |
| Work tracking | Task descriptions | Beads framework |
| Complexity | Minimal | Comprehensive |
| Maturity | Early | More established |
| Philosophy | Unix simplicity | Full orchestration |

multiclaude aims to be simpler. If you need Gastown's features, use Gastown.

## Future Considerations

### Potential Additions
- Cost tracking per agent
- Agent templates for common tasks
- Issue tracker integration
- Performance profiling

### Unlikely Additions
- Web UI (breaks terminal-first philosophy)
- Remote daemon (too much infrastructure)
- Automatic agent spawning (too magic)

### Open Questions
- Should agents auto-restart on crash?
- Should supervisor have more programmatic control?
- How to handle very long-running tasks?

## Evolution

multiclaude is designed to be extended. The current architecture supports:

- **New agent types**: Add to `AgentType` enum and create prompts
- **New commands**: Add to CLI command tree
- **Custom prompts**: Repository `.multiclaude/` directory
- **Hooks integration**: Via `.multiclaude/hooks.json`

The core abstractions (tmux, worktrees, messages) are stable and unlikely to change.
