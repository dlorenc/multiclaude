# /status - Show system status

Display the current multiclaude system status including agent information.

## Instructions

Run the following commands and summarize the results:

1. Check daemon status:
   ```bash
   multiclaude daemon status
   ```

2. Show git status of the current worktree:
   ```bash
   git status
   ```

3. Show the current branch and recent commits:
   ```bash
   git log --oneline -5
   ```

4. Check for any pending messages:
   ```bash
   multiclaude agent list-messages
   ```

Present the results in a clear, organized format with sections for:
- Daemon status
- Current branch and git status
- Recent commits
- Pending messages (if any)
