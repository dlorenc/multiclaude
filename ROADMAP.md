# Multiclaude Roadmap

This document defines the project direction. **All work must align with this roadmap.**

Agents (supervisor, merge-queue, workers) should reject or deprioritize work that doesn't fit.

## Mission

**Multiclaude is a lightweight local orchestrator for running multiple Claude Code agents on GitHub repositories.**

Key constraints:
- **Local-first**: No cloud dependencies, remote coordination, or external services
- **Claude-only**: No multi-provider abstraction. We use Claude Code CLI directly.
- **Simple**: Prefer deleting code over adding complexity
- **Terminal-native**: No web dashboards, GUIs, or browser-based interfaces

## Current Phase: Stabilization

Focus: Make the core experience rock-solid before adding features.

### P0 - Must Have (blocking other work)

- [ ] **Reliable worker lifecycle**: Workers should start, complete, and clean up without manual intervention
- [ ] **Worktree sync**: Keep agent worktrees in sync with main as PRs merge
- [ ] **Clear error messages**: Every failure should tell the user what went wrong and how to fix it

### P1 - Should Have (this quarter)

- [ ] **Task history**: Track what workers have done and their outcomes (PR merged/closed/pending)
- [ ] **Agent restart**: Gracefully restart crashed agents without losing context
- [ ] **Workspace refresh**: Easy command to sync workspace with latest main

### P2 - Nice to Have (backlog)

- [ ] **Better onboarding**: Improve first-run experience and documentation
- [ ] **Agent metrics**: Simple stats on agent activity (tasks completed, PRs created)
- [ ] **Selective wakeup**: Only wake agents when there's work to do

## Out of Scope (Do Not Implement)

These features are explicitly **not wanted**. PRs implementing them should be closed:

1. **Multi-provider support** (e.g., OpenAI, Gemini, other LLMs)
   - We are Claude-only. Period.

2. **Remote/hybrid deployment**
   - No cloud coordination, remote agents, or distributed orchestration
   - Multiclaude runs locally on one machine

3. **Web interfaces or dashboards**
   - No REST APIs for external consumption
   - No browser-based UIs
   - Terminal is the interface

4. **Notification systems** (Slack, Discord, webhooks, etc.)
   - Users can build this themselves if needed
   - Not a core responsibility of the orchestrator

5. **Plugin/extension systems**
   - Keep the codebase simple and integrated
   - No dynamic loading or third-party extensions

6. **Enterprise features** (SSO, audit logs, role-based access)
   - This is a developer tool, not an enterprise platform

## How to Use This Roadmap

### For the Supervisor
- Assign work from P0 first, then P1, then P2
- Reject or close issues requesting out-of-scope features
- When in doubt, ask: "Does this make the core experience better?"

### For the Merge Queue
- Merge PRs that advance roadmap items
- Flag PRs that introduce out-of-scope features for human review
- Don't merge "improvements" that add complexity without roadmap justification

### For Workers
- Focus on the task assigned, don't expand scope
- If you notice your task conflicts with the roadmap, stop and notify supervisor

### For Humans
- Update this roadmap as priorities change
- Mark items complete when done
- Move items between priority levels as needed

## Changelog

- **2026-01-20**: Initial roadmap after Phase 1 cleanup (removed notifications, coordination, multi-provider)
