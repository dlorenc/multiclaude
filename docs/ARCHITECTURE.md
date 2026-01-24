# Architecture

How the sausage gets made.

## Design Principles

1. **Observable** - Everything happens in tmux. Watch it. Poke it. Intervene if you want.
2. **Isolated** - Each agent gets its own git worktree. No stepping on toes.
3. **Recoverable** - State lives on disk. Daemon crashes? It comes back.
4. **Safe** - Agents can't weaken CI or bypass humans. That's the deal.
5. **Simple** - Files for state. tmux for visibility. git for isolation. No magic.

## The Big Picture

```
┌─────────────────────────────────────────────────────────────────┐
│                         CLI (cmd/multiclaude)                    │
└────────────────────────────────┬────────────────────────────────┘
                                 │ Unix Socket
┌────────────────────────────────▼────────────────────────────────┐
│                          Daemon (internal/daemon)                │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐        │
│  │ Health   │  │ Message  │  │ Wake/    │  │ Socket   │        │
│  │ Check    │  │ Router   │  │ Nudge    │  │ Server   │        │
│  │ (2min)   │  │ (2min)   │  │ (2min)   │  │          │        │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘        │
└────────────────────────────────┬────────────────────────────────┘
                                 │
    ┌────────────────────────────┼────────────────────────────────┐
    │                            │                                │
┌───▼───┐  ┌───────────┐  ┌─────▼─────┐  ┌──────────┐  ┌────────┐
│super- │  │merge-     │  │workspace  │  │worker-N  │  │review  │
│visor  │  │queue      │  │           │  │          │  │        │
└───────┘  └───────────┘  └───────────┘  └──────────┘  └────────┘
    │           │              │              │             │
    └───────────┴──────────────┴──────────────┴─────────────┘
              tmux session: mc-<repo>  (one window per agent)
```

## Package Map

| Package | What It Does |
|---------|--------------|
| `cmd/multiclaude` | Entry point. The `main()` lives here. |
| `internal/cli` | All the CLI commands. It's a big file. |
| `internal/daemon` | The brain. Runs the loops, manages everything. |
| `internal/state` | Persistence. `state.json` lives and breathes here. |
| `internal/messages` | How agents talk to each other. |
| `internal/prompts` | Embedded system prompts for agents. |
| `internal/worktree` | Git worktree wrangling. |
| `internal/socket` | Unix socket IPC between CLI and daemon. |
| `internal/errors` | Nice error messages for humans. |
| `internal/names` | Generates worker names (adjective-animal style). |
| `pkg/tmux` | **Public library** - programmatic tmux control. |
| `pkg/claude` | **Public library** - launch and talk to Claude Code. |

## Data Flow

1. **CLI** parses your command → sends request over Unix socket
2. **Daemon** handles it → updates `state.json` → pokes tmux
3. **Agents** run in tmux windows with their prompts and slash commands
4. **Messages** flow through JSON files, daemon routes them
5. **Health checks** run every 2 min, clean up the dead, resurrect the fallen

## Where Stuff Lives

```
~/.multiclaude/
├── daemon.pid          # Is the daemon alive?
├── daemon.sock         # CLI talks to daemon here
├── daemon.log          # What the daemon is thinking
├── state.json          # The source of truth
├── repos/<repo>/       # Cloned repos
│   └── agents/         # Local agent customizations
├── wts/<repo>/         # Git worktrees (one per agent)
├── messages/<repo>/    # Agent DMs
└── claude-config/<repo>/<agent>/  # Slash commands per agent
```

Check `.multiclaude/agents/` in your repo to share custom agents with your team. Those take priority over local ones.

## State

Everything the daemon knows lives in `~/.multiclaude/state.json`:

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
          "tmux_window": "supervisor"
        },
        "clever-fox": {
          "type": "worker",
          "task": "Implement auth feature",
          "ready_for_cleanup": false
        }
      }
    }
  }
}
```

Writes are atomic: temp file → rename. No corruption.

## Self-Healing

The daemon doesn't give up easily. Every 2 minutes it:

1. Checks if tmux sessions exist
2. If something died, tries to bring it back
3. Only gives up if restoration fails
4. Cleans up anything marked for cleanup
5. Prunes orphaned worktrees and message directories

Kill tmux accidentally? Daemon will notice and rebuild.

## The Nudge

Agents can get stuck. The daemon pokes them every 2 minutes:

| Agent | Nudge |
|-------|-------|
| supervisor | "Status check: Review worker progress and check merge queue." |
| merge-queue | "Status check: Review open PRs and check CI status." |
| worker | "Status check: Update on your progress?" |
| workspace | **Never nudged** - that's your space |

## Public Libraries

Want to use our building blocks? Go for it.

### pkg/tmux

```bash
go get github.com/dlorenc/multiclaude/pkg/tmux
```

Programmatic tmux control with multiline support. Send complex input atomically. Capture output. Monitor processes.

```go
client := tmux.NewClient()
client.SendKeysLiteral("session", "window", "multi\nline\ntext")
pid, _ := client.GetPanePID("session", "window")
```

### pkg/claude

```bash
go get github.com/dlorenc/multiclaude/pkg/claude
```

Launch and interact with Claude Code instances.

```go
runner := claude.NewRunner(
    claude.WithTerminal(tmuxClient),
    claude.WithBinaryPath(claude.ResolveBinaryPath()),
)
runner.Start("session", "window", claude.Config{
    SystemPromptFile: "/path/to/prompt.md",
})
runner.SendMessage("session", "window", "Hello, Claude!")
```
