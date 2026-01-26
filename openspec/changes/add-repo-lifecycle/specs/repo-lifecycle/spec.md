# Repo Lifecycle Management

## ADDED Requirements

### Requirement: Repo Start Command
The system SHALL provide a `repo start [name]` command that initializes all standard agents for a repository.

#### Scenario: Start repo with default agents
- **WHEN** user runs `multiclaude repo start myrepo`
- **THEN** system spawns supervisor, merge-queue, and workspace agents
- **AND** all agents are running in tmux session `mc-myrepo`
- **AND** command returns success with agent summary

#### Scenario: Start repo already running
- **WHEN** user runs `multiclaude repo start myrepo` on running repo
- **THEN** system reports current status without spawning duplicates
- **AND** suggests using `repo status` for detailed view

#### Scenario: Start with specific agents
- **WHEN** user runs `multiclaude repo start myrepo --agents=supervisor,workspace`
- **THEN** system spawns only specified agents
- **AND** merge-queue is not started

### Requirement: Repo Status Command
The system SHALL provide a `repo status [name]` command that displays comprehensive repository state.

#### Scenario: Full status display
- **WHEN** user runs `multiclaude repo status myrepo`
- **THEN** system displays:
  - Agent list with type, status, task, last activity
  - Open PRs with mergeable state and CI status
  - Pending messages count per agent
  - Worktree sync status (ahead/behind main)
  - Health indicators

#### Scenario: Status with JSON output
- **WHEN** user runs `multiclaude repo status myrepo --format=json`
- **THEN** system outputs structured JSON with all status fields
- **AND** output is machine-parseable

#### Scenario: Status with YAML output
- **WHEN** user runs `multiclaude repo status myrepo --format=yaml`
- **THEN** system outputs YAML-formatted status
- **AND** output follows standard YAML conventions

#### Scenario: Status in TUI mode
- **WHEN** user runs `multiclaude repo status myrepo --tui`
- **THEN** system launches interactive terminal UI
- **AND** UI updates in real-time as state changes
- **AND** user can navigate with keyboard

### Requirement: Repo Hibernate Command
The system SHALL provide a `repo hibernate [name]` command that pauses all agents while preserving state.

#### Scenario: Hibernate active repo
- **WHEN** user runs `multiclaude repo hibernate myrepo`
- **THEN** system gracefully stops all agents
- **AND** agent state is saved to disk
- **AND** worktrees are preserved
- **AND** messages are preserved
- **AND** repo is marked as hibernated in state.json

#### Scenario: Hibernate with timeout
- **WHEN** user runs `multiclaude repo hibernate myrepo --timeout=30s`
- **THEN** system waits up to 30s for graceful shutdown
- **AND** force-kills agents after timeout

#### Scenario: Hibernate already hibernated repo
- **WHEN** user runs `multiclaude repo hibernate myrepo` on hibernated repo
- **THEN** system reports repo is already hibernated
- **AND** no changes are made

### Requirement: Repo Wake Command
The system SHALL provide a `repo wake [name]` command that resumes a hibernated repository.

#### Scenario: Wake hibernated repo
- **WHEN** user runs `multiclaude repo wake myrepo`
- **THEN** system restores all previously active agents
- **AND** agents resume with their saved state
- **AND** messages are delivered to awakened agents
- **AND** repo is marked as active

#### Scenario: Wake with fresh state
- **WHEN** user runs `multiclaude repo wake myrepo --fresh`
- **THEN** system starts default agents (supervisor, merge-queue, workspace)
- **AND** previous agent state is discarded

#### Scenario: Wake non-hibernated repo
- **WHEN** user runs `multiclaude repo wake myrepo` on active repo
- **THEN** system reports repo is already active
- **AND** suggests using `repo status`

### Requirement: Repo Refresh Command
The system SHALL provide a `repo refresh [name]` command that syncs all worktrees with main branch.

#### Scenario: Refresh all worktrees
- **WHEN** user runs `multiclaude repo refresh myrepo`
- **THEN** system fetches latest from remote
- **AND** rebases each worktree onto main
- **AND** reports success/failure per worktree

#### Scenario: Refresh with conflicts
- **WHEN** user runs `multiclaude repo refresh myrepo` and conflicts exist
- **THEN** system reports which worktrees have conflicts
- **AND** provides resolution guidance
- **AND** does not abort other worktrees

#### Scenario: Refresh specific worktree
- **WHEN** user runs `multiclaude repo refresh myrepo --agent=worker-1`
- **THEN** system refreshes only that agent's worktree

### Requirement: Repo Clean Command
The system SHALL provide a `repo clean [name]` command that removes orphaned resources.

#### Scenario: Clean orphaned worktrees
- **WHEN** user runs `multiclaude repo clean myrepo`
- **THEN** system identifies worktrees without active agents
- **AND** prompts for confirmation
- **AND** removes orphaned worktrees

#### Scenario: Clean with dry-run
- **WHEN** user runs `multiclaude repo clean myrepo --dry-run`
- **THEN** system lists what would be cleaned
- **AND** does not remove anything

#### Scenario: Clean with force
- **WHEN** user runs `multiclaude repo clean myrepo --force`
- **THEN** system removes orphaned resources without confirmation

### Requirement: Output Format Options
The system SHALL support multiple output formats for all repo commands.

#### Scenario: Text output (default)
- **WHEN** user runs any repo command without --format flag
- **THEN** output is human-readable text with formatting
- **AND** uses colors when terminal supports it

#### Scenario: JSON output
- **WHEN** user runs repo command with `--format=json`
- **THEN** output is valid JSON
- **AND** includes all data fields
- **AND** is suitable for piping to jq

#### Scenario: YAML output
- **WHEN** user runs repo command with `--format=yaml`
- **THEN** output is valid YAML
- **AND** uses standard YAML formatting

#### Scenario: TUI mode
- **WHEN** user runs repo command with `--tui`
- **THEN** launches interactive terminal interface
- **AND** interface supports keyboard navigation
- **AND** updates in real-time for status commands

#### Scenario: WebSocket streaming
- **WHEN** user runs `multiclaude repo status --websocket=:8080`
- **THEN** system starts WebSocket server on port 8080
- **AND** streams status updates as JSON messages
- **AND** clients can connect and receive updates

### Requirement: Repo List Enhancement
The system SHALL enhance `repo list` with status information and output formats.

#### Scenario: List with status
- **WHEN** user runs `multiclaude repo list`
- **THEN** output includes for each repo:
  - Name and GitHub URL
  - Status (active/hibernated/uninitialized)
  - Agent count and types
  - Open PR count
  - Last activity timestamp

#### Scenario: List with format option
- **WHEN** user runs `multiclaude repo list --format=json`
- **THEN** output is JSON array of repo objects
- **AND** each object contains full status information
