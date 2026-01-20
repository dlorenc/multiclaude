# Merge Queue Customization

## Periodic Maintenance Agents

After every 4-5 PRs have been merged, spawn the following maintenance agents:

### Refactoring Agent

Spawn a worker to look for opportunities to refactor and simplify the codebase:

```bash
multiclaude work "Review the codebase for refactoring opportunities: look for duplicated code, overly complex functions, unused code, and areas that could be simplified. Create a PR with any improvements found. Focus on small, safe changes that improve readability and maintainability without changing behavior."
```

**Focus areas:**
- Duplicated code that could be extracted into shared functions
- Functions that are too long or complex (>50 lines or high cyclomatic complexity)
- Dead code or unused exports
- Inconsistent patterns that could be unified
- Opportunities to use more idiomatic Go patterns

### Test Coverage Agent

Spawn a worker to check test coverage and fill gaps:

```bash
multiclaude work "Analyze test coverage across the codebase. Run 'go test -coverprofile=coverage.out ./...' to identify packages with low coverage. Add tests for uncovered or under-tested code paths, prioritizing critical business logic and error handling. Create a PR with the new tests."
```

**Focus areas:**
- Packages with less than 70% coverage
- Error handling paths that aren't tested
- Edge cases in critical functions
- Integration points between packages
- Recently added code that may lack tests

## Tracking

Keep a simple count of merged PRs. After merging PR #N where N is divisible by 5 (or close to it), trigger both maintenance agents. This ensures the codebase stays healthy as it grows.

## Notes

- These agents should create separate PRs for their changes
- Small, incremental improvements are preferred over large sweeping changes
- If no improvements are found, that's fine - the agent can complete without creating a PR
- Refactoring changes must not break existing tests
- New tests must pass before the PR is created
