# /refresh - Sync worktree with main branch

Sync your worktree with the latest changes from the main branch.

## Instructions

1. First, fetch the latest changes:
   ```bash
   git fetch origin main
   ```

2. Check if there are any uncommitted changes:
   ```bash
   git status --porcelain
   ```

3. If there are uncommitted changes, stash them first:
   ```bash
   git stash push -m "refresh-stash-$(date +%s)"
   ```

4. Rebase your current branch onto main:
   ```bash
   git rebase origin/main
   ```

5. If you stashed changes, pop them:
   ```bash
   git stash pop
   ```

6. Report the result to the user, including:
   - How many commits were rebased
   - Whether there were any conflicts
   - Current status after refresh

If there are rebase conflicts, stop and let the user know which files have conflicts.
