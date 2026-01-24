# Documentation Audit Report

**Auditor:** silly-tiger (worker agent)
**Date:** 2026-01-23
**Scope:** Full documentation audit for accuracy

## Executive Summary

This audit found **significant documentation issues**, particularly in the extension documentation. Several files document features that do not exist in the codebase (event hooks CLI, web dashboard, examples). The core documentation (README, AGENTS, ARCHITECTURE) is mostly accurate but has some outdated CLI references.

---

## Critical Issues (Must Fix)

### 1. docs/extending/EVENT_HOOKS.md - Documents Non-Existent Feature

**Status:** ❌ CRITICAL - Feature does not exist

The entire EVENT_HOOKS.md file documents a `multiclaude hooks` command that does not exist:

```bash
# These commands do not exist:
multiclaude hooks set on_event /path/to/notify-all.sh
multiclaude hooks list
multiclaude hooks clear
```

**CLI Test:**
```
$ ./multiclaude hooks
Usage error: unknown command: hooks
```

**Also references non-existent code:**
- `internal/events/events.go` - File does not exist
- The entire event system described is not implemented

**Recommendation:** Either:
- Remove EVENT_HOOKS.md entirely OR
- Mark it as "PLANNED FEATURE - NOT IMPLEMENTED"

---

### 2. docs/extending/SOCKET_API.md - Documents Non-Existent Socket Commands

**Status:** ❌ Commands Not Verified

Many socket API commands documented may not exist:
- `get_hook_config` / `update_hook_config` - Hook system not implemented
- `route_messages` - Not verified
- `restart_agent` - Not verified

**Recommendation:** Verify each socket command against `internal/daemon/daemon.go` and update documentation

---

### 3. docs/EXTENSIBILITY.md - References Non-Existent Paths

**Status:** ❌ Multiple issues

| Referenced Path | Actual Status |
|----------------|---------------|
| `cmd/multiclaude-web/` | Does not exist |
| `examples/hooks/slack-notify.sh` | Does not exist |
| `internal/dashboard/` | Does not exist |
| `internal/events/events.go` | Does not exist |

**Recommendation:** Remove references to non-existent code or mark as "planned"

---

### 4. docs/extending/WEB_UI_DEVELOPMENT.md - Documents Non-Existent Implementation

**Status:** ❌ Reference implementation does not exist

The document extensively references:
- `cmd/multiclaude-web` - Does not exist
- `internal/dashboard/reader.go` - Does not exist
- `internal/dashboard/api.go` - Does not exist
- `internal/dashboard/server.go` - Does not exist
- `internal/dashboard/web/` - Does not exist

**Note:** This also conflicts with ROADMAP.md which states "No web interfaces or dashboards" is out of scope.

**Recommendation:** Either:
- Remove WEB_UI_DEVELOPMENT.md (aligns with ROADMAP.md) OR
- Mark as "FORK ONLY - Not part of upstream" with appropriate warnings

---

### 5. docs/extending/STATE_FILE_INTEGRATION.md - References Non-Existent Code

**Status:** ⚠️ Contains inaccurate references

Line 740: "See `internal/dashboard/api.go` for a complete implementation" - File does not exist

**Recommendation:** Remove reference to non-existent code

---

## Moderate Issues (Should Fix)

### 6. ARCHITECTURE.md - Outdated CLI Tree

**Status:** ⚠️ CLI tree is outdated

**Documented (lines 283-305):**
```
multiclaude
├── init <url>               # Initialize repo
├── work <task>              # Create worker
│   ├── list                 # List workers
│   └── rm <name>            # Remove worker
...
```

**Actual CLI structure:**
```
multiclaude
├── repo
│   ├── init <url>           # Initialize repo
│   ├── list                 # List repos
│   └── rm <name>            # Remove repo
├── worker                   # (not 'work')
│   ├── create <task>        # Create worker
│   ├── list                 # List workers
│   └── rm <name>            # Remove worker
...
```

**Recommendation:** Update CLI tree to reflect current structure

---

### 7. CLAUDE.md, AGENTS.md - Incorrect Prompt File Locations

**Status:** ⚠️ Misleading information

**Documented locations:**
- `internal/prompts/worker.md`
- `internal/prompts/merge-queue.md`
- `internal/prompts/review.md`

**Actual locations:**
- `internal/templates/agent-templates/worker.md`
- `internal/templates/agent-templates/merge-queue.md`
- `internal/templates/agent-templates/reviewer.md`

Only `supervisor.md` and `workspace.md` are in `internal/prompts/`.

**Files affected:**
- CLAUDE.md (line 26, table references)
- AGENTS.md (line 30: `internal/prompts/supervisor.md` - correct, line 44: `internal/prompts/merge-queue.md` - incorrect, etc.)

**Recommendation:** Update all references to use correct paths

---

### 8. SPEC.md - Implementation Status Outdated

**Status:** ⚠️ Outdated

Lines 337-340 show implementation phases:
```
- **Phase 1: Core Infrastructure** - Complete
- **Phase 2: Daemon & Management** - Complete
- **Phase 3: Claude Integration** - Complete
- **Phase 4: Polish & Refinement** - In Progress
```

**Recommendation:** Update to reflect current state or remove

---

## Minor Issues (Nice to Fix)

### 9. README.md - Minor CLI Discrepancies

**Status:** ⚠️ Minor updates needed

The README shows message commands as:
```bash
multiclaude message send <to> "msg"
```

This is correct but could be clearer that `agent send-message` is an alias.

**Recommendation:** Clarify alias relationships in documentation

---

### 10. CRASH_RECOVERY.md - References `multiclaude agent attach`

**Status:** ✅ Verified correct

Commands like `multiclaude agent attach supervisor` are correct.

---

### 11. README.md - Package tmux references

**Status:** ⚠️ Needs verification

Line 176-179:
```go
client.SendKeysLiteralWithEnter("session", "window", message)
```

**Note:** Function is `SendKeysLiteral` in docs but `SendKeysLiteralWithEnter` in pkg/tmux/README.md. Need to verify correct function name.

---

## Verified as Correct

### ✅ README.md - Core Commands

| Command | Status | Verified |
|---------|--------|----------|
| `multiclaude start` | ✅ Exists | Yes |
| `multiclaude daemon stop` | ✅ Exists | Yes |
| `multiclaude daemon status` | ✅ Exists | Yes |
| `multiclaude daemon logs` | ✅ Exists | Yes |
| `multiclaude repo init <url>` | ✅ Exists | Yes |
| `multiclaude repo list` | ✅ Exists | Yes |
| `multiclaude repo rm <name>` | ✅ Exists | Yes |
| `multiclaude worker create <task>` | ✅ Exists | Yes |
| `multiclaude worker list` | ✅ Exists | Yes |
| `multiclaude worker rm <name>` | ✅ Exists | Yes |
| `multiclaude workspace add` | ✅ Exists | Yes |
| `multiclaude workspace list` | ✅ Exists | Yes |
| `multiclaude workspace connect` | ✅ Exists | Yes |
| `multiclaude workspace rm` | ✅ Exists | Yes |
| `multiclaude agent attach` | ✅ Exists | Yes |
| `multiclaude agent complete` | ✅ Exists | Yes |
| `multiclaude message send` | ✅ Exists | Yes |
| `multiclaude message list` | ✅ Exists | Yes |
| `multiclaude message read` | ✅ Exists | Yes |
| `multiclaude message ack` | ✅ Exists | Yes |
| `multiclaude agents list` | ✅ Exists | Yes |
| `multiclaude agents reset` | ✅ Exists | Yes |
| `multiclaude agents spawn` | ✅ Exists | Yes |
| `multiclaude repair` | ✅ Exists | Yes |
| `multiclaude cleanup` | ✅ Exists | Yes |
| `multiclaude stop-all` | ✅ Exists | Yes |

### ✅ AGENTS.md - Slash Commands

Commands documented in AGENTS.md:
- `/refresh`, `/status`, `/workers`, `/messages`

Verified: These exist in `internal/prompts/commands/` directory.

### ✅ Directory Structure

The directory structure documented in README.md and CLAUDE.md is accurate:
- `~/.multiclaude/daemon.pid` ✅
- `~/.multiclaude/daemon.sock` ✅
- `~/.multiclaude/daemon.log` ✅
- `~/.multiclaude/state.json` ✅
- `~/.multiclaude/repos/<repo>/` ✅
- `~/.multiclaude/wts/<repo>/` ✅
- `~/.multiclaude/messages/<repo>/` ✅
- `~/.multiclaude/claude-config/<repo>/<agent>/` ✅

### ✅ ROADMAP.md

The ROADMAP.md correctly identifies out-of-scope features:
- Multi-provider support
- Remote/hybrid deployment
- Web interfaces or dashboards
- Notification systems
- Plugin/extension systems
- Enterprise features

**Note:** The extension documentation conflicts with this roadmap - it documents web UIs and notification hooks that are explicitly out of scope.

---

## Conflict Analysis: ROADMAP.md vs Extension Docs

There is a significant conflict between:

**ROADMAP.md says:**
> "3. **Web interfaces or dashboards** - No REST APIs for external consumption, No browser-based UIs, Terminal is the interface"
> "4. **Notification systems** (Slack, Discord, webhooks, etc.) - Users can build this themselves if needed"

**Extension docs say:**
> - EXTENSIBILITY.md documents web dashboards as a primary use case
> - EVENT_HOOKS.md documents notification integration in detail
> - WEB_UI_DEVELOPMENT.md provides full implementation guide

**Resolution Options:**
1. Remove the extension docs (align with ROADMAP.md)
2. Mark extension docs as "fork-only" features clearly
3. Update ROADMAP.md to allow extension points

---

## Summary of Fixes Needed

| Priority | File | Issue | Suggested Fix |
|----------|------|-------|---------------|
| CRITICAL | docs/extending/EVENT_HOOKS.md | Feature doesn't exist | Remove or mark as planned |
| CRITICAL | docs/EXTENSIBILITY.md | References non-existent code | Remove invalid references |
| CRITICAL | docs/extending/WEB_UI_DEVELOPMENT.md | Reference impl doesn't exist | Remove or mark as fork-only |
| CRITICAL | docs/extending/SOCKET_API.md | Unverified commands | Verify against daemon.go |
| HIGH | ARCHITECTURE.md | Outdated CLI tree | Update CLI tree |
| HIGH | CLAUDE.md | Wrong prompt file paths | Update paths |
| HIGH | AGENTS.md | Wrong prompt file paths | Update paths |
| MEDIUM | SPEC.md | Outdated status | Update or remove status section |
| LOW | README.md | Minor clarifications | Add alias notes |

---

## Files Audited

1. README.md - ⚠️ Minor issues
2. CLAUDE.md - ⚠️ Wrong prompt paths
3. AGENTS.md - ⚠️ Wrong prompt paths
4. ARCHITECTURE.md - ⚠️ Outdated CLI tree
5. SPEC.md - ⚠️ Outdated status
6. ROADMAP.md - ✅ Accurate
7. docs/CRASH_RECOVERY.md - ✅ Accurate
8. docs/EXTENSIBILITY.md - ❌ References non-existent code
9. docs/extending/EVENT_HOOKS.md - ❌ Feature doesn't exist
10. docs/extending/SOCKET_API.md - ⚠️ Unverified
11. docs/extending/STATE_FILE_INTEGRATION.md - ⚠️ Minor issues
12. docs/extending/WEB_UI_DEVELOPMENT.md - ❌ Reference impl doesn't exist

---

## Recommended Action Plan

1. **Immediate:** Remove or clearly mark EVENT_HOOKS.md, WEB_UI_DEVELOPMENT.md as planned/fork-only
2. **Short-term:** Update ARCHITECTURE.md CLI tree, fix prompt path references
3. **Medium-term:** Verify all socket API commands, update SOCKET_API.md
4. **Ongoing:** Add documentation CI checks to prevent drift

---

*Report generated by documentation audit task*
