# Worker Naming Specification

## Overview

Workers should have descriptive, task-based names instead of random adjective-animal combinations. This makes it easier to identify what each worker is doing at a glance.

## Requirements

### 1. Task Summary Extraction

Extract a 3-4 word summary from the task description using heuristic processing:

- Remove common stop words (a, an, the, is, are, to, for, in, on, at, etc.)
- Identify and extract meaningful keywords (nouns, verbs, technical terms)
- Prioritize words at the beginning of the task description
- Limit to 3-4 words to keep names concise

### 2. Sanitization

Convert the extracted summary to a valid worker name:

- Convert to lowercase
- Replace spaces with hyphens
- Remove or replace special characters (keep only alphanumeric and hyphens)
- Collapse multiple consecutive hyphens into one
- Trim leading/trailing hyphens
- Maximum length: 50 characters (truncate if needed)

### 3. Uniqueness Handling

Ensure worker names are unique within a repository:

- Check if the generated name already exists
- If it exists, append numeric suffix: `-2`, `-3`, etc.
- Keep incrementing until a unique name is found

### 4. Fallback Strategy

If task extraction fails or produces invalid names:

- Fall back to the existing random name generator (`names.Generate()`)
- This ensures workers can always be created, even with unusual task descriptions

### 5. Manual Override

Preserve the existing `--name` flag to allow users to manually specify worker names.

## Examples

| Task Description | Generated Name |
|-----------------|----------------|
| "Fix the session ID bug in authentication" | `fix-session-id-bug` |
| "Add user profile editing feature" | `add-user-profile` |
| "Refactor the database connection logic" | `refactor-database-connection` |
| "Update README documentation" | `update-readme-documentation` |
| "Implement OAuth2 login flow" | `implement-oauth2-login` |
| "Fix bug" (too short) | `fix-bug` |
| "!!!" (invalid) | `happy-platypus` (fallback) |

## Implementation Details

### Stop Words List

Common words to filter out:
```
a, an, the, is, are, am, was, were, be, been, being, have, has, had,
do, does, did, will, would, should, could, may, might, must, can,
to, for, of, in, on, at, by, with, from, as, into, through,
this, that, these, those, it, its, they, their, there, here,
and, or, but, if, because, when, where, how, what, which, who, why
```

### Name Validation

A valid worker name must:
- Be between 3 and 50 characters long
- Contain at least one alphabetic character
- Not start or end with a hyphen
- Contain only lowercase letters, numbers, and hyphens

### Edge Cases

- Empty task description → fallback to random name
- Task with only stop words → fallback to random name
- Task producing name less than 3 characters → fallback to random name
- Very long task → extract key terms and truncate
- Special characters in task → sanitize and remove
- Duplicate name → append numeric suffix

## Testing Requirements

Comprehensive tests must cover:

1. **Basic extraction**: Verify correct keyword extraction from various task descriptions
2. **Sanitization**: Test lowercase conversion, special character handling, hyphen collapsing
3. **Uniqueness**: Test numeric suffix appending for duplicate names
4. **Fallback**: Verify fallback to random names for invalid inputs
5. **Edge cases**: Empty strings, very long strings, special characters only
6. **Integration**: Test within the full worker creation flow

## Migration

Existing code should continue to work:
- The `names.Generate()` function remains available for backward compatibility
- New function `names.FromTask(task string)` implements the task-based naming
- CLI code updated to use `FromTask()` by default, with `Generate()` as fallback
