# Multiclaude Extensibility Guide

> **NOTE: IMPLEMENTATION STATUS VARIES**
>
> Some extension points documented here are **not fully implemented**:
> - **State File**: ✅ Implemented - works as documented
> - **Event Hooks**: ❌ NOT IMPLEMENTED - `multiclaude hooks` command does not exist
> - **Socket API**: ⚠️ Partially implemented - some commands may not exist
> - **Web UI Reference**: ❌ NOT IMPLEMENTED - `cmd/multiclaude-web` does not exist
>
> Per ROADMAP.md, web interfaces and notification systems are **out of scope** for upstream multiclaude.
> These docs are preserved for fork implementations.

**Target Audience:** Future LLMs and developers building extensions for multiclaude

This guide documents how to extend multiclaude **without modifying the core binary**. Multiclaude is designed with a clean separation between core orchestration and external integrations, allowing downstream projects to build custom notifications, web UIs, monitoring tools, and more.

## Philosophy

**Zero-Modification Extension:** Multiclaude provides clean interfaces for external tools:
- **State File**: Read-only JSON state for monitoring and visualization (✅ IMPLEMENTED)
- **Event Hooks**: Execute custom scripts on lifecycle events (❌ NOT IMPLEMENTED)
- **Socket API**: Programmatic control via Unix socket IPC (⚠️ PARTIAL)
- **File System**: Standard directories for messages, logs, and worktrees (✅ IMPLEMENTED)

**Fork-Friendly Architecture:** Extensions that upstream rejects (web UIs, notifications) can be maintained in forks without conflicts, as they operate entirely outside the core binary.

## Extension Points Overview

| Extension Point | Use Cases | Read | Write | Complexity |
|----------------|-----------|------|-------|------------|
| **State File** | Monitoring, dashboards, analytics | ✓ | ✗ | Low |
| **Event Hooks** | Notifications, webhooks, alerting | ✓ | ✗ | Low |
| **Socket API** | Custom CLIs, automation, control planes | ✓ | ✓ | Medium |
| **File System** | Log parsing, message injection, debugging | ✓ | ⚠️ | Low |
| **Public Packages** | Embedded orchestration, custom runners | ✓ | ✓ | High |

## Quick Start for Common Use Cases

### Building a Notification Integration

**Goal:** Send Slack/Discord/email alerts when PRs are created, CI fails, etc.

**Best Approach:** Event Hooks (see [`EVENT_HOOKS.md`](extending/EVENT_HOOKS.md))

**Why:** Fire-and-forget design, no dependencies, user-controlled scripts.

**Example:**
```bash
# Configure hook
multiclaude hooks set on_pr_created /usr/local/bin/notify-slack.sh

# Your hook receives JSON via stdin
# {
#   "type": "pr_created",
#   "timestamp": "2024-01-15T10:30:00Z",
#   "repo_name": "my-repo",
#   "agent_name": "clever-fox",
#   "data": {"pr_number": 42, "title": "...", "url": "..."}
# }
```

### Building a Web Dashboard

**Goal:** Visual monitoring of agents, tasks, and PRs across repositories.

**Best Approach:** State File Reader + Web Server (see [`WEB_UI_DEVELOPMENT.md`](extending/WEB_UI_DEVELOPMENT.md))

**Why:** Read-only, no daemon interaction needed, simple architecture.

**Reference Implementation:** `cmd/multiclaude-web/` - Full web dashboard in <500 LOC

**Architecture Pattern:**
```
┌──────────────┐
│ state.json   │ ← Atomic writes by daemon
└──────┬───────┘
       │ (fsnotify watch)
┌──────▼───────┐
│ StateReader  │ ← Your tool
└──────┬───────┘
       │
┌──────▼───────┐
│ HTTP Server  │ ← REST API + SSE for live updates
└──────────────┘
```

### Building Custom Automation

**Goal:** Programmatically spawn workers, query status, manage repos.

**Best Approach:** Socket API client (see [`SOCKET_API.md`](extending/SOCKET_API.md))

**Why:** Full control plane access, structured request/response.

**Example:**
```go
client := socket.NewClient("~/.multiclaude/daemon.sock")
resp, err := client.Send(socket.Request{
    Command: "spawn_worker",
    Args: map[string]interface{}{
        "repo": "my-repo",
        "task": "Add authentication",
    },
})
```

### Building Analytics/Reporting

**Goal:** Task history analysis, success rates, PR metrics.

**Best Approach:** State File Reader (see [`STATE_FILE_INTEGRATION.md`](extending/STATE_FILE_INTEGRATION.md))

**Why:** Complete historical data, no daemon dependency, simple JSON parsing.

**Schema:**
```json
{
  "repos": {
    "my-repo": {
      "task_history": [
        {
          "name": "clever-fox",
          "task": "...",
          "status": "merged",
          "pr_url": "...",
          "created_at": "...",
          "completed_at": "..."
        }
      ]
    }
  }
}
```

## Extension Categories

### 1. Read-Only Monitoring Tools

**Characteristics:**
- No daemon interaction required
- Watch `state.json` with `fsnotify` or periodic polling
- Zero risk of breaking multiclaude operation
- Can run on different machines (via file sharing or SSH)

**Examples:**
- Web dashboards (`multiclaude-web`)
- CLI status monitors
- Metrics exporters (Prometheus, Datadog)
- Log aggregators

**See:** [`STATE_FILE_INTEGRATION.md`](extending/STATE_FILE_INTEGRATION.md)

### 2. Event-Driven Integrations

**Characteristics:**
- Daemon emits events → your script executes
- Fire-and-forget (30s timeout, no retries)
- User-controlled, zero core dependencies
- Ideal for notifications and webhooks

**Examples:**
- Slack/Discord/email notifications
- GitHub status updates
- Custom alerting systems
- Audit logging

**See:** [`EVENT_HOOKS.md`](extending/EVENT_HOOKS.md)

### 3. Control Plane Tools

**Characteristics:**
- Read and write via socket API
- Structured JSON request/response
- Full programmatic control
- Requires daemon to be running

**Examples:**
- Custom CLIs (alternative to `multiclaude` binary)
- IDE integrations
- CI/CD orchestration
- Workflow automation tools

**See:** [`SOCKET_API.md`](extending/SOCKET_API.md)

### 4. Embedded Orchestration

**Characteristics:**
- Import multiclaude as a Go library
- Use public packages: `pkg/claude`, `pkg/tmux`, `pkg/config`
- Build custom orchestrators with multiclaude primitives
- Maximum flexibility, maximum complexity

**Examples:**
- Custom multi-agent systems
- Alternative daemon implementations
- Testing frameworks

**See:** Package documentation in `pkg/*/README.md`

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                    External Extensions (Your Tools)              │
│                                                                  │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │ Web Dashboard│  │ Slack Notif. │  │ Custom CLI   │          │
│  │ (fsnotify)   │  │ (hook script)│  │ (socket API) │          │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘          │
└─────────┼──────────────────┼──────────────────┼─────────────────┘
          │                  │                  │
          │ READ             │ EXECUTE          │ READ/WRITE
          │                  │                  │
┌─────────▼──────────────────▼──────────────────▼─────────────────┐
│                     Multiclaude Core                             │
│                                                                  │
│  state.json        Hooks Config      daemon.sock                │
│  (atomic writes)   (in state)        (Unix socket)              │
│                                                                  │
│  ┌──────────┐    ┌──────────┐      ┌──────────┐                │
│  │  Daemon  │    │  Event   │      │  Socket  │                │
│  │  Loops   │───▶│  Bus     │      │  Server  │                │
│  └──────────┘    └──────────┘      └──────────┘                │
└──────────────────────────────────────────────────────────────────┘
```

## File System Layout for Extensions

Extensions interact with multiclaude through the `~/.multiclaude/` directory:

```
~/.multiclaude/
├── state.json                # Primary extension point - READ ONLY
│                             # Contains all repos, agents, task history
│                             # Updated atomically by daemon
│
├── daemon.sock               # Socket API - READ/WRITE
│                             # Unix socket for programmatic control
│
├── daemon.log                # Daemon logs - READ ONLY
│                             # Useful for debugging, monitoring
│
├── output/<repo>/            # Agent output logs - READ ONLY
│   ├── supervisor.log
│   ├── merge-queue.log
│   └── workers/
│       └── <worker-name>.log
│
├── messages/<repo>/<agent>/  # Inter-agent messages - READ ONLY
│   └── <message-id>.json     # (Write only via socket API)
│
├── wts/<repo>/<agent>/       # Git worktrees - ADVANCED ONLY
│                             # Direct access not recommended
│
└── repos/<repo>/             # Cloned repositories - READ ONLY
```

**Key Principles:**
1. **State File is Source of Truth**: Always read from `state.json` for current state
2. **Never Write Directly**: Use socket API for any state modifications
3. **Watch, Don't Poll Aggressively**: Use `fsnotify` for state changes
4. **Logs are Append-Only**: Safe to tail, but daemon manages rotation

## Best Practices

### For Future LLMs Building Extensions

When modifying multiclaude core and asked to update extension documentation:

1. **Check State Schema Changes**
   - If `internal/state/state.go` types change, update [`STATE_FILE_INTEGRATION.md`](extending/STATE_FILE_INTEGRATION.md)
   - Run `go generate ./docs` (when verification tool is implemented)

2. **Check Event Type Changes**
   - If `internal/events/events.go` adds/removes events, update [`EVENT_HOOKS.md`](extending/EVENT_HOOKS.md)
   - Update event table with new hooks

3. **Check Socket API Changes**
   - If `internal/daemon/daemon.go` handler adds commands, update [`SOCKET_API.md`](extending/SOCKET_API.md)
   - Document new request/response formats

4. **Check Directory Layout Changes**
   - If `pkg/config/config.go` paths change, update all extension docs
   - Verify examples still work

5. **Add Examples for New Features**
   - New feature → new extension example
   - Show how external tools should interact

### For Extension Developers

1. **Start Read-Only**
   - Build monitoring/dashboards before control tools
   - Less risk, easier to debug

2. **Use Existing Patterns**
   - Copy `cmd/multiclaude-web/` structure for dashboards
   - Copy `examples/hooks/slack-notify.sh` for notifications
   - Copy socket client examples for automation

3. **Handle Daemon Restarts**
   - State file persists across restarts
   - Socket reconnection logic required
   - Event hooks re-register automatically

4. **Don't Block the Daemon**
   - Event hooks timeout after 30s
   - Socket requests should be fast
   - Heavy processing in background threads

5. **Respect Atomic Operations**
   - State file writes are atomic (temp + rename)
   - You may read during writes (you'll get old or new, never corrupt)
   - Watch for WRITE events, ignore CREATE/CHMOD/etc.

## Testing Your Extension

### Without a Real Daemon

```bash
# Create fake state for testing
cat > /tmp/test-state.json <<EOF
{
  "repos": {
    "test-repo": {
      "github_url": "https://github.com/user/repo",
      "tmux_session": "mc-test-repo",
      "agents": {
        "test-worker": {
          "type": "worker",
          "task": "Test task",
          "created_at": "2024-01-15T10:00:00Z"
        }
      },
      "task_history": []
    }
  }
}
EOF

# Point your extension at test state
multiclaude-web --state /tmp/test-state.json
```

### With Test Events

```bash
# Test your event hook
echo '{
  "type": "pr_created",
  "timestamp": "2024-01-15T10:30:00Z",
  "repo_name": "test-repo",
  "agent_name": "test-agent",
  "data": {
    "pr_number": 123,
    "title": "Test PR",
    "url": "https://github.com/user/repo/pull/123"
  }
}' | /path/to/your-hook.sh
```

### With Socket API

```go
// Use test mode to avoid real daemon
paths := config.NewTestPaths(t.TempDir())
// Build your tool with test paths
```

## Documentation Index

- **[`STATE_FILE_INTEGRATION.md`](extending/STATE_FILE_INTEGRATION.md)** - Complete state.json schema reference and examples
- **[`EVENT_HOOKS.md`](extending/EVENT_HOOKS.md)** - Event system, hook development, notification patterns
- **[`WEB_UI_DEVELOPMENT.md`](extending/WEB_UI_DEVELOPMENT.md)** - Building web dashboards and UIs
- **[`SOCKET_API.md`](extending/SOCKET_API.md)** - Socket API reference for automation tools
- **[`FORK_FEATURES_ROADMAP.md`](FORK_FEATURES_ROADMAP.md)** - Fork-only extensions (not upstream)

## Examples

Complete working examples are provided in the repository:

- **`cmd/multiclaude-web/`** - Full web dashboard implementation
- **`examples/hooks/slack-notify.sh`** - Slack notification hook
- **`internal/dashboard/`** - State reader and API handler patterns
- **`pkg/*/README.md`** - Public package usage examples

## Support and Community

When building extensions:

1. **Read existing examples first** - Most patterns are demonstrated
2. **Check CLAUDE.md** - Core architecture reference
3. **File issues** - Tag with `extension` label for visibility
4. **Contribute examples** - PR your working integrations to `examples/`

## Maintenance Strategy

This documentation is designed to stay synchronized with code:

1. **Automated Verification**: Run `go generate ./docs` to verify docs match code
2. **LLM Instructions**: CLAUDE.md directs LLMs to update extension docs when changing core
3. **CI Checks**: Extension doc verification in `.github/workflows/ci.yml`
4. **Quarterly Review**: Manual review of examples and patterns

**Last Updated:** 2024-01-23 (Initial version)
**Schema Version:** 1.0 (matches multiclaude v0.1.0)
