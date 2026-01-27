# Worker Naming Examples

This document demonstrates the new task-based worker naming feature.

## Before (Random Names)

```bash
$ multiclaude worker create "Fix session ID bug in authentication"
Creating worker 'calm-owl' in repo 'myproject'

$ multiclaude worker create "Add user profile editing"
Creating worker 'jolly-hawk' in repo 'myproject'
```

Workers had random adjective-animal names that provided no context about their purpose.

## After (Task-Based Names)

```bash
$ multiclaude worker create "Fix session ID bug in authentication"
Creating worker 'fix-session-id-bug' in repo 'myproject'

$ multiclaude worker create "Add user profile editing"
Creating worker 'add-user-profile-editing' in repo 'myproject'
```

Workers now have descriptive names derived from their task descriptions.

## How It Works

### 1. Keyword Extraction

The system extracts meaningful keywords from the task description:

```
Task: "Fix the session ID bug in authentication"
Keywords: ["fix", "session", "id", "bug"]  (stop words removed: "the", "in")
Name: "fix-session-id-bug"
```

### 2. Sanitization

Names are converted to valid format:

- Lowercase letters only
- Hyphens separate words
- Special characters removed
- Maximum 50 characters

```
Task: "Update API (v2) endpoint configuration!!!"
Keywords: ["update", "api", "v2", "endpoint"]
Name: "update-api-v2-endpoint"
```

### 3. Uniqueness

Duplicate names get numeric suffixes:

```bash
$ multiclaude worker create "Fix bug in login"
Creating worker 'fix-bug-login' in repo 'myproject'

$ multiclaude worker create "Fix bug in login"  # Same task
Creating worker 'fix-bug-login-2' in repo 'myproject'

$ multiclaude worker create "Fix bug in login"  # Again
Creating worker 'fix-bug-login-3' in repo 'myproject'
```

### 4. Fallback to Random Names

If the task description is invalid or too short, the system falls back to random names:

```bash
$ multiclaude worker create "!!!"
Creating worker 'happy-platypus' in repo 'myproject'  # Fallback

$ multiclaude worker create "the a an is"  # Only stop words
Creating worker 'clever-dolphin' in repo 'myproject'  # Fallback
```

## Manual Override

The `--name` flag still works for manual naming:

```bash
$ multiclaude worker create "Fix bug" --name my-custom-name
Creating worker 'my-custom-name' in repo 'myproject'
```

## Real-World Examples

| Task Description | Generated Name |
|-----------------|----------------|
| "Fix memory leak in database connection pool" | `fix-memory-leak-database` |
| "Implement OAuth2 authentication flow" | `implement-oauth2-authentication-flow` |
| "Refactor user service to use new API" | `refactor-user-service-new` |
| "Add unit tests for payment module" | `add-unit-tests-payment` |
| "Update README with installation instructions" | `update-readme-installation-instructions` |
| "Debug timeout in webhook handler" | `debug-timeout-webhook-handler` |

## Benefits

1. **Clarity**: Immediately understand what each worker is doing
2. **Tracking**: Easier to monitor worker progress in logs and tmux
3. **Git branches**: Branch names like `work/fix-session-id-bug` are self-documenting
4. **PR identification**: PRs are easier to identify from their branch names
5. **Debugging**: When something goes wrong, you know which worker to investigate

## Technical Details

- Implementation: `internal/names/names.go`
- Tests: `internal/names/names_test.go`
- Specification: `WORKER_NAMING_SPEC.md`
- Integration: `internal/cli/cli.go:createWorker()`
