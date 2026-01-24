# multiclaude

> *Why tell Claude what to do when you can tell Claude to tell Claude what to do?*

Multiple Claude Code agents. One repo. Controlled chaos.

multiclaude spawns autonomous Claude Code instances that coordinate, compete, and collaborate on your codebase. Each agent gets its own tmux window and git worktree. You watch. They work. PRs appear.

**Self-hosting since day one.** multiclaude builds itself. The agents you're reading about wrote the code you're reading about.

## The Philosophy: Brownian Ratchet

Inspired by the [Brownian ratchet](https://en.wikipedia.org/wiki/Brownian_ratchet) - random motion converted to forward progress through a one-way mechanism.

Multiple agents work simultaneously. They might duplicate effort. They might conflict. *This is fine.*

**CI is the ratchet.** Every PR that passes gets merged. Progress is permanent. We never go backward.

- 🎲 **Chaos is Expected** - Redundant work is cheaper than blocked work
- 🔒 **CI is King** - If tests pass, ship it. If tests fail, fix it.
- ⚡ **Forward > Perfect** - Three okay PRs beat one perfect PR
- 👤 **Humans Approve** - Agents propose. You dispose.

## Quick Start

```bash
# Install
go install github.com/dlorenc/multiclaude/cmd/multiclaude@latest

# Prerequisites: tmux, git, gh (authenticated)

# Fire it up
multiclaude start
multiclaude repo init https://github.com/your/repo

# Spawn a worker and watch the magic
multiclaude worker create "Add unit tests for the auth module"
tmux attach -t mc-repo
```

That's it. You now have a supervisor, merge queue, and worker grinding away. Detach with `Ctrl-b d` and they keep working while you sleep.

## Two Modes

**Single Player** - [Merge-queue](internal/templates/agent-templates/merge-queue.md) auto-merges PRs when CI passes. You're the only human. Maximum velocity.

```bash
multiclaude repo init https://github.com/you/repo  # your repo
```

**Multiplayer** - [PR-shepherd](internal/templates/agent-templates/pr-shepherd.md) coordinates with human reviewers, tracks approvals, respects your team's review process.

```bash
multiclaude repo init https://github.com/you/fork  # auto-detected as fork
```

Fork detection is automatic. If you're initializing a fork, multiclaude enables pr-shepherd and disables merge-queue (you can't merge to upstream anyway).

## Built-in Agents

```
┌─────────────────────────────────────────────────────────────┐
│                     tmux session: mc-repo                   │
├───────────────┬───────────────┬───────────────┬─────────────┤
│  supervisor   │  merge-queue  │  workspace    │ swift-eagle │
│               │               │               │             │
│ Coordinates   │ Merges when   │ Your personal │ Working on  │
│ the chaos     │ CI passes     │ Claude        │ a task      │
└───────────────┴───────────────┴───────────────┴─────────────┘
```

| Agent | Role | Definition |
|-------|------|------------|
| **Supervisor** | Air traffic control. Nudges stuck agents. Answers "what's happening?" | [supervisor.md](internal/prompts/supervisor.md) |
| **Merge Queue** | The bouncer (single player). CI passes? You're in. | [merge-queue.md](internal/templates/agent-templates/merge-queue.md) |
| **PR Shepherd** | The diplomat (multiplayer). Coordinates human reviewers. | [pr-shepherd.md](internal/templates/agent-templates/pr-shepherd.md) |
| **Workspace** | Your personal Claude. Spawn workers, check status. | [workspace.md](internal/prompts/workspace.md) |
| **Worker** | The grunts. One task, one branch, one PR. Done. | [worker.md](internal/templates/agent-templates/worker.md) |
| **Reviewer** | Code review bot. Reads PRs, leaves comments. | [reviewer.md](internal/templates/agent-templates/reviewer.md) |

## Fully Extensible in Markdown

These are just the built-in agents. **Want more? Write markdown.**

Create `~/.multiclaude/repos/<repo>/agents/docs-reviewer.md`:

```markdown
# Docs Reviewer

You review documentation changes. Focus on:
- Accuracy - does the docs match the code?
- Clarity - can a new developer understand this?
- Completeness - are edge cases documented?

When you find issues, leave helpful PR comments. Be constructive, not pedantic.
```

Then spawn it:

```bash
multiclaude agents spawn --name docs-bot --class docs-reviewer --prompt-file docs-reviewer.md
```

Check your repo's `.multiclaude/agents/` to share custom agents with your team.

## The MMORPG Model

multiclaude treats software engineering like an **MMO, not a single-player game**.

Your workspace is your character. Workers are party members you summon. The supervisor is your guild leader. The merge queue is the raid boss guarding main.

Log off. The game keeps running. Come back to progress.

## Documentation

- **[Commands Reference](docs/COMMANDS.md)** - All the CLI commands
- **[Agent Guide](docs/AGENTS.md)** - How agents work and customization
- **[Architecture](docs/ARCHITECTURE.md)** - System design and internals
- **[Workflows](docs/WORKFLOWS.md)** - Detailed examples and patterns
- **[vs Gastown](docs/GASTOWN.md)** - Comparison with Steve Yegge's orchestrator

## Public Libraries

Two reusable Go packages:

- **[pkg/tmux](pkg/tmux/)** - Programmatic tmux control with multiline support
- **[pkg/claude](pkg/claude/)** - Launch and interact with Claude Code instances

## Building

```bash
go build ./cmd/multiclaude    # Build
go test ./...                  # Test
go install ./cmd/multiclaude  # Install
```

Requires: Go 1.21+, tmux, git, gh (authenticated)

## License

MIT
