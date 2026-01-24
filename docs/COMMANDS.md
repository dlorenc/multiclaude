# Commands Reference

Everything you can tell multiclaude to do.

## Daemon

The daemon is the brain. Start it, and agents come alive.

```bash
multiclaude start              # Wake up
multiclaude daemon stop        # Go to sleep
multiclaude daemon status      # You alive?
multiclaude daemon logs -f     # What are you thinking?
multiclaude stop-all           # Kill everything
multiclaude stop-all --clean   # Kill everything and forget it ever happened
```

## Repositories

Point multiclaude at a repo and watch it go.

```bash
multiclaude repo init <github-url>              # Track a repo
multiclaude repo init <github-url> [name]       # Track with a custom name
multiclaude repo list                           # What repos do I have?
multiclaude repo rm <name>                      # Forget about this one
```

## Workspaces

Your workspace is your home base. A persistent Claude session that remembers you.

```bash
multiclaude workspace add <name>           # New workspace
multiclaude workspace add <name> --branch main  # New workspace from a specific branch
multiclaude workspace list                 # Show all workspaces
multiclaude workspace connect <name>       # Jump in
multiclaude workspace rm <name>            # Tear it down (warns if you have uncommitted work)
multiclaude workspace                      # List (shorthand)
multiclaude workspace <name>               # Connect (shorthand)
```

Workspaces use `workspace/<name>` branches. A "default" workspace spawns automatically when you init a repo.

## Workers

Workers do the grunt work. Give them a task, they make a PR.

```bash
multiclaude worker create "task description"        # Spawn a worker
multiclaude worker create "task" --branch feature   # Start from a specific branch
multiclaude worker create "Fix tests" --branch origin/work/fox --push-to work/fox  # Iterate on existing PR
multiclaude worker list                      # Who's working?
multiclaude worker rm <name>                 # Fire this one
```

`multiclaude work` works too. We're flexible.

The `--push-to` flag is for iterating on existing PRs. Worker pushes to that branch instead of making a new one.

## Observing

Watch the magic happen.

```bash
multiclaude agent attach <agent-name>            # Jump into an agent's terminal
multiclaude agent attach <agent-name> --read-only # Watch without touching
tmux attach -t mc-<repo>                         # See the whole session
```

## Messaging

Agents talk to each other. You can eavesdrop. Or join the conversation.

```bash
multiclaude message send <to> "msg"        # Slide into their DMs
multiclaude message list                   # What's in my inbox?
multiclaude message read <id>              # Read a message
multiclaude message ack <id>               # Mark it read
```

## Agent Commands

Commands agents run (not you, usually).

```bash
multiclaude agent complete                 # Worker says "I'm done, clean me up"
```

## Slash Commands

Inside Claude sessions, agents get these superpowers:

- `/refresh` - Sync with main (fetch, rebase, the works)
- `/status` - What's the situation?
- `/workers` - Who else is working?
- `/messages` - Check the group chat

## Custom Agents

Roll your own agents with markdown.

```bash
multiclaude agents list                    # What agent types exist?
multiclaude agents reset                   # Reset to factory defaults
multiclaude agents spawn --name <n> --class <c> --prompt-file <f>  # Birth a custom agent
```

Local definitions: `~/.multiclaude/repos/<repo>/agents/`
Shared with team: `<repo>/.multiclaude/agents/`

## Debugging

Things broken? Here's how to poke around.

```bash
# Watch an agent think
multiclaude agent attach <agent-name> --read-only

# Check messages
multiclaude message list

# Daemon brain dump
tail -f ~/.multiclaude/daemon.log

# Fix broken state
multiclaude repair                 # Local fix
multiclaude cleanup --dry-run      # What would we clean?
multiclaude cleanup                # Actually clean it
```
