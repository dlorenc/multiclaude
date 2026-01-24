# Workflows

How to actually use this thing.

## The tmux Session

Attach to a repo and you'll see this:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ mc-myrepo: supervisor | merge-queue | workspace | swift-eagle | calm-deer   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  $ claude                                                                   │
│                                                                             │
│  ╭─────────────────────────────────────────────────────────────────────────╮│
│  │ I'll check on the current workers and see if anyone needs help.        ││
│  │                                                                         ││
│  │ > multiclaude worker list                                               ││
│  │ Workers (2):                                                            ││
│  │   - swift-eagle: working on issue #44                                   ││
│  │   - calm-deer: working on issue #24                                     ││
│  │                                                                         ││
│  │ Both workers are making progress. swift-eagle just pushed a commit.    ││
│  ╰─────────────────────────────────────────────────────────────────────────╯│
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

Standard tmux controls:
- `Ctrl-b n` / `Ctrl-b p` — Next/previous window
- `Ctrl-b 0-9` — Jump to window by number
- `Ctrl-b w` — Window picker
- `Ctrl-b d` — Detach (agents keep grinding)

## Spawning Workers

You're in your workspace. You want stuff done. Spawn workers.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ You:                                                                        │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  > Let's tackle issues #44 and #45 in parallel                              │
│                                                                             │
│  ╭─────────────────────────────────────────────────────────────────────────╮│
│  │ On it.                                                                  ││
│  │                                                                         ││
│  │ > multiclaude worker create "Implement rich list commands per issue #44"││
│  │ ✓ Worker created: swift-eagle (branch: work/swift-eagle)                ││
│  │                                                                         ││
│  │ > multiclaude worker create "Improve error messages per issue #45"      ││
│  │ ✓ Worker created: calm-deer (branch: work/calm-deer)                    ││
│  │                                                                         ││
│  │ Two workers deployed. Check on them with:                               ││
│  │   multiclaude worker list                                               ││
│  │   multiclaude agent attach swift-eagle                                  ││
│  ╰─────────────────────────────────────────────────────────────────────────╯│
│                                                                             │
│  > Cool. Going to lunch.                                                    │
│                                                                             │
│  ╭─────────────────────────────────────────────────────────────────────────╮│
│  │ Enjoy. I'll keep an eye on things. Workers will keep running.           ││
│  ╰─────────────────────────────────────────────────────────────────────────╯│
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

Come back later:

```
│  > Back. What happened?                                                     │
│                                                                             │
│  ╭─────────────────────────────────────────────────────────────────────────╮│
│  │ Welcome back.                                                           ││
│  │                                                                         ││
│  │ ✓ swift-eagle completed - PR #47 for rich list commands                 ││
│  │ ✓ calm-deer completed - PR #48 for error messages                       ││
│  │                                                                         ││
│  │ Both PRs passing CI. Merge queue is on it.                              ││
│  ╰─────────────────────────────────────────────────────────────────────────╯│
```

## Watching the Supervisor

The supervisor is air traffic control. Watch it coordinate:

```bash
multiclaude agent attach supervisor --read-only
```

```
│  ╭─────────────────────────────────────────────────────────────────────────╮│
│  │ [Periodic check - 14:32]                                                ││
│  │                                                                         ││
│  │ Checking agent status...                                                ││
│  │                                                                         ││
│  │ Agents:                                                                 ││
│  │   supervisor: healthy (me)                                              ││
│  │   merge-queue: healthy, monitoring 2 PRs                                ││
│  │   workspace: healthy, user attached                                     ││
│  │   swift-eagle: healthy, working on #44                                  ││
│  │   calm-deer: stuck on test failure                                      ││
│  │                                                                         ││
│  │ Sending help to calm-deer...                                            ││
│  │                                                                         ││
│  │ > multiclaude message send calm-deer "Stuck on tests? The flaky test    ││
│  │   in auth_test.go has timing issues. Try mocking the clock."            ││
│  ╰─────────────────────────────────────────────────────────────────────────╯│
```

## Watching the Merge Queue

The merge queue is the bouncer. CI passes? You're in.

```bash
multiclaude agent attach merge-queue --read-only
```

```
│  ╭─────────────────────────────────────────────────────────────────────────╮│
│  │ [PR Check - 14:45]                                                      ││
│  │                                                                         ││
│  │ > gh pr list --author @me                                               ││
│  │ #47  Add rich list commands      work/swift-eagle                       ││
│  │ #48  Improve error messages      work/calm-deer                         ││
│  │                                                                         ││
│  │ Checking #47...                                                         ││
│  │ > gh pr checks 47                                                       ││
│  │ ✓ All checks passed                                                     ││
│  │                                                                         ││
│  │ Merging.                                                                ││
│  │ > gh pr merge 47 --squash --auto                                        ││
│  │ ✓ Merged #47 into main                                                  ││
│  │                                                                         ││
│  │ > multiclaude message send supervisor "Merged PR #47"                   ││
│  ╰─────────────────────────────────────────────────────────────────────────╯│
```

CI fails? Merge queue spawns a fixer:

```
│  │ Checking #48...                                                         ││
│  │ ✗ Tests failed: 2 failures in error_test.go                             ││
│  │                                                                         ││
│  │ Spawning fixup worker...                                                ││
│  │ > multiclaude worker create "Fix test failures in PR #48" \             ││
│  │     --branch work/calm-deer                                             ││
│  │ ✓ Worker created: quick-fox                                             ││
│  │                                                                         ││
│  │ I'll check back after quick-fox pushes.                                 ││
```

## Iterating on a PR

Got review comments? Spawn a worker to fix them:

```bash
multiclaude worker create "Fix review comments on PR #48" \
  --branch origin/work/calm-deer \
  --push-to work/calm-deer
```

Worker pushes to the existing branch. Same PR. No mess.

## Agent Stuck?

```bash
# Watch what it's doing
multiclaude agent attach <name> --read-only

# Check its messages
multiclaude message list

# Watch daemon logs
tail -f ~/.multiclaude/daemon.log
```

## State Broken?

```bash
# Quick fix
multiclaude repair

# See what's wrong
multiclaude cleanup --dry-run

# Nuke the cruft
multiclaude cleanup
```
