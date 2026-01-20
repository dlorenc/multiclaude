# /workers - List active workers

Display all active worker agents for the current repository.

## Instructions

Run the following command to list workers:

```bash
multiclaude work list
```

Present the results showing:
- Worker names
- Their current status
- What task they are working on (if available)

If no workers are active, let the user know and suggest using `multiclaude work "task description"` to spawn a new worker.
