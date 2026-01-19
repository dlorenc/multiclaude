# Multiclaude Architectural Review

## Executive Summary

This document presents a comprehensive architectural review of the multiclaude codebase. The analysis identifies key strengths, areas for improvement, and actionable recommendations for enhancing maintainability, reducing duplication, and improving code quality.

**Overall Assessment:** The codebase demonstrates solid fundamentals with clear package separation, good test coverage (~160+ tests across 14 test files), and pragmatic use of real system integrations. However, several areas of code duplication and inconsistent patterns have emerged as the codebase has grown, presenting opportunities for refactoring.

---

## Architecture Overview

### Directory Structure
```
multiclaude/
├── cmd/                          # Executable entry points
│   ├── multiclaude/              # Main CLI application
│   └── generate-docs/            # Documentation generator
├── internal/                     # Private packages
│   ├── cli/                      # CLI command routing (~2500 lines)
│   ├── daemon/                   # Background coordinator (~1100 lines)
│   ├── state/                    # Persistent state management
│   ├── socket/                   # Unix socket communication
│   ├── tmux/                     # tmux operations wrapper
│   ├── worktree/                 # Git worktree management
│   ├── messages/                 # Inter-agent messaging
│   ├── prompts/                  # Agent role prompts
│   ├── logging/                  # Structured logging
│   └── names/                    # Random name generation
├── pkg/config/                   # Public configuration
└── test/                         # Integration/E2E tests
```

### Core Components

| Component | Responsibility | Lines |
|-----------|---------------|-------|
| CLI (`internal/cli`) | Command routing, user interaction | ~2500 |
| Daemon (`internal/daemon`) | Background orchestration, health checks | ~1100 |
| State (`internal/state`) | Thread-safe persistence | ~200 |
| Socket (`internal/socket`) | CLI-Daemon communication | ~160 |
| Tmux (`internal/tmux`) | Terminal session management | ~200 |
| Worktree (`internal/worktree`) | Git isolation per agent | ~200 |
| Messages (`internal/messages`) | Inter-agent communication | ~220 |

---

## Key Findings

### 1. Code Duplication (HIGH Priority)

#### 1.1 Socket Request/Response Pattern - 37+ Occurrences

**Files:** `internal/cli/cli.go`, `internal/daemon/daemon.go`

The same socket communication pattern is repeated throughout the CLI:
```go
client := socket.NewClient(c.paths.DaemonSock)
resp, err := client.Send(socket.Request{
    Command: "add_agent",
    Args: map[string]interface{}{...},
})
if err != nil {
    return fmt.Errorf("failed to register ...: %w", err)
}
if !resp.Success {
    return fmt.Errorf("failed to register...: %s", resp.Error)
}
```

**Locations in cli.go:** Lines 354-366, 383-394, 448-451, 562-576, 643-656, 659-676, 679-696, 743-760, 870-880, 923-941, 1064-1075, 1154-1166, 1409-1421, 1558-1575, 1617-1627, 1687-1698, 1946-1954

**Recommendation:** Create a `DaemonClient` wrapper:
```go
func (c *CLI) sendDaemonRequest(command string, args map[string]interface{}) (socket.Response, error)
```

#### 1.2 Repository Context Resolution - 4+ Implementations

Different implementations for determining the current repository context:

| Location | Lines | Approach |
|----------|-------|----------|
| `getReposList()` | 1172-1197 | Helper function |
| `stopAll()` | 445-464 | Inline implementation |
| `createWorker()` | 825-837 | Inline implementation |
| `reviewPR()` | 1463-1475 | Inline implementation |

**Recommendation:** Extract unified `resolveRepository()` helper.

#### 1.3 Agent Creation Logic - 4 Similar Implementations

Agent creation is implemented similarly in:
- `cli.go:initRepo()` - supervisor, merge-queue, workspace (lines 541-770)
- `cli.go:createWorker()` - worker creation (lines 804-952)
- `cli.go:reviewPR()` - review agent creation (lines 1428-1587)
- `daemon.go:startAgent()` - daemon restoration (lines 1016-1073)

**Recommendation:** Extract `AgentLifecycleManager` with unified `CreateAndRegister()` method.

---

### 2. Large Functions Requiring Decomposition (MEDIUM Priority)

| Function | File | Lines | Responsibilities |
|----------|------|-------|------------------|
| `initRepo()` | cli.go | 541-770 (230 lines) | Clone, tmux setup, prompts, Claude startup, registration |
| `createWorker()` | cli.go | 804-952 (150 lines) | Repo selection, worktree, tmux, Claude startup |
| `localCleanup()` | cli.go | 1666-1928 (260 lines) | Tmux, worktree, message, PID cleanup |
| `reviewPR()` | cli.go | 1428-1587 (160 lines) | PR extraction, agent creation |
| `checkAgentHealth()` | daemon.go | 220-293 (75 lines) | Health checks with repeated patterns |

**Recommendation:** Decompose into focused functions:
- `initRepo()` → `createTmuxSession()`, `createAgentWindow()`, `startAgent()`, `registerAgent()`
- `localCleanup()` → `cleanupOrphanedTmuxSessions()`, `cleanupOrphanedWorktrees()`, `cleanupOrphanedMessages()`, `cleanupStaleDaemonFiles()`

---

### 3. Inconsistent Patterns (MEDIUM Priority)

#### 3.1 Logging Approach

| Component | Approach | Count |
|-----------|----------|-------|
| CLI (`cli.go`) | `fmt.Println`/`fmt.Printf` | 191 instances |
| Daemon (`daemon.go`) | Structured logger (`d.logger.*`) | Consistent |

**Impact:** Different observability. Daemon logs go to file, CLI output goes to stdout.

**Recommendation:** Add structured logging option to CLI for consistent debugging.

#### 3.2 Error Handling Styles

```go
// Style 1: Check both error and success separately
if err != nil {
    return fmt.Errorf("failed to list repos: %w (is daemon running?)", err)
}
if !resp.Success {
    return fmt.Errorf("failed to list repos: %s", resp.Error)
}

// Style 2: Inline check
if err == nil && resp.Success { /* ... */ }

// Style 3: Only check error
if err != nil {
    return fmt.Errorf("daemon not running...")
}
```

**Recommendation:** Standardize on the `DaemonClient` wrapper with consistent error handling.

#### 3.3 Agent Data Model Mixed Concerns

```go
type Agent struct {
    Type            AgentType
    WorktreePath    string
    TmuxWindow      string
    SessionID       string
    PID             int
    Task            string         // Worker-only
    CreatedAt       time.Time
    LastNudge       time.Time      // Daemon-only for tracking
    ReadyForCleanup bool           // Cleanup-only
}
```

**Recommendation:** Consider composition pattern or clear documentation of field usage.

---

### 4. Error Handling Issues (HIGH Priority)

#### 4.1 Message Delivery Without Retry Limits

**File:** `internal/daemon/daemon.go:352-367`

Messages stuck in `pending` state are retried infinitely without failure tracking:
```go
if err := d.tmux.SendKeysLiteral(...); err != nil {
    d.logger.Error("Failed to deliver message...")
    continue  // Message stays pending, retried forever
}
```

**Recommendation:** Implement retry limits and a `failed` status for messages.

#### 4.2 Silent Error Swallowing

**File:** `internal/messages/messages.go:80-84`
```go
msg, err := m.read(repoName, agentName, entry.Name())
if err != nil {
    continue  // Silently skips - no logging
}
```

**Recommendation:** Log or track skipped items for troubleshooting.

#### 4.3 Socket Server No Timeout/Recovery

**File:** `internal/socket/socket.go:107-116`
```go
for {
    conn, err := s.listener.Accept()
    if err != nil {
        return fmt.Errorf("failed to accept: %w", err)  // Crashes server
    }
}
```

**Recommendation:** Add graceful error handling for transient accept failures.

---

### 5. Testing Assessment

#### Coverage Summary
- **Total:** 14 test files, ~7,600 lines of test code, ~160+ tests
- **Approach:** Integration-heavy with real system dependencies (tmux, git, filesystem)
- **Strengths:** Table-driven tests, concurrency testing, recovery scenarios

#### Gaps Identified

| Gap | Priority | Location |
|-----|----------|----------|
| No CLI main.go tests | Low | `cmd/multiclaude/main.go` |
| No performance benchmarks | Medium | All packages |
| Limited chaos/failure injection | Medium | Daemon, socket |
| No mock filesystem option | Low | All file I/O heavy packages |

---

## Prioritized Recommendations

### High Priority (Address First)

1. **Create DaemonClient Wrapper** - Eliminate 37+ duplicated socket patterns
   - New file: `internal/cli/daemon_client.go`
   - Estimated impact: ~200 lines of duplication removed

2. **Implement Message Retry Limits** - Prevent infinite retry loops
   - File: `internal/daemon/daemon.go`
   - Add `DeliveryAttempts` and `FailedAt` fields to Message struct

3. **Extract Repository Context Resolver** - Unify 4+ implementations
   - File: `internal/cli/cli.go`
   - Create: `func (c *CLI) resolveRepository(flags, cwd) (string, error)`

### Medium Priority

4. **Decompose `initRepo()` Function** (230 lines → 4-5 functions)
   - Extract: `createTmuxSession()`, `createAgentWindow()`, `startAgent()`

5. **Decompose `localCleanup()` Function** (260 lines → 4 functions)
   - Extract: `cleanupOrphanedTmuxSessions()`, `cleanupOrphanedWorktrees()`, etc.

6. **Standardize Error Handling** - Consistent wrapping and messages
   - Add context to all returned errors
   - Standardize daemon availability error messages

7. **Add Socket Server Resilience** - Handle transient accept errors
   - File: `internal/socket/socket.go`

### Low Priority (Technical Debt)

8. **Extract Agent Lifecycle Manager** - Consolidate agent creation
   - New file: `internal/agent/manager.go`

9. **Add Structured Logging to CLI** - Optional file logging
   - Enhance `internal/logging/` for CLI use

10. **Add Performance Benchmarks** - State load/save, message routing
    - New benchmarks in `*_test.go` files

---

## Refactoring Targets

### Files Requiring Most Attention

| File | Issues | Lines to Refactor |
|------|--------|------------------|
| `internal/cli/cli.go` | 4 large functions, 37+ duplications | ~500 lines |
| `internal/daemon/daemon.go` | Agent creation duplication, health check patterns | ~150 lines |
| `internal/socket/socket.go` | Error handling resilience | ~30 lines |
| `internal/messages/messages.go` | Silent error handling | ~20 lines |

### New Files to Create

| File | Purpose |
|------|---------|
| `internal/cli/daemon_client.go` | DaemonClient wrapper for socket communication |
| `internal/cli/repository.go` | Repository context resolution logic |
| `internal/agent/manager.go` | Agent lifecycle management (optional) |

---

## Architectural Strengths

The codebase exhibits several positive architectural qualities:

1. **Clear Package Boundaries** - Each package has a single responsibility
2. **Pragmatic Testing** - Real system integration over mocks provides high confidence
3. **Graceful Degradation** - Daemon continues despite individual agent failures
4. **Atomic State Persistence** - Temp file + rename pattern prevents corruption
5. **Observable by Design** - Tmux visibility allows real-time inspection
6. **Clean Entry Points** - Minimal bootstrap in main.go

---

## Conclusion

The multiclaude codebase is well-structured with clear separation of concerns. The main areas for improvement are:

1. **Code duplication** in CLI socket communication and repository resolution
2. **Large function decomposition** needed in cli.go
3. **Error handling consistency** across the codebase

Addressing the high-priority items would significantly improve maintainability while requiring modest refactoring effort. The existing test suite provides good coverage for validation of changes.

---

*Generated: 2026-01-19*
*Review Scope: Full codebase architectural analysis*
