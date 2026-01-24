# Web UI Development Guide

> **WARNING: REFERENCE IMPLEMENTATION DOES NOT EXIST**
>
> This document references `cmd/multiclaude-web` and `internal/dashboard/` which do **not exist** in the codebase.
> Per ROADMAP.md, web interfaces and dashboards are explicitly out of scope for upstream multiclaude.
>
> This document is preserved as a design guide for potential fork implementations.

**Extension Point:** Building web dashboards and monitoring UIs (FOR FORKS ONLY)

This guide shows you how to build web-based user interfaces for multiclaude in a fork.

**IMPORTANT:** Web UIs are a **fork-only feature**. Upstream multiclaude explicitly rejects web interfaces per ROADMAP.md. This guide is for fork maintainers and downstream projects only.

## Overview

Building a web UI for multiclaude is straightforward:
1. Read `state.json` with a file watcher (`fsnotify`)
2. Serve state via REST API
3. (Optional) Add Server-Sent Events for live updates
4. Build a simple HTML/CSS/JS frontend

**Architecture Benefits:**
- **No daemon dependency**: Read state directly from file
- **Read-only**: Can't break multiclaude operation
- **Simple**: Standard web stack, no special protocols
- **Live updates**: SSE provides real-time updates efficiently

## Reference Implementation

The `cmd/multiclaude-web` binary provides:
- Multi-repository dashboard
- Live agent status
- Task history browser
- REST API + SSE
- Single-page app (embedded in binary)

**Total LOC:** ~500 lines (excluding HTML/CSS)

## Quick Start

### Running the Reference Implementation

```bash
# Build
go build ./cmd/multiclaude-web

# Run on localhost:8080
./multiclaude-web

# Custom port
./multiclaude-web --port 3000

# Listen on all interfaces (⚠️ no auth!)
./multiclaude-web --bind 0.0.0.0

# Custom state file location
./multiclaude-web --state /path/to/state.json
```

### Testing Without Multiclaude

```bash
# Create fake state for UI development
cat > /tmp/test-state.json <<'EOF'
{
  "repos": {
    "test-repo": {
      "github_url": "https://github.com/user/repo",
      "tmux_session": "mc-test-repo",
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
      },
      "task_history": [
        {
          "name": "brave-lion",
          "task": "Fix bug",
          "status": "merged",
          "pr_url": "https://github.com/user/repo/pull/42",
          "pr_number": 42,
          "created_at": "2024-01-14T10:00:00Z",
          "completed_at": "2024-01-14T11:00:00Z"
        }
      ]
    }
  }
}
EOF

# Point your UI at test state
./multiclaude-web --state /tmp/test-state.json
```

## Architecture

### Component Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                      Browser                                │
│  ┌─────────────────────────────────────────────────────┐    │
│  │  HTML/CSS/JS (Single Page App)                      │    │
│  │  - Repo list                                        │    │
│  │  - Agent cards                                      │    │
│  │  - Task history                                     │    │
│  └───────────┬─────────────────────────────────────────┘    │
└──────────────┼──────────────────────────────────────────────┘
               │
               │ HTTP (REST + SSE)
               │
┌──────────────▼──────────────────────────────────────────────┐
│                   Web Server (Go)                           │
│  ┌──────────────────┐         ┌──────────────────┐          │
│  │  API Handler     │         │  SSE Broadcaster │          │
│  │  - GET /api/*    │────────▶│  - Event stream  │          │
│  └──────────────────┘         └──────────────────┘          │
│           │                            ▲                     │
│           ▼                            │                     │
│  ┌──────────────────────────────────────────────────┐       │
│  │            State Reader                          │       │
│  │  - Watches state.json with fsnotify              │       │
│  │  - Caches parsed state                           │       │
│  │  - Notifies subscribers on change                │       │
│  └────────────────┬─────────────────────────────────┘       │
└───────────────────┼──────────────────────────────────────────┘
                    │ fsnotify.Watch
                    │
┌───────────────────▼──────────────────────────────────────────┐
│               ~/.multiclaude/state.json                      │
│               (Written atomically by daemon)                 │
└──────────────────────────────────────────────────────────────┘
```

### Core Components

1. **StateReader** (`internal/dashboard/reader.go`)
   - Watches state file with fsnotify
   - Caches parsed state
   - Notifies callbacks on changes
   - Handles multiple state files (multi-machine support)

2. **APIHandler** (`internal/dashboard/api.go`)
   - REST endpoints for querying state
   - SSE endpoint for live updates
   - JSON serialization

3. **Server** (`internal/dashboard/server.go`)
   - HTTP server setup
   - Static file serving (embedded frontend)
   - Routing

4. **Frontend** (`internal/dashboard/web/`)
   - HTML/CSS/JS single-page app
   - Connects to REST API
   - Subscribes to SSE for live updates

## Building Your Own UI

### Step 1: State Reader

```go
package main

import (
    "encoding/json"
    "log"
    "os"
    "sync"

    "github.com/fsnotify/fsnotify"
    "github.com/dlorenc/multiclaude/internal/state"
)

type StateReader struct {
    path     string
    watcher  *fsnotify.Watcher
    mu       sync.RWMutex
    state    *state.State
    onChange func(*state.State)
}

func NewStateReader(path string) (*StateReader, error) {
    watcher, err := fsnotify.NewWatcher()
    if err != nil {
        return nil, err
    }

    reader := &StateReader{
        path:    path,
        watcher: watcher,
    }

    // Initial read
    if err := reader.reload(); err != nil {
        return nil, err
    }

    // Watch for changes
    if err := watcher.Add(path); err != nil {
        return nil, err
    }

    // Start watch loop
    go reader.watchLoop()

    return reader, nil
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

func (r *StateReader) watchLoop() {
    for {
        select {
        case event := <-r.watcher.Events:
            if event.Op&fsnotify.Write == fsnotify.Write {
                r.reload()
                if r.onChange != nil {
                    r.onChange(r.Get())
                }
            }
        case err := <-r.watcher.Errors:
            log.Printf("Watcher error: %v", err)
        }
    }
}

func (r *StateReader) Get() *state.State {
    r.mu.RLock()
    defer r.mu.RUnlock()
    return r.state
}

func (r *StateReader) Watch(onChange func(*state.State)) {
    r.onChange = onChange
}

func (r *StateReader) Close() error {
    return r.watcher.Close()
}
```

### Step 2: REST API

```go
package main

import (
    "encoding/json"
    "net/http"
)

type APIHandler struct {
    reader *StateReader
}

func NewAPIHandler(reader *StateReader) *APIHandler {
    return &APIHandler{reader: reader}
}

// GET /api/repos
func (h *APIHandler) HandleRepos(w http.ResponseWriter, r *http.Request) {
    state := h.reader.Get()

    type RepoInfo struct {
        Name       string `json:"name"`
        GithubURL  string `json:"github_url"`
        AgentCount int    `json:"agent_count"`
    }

    repos := []RepoInfo{}
    for name, repo := range state.Repos {
        repos = append(repos, RepoInfo{
            Name:       name,
            GithubURL:  repo.GithubURL,
            AgentCount: len(repo.Agents),
        })
    }

    h.writeJSON(w, repos)
}

// GET /api/repos/{name}/agents
func (h *APIHandler) HandleAgents(w http.ResponseWriter, r *http.Request) {
    state := h.reader.Get()

    // Extract repo name from path (you'll need a router for this)
    repoName := extractRepoName(r.URL.Path)

    repo, ok := state.Repos[repoName]
    if !ok {
        http.Error(w, "Repository not found", http.StatusNotFound)
        return
    }

    h.writeJSON(w, repo.Agents)
}

// GET /api/repos/{name}/history
func (h *APIHandler) HandleHistory(w http.ResponseWriter, r *http.Request) {
    state := h.reader.Get()

    repoName := extractRepoName(r.URL.Path)
    repo, ok := state.Repos[repoName]
    if !ok {
        http.Error(w, "Repository not found", http.StatusNotFound)
        return
    }

    // Return task history (most recent first)
    history := repo.TaskHistory
    if history == nil {
        history = []state.TaskHistoryEntry{}
    }

    // Reverse for most recent first
    reversed := make([]state.TaskHistoryEntry, len(history))
    for i, entry := range history {
        reversed[len(history)-1-i] = entry
    }

    h.writeJSON(w, reversed)
}

func (h *APIHandler) writeJSON(w http.ResponseWriter, data interface{}) {
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(data)
}
```

### Step 3: Server-Sent Events (Live Updates)

```go
package main

import (
    "encoding/json"
    "fmt"
    "net/http"
    "sync"
)

type APIHandler struct {
    reader  *StateReader
    mu      sync.RWMutex
    clients map[chan []byte]bool
}

func NewAPIHandler(reader *StateReader) *APIHandler {
    handler := &APIHandler{
        reader:  reader,
        clients: make(map[chan []byte]bool),
    }

    // Register for state changes
    reader.Watch(func(s *state.State) {
        handler.broadcastUpdate(s)
    })

    return handler
}

// GET /api/events - SSE endpoint
func (h *APIHandler) HandleEvents(w http.ResponseWriter, r *http.Request) {
    // Set SSE headers
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    w.Header().Set("Access-Control-Allow-Origin", "*")

    // Create channel for this client
    clientChan := make(chan []byte, 10)

    h.mu.Lock()
    h.clients[clientChan] = true
    h.mu.Unlock()

    // Remove client on disconnect
    defer func() {
        h.mu.Lock()
        delete(h.clients, clientChan)
        close(clientChan)
        h.mu.Unlock()
    }()

    // Send initial state
    state := h.reader.Get()
    data, _ := json.Marshal(state)
    fmt.Fprintf(w, "data: %s\n\n", data)
    if f, ok := w.(http.Flusher); ok {
        f.Flush()
    }

    // Listen for updates or client disconnect
    for {
        select {
        case msg := <-clientChan:
            fmt.Fprintf(w, "data: %s\n\n", msg)
            if f, ok := w.(http.Flusher); ok {
                f.Flush()
            }
        case <-r.Context().Done():
            return
        }
    }
}

func (h *APIHandler) broadcastUpdate(s *state.State) {
    data, err := json.Marshal(s)
    if err != nil {
        return
    }

    h.mu.RLock()
    defer h.mu.RUnlock()

    for client := range h.clients {
        select {
        case client <- data:
        default:
            // Client buffer full, skip this update
        }
    }
}
```

### Step 4: Main Server

```go
package main

import (
    "embed"
    "log"
    "net/http"
    "os"
    "path/filepath"
)

//go:embed web/*
var webFiles embed.FS

func main() {
    // Create state reader
    statePath := filepath.Join(os.Getenv("HOME"), ".multiclaude/state.json")
    reader, err := NewStateReader(statePath)
    if err != nil {
        log.Fatalf("Failed to create state reader: %v", err)
    }
    defer reader.Close()

    // Create API handler
    api := NewAPIHandler(reader)

    // Setup routes
    http.HandleFunc("/api/repos", api.HandleRepos)
    http.HandleFunc("/api/repos/", api.HandleAgents)  // Handles /api/repos/{name}/*
    http.HandleFunc("/api/events", api.HandleEvents)

    // Serve static files
    http.Handle("/", http.FileServer(http.FS(webFiles)))

    // Start server
    addr := ":8080"
    log.Printf("Server starting on %s", addr)
    log.Fatal(http.ListenAndServe(addr, nil))
}
```

### Step 5: Frontend (HTML/JS)

```html
<!DOCTYPE html>
<html>
<head>
    <title>Multiclaude Dashboard</title>
    <style>
        body {
            font-family: system-ui, sans-serif;
            margin: 0;
            padding: 20px;
            background: #f5f5f5;
        }
        .repo {
            background: white;
            border-radius: 8px;
            padding: 20px;
            margin-bottom: 20px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        .agent {
            display: inline-block;
            margin: 10px;
            padding: 10px 15px;
            background: #e3f2fd;
            border-radius: 6px;
            border-left: 4px solid #2196f3;
        }
        .agent.worker { border-left-color: #4caf50; background: #e8f5e9; }
        .agent.supervisor { border-left-color: #ff9800; background: #fff3e0; }
        .status { color: #666; font-size: 0.9em; }
        .live-indicator {
            display: inline-block;
            width: 8px;
            height: 8px;
            border-radius: 50%;
            background: #4caf50;
            animation: pulse 2s infinite;
        }
        @keyframes pulse {
            0%, 100% { opacity: 1; }
            50% { opacity: 0.3; }
        }
    </style>
</head>
<body>
    <h1>
        Multiclaude Dashboard
        <span class="live-indicator" id="liveIndicator"></span>
    </h1>

    <div id="repos"></div>

    <script>
        const reposDiv = document.getElementById('repos');
        const liveIndicator = document.getElementById('liveIndicator');

        // Fetch initial state
        async function fetchState() {
            const res = await fetch('/api/repos');
            const repos = await res.json();
            renderRepos(repos);
        }

        // Render repositories
        async function renderRepos(repos) {
            reposDiv.innerHTML = '';

            for (const repo of repos) {
                const repoDiv = document.createElement('div');
                repoDiv.className = 'repo';

                const title = document.createElement('h2');
                title.textContent = repo.name;
                repoDiv.appendChild(title);

                // Fetch agents
                const agentsRes = await fetch(`/api/repos/${repo.name}/agents`);
                const agents = await agentsRes.json();

                const agentsDiv = document.createElement('div');
                for (const [name, agent] of Object.entries(agents)) {
                    const agentDiv = document.createElement('div');
                    agentDiv.className = `agent ${agent.type}`;
                    agentDiv.innerHTML = `
                        <strong>${name}</strong> (${agent.type})
                        ${agent.task ? `<br><span class="status">${agent.task}</span>` : ''}
                    `;
                    agentsDiv.appendChild(agentDiv);
                }
                repoDiv.appendChild(agentsDiv);

                reposDiv.appendChild(repoDiv);
            }
        }

        // Subscribe to live updates via SSE
        const eventSource = new EventSource('/api/events');

        eventSource.onmessage = (event) => {
            const state = JSON.parse(event.data);

            // Convert state to repos array
            const repos = Object.entries(state.repos || {}).map(([name, repo]) => ({
                name,
                github_url: repo.github_url,
                agent_count: Object.keys(repo.agents || {}).length
            }));

            renderRepos(repos);

            // Blink live indicator
            liveIndicator.style.animation = 'none';
            setTimeout(() => {
                liveIndicator.style.animation = 'pulse 2s infinite';
            }, 10);
        };

        eventSource.onerror = () => {
            console.error('SSE connection lost');
            liveIndicator.style.background = '#f44336';
        };

        // Initial load
        fetchState();
    </script>
</body>
</html>
```

## REST API Reference

### Endpoints

| Endpoint | Method | Description | Response |
|----------|--------|-------------|----------|
| `/api/repos` | GET | List all repositories | Array of repo info |
| `/api/repos/{name}` | GET | Get repository details | Repository object |
| `/api/repos/{name}/agents` | GET | Get agents for repo | Map of agents |
| `/api/repos/{name}/history` | GET | Get task history | Array of history entries |
| `/api/events` | GET | SSE stream of state updates | Server-Sent Events |

### Example Responses

#### GET /api/repos

```json
[
  {
    "name": "my-app",
    "github_url": "https://github.com/user/my-app",
    "agent_count": 3
  }
]
```

#### GET /api/repos/my-app/agents

```json
{
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
```

#### GET /api/repos/my-app/history

```json
[
  {
    "name": "brave-lion",
    "task": "Fix bug",
    "status": "merged",
    "pr_url": "https://github.com/user/my-app/pull/42",
    "pr_number": 42,
    "created_at": "2024-01-14T10:00:00Z",
    "completed_at": "2024-01-14T11:00:00Z"
  }
]
```

## Frontend Frameworks

### React Example

```jsx
import React, { useState, useEffect } from 'react';

function Dashboard() {
    const [repos, setRepos] = useState([]);

    useEffect(() => {
        // Fetch initial state
        fetch('/api/repos')
            .then(res => res.json())
            .then(setRepos);

        // Subscribe to SSE
        const eventSource = new EventSource('/api/events');
        eventSource.onmessage = (event) => {
            const state = JSON.parse(event.data);
            const repos = Object.entries(state.repos || {}).map(([name, repo]) => ({
                name,
                agentCount: Object.keys(repo.agents || {}).length
            }));
            setRepos(repos);
        };

        return () => eventSource.close();
    }, []);

    return (
        <div>
            <h1>Multiclaude Dashboard</h1>
            {repos.map(repo => (
                <div key={repo.name}>
                    <h2>{repo.name}</h2>
                    <p>{repo.agentCount} agents</p>
                </div>
            ))}
        </div>
    );
}
```

### Vue Example

```vue
<template>
  <div>
    <h1>Multiclaude Dashboard</h1>
    <div v-for="repo in repos" :key="repo.name">
      <h2>{{ repo.name }}</h2>
      <p>{{ repo.agentCount }} agents</p>
    </div>
  </div>
</template>

<script>
export default {
  data() {
    return {
      repos: []
    };
  },
  mounted() {
    // Fetch initial state
    fetch('/api/repos')
      .then(res => res.json())
      .then(repos => this.repos = repos);

    // Subscribe to SSE
    const eventSource = new EventSource('/api/events');
    eventSource.onmessage = (event) => {
      const state = JSON.parse(event.data);
      this.repos = Object.entries(state.repos || {}).map(([name, repo]) => ({
        name,
        agentCount: Object.keys(repo.agents || {}).length
      }));
    };
  }
};
</script>
```

## Advanced Features

### Multi-Machine Support

```go
// Watch multiple state files
reader, _ := dashboard.NewStateReader([]string{
    "/home/user/.multiclaude/state.json",
    "ssh://dev-box/home/user/.multiclaude/state.json",  // Future: remote support
})

// Aggregated state includes machine identifier
type AggregatedState struct {
    Machines map[string]*MachineState
}

type MachineState struct {
    Path   string
    Repos  map[string]*state.Repository
}
```

### Filtering and Search

```javascript
// Frontend: Filter agents by type
const workers = Object.entries(agents)
    .filter(([_, agent]) => agent.type === 'worker');

// Backend: Add query parameters
func (h *APIHandler) HandleAgents(w http.ResponseWriter, r *http.Request) {
    agentType := r.URL.Query().Get("type")  // ?type=worker

    // Filter agents...
}
```

### Historical Charts

```javascript
// Fetch history and render chart
async function fetchHistory(repo) {
    const res = await fetch(`/api/repos/${repo}/history`);
    const history = await res.json();

    // Count by status
    const statusCounts = {};
    history.forEach(entry => {
        statusCounts[entry.status] = (statusCounts[entry.status] || 0) + 1;
    });

    // Render with Chart.js, D3, etc.
    renderChart(statusCounts);
}
```

## Security

### Authentication (Not Implemented)

The reference implementation has **no authentication**. For production:

```go
// Add basic auth middleware
func authMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        user, pass, ok := r.BasicAuth()
        if !ok || user != "admin" || pass != "secret" {
            w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
            http.Error(w, "Unauthorized", http.StatusUnauthorized)
            return
        }
        next.ServeHTTP(w, r)
    })
}

http.Handle("/", authMiddleware(handler))
```

### HTTPS

```go
// Generate self-signed cert for development
// openssl req -x509 -newkey rsa:4096 -nodes -keyout key.pem -out cert.pem -days 365

log.Fatal(http.ListenAndServeTLS(":8443", "cert.pem", "key.pem", nil))
```

### CORS (for separate frontend)

```go
func corsMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Access-Control-Allow-Origin", "http://localhost:3000")
        w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
        if r.Method == "OPTIONS" {
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

## Deployment

### SSH Tunnel (Recommended)

```bash
# On remote machine
./multiclaude-web

# On local machine
ssh -L 8080:localhost:8080 user@remote-machine

# Browse to http://localhost:8080
```

### Docker

```dockerfile
FROM golang:1.21 AS builder
WORKDIR /app
COPY . .
RUN go build ./cmd/multiclaude-web

FROM debian:12-slim
COPY --from=builder /app/multiclaude-web /usr/local/bin/
CMD ["multiclaude-web", "--bind", "0.0.0.0"]
```

### systemd Service

```ini
[Unit]
Description=Multiclaude Web Dashboard
After=network.target

[Service]
Type=simple
User=multiclaude
ExecStart=/usr/local/bin/multiclaude-web
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

## Performance

### State File Size Growth

- **Typical**: 10-100KB
- **With large history**: 100KB-1MB
- **Memory impact**: Minimal (parsed once, cached)

### SSE Connection Limits

- **Per server**: Thousands of concurrent connections
- **Per client**: Browser limit ~6 connections per domain

### Update Frequency

- State updates: Every agent action (~1-10/minute)
- SSE broadcasts: Debounced to avoid spam
- Client receives: Latest state on each update

## Troubleshooting

### SSE Not Working

```javascript
// Check SSE connection
eventSource.onerror = (err) => {
    console.error('SSE error:', err);

    // Fall back to polling
    setInterval(fetchState, 5000);
};
```

### State File Not Found

```go
statePath := filepath.Join(os.Getenv("HOME"), ".multiclaude/state.json")
if _, err := os.Stat(statePath); os.IsNotExist(err) {
    log.Fatalf("State file not found: %s\nIs multiclaude initialized?", statePath)
}
```

### CORS Issues

```javascript
// If API and frontend on different ports
fetch('http://localhost:8080/api/repos', {
    mode: 'cors'
})
```

## Related Documentation

- **[`EXTENSIBILITY.md`](../EXTENSIBILITY.md)** - Overview of extension points
- **[`STATE_FILE_INTEGRATION.md`](STATE_FILE_INTEGRATION.md)** - State schema reference
- **[`WEB_DASHBOARD.md`](../WEB_DASHBOARD.md)** - Reference implementation docs
- `cmd/multiclaude-web/` - Complete working example
- `internal/dashboard/` - Backend implementation

## Contributing

Improvements to the reference implementation are welcome:

1. **UI enhancements**: Better styling, charts, filters
2. **Features**: Search, notifications, keyboard shortcuts
3. **Accessibility**: ARIA labels, keyboard navigation
4. **Documentation**: More examples, troubleshooting tips

Submit PRs to the fork repository (marked `[fork-only]`).
