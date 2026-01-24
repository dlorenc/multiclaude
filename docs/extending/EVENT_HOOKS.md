# Event Hooks Integration Guide

> **WARNING: THIS FEATURE IS NOT IMPLEMENTED**
>
> This document describes a **planned feature** that does not exist in the current codebase.
> The `multiclaude hooks` command does not exist, and `internal/events/events.go` has not been implemented.
> Per ROADMAP.md, notification systems are explicitly out of scope for upstream multiclaude.
>
> This document is preserved for potential fork implementations.

**Extension Point:** Event-driven notifications via hook scripts (PLANNED - NOT IMPLEMENTED)

This guide documents a **planned** event system for building notification integrations (Slack, Discord, email, custom webhooks) using hook scripts.

## Overview

Multiclaude emits events at key lifecycle points and executes user-provided hook scripts when these events occur. Your hook receives event data as JSON via stdin and can do anything: send notifications, update external systems, trigger workflows, log to external services.

**Philosophy:**
- **Hook-based, not built-in**: Notifications belong in user scripts, not core daemon
- **Fire-and-forget**: No retries, no delivery guarantees (hooks timeout after 30s)
- **Zero dependencies**: Core only emits events; notification logic is yours
- **Unix philosophy**: multiclaude emits JSON, you compose the rest

## Event Types

| Event Type | When It Fires | Common Use Cases |
|------------|---------------|------------------|
| `agent_started` | Agent starts (supervisor, worker, merge-queue, etc.) | Log agent activity, send startup notifications |
| `agent_stopped` | Agent stops (completed, failed, or killed) | Track completion, alert on failures |
| `agent_idle` | Agent has been idle for a threshold period | Detect stuck workers, send reminders |
| `agent_failed` | Agent crashes or fails | Alert on-call, create incidents |
| `pr_created` | Worker creates a PR | Notify team, update project board |
| `pr_merged` | PR is merged | Celebrate wins, update metrics |
| `pr_closed` | PR is closed without merging | Track rejected work |
| `task_assigned` | Task is assigned to a worker | Track work distribution |
| `task_complete` | Task completes (success or failure) | Update project management tools |
| `ci_failed` | CI checks fail on a PR | Alert author, create follow-up task |
| `ci_passed` | CI checks pass on a PR | Auto-merge if configured |
| `worker_stuck` | Worker hasn't made progress in N minutes | Alert supervisor, offer to restart |
| `message_sent` | Inter-agent message is sent | Debug message flow, log conversations |

## Event JSON Format

All hooks receive events via stdin as JSON:

```json
{
  "type": "pr_created",
  "timestamp": "2024-01-15T10:30:00Z",
  "repo_name": "my-repo",
  "agent_name": "clever-fox",
  "data": {
    "pr_number": 42,
    "title": "Add user authentication",
    "url": "https://github.com/user/repo/pull/42"
  }
}
```

### Common Fields

- `type` (string): Event type (see table above)
- `timestamp` (ISO 8601 string): When the event occurred
- `repo_name` (string, optional): Repository name (if event is repo-specific)
- `agent_name` (string, optional): Agent name (if event is agent-specific)
- `data` (object): Event-specific data

### Event-Specific Data

#### agent_started

```json
{
  "type": "agent_started",
  "repo_name": "my-repo",
  "agent_name": "clever-fox",
  "data": {
    "agent_type": "worker",
    "task": "Add user authentication"
  }
}
```

#### agent_stopped

```json
{
  "type": "agent_stopped",
  "repo_name": "my-repo",
  "agent_name": "clever-fox",
  "data": {
    "reason": "completed"  // "completed" | "failed" | "killed"
  }
}
```

#### agent_idle

```json
{
  "type": "agent_idle",
  "repo_name": "my-repo",
  "agent_name": "clever-fox",
  "data": {
    "duration_seconds": 1800  // 30 minutes
  }
}
```

#### pr_created

```json
{
  "type": "pr_created",
  "repo_name": "my-repo",
  "agent_name": "clever-fox",
  "data": {
    "pr_number": 42,
    "title": "Add user authentication",
    "url": "https://github.com/user/repo/pull/42"
  }
}
```

#### pr_merged

```json
{
  "type": "pr_merged",
  "repo_name": "my-repo",
  "data": {
    "pr_number": 42,
    "title": "Add user authentication"
  }
}
```

#### task_assigned

```json
{
  "type": "task_assigned",
  "repo_name": "my-repo",
  "agent_name": "clever-fox",
  "data": {
    "task": "Add user authentication"
  }
}
```

#### ci_failed

```json
{
  "type": "ci_failed",
  "repo_name": "my-repo",
  "data": {
    "pr_number": 42,
    "job_name": "test-suite"
  }
}
```

#### worker_stuck

```json
{
  "type": "worker_stuck",
  "repo_name": "my-repo",
  "agent_name": "clever-fox",
  "data": {
    "duration_minutes": 30
  }
}
```

#### message_sent

```json
{
  "type": "message_sent",
  "repo_name": "my-repo",
  "data": {
    "from": "supervisor",
    "to": "clever-fox",
    "message_type": "task_assignment",
    "body": "Please review the auth implementation"
  }
}
```

## Hook Configuration

### Available Hooks

| Hook Name | Fires On | Priority |
|-----------|----------|----------|
| `on_event` | **All events** (catch-all) | Lower |
| `on_agent_started` | `agent_started` events | Higher |
| `on_agent_stopped` | `agent_stopped` events | Higher |
| `on_agent_idle` | `agent_idle` events | Higher |
| `on_pr_created` | `pr_created` events | Higher |
| `on_pr_merged` | `pr_merged` events | Higher |
| `on_task_assigned` | `task_assigned` events | Higher |
| `on_ci_failed` | `ci_failed` events | Higher |
| `on_worker_stuck` | `worker_stuck` events | Higher |
| `on_message_sent` | `message_sent` events | Higher |

**Priority**: If both `on_event` and a specific hook (e.g., `on_pr_created`) are configured, **both** will fire. Specific hooks run **in addition to**, not instead of, the catch-all.

### Setting Hooks

```bash
# Set catch-all hook (gets all events)
multiclaude hooks set on_event /path/to/notify-all.sh

# Set specific event hooks
multiclaude hooks set on_pr_created /path/to/notify-pr.sh
multiclaude hooks set on_ci_failed /path/to/alert-ci.sh

# View current configuration
multiclaude hooks list

# Clear a specific hook
multiclaude hooks clear on_pr_created

# Clear all hooks
multiclaude hooks clear-all
```

### Hook Script Requirements

1. **Executable**: `chmod +x /path/to/hook.sh`
2. **Read from stdin**: Event JSON is passed via stdin
3. **Exit quickly**: Hooks timeout after 30 seconds
4. **Handle errors**: multiclaude doesn't retry or log hook failures
5. **Be idempotent**: Same event may fire multiple times (rare, but possible)

## Writing Hook Scripts

### Template (Bash)

```bash
#!/usr/bin/env bash
set -euo pipefail

# Read event JSON from stdin
EVENT_JSON=$(cat)

# Parse fields
EVENT_TYPE=$(echo "$EVENT_JSON" | jq -r '.type')
REPO_NAME=$(echo "$EVENT_JSON" | jq -r '.repo_name // "unknown"')
AGENT_NAME=$(echo "$EVENT_JSON" | jq -r '.agent_name // "unknown"')

# Handle specific events
case "$EVENT_TYPE" in
    pr_created)
        PR_NUMBER=$(echo "$EVENT_JSON" | jq -r '.data.pr_number')
        PR_URL=$(echo "$EVENT_JSON" | jq -r '.data.url')
        TITLE=$(echo "$EVENT_JSON" | jq -r '.data.title')

        # Your notification logic
        echo "PR #$PR_NUMBER created: $TITLE"
        echo "URL: $PR_URL"
        ;;

    ci_failed)
        PR_NUMBER=$(echo "$EVENT_JSON" | jq -r '.data.pr_number')
        JOB_NAME=$(echo "$EVENT_JSON" | jq -r '.data.job_name')

        # Send alert
        echo "CI failed: $JOB_NAME on PR #$PR_NUMBER in $REPO_NAME"
        ;;

    *)
        # Unhandled event type
        ;;
esac

exit 0
```

### Template (Python)

```python
#!/usr/bin/env python3
import json
import sys

def main():
    # Read event from stdin
    event = json.load(sys.stdin)

    event_type = event['type']
    repo_name = event.get('repo_name', 'unknown')
    agent_name = event.get('agent_name', 'unknown')
    data = event.get('data', {})

    # Handle specific events
    if event_type == 'pr_created':
        pr_number = data['pr_number']
        title = data['title']
        url = data['url']
        print(f"PR #{pr_number} created: {title}")
        print(f"URL: {url}")

    elif event_type == 'ci_failed':
        pr_number = data['pr_number']
        job_name = data['job_name']
        print(f"CI failed: {job_name} on PR #{pr_number} in {repo_name}")

    else:
        # Unhandled event
        pass

if __name__ == '__main__':
    main()
```

### Template (Node.js)

```javascript
#!/usr/bin/env node

const readline = require('readline');

// Read event from stdin
let input = '';
const rl = readline.createInterface({ input: process.stdin });

rl.on('line', (line) => { input += line; });

rl.on('close', () => {
    const event = JSON.parse(input);

    const { type, repo_name, agent_name, data } = event;

    switch (type) {
        case 'pr_created':
            console.log(`PR #${data.pr_number} created: ${data.title}`);
            console.log(`URL: ${data.url}`);
            break;

        case 'ci_failed':
            console.log(`CI failed: ${data.job_name} on PR #${data.pr_number}`);
            break;

        default:
            // Unhandled event
            break;
    }
});
```

## Notification Examples

### Slack Notification

```bash
#!/usr/bin/env bash
# slack-notify.sh - Send multiclaude events to Slack

set -euo pipefail

# Configuration (set via environment or config file)
SLACK_WEBHOOK_URL="${SLACK_WEBHOOK_URL:-}"

if [ -z "$SLACK_WEBHOOK_URL" ]; then
    echo "Error: SLACK_WEBHOOK_URL not set" >&2
    exit 1
fi

# Read event
EVENT_JSON=$(cat)
EVENT_TYPE=$(echo "$EVENT_JSON" | jq -r '.type')

# Build Slack message based on event type
case "$EVENT_TYPE" in
    pr_created)
        REPO=$(echo "$EVENT_JSON" | jq -r '.repo_name')
        AGENT=$(echo "$EVENT_JSON" | jq -r '.agent_name')
        PR_NUMBER=$(echo "$EVENT_JSON" | jq -r '.data.pr_number')
        TITLE=$(echo "$EVENT_JSON" | jq -r '.data.title')
        URL=$(echo "$EVENT_JSON" | jq -r '.data.url')

        MESSAGE=":pr: *New PR in $REPO*\n<$URL|#$PR_NUMBER: $TITLE>\nCreated by: $AGENT"
        ;;

    ci_failed)
        REPO=$(echo "$EVENT_JSON" | jq -r '.repo_name')
        PR_NUMBER=$(echo "$EVENT_JSON" | jq -r '.data.pr_number')
        JOB_NAME=$(echo "$EVENT_JSON" | jq -r '.data.job_name')

        MESSAGE=":x: *CI Failed in $REPO*\nPR: #$PR_NUMBER\nJob: $JOB_NAME"
        ;;

    pr_merged)
        REPO=$(echo "$EVENT_JSON" | jq -r '.repo_name')
        PR_NUMBER=$(echo "$EVENT_JSON" | jq -r '.data.pr_number')
        TITLE=$(echo "$EVENT_JSON" | jq -r '.data.title')

        MESSAGE=":tada: *PR Merged in $REPO*\n#$PR_NUMBER: $TITLE"
        ;;

    *)
        # Skip other events or handle generically
        exit 0
        ;;
esac

# Send to Slack
curl -X POST "$SLACK_WEBHOOK_URL" \
    -H "Content-Type: application/json" \
    -d "{\"text\": \"$MESSAGE\"}" \
    --silent --show-error

exit 0
```

**Setup:**
```bash
# Get webhook URL from Slack: https://api.slack.com/messaging/webhooks
export SLACK_WEBHOOK_URL="https://hooks.slack.com/services/YOUR/WEBHOOK/URL"

# Configure multiclaude
multiclaude hooks set on_event /usr/local/bin/slack-notify.sh
```

### Discord Webhook

```python
#!/usr/bin/env python3
# discord-notify.py - Send multiclaude events to Discord

import json
import sys
import os
import requests

DISCORD_WEBHOOK_URL = os.environ.get('DISCORD_WEBHOOK_URL')

if not DISCORD_WEBHOOK_URL:
    print("Error: DISCORD_WEBHOOK_URL not set", file=sys.stderr)
    sys.exit(1)

# Read event
event = json.load(sys.stdin)
event_type = event['type']

# Build Discord message
if event_type == 'pr_created':
    repo = event['repo_name']
    agent = event['agent_name']
    pr_number = event['data']['pr_number']
    title = event['data']['title']
    url = event['data']['url']

    message = {
        "content": f"🔔 **New PR in {repo}**",
        "embeds": [{
            "title": f"#{pr_number}: {title}",
            "url": url,
            "color": 0x00FF00,
            "fields": [
                {"name": "Created by", "value": agent, "inline": True}
            ]
        }]
    }

elif event_type == 'ci_failed':
    repo = event['repo_name']
    pr_number = event['data']['pr_number']
    job_name = event['data']['job_name']

    message = {
        "content": f"❌ **CI Failed in {repo}**",
        "embeds": [{
            "title": f"PR #{pr_number}",
            "color": 0xFF0000,
            "fields": [
                {"name": "Job", "value": job_name, "inline": True}
            ]
        }]
    }

else:
    # Skip other events
    sys.exit(0)

# Send to Discord
requests.post(DISCORD_WEBHOOK_URL, json=message)
```

### Email Notification

```bash
#!/usr/bin/env bash
# email-notify.sh - Send multiclaude events via email

set -euo pipefail

EMAIL_TO="${EMAIL_TO:-admin@example.com}"

EVENT_JSON=$(cat)
EVENT_TYPE=$(echo "$EVENT_JSON" | jq -r '.type')

# Only email critical events
case "$EVENT_TYPE" in
    ci_failed|agent_failed|worker_stuck)
        SUBJECT="[multiclaude] $EVENT_TYPE"

        # Format event as readable text
        BODY=$(echo "$EVENT_JSON" | jq -r .)

        # Send email (requires mail/sendmail configured)
        echo "$BODY" | mail -s "$SUBJECT" "$EMAIL_TO"
        ;;

    *)
        # Skip non-critical events
        exit 0
        ;;
esac
```

### PagerDuty Alert

```python
#!/usr/bin/env python3
# pagerduty-alert.py - Create PagerDuty incidents for critical events

import json
import sys
import os
import requests

PAGERDUTY_API_KEY = os.environ.get('PAGERDUTY_API_KEY')
PAGERDUTY_SERVICE_ID = os.environ.get('PAGERDUTY_SERVICE_ID')

if not PAGERDUTY_API_KEY or not PAGERDUTY_SERVICE_ID:
    print("Error: PagerDuty credentials not set", file=sys.stderr)
    sys.exit(1)

event = json.load(sys.stdin)
event_type = event['type']

# Only alert on critical events
if event_type not in ['ci_failed', 'agent_failed', 'worker_stuck']:
    sys.exit(0)

# Build incident
incident = {
    "incident": {
        "type": "incident",
        "title": f"multiclaude: {event_type} in {event.get('repo_name', 'unknown')}",
        "service": {
            "id": PAGERDUTY_SERVICE_ID,
            "type": "service_reference"
        },
        "body": {
            "type": "incident_body",
            "details": json.dumps(event, indent=2)
        }
    }
}

# Create incident
requests.post(
    'https://api.pagerduty.com/incidents',
    headers={
        'Authorization': f'Token token={PAGERDUTY_API_KEY}',
        'Content-Type': 'application/json',
        'Accept': 'application/vnd.pagerduty+json;version=2'
    },
    json=incident
)
```

### Custom Webhook

```javascript
#!/usr/bin/env node
// webhook-notify.js - Send events to custom webhook endpoint

const https = require('https');
const readline = require('readline');

const WEBHOOK_URL = process.env.WEBHOOK_URL || '';
const WEBHOOK_SECRET = process.env.WEBHOOK_SECRET || '';

if (!WEBHOOK_URL) {
    console.error('Error: WEBHOOK_URL not set');
    process.exit(1);
}

// Read event from stdin
let input = '';
const rl = readline.createInterface({ input: process.stdin });

rl.on('line', (line) => { input += line; });

rl.on('close', () => {
    const event = JSON.parse(input);

    const url = new URL(WEBHOOK_URL);

    const data = JSON.stringify(event);

    const options = {
        hostname: url.hostname,
        port: url.port || 443,
        path: url.pathname,
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            'Content-Length': data.length,
            'X-Webhook-Secret': WEBHOOK_SECRET
        }
    };

    const req = https.request(options, (res) => {
        // Don't care about response for fire-and-forget
    });

    req.on('error', (error) => {
        console.error('Webhook error:', error);
    });

    req.write(data);
    req.end();
});
```

## Advanced Patterns

### Filtering Events

```bash
#!/usr/bin/env bash
# filtered-notify.sh - Only notify on specific conditions

EVENT_JSON=$(cat)
EVENT_TYPE=$(echo "$EVENT_JSON" | jq -r '.type')
REPO_NAME=$(echo "$EVENT_JSON" | jq -r '.repo_name')

# Only notify for production repo
if [ "$REPO_NAME" != "production" ]; then
    exit 0
fi

# Only notify for PR events
case "$EVENT_TYPE" in
    pr_created|pr_merged|ci_failed)
        # Send notification...
        ;;
    *)
        exit 0
        ;;
esac
```

### Rate Limiting

```python
#!/usr/bin/env python3
# rate-limited-notify.py - Prevent notification spam

import json
import sys
import time
import os

STATE_FILE = '/tmp/multiclaude-notify-state.json'
RATE_LIMIT_SECONDS = 300  # Max 1 notification per 5 minutes

# Load rate limit state
if os.path.exists(STATE_FILE):
    with open(STATE_FILE) as f:
        state = json.load(f)
else:
    state = {}

event = json.load(sys.stdin)
event_type = event['type']

# Check rate limit
now = time.time()
last_sent = state.get(event_type, 0)

if now - last_sent < RATE_LIMIT_SECONDS:
    # Rate limited
    sys.exit(0)

# Send notification...
# (your notification logic here)

# Update state
state[event_type] = now
with open(STATE_FILE, 'w') as f:
    json.dump(state, f)
```

### Aggregating Events

```python
#!/usr/bin/env python3
# aggregate-notify.py - Batch events and send summary

import json
import sys
import time
import os
from collections import defaultdict

BUFFER_FILE = '/tmp/multiclaude-event-buffer.json'
FLUSH_INTERVAL = 600  # Flush every 10 minutes

# Load buffer
if os.path.exists(BUFFER_FILE):
    with open(BUFFER_FILE) as f:
        buffer_data = json.load(f)
        events = buffer_data.get('events', [])
        last_flush = buffer_data.get('last_flush', 0)
else:
    events = []
    last_flush = 0

# Add current event
event = json.load(sys.stdin)
events.append(event)

# Check if we should flush
now = time.time()
if now - last_flush < FLUSH_INTERVAL:
    # Just buffer, don't send yet
    with open(BUFFER_FILE, 'w') as f:
        json.dump({'events': events, 'last_flush': last_flush}, f)
    sys.exit(0)

# Flush: aggregate and send summary
summary = defaultdict(int)
for e in events:
    summary[e['type']] += 1

# Send notification with summary...
# (your notification logic here)

# Clear buffer
with open(BUFFER_FILE, 'w') as f:
    json.dump({'events': [], 'last_flush': now}, f)
```

## Testing Hooks

### Manual Testing

```bash
# Test your hook with sample event
echo '{
  "type": "pr_created",
  "timestamp": "2024-01-15T10:30:00Z",
  "repo_name": "test-repo",
  "agent_name": "test-agent",
  "data": {
    "pr_number": 123,
    "title": "Test PR",
    "url": "https://github.com/user/repo/pull/123"
  }
}' | /path/to/your-hook.sh
```

### Automated Testing

```bash
#!/usr/bin/env bash
# test-hook.sh - Test hook script

set -e

HOOK_SCRIPT="$1"

if [ ! -x "$HOOK_SCRIPT" ]; then
    echo "Error: Hook script not executable: $HOOK_SCRIPT"
    exit 1
fi

# Test pr_created event
echo "Testing pr_created event..."
echo '{
  "type": "pr_created",
  "repo_name": "test",
  "agent_name": "test",
  "data": {"pr_number": 1, "title": "Test", "url": "https://example.com"}
}' | timeout 5 "$HOOK_SCRIPT"

# Test ci_failed event
echo "Testing ci_failed event..."
echo '{
  "type": "ci_failed",
  "repo_name": "test",
  "data": {"pr_number": 1, "job_name": "test"}
}' | timeout 5 "$HOOK_SCRIPT"

echo "All tests passed!"
```

## Debugging Hooks

### Hook Not Firing

```bash
# Check hook configuration
multiclaude hooks list

# Check daemon logs for hook execution
tail -f ~/.multiclaude/daemon.log | grep -i hook

# Manually trigger an event (future feature)
# multiclaude debug emit-event pr_created --repo test --pr 123
```

### Hook Errors

Hook scripts run with stderr captured. To debug:

```bash
# Add logging to your hook
echo "Hook started: $EVENT_TYPE" >> /tmp/hook-debug.log
echo "Event JSON: $EVENT_JSON" >> /tmp/hook-debug.log
```

### Hook Timeouts

Hooks must complete within 30 seconds. If your hook does heavy work:

```bash
#!/usr/bin/env bash
# Long-running work in background
(
    # Your slow notification logic
    sleep 10
    curl ...
) &

# Hook exits immediately
exit 0
```

## Security Considerations

1. **Validate Webhook URLs**: Don't allow user input in webhook URLs
2. **Protect Secrets**: Use environment variables, not hardcoded credentials
3. **Sanitize Event Data**: Event data comes from multiclaude, but sanitize before shell execution
4. **Limit Permissions**: Run hooks with minimal necessary permissions
5. **Rate Limit**: Prevent notification spam with rate limiting

Example (preventing injection):

```bash
# Bad - vulnerable to injection
MESSAGE="PR created: $(echo $EVENT_JSON | jq -r .data.title)"

# Good - use jq's raw output
MESSAGE="PR created: $(echo "$EVENT_JSON" | jq -r '.data.title')"
```

## Hook Performance

- **Timeout**: 30 seconds hard limit
- **Concurrency**: Hooks run in parallel (daemon doesn't wait)
- **Memory**: Hook processes are independent, can use any amount
- **Retries**: None - hook failures are silent

**Best Practices:**
- Keep hooks fast (<5 seconds)
- Do heavy work in background
- Use async notification APIs
- Log to file for debugging, not stdout

## Related Documentation

- **[`EXTENSIBILITY.md`](../EXTENSIBILITY.md)** - Overview of extension points
- **[`STATE_FILE_INTEGRATION.md`](STATE_FILE_INTEGRATION.md)** - For building monitoring tools
- **[`examples/hooks/`](../../examples/hooks/)** - Working hook examples
- `internal/events/events.go` - Event type definitions (canonical source)

## Contributing Hook Examples

Have a working hook integration? Contribute it!

1. Add to `examples/hooks/<service>-notify.sh`
2. Include setup instructions in comments
3. Test with `echo '...' | your-hook.sh`
4. PR to the repository

Popular integrations we'd love to see:
- Microsoft Teams
- Telegram
- Matrix
- Custom monitoring systems
- Project management tools (Jira, Linear, etc.)
