# State File Integration Guide

**Extension Point:** Read-only monitoring via `~/.multiclaude/state.json`

This guide documents the complete state file schema and patterns for building external tools that read multiclaude state. This is the **simplest and safest** extension point - no daemon interaction required, zero risk of breaking multiclaude operation.

## Overview

The state file (`~/.multiclaude/state.json`) is the single source of truth for:
- All tracked repositories
- All active agents (supervisor, merge-queue, workers, reviews)
- Task history and PR status
- Hook configuration
- Merge queue settings

**Key Characteristics:**
- **Atomic Writes**: Daemon writes to temp file, then atomic rename (never corrupt)
- **Read-Only for Extensions**: Never modify directly - use socket API instead
- **JSON Format**: Standard, easy to parse in any language
- **Always Available**: Persists across daemon restarts

## File Location

```bash
# Default location
~/.multiclaude/state.json

# Find it programmatically
state_path="$HOME/.multiclaude/state.json"

# Or use multiclaude config
multiclaude_dir=$(multiclaude config --paths | jq -r .state_file)
```

## Complete Schema Reference

### Root Structure

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
  "merge_queue_config": { /* MergeQueueConfig object */ }
}
```

### Agent Object

```json
{
  "type": "worker",                    // "supervisor" | "worker" | "merge-queue" | "workspace" | "review"
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
          "worktree_path": "/home/user/.multiclaude/wts/my-app/supervisor",
          "tmux_window": "0",
          "session_id": "claude-abc123",
          "pid": 12345,
          "created_at": "2024-01-15T10:00:00Z",
          "last_nudge": "2024-01-15T10:30:00Z"
        },
        "merge-queue": {
          "type": "merge-queue",
          "worktree_path": "/home/user/.multiclaude/wts/my-app/merge-queue",
          "tmux_window": "1",
          "session_id": "claude-def456",
          "pid": 12346,
          "created_at": "2024-01-15T10:00:00Z",
          "last_nudge": "2024-01-15T10:30:00Z"
        },
        "clever-fox": {
          "type": "worker",
          "worktree_path": "/home/user/.multiclaude/wts/my-app/clever-fox",
          "tmux_window": "2",
          "session_id": "claude-ghi789",
          "pid": 12347,
          "task": "Add user authentication",
          "summary": "",
          "failure_reason": "",
          "created_at": "2024-01-15T10:15:00Z",
          "last_nudge": "2024-01-15T10:30:00Z",
          "ready_for_cleanup": false
        }
      },
      "task_history": [
        {
          "name": "brave-lion",
          "task": "Fix login bug",
          "branch": "multiclaude/brave-lion",
          "pr_url": "https://github.com/user/my-app/pull/41",
          "pr_number": 41,
          "status": "merged",
          "summary": "Fixed race condition in session validation",
          "failure_reason": "",
          "created_at": "2024-01-14T15:00:00Z",
          "completed_at": "2024-01-14T16:30:00Z"
        }
      ],
      "merge_queue_config": {
        "enabled": true,
        "track_mode": "all"
      }
    }
  },
  "current_repo": "my-app",
  "hooks": {
    "on_event": "",
    "on_pr_created": "/usr/local/bin/notify-slack.sh",
    "on_ci_failed": "/usr/local/bin/alert-pagerduty.sh"
  }
}
```

## Reading the State File

### Basic Read (Any Language)

```bash
# Bash
state=$(cat ~/.multiclaude/state.json)
repo_count=$(echo "$state" | jq '.repos | length')

# Python
import json
with open(os.path.expanduser('~/.multiclaude/state.json')) as f:
    state = json.load(f)

# Node.js
const state = JSON.parse(fs.readFileSync(
    path.join(os.homedir(), '.multiclaude/state.json'),
    'utf8'
));

# Go
data, _ := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".multiclaude/state.json"))
var state State
json.Unmarshal(data, &state)
```

### Watching for Changes

The state file is updated frequently (every agent action, every status change). Use file watching instead of polling.

#### Go (fsnotify)

```go
package main

import (
    "encoding/json"
    "log"
    "os"

    "github.com/fsnotify/fsnotify"
    "github.com/dlorenc/multiclaude/internal/state"
)

func main() {
    watcher, _ := fsnotify.NewWatcher()
    defer watcher.Close()

    statePath := os.ExpandEnv("$HOME/.multiclaude/state.json")
    watcher.Add(statePath)

    for {
        select {
        case event := <-watcher.Events:
            if event.Op&fsnotify.Write == fsnotify.Write {
                // Re-read state
                data, _ := os.ReadFile(statePath)
                var s state.State
                json.Unmarshal(data, &s)

                // Do something with updated state
                processState(&s)
            }
        case err := <-watcher.Errors:
            log.Println("Error:", err)
        }
    }
}
```

#### Python (watchdog)

```python
from watchdog.observers import Observer
from watchdog.events import FileSystemEventHandler
import json
import os

class StateFileHandler(FileSystemEventHandler):
    def on_modified(self, event):
        if event.src_path.endswith('state.json'):
            with open(event.src_path) as f:
                state = json.load(f)
                process_state(state)

observer = Observer()
observer.schedule(
    StateFileHandler(),
    os.path.expanduser('~/.multiclaude'),
    recursive=False
)
observer.start()
```

#### Node.js (chokidar)

```javascript
const chokidar = require('chokidar');
const fs = require('fs');
const path = require('path');

const statePath = path.join(os.homedir(), '.multiclaude/state.json');

chokidar.watch(statePath).on('change', (path) => {
    const state = JSON.parse(fs.readFileSync(path, 'utf8'));
    processState(state);
});
```

## Common Queries

### Get All Active Workers

```javascript
// JavaScript
const workers = Object.entries(state.repos)
    .flatMap(([repoName, repo]) =>
        Object.entries(repo.agents)
            .filter(([_, agent]) => agent.type === 'worker' && agent.pid > 0)
            .map(([name, agent]) => ({
                repo: repoName,
                name: name,
                task: agent.task,
                created: new Date(agent.created_at)
            }))
    );
```

```python
# Python
workers = [
    {
        'repo': repo_name,
        'name': agent_name,
        'task': agent['task'],
        'created': agent['created_at']
    }
    for repo_name, repo in state['repos'].items()
    for agent_name, agent in repo['agents'].items()
    if agent['type'] == 'worker' and agent.get('pid', 0) > 0
]
```

```bash
# Bash/jq
workers=$(cat ~/.multiclaude/state.json | jq -r '
    .repos | to_entries[] |
    .value.agents | to_entries[] |
    select(.value.type == "worker" and .value.pid > 0) |
    {repo: .key, name: .key, task: .value.task}
')
```

### Get Recent Task History

```python
# Python - Get last 10 completed tasks across all repos
from datetime import datetime

tasks = []
for repo_name, repo in state['repos'].items():
    for entry in repo.get('task_history', []):
        tasks.append({
            'repo': repo_name,
            **entry
        })

# Sort by completion time, most recent first
tasks.sort(key=lambda x: x.get('completed_at', ''), reverse=True)
recent_tasks = tasks[:10]
```

### Calculate Success Rate

```javascript
// JavaScript
function calculateSuccessRate(state, repoName) {
    const history = state.repos[repoName]?.task_history || [];
    const total = history.length;
    const merged = history.filter(t => t.status === 'merged').length;
    return total > 0 ? (merged / total * 100).toFixed(1) : 0;
}
```

### Find Stuck Workers

```python
# Python - Find workers idle for > 30 minutes
from datetime import datetime, timedelta

now = datetime.utcnow()
stuck_threshold = timedelta(minutes=30)

stuck_workers = []
for repo_name, repo in state['repos'].items():
    for agent_name, agent in repo['agents'].items():
        if agent['type'] != 'worker' or agent.get('pid', 0) == 0:
            continue

        last_nudge = datetime.fromisoformat(
            agent.get('last_nudge', agent['created_at']).replace('Z', '+00:00')
        )
        idle_time = now - last_nudge

        if idle_time > stuck_threshold:
            stuck_workers.append({
                'repo': repo_name,
                'name': agent_name,
                'task': agent.get('task'),
                'idle_minutes': idle_time.total_seconds() / 60
            })
```

### Get PR Status Summary

```bash
# Bash/jq - Count PRs by status
cat ~/.multiclaude/state.json | jq -r '
    .repos[].task_history[] | .status
' | sort | uniq -c

# Output:
#   5 merged
#   2 open
#   1 closed
```

## Building a State Reader Library

### Go Example

```go
package multiclaude

import (
    "encoding/json"
    "os"
    "path/filepath"
    "sync"

    "github.com/dlorenc/multiclaude/internal/state"
    "github.com/fsnotify/fsnotify"
)

type StateReader struct {
    path     string
    mu       sync.RWMutex
    state    *state.State
    onChange func(*state.State)
}

func NewStateReader(path string) (*StateReader, error) {
    r := &StateReader{path: path}
    if err := r.reload(); err != nil {
        return nil, err
    }
    return r, nil
}

func (r *StateReader) reload() error {
    data, err := os.ReadFile(r.path)
    if err != nil {
        return err
    }

    var s state.State
    if err := json.Unmarshal(data, &s); err != nil {
        return err
    }

    r.mu.Lock()
    r.state = &s
    r.mu.Unlock()

    return nil
}

func (r *StateReader) Get() *state.State {
    r.mu.RLock()
    defer r.mu.RUnlock()
    return r.state
}

func (r *StateReader) Watch(onChange func(*state.State)) error {
    r.onChange = onChange

    watcher, err := fsnotify.NewWatcher()
    if err != nil {
        return err
    }

    if err := watcher.Add(r.path); err != nil {
        return err
    }

    go func() {
        for {
            select {
            case event := <-watcher.Events:
                if event.Op&fsnotify.Write == fsnotify.Write {
                    r.reload()
                    if r.onChange != nil {
                        r.onChange(r.Get())
                    }
                }
            case <-watcher.Errors:
                // Handle error
            }
        }
    }()

    return nil
}
```

Usage:
```go
reader, _ := multiclaude.NewStateReader(
    filepath.Join(os.Getenv("HOME"), ".multiclaude/state.json")
)

reader.Watch(func(s *state.State) {
    fmt.Printf("State updated: %d repos\n", len(s.Repos))
})

// Query current state
state := reader.Get()
for name, repo := range state.Repos {
    fmt.Printf("Repo: %s (%d agents)\n", name, len(repo.Agents))
}
```

## Performance Considerations

### Read Performance

- **File Size**: Typically 10-100KB, grows with task history
- **Parse Time**: <1ms for typical state files
- **Watch Overhead**: Minimal with fsnotify/inotify

### Update Frequency

The daemon writes to state.json:
- Every agent start/stop
- Every task assignment/completion
- Every status update (every 2 minutes during health checks)
- Every PR created/merged

**Recommendation:** Use file watching, not polling. Polling < 1s is wasteful.

### Handling Rapid Updates

During busy periods (many agents, frequent changes), you may see multiple updates per second.

**Debouncing Pattern:**

```javascript
let updateTimeout;
watcher.on('change', () => {
    clearTimeout(updateTimeout);
    updateTimeout = setTimeout(() => {
        const state = JSON.parse(fs.readFileSync(statePath, 'utf8'));
        processState(state);  // Your logic here
    }, 100);  // Wait 100ms for update burst to finish
});
```

## Atomic Reads

The daemon uses atomic writes (write to temp, rename), so you'll never read a corrupt file. However:

1. **During a write**, you might read the old state
2. **After the rename**, you'll read the new state
3. **Never** will you read a partial write

This means: **No locking required** - just read whenever you want.

## Schema Evolution

### Version Compatibility

Currently, the state file has no explicit version field. If the schema changes:

1. **Backward-compatible changes** (new fields): Your code ignores unknown fields
2. **Breaking changes** (removed/renamed fields): Will be announced in release notes

**Future-proofing your code:**

```javascript
// Defensive access
const agentTask = agent.task || agent.description || 'Unknown';
const status = entry.status || 'unknown';
```

### Deprecated Fields

The schema has evolved over time. Some historical notes:

- `merge_queue_config` was added later - older state files won't have it
- If missing, assume `DefaultMergeQueueConfig()`: `{enabled: true, track_mode: "all"}`

## Troubleshooting

### State File Missing

```bash
# Check if multiclaude is initialized
if [ ! -f ~/.multiclaude/state.json ]; then
    echo "Error: multiclaude not initialized"
    echo "Run: multiclaude init <github-url>"
    exit 1
fi
```

### State File Permissions

```bash
# State file should be user-readable
ls -l ~/.multiclaude/state.json
# -rw-r--r-- 1 user user ...

# If not readable, check daemon logs
tail ~/.multiclaude/daemon.log
```

### Parse Errors

```python
import json
try:
    with open(state_path) as f:
        state = json.load(f)
except json.JSONDecodeError as e:
    # This should never happen due to atomic writes
    # If it does, the state file is corrupted
    print(f"Error parsing state: {e}")
    print("Check daemon logs and consider restarting daemon")
```

### Stale Data

If state seems stale (agents shown as running but they're not):

```bash
# Trigger daemon health check
multiclaude cleanup --dry-run

# Or force state refresh
pkill -USR1 multiclaude  # Send signal to daemon (future feature)
```

## Real-World Examples

### Example 1: Prometheus Exporter

Export multiclaude metrics to Prometheus:

```python
from prometheus_client import start_http_server, Gauge
import json, time, os

# Define metrics
agents_gauge = Gauge('multiclaude_agents_total', 'Number of agents', ['repo', 'type'])
tasks_counter = Gauge('multiclaude_tasks_total', 'Completed tasks', ['repo', 'status'])

def update_metrics():
    with open(os.path.expanduser('~/.multiclaude/state.json')) as f:
        state = json.load(f)

    # Update agent counts
    for repo_name, repo in state['repos'].items():
        agent_types = {}
        for agent in repo['agents'].values():
            t = agent['type']
            agent_types[t] = agent_types.get(t, 0) + 1

        for agent_type, count in agent_types.items():
            agents_gauge.labels(repo=repo_name, type=agent_type).set(count)

    # Update task history counts
    for repo_name, repo in state['repos'].items():
        status_counts = {}
        for entry in repo.get('task_history', []):
            s = entry['status']
            status_counts[s] = status_counts.get(s, 0) + 1

        for status, count in status_counts.items():
            tasks_counter.labels(repo=repo_name, status=status).set(count)

if __name__ == '__main__':
    start_http_server(9090)
    while True:
        update_metrics()
        time.sleep(15)  # Update every 15 seconds
```

### Example 2: CLI Status Monitor

Simple CLI tool to show current status:

```bash
#!/bin/bash
# multiclaude-status.sh - Show active workers

state=$(cat ~/.multiclaude/state.json)

echo "=== Active Workers ==="
echo "$state" | jq -r '
    .repos | to_entries[] |
    .value.agents | to_entries[] |
    select(.value.type == "worker" and .value.pid > 0) |
    "\(.key): \(.value.task)"
'

echo ""
echo "=== Recent Completions ==="
echo "$state" | jq -r '
    .repos[].task_history[] |
    select(.status == "merged") |
    "\(.name): \(.summary)"
' | tail -5
```

### Example 3: Web Dashboard API

> **Note:** The reference implementation (`internal/dashboard/`, `cmd/multiclaude-web`) does not exist.
> See WEB_UI_DEVELOPMENT.md for design patterns if building a dashboard in a fork.

A web dashboard would typically include:
- REST endpoints for repos, agents, history
- Server-Sent Events for live updates
- State watching with fsnotify

## Related Documentation

- **[`EXTENSIBILITY.md`](../EXTENSIBILITY.md)** - Overview of all extension points
- **[`WEB_UI_DEVELOPMENT.md`](WEB_UI_DEVELOPMENT.md)** - Building dashboards with state reader
- **[`SOCKET_API.md`](SOCKET_API.md)** - For writing state (not just reading)
- `internal/state/state.go` - Canonical Go schema definition

## Contributing

When proposing schema changes:

1. Update this document first
2. Update `internal/state/state.go`
3. Verify backward compatibility
4. Add migration notes to release notes
5. Update all code examples in this doc
