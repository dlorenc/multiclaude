# Socket API (Current Implementation)

<!-- socket-commands:
ping
status
stop
list_repos
add_repo
remove_repo
add_agent
remove_agent
list_agents
complete_agent
restart_agent
trigger_cleanup
repair_state
get_repo_config
update_repo_config
set_current_repo
get_current_repo
clear_current_repo
route_messages
task_history
spawn_agent
-->

The socket API is the only write-capable extension surface in multiclaude today. It is implemented in `internal/daemon/daemon.go` (`handleRequest`). This document tracks only the commands that exist in the code. Anything not listed here is **not implemented**.

## Protocol
- Transport: Unix domain socket at `~/.multiclaude/daemon.sock`
- Request type: JSON object `{ "command": "<name>", "args": { ... } }`
- Response type: `{ "success": true|false, "data": any, "error": string }`
- Client helper: `internal/socket.Client`

## Command Reference (source of truth)
Each command below matches a `case` in `handleRequest`.

| Command | Description | Args |
|---------|-------------|------|
| `ping` | Health check | none |
| `status` | Daemon status summary | none |
| `stop` | Stop the daemon | none |
| `list_repos` | List tracked repos (optionally rich info) | `rich` (bool, optional) |
| `add_repo` | Track a new repo | `path` (string) |
| `remove_repo` | Stop tracking a repo | `name` (string) |
| `add_agent` | Register an agent in state | `repo`, `name`, `type`, `worktree_path`, `tmux_window`, `session_id`, `pid` |
| `remove_agent` | Remove agent from state | `repo`, `name` |
| `list_agents` | List agents for a repo | `repo` |
| `complete_agent` | Mark agent ready for cleanup | `repo`, `name`, `summary`, `failure_reason` |
| `restart_agent` | Restart a persistent agent | `repo`, `name` |
| `trigger_cleanup` | Force cleanup cycle | none |
| `repair_state` | Run state repair routine | none |
| `get_repo_config` | Get merge-queue / pr-shepherd config | `repo` |
| `update_repo_config` | Update repo config | `repo`, `config` (JSON object) |
| `set_current_repo` | Persist current repo selection | `repo` |
| `get_current_repo` | Read current repo selection | none |
| `clear_current_repo` | Clear current repo selection | none |
| `route_messages` | Force message routing cycle | none |
| `task_history` | Return task history for a repo | `repo` |
| `spawn_agent` | Create a new agent worktree | `repo`, `type`, `task`, `name` (optional) |

## Minimal client examples

### Go
```go
package main

import (
    "fmt"

    "github.com/dlorenc/multiclaude/internal/socket"
)

func main() {
    client := socket.NewClient("/home/user/.multiclaude/daemon.sock")
    resp, err := client.Send(socket.Request{Command: "ping"})
    if err != nil {
        panic(err)
    }
    fmt.Printf("success=%v data=%v\n", resp.Success, resp.Data)
}
```

### Python
```python
import json
import socket

sock_path = "/home/user/.multiclaude/daemon.sock"
req = {"command": "status", "args": {}}

with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as s:
    s.connect(sock_path)
    s.sendall(json.dumps(req).encode("utf-8"))
    raw = s.recv(8192)
    resp = json.loads(raw.decode("utf-8"))
    print(resp)
```

### Bash

```bash
#!/bin/bash
# multiclaude-api.sh - Socket API client in bash

SOCK="$HOME/.multiclaude/daemon.sock"

multiclaude_api() {
    local command="$1"
    shift
    local args="$@"

    # Build request JSON
    local request
    if [ -n "$args" ]; then
        request=$(jq -n --arg cmd "$command" --argjson args "$args" \
            '{command: $cmd, args: $args}')
    else
        request=$(jq -n --arg cmd "$command" '{command: $cmd}')
    fi

    # Send to socket and parse response
    echo "$request" | nc -U "$SOCK" | jq -r .
}

# Usage
multiclaude_api "status"
multiclaude_api "list_repos"
```

### Node.js

```javascript
const net = require('net');
const os = require('os');
const path = require('path');

class MulticlaudeClient {
    constructor(sockPath = path.join(os.homedir(), '.multiclaude/daemon.sock')) {
        this.sockPath = sockPath;
    }

    async send(command, args = null) {
        return new Promise((resolve, reject) => {
            const client = net.createConnection(this.sockPath);

            // Build request
            const request = { command };
            if (args) request.args = args;

            client.on('connect', () => {
                client.write(JSON.stringify(request) + '\n');
            });

            let data = '';
            client.on('data', (chunk) => {
                data += chunk.toString();
                try {
                    const response = JSON.parse(data);
                    client.end();

                    if (!response.success) {
                        reject(new Error(response.error));
                    } else {
                        resolve(response.data);
                    }
                } catch (e) {
                    // Incomplete JSON, wait for more data
                }
            });

            client.on('error', reject);
        });
    }
}

// Usage
(async () => {
    const client = new MulticlaudeClient();
    const status = await client.send('status');
    console.log('Daemon status:', status);
})();
```

## Command Reference

### Daemon Management

#### ping

**Description:** Check if daemon is alive

**Request:**
```json
{
  "command": "ping"
}
```

**Response:**
```json
{
  "success": true,
  "data": "pong"
}
```

#### status

**Description:** Get daemon status

**Request:**
```json
{
  "command": "status"
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "running": true,
    "pid": 12345,
    "repos": 2,
    "agents": 5,
    "socket_path": "/home/user/.multiclaude/daemon.sock"
  }
}
```

#### stop

**Description:** Stop the daemon gracefully

**Request:**
```json
{
  "command": "stop"
}
```

**Response:**
```json
{
  "success": true,
  "data": "Daemon stopping"
}
```

**Note:** Daemon will stop asynchronously after responding.

### Repository Management

#### list_repos

**Description:** List all tracked repositories

**Request:**
```json
{
  "command": "list_repos"
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "repos": ["my-app", "backend-api"]
  }
}
```

#### add_repo

**Description:** Add a new repository (equivalent to `multiclaude init`)

**Request:**
```json
{
  "command": "add_repo",
  "args": {
    "name": "my-app",
    "github_url": "https://github.com/user/my-app",
    "merge_queue_enabled": true,
    "merge_queue_track_mode": "all"
  }
}
```

**Args:**
- `name` (string, required): Repository name
- `github_url` (string, required): GitHub URL
- `merge_queue_enabled` (boolean, optional): Enable merge queue (default: true)
- `merge_queue_track_mode` (string, optional): Track mode: "all", "author", "assigned" (default: "all")

**Response:**
```json
{
  "success": true,
  "data": "Repository 'my-app' initialized"
}
```

#### remove_repo

**Description:** Remove a repository

**Request:**
```json
{
  "command": "remove_repo",
  "args": {
    "name": "my-app"
  }
}
```

**Response:**
```json
{
  "success": true,
  "data": "Repository 'my-app' removed"
}
```

#### get_repo_config

**Description:** Get repository configuration

**Request:**
```json
{
  "command": "get_repo_config",
  "args": {
    "name": "my-app"
  }
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "merge_queue_enabled": true,
    "merge_queue_track_mode": "all"
  }
}
```

#### update_repo_config

**Description:** Update repository configuration

**Request:**
```json
{
  "command": "update_repo_config",
  "args": {
    "name": "my-app",
    "merge_queue_enabled": false,
    "merge_queue_track_mode": "author"
  }
}
```

**Response:**
```json
{
  "success": true,
  "data": "Repository configuration updated"
}
```

#### set_current_repo

**Description:** Set the default repository

**Request:**
```json
{
  "command": "set_current_repo",
  "args": {
    "name": "my-app"
  }
}
```

**Response:**
```json
{
  "success": true,
  "data": "Current repository set to 'my-app'"
}
```

#### get_current_repo

**Description:** Get the default repository name

**Request:**
```json
{
  "command": "get_current_repo"
}
```

**Response:**
```json
{
  "success": true,
  "data": "my-app"
}
```

#### clear_current_repo

**Description:** Clear the default repository

**Request:**
```json
{
  "command": "clear_current_repo"
}
```

**Response:**
```json
{
  "success": true,
  "data": "Current repository cleared"
}
```

### Agent Management

#### list_agents

**Description:** List all agents for a repository

**Request:**
```json
{
  "command": "list_agents",
  "args": {
    "repo": "my-app"
  }
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "agents": {
      "supervisor": {
        "type": "supervisor",
        "pid": 12345,
        "created_at": "2024-01-15T10:00:00Z"
      },
      "clever-fox": {
        "type": "worker",
        "task": "Add authentication",
        "pid": 12346,
        "created_at": "2024-01-15T10:15:00Z"
      }
    }
  }
}
```

#### add_agent

**Description:** Add/spawn a new agent

**Request:**
```json
{
  "command": "add_agent",
  "args": {
    "repo": "my-app",
    "name": "clever-fox",
    "type": "worker",
    "task": "Add user authentication"
  }
}
```

**Args:**
- `repo` (string, required): Repository name
- `name` (string, required): Agent name
- `type` (string, required): Agent type: "supervisor", "worker", "merge-queue", "workspace", "review"
- `task` (string, optional): Task description (for workers)

**Response:**
```json
{
  "success": true,
  "data": "Agent 'clever-fox' created"
}
```

#### remove_agent

**Description:** Remove/kill an agent

**Request:**
```json
{
  "command": "remove_agent",
  "args": {
    "repo": "my-app",
    "name": "clever-fox"
  }
}
```

**Response:**
```json
{
  "success": true,
  "data": "Agent 'clever-fox' removed"
}
```

#### complete_agent

**Description:** Mark a worker as completed (called by workers themselves)

**Request:**
```json
{
  "command": "complete_agent",
  "args": {
    "repo": "my-app",
    "name": "clever-fox",
    "summary": "Added JWT authentication with refresh tokens",
    "failure_reason": ""
  }
}
```

**Args:**
- `repo` (string, required): Repository name
- `name` (string, required): Agent name
- `summary` (string, optional): Completion summary
- `failure_reason` (string, optional): Failure reason (if task failed)

**Response:**
```json
{
  "success": true,
  "data": "Agent marked for cleanup"
}
```

#### restart_agent

**Description:** Restart a crashed or stopped agent

**Request:**
```json
{
  "command": "restart_agent",
  "args": {
    "repo": "my-app",
    "name": "supervisor"
  }
}
```

**Response:**
```json
{
  "success": true,
  "data": "Agent 'supervisor' restarted"
}
```

### Task History

#### task_history

**Description:** Get task history for a repository

**Request:**
```json
{
  "command": "task_history",
  "args": {
    "repo": "my-app",
    "limit": 10
  }
}
```

**Args:**
- `repo` (string, required): Repository name
- `limit` (integer, optional): Max entries to return (0 = all)

**Response:**
```json
{
  "success": true,
  "data": {
    "history": [
      {
        "name": "brave-lion",
        "task": "Fix login bug",
        "status": "merged",
        "pr_url": "https://github.com/user/my-app/pull/42",
        "pr_number": 42,
        "created_at": "2024-01-14T10:00:00Z",
        "completed_at": "2024-01-14T11:00:00Z"
      }
    ]
  }
}
```

### Maintenance

#### trigger_cleanup

**Description:** Trigger immediate cleanup of dead agents

**Request:**
```json
{
  "command": "trigger_cleanup"
}
```

**Response:**
```json
{
  "success": true,
  "data": "Cleanup triggered"
}
```

#### repair_state

**Description:** Repair inconsistent state (equivalent to `multiclaude repair`)

**Request:**
```json
{
  "command": "repair_state"
}
```

**Response:**
```json
{
  "success": true,
  "data": "State repaired"
}
```

#### route_messages

**Description:** Trigger immediate message routing (normally runs every 2 minutes)

**Request:**
```json
{
  "command": "route_messages"
}
```

**Response:**
```json
{
  "success": true,
  "data": "Message routing triggered"
}
```

## Error Handling

### Connection Errors

```python
try:
    response = client.send("status")
except FileNotFoundError:
    print("Error: Daemon not running")
    print("Start with: multiclaude start")
except PermissionError:
    print("Error: Socket permission denied")
```

### Command Errors

```python
response = client.send("add_repo", {"name": "test"})  # Missing github_url

# Response:
# {
#   "success": false,
#   "error": "missing required argument: github_url"
# }
```

### Unknown Commands

```python
response = client.send("invalid_command")

# Response:
# {
#   "success": false,
#   "error": "unknown command: \"invalid_command\""
# }
```

## Common Patterns

### Check If Daemon Is Running

```python
def is_daemon_running():
    try:
        client = MulticlaudeClient()
        client.send("ping")
        return True
    except:
        return False
```

### Spawn Worker

```python
def spawn_worker(repo, task):
    client = MulticlaudeClient()

    # Generate random worker name (you could use internal/names package)
    import random
    adjectives = ["clever", "brave", "swift", "keen"]
    animals = ["fox", "lion", "eagle", "wolf"]
    name = f"{random.choice(adjectives)}-{random.choice(animals)}"

    client.send("add_agent", {
        "repo": repo,
        "name": name,
        "type": "worker",
        "task": task
    })

    return name
```

### Wait for Worker Completion

```python
import time

def wait_for_completion(repo, worker_name, timeout=3600):
    client = MulticlaudeClient()
    start = time.time()

    while time.time() - start < timeout:
        # Check if worker still exists
        agents = client.send("list_agents", {"repo": repo})['agents']

        if worker_name not in agents:
            # Worker completed
            return True

        agent = agents[worker_name]
        if agent.get('ready_for_cleanup'):
            return True

        time.sleep(30)  # Poll every 30 seconds

    return False
```

### Get Active Workers

```python
def get_active_workers(repo):
    client = MulticlaudeClient()
    agents = client.send("list_agents", {"repo": repo})['agents']

    return [
        {
            'name': name,
            'task': agent['task'],
            'created': agent['created_at']
        }
        for name, agent in agents.items()
        if agent['type'] == 'worker' and agent.get('pid', 0) > 0
    ]
```

## Building a Custom CLI

```python
#!/usr/bin/env python3
# myclaude - Custom CLI wrapping socket API

import sys
from multiclaude_client import MulticlaudeClient

def main():
    if len(sys.argv) < 2:
        print("Usage: myclaude <command> [args...]")
        sys.exit(1)

    command = sys.argv[1]
    client = MulticlaudeClient()

    try:
        if command == "status":
            status = client.send("status")
            print(f"Daemon PID: {status['pid']}")
            print(f"Repos: {status['repos']}")
            print(f"Agents: {status['agents']}")

        elif command == "spawn":
            if len(sys.argv) < 4:
                print("Usage: myclaude spawn <repo> <task>")
                sys.exit(1)

            repo = sys.argv[2]
            task = ' '.join(sys.argv[3:])
            name = spawn_worker(repo, task)
            print(f"Spawned worker: {name}")

        elif command == "workers":
            repo = sys.argv[2] if len(sys.argv) > 2 else None
            if not repo:
                print("Usage: myclaude workers <repo>")
                sys.exit(1)

            workers = get_active_workers(repo)
            for w in workers:
                print(f"{w['name']}: {w['task']}")

        else:
            print(f"Unknown command: {command}")
            sys.exit(1)

    except Exception as e:
        print(f"Error: {e}")
        sys.exit(1)

if __name__ == '__main__':
    main()
```

## Integration Examples

### CI/CD Pipeline

```yaml
# .github/workflows/multiclaude.yml
name: Multiclaude Task

on: [push]

jobs:
  spawn-task:
    runs-on: self-hosted  # Requires multiclaude on runner
    steps:
      - name: Spawn multiclaude worker
        run: |
          python3 <<EOF
          from multiclaude_client import MulticlaudeClient
          client = MulticlaudeClient()
          client.send("add_agent", {
              "repo": "my-app",
              "name": "ci-worker",
              "type": "worker",
              "task": "Review PR ${{ github.event.pull_request.number }}"
          })
          EOF
```

### Slack Bot

```python
from slack_bolt import App
from multiclaude_client import MulticlaudeClient

app = App(token=os.environ["SLACK_TOKEN"])
client = MulticlaudeClient()

@app.command("/spawn")
def spawn_command(ack, command):
    ack()

    task = command['text']
    name = spawn_worker("my-app", task)

    app.client.chat_postMessage(
        channel=command['channel_id'],
        text=f"Spawned worker {name} for task: {task}"
    )

app.start(port=3000)
```

### Monitoring Dashboard Backend

```javascript
// Express.js API wrapping socket API

const express = require('express');
const MulticlaudeClient = require('./multiclaude-client');

const app = express();
const client = new MulticlaudeClient();

app.get('/api/status', async (req, res) => {
    const status = await client.send('status');
    res.json(status);
});

app.get('/api/repos', async (req, res) => {
    const data = await client.send('list_repos');
    res.json(data.repos);
});

app.post('/api/spawn', async (req, res) => {
    const { repo, task } = req.body;
    await client.send('add_agent', {
        repo,
        name: generateName(),
        type: 'worker',
        task
    });
    res.json({ success: true });
});

app.listen(3000);
```

## Performance

- **Latency**: <1ms for simple commands (ping, status)
- **Throughput**: Hundreds of requests/second
- **Concurrency**: Daemon handles requests in parallel via goroutines
- **Blocking**: Long-running operations return immediately (async execution)

## Security

### Socket Permissions

```bash
# Socket is user-only by default
ls -l ~/.multiclaude/daemon.sock
# srw------- 1 user user 0 ... daemon.sock
```

**Recommendation:** Don't change socket permissions. Only the owning user should access.

### Input Validation

The daemon validates all inputs:
- Repository names: alphanumeric + hyphens
- Agent names: alphanumeric + hyphens
- File paths: checked for existence
- URLs: basic validation

**Client-side:** Still validate inputs before sending to prevent API errors.

### Command Injection

Daemon never executes shell commands with user input. Safe patterns:
- Agent names → tmux window names (sanitized)
- Tasks → embedded in prompts (not executed)
- URLs → passed to `git clone` (validated)

## Troubleshooting

### Socket Not Found

```bash
# Check if daemon is running
ps aux | grep multiclaude

# If not running
multiclaude start
```

### Permission Denied

```bash
# Check socket permissions
ls -l ~/.multiclaude/daemon.sock

# Ensure you're the same user that started daemon
whoami
ps aux | grep multiclaude | grep -v grep
```

### Stale Socket

```bash
# Socket exists but daemon not running
multiclaude repair

# Or manually remove and restart
rm ~/.multiclaude/daemon.sock
multiclaude start
```

### Timeout

Long commands (add_repo with clone) may take time. Set longer timeout:

```python
# Python
sock.settimeout(60)  # 60 second timeout

# Node.js
client.setTimeout(60000);
```

## Related Documentation

- **[`EXTENSIBILITY.md`](../EXTENSIBILITY.md)** - Overview of extension points
- **[`STATE_FILE_INTEGRATION.md`](STATE_FILE_INTEGRATION.md)** - For read-only monitoring
- `internal/socket/socket.go` - Socket implementation
- `internal/daemon/daemon.go` - Request handlers (lines 574-653)

## Contributing

When adding new socket commands:

1. Add command to `handleRequest()` in `internal/daemon/daemon.go`
2. Implement handler function (e.g., `handleMyCommand()`)
3. Update this document with command reference
4. Add tests in `internal/daemon/daemon_test.go`
5. Update CLI wrapper in `internal/cli/cli.go` if applicable
6. Add/remove commands **only** when the `handleRequest` switch changes.
7. Keep the `socket-commands` marker above in sync; `go run ./cmd/verify-docs` enforces alignment.
8. If you add arguments, update the table here with the real fields used by the handler.