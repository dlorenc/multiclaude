# multiclaude vs Gastown

How we stack up against [Gastown](https://github.com/steveyegge/gastown), Steve Yegge's multi-agent orchestrator.

## The Short Version

Both projects do the same thing: run multiple Claude Code agents on a shared codebase. Both use Go, tmux, and git worktrees. Both shipped in early 2026.

If you're shopping for a multi-agent orchestrator, try both. Seriously.

## The Differences

| | multiclaude | Gastown |
|--|-------------|---------|
| **Philosophy** | Worse is better. Unix vibes. | Full-featured orchestration. |
| **Agents** | 6 types | 7 types (Mayor, Polecats, Refinery...) |
| **State** | JSON file | Git-backed "hooks" |
| **Work tracking** | Task descriptions | "Beads" framework |
| **Messaging** | Filesystem JSON | Beads framework |
| **Crash recovery** | Daemon self-heals | Git-based recovery |

multiclaude is simpler. Gastown is richer. Pick your poison.

## The Big Difference: MMO vs Single-Player

Here's where we really diverge.

**Gastown** treats agents like NPCs in a single-player game. You're the player. Agents are your minions. Great for solo dev wanting to parallelize.

**multiclaude** treats software engineering like an **MMO**. You're one player among many—some human, some AI.

- Your workspace is your character
- Workers are party members you summon
- The supervisor is your guild leader
- The merge queue is the raid boss guarding main

**What this means in practice:**

- Your workspace persists. It's home base, not a temp session.
- You spawn workers and check on them later. You don't micromanage.
- Other humans can have their own workspaces on the same repo.
- Log off. System keeps running. Come back to progress.

## When to Use Which

**Use multiclaude if:**
- You like simple tools
- You're working with a team
- You want agents running while you sleep
- You prefer "it just works" over "it has every feature"

**Use Gastown if:**
- You want sophisticated work tracking
- You need git-backed crash recovery
- You prefer structured orchestration
- You're mostly solo and want max parallelization
