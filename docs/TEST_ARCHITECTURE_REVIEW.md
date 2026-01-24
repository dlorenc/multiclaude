# Test Architecture Review

This document provides a comprehensive analysis of the test architecture across the multiclaude codebase, identifying opportunities to reduce duplication, simplify test setups, and consolidate patterns without decreasing coverage.

## Executive Summary

After reviewing all 29 test files across the codebase, several patterns of duplication were identified along with specific recommendations for improvement. The proposed changes would reduce test code by approximately 530 lines (10-15%) while maintaining or improving coverage and readability.

---

## 1. Shared Test Helpers to Extract

### 1.1 Git Repository Setup Helper (HIGH PRIORITY)

**Current Problem:** The git repo setup pattern is duplicated in 4 locations with minor variations:
- `internal/fork/fork_test.go:119` - `setupTestRepo()`
- `internal/cli/cli_test.go:356` - `setupTestRepo()`
- `internal/worktree/worktree_test.go:25` - `createTestRepo()`
- `test/agents_test.go` - `setupTestGitRepo()`

**Recommendation:** Create a shared test helper package at `internal/testutil/git.go`:

```go
// internal/testutil/git.go
package testutil

import (
    "os"
    "os/exec"
    "testing"
)

// SetupGitRepo creates a temporary git repository for testing.
// Returns the path to the repository (cleanup is handled by t.TempDir).
func SetupGitRepo(t *testing.T) string {
    t.Helper()
    tmpDir := t.TempDir()

    // Initialize with explicit 'main' branch for consistency
    cmd := exec.Command("git", "init", "-b", "main")
    cmd.Dir = tmpDir
    if err := cmd.Run(); err != nil {
        t.Fatalf("Failed to init git repo: %v", err)
    }

    // Configure git user
    for _, args := range [][]string{
        {"config", "user.name", "Test User"},
        {"config", "user.email", "test@example.com"},
    } {
        cmd = exec.Command("git", args...)
        cmd.Dir = tmpDir
        cmd.Run()
    }

    // Create initial commit
    if err := os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("# Test\n"), 0644); err != nil {
        t.Fatalf("Failed to create README: %v", err)
    }

    cmd = exec.Command("git", "add", "README.md")
    cmd.Dir = tmpDir
    cmd.Run()

    cmd = exec.Command("git", "commit", "-m", "Initial commit")
    cmd.Dir = tmpDir
    if err := cmd.Run(); err != nil {
        t.Fatalf("Failed to commit: %v", err)
    }

    return tmpDir
}

// SetupGitRepoWithBranch creates a git repo and an additional branch.
func SetupGitRepoWithBranch(t *testing.T, branchName string) string {
    t.Helper()
    repoPath := SetupGitRepo(t)

    cmd := exec.Command("git", "branch", branchName)
    cmd.Dir = repoPath
    if err := cmd.Run(); err != nil {
        t.Fatalf("Failed to create branch %s: %v", branchName, err)
    }

    return repoPath
}
```

**Files to Update:** 4 files, ~150 lines removed

---

### 1.2 Daemon Test Environment Helper (HIGH PRIORITY)

**Current Problem:** Complex test environment setup is duplicated in:
- `internal/cli/cli_test.go:288` - `setupTestEnvironment()` (50+ lines)
- `test/agents_test.go` - Similar setup repeated in each test
- `test/integration_test.go` - Similar patterns

**Recommendation:** Create `internal/testutil/environment.go`:

```go
package testutil

import (
    "os"
    "path/filepath"
    "testing"
    "time"

    "github.com/dlorenc/multiclaude/internal/cli"
    "github.com/dlorenc/multiclaude/internal/daemon"
    "github.com/dlorenc/multiclaude/pkg/config"
)

// TestEnv encapsulates a test environment with CLI, daemon, and paths.
type TestEnv struct {
    CLI     *cli.CLI
    Daemon  *daemon.Daemon
    Paths   *config.Paths
    TmpDir  string
}

// SetupTestEnvironment creates a complete test environment.
// Call the returned cleanup function when done.
func SetupTestEnvironment(t *testing.T) (*TestEnv, func()) {
    t.Helper()

    // Set test mode
    os.Setenv("MULTICLAUDE_TEST_MODE", "1")

    tmpDir := t.TempDir()
    tmpDir, _ = filepath.EvalSymlinks(tmpDir) // Handle macOS symlinks

    paths := config.NewTestPaths(tmpDir)
    paths.EnsureDirectories()

    d, err := daemon.New(paths)
    if err != nil {
        t.Fatalf("Failed to create daemon: %v", err)
    }

    if err := d.Start(); err != nil {
        t.Fatalf("Failed to start daemon: %v", err)
    }

    time.Sleep(100 * time.Millisecond)

    c := cli.NewWithPaths(paths)

    cleanup := func() {
        d.Stop()
        os.Unsetenv("MULTICLAUDE_TEST_MODE")
    }

    return &TestEnv{
        CLI:    c,
        Daemon: d,
        Paths:  paths,
        TmpDir: tmpDir,
    }, cleanup
}
```

**Files to Update:** 3+ files, ~200 lines removed

---

### 1.3 config.Paths Construction (MEDIUM PRIORITY)

**Current Problem:** Manual `config.Paths` construction repeated in 10+ test files:

```go
// This pattern appears in many files - should use NewTestPaths instead
paths := &config.Paths{
    Root:            tmpDir,
    DaemonPID:       filepath.Join(tmpDir, "daemon.pid"),
    DaemonSock:      filepath.Join(tmpDir, "daemon.sock"),
    DaemonLog:       filepath.Join(tmpDir, "daemon.log"),
    StateFile:       filepath.Join(tmpDir, "state.json"),
    ReposDir:        filepath.Join(tmpDir, "repos"),
    WorktreesDir:    filepath.Join(tmpDir, "wts"),
    MessagesDir:     filepath.Join(tmpDir, "messages"),
    OutputDir:       filepath.Join(tmpDir, "output"),
    ClaudeConfigDir: filepath.Join(tmpDir, "claude-config"),
}
```

**Recommendation:** The helper `config.NewTestPaths(tmpDir)` already exists but is underutilized. Update these files:
- `internal/bugreport/collector_test.go:23-34`
- `internal/cli/cli_test.go:308-319`
- `test/agents_test.go` (multiple locations)
- `test/integration_test.go`
- `test/e2e_test.go`

**Files to Update:** 6+ files, ~80 lines removed

---

### 1.4 Tmux Test Helpers (MEDIUM PRIORITY)

**Current Problem:** `pkg/tmux/client_test.go` has excellent helpers that could benefit integration tests:
- `skipIfCannotCreateSessions(t)`
- `createTestSessionOrSkip(t, sessionName)`
- `waitForSession(sessionName, timeout)`
- `cleanupTestSessions(t, sessions)`
- `uniqueSessionName(prefix)`

**Recommendation:** Move these to `internal/testutil/tmux.go` and use in:
- `test/e2e_test.go`
- `test/integration_test.go`
- `test/recovery_test.go`

---

## 2. Redundant Test Cases

### 2.1 Empty/Nil Input Tests

Several files duplicate empty/nil input tests that could be consolidated using subtests:
- `internal/cli/selector_test.go:145-158` - `TestAgentsToSelectableItems_EmptyInput`
- `internal/cli/selector_test.go:263-275` - `TestReposToSelectableItems_EmptyInput`

### 2.2 Idempotency Tests

Multiple packages test idempotent operations identically:
- `internal/prompts/commands/commands_test.go:147` - `TestSetupAgentCommandsIdempotent`
- `internal/templates/templates_test.go:73` - `TestCopyAgentTemplatesIdempotent`

**Recommendation:** Create a shared idempotency test helper:

```go
func TestIdempotent(t *testing.T, name string, setup func() error) {
    t.Helper()
    t.Run(name+"_first_call", func(t *testing.T) {
        if err := setup(); err != nil {
            t.Fatalf("First call failed: %v", err)
        }
    })
    t.Run(name+"_second_call", func(t *testing.T) {
        if err := setup(); err != nil {
            t.Fatalf("Second call (idempotent) failed: %v", err)
        }
    })
}
```

---

## 3. Overly Complex Test Setups

### 3.1 Socket Server/Client Tests

**File:** `internal/socket/socket_test.go`

**Problem:** Each test repeats the same server setup (~15 lines per test):

```go
tmpDir := t.TempDir()
sockPath := filepath.Join(tmpDir, "test.sock")
handler := HandlerFunc(func(req Request) Response { ... })
server := NewServer(sockPath, handler)
server.Start()
defer server.Stop()
go server.Serve()
time.Sleep(100 * time.Millisecond)
```

**Recommendation:** Create a test fixture:

```go
func setupSocketTest(t *testing.T, handler Handler) (*Client, func()) {
    t.Helper()
    tmpDir := t.TempDir()
    sockPath := filepath.Join(tmpDir, "test.sock")

    server := NewServer(sockPath, handler)
    if err := server.Start(); err != nil {
        t.Fatalf("Failed to start server: %v", err)
    }

    go server.Serve()
    time.Sleep(100 * time.Millisecond)

    client := NewClient(sockPath)

    cleanup := func() {
        server.Stop()
    }

    return client, cleanup
}
```

**Lines saved:** ~100 lines

---

### 3.2 Messages Manager Tests

**File:** `internal/messages/messages_test.go`

**Problem:** Similar pattern - `NewManager(tmpDir)` created in each of 15+ tests.

**Recommendation:** Use a test fixture:

```go
func setupMessageTest(t *testing.T) (*Manager, string) {
    t.Helper()
    tmpDir := t.TempDir()
    m := NewManager(tmpDir)
    return m, "test-repo"
}
```

---

## 4. Patterns to Consolidate

### 4.1 Inconsistent Table-Driven Tests

**Good examples to follow:**
- `internal/fork/fork_test.go:10` - `TestParseGitHubURL`
- `internal/format/format_test.go:62` - `TestTimeAgo`
- `internal/cli/cli_test.go:20` - `TestParseFlags`

**Files that should adopt table-driven patterns:**
- `internal/redact/redact_test.go` - Could consolidate 6 similar tests
- `internal/hooks/hooks_test.go` - Could use subtests more consistently

### 4.2 Error Path Testing

**Inconsistent pattern:** Some tests check error messages, others just check `err != nil`.

**Good pattern (follow this):**
```go
// From pkg/claude/runner_test.go
if !strings.Contains(err.Error(), "terminal runner not configured") {
    t.Errorf("expected 'terminal runner not configured' error, got %q", err.Error())
}
```

**Less specific (avoid):**
```go
if err == nil {
    t.Error("expected error")
}
```

---

## 5. Specific Recommendations by Package

| Package | Issue | Recommendation | Effort |
|---------|-------|----------------|--------|
| `internal/socket` | Repeated server setup | Extract `setupSocketTest()` helper | Low |
| `internal/messages` | Repeated manager creation | Extract fixture | Low |
| `internal/fork`, `internal/cli`, `internal/worktree` | Duplicate `setupTestRepo` | Share via `testutil` | Medium |
| `test/*` | Manual `config.Paths` | Use `config.NewTestPaths()` | Low |
| `pkg/tmux` | Good helpers isolated | Export to `testutil/tmux.go` | Medium |

---

## 6. Proposed New Package Structure

```
internal/testutil/
├── git.go           # SetupGitRepo, SetupGitRepoWithBranch
├── environment.go   # SetupTestEnvironment, TestEnv
├── tmux.go          # Tmux session helpers (moved from pkg/tmux)
├── fixtures.go      # Common test fixtures
└── helpers.go       # Utility functions (idempotency testing, etc.)
```

---

## 7. Implementation Priority

### Immediate (Low Effort, High Impact)
- Replace manual `config.Paths` construction with `NewTestPaths()` (6+ files)
- Add socket test helper in `internal/socket/socket_test.go`

### Short-term (Medium Effort)
- Create `internal/testutil/git.go` with shared git helpers
- Consolidate table-driven tests in `internal/redact/redact_test.go`

### Medium-term (Higher Effort)
- Create `internal/testutil/environment.go` for integration tests
- Move tmux helpers to shared location

---

## 8. Estimated Impact

| Change | Lines Removed | Files Affected |
|--------|--------------|----------------|
| Use `NewTestPaths()` | ~80 | 6 |
| Consolidate git setup | ~150 | 4 |
| Socket test helper | ~100 | 1 |
| Environment helper | ~200 | 3 |
| **Total** | **~530** | **14** |

This represents approximately a 10-15% reduction in test code while maintaining or improving coverage and readability.

---

## 9. Test Files Analyzed

**Unit Tests (internal/):**
1. `internal/agents/agents_test.go`
2. `internal/bugreport/collector_test.go`
3. `internal/cli/cli_test.go`
4. `internal/cli/selector_test.go`
5. `internal/daemon/daemon_test.go`
6. `internal/daemon/handlers_test.go`
7. `internal/daemon/pid_test.go`
8. `internal/daemon/utils_test.go`
9. `internal/errors/errors_test.go`
10. `internal/fork/fork_test.go`
11. `internal/format/format_test.go`
12. `internal/hooks/hooks_test.go`
13. `internal/logging/logger_test.go`
14. `internal/messages/messages_test.go`
15. `internal/names/names_test.go`
16. `internal/prompts/commands/commands_test.go`
17. `internal/prompts/prompts_test.go`
18. `internal/redact/redact_test.go`
19. `internal/socket/socket_test.go`
20. `internal/state/state_test.go`
21. `internal/templates/templates_test.go`
22. `internal/worktree/worktree_test.go`

**Unit Tests (pkg/):**
23. `pkg/claude/runner_test.go`
24. `pkg/claude/prompt/builder_test.go`
25. `pkg/config/config_test.go`
26. `pkg/config/doc_test.go`
27. `pkg/tmux/client_test.go`

**Integration/E2E Tests (test/):**
28. `test/agents_test.go`
29. `test/e2e_test.go`
30. `test/integration_test.go`
31. `test/recovery_test.go`
