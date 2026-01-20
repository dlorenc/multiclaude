# /messages - Check and manage messages

Check for and manage inter-agent messages.

## Instructions

1. List pending messages:
   ```bash
   multiclaude agent list-messages
   ```

2. If there are messages, show the user:
   - Message ID
   - Sender
   - Preview of the message content

3. Ask the user if they want to read or acknowledge any specific message.

To read a specific message:
```bash
multiclaude agent read-message <message-id>
```

To acknowledge a message:
```bash
multiclaude agent ack-message <message-id>
```

If there are no pending messages, let the user know.
