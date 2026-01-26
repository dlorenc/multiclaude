# Implementation Tasks

## 1. Core Infrastructure

- [ ] 1.1 Add `RepoState` enum to state.go (active, hibernated, uninitialized)
- [ ] 1.2 Add `HibernationState` struct to preserve agent configuration
- [ ] 1.3 Add output format types (text, json, yaml) to CLI
- [ ] 1.4 Create `OutputFormatter` interface for consistent formatting

## 2. Repo Start Command

- [ ] 2.1 Implement `repo start` command in cli.go
- [ ] 2.2 Add `--agents` flag for selective agent spawning
- [ ] 2.3 Add daemon socket handler for start operation
- [ ] 2.4 Add idempotency check (skip already running agents)
- [ ] 2.5 Write tests for start command

## 3. Repo Status Command

- [ ] 3.1 Implement `repo status` command in cli.go
- [ ] 3.2 Aggregate data: agents, PRs (via gh), messages, worktree sync
- [ ] 3.3 Implement text formatter with colors
- [ ] 3.4 Implement JSON formatter
- [ ] 3.5 Implement YAML formatter
- [ ] 3.6 Write tests for status command

## 4. Repo Hibernate/Wake Commands

- [ ] 4.1 Implement `repo hibernate` command
- [ ] 4.2 Add graceful agent shutdown with timeout
- [ ] 4.3 Save hibernation state to state.json
- [ ] 4.4 Implement `repo wake` command
- [ ] 4.5 Restore agents from hibernation state
- [ ] 4.6 Add `--fresh` flag for clean wake
- [ ] 4.7 Write tests for hibernate/wake cycle

## 5. Repo Refresh Command

- [ ] 5.1 Implement `repo refresh` command
- [ ] 5.2 Add parallel worktree rebase logic
- [ ] 5.3 Handle conflicts gracefully (continue others)
- [ ] 5.4 Add `--agent` flag for single worktree
- [ ] 5.5 Write tests for refresh command

## 6. Repo Clean Command

- [ ] 6.1 Implement `repo clean` command
- [ ] 6.2 Identify orphaned worktrees (no agent match)
- [ ] 6.3 Add confirmation prompt
- [ ] 6.4 Add `--dry-run` and `--force` flags
- [ ] 6.5 Write tests for clean command

## 7. Repo List Enhancement

- [ ] 7.1 Extend list output with status info
- [ ] 7.2 Add `--format` flag to list command
- [ ] 7.3 Write tests for enhanced list

## 8. TUI Mode (Phase 2)

- [ ] 8.1 Add bubbletea dependency
- [ ] 8.2 Create TUI model for status display
- [ ] 8.3 Implement real-time updates via state watcher
- [ ] 8.4 Add keyboard navigation
- [ ] 8.5 Write TUI tests

## 9. WebSocket Streaming (Phase 2)

- [ ] 9.1 Add WebSocket server to daemon
- [ ] 9.2 Implement status streaming endpoint
- [ ] 9.3 Add client connection management
- [ ] 9.4 Write WebSocket integration tests

## 10. Documentation

- [ ] 10.1 Update CLI docs with new commands
- [ ] 10.2 Add examples to README
- [ ] 10.3 Update COMMANDS.md reference
- [ ] 10.4 Run `go generate ./pkg/config` to regenerate docs
