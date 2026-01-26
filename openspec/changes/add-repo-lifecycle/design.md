# Design: Repo Lifecycle Management

## Context

Users interact with multiclaude through fragmented commands that don't provide a cohesive "session" experience. Starting work on a repo requires multiple commands, and there's no way to pause/resume work or get comprehensive status.

**Stakeholders**: CLI users, automation scripts, external tools (TUI, web dashboard)

**Constraints**:
- Must be backward compatible (existing commands unchanged)
- State changes must be atomic (crash-safe)
- Output formats must be consistent across commands

## Goals / Non-Goals

### Goals
- Unified repo lifecycle: start → work → hibernate → wake → clean
- Comprehensive status in single command
- Machine-readable output for tooling integration
- Interactive TUI for power users
- WebSocket streaming for external dashboards

### Non-Goals
- Web UI (separate project: multiclaude-ui)
- Multi-machine coordination
- Cloud sync of hibernation state

## Decisions

### Decision 1: Hibernation vs Stop

**What**: Hibernate preserves agent configuration for later resume. Stop terminates completely.

**Why**: Users often context-switch between repos. Hibernate allows quick resume without re-specifying agent configuration.

**Alternatives considered**:
- Just use stop/start: Loses agent configuration and task context
- Auto-save always: Adds complexity, may save unwanted state

### Decision 2: Output Format Architecture

**What**: All commands use `OutputFormatter` interface with implementations for text/json/yaml.

```go
type OutputFormatter interface {
    FormatStatus(status *RepoStatus) ([]byte, error)
    FormatList(repos []RepoInfo) ([]byte, error)
    FormatResult(result *CommandResult) ([]byte, error)
}
```

**Why**: Consistent formatting, easy to add new formats, testable.

**Alternatives considered**:
- Per-command formatting: Leads to inconsistency
- Template-based: Harder to maintain, less flexible

### Decision 3: TUI Library

**What**: Use [bubbletea](https://github.com/charmbracelet/bubbletea) for TUI.

**Why**:
- Popular in Go ecosystem (more LLM training data)
- Elm architecture is simple and testable
- Good accessibility support
- Active maintenance

**Alternatives considered**:
- [tview](https://github.com/rivo/tview): More traditional, less modern feel
- Custom: Too much work, maintenance burden

### Decision 4: WebSocket Protocol

**What**: JSON messages over WebSocket with message types.

```json
{
  "type": "status_update",
  "repo": "myrepo",
  "data": { /* RepoStatus JSON */ }
}
```

**Why**: Simple, standard, easy to consume from any language.

**Alternatives considered**:
- gRPC streaming: Overkill for local use
- Server-Sent Events: Less bidirectional capability

### Decision 5: Refresh Strategy

**What**: Parallel worktree rebase with continue-on-failure.

**Why**:
- Don't block all worktrees if one has conflicts
- Report all issues at once
- User can address conflicts selectively

**Alternatives considered**:
- Sequential: Slower, stops at first failure
- Merge instead of rebase: Creates merge commits, messier history

## Data Model Changes

### State.json Extensions

```go
type Repository struct {
    // ... existing fields ...

    // New fields
    Status          RepoStatus       `json:"status"`           // active, hibernated
    HibernatedAt    *time.Time       `json:"hibernated_at"`    // when hibernated
    HibernationData *HibernationData `json:"hibernation_data"` // preserved state
}

type RepoStatus string

const (
    RepoStatusActive      RepoStatus = "active"
    RepoStatusHibernated  RepoStatus = "hibernated"
    RepoStatusUninitialized RepoStatus = "uninitialized"
)

type HibernationData struct {
    Agents    map[string]AgentConfig `json:"agents"`     // agent configs to restore
    Timestamp time.Time              `json:"timestamp"`
}

type AgentConfig struct {
    Type    AgentType `json:"type"`
    Task    string    `json:"task,omitempty"`
    Branch  string    `json:"branch,omitempty"`
}
```

## Risks / Trade-offs

### Risk: Hibernation state becomes stale
- **Mitigation**: Warn if hibernation > 7 days old
- **Mitigation**: Offer `--fresh` flag to ignore hibernation state

### Risk: WebSocket adds daemon complexity
- **Mitigation**: Make it opt-in (only when --websocket flag used)
- **Mitigation**: Separate goroutine, isolated from main daemon logic

### Risk: TUI dependency adds bloat
- **Mitigation**: Lazy-load TUI (only import when --tui used)
- **Mitigation**: Consider making TUI a separate binary

### Trade-off: Parallel refresh can leave partial state
- **Accepted**: Better than blocking. Clear error reporting mitigates.

## Migration Plan

1. **Phase 1** (this change): Core commands (start, status, hibernate, wake, refresh, clean)
2. **Phase 2**: TUI mode
3. **Phase 3**: WebSocket streaming

No breaking changes. Existing commands continue to work.

## Open Questions

1. Should `repo start` be the default when running `multiclaude` with a repo argument?
2. Should hibernation auto-expire after N days?
3. Should WebSocket require authentication for security?
