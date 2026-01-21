# Dual-Layer Brownian Ratchet Design

## Problem Statement

When working with forked repositories, the current multiclaude system only validates CI at a single layer (the current repository). This led to the incident documented in PR #205 where an upstream merge broke the fork's main branch:

1. Upstream (dlorenc/multiclaude) changed `daemon.New()` signature
2. Fork (aronchick/multiclaude) merged from upstream
3. Conflict resolution didn't update test files
4. Fork main broke for 3 commits (CI failing)
5. All PRs branching from fork/main inherited the breakage

**Root Cause**: No validation that fork CI passes after upstream merges, and no continuous monitoring to ensure fork stays healthy relative to upstream.

## Proposed Solution: Multi-Layer Brownian Ratchet

Implement a **two-tier CI validation system** where both fork-level CI and upstream-level CI must pass for forward progress:

```
┌─────────────────────────────────────────────────┐
│           Dual-Layer Ratchet System             │
├─────────────────────────────────────────────────┤
│                                                 │
│  Layer 1: Fork CI Validation                    │
│  ├─ Monitor fork/main CI status                 │
│  ├─ Validate PRs against fork                   │
│  └─ Block if fork CI fails                      │
│                                                 │
│  Layer 2: Upstream CI Validation                │
│  ├─ Monitor upstream/main CI status             │
│  ├─ Sync fork with upstream regularly           │
│  ├─ Validate PRs would pass upstream CI         │
│  └─ Block if upstream CI fails                  │
│                                                 │
│  Ratchet Mechanism:                             │
│  ├─ Forward progress ONLY if BOTH layers pass   │
│  ├─ Spawn fix agents immediately on ANY failure │
│  └─ Constant vigilance on both CI systems       │
│                                                 │
└─────────────────────────────────────────────────┘
```

## Core Principles

1. **Dual Validation**: PRs must pass CI on BOTH fork and upstream before merge
2. **Constant Monitoring**: Daemon continuously checks both fork and upstream CI status
3. **Immediate Response**: Any CI failure at either layer spawns fix agents automatically
4. **Sync Discipline**: Fork is kept up-to-date with upstream via regular sync operations
5. **No Compromise**: CI is king at BOTH layers - never weaken CI to make work pass

## Architecture Changes

### 1. State Extensions

Add to `internal/state/state.go`:

```go
// Repository tracking
type Repository struct {
    GithubURL        string             `json:"github_url"`
    TmuxSession      string             `json:"tmux_session"`
    Agents           map[string]Agent   `json:"agents"`
    TaskHistory      []TaskHistoryEntry `json:"task_history,omitempty"`
    MergeQueueConfig MergeQueueConfig   `json:"merge_queue_config,omitempty"`

    // NEW: Dual-layer CI tracking
    UpstreamConfig   *UpstreamConfig    `json:"upstream_config,omitempty"`
    DualCIStatus     *DualCIStatus      `json:"dual_ci_status,omitempty"`
}

// Upstream configuration
type UpstreamConfig struct {
    UpstreamURL    string    `json:"upstream_url"`     // e.g., "https://github.com/dlorenc/multiclaude"
    UpstreamRemote string    `json:"upstream_remote"`  // Usually "upstream"
    ForkRemote     string    `json:"fork_remote"`      // Usually "origin"
    SyncEnabled    bool      `json:"sync_enabled"`     // Enable fork/upstream sync
    SyncInterval   int       `json:"sync_interval"`    // Minutes between sync checks (default: 30)
}

// Dual-layer CI status tracking
type DualCIStatus struct {
    ForkCI       CILayerStatus `json:"fork_ci"`
    UpstreamCI   CILayerStatus `json:"upstream_ci"`
    LastSyncTime time.Time     `json:"last_sync_time"`
    LastSyncSHA  string        `json:"last_sync_sha"`      // Last upstream commit synced
    DivergenceCount int        `json:"divergence_count"`   // Commits fork is behind upstream
}

// CI status for one layer (fork or upstream)
type CILayerStatus struct {
    Status       string    `json:"status"`        // "passing", "failing", "pending", "unknown"
    LastCheck    time.Time `json:"last_check"`
    LastCommit   string    `json:"last_commit"`   // SHA of last checked commit
    FailingSince *time.Time `json:"failing_since,omitempty"` // When did it start failing
    CheckURL     string    `json:"check_url,omitempty"`      // GH Actions URL
}
```

### 2. CLI Changes

Add initialization flag to `multiclaude init`:

```bash
# Initialize with upstream tracking
multiclaude init <fork-url> [name] \
  --upstream <upstream-url> \
  --sync-interval 30 \
  --dual-ci-validation
```

Add new command for manual sync:

```bash
# Manually sync fork with upstream
multiclaude sync [--repo <repo>]

# Check dual-CI status
multiclaude ci-status [--repo <repo>]
```

### 3. Daemon Loop: Fork/Upstream Sync Monitor

Add new daemon loop in `internal/daemon/daemon.go`:

```go
// forkUpstreamSyncLoop runs every N minutes (configurable, default 30)
// Monitors fork/upstream divergence and CI status at both layers
func (d *Daemon) forkUpstreamSyncLoop(ctx context.Context) {
    ticker := time.NewTicker(30 * time.Minute) // Configurable per repo
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            d.checkForkUpstreamStatus()
        }
    }
}

func (d *Daemon) checkForkUpstreamStatus() {
    repos := d.state.ListRepos()

    for _, repoName := range repos {
        repo := d.state.GetRepo(repoName)
        if repo.UpstreamConfig == nil || !repo.UpstreamConfig.SyncEnabled {
            continue // Skip repos without upstream tracking
        }

        // 1. Check fork CI status
        forkCI := d.checkCIStatus(repo, "fork")

        // 2. Check upstream CI status
        upstreamCI := d.checkCIStatus(repo, "upstream")

        // 3. Check divergence (is fork behind upstream?)
        divergence := d.checkUpstreamDivergence(repo)

        // 4. Update state
        d.state.UpdateDualCIStatus(repoName, forkCI, upstreamCI, divergence)

        // 5. Take action if needed
        d.handleCIFailures(repoName, forkCI, upstreamCI, divergence)
    }
}

func (d *Daemon) handleCIFailures(repoName string, forkCI, upstreamCI CILayerStatus, divergence int) {
    // CRITICAL: If either layer fails, spawn fix agent immediately

    if forkCI.Status == "failing" {
        d.logger.Printf("[%s] FORK CI FAILING - spawning fix agent", repoName)
        d.spawnForkCIFixAgent(repoName, forkCI)
    }

    if upstreamCI.Status == "failing" {
        // Upstream failing means upstream main is broken
        // Log and notify, but don't try to fix upstream directly
        d.logger.Printf("[%s] UPSTREAM CI FAILING - monitoring only", repoName)
    }

    if divergence > 10 {
        // Fork is significantly behind upstream, spawn sync agent
        d.logger.Printf("[%s] Fork is %d commits behind upstream - spawning sync agent", repoName, divergence)
        d.spawnSyncAgent(repoName, divergence)
    }
}
```

### 4. Worker PR Creation Logic

Modify worker prompt and behavior in `internal/prompts/worker.md`:

**Before creating PR:**
1. Check fork CI status - if failing, fix fork first
2. Check if fork is diverged from upstream - if yes, sync first
3. Run tests locally
4. Create PR against fork
5. Wait for fork CI to pass
6. If targeting upstream: create upstream PR only after fork PR passes

**Worker prompt addition:**

```markdown
## Dual-Layer CI Validation

This repository uses dual-layer CI validation (fork + upstream). Before creating PRs:

1. **Check fork CI**: Run `multiclaude ci-status` to ensure fork/main is green
2. **Sync if needed**: If fork is behind upstream, run `multiclaude sync` first
3. **Local validation**: Always run tests locally before pushing
4. **Fork PR first**: Create PR against fork, ensure CI passes
5. **Upstream PR**: Only create upstream PR after fork CI is green

**CRITICAL**: Never create a PR if either fork or upstream CI is failing. Fix the failing layer first.
```

### 5. Merge Queue Integration

Modify `internal/prompts/merge-queue.md` to check both layers:

```markdown
## Dual-Layer CI Validation for Merge Queue

Before merging any PR:

1. **Fork CI**: Check fork CI status - must be GREEN ✅
2. **Upstream CI**: Check upstream CI status - must be GREEN ✅
3. **PR CI**: Check PR's own CI - must be GREEN ✅
4. **Divergence**: Ensure fork is not significantly behind upstream (< 5 commits)
5. **Sync after merge**: After merging to fork, check if should sync to upstream

**Merge Conditions (ALL must be true):**
- ✅ Fork main CI passing
- ✅ Upstream main CI passing
- ✅ PR CI passing
- ✅ Fork not severely diverged from upstream
- ✅ No merge conflicts

**If ANY layer fails**: STOP and spawn fix agent before merging.
```

## Implementation Phases

### Phase 1: State & Configuration (P0)
- [ ] Add `UpstreamConfig` and `DualCIStatus` to state
- [ ] Add `--upstream` flag to `multiclaude init`
- [ ] Add `multiclaude sync` command
- [ ] Add `multiclaude ci-status` command
- [ ] Add state migration for existing repos

### Phase 2: Monitoring Loop (P0)
- [ ] Implement `forkUpstreamSyncLoop` in daemon
- [ ] Implement `checkCIStatus` using GitHub API
- [ ] Implement `checkUpstreamDivergence` using git
- [ ] Add configurable sync interval per repo

### Phase 3: Automatic Actions (P1)
- [ ] Implement `spawnForkCIFixAgent`
- [ ] Implement `spawnSyncAgent`
- [ ] Add merge queue dual-CI checks
- [ ] Update worker prompt for dual-validation

### Phase 4: Refinements (P2)
- [ ] Add CI status to `multiclaude list` output
- [ ] Add alerts/logging for CI failures
- [ ] Add metrics for sync frequency and CI health
- [ ] Document fork workflow best practices

## ROADMAP Alignment

**Aligns with:**
- ✅ P0: "Worktree sync" - Extends to fork/upstream sync
- ✅ P0: "Clear error messages" - Better CI failure detection
- ✅ P0: "Reliable worker lifecycle" - Prevents broken base branches

**Does NOT violate:**
- ✅ Local-first: Still runs locally, just checks remote CI
- ✅ Claude-only: No multi-provider abstraction
- ✅ Simple: Adds necessary complexity for reliability
- ✅ Terminal-native: No web dashboards

## Success Criteria

1. **No broken fork/main**: Fork main branch CI never fails for > 1 commit
2. **Fast detection**: CI failures detected within sync interval (default 30 min)
3. **Automatic recovery**: Fix agents spawned automatically on failure
4. **Sync discipline**: Fork stays within 5 commits of upstream
5. **Zero upstream breakage**: Never create upstream PR if fork CI failing

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| **GitHub API rate limits** | Cache CI status, respect rate limits, exponential backoff |
| **False positives** | Add retry logic, distinguish pending vs failing |
| **Sync conflicts** | Manual resolution required, spawn agent to handle |
| **Upstream divergence** | Set threshold (e.g., 10 commits), alert user |
| **Performance** | Async checks, don't block daemon, configurable intervals |

## Example Workflow

### Scenario: Fork behind upstream, upstream has breaking change

1. **Detection** (t=0):
   - Daemon detects fork is 5 commits behind upstream
   - Daemon checks upstream CI: GREEN ✅
   - Daemon checks fork CI: GREEN ✅

2. **Sync** (t=0):
   - Daemon spawns sync agent
   - Sync agent fetches upstream, attempts merge
   - Merge succeeds, but tests fail locally

3. **Fix** (t=5min):
   - Sync agent detects test failures
   - Sync agent identifies breaking change (e.g., `daemon.New()` signature)
   - Sync agent fixes test files
   - Sync agent creates PR: "fix: Update tests after upstream merge"

4. **Validation** (t=10min):
   - PR CI runs: GREEN ✅
   - Merge queue validates: fork CI + upstream CI + PR CI all green
   - Merge queue merges PR

5. **Monitoring** (t=30min):
   - Daemon checks fork CI: GREEN ✅
   - Daemon checks upstream CI: GREEN ✅
   - Divergence: 0 commits
   - Status: HEALTHY ✅

## Open Questions

1. **GitHub API dependency**: Should we require GitHub PAT for CI status checks?
2. **Upstream write access**: Should we support pushing to upstream, or only PRs?
3. **Multiple upstreams**: Support repos with multiple upstream remotes?
4. **CI provider support**: GitHub Actions only, or support CircleCI, etc.?
5. **Manual override**: Allow users to disable dual-validation temporarily?

## Next Steps

1. **Get approval**: Confirm this design aligns with project goals
2. **Prototype**: Implement Phase 1 (state & config) as proof of concept
3. **Test**: Validate with aronchick/multiclaude fork scenario
4. **Iterate**: Refine based on real-world usage
5. **Document**: Add to CLAUDE.md and create user guide

## References

- PR #205: Original issue documenting fork breakage
- ROADMAP.md: P0 "Worktree sync" requirement
- internal/worktree/worktree.go: Existing upstream handling code
- CLAUDE.md: Brownian Ratchet philosophy
