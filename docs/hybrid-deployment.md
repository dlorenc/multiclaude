# Hybrid Deployment Design

This document describes the design for hybrid local/remote agent deployment in multiclaude (Issue #58).

## Overview

Hybrid deployment enables multiclaude agents to run across local developer machines and remote infrastructure, coordinated through a lightweight API. This allows flexible deployment scenarios:

| Local | Remote | Use Case |
|-------|--------|----------|
| workspace | merge-queue, supervisor | Individual dev with shared automation |
| workspace, workers | merge-queue | Team with shared merge infrastructure |
| workspace only | everything else | Thin client mode |
| nothing | all agents | Fully automated repo maintenance |

## Architecture

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                           Coordination API                                    │
│  ┌────────────────┐  ┌────────────────┐  ┌────────────────┐                  │
│  │ Agent Registry │  │ Message Router │  │ Worker Spawner │                  │
│  └────────────────┘  └────────────────┘  └────────────────┘                  │
└──────────────────────────────────────────────────────────────────────────────┘
         ▲                    ▲                    ▲
         │                    │                    │
    ┌────┴────┐          ┌────┴────┐          ┌────┴────┐
    │  Local  │          │  Local  │          │ Remote  │
    │ Daemon  │          │ Daemon  │          │ Workers │
    │(Dev A)  │          │(Dev B)  │          │ (Cloud) │
    └─────────┘          └─────────┘          └─────────┘
```

## Coordination API

### Base URL

Production: `https://api.multiclaude.dev/v1`
Self-hosted: Configurable via `MULTICLAUDE_API_URL`

### Authentication

All requests require a bearer token:
```
Authorization: Bearer <token>
```

Token types:
- **User tokens**: For local daemons, scoped to user's repos
- **Service tokens**: For remote infrastructure, scoped to org/repo

### Endpoints

#### Agent Registry

```
POST   /repos/{owner}/{repo}/agents           # Register agent
GET    /repos/{owner}/{repo}/agents           # List agents
GET    /repos/{owner}/{repo}/agents/{name}    # Get agent details
DELETE /repos/{owner}/{repo}/agents/{name}    # Deregister agent
PATCH  /repos/{owner}/{repo}/agents/{name}    # Update agent status
```

#### Messages

```
POST   /repos/{owner}/{repo}/messages         # Send message
GET    /repos/{owner}/{repo}/agents/{name}/messages  # Get messages for agent
PATCH  /repos/{owner}/{repo}/messages/{id}    # Update message status (ack)
```

#### Worker Spawning

```
POST   /repos/{owner}/{repo}/workers/spawn    # Request worker spawn
GET    /repos/{owner}/{repo}/workers/pending  # List pending spawn requests
```

#### Discovery

```
GET    /repos/{owner}/{repo}/config           # Get repo hybrid config
PUT    /repos/{owner}/{repo}/config           # Update repo hybrid config
```

## Data Models

### Agent

```go
type Agent struct {
    Name        string            `json:"name"`
    Type        AgentType         `json:"type"`
    Location    AgentLocation     `json:"location"`    // "local" or "remote"
    Owner       string            `json:"owner"`       // User/service that owns this agent
    Repo        string            `json:"repo"`        // owner/repo
    Status      AgentStatus       `json:"status"`      // "active", "idle", "stopped"
    Endpoint    string            `json:"endpoint"`    // For message delivery (webhook URL or empty for polling)
    LastSeen    time.Time         `json:"last_seen"`
    CreatedAt   time.Time         `json:"created_at"`
    Metadata    map[string]string `json:"metadata"`    // Flexible key-value pairs
}

type AgentLocation string
const (
    LocationLocal  AgentLocation = "local"
    LocationRemote AgentLocation = "remote"
)

type AgentStatus string
const (
    StatusActive  AgentStatus = "active"
    StatusIdle    AgentStatus = "idle"
    StatusStopped AgentStatus = "stopped"
)
```

### Message

```go
type Message struct {
    ID        string       `json:"id"`
    From      string       `json:"from"`
    To        string       `json:"to"`
    Repo      string       `json:"repo"`
    Body      string       `json:"body"`
    Status    MessageStatus `json:"status"`
    CreatedAt time.Time    `json:"created_at"`
    AckedAt   *time.Time   `json:"acked_at,omitempty"`
}

type MessageStatus string
const (
    MessagePending   MessageStatus = "pending"
    MessageDelivered MessageStatus = "delivered"
    MessageAcked     MessageStatus = "acked"
)
```

### Spawn Request

```go
type SpawnRequest struct {
    ID         string            `json:"id"`
    Repo       string            `json:"repo"`
    Task       string            `json:"task"`
    SpawnedBy  string            `json:"spawned_by"`  // "workspace:dan" or "supervisor"
    Priority   int               `json:"priority"`    // Higher = more urgent
    Status     SpawnStatus       `json:"status"`
    WorkerName string            `json:"worker_name,omitempty"` // Set once spawned
    CreatedAt  time.Time         `json:"created_at"`
    Metadata   map[string]string `json:"metadata"`
}

type SpawnStatus string
const (
    SpawnPending    SpawnStatus = "pending"
    SpawnInProgress SpawnStatus = "in_progress"
    SpawnCompleted  SpawnStatus = "completed"
    SpawnFailed     SpawnStatus = "failed"
)
```

### Repo Config

```go
type HybridConfig struct {
    Repo           string   `json:"repo"`
    Enabled        bool     `json:"enabled"`
    RemoteAgents   []string `json:"remote_agents"`   // Agent types to run remotely
    SpawnEndpoint  string   `json:"spawn_endpoint"`  // Where to send spawn requests
    WebhookSecret  string   `json:"webhook_secret"`  // For validating callbacks
}
```

## Local Daemon Changes

### New Configuration

```go
// In state.json
type Repository struct {
    // ... existing fields ...
    HybridConfig *HybridConfig `json:"hybrid_config,omitempty"`
}

type HybridConfig struct {
    APIEndpoint string `json:"api_endpoint"` // Coordination API URL
    Token       string `json:"token"`        // Auth token (stored separately for security)
    Enabled     bool   `json:"enabled"`
}
```

### Daemon Behavior Changes

When hybrid mode is enabled:

1. **Startup**: Register local agents with coordination API
2. **Message Sending**:
   - Check if target is local or remote
   - Local: Use existing filesystem-based messaging
   - Remote: POST to coordination API
3. **Message Receiving**:
   - Poll coordination API for messages to local agents
   - Deliver via existing tmux mechanism
4. **Agent Discovery**:
   - On repo init, check API for existing remote agents
   - Skip spawning agents that already exist remotely
5. **Heartbeat**:
   - Periodically update agent status in API
   - Mark agents as stopped on daemon shutdown

### New CLI Commands

```bash
# Enable hybrid mode for a repo
multiclaude config <repo> --hybrid=true --api-endpoint=https://api.multiclaude.dev

# View hybrid status
multiclaude config <repo>
# Output includes:
#   Hybrid Mode: enabled
#   API Endpoint: https://api.multiclaude.dev
#   Remote Agents: merge-queue, supervisor
#   Local Agents: workspace, workers

# Spawn remote worker (when in hybrid mode)
multiclaude work --remote "Fix auth bug"
```

## Implementation Plan

### Phase 1: API Types and Client (This PR)

1. Define Go types for API models in `internal/coordination/types.go`
2. Create API client in `internal/coordination/client.go`
3. Add configuration support for hybrid mode

### Phase 2: Local Integration

1. Modify daemon to register agents with API on startup
2. Add message routing through API for remote agents
3. Implement polling for incoming messages
4. Add heartbeat mechanism

### Phase 3: Remote Agent Support

1. Create remote agent runner (container-based)
2. Implement spawn request handling
3. Add worker spawning via API

### Phase 4: CLI Integration

1. Add `--hybrid` flag to `multiclaude config`
2. Add `--remote` flag to `multiclaude work`
3. Display hybrid status in `multiclaude list`

## Security Considerations

1. **Token Storage**: API tokens should be stored securely, not in state.json
   - Use system keychain where available
   - Fall back to `~/.multiclaude/credentials` with restricted permissions

2. **Message Content**: Messages may contain sensitive information
   - Consider end-to-end encryption for message bodies
   - API should use TLS only

3. **Agent Impersonation**: Prevent agents from impersonating others
   - Token scoping to specific repos
   - Agent names must be unique within a repo

4. **Spawn Abuse**: Prevent runaway worker spawning
   - Rate limiting on spawn endpoint
   - Maximum concurrent workers per repo

## Open Questions

1. **Webhook vs Polling**: Should remote-to-local messages use webhooks or polling?
   - Polling is simpler but higher latency
   - Webhooks require local daemon to be reachable

2. **State Consistency**: How to handle split-brain scenarios?
   - Local daemon thinks agent exists, API doesn't
   - Conflict resolution strategy needed

3. **Offline Mode**: What happens when API is unreachable?
   - Graceful degradation to local-only mode?
   - Queue messages for later delivery?

## References

- Issue #58: Hybrid deployment architecture
- AGENTS.md: Current agent system documentation
- internal/messages/: Existing message implementation
