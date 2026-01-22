package claude

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// mockTerminal implements TerminalRunner for testing.
type mockTerminal struct {
	sendKeysCalls                 []sendKeysCall
	sendKeysLiteralCalls          []sendKeysCall
	sendKeysLiteralWithEnterCalls []sendKeysCall
	sendEnterCalls                []targetCall
	getPanePIDCalls               []targetCall
	startPipePaneCalls            []pipePaneCall
	stopPipePaneCalls             []targetCall

	getPanePIDReturn int
	getPanePIDError  error
	sendKeysError    error
}

type sendKeysCall struct {
	session string
	window  string
	text    string
}

type targetCall struct {
	session string
	window  string
}

type pipePaneCall struct {
	session    string
	window     string
	outputFile string
}

func (m *mockTerminal) SendKeys(ctx context.Context, session, window, text string) error {
	m.sendKeysCalls = append(m.sendKeysCalls, sendKeysCall{session, window, text})
	return m.sendKeysError
}

func (m *mockTerminal) SendKeysLiteral(ctx context.Context, session, window, text string) error {
	m.sendKeysLiteralCalls = append(m.sendKeysLiteralCalls, sendKeysCall{session, window, text})
	return m.sendKeysError
}

func (m *mockTerminal) SendKeysLiteralWithEnter(ctx context.Context, session, window, text string) error {
	m.sendKeysLiteralWithEnterCalls = append(m.sendKeysLiteralWithEnterCalls, sendKeysCall{session, window, text})
	return m.sendKeysError
}

func (m *mockTerminal) SendEnter(ctx context.Context, session, window string) error {
	m.sendEnterCalls = append(m.sendEnterCalls, targetCall{session, window})
	return nil
}

func (m *mockTerminal) GetPanePID(ctx context.Context, session, window string) (int, error) {
	m.getPanePIDCalls = append(m.getPanePIDCalls, targetCall{session, window})
	return m.getPanePIDReturn, m.getPanePIDError
}

func (m *mockTerminal) StartPipePane(ctx context.Context, session, window, outputFile string) error {
	m.startPipePaneCalls = append(m.startPipePaneCalls, pipePaneCall{session, window, outputFile})
	return nil
}

func (m *mockTerminal) StopPipePane(ctx context.Context, session, window string) error {
	m.stopPipePaneCalls = append(m.stopPipePaneCalls, targetCall{session, window})
	return nil
}

func TestNewRunner(t *testing.T) {
	runner := NewRunner()
	if runner == nil {
		t.Fatal("NewRunner() returned nil")
	}
	if runner.BinaryPath != "claude" {
		t.Errorf("expected default BinaryPath to be 'claude', got %q", runner.BinaryPath)
	}
	if runner.StartupDelay != 500*time.Millisecond {
		t.Errorf("expected default StartupDelay to be 500ms, got %v", runner.StartupDelay)
	}
	if runner.MessageDelay != 1*time.Second {
		t.Errorf("expected default MessageDelay to be 1s, got %v", runner.MessageDelay)
	}
	if !runner.SkipPermissions {
		t.Error("expected default SkipPermissions to be true")
	}
}

func TestNewRunnerWithOptions(t *testing.T) {
	terminal := &mockTerminal{}
	runner := NewRunner(
		WithBinaryPath("/custom/claude"),
		WithTerminal(terminal),
		WithStartupDelay(1*time.Second),
		WithMessageDelay(2*time.Second),
		WithPermissions(false),
	)

	if runner.BinaryPath != "/custom/claude" {
		t.Errorf("expected BinaryPath to be '/custom/claude', got %q", runner.BinaryPath)
	}
	if runner.Terminal != terminal {
		t.Error("expected Terminal to be set")
	}
	if runner.StartupDelay != 1*time.Second {
		t.Errorf("expected StartupDelay to be 1s, got %v", runner.StartupDelay)
	}
	if runner.MessageDelay != 2*time.Second {
		t.Errorf("expected MessageDelay to be 2s, got %v", runner.MessageDelay)
	}
	if runner.SkipPermissions {
		t.Error("expected SkipPermissions to be false")
	}
}

func TestStart(t *testing.T) {
	ctx := context.Background()
	terminal := &mockTerminal{
		getPanePIDReturn: 12345,
	}

	runner := NewRunner(
		WithTerminal(terminal),
		WithBinaryPath("/path/to/claude"),
		WithStartupDelay(0), // No delay for tests
	)

	result, err := runner.Start(ctx, "my-session", "my-window", Config{
		SystemPromptFile: "/path/to/prompt.md",
	})

	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	if result.SessionID == "" {
		t.Error("expected SessionID to be generated")
	}

	if result.PID != 12345 {
		t.Errorf("expected PID to be 12345, got %d", result.PID)
	}

	// Verify SendKeys was called once for the command (no MOTD since it's empty)
	if len(terminal.sendKeysCalls) != 1 {
		t.Fatalf("expected 1 SendKeys call (command only), got %d", len(terminal.sendKeysCalls))
	}

	// First call is the actual command
	call := terminal.sendKeysCalls[0]
	if call.session != "my-session" {
		t.Errorf("expected session 'my-session', got %q", call.session)
	}
	if call.window != "my-window" {
		t.Errorf("expected window 'my-window', got %q", call.window)
	}

	// Verify command structure
	if !strings.Contains(call.text, "/path/to/claude") {
		t.Errorf("expected command to contain binary path, got %q", call.text)
	}
	if !strings.Contains(call.text, "--session-id") {
		t.Errorf("expected command to contain --session-id, got %q", call.text)
	}
	if !strings.Contains(call.text, "--dangerously-skip-permissions") {
		t.Errorf("expected command to contain --dangerously-skip-permissions, got %q", call.text)
	}
	if !strings.Contains(call.text, "--append-system-prompt-file /path/to/prompt.md") {
		t.Errorf("expected command to contain prompt file, got %q", call.text)
	}
}

func TestStartWithMOTD(t *testing.T) {
	ctx := context.Background()
	terminal := &mockTerminal{
		getPanePIDReturn: 12345,
	}

	runner := NewRunner(
		WithTerminal(terminal),
		WithBinaryPath("/path/to/claude"),
		WithStartupDelay(0),
	)

	result, err := runner.Start(ctx, "my-session", "my-window", Config{
		MOTD: "Welcome to the agent session!",
	})

	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	if result.SessionID == "" {
		t.Error("expected SessionID to be generated")
	}

	// Verify SendKeys was called twice (MOTD + command)
	if len(terminal.sendKeysCalls) != 2 {
		t.Fatalf("expected 2 SendKeys calls (MOTD + command), got %d", len(terminal.sendKeysCalls))
	}

	// First call is MOTD
	motdCall := terminal.sendKeysCalls[0]
	if !strings.Contains(motdCall.text, "Welcome to the agent session!") {
		t.Errorf("expected MOTD to contain message, got %q", motdCall.text)
	}

	// Second call is the actual command
	cmdCall := terminal.sendKeysCalls[1]
	if !strings.Contains(cmdCall.text, "/path/to/claude") {
		t.Errorf("expected command to contain binary path, got %q", cmdCall.text)
	}
}

func TestStartWithCustomSessionID(t *testing.T) {
	ctx := context.Background()
	terminal := &mockTerminal{
		getPanePIDReturn: 12345,
	}

	runner := NewRunner(
		WithTerminal(terminal),
		WithStartupDelay(0),
	)

	result, err := runner.Start(ctx, "session", "window", Config{
		SessionID: "my-custom-session-id",
	})

	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	if result.SessionID != "my-custom-session-id" {
		t.Errorf("expected SessionID to be 'my-custom-session-id', got %q", result.SessionID)
	}

	// Verify command contains custom session ID
	if len(terminal.sendKeysCalls) < 1 {
		t.Fatalf("expected at least 1 SendKeys call, got %d", len(terminal.sendKeysCalls))
	}
	if !strings.Contains(terminal.sendKeysCalls[0].text, "--session-id my-custom-session-id") {
		t.Errorf("expected command to contain custom session ID, got %q", terminal.sendKeysCalls[0].text)
	}
}

func TestStartWithOutputCapture(t *testing.T) {
	ctx := context.Background()
	terminal := &mockTerminal{
		getPanePIDReturn: 12345,
	}

	runner := NewRunner(
		WithTerminal(terminal),
		WithStartupDelay(0),
	)

	_, err := runner.Start(ctx, "session", "window", Config{
		OutputFile: "/tmp/output.log",
	})

	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Verify StartPipePane was called
	if len(terminal.startPipePaneCalls) != 1 {
		t.Fatalf("expected 1 StartPipePane call, got %d", len(terminal.startPipePaneCalls))
	}

	call := terminal.startPipePaneCalls[0]
	if call.outputFile != "/tmp/output.log" {
		t.Errorf("expected outputFile to be '/tmp/output.log', got %q", call.outputFile)
	}
}

func TestStartWithInitialMessage(t *testing.T) {
	ctx := context.Background()
	terminal := &mockTerminal{
		getPanePIDReturn: 12345,
	}

	runner := NewRunner(
		WithTerminal(terminal),
		WithStartupDelay(0),
		WithMessageDelay(0),
	)

	_, err := runner.Start(ctx, "session", "window", Config{
		InitialMessage: "Hello, Claude!",
	})

	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Verify SendKeysLiteralWithEnter was called for the initial message
	if len(terminal.sendKeysLiteralWithEnterCalls) != 1 {
		t.Fatalf("expected 1 SendKeysLiteralWithEnter call, got %d", len(terminal.sendKeysLiteralWithEnterCalls))
	}

	if terminal.sendKeysLiteralWithEnterCalls[0].text != "Hello, Claude!" {
		t.Errorf("expected initial message 'Hello, Claude!', got %q", terminal.sendKeysLiteralWithEnterCalls[0].text)
	}
}

func TestStartNoTerminal(t *testing.T) {
	ctx := context.Background()
	runner := NewRunner()

	_, err := runner.Start(ctx, "session", "window", Config{})
	if err == nil {
		t.Error("expected error when terminal not configured")
	}
	if !strings.Contains(err.Error(), "terminal runner not configured") {
		t.Errorf("expected 'terminal runner not configured' error, got %q", err.Error())
	}
}

func TestStartSendKeysError(t *testing.T) {
	ctx := context.Background()
	terminal := &mockTerminal{
		sendKeysError: errors.New("send keys failed"),
	}

	runner := NewRunner(
		WithTerminal(terminal),
		WithStartupDelay(0),
	)

	_, err := runner.Start(ctx, "session", "window", Config{})
	if err == nil {
		t.Error("expected error when SendKeys fails")
	}
	if !strings.Contains(err.Error(), "send keys failed") {
		t.Errorf("expected 'send keys failed' error, got %q", err.Error())
	}
}

func TestStartGetPIDError(t *testing.T) {
	ctx := context.Background()
	terminal := &mockTerminal{
		getPanePIDError: errors.New("get PID failed"),
	}

	runner := NewRunner(
		WithTerminal(terminal),
		WithStartupDelay(0),
	)

	_, err := runner.Start(ctx, "session", "window", Config{})
	if err == nil {
		t.Error("expected error when GetPanePID fails")
	}
	if !strings.Contains(err.Error(), "get PID failed") {
		t.Errorf("expected 'get PID failed' error, got %q", err.Error())
	}
}

func TestStartContextCancellation(t *testing.T) {
	terminal := &mockTerminal{
		getPanePIDReturn: 12345,
	}

	runner := NewRunner(
		WithTerminal(terminal),
		WithStartupDelay(100 * time.Millisecond),
	)

	// Create a context that will be cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := runner.Start(ctx, "session", "window", Config{})
	if err == nil {
		t.Error("expected error when context is cancelled")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got %v", err)
	}
}

func TestSendMessage(t *testing.T) {
	ctx := context.Background()
	terminal := &mockTerminal{}

	runner := NewRunner(WithTerminal(terminal))

	err := runner.SendMessage(ctx, "session", "window", "Hello, Claude!")
	if err != nil {
		t.Fatalf("SendMessage() failed: %v", err)
	}

	// Verify SendKeysLiteralWithEnter was called (atomic send)
	if len(terminal.sendKeysLiteralWithEnterCalls) != 1 {
		t.Fatalf("expected 1 SendKeysLiteralWithEnter call, got %d", len(terminal.sendKeysLiteralWithEnterCalls))
	}

	call := terminal.sendKeysLiteralWithEnterCalls[0]
	if call.text != "Hello, Claude!" {
		t.Errorf("expected message 'Hello, Claude!', got %q", call.text)
	}
}

func TestSendMessageMultiline(t *testing.T) {
	ctx := context.Background()
	terminal := &mockTerminal{}

	runner := NewRunner(WithTerminal(terminal))

	multilineMsg := "Line 1\nLine 2\nLine 3"
	err := runner.SendMessage(ctx, "session", "window", multilineMsg)
	if err != nil {
		t.Fatalf("SendMessage() failed: %v", err)
	}

	// Verify the full multiline message was sent via atomic send
	if terminal.sendKeysLiteralWithEnterCalls[0].text != multilineMsg {
		t.Errorf("expected multiline message preserved, got %q", terminal.sendKeysLiteralWithEnterCalls[0].text)
	}
}

func TestSendMessageNoTerminal(t *testing.T) {
	ctx := context.Background()
	runner := NewRunner()

	err := runner.SendMessage(ctx, "session", "window", "Hello")
	if err == nil {
		t.Error("expected error when terminal not configured")
	}
}

func TestGenerateSessionID(t *testing.T) {
	id1, err := GenerateSessionID()
	if err != nil {
		t.Fatalf("GenerateSessionID() failed: %v", err)
	}

	// Check format (UUID v4: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx)
	parts := strings.Split(id1, "-")
	if len(parts) != 5 {
		t.Errorf("expected 5 parts in UUID, got %d", len(parts))
	}

	// Verify uniqueness
	id2, err := GenerateSessionID()
	if err != nil {
		t.Fatalf("GenerateSessionID() failed: %v", err)
	}

	if id1 == id2 {
		t.Error("expected different session IDs for each call")
	}
}

func TestBuildCommand(t *testing.T) {
	runner := NewRunner(
		WithBinaryPath("/path/to/claude"),
		WithPermissions(true),
	)

	tests := []struct {
		name     string
		config   Config
		contains []string
		excludes []string
	}{
		{
			name: "basic",
			config: Config{
				SessionID: "test-session",
			},
			contains: []string{
				"/path/to/claude",
				"--session-id test-session",
				"--dangerously-skip-permissions",
			},
		},
		{
			name: "with prompt file",
			config: Config{
				SessionID:        "test-session",
				SystemPromptFile: "/path/to/prompt.md",
			},
			contains: []string{
				"--append-system-prompt-file /path/to/prompt.md",
			},
		},
		{
			name: "excludes CLAUDE_CONFIG_DIR (no longer used)",
			config: Config{
				SessionID: "test-session",
			},
			excludes: []string{
				"CLAUDE_CONFIG_DIR",
			},
		},
		{
			name: "with workdir",
			config: Config{
				SessionID: "test-session",
				WorkDir:   "/path/to/workdir",
			},
			contains: []string{
				"cd \"/path/to/workdir\" &&",
				"/path/to/claude",
			},
		},
		{
			name: "with workdir excludes CLAUDE_CONFIG_DIR",
			config: Config{
				SessionID: "test-session",
				WorkDir:   "/path/to/workdir",
			},
			contains: []string{
				"cd \"/path/to/workdir\" &&",
				"/path/to/claude",
			},
			excludes: []string{
				"CLAUDE_CONFIG_DIR",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := runner.buildCommand(tc.config.SessionID, tc.config)

			for _, s := range tc.contains {
				if !strings.Contains(cmd, s) {
					t.Errorf("expected command to contain %q, got %q", s, cmd)
				}
			}

			for _, s := range tc.excludes {
				if strings.Contains(cmd, s) {
					t.Errorf("expected command not to contain %q, got %q", s, cmd)
				}
			}
		})
	}
}

func TestBuildCommandWithoutSkipPermissions(t *testing.T) {
	runner := NewRunner(
		WithBinaryPath("claude"),
		WithPermissions(false),
	)

	cmd := runner.buildCommand("session-id", Config{})

	if strings.Contains(cmd, "--dangerously-skip-permissions") {
		t.Error("expected command not to contain --dangerously-skip-permissions when disabled")
	}
}

func TestBuildCommandWithResume(t *testing.T) {
	runner := NewRunner(WithBinaryPath("claude"))

	// Test with Resume=false (default)
	cmd := runner.buildCommand("test-session-id", Config{})
	if !strings.Contains(cmd, "--session-id test-session-id") {
		t.Errorf("expected command to contain --session-id, got %q", cmd)
	}
	if strings.Contains(cmd, "--resume") {
		t.Error("expected command not to contain --resume when Resume=false")
	}

	// Test with Resume=true
	cmd = runner.buildCommand("test-session-id", Config{Resume: true})
	if !strings.Contains(cmd, "--resume test-session-id") {
		t.Errorf("expected command to contain --resume, got %q", cmd)
	}
	if strings.Contains(cmd, "--session-id") {
		t.Error("expected command not to contain --session-id when Resume=true")
	}
}

func TestResolveBinaryPath(t *testing.T) {
	// This test is environment-dependent, so we just verify it doesn't panic
	// and returns something
	path := ResolveBinaryPath()
	if path == "" {
		t.Error("ResolveBinaryPath() returned empty string")
	}
}

func TestIsBinaryAvailable(t *testing.T) {
	// Test with a binary that definitely exists
	runner := NewRunner(WithBinaryPath("echo"))
	if !runner.IsBinaryAvailable() {
		t.Error("IsBinaryAvailable() should return true for 'echo'")
	}

	// Test with a binary that doesn't exist
	runner = NewRunner(WithBinaryPath("/nonexistent/binary/path"))
	if runner.IsBinaryAvailable() {
		t.Error("IsBinaryAvailable() should return false for nonexistent binary")
	}
}

// Note: TestBuildCommandClaudeConfigDirPrepended and TestStartWithClaudeConfigDir
// were removed because CLAUDE_CONFIG_DIR is no longer used. Claude Code only reads
// credentials from ~/.claude/.credentials.json regardless of CLAUDE_CONFIG_DIR,
// and slash commands are now embedded directly in agent prompts.
