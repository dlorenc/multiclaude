# State File Integration (Read-Only)

<!-- state-struct: State repos current_repo -->
<!-- state-struct: Repository github_url tmux_session agents task_history merge_queue_config pr_shepherd_config fork_config target_branch -->
<!-- state-struct: Agent type worktree_path tmux_window session_id pid task summary failure_reason created_at last_nudge ready_for_cleanup -->
<!-- state-struct: TaskHistoryEntry name task branch pr_url pr_number status summary failure_reason created_at completed_at -->
<!-- state-struct: MergeQueueConfig enabled track_mode -->
<!-- state-struct: PRShepherdConfig enabled track_mode -->
<!-- state-struct: ForkConfig is_fork upstream_url upstream_owner upstream_repo force_fork_mode -->

The daemon persists state to `~/.multiclaude/state.json` and writes it atomically. This file is safe for external tools to **read only**. Write access belongs to the daemon.

## Schema (from `internal/state/state.go`)
```json
{
  "repos": {
    "<repo-name>": { /* Repository object */ }
  },
  "current_repo": "my-repo",  // Optional: default repository
  "hooks": { /* HookConfig object */ }
}
```

### Repository Object

```json
{
  "github_url": "https://github.com/user/repo",
  "tmux_session": "mc-my-repo",
  "agents": {
    "<agent-name>": { /* Agent object */ }
  },
  "task_history": [ /* TaskHistoryEntry objects */ ],
  "merge_queue_config": { /* MergeQueueConfig object */ },
  "pr_shepherd_config": { /* PRShepherdConfig object */ },
  "fork_config": { /* ForkConfig object */ },
  "target_branch": "main"
}
```

### Agent Object

```json
{
  "type": "worker",                    // "supervisor" | "worker" | "merge-queue" | "workspace" | "review" | "pr-shepherd"
  "worktree_path": "/path/to/worktree",
  "tmux_window": "0",                  // Window index in tmux session
  "session_id": "claude-session-id",
  "pid": 12345,                        // Process ID (0 if not running)
  "task": "Implement feature X",       // Only for workers
  "summary": "Added auth module",      // Only for workers (completion summary)
  "failure_reason": "Tests failed",    // Only for workers (if task failed)
  "created_at": "2024-01-15T10:30:00Z",
  "last_nudge": "2024-01-15T10:35:00Z",
  "ready_for_cleanup": false           // Only for workers (signals completion)
}
```

**Agent Types:**
- `supervisor`: Main orchestrator for the repository
- `merge-queue`: Monitors and merges approved PRs
- `worker`: Executes specific tasks
- `workspace`: Interactive workspace agent
- `review`: Reviews a specific PR
- `pr-shepherd`: Monitors PRs in fork mode
- `generic-persistent`: Custom persistent agents

### TaskHistoryEntry Object

```json
{
  "name": "clever-fox",                // Worker name
  "task": "Add user authentication",   // Task description
  "branch": "multiclaude/clever-fox",  // Git branch
  "pr_url": "https://github.com/user/repo/pull/42",
  "pr_number": 42,
  "status": "merged",                  // See status values below
  "summary": "Implemented JWT-based auth with refresh tokens",
  "failure_reason": "",                // Populated if status is "failed"
  "created_at": "2024-01-15T10:00:00Z",
  "completed_at": "2024-01-15T11:30:00Z"
}
```

**Status Values:**
- `open`: PR created, not yet merged or closed
- `merged`: PR was merged successfully
- `closed`: PR was closed without merging
- `no-pr`: Task completed but no PR was created
- `failed`: Task failed (see `failure_reason`)
- `unknown`: Status couldn't be determined

### MergeQueueConfig Object

```json
{
  "enabled": true,                     // Whether merge-queue agent runs
  "track_mode": "all"                  // "all" | "author" | "assigned"
}
```

**Track Modes:**
- `all`: Monitor all PRs in the repository
- `author`: Only PRs where multiclaude user is the author
- `assigned`: Only PRs where multiclaude user is assigned

### PRShepherdConfig Object

```json
{
  "enabled": true,                     // Whether pr-shepherd agent runs
  "track_mode": "author"               // "all" | "author" | "assigned"
}
```

### ForkConfig Object

```json
{
  "is_fork": true,
  "upstream_url": "https://github.com/upstream/repo",
  "upstream_owner": "upstream",
  "upstream_repo": "repo",
  "force_fork_mode": false
}
```

### HookConfig Object

```json
{
  "on_event": "/usr/local/bin/notify.sh",          // Catch-all hook
  "on_pr_created": "/usr/local/bin/slack-pr.sh",
  "on_agent_idle": "",
  "on_merge_complete": "",
  "on_agent_started": "",
  "on_agent_stopped": "",
  "on_task_assigned": "",
  "on_ci_failed": "/usr/local/bin/alert-ci.sh",
  "on_worker_stuck": "",
  "on_message_sent": ""
}
```

## Example State File

```json
{
  "repos": {
    "my-app": {
      "github_url": "https://github.com/user/my-app",
      "tmux_session": "mc-my-app",
      "agents": {
        "supervisor": {
          "type": "supervisor",
          "pid": 12345,
          "created_at": "2025-01-01T00:00:00Z",
          "last_nudge": "2025-01-01T00:00:00Z",
          "ready_for_cleanup": false
        }
      },
      "task_history": [
        {
          "name": "clever-fox",
          "task": "Add auth",
          "branch": "work/clever-fox",
          "pr_url": "https://github.com/user/my-app/pull/42",
          "pr_number": 42,
          "status": "merged",
          "created_at": "2025-01-01T00:00:00Z",
          "completed_at": "2025-01-02T00:00:00Z"
        }
      ],
      "merge_queue_config": {
        "enabled": true,
        "track_mode": "all"
      },
      "pr_shepherd_config": {
        "enabled": true,
        "track_mode": "author"
      },
      "fork_config": {
        "is_fork": true,
        "upstream_url": "https://github.com/original/my-app",
        "upstream_owner": "original",
        "upstream_repo": "my-app",
        "force_fork_mode": false
      },
      "target_branch": "main"
    }
  },
  "current_repo": "my-app"
}
```

## Reading the state file

### Go
```go
package main

import (
    "encoding/json"
    "fmt"
    "os"

    "github.com/dlorenc/multiclaude/internal/state"
)

func main() {
    data, err := os.ReadFile("/home/user/.multiclaude/state.json")
    if err != nil {
        panic(err)
    }

    var st state.State
    if err := json.Unmarshal(data, &st); err != nil {
        panic(err)
    }

    for name := range st.Repos {
        fmt.Println("repo", name)
    }
}
```

### Python
```python
import json
from pathlib import Path

state_path = Path.home() / ".multiclaude" / "state.json"
state = json.loads(state_path.read_text())
for repo, data in state.get("repos", {}).items():
    print("repo", repo, "agents", list(data.get("agents", {}).keys()))
```

## Updating this doc
- Keep the `state-struct` markers above in sync with `internal/state/state.go`.
- Do **not** add fields here unless they exist in the structs.
- Run `go run ./cmd/verify-docs` after schema changes; CI will block if docs drift.