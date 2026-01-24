# Makefile for multiclaude - Local CI Guard Rails
# Run these targets to verify changes before pushing

.PHONY: help build test unit-tests e2e-tests verify-docs coverage check-all pre-commit clean

# Default target
help:
	@echo "Multiclaude Local CI Guard Rails"
	@echo ""
	@echo "Targets that mirror CI checks:"
	@echo "  make build          - Build all packages (CI: Build job)"
	@echo "  make unit-tests     - Run unit tests (CI: Unit Tests job)"
	@echo "  make e2e-tests      - Run E2E tests (CI: E2E Tests job)"
	@echo "  make verify-docs    - Check generated docs are up to date (CI: Verify Generated Docs job)"
	@echo "  make coverage       - Run coverage check (CI: Coverage Check job)"
	@echo ""
	@echo "Comprehensive checks:"
	@echo "  make check-all      - Run all CI checks locally (recommended before push)"
	@echo "  make pre-commit     - Fast checks suitable for git pre-commit hook"
	@echo ""
	@echo "Setup:"
	@echo "  make install-hooks  - Install git pre-commit hook"
	@echo ""
	@echo "Other:"
	@echo "  make test           - Alias for unit-tests"
	@echo "  make clean          - Clean build artifacts"

# Build - matches CI build job
build:
	@echo "==> Building all packages..."
	@go build -v ./...
	@echo "✓ Build successful"

# Unit tests - matches CI unit-tests job
unit-tests:
	@echo "==> Running unit tests..."
	@command -v tmux >/dev/null 2>&1 || { echo "Error: tmux is required for tests. Install with: sudo apt-get install tmux"; exit 1; }
	@go test -coverprofile=coverage.out -covermode=atomic ./internal/... ./pkg/...
	@go tool cover -func=coverage.out | tail -1
	@echo "✓ Unit tests passed"

# E2E tests - matches CI e2e-tests job
e2e-tests:
	@echo "==> Running E2E tests..."
	@command -v tmux >/dev/null 2>&1 || { echo "Error: tmux is required for tests. Install with: sudo apt-get install tmux"; exit 1; }
	@git config user.email >/dev/null 2>&1 || git config --global user.email "ci@local.dev"
	@git config user.name >/dev/null 2>&1 || git config --global user.name "Local CI"
	@go test -v ./test/...
	@echo "✓ E2E tests passed"

# Verify generated docs - matches CI verify-generated-docs job
verify-docs:
	@echo "==> Verifying generated docs are up to date..."
	@go generate ./pkg/config/...
	@if ! git diff --quiet docs/DIRECTORY_STRUCTURE.md; then \
		echo "Error: docs/DIRECTORY_STRUCTURE.md is out of date!"; \
		echo "Run 'go generate ./pkg/config/...' or 'make generate' and commit the changes."; \
		echo ""; \
		echo "Diff:"; \
		git diff docs/DIRECTORY_STRUCTURE.md; \
		exit 1; \
	fi
	@echo "==> Verifying extension documentation consistency..."
	@go run ./cmd/verify-docs
	@echo "✓ Generated docs are up to date"

# Coverage check - matches CI coverage-check job
coverage:
	@echo "==> Checking coverage thresholds..."
	@command -v tmux >/dev/null 2>&1 || { echo "Error: tmux is required for tests. Install with: sudo apt-get install tmux"; exit 1; }
	@go test -coverprofile=coverage.out -covermode=atomic ./internal/... ./pkg/...
	@echo ""
	@echo "Coverage summary:"
	@go tool cover -func=coverage.out | grep "total:" || true
	@echo ""
	@echo "Per-package coverage:"
	@go test -cover ./internal/... ./pkg/... 2>&1 | grep "coverage:" | sort
	@echo "✓ Coverage check complete"

# Helper to regenerate docs
generate:
	@echo "==> Regenerating documentation..."
	@go generate ./pkg/config/...
	@echo "✓ Documentation regenerated"

# Alias for unit-tests
test: unit-tests

# Pre-commit: Fast checks suitable for git hook
# Runs build + unit tests + verify docs (skips slower e2e tests)
pre-commit: build unit-tests verify-docs
	@echo ""
	@echo "✓ All pre-commit checks passed"

# Check all: Complete CI validation locally
# Runs all checks that CI will run
check-all: build unit-tests e2e-tests verify-docs coverage
	@echo ""
	@echo "=========================================="
	@echo "✓ All CI checks passed locally!"
	@echo "Your changes are ready to push."
	@echo "=========================================="

# Install git hooks
install-hooks:
	@echo "==> Installing git pre-commit hook..."
	@mkdir -p .git/hooks
	@if [ -f .git/hooks/pre-commit ]; then \
		echo "Warning: .git/hooks/pre-commit already exists"; \
		echo "Backing up to .git/hooks/pre-commit.backup"; \
		cp .git/hooks/pre-commit .git/hooks/pre-commit.backup; \
	fi
	@cp scripts/pre-commit.sh .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo "✓ Git pre-commit hook installed"
	@echo ""
	@echo "The hook will run 'make pre-commit' before each commit."
	@echo "To skip the hook temporarily, use: git commit --no-verify"

# Clean build artifacts
clean:
	@echo "==> Cleaning build artifacts..."
	@rm -f coverage.out
	@go clean -cache
	@echo "✓ Clean complete"
