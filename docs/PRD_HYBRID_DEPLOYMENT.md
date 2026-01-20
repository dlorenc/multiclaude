# PRD: Hybrid Deployment & Coordination API

**Status:** Approved
**Author:** Claude (fancy-rabbit worker)
**Date:** 2026-01-20

## Problem Statement

Multiclaude currently operates as a single-machine system where the daemon, agents, and tmux sessions all run on one host. This creates several limitations:

1. **Resource constraints**: A single machine may not have enough CPU/memory for many concurrent agents
2. **Network locality**: All agents must access the same local filesystem and git worktrees
3. **Single point of failure**: If the host machine goes down, all agent work stops
4. **Team collaboration**: Multiple developers can't easily share or distribute agent workloads

The current architecture assumes local-only operation via Unix sockets and local tmux sessions. There's no way to coordinate agents across multiple machines or integrate with cloud-based compute resources.

## Goals

1. **Multi-machine coordination**: Allow agents on different machines to work on the same repository
2. **Centralized orchestration**: Provide a coordination service that manages distributed agents
3. **Remote API access**: Enable external systems to observe and interact with agent state
4. **Cloud integration ready**: Architecture that supports future cloud/Kubernetes deployments
5. **Backward compatible**: Existing single-machine deployments continue to work unchanged

## Non-Goals

- Full Kubernetes operator (future work)
- Built-in cloud provider integrations (use generic API)
- Real-time streaming of agent output across network
- Remote daemon execution (each machine runs its own daemon)

## Architecture Overview

### Hybrid Deployment Model

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Coordination Server                                   │
│                    (standalone or embedded in daemon)                         │
│                                                                               │
│  ┌────────────────┐  ┌────────────────┐  ┌──────────────────────────────┐  │
│  │ Registration   │  │ State Sync     │  │ Task Distribution           │  │
│  │ Service        │  │ Service        │  │ Service                      │  │
│  └────────────────┘  └────────────────┘  └──────────────────────────────┘  │
│                              │                                              │
│                              ▼                                              │
│                    ┌──────────────────┐                                    │
│                    │ Shared State     │                                    │
│                    │ (Redis/etcd/etc) │                                    │
│                    └──────────────────┘                                    │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
          ┌─────────────────────────┼─────────────────────────┐
          │                         │                         │
          ▼                         ▼                         ▼
┌─────────────────────┐  ┌─────────────────────┐  ┌─────────────────────┐
│   Machine A         │  │   Machine B         │  │   Machine C         │
│   (Primary)         │  │   (Worker Pool)     │  │   (Worker Pool)     │
│                     │  │                     │  │                     │
│  ┌───────────────┐  │  │  ┌───────────────┐  │  │  ┌───────────────┐  │
│  │ multiclaude   │  │  │  │ multiclaude   │  │  │  │ multiclaude   │  │
│  │ daemon        │  │  │  │ daemon        │  │  │  │ daemon        │  │
│  │               │  │  │  │               │  │  │  │               │  │
│  │ - supervisor  │  │  │  │ - worker-1    │  │  │  │ - worker-4    │  │
│  │ - merge-queue │  │  │  │ - worker-2    │  │  │  │ - worker-5    │  │
│  │ - workspace   │  │  │  │ - worker-3    │  │  │  │ - worker-6    │  │
│  └───────────────┘  │  │  └───────────────┘  │  │  └───────────────┘  │
└─────────────────────┘  └─────────────────────┘  └─────────────────────┘
```

### Deployment Modes

| Mode | Description | Use Case |
|------|-------------|----------|
| **Standalone** | Single machine, no coordination | Individual developer |
| **Primary** | Runs coordinator + core agents | Team lead machine |
| **Worker Pool** | Only runs worker agents | CI runners, cloud VMs |
| **Headless** | No local agents, API only | Dashboard/monitoring |

### Coordination API

The coordination API provides a REST interface for:

1. **Agent Registration**: Worker pools register their availability
2. **Task Distribution**: Primary assigns tasks to available workers
3. **State Synchronization**: Keep agent states consistent across machines
4. **Event Streaming**: SSE for real-time updates
5. **Health Monitoring**: Track liveness of remote agents

## API Design

### Base URL

```
http(s)://<coordinator>:7331/api/v1
```

### Authentication

Bearer token authentication with optional mTLS for production deployments:

```http
Authorization: Bearer <token>
```

### Endpoints

#### Registration

```http
POST /register
Content-Type: application/json

{
  "node_id": "machine-b",
  "hostname": "worker-pool-1.example.com",
  "capacity": {
    "max_workers": 5,
    "current_workers": 2
  },
  "labels": {
    "pool": "gpu",
    "region": "us-west-2"
  }
}
```

Response:
```json
{
  "success": true,
  "data": {
    "registration_id": "reg-abc123",
    "heartbeat_interval_seconds": 30
  }
}
```

#### Heartbeat

```http
POST /heartbeat
Content-Type: application/json

{
  "registration_id": "reg-abc123",
  "status": "healthy",
  "agents": [
    {
      "name": "worker-1",
      "type": "worker",
      "status": "running",
      "task": "Implement auth feature"
    }
  ],
  "metrics": {
    "cpu_percent": 45.2,
    "memory_percent": 62.1
  }
}
```

#### Task Assignment

```http
GET /tasks/pending?labels=pool:gpu
```

Response:
```json
{
  "tasks": [
    {
      "id": "task-xyz789",
      "repo": "my-repo",
      "description": "Implement GPU-accelerated image processing",
      "priority": "high",
      "labels": {
        "pool": "gpu"
      }
    }
  ]
}
```

```http
POST /tasks/{task_id}/claim
Content-Type: application/json

{
  "registration_id": "reg-abc123",
  "worker_name": "worker-3"
}
```

#### State Sync

```http
GET /state/{repo}
```

Response:
```json
{
  "repo": "my-repo",
  "agents": {
    "supervisor": {
      "node": "machine-a",
      "status": "running"
    },
    "worker-1": {
      "node": "machine-b",
      "status": "running",
      "task": "auth feature"
    }
  },
  "pending_tasks": 3,
  "active_prs": 2
}
```

#### Event Stream

```http
GET /events/stream
Accept: text/event-stream
```

Events:
```
event: agent.spawned
data: {"repo":"my-repo","agent":"worker-7","node":"machine-c"}

event: task.completed
data: {"task_id":"task-123","worker":"worker-1","pr":"#42"}

event: node.offline
data: {"node_id":"machine-c","last_seen":"2026-01-20T10:30:00Z"}
```

#### Inter-Agent Messaging

```http
POST /messages
Content-Type: application/json

{
  "from": "worker-1@machine-b",
  "to": "supervisor@machine-a",
  "body": "I need clarification on the API design"
}
```

### Error Responses

```json
{
  "success": false,
  "error": "task already claimed",
  "code": "TASK_CLAIMED",
  "details": {
    "claimed_by": "worker-2@machine-c",
    "claimed_at": "2026-01-20T10:25:00Z"
  }
}
```

## Implementation

### Package Structure

```
internal/coordination/
├── coordinator.go     # Main coordinator logic
├── client.go          # API client for remote daemons
├── server.go          # HTTP server for coordination API
├── types.go           # Shared types and interfaces
├── registry.go        # Node registration management
├── tasks.go           # Task distribution logic
├── sync.go            # State synchronization
└── *_test.go          # Tests
```

### Key Types

```go
// Node represents a registered multiclaude instance
type Node struct {
    ID           string            `json:"id"`
    Hostname     string            `json:"hostname"`
    Capacity     NodeCapacity      `json:"capacity"`
    Labels       map[string]string `json:"labels"`
    Status       NodeStatus        `json:"status"`
    LastSeen     time.Time         `json:"last_seen"`
    Agents       []AgentSummary    `json:"agents"`
}

// Task represents a distributable work item
type Task struct {
    ID          string            `json:"id"`
    Repo        string            `json:"repo"`
    Description string            `json:"description"`
    Priority    Priority          `json:"priority"`
    Labels      map[string]string `json:"labels"`
    ClaimedBy   string            `json:"claimed_by,omitempty"`
    ClaimedAt   time.Time         `json:"claimed_at,omitempty"`
    Status      TaskStatus        `json:"status"`
    CreatedAt   time.Time         `json:"created_at"`
}

// Coordinator manages distributed agent coordination
type Coordinator struct {
    config   *Config
    nodes    *Registry
    tasks    *TaskQueue
    events   *EventBus
    server   *http.Server
}
```

### Client Interface

```go
// Client provides access to a remote coordination server
type Client struct {
    baseURL    string
    httpClient *http.Client
    token      string
    nodeID     string
}

// NewClient creates a new coordination client
func NewClient(baseURL, token string) *Client

// Register registers this node with the coordinator
func (c *Client) Register(ctx context.Context, req *RegisterRequest) (*RegisterResponse, error)

// Heartbeat sends a heartbeat to the coordinator
func (c *Client) Heartbeat(ctx context.Context, status *HeartbeatRequest) error

// GetPendingTasks fetches tasks matching labels
func (c *Client) GetPendingTasks(ctx context.Context, labels map[string]string) ([]Task, error)

// ClaimTask claims a task for execution
func (c *Client) ClaimTask(ctx context.Context, taskID, workerName string) error

// SendMessage sends an inter-agent message
func (c *Client) SendMessage(ctx context.Context, msg *Message) error

// StreamEvents opens an SSE connection for events
func (c *Client) StreamEvents(ctx context.Context) (<-chan Event, error)
```

## Configuration

### Coordinator Configuration

```yaml
# ~/.multiclaude/coordination.yaml

# Enable coordination server
server:
  enabled: true
  listen_addr: ":7331"

  # TLS configuration
  tls:
    enabled: false
    cert_file: "/path/to/cert.pem"
    key_file: "/path/to/key.pem"

  # Authentication
  auth:
    tokens:
      - "${COORD_TOKEN_1}"
      - "${COORD_TOKEN_2}"
    require_auth: true

# Node registration settings
registration:
  heartbeat_interval: 30s
  offline_threshold: 90s

# Task distribution settings
tasks:
  default_priority: medium
  claim_timeout: 5m
```

### Client Configuration

```yaml
# Per-repo or global client config
coordination:
  enabled: true
  server_url: "https://coordinator.example.com:7331"
  token: "${COORD_TOKEN}"

  # This node's identity
  node_id: "machine-b"
  labels:
    pool: "gpu"
    region: "us-west-2"

  # Capacity limits
  max_workers: 5
```

### CLI Commands

```bash
# Check coordinator status
multiclaude coordination status

# List registered nodes
multiclaude coordination nodes

# List pending tasks
multiclaude coordination tasks

# Force re-registration
multiclaude coordination register

# View event stream
multiclaude coordination events
```

## Security Considerations

1. **Authentication**: All API calls require valid bearer token
2. **TLS**: Production deployments should use TLS
3. **Network isolation**: Coordinator should be on private network
4. **Token rotation**: Support multiple tokens for rolling updates
5. **Rate limiting**: Prevent registration spam and DoS
6. **Audit logging**: Log all task claims and state changes

## Migration Path

### Phase 1: API Foundation

- [ ] Create `internal/coordination/` package
- [ ] Implement types and interfaces
- [ ] Build coordination client
- [ ] Add client unit tests
- [ ] Wire into existing daemon (disabled by default)

### Phase 2: Server Implementation

- [ ] Implement coordination HTTP server
- [ ] Add node registration and heartbeat
- [ ] Implement task distribution
- [ ] Add state synchronization
- [ ] Add server unit tests

### Phase 3: Daemon Integration

- [ ] Add coordination config parsing
- [ ] Integrate client into daemon startup
- [ ] Add CLI commands for coordination
- [ ] Integration tests with multiple daemons

### Phase 4: Production Hardening

- [ ] Add TLS support
- [ ] Implement proper auth middleware
- [ ] Add metrics and monitoring
- [ ] Documentation and examples

## Success Metrics

1. **Distribution efficiency**: Ratio of claimed to pending tasks
2. **Node utilization**: Average worker utilization across nodes
3. **Latency**: Time from task creation to claim
4. **Reliability**: Node offline events and recovery time
5. **Adoption**: Number of multi-node deployments

## Open Questions

1. **State backend**: Should we require Redis/etcd or use file-based sync initially?
   - Recommendation: Start with in-memory, add pluggable backends later

2. **Conflict resolution**: What happens if two nodes claim the same task simultaneously?
   - Recommendation: First-write-wins with optimistic locking

3. **Message routing**: How do cross-node agent messages work?
   - Recommendation: Route through coordinator, which forwards to target node

4. **Failure handling**: What happens when a node goes offline mid-task?
   - Recommendation: Mark task as orphaned after timeout, allow re-claim

5. **Git synchronization**: How do multiple machines share git state?
   - Recommendation: Each node has full clone, push/pull through GitHub

## Appendix: Alternative Approaches

### A1: Peer-to-Peer Mesh

**Rejected because:**
- Complex discovery and consistency
- No central point for monitoring
- Harder to reason about task distribution

### A2: Message Queue Based (RabbitMQ/NATS)

**Considered for future:**
- Better for very large scale
- Adds infrastructure dependency
- May be added as optional backend later

### A3: gRPC Instead of REST

**Rejected because:**
- REST is simpler for initial implementation
- Easier to debug with curl
- SSE provides good real-time support
- Can add gRPC later if needed

## References

- [CLAUDE.md](../CLAUDE.md) - Project architecture overview
- [AGENTS.md](../AGENTS.md) - Agent system documentation
- [Notification API](../internal/notify/api.go) - Similar API pattern
- [Socket IPC](../internal/socket/socket.go) - Existing IPC implementation
