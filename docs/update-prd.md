# PRD: Self-Update Feature for Multiclaude

## Overview

This document describes the design and implementation of self-update functionality for multiclaude, enabling users to check for and install updates without manual intervention.

## Goals

1. **Automatic Update Notification**: Users should be informed when a new version is available
2. **Easy Update Process**: A single command (`multiclaude update`) should handle the entire update process
3. **Graceful Daemon Handling**: The update process should cleanly stop the daemon, install the new version, and optionally restart it
4. **Minimal User Friction**: The update process should be as seamless as possible

## Non-Goals

- Auto-updating without user consent (updates are user-initiated)
- Supporting non-`go install` installation methods (homebrew, apt, etc.) in this iteration
- Rollback functionality (can be added later)

## Design

### Update Check (Daemon Loop)

The daemon includes an update check loop that runs every 30 minutes:

1. Uses `go list -m -u -json github.com/dlorenc/multiclaude@latest` to check for updates
2. Compares the latest available version with the current running version
3. Stores the result in the daemon state (`state.json`)
4. Logs a message when an update is available

### Update Status Command

`multiclaude daemon status` (or a new `multiclaude update --check` flag) displays:
- Current version
- Latest available version
- Whether an update is available
- When the last check was performed

### Update Command

`multiclaude update` performs the following steps:

```
┌────────────────────────────────────────────────────────────────┐
│                    multiclaude update                          │
└─────────────────────────────┬──────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ 1. Check for updates                                            │
│    - Query latest version from Go proxy                         │
│    - Compare with current version                               │
└─────────────────────────────┬───────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ 2. Verify installation method                                   │
│    - Check if binary is under GOPATH/bin                        │
│    - Fail gracefully if not go-installed                        │
└─────────────────────────────┬───────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ 3. Stop daemon gracefully                                       │
│    - Send stop command via socket                               │
│    - Wait for daemon to terminate                               │
└─────────────────────────────┬───────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ 4. Install new version                                          │
│    - Run: go install github.com/dlorenc/multiclaude/cmd/...@latest │
│    - Verify installation succeeded                              │
└─────────────────────────────┬───────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ 5. (Optional) Restart daemon                                    │
│    - If daemon was running, restart with new binary             │
│    - New binary handles all agent restoration automatically     │
└─────────────────────────────┴───────────────────────────────────┘
```

### Command Options

```
multiclaude update [options]

Options:
  --check           Only check for updates, don't install
  --yes, -y         Skip confirmation prompt
  --no-restart      Don't restart daemon after update
  --force           Update even if already on latest version
```

### Limitations and Considerations

#### Binary Replacement Challenges

1. **Running Binary**: On some systems (notably Windows), a running binary cannot be replaced. On Unix systems, the binary can be replaced while running because the OS keeps the old binary in memory until the process exits.

2. **Process Lifecycle**: The `multiclaude update` command itself is running the old binary. After `go install` completes, the new binary is on disk, but the current process continues with the old code in memory. This is handled by:
   - Stopping the daemon (separate process) before update
   - Restarting the daemon after update uses the new binary
   - The CLI command completes using the old binary, but subsequent commands use the new one

3. **Daemon Restart**: When the daemon is restarted after update, it:
   - Loads from the new binary location
   - Restores all tracked repositories
   - Recreates agent windows (supervisor, merge-queue, workspace)
   - Existing workers with active PRs remain as tmux windows but need manual restart

#### Installation Method Detection

The update command only works for `go install` installations:

```go
// Check if binary is under GOPATH/bin
func (u *Updater) CanUpdate() (bool, string) {
    currentExe, _ := os.Executable()
    gopath := os.Getenv("GOPATH")
    if gopath == "" {
        gopath = filepath.Join(os.UserHomeDir(), "go")
    }
    goBin := filepath.Join(gopath, "bin")

    if !strings.HasPrefix(currentExe, goBin) {
        return false, "binary not installed via 'go install'"
    }
    return true, ""
}
```

For other installation methods:
- **Homebrew**: Users should use `brew upgrade multiclaude`
- **apt/deb**: Users should use their package manager
- **Manual download**: Users should re-download from releases

### State Tracking

Update status is stored in `~/.multiclaude/state.json`:

```json
{
  "update_status": {
    "last_checked": "2025-01-20T12:00:00Z",
    "current_version": "v0.1.0",
    "latest_version": "v0.2.0",
    "update_available": true,
    "last_error": ""
  }
}
```

### User Experience

#### Update Available Notification

When an update is available and the user runs any multiclaude command:

```
$ multiclaude list
Notice: Update available (v0.1.0 → v0.2.0). Run 'multiclaude update' to upgrade.

repo-1  Running  3 workers
repo-2  Running  1 worker
```

#### Update Flow

```
$ multiclaude update
Checking for updates...
Current version: v0.1.0
Latest version:  v0.2.0

An update is available. Proceed? [Y/n] y

Stopping daemon...
Installing multiclaude v0.2.0...
Successfully installed v0.2.0

Restarting daemon...
Daemon started successfully.

Update complete! You are now running multiclaude v0.2.0
```

#### No Update Available

```
$ multiclaude update
Checking for updates...
You are already running the latest version (v0.2.0)
```

## Future Enhancements

1. **Rollback Support**: Store the previous version and provide `multiclaude update --rollback`
2. **Release Notes**: Fetch and display release notes for the new version
3. **Scheduled Updates**: Allow configuring automatic update installation during off-hours
4. **Package Manager Support**: Detect and use the correct update method (brew, apt, etc.)
5. **Binary Signature Verification**: Verify downloaded binaries are signed by Anthropic

## Implementation Status

- [x] Update checker package (`internal/update`)
- [x] Daemon update check loop (30-minute interval)
- [x] Update status in state
- [x] Socket handlers for update status
- [ ] `multiclaude update` CLI command
- [ ] Update notification on CLI commands
- [ ] Tests for update functionality
