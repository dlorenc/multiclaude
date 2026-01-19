# Multiclaude Notifications Examples

This directory contains example configurations and implementations for the multiclaude notification system.

## Quick Start

1. Copy the example configuration to your multiclaude config directory:
   ```bash
   cp notifications.yaml ~/.multiclaude/notifications.yaml
   ```

2. Set the required environment variables for your chosen notification service.

3. Restart the multiclaude daemon.

## Configuration Examples

### Slack (Recommended)

**Simple Setup (webhook only):**
- `slack-simple.yaml` - Minimal configuration for Slack notifications

**Setup Steps:**
1. Go to https://api.slack.com/apps
2. Create a new app
3. Enable Incoming Webhooks
4. Create a webhook for your channel
5. Copy the webhook URL to your config

### Telegram

**File:** `telegram.yaml`

**Setup Steps:**
1. Message @BotFather on Telegram
2. Create a new bot with `/newbot`
3. Copy the API token
4. Start a chat with your bot
5. Get your chat ID from: `https://api.telegram.org/bot<TOKEN>/getUpdates`
6. Add the token and chat ID to your config

### Discord

**File:** `discord.yaml`

**Setup Steps:**
1. Go to Server Settings > Integrations > Webhooks
2. Create a new webhook
3. Copy the webhook URL to your config

### Custom Webhook

**File:** `webhook-custom.yaml`

For building your own integration:
- See `webhook-receiver/main.go` for a complete Go implementation
- Webhook payloads are signed with HMAC-SHA256 when a secret is configured
- Use the `X-Multiclaude-Signature` header to verify authenticity

## REST API Dashboard

The notification system includes a REST API for building custom dashboards.

### Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/health` | GET | Health check |
| `/api/v1/events` | GET | List recent events |
| `/api/v1/events/stream` | GET | SSE stream of events |
| `/api/v1/status` | GET | Current status summary |
| `/api/v1/status/{repo}` | GET | Status for specific repo |
| `/api/v1/respond` | POST | Send response to agent |
| `/api/v1/adapters` | GET | List configured adapters |
| `/api/v1/stats` | GET | Notification statistics |

### Example Dashboard

See `dashboard/index.html` for a complete example of a web dashboard that:
- Displays real-time agent status
- Shows notification events
- Uses Server-Sent Events for live updates

To use:
1. Enable the API in your config
2. Open `dashboard/index.html` in a browser
3. Enter the API URL (e.g., `http://localhost:8080`)
4. Click Connect

## Environment Variables

| Variable | Description | Used By |
|----------|-------------|---------|
| `SLACK_WEBHOOK_URL` | Slack incoming webhook URL | Slack |
| `SLACK_BOT_TOKEN` | Slack bot OAuth token | Slack (interactive) |
| `SLACK_SIGNING_SECRET` | Slack request signing secret | Slack (interactive) |
| `TELEGRAM_BOT_TOKEN` | Telegram bot API token | Telegram |
| `TELEGRAM_CHAT_ID` | Telegram chat/group ID | Telegram |
| `DISCORD_WEBHOOK_URL` | Discord webhook URL | Discord |
| `WEBHOOK_URL` | Custom webhook URL | Webhook |
| `WEBHOOK_TOKEN` | Bearer token for webhook auth | Webhook |
| `WEBHOOK_SECRET` | HMAC signing secret | Webhook |
| `API_AUTH_TOKEN` | REST API authentication token | API |

## Event Types

| Event | Priority | Description |
|-------|----------|-------------|
| `agent.question` | High | Agent is asking a question |
| `agent.completed` | Medium | Worker finished their task |
| `agent.stuck` | High | Agent hasn't made progress |
| `agent.error` | High | Agent process crashed |
| `pr.created` | Medium | Pull request was created |
| `pr.merged` | Low | Pull request was merged |
| `ci.failed` | High | CI checks failed |
| `status.update` | Low | Periodic status summary |

## Responding to Agents

For platforms that support it (Slack with bot token, Telegram with webhook, custom webhooks), you can respond to agent questions directly:

**Slack:** Click the "Respond" button in the notification

**Telegram:** Reply to the notification message

**API:** POST to `/api/v1/respond`:
```json
{
  "response_id": "resp-abc123",
  "message": "Your response here"
}
```

**Custom Webhook:** POST to your configured response endpoint with the same payload format.

## Rate Limiting

The notification system includes rate limiting to prevent spam:
- Default: 10 notifications per minute per adapter
- Configurable cooldown after burst
- Automatic deduplication of identical events within 5 minutes

## Quiet Hours

Suppress notifications during specified hours:
```yaml
quiet_hours:
  enabled: true
  start: "22:00"
  end: "08:00"
  timezone: "America/Los_Angeles"
```

## Security Considerations

1. **Use environment variables** for all secrets (tokens, keys, etc.)
2. **Enable signature verification** for webhooks using the `secret` field
3. **Use HTTPS** for all webhook URLs in production
4. **Enable auth tokens** for the REST API in production
5. **Don't include sensitive code** in notification messages - only metadata
