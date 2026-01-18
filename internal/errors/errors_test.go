package errors

import (
	"errors"
	"strings"
	"testing"
)

func TestCLIError_Error(t *testing.T) {
	err := New(CategoryRuntime, "test error")
	if err.Error() != "test error" {
		t.Errorf("expected 'test error', got '%s'", err.Error())
	}
}

func TestCLIError_Unwrap(t *testing.T) {
	cause := errors.New("underlying error")
	err := Wrap(CategoryRuntime, "wrapper", cause)

	if err.Unwrap() != cause {
		t.Error("Unwrap should return the cause")
	}
}

func TestFormat_CLIError(t *testing.T) {
	tests := []struct {
		name     string
		err      *CLIError
		contains []string
	}{
		{
			name:     "basic error",
			err:      New(CategoryRuntime, "something failed"),
			contains: []string{"Error:", "something failed"},
		},
		{
			name:     "usage error",
			err:      New(CategoryUsage, "invalid argument"),
			contains: []string{"Usage error:", "invalid argument"},
		},
		{
			name:     "config error",
			err:      New(CategoryConfig, "missing config"),
			contains: []string{"Configuration error:", "missing config"},
		},
		{
			name:     "connection error",
			err:      New(CategoryConnection, "daemon unreachable"),
			contains: []string{"Connection error:", "daemon unreachable"},
		},
		{
			name:     "not found error",
			err:      New(CategoryNotFound, "worker missing"),
			contains: []string{"Not found:", "worker missing"},
		},
		{
			name:     "error with cause",
			err:      Wrap(CategoryRuntime, "operation failed", errors.New("permission denied")),
			contains: []string{"operation failed", "permission denied"},
		},
		{
			name:     "error with suggestion",
			err:      New(CategoryConnection, "daemon offline").WithSuggestion("multiclaude start"),
			contains: []string{"daemon offline", "Try:", "multiclaude start"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatted := Format(tt.err)
			for _, s := range tt.contains {
				if !strings.Contains(formatted, s) {
					t.Errorf("expected formatted error to contain '%s', got: %s", s, formatted)
				}
			}
		})
	}
}

func TestFormat_RegularError(t *testing.T) {
	err := errors.New("regular error")
	formatted := Format(err)

	if !strings.Contains(formatted, "Error:") {
		t.Errorf("expected 'Error:' prefix, got: %s", formatted)
	}
	if !strings.Contains(formatted, "regular error") {
		t.Errorf("expected error message, got: %s", formatted)
	}
}

func TestFormat_Nil(t *testing.T) {
	if Format(nil) != "" {
		t.Error("Format(nil) should return empty string")
	}
}

func TestDaemonNotRunning(t *testing.T) {
	err := DaemonNotRunning()

	if err.Category != CategoryConnection {
		t.Error("DaemonNotRunning should have CategoryConnection")
	}
	if err.Suggestion == "" {
		t.Error("DaemonNotRunning should have a suggestion")
	}

	formatted := Format(err)
	if !strings.Contains(formatted, "daemon") {
		t.Errorf("expected 'daemon' in message, got: %s", formatted)
	}
	if !strings.Contains(formatted, "multiclaude start") {
		t.Errorf("expected suggestion, got: %s", formatted)
	}
}

func TestDaemonCommunicationFailed(t *testing.T) {
	cause := errors.New("connection refused")
	err := DaemonCommunicationFailed("listing repos", cause)

	if err.Category != CategoryConnection {
		t.Error("should have CategoryConnection")
	}
	if err.Cause != cause {
		t.Error("should wrap cause")
	}

	formatted := Format(err)
	if !strings.Contains(formatted, "listing repos") {
		t.Errorf("expected operation in message, got: %s", formatted)
	}
	if !strings.Contains(formatted, "connection refused") {
		t.Errorf("expected cause in message, got: %s", formatted)
	}
}

func TestNotInRepo(t *testing.T) {
	err := NotInRepo()
	formatted := Format(err)

	if !strings.Contains(formatted, "not in a tracked repository") {
		t.Errorf("expected message, got: %s", formatted)
	}
	if !strings.Contains(formatted, "multiclaude init") {
		t.Errorf("expected init suggestion, got: %s", formatted)
	}
}

func TestMultipleRepos(t *testing.T) {
	err := MultipleRepos()
	formatted := Format(err)

	if !strings.Contains(formatted, "--repo") {
		t.Errorf("expected --repo flag suggestion, got: %s", formatted)
	}
}

func TestAgentNotFound(t *testing.T) {
	err := AgentNotFound("worker", "test-worker", "my-repo")
	formatted := Format(err)

	if !strings.Contains(formatted, "test-worker") {
		t.Errorf("expected agent name, got: %s", formatted)
	}
	if !strings.Contains(formatted, "my-repo") {
		t.Errorf("expected repo name, got: %s", formatted)
	}
	if !strings.Contains(formatted, "multiclaude work list") {
		t.Errorf("expected list suggestion, got: %s", formatted)
	}
}

func TestInvalidPRURL(t *testing.T) {
	err := InvalidPRURL()
	formatted := Format(err)

	if !strings.Contains(formatted, "github.com") {
		t.Errorf("expected example URL format, got: %s", formatted)
	}
}

func TestClaudeNotFound(t *testing.T) {
	err := ClaudeNotFound(errors.New("not found"))
	formatted := Format(err)

	if !strings.Contains(formatted, "claude") {
		t.Errorf("expected claude mention, got: %s", formatted)
	}
	if !strings.Contains(formatted, "install") || !strings.Contains(formatted, "anthropic") {
		t.Errorf("expected install suggestion, got: %s", formatted)
	}
}

func TestMissingArgument(t *testing.T) {
	err := MissingArgument("repo", "string")
	formatted := Format(err)

	if !strings.Contains(formatted, "repo") {
		t.Errorf("expected argument name, got: %s", formatted)
	}
	if !strings.Contains(formatted, "string") {
		t.Errorf("expected type hint, got: %s", formatted)
	}
}

func TestInvalidArgument(t *testing.T) {
	err := InvalidArgument("count", "abc", "integer")
	formatted := Format(err)

	if !strings.Contains(formatted, "count") {
		t.Errorf("expected argument name, got: %s", formatted)
	}
	if !strings.Contains(formatted, "abc") {
		t.Errorf("expected value, got: %s", formatted)
	}
	if !strings.Contains(formatted, "integer") {
		t.Errorf("expected expected type, got: %s", formatted)
	}
}

func TestUnknownCommand(t *testing.T) {
	err := UnknownCommand("foobar")
	formatted := Format(err)

	if !strings.Contains(formatted, "foobar") {
		t.Errorf("expected command name, got: %s", formatted)
	}
	if !strings.Contains(formatted, "--help") {
		t.Errorf("expected help suggestion, got: %s", formatted)
	}
}

func TestWithSuggestion_Chaining(t *testing.T) {
	err := New(CategoryRuntime, "failed").WithSuggestion("try again")

	if err.Suggestion != "try again" {
		t.Errorf("expected suggestion to be set, got: %s", err.Suggestion)
	}
}
