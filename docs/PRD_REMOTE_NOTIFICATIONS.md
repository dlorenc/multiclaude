# PRD: Remote Notifications & Interaction

**Status:** Draft
**Author:** Claude (eager-badger worker)
**Date:** 2026-01-19

## Problem Statement

Multiclaude operators currently have no way to monitor agent activity or respond to agent questions without being physically attached to a tmux session. This creates several pain points:

1. **Blocked agents go unnoticed**: Workers asking questions to the supervisor can wait indefinitely if no one is watching the tmux session
2. **No mobile access**: Can't check on progress from a phone while away from the computer
3. **Delayed awareness**: Task completions, failures, or stalls aren't visible until you manually check
4. **Context switching**: Must switch to terminal to see what's happening, breaking flow in other work

The current architecture relies on tmux for observability, which works great when you're at your computer but provides zero visibility otherwise.

## Goals

1. **Real-time awareness**: Know when agents need attention without watching tmux
2. **Mobile-friendly**: Monitor and respond from any device
3. **Low friction**: Trivial to set up, no complex infrastructure
4. **Bi-directional**: Not just notifications, but ability to respond to agent questions
5. **Non-invasive**: Should integrate with existing message system, not replace it

## Non-Goals

- Building a web dashboard (violates terminal-first philosophy)
- Remote daemon operation (out of scope per DESIGN.md)
- Replacing tmux as the primary interface
- Real-time streaming of agent output (too noisy, too much data)

## User Stories

### As a multiclaude operator, I want to...

1. **Get notified when a worker asks the supervisor a question**, so I can respond quickly and unblock them
2. **Get notified when a task completes**, so I know to review the PR
3. **Get notified when an agent appears stuck**, so I can investigate before too much time is wasted
4. **Respond to agent questions from my phone**, so agents aren't blocked when I'm away from my desk
5. **See a summary of current agent status**, so I can quickly understand what's happening across all tasks
6. **Mute notifications temporarily**, so I'm not disturbed during focused work

## Proposed Solution

### Overview

Add a **notification adapter system** to the daemon that can push events to external services (Slack, Telegram, Discord, webhooks) and receive responses. The adapter watches for specific events in the message system and state changes, then forwards them to configured channels.

### Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│  DAEMON                                                          │
│                                                                  │
│  ┌─────────────┐    ┌──────────────────┐    ┌────────────────┐ │
│  │ Message     │───▶│ Notification     │───▶│ Slack Adapter  │ │
│  │ Router      │    │ Hub (new)        │    └────────────────┘ │
│  └─────────────┘    │                  │    ┌────────────────┐ │
│                     │  - Event filter  │───▶│ Telegram Adptr │ │
│  ┌─────────────┐    │  - Rate limit    │    └────────────────┘ │
│  │ Health      │───▶│  - Dedup         │    ┌────────────────┐ │
│  │ Check       │    │                  │───▶│ Webhook Adapter│ │
│  └─────────────┘    └──────────────────┘    └────────────────┘ │
│                            ▲                        │           │
│  ┌─────────────┐           │                        ▼           │
│  │ State       │───────────┘           ┌────────────────────┐  │
│  │ Changes     │                       │ Response Handler   │  │
│  └─────────────┘                       │ (webhook receiver) │  │
│                                        └────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

### Events to Notify On

| Event | Priority | Description |
|-------|----------|-------------|
| `agent.question` | High | Agent sent a message asking for help/guidance |
| `agent.completed` | Medium | Worker signaled completion via `multiclaude agent complete` |
| `agent.stuck` | High | Agent hasn't made progress in configurable time window |
| `agent.error` | High | Agent process died or crashed |
| `pr.created` | Medium | Agent created a pull request |
| `pr.merged` | Low | Merge queue successfully merged a PR |
| `ci.failed` | High | CI failed on an agent's PR |

### Notification Content

Each notification should include:
- **Agent name** (e.g., "eager-badger")
- **Repository name**
- **Event type** with appropriate emoji
- **Context** (the question being asked, PR link, error details)
- **Action buttons** where supported (e.g., Slack buttons to respond)

Example Slack notification:
```
🙋 eager-badger needs help (multiclaude/my-repo)

"Should I update the React version to 19, or keep it at 18 for
compatibility? There are some breaking changes in the upgrade."

[Respond] [View in tmux] [Dismiss]
```

### Response Handling

For platforms that support it, enable responding directly:

1. **Slack**: Use interactive messages with text input
2. **Telegram**: Reply to the message
3. **Discord**: React or reply
4. **Webhook**: POST back to a configurable endpoint

Responses get injected into the multiclaude message system:
```
User replies in Slack → Webhook to daemon → Message to agent → Delivered via tmux
```

## Configuration

### File Location

`~/.multiclaude/notifications.yaml` (or per-repo in `.multiclaude/notifications.yaml`)

### Configuration Schema

```yaml
# Global enable/disable
enabled: true

# Notification channels (can have multiple)
channels:
  - type: slack
    name: "team-notifications"
    webhook_url: "${SLACK_WEBHOOK_URL}"  # env var support
    channel: "#multiclaude-alerts"
    # Optional: bot token for interactive features
    bot_token: "${SLACK_BOT_TOKEN}"

  - type: telegram
    name: "personal-telegram"
    bot_token: "${TELEGRAM_BOT_TOKEN}"
    chat_id: "${TELEGRAM_CHAT_ID}"

  - type: webhook
    name: "custom-webhook"
    url: "https://my-service.com/multiclaude"
    headers:
      Authorization: "Bearer ${WEBHOOK_TOKEN}"

# Event filtering
events:
  agent.question:
    enabled: true
    channels: ["team-notifications", "personal-telegram"]
  agent.completed:
    enabled: true
    channels: ["team-notifications"]
  agent.stuck:
    enabled: true
    # Delay before notification (gives agent chance to unstick)
    delay_minutes: 10
    channels: ["personal-telegram"]
  agent.error:
    enabled: true
    channels: ["team-notifications", "personal-telegram"]

# Quiet hours (optional)
quiet_hours:
  enabled: false
  start: "22:00"
  end: "08:00"
  timezone: "America/Los_Angeles"

# Rate limiting
rate_limit:
  max_per_minute: 10
  cooldown_after_burst: 60  # seconds
```

### CLI Commands

```bash
# Test notification configuration
multiclaude notify test

# Send custom notification
multiclaude notify send "Heads up: starting large refactor"

# Mute notifications temporarily
multiclaude notify mute 2h

# Check notification status
multiclaude notify status
```

## Implementation Options

### Option A: Slack-First (Recommended)

**Scope:** Implement Slack integration only, with generic webhook as fallback.

**Pros:**
- Slack is the most commonly requested platform
- Rich interactive features (buttons, threads, modals)
- Well-documented API with official Go SDK
- Can handle responses natively

**Cons:**
- Slack-specific initially
- Requires Slack workspace admin access for full features

**Effort:** Medium

### Option B: Generic Webhook Only

**Scope:** Implement generic webhook system, let users build their own integrations.

**Pros:**
- Maximum flexibility
- Works with any platform that accepts webhooks
- Simplest implementation

**Cons:**
- No built-in response handling
- Users must build their own integration
- Less polished experience

**Effort:** Low

### Option C: Multi-Platform (Slack + Telegram + Discord)

**Scope:** Build adapters for all major platforms from the start.

**Pros:**
- Covers most user preferences
- Consistent experience across platforms

**Cons:**
- Highest implementation effort
- More maintenance burden
- Feature parity across platforms is hard

**Effort:** High

### Recommendation: Option A (Slack-First)

Start with Slack integration plus generic webhooks. Slack covers the majority of professional use cases, and webhooks provide an escape hatch for other platforms. Additional adapters (Telegram, Discord) can be added incrementally based on demand.

## Detailed Design: Slack Integration

### Components

1. **SlackNotifier** (`internal/notify/slack.go`)
   - Sends messages via Slack Incoming Webhooks
   - Formats events into Slack Block Kit messages
   - Handles rate limiting

2. **SlackInteractiveHandler** (`internal/notify/slack_interactive.go`)
   - HTTP server to receive Slack interactions
   - Verifies Slack request signatures
   - Routes button clicks and form submissions

3. **NotificationHub** (`internal/notify/hub.go`)
   - Central event bus for all notification events
   - Deduplication and rate limiting
   - Routes events to configured adapters

### Slack App Setup (User Docs)

1. Create Slack App at api.slack.com
2. Enable Incoming Webhooks
3. (Optional) Add Bot Token for responses
4. (Optional) Configure Interactive Components
5. Copy webhook URL to config

### Minimal Slack Setup (Just Notifications)

For users who just want notifications without responses:

1. Create Incoming Webhook (takes 2 minutes)
2. Add webhook URL to config
3. Done

### Full Slack Setup (With Responses)

For bi-directional communication:

1. Create Slack App with Bot Token
2. Configure OAuth & Permissions
3. Enable Interactive Components with callback URL
4. Add Socket Mode or expose webhook endpoint
5. Configure in multiclaude

## Implementation Phases

### Phase 1: Foundation (MVP)

- [ ] Create `internal/notify/` package
- [ ] Implement NotificationHub with event types
- [ ] Add configuration parsing for notifications.yaml
- [ ] Implement Slack webhook notifier (outgoing only)
- [ ] Add generic webhook adapter
- [ ] Wire hub to daemon event sources (messages, health checks)
- [ ] Add `multiclaude notify test` command

**Deliverable:** Outgoing notifications to Slack/webhooks

### Phase 2: Rich Notifications

- [ ] Slack Block Kit formatting for all event types
- [ ] Include relevant context (PR links, agent status)
- [ ] Add quiet hours support
- [ ] Add rate limiting
- [ ] Add `multiclaude notify mute` command

**Deliverable:** Polished notification experience

### Phase 3: Bi-Directional (Slack)

- [ ] Implement Slack interactive message handler
- [ ] Add response routing to message system
- [ ] Handle button clicks for common actions
- [ ] Add "View in tmux" deep link (local machine only)

**Deliverable:** Respond to agents from Slack

### Phase 4: Additional Platforms (Optional)

- [ ] Telegram adapter
- [ ] Discord adapter
- [ ] SMS via Twilio (for critical alerts)

## Security Considerations

1. **Secrets management**: Support environment variables for tokens
2. **Webhook validation**: Verify Slack signatures for incoming webhooks
3. **Rate limiting**: Prevent notification spam
4. **Data exposure**: Don't include sensitive code in notifications, just metadata
5. **HTTPS only**: Require HTTPS for webhook endpoints

## Success Metrics

1. **Time to unblock**: Reduce time between agent question and human response
2. **Adoption**: Number of users who enable notifications
3. **Response rate**: Percentage of agent questions answered via remote channel
4. **False positive rate**: Notifications that didn't require action

## Open Questions

1. **Should responses go through supervisor or directly to worker?**
   - Option A: Always route through supervisor (maintains hierarchy)
   - Option B: Direct to worker for speed (supervisor sees it too)
   - Recommendation: Route through supervisor, it's the existing pattern

2. **How to handle multiple operators?**
   - If multiple people receive notifications, who responds?
   - Recommendation: First response wins, others see it was handled

3. **Should we expose daemon on network for webhooks?**
   - Current daemon is local-only via Unix socket
   - Options: Expose HTTP port, use ngrok, use Slack Socket Mode
   - Recommendation: Start with Slack Socket Mode (no port exposure needed)

4. **What about cost tracking?**
   - Could include cost-so-far in notifications
   - Depends on whether cost tracking is implemented elsewhere
   - Recommendation: Out of scope for v1, add later if cost tracking exists

## Appendix: Alternative Approaches Considered

### A1: Email Notifications

**Rejected because:**
- Email is too slow for real-time awareness
- No good way to respond quickly
- Would feel dated compared to Slack/Telegram

### A2: macOS/Linux Desktop Notifications

**Rejected because:**
- Only works when at the computer (defeats the purpose)
- Could be added as a complementary feature later

### A3: Mobile App

**Rejected because:**
- Massive implementation effort
- Would need iOS and Android apps
- Overkill for the use case

### A4: Polling-Based Web UI

**Rejected because:**
- Violates terminal-first philosophy
- Would need to host somewhere
- Adds deployment complexity

## References

- [Slack Block Kit Builder](https://app.slack.com/block-kit-builder)
- [Slack Incoming Webhooks](https://api.slack.com/messaging/webhooks)
- [Slack Socket Mode](https://api.slack.com/apis/connections/socket)
- [Telegram Bot API](https://core.telegram.org/bots/api)
- [Discord Webhooks](https://discord.com/developers/docs/resources/webhook)
