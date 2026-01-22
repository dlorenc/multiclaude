# Upstream Contribution Workflow

**For repositories that are forks of upstream projects.**

This document describes how multiclaude manages bidirectional sync with upstream repositories and contributes improvements back.

## Quick Reference

| Agent | Responsibility |
|-------|---------------|
| **Merge Queue** | Execute upstream sync, identify & submit upstream contributions |
| **Supervisor** | Monitor upstream status, nudge merge-queue, enforce focused PRs |
| **Workers** | Create focused, upstream-ready PRs (one task = one PR) |

## Philosophy

Our fork should be a **good upstream citizen**:
- Stay current with upstream changes (minimize drift)
- Contribute improvements back regularly (share the value)
- Submit focused, reviewable PRs (respect upstream maintainers' time)
- Handle conflicts promptly (don't let them accumulate)

## Upstream Sync (Pulling from Upstream)

### Frequency

**Check on every merge session** (multiple times per day).

The merge-queue agent runs this check:

```bash
git fetch upstream
git log --oneline main..upstream/main | head -10
```

If any commits appear, we're behind upstream.

### Process

1. **Merge-queue creates sync PR immediately**
   - Spawns a worker: `multiclaude work "Sync with upstream: merge latest changes from upstream/main"`
   - Notifies supervisor of the sync operation
   - Treats sync PR as P0 priority

2. **Supervisor fast-tracks the sync PR**
   - No new feature work while sync is pending
   - Conflicts get immediate attention

3. **Worker resolves any conflicts**
   - Prefers upstream's changes unless ours are clearly intentional improvements
   - Documents conflict resolution decisions in PR description

4. **Merge-queue merges once CI passes**
   - Updates local main: `git fetch origin main:main`
   - Resumes normal operations

### Why This Matters

- **Prevents drift**: Small, frequent syncs are easier than large, infrequent ones
- **Reduces conflicts**: Fewer changes to reconcile at once
- **Keeps our improvements compatible**: We're building on the latest upstream

## Upstream Contributions (Pushing to Upstream)

### Frequency

**Every 5-10 merged PRs** or **weekly** (whichever comes first).

The merge-queue agent checks:

```bash
git log --oneline upstream/main..main --no-merges | wc -l
```

If we have 5+ commits ahead, it's time to contribute back.

### Identifying Contribution Candidates

**Include:**
- Features that add value to the upstream project (P0/P1 roadmap items)
- Bug fixes that affect upstream (not fork-specific)
- Test improvements
- Documentation improvements
- Performance optimizations

**Exclude:**
- Fork-specific features (custom workflows, fork-only config)
- Experimental features not yet proven stable
- Changes that conflict with upstream's stated direction
- Work-in-progress or incomplete features

### Grouping Strategy

**Group related commits into focused upstream PRs:**

✅ **Good grouping:**
- All commits for "agent restart feature" → 1 upstream PR
- All commits for "enhanced task history" → 1 upstream PR
- All commits for "improved error messages" → 1 upstream PR

❌ **Bad grouping:**
- "Agent restart" + "task history" + "error messages" → 1 upstream PR (too broad)
- Random commits from different features → 1 upstream PR (unfocused)

**Each upstream PR should tell one coherent story.**

### Submission Process

1. **Create branch from upstream/main:**
   ```bash
   git checkout -b upstream-contrib/<feature-name> upstream/main
   ```

2. **Cherry-pick relevant commits:**
   ```bash
   git cherry-pick <commit-hash-1> <commit-hash-2> ...
   ```

3. **Create PR against upstream:**
   ```bash
   gh pr create \
     --repo <upstream-owner>/<upstream-repo> \
     --base main \
     --head <your-fork>:upstream-contrib/<feature-name> \
     --title "feat: <focused title>" \
     --body "<clear description>"
   ```

4. **Track the upstream PR:**
   ```bash
   multiclaude agent send-message supervisor "Created upstream PR: <url>"
   ```

### Example Session

```bash
# Check what we can contribute
git log --oneline upstream/main..main --no-merges | head -20

# Identify: commits f26afa7, 72e4d13, 61fac9b are "agent restart"
git checkout -b upstream-contrib/agent-restart upstream/main
git cherry-pick f26afa7 72e4d13 61fac9b

# Create PR to upstream
gh pr create \
  --repo dlorenc/multiclaude \
  --base main \
  --head aronchick:upstream-contrib/agent-restart \
  --title "feat: Add agent restart command" \
  --body "## Summary
This PR adds the ability to restart crashed agents without losing context.

## Changes
- Added 'multiclaude agent restart' command
- Agent state is preserved during restarts
- Enhanced error recovery with automatic workspace restoration

Addresses P1 roadmap item: Agent restart

## Testing
- Added unit tests for restart logic
- Tested manual crash scenarios
- Verified state preservation across restarts"
```

### Monitoring Upstream PRs

Once submitted, the merge-queue monitors upstream PRs:

```bash
gh pr list --repo <upstream-owner>/<upstream-repo> --author <your-username> --state open
```

If upstream PRs get feedback:
- Supervisor decides: human intervention or spawn worker to address
- If addressing feedback, worker operates on the contribution branch
- Update upstream PR with force-push if needed

## PR Scope Enforcement

**For PRs to be upstream-ready, they must be focused.** This is enforced at multiple levels:

### Workers (Creation Time)

Workers are instructed:
- **ONE TASK = ONE PR** (no exceptions)
- No drive-by changes
- No scope expansion
- Size guidelines: bug fix <300 lines, small feature <800 lines

### Merge Queue (Merge Time)

Before merging, merge-queue validates:
- Mandatory scope checklist (all items must pass)
- Size vs. stated purpose match
- No unrelated files modified
- All commits relate to the same goal

**If scope mismatch detected:** PR is blocked with `scope-mismatch` label and detailed comment.

### Supervisor (Oversight)

Supervisor monitors for:
- Workers expanding scope during implementation
- Task assignments that bundle multiple goals
- Patterns of scope creep

**Interventions:**
- Nudge workers to stay focused
- Split overly broad tasks before assignment
- Support merge-queue's scope enforcement

## Size Guidelines

| PR Type | Expected Size | Flag If | Action If Exceeded |
|---------|---------------|---------|-------------------|
| Typo/config fix | <20 lines | >100 lines | Reject immediately |
| Bug fix | <100 lines | >300 lines | Verify single bug |
| Small feature | <300 lines | >800 lines | Check for scope creep |
| Medium feature | <800 lines | >1500 lines | Verify all changes essential |
| Large feature | <1500 lines | >2500 lines | Must have issue/PRD |

*Test files excluded from line counts*

## Benefits of This Workflow

**For Upstream:**
- Receives focused, reviewable contributions
- Each PR is self-contained and understandable
- Lower maintenance burden (easier to review, test, rollback)

**For Our Fork:**
- Stays current with upstream improvements
- Smaller, easier conflict resolution
- Our improvements get wider adoption
- Better relationship with upstream maintainers

**For The Codebase:**
- Changes are documented clearly
- History is readable
- Rollbacks are surgical, not catastrophic
- Testing is focused

## Red Flags (Scope Violations)

Watch for these patterns that indicate scope problems:

1. **Size mismatch**: "Fix typo" with 500+ line diff
2. **Unrelated files**: "URL parsing" but touches notification system
3. **Multiple unrelated commits**: No coherent story
4. **Kitchen sink PRs**: Touches 30+ files across all packages
5. **Drive-by changes**: Reformats unrelated code
6. **Misleading titles**: Title describes only last commit

**Response:** Block merge, require human review or PR split.

## Human Override

Humans can override scope enforcement when justified:
- Complex refactoring that genuinely needs to touch many files
- Security fixes that require changes across layers
- Major features with explicit approval

**To override:** Add `scope-override-approved` label with comment explaining why.

## Metrics to Track

**Upstream Health:**
- Commits behind upstream (target: <5)
- Days since last upstream sync (target: <7)
- Commits ahead of upstream (target: contribute when >10)
- Open upstream PRs from our fork

**PR Quality:**
- Average PR size (target: <300 lines for most PRs)
- Scope mismatch rate (target: <5%)
- Time to merge (focused PRs merge faster)

## Troubleshooting

### "We're falling behind upstream"

**Symptoms:** `git log main..upstream/main` shows many commits

**Fix:**
1. Merge-queue spawns sync worker immediately
2. Supervisor halts new feature work until sync completes
3. Address conflicts promptly
4. Consider more frequent sync checks

### "Upstream rejecting our PRs"

**Symptoms:** Upstream PRs get closed or heavily criticized

**Possible causes:**
- PRs too large or unfocused
- Not following upstream's contribution guidelines
- Features don't align with upstream's direction

**Fix:**
1. Review upstream's CONTRIBUTING.md
2. Make PRs even more focused
3. Discuss approach before implementing
4. Consider keeping fork-specific features separate

### "Too many scope mismatch blocks"

**Symptoms:** Many PRs flagged by merge-queue for scope issues

**Root causes:**
- Workers not following focus discipline
- Tasks assigned with multiple goals
- "While I'm here" mentality

**Fix:**
1. Supervisor reviews task assignments for focus
2. Stronger nudges to workers expanding scope
3. Review worker prompt effectiveness
4. Training: share examples of good vs. bad PRs

## See Also

- [CONTRIBUTING.md](../CONTRIBUTING.md) - General contribution guidelines
- [ROADMAP.md](../ROADMAP.md) - Project priorities and scope
- [Agent prompts](../internal/prompts/) - Detailed agent instructions
- `.multiclaude/REVIEWER.md` - Repository-specific merge criteria
