#!/bin/bash
# Pre-commit hook for multiclaude
# Runs fast CI checks before allowing commit

set -e

echo "Running pre-commit checks..."
echo ""

# Run pre-commit target (build + unit tests + verify docs)
# This skips the slower E2E tests for faster commits
if make pre-commit; then
    echo ""
    echo "✓ Pre-commit checks passed"
    exit 0
else
    echo ""
    echo "✗ Pre-commit checks failed"
    echo ""
    echo "Your commit has been blocked because local checks failed."
    echo "Fix the issues above and try again."
    echo ""
    echo "To skip this hook (not recommended), use: git commit --no-verify"
    echo ""
    exit 1
fi

