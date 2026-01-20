# Hybrid Deployment Architecture

**Status:** Design Document
**Package:** `internal/coordination`
**Date:** 2026-01-20

## Overview

Hybrid deployment enables multiclaude to run agents across both local machines and remote infrastructure. This provides the best of both worlds: local agents for interactive work and immediate feedback, remote agents for heavy compute tasks that would block the developer's machine.

## Problem Statement

Current multiclaude deployments are entirely local:

1. **Resource constraints**: Running multiple Claude agents taxes the developer's machine
2. **Network limitations**: Local machines may have limited bandwidth for API calls
3. **Availability**: Agents stop when the developer's machine sleeps or goes offline
4. **Scaling**: Can't easily add more compute capacity for large tasks
5. **Team coordination**: Shared agents (supervisor, merge-queue) must run somewhere

The hybrid deployment model addresses these by allowing agents to run wherever makes the most sense.

## Goals

1. **Transparent coordination**: Local and remote agents communicate seamlessly
2. **Flexible placement**: Each agent type can run locally or remotely based on configuration
3. **Graceful degradation**: If remote is unavailable, fall back to local execution
4. **Minimal configuration**: Works out of the box with sensible defaults
5. **Security**: Secure communication between local daemon and remote services

## Non-Goals

- Building hosted infrastructure (users bring their own)
- Multi-tenancy (one coordination API per team/project)
- Real-time streaming of agent output (existing tmux model works for local)
- Replacing the local daemon (it remains the source of truth for local agents)

## Architecture

### System Overview

```
┌──────────────────────────────────────────────────────────────────────────┐
│                         DEVELOPER MACHINE                                 │
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────────┐ │
│  │                          LOCAL DAEMON                                │ │
│  │                                                                      │ │
│  │  ┌──────────────┐    ┌──────────────┐    ┌──────────────────────┐  │ │
│  │  │ Local        │    │ Coordination │    │ Message Router       │  │ │
│  │  │ Registry     │◄──►│ Client       │◄──►│ (hybrid-aware)       │  │ │
│  │  │ (cache)      │    │              │    │                      │  │ │
│  │  └──────────────┘    └──────────────┘    └──────────────────────┘  │ │
│  │         ▲                   │                      │               │ │
│  │         │                   │                      │               │ │
│  └─────────┼───────────────────┼──────────────────────┼───────────────┘ │
│            │                   │                      │                  │
│     ┌──────┴──────┐           │                      │                  │
│     │             │           │                      │                  │
│  ┌──▼───┐    ┌────▼───┐      │                      │                  │
│  │work- │    │super-  │      │                      │                  │
│  │space │    │visor   │      │                      │                  │
│  │(local│    │(local) │      │                      │                  │
│  │only) │    │        │      │                      │                  │
│  └──────┘    └────────┘      │                      │                  │
│                              │                      │                  │
└──────────────────────────────┼──────────────────────┼──────────────────┘
                               │                      │
                               │ HTTPS                │ HTTPS
                               ▼                      ▼
┌──────────────────────────────────────────────────────────────────────────┐
│                      REMOTE INFRASTRUCTURE                                │
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────────┐ │
│  │                     COORDINATION API                                 │ │
│  │                                                                      │ │
│  │  ┌──────────────┐    ┌──────────────┐    ┌──────────────────────┐  │ │
│  │  │ Remote       │    │ Spawn        │    │ Message Relay        │  │ │
│  │  │ Registry     │    │ Manager      │    │                      │  │ │
│  │  │              │    │              │    │                      │  │ │
│  │  └──────────────┘    └──────────────┘    └──────────────────────┘  │ │
│  │                              │                                      │ │
│  └──────────────────────────────┼──────────────────────────────────────┘ │
│                                 │                                        │
│                    ┌────────────┼────────────┐                          │
│                    │            │            │                          │
│              ┌─────▼────┐ ┌─────▼────┐ ┌─────▼────┐                     │
│              │ worker-1 │ │ worker-2 │ │ worker-N │                     │
│              │ (remote) │ │ (remote) │ │ (remote) │                     │
│              └──────────┘ └──────────┘ └──────────┘                     │
│                                                                          │
└──────────────────────────────────────────────────────────────────────────┘
```

### Component Responsibilities

| Component | Location | Purpose |
|-----------|----------|---------|
| Local Daemon | Developer machine | Orchestrates local agents, caches registry, routes messages |
| Coordination Client | Developer machine | Communicates with remote Coordination API |
| Local Registry | Developer machine | Caches agent state for fast lookups |
| Remote Registry | Remote | Source of truth for all agent registrations |
| Spawn Manager | Remote | Creates and manages remote agent processes |
| Message Relay | Remote | Routes messages between local and remote agents |

### Agent Placement Strategy

By default, agents are placed according to their ownership level:

| Agent Type | Ownership | Default Location | Rationale |
|------------|-----------|------------------|-----------|
| `workspace` | User | Local | Interactive, needs local file access |
| `supervisor` | Repo | Configurable | Can run anywhere, but local gives faster response |
| `merge-queue` | Repo | Remote | Long-running, shouldn't block developer machine |
| `worker` | Task | Remote | Compute-intensive, benefits from remote resources |
| `review` | Task | Remote | Can be spawned on demand |

Configuration can override these defaults:

```yaml
# ~/.multiclaude/hybrid.yaml
hybrid:
  enabled: true
  coordination_api_url: "https://multiclaude-api.example.com"
  api_token: "${MULTICLAUDE_API_TOKEN}"

  # Override default placement
  local_agent_types:
    - workspace
    - supervisor  # Keep supervisor local for faster interaction

  remote_agent_types:
    - merge-queue
    - worker
    - review

  # Fall back to local if remote is unavailable
  fallback_to_local: true
```

## Coordination API

The Coordination API is a REST service that provides:

1. **Agent Registry**: Track all agents across local and remote
2. **Spawn Management**: Create remote agent instances
3. **Message Relay**: Route messages between agents

### API Endpoints

#### Registry Operations

```
POST   /api/v1/agents                    # Register an agent
DELETE /api/v1/agents/{repo}/{name}      # Unregister an agent
GET    /api/v1/agents/{repo}/{name}      # Get agent info
GET    /api/v1/agents/{repo}             # List agents for repo
PUT    /api/v1/agents/{repo}/{name}/heartbeat  # Update heartbeat
PUT    /api/v1/agents/{repo}/{name}/status     # Update status
```

#### Spawn Operations

```
POST   /api/v1/spawn                     # Request worker spawn
GET    /api/v1/spawn/{id}                # Get spawn status
DELETE /api/v1/spawn/{id}                # Cancel spawn request
```

#### Message Operations

```
POST   /api/v1/messages                  # Send a message
GET    /api/v1/messages/{repo}/{agent}   # Get messages for agent
PUT    /api/v1/messages/{id}/ack         # Acknowledge message
```

### Request/Response Formats

#### Register Agent

```http
POST /api/v1/agents
Content-Type: application/json
Authorization: Bearer <token>

{
  "name": "eager-badger",
  "type": "worker",
  "repo_name": "my-project",
  "location": "remote",
  "owner": "alice@example.com",
  "metadata": {
    "task": "Implement user authentication"
  }
}
```

Response:

```json
{
  "success": true,
  "agent": {
    "name": "eager-badger",
    "type": "worker",
    "repo_name": "my-project",
    "location": "remote",
    "ownership": "task",
    "registered_at": "2026-01-20T10:30:00Z",
    "last_heartbeat": "2026-01-20T10:30:00Z",
    "status": "active"
  }
}
```

#### Spawn Worker

```http
POST /api/v1/spawn
Content-Type: application/json
Authorization: Bearer <token>

{
  "repo_name": "my-project",
  "task": "Implement user authentication",
  "spawned_by": "supervisor",
  "prefer_location": "remote",
  "metadata": {
    "priority": "high"
  }
}
```

Response:

```json
{
  "success": true,
  "spawn": {
    "worker_name": "eager-badger",
    "location": "remote",
    "endpoint": "wss://multiclaude-api.example.com/agents/eager-badger"
  }
}
```

#### Send Message

```http
POST /api/v1/messages
Content-Type: application/json
Authorization: Bearer <token>

{
  "from": "supervisor",
  "to": "eager-badger",
  "repo_name": "my-project",
  "body": "Can you clarify the authentication requirements?"
}
```

Response:

```json
{
  "success": true,
  "message": {
    "id": "msg_abc123",
    "from": "supervisor",
    "to": "eager-badger",
    "repo_name": "my-project",
    "body": "Can you clarify the authentication requirements?",
    "timestamp": "2026-01-20T10:35:00Z",
    "route_info": {
      "source_location": "local",
      "dest_location": "remote",
      "routed_via": "api",
      "routed_at": "2026-01-20T10:35:00Z"
    }
  }
}
```

## Client Implementation

The `coordination.Client` provides the Go interface for interacting with the Coordination API.

### Client Interface

```go
// Client communicates with the remote Coordination API
type Client struct {
    baseURL    string
    apiToken   string
    httpClient *http.Client
    localCache *LocalRegistry
}

// NewClient creates a new coordination client
func NewClient(config HybridConfig) (*Client, error)

// Registry operations (implements Registry interface)
func (c *Client) Register(agent *AgentInfo) error
func (c *Client) Unregister(repoName, agentName string) error
func (c *Client) Get(repoName, agentName string) (*AgentInfo, error)
func (c *Client) List(repoName string) ([]*AgentInfo, error)
func (c *Client) ListByType(repoName, agentType string) ([]*AgentInfo, error)
func (c *Client) ListByLocation(repoName string, location Location) ([]*AgentInfo, error)
func (c *Client) UpdateHeartbeat(repoName, agentName string) error
func (c *Client) UpdateStatus(repoName, agentName string, status AgentStatus) error

// Spawn operations
func (c *Client) RequestSpawn(req SpawnRequest) (*SpawnResponse, error)
func (c *Client) GetSpawnStatus(spawnID string) (*SpawnResponse, error)
func (c *Client) CancelSpawn(spawnID string) error

// Message operations
func (c *Client) SendMessage(msg *RoutedMessage) error
func (c *Client) GetMessages(repoName, agentName string) ([]*RoutedMessage, error)
func (c *Client) AcknowledgeMessage(messageID string) error

// Health
func (c *Client) Ping() error
```

### Error Handling

The client uses structured errors consistent with the rest of multiclaude:

```go
// Coordination-specific errors
func CoordinationAPIUnavailable(cause error) *CLIError
func CoordinationAuthFailed() *CLIError
func AgentAlreadyRegistered(name, repo string) *CLIError
func RemoteSpawnFailed(cause error) *CLIError
```

### Caching Strategy

The client maintains a local cache to reduce API calls and provide resilience:

1. **Read-through cache**: `Get` and `List` check local cache first
2. **Write-through cache**: `Register` and `Unregister` update both local and remote
3. **TTL-based refresh**: Cache entries expire after 30 seconds
4. **Heartbeat updates**: Local cache updated on every heartbeat

```go
type cacheEntry struct {
    agent     *AgentInfo
    fetchedAt time.Time
}

const cacheTTL = 30 * time.Second

func (c *Client) Get(repoName, agentName string) (*AgentInfo, error) {
    // Check cache first
    if entry, ok := c.cache.Get(repoName, agentName); ok {
        if time.Since(entry.fetchedAt) < cacheTTL {
            return entry.agent, nil
        }
    }

    // Fetch from API
    agent, err := c.fetchFromAPI(repoName, agentName)
    if err != nil {
        // If API unavailable and we have cached data, use it
        if entry, ok := c.cache.Get(repoName, agentName); ok {
            return entry.agent, nil
        }
        return nil, err
    }

    // Update cache
    c.cache.Set(repoName, agentName, agent)
    return agent, nil
}
```

## Message Routing

Messages between agents are routed based on location:

### Routing Logic

```
┌─────────────────────────────────────────────────────────────┐
│                     Message Router                           │
│                                                             │
│  Source    Destination    Route                             │
│  ──────    ───────────    ─────                             │
│  local     local          Direct (filesystem)               │
│  local     remote         Via Coordination API              │
│  remote    local          Via Coordination API → Daemon     │
│  remote    remote         Direct (remote infrastructure)    │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### Implementation in Daemon

The daemon's message router is extended to handle hybrid routing:

```go
func (d *Daemon) routeMessage(msg Message) error {
    // Get sender and recipient info
    sender, _ := d.registry.Get(msg.RepoName, msg.From)
    recipient, _ := d.registry.Get(msg.RepoName, msg.To)

    // Determine routing strategy
    switch {
    case sender.Location == LocationLocal && recipient.Location == LocationLocal:
        // Local-to-local: use existing filesystem routing
        return d.routeLocalMessage(msg)

    case sender.Location == LocationLocal && recipient.Location == LocationRemote:
        // Local-to-remote: send via API
        return d.coordination.SendMessage(toRoutedMessage(msg))

    case sender.Location == LocationRemote && recipient.Location == LocationLocal:
        // Remote-to-local: already received via API, deliver locally
        return d.routeLocalMessage(msg)

    case sender.Location == LocationRemote && recipient.Location == LocationRemote:
        // Remote-to-remote: let API handle it
        return d.coordination.SendMessage(toRoutedMessage(msg))
    }

    return fmt.Errorf("unknown routing scenario")
}
```

### Polling for Remote Messages

The daemon polls for messages destined for local agents:

```go
func (d *Daemon) startRemoteMessagePoller() {
    ticker := time.NewTicker(5 * time.Second)
    for range ticker.C {
        // Get messages for all local agents
        localAgents, _ := d.registry.ListByLocation(d.repoName, LocationLocal)
        for _, agent := range localAgents {
            messages, err := d.coordination.GetMessages(d.repoName, agent.Name)
            if err != nil {
                continue
            }
            for _, msg := range messages {
                d.deliverLocalMessage(msg)
                d.coordination.AcknowledgeMessage(msg.ID)
            }
        }
    }
}
```

## Configuration

### Hybrid Configuration File

Location: `~/.multiclaude/hybrid.yaml` or per-repo `.multiclaude/hybrid.yaml`

```yaml
# Hybrid deployment configuration
hybrid:
  # Enable/disable hybrid mode
  enabled: true

  # Coordination API endpoint
  coordination_api_url: "https://multiclaude-api.example.com"

  # Authentication token (supports env vars)
  api_token: "${MULTICLAUDE_API_TOKEN}"

  # Agent placement configuration
  local_agent_types:
    - workspace

  remote_agent_types:
    - supervisor
    - merge-queue
    - worker
    - review

  # Behavior settings
  fallback_to_local: true
  heartbeat_interval: 30s
  message_poll_interval: 5s

  # Timeouts
  api_timeout: 10s
  spawn_timeout: 60s
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `MULTICLAUDE_API_TOKEN` | Authentication token for Coordination API | (required if hybrid enabled) |
| `MULTICLAUDE_API_URL` | Override coordination_api_url | (from config) |
| `MULTICLAUDE_HYBRID_ENABLED` | Enable/disable hybrid mode | `false` |

### CLI Commands

```bash
# Enable hybrid mode
multiclaude config hybrid --enabled=true

# Set coordination API URL
multiclaude config hybrid --api-url="https://multiclaude-api.example.com"

# View current hybrid configuration
multiclaude config hybrid --show

# Test connectivity to coordination API
multiclaude config hybrid --test

# View agent locations
multiclaude work list --show-location
```

## Usage Examples

### Basic Hybrid Setup

```bash
# 1. Set up coordination API token
export MULTICLAUDE_API_TOKEN="your-token-here"

# 2. Enable hybrid mode and configure API
multiclaude config hybrid \
  --enabled=true \
  --api-url="https://multiclaude-api.example.com"

# 3. Initialize repository (same as before)
multiclaude init https://github.com/org/repo

# 4. Agents now automatically use hybrid placement
# - workspace runs locally
# - supervisor starts locally
# - workers spawn remotely
```

### Viewing Agent Locations

```bash
$ multiclaude work list --show-location

REPO: my-project

Agent           Type         Location   Status
──────────────  ───────────  ─────────  ────────
workspace       workspace    local      active
supervisor      supervisor   local      active
merge-queue     merge-queue  remote     active
eager-badger    worker       remote     busy
calm-penguin    worker       remote     busy
```

### Force Local Execution

```bash
# Spawn a worker locally (overrides default)
multiclaude spawn --task "Quick fix" --local
```

### Debugging Hybrid Issues

```bash
# Check coordination API connectivity
multiclaude config hybrid --test

# View detailed routing info
multiclaude agent list-messages --show-routing

# Check if agent is registered remotely
multiclaude work show eager-badger --registration
```

## Security Considerations

### Authentication

- All API requests require Bearer token authentication
- Tokens should be stored securely (environment variables, secrets manager)
- Tokens can be scoped per-repository or per-team

### Transport Security

- All communication uses HTTPS/TLS
- Client validates server certificates
- Optional mTLS for additional security

### Data Privacy

- Agent names and task descriptions are transmitted
- Code is NOT transmitted through the coordination API
- Git operations happen locally in worktrees (for local agents) or via standard git (for remote)

### Authorization

- Coordination API should implement RBAC
- Users can only access agents for repos they have access to
- Spawn requests validated against user permissions

## Monitoring and Observability

### Metrics

The coordination client exposes metrics:

```go
// Metrics for monitoring
type Metrics struct {
    APIRequestsTotal     prometheus.Counter
    APIRequestDuration   prometheus.Histogram
    CacheHits            prometheus.Counter
    CacheMisses          prometheus.Counter
    MessagesRouted       prometheus.Counter
    HeartbeatsTotal      prometheus.Counter
    SpawnRequestsTotal   prometheus.Counter
}
```

### Logging

```go
// Log levels for hybrid operations
// INFO: Agent registered, message routed, spawn completed
// WARN: Cache miss, retry attempt, fallback to local
// ERROR: API unreachable, auth failed, spawn failed
```

### Health Checks

```bash
# Daemon health endpoint includes hybrid status
curl localhost:8080/health

{
  "status": "healthy",
  "hybrid": {
    "enabled": true,
    "api_reachable": true,
    "last_heartbeat": "2026-01-20T10:30:00Z",
    "local_agents": 2,
    "remote_agents": 5
  }
}
```

## Implementation Phases

### Phase 1: Foundation

- [x] Define coordination types (`types.go`)
- [x] Implement local registry (`registry.go`)
- [ ] Implement coordination client (`client.go`)
- [ ] Add hybrid configuration support
- [ ] Write unit tests

**Deliverable:** Client can communicate with Coordination API

### Phase 2: Integration

- [ ] Integrate client into daemon
- [ ] Extend message router for hybrid routing
- [ ] Add remote message polling
- [ ] Implement spawn requests through client
- [ ] Add CLI commands for hybrid configuration

**Deliverable:** End-to-end hybrid message flow working

### Phase 3: Resilience

- [ ] Implement client-side caching
- [ ] Add fallback-to-local behavior
- [ ] Handle network failures gracefully
- [ ] Add retry logic with backoff
- [ ] Implement health checks

**Deliverable:** Reliable operation even with intermittent connectivity

### Phase 4: Observability

- [ ] Add metrics collection
- [ ] Enhanced logging
- [ ] Health check endpoints
- [ ] Debugging CLI commands

**Deliverable:** Full visibility into hybrid operations

## Testing Strategy

### Unit Tests

- Client methods tested with mock HTTP server
- Cache behavior tested in isolation
- Error handling tested for all failure modes

### Integration Tests

- Full flow with test Coordination API
- Hybrid message routing
- Spawn request handling

### E2E Tests

- Real deployment with local + remote agents
- Network failure scenarios
- Fallback behavior verification

## Open Questions

1. **Should the Coordination API be part of multiclaude?**
   - Option A: Separate service (users deploy their own)
   - Option B: Built into multiclaude as optional component
   - Recommendation: Separate service, multiclaude is the client only

2. **How to handle agent name conflicts?**
   - Local and remote could have same agent name
   - Recommendation: Registry enforces uniqueness across locations

3. **What happens when remote agent goes offline?**
   - Recommendation: Mark unreachable after 3 missed heartbeats, cleanup after 10 minutes

4. **Should we support multiple Coordination APIs?**
   - One per team? One per repo?
   - Recommendation: One per user/team, repos configure which API to use

## References

- [AGENTS.md](../AGENTS.md) - Agent types and lifecycle
- [PRD_REMOTE_NOTIFICATIONS.md](PRD_REMOTE_NOTIFICATIONS.md) - Notification system design
- [coordination/types.go](../internal/coordination/types.go) - Type definitions
- [coordination/registry.go](../internal/coordination/registry.go) - Local registry implementation
