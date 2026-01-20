package claude

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// mockTerminal implements TerminalRunner for testing.
type mockTerminal struct {
	sendKeysCalls        []sendKeysCall
	sendKeysLiteralCalls []sendKeysCall
	sendEnterCalls       []targetCall
	getPanePIDCalls      []targetCall
	startPipePaneCalls   []pipePaneCall
	stopPipePaneCalls    []targetCall

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

func (m *mockTerminal) SendKeys(session, window, text string) error {
	m.sendKeysCalls = append(m.sendKeysCalls, sendKeysCall{session, window, text})
	return m.sendKeysError
}

func (m *mockTerminal) SendKeysLiteral(session, window, text string) error {
	m.sendKeysLiteralCalls = append(m.sendKeysLiteralCalls, sendKeysCall{session, window, text})
	return m.sendKeysError
}

func (m *mockTerminal) SendEnter(session, window string) error {
	m.sendEnterCalls = append(m.sendEnterCalls, targetCall{session, window})
	return nil
}

func (m *mockTerminal) GetPanePID(session, window string) (int, error) {
	m.getPanePIDCalls = append(m.getPanePIDCalls, targetCall{session, window})
	return m.getPanePIDReturn, m.getPanePIDError
}

func (m *mockTerminal) StartPipePane(session, window, outputFile string) error {
	m.startPipePaneCalls = append(m.startPipePaneCalls, pipePaneCall{session, window, outputFile})
	return nil
}

func (m *mockTerminal) StopPipePane(session, window string) error {
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
	terminal := &mockTerminal{
		getPanePIDReturn: 12345,
	}

	runner := NewRunner(
		WithTerminal(terminal),
		WithBinaryPath("/path/to/claude"),
		WithStartupDelay(0), // No delay for tests
	)

	result, err := runner.Start("my-session", "my-window", Config{
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

	// Verify SendKeys was called (MOTD + command = 2 calls)
	if len(terminal.sendKeysCalls) != 2 {
		t.Fatalf("expected 2 SendKeys calls (MOTD + command), got %d", len(terminal.sendKeysCalls))
	}

	// First call is MOTD
	motdCall := terminal.sendKeysCalls[0]
	if !strings.Contains(motdCall.text, "multiclaude claude") {
		t.Errorf("expected MOTD to contain restart hint, got %q", motdCall.text)
	}

	// Second call is the actual command
	call := terminal.sendKeysCalls[1]
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

func TestStartWithCustomSessionID(t *testing.T) {
	terminal := &mockTerminal{
		getPanePIDReturn: 12345,
	}

	runner := NewRunner(
		WithTerminal(terminal),
		WithStartupDelay(0),
	)

	result, err := runner.Start("session", "window", Config{
		SessionID: "my-custom-session-id",
	})

	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	if result.SessionID != "my-custom-session-id" {
		t.Errorf("expected SessionID to be 'my-custom-session-id', got %q", result.SessionID)
	}

	// Verify command contains custom session ID (second call, after MOTD)
	if len(terminal.sendKeysCalls) < 2 {
		t.Fatalf("expected at least 2 SendKeys calls, got %d", len(terminal.sendKeysCalls))
	}
	if !strings.Contains(terminal.sendKeysCalls[1].text, "--session-id my-custom-session-id") {
		t.Errorf("expected command to contain custom session ID, got %q", terminal.sendKeysCalls[1].text)
	}
}

func TestStartWithOutputCapture(t *testing.T) {
	terminal := &mockTerminal{
		getPanePIDReturn: 12345,
	}

	runner := NewRunner(
		WithTerminal(terminal),
		WithStartupDelay(0),
	)

	_, err := runner.Start("session", "window", Config{
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
	terminal := &mockTerminal{
		getPanePIDReturn: 12345,
	}

	runner := NewRunner(
		WithTerminal(terminal),
		WithStartupDelay(0),
		WithMessageDelay(0),
	)

	_, err := runner.Start("session", "window", Config{
		InitialMessage: "Hello, Claude!",
	})

	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Verify SendKeysLiteral was called for the initial message
	if len(terminal.sendKeysLiteralCalls) != 1 {
		t.Fatalf("expected 1 SendKeysLiteral call, got %d", len(terminal.sendKeysLiteralCalls))
	}

	if terminal.sendKeysLiteralCalls[0].text != "Hello, Claude!" {
		t.Errorf("expected initial message 'Hello, Claude!', got %q", terminal.sendKeysLiteralCalls[0].text)
	}

	// Verify SendEnter was called
	if len(terminal.sendEnterCalls) != 1 {
		t.Fatalf("expected 1 SendEnter call, got %d", len(terminal.sendEnterCalls))
	}
}

func TestStartNoTerminal(t *testing.T) {
	runner := NewRunner()

	_, err := runner.Start("session", "window", Config{})
	if err == nil {
		t.Error("expected error when terminal not configured")
	}
	if !strings.Contains(err.Error(), "terminal runner not configured") {
		t.Errorf("expected 'terminal runner not configured' error, got %q", err.Error())
	}
}

func TestStartSendKeysError(t *testing.T) {
	terminal := &mockTerminal{
		sendKeysError: errors.New("send keys failed"),
	}

	runner := NewRunner(
		WithTerminal(terminal),
		WithStartupDelay(0),
	)

	_, err := runner.Start("session", "window", Config{})
	if err == nil {
		t.Error("expected error when SendKeys fails")
	}
	if !strings.Contains(err.Error(), "send keys failed") {
		t.Errorf("expected 'send keys failed' error, got %q", err.Error())
	}
}

func TestStartGetPIDError(t *testing.T) {
	terminal := &mockTerminal{
		getPanePIDError: errors.New("get PID failed"),
	}

	runner := NewRunner(
		WithTerminal(terminal),
		WithStartupDelay(0),
	)

	_, err := runner.Start("session", "window", Config{})
	if err == nil {
		t.Error("expected error when GetPanePID fails")
	}
	if !strings.Contains(err.Error(), "get PID failed") {
		t.Errorf("expected 'get PID failed' error, got %q", err.Error())
	}
}

func TestSendMessage(t *testing.T) {
	terminal := &mockTerminal{}

	runner := NewRunner(WithTerminal(terminal))

	err := runner.SendMessage("session", "window", "Hello, Claude!")
	if err != nil {
		t.Fatalf("SendMessage() failed: %v", err)
	}

	// Verify SendKeysLiteral was called
	if len(terminal.sendKeysLiteralCalls) != 1 {
		t.Fatalf("expected 1 SendKeysLiteral call, got %d", len(terminal.sendKeysLiteralCalls))
	}

	call := terminal.sendKeysLiteralCalls[0]
	if call.text != "Hello, Claude!" {
		t.Errorf("expected message 'Hello, Claude!', got %q", call.text)
	}

	// Verify SendEnter was called
	if len(terminal.sendEnterCalls) != 1 {
		t.Fatalf("expected 1 SendEnter call, got %d", len(terminal.sendEnterCalls))
	}
}

func TestSendMessageMultiline(t *testing.T) {
	terminal := &mockTerminal{}

	runner := NewRunner(WithTerminal(terminal))

	multilineMsg := "Line 1\nLine 2\nLine 3"
	err := runner.SendMessage("session", "window", multilineMsg)
	if err != nil {
		t.Fatalf("SendMessage() failed: %v", err)
	}

	// Verify the full multiline message was sent
	if terminal.sendKeysLiteralCalls[0].text != multilineMsg {
		t.Errorf("expected multiline message preserved, got %q", terminal.sendKeysLiteralCalls[0].text)
	}
}

func TestSendMessageNoTerminal(t *testing.T) {
	runner := NewRunner()

	err := runner.SendMessage("session", "window", "Hello")
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
			name: "with claude config dir",
			config: Config{
				SessionID:       "test-session",
				ClaudeConfigDir: "/home/user/.multiclaude/claude-config/repo/agent",
			},
			contains: []string{
				"CLAUDE_CONFIG_DIR=/home/user/.multiclaude/claude-config/repo/agent",
				"/path/to/claude",
				"--session-id test-session",
			},
		},
		{
			name: "without claude config dir",
			config: Config{
				SessionID: "test-session",
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

func TestResolveBinaryPath(t *testing.T) {
	// This test is environment-dependent, so we just verify it doesn't panic
	// and returns something
	path := ResolveBinaryPath()
	if path == "" {
		t.Error("ResolveBinaryPath() returned empty string")
	}
}

func TestBuildCommandClaudeConfigDirPrepended(t *testing.T) {
	runner := NewRunner(
		WithBinaryPath("claude"),
		WithPermissions(true),
	)

	cmd := runner.buildCommand("test-session", Config{
		ClaudeConfigDir: "/tmp/config",
	})

	// Verify CLAUDE_CONFIG_DIR is prepended before the binary
	if !strings.HasPrefix(cmd, "CLAUDE_CONFIG_DIR=/tmp/config claude") {
		t.Errorf("expected CLAUDE_CONFIG_DIR to be prepended before binary, got %q", cmd)
	}
}

func TestStartWithClaudeConfigDir(t *testing.T) {
	terminal := &mockTerminal{
		getPanePIDReturn: 12345,
	}

	runner := NewRunner(
		WithTerminal(terminal),
		WithBinaryPath("/path/to/claude"),
		WithStartupDelay(0),
	)

	result, err := runner.Start("my-session", "my-window", Config{
		ClaudeConfigDir: "/home/user/.multiclaude/claude-config/repo/agent",
	})

	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	if result.SessionID == "" {
		t.Error("expected SessionID to be generated")
	}

	// Verify SendKeys was called (MOTD + command = 2 calls)
	if len(terminal.sendKeysCalls) < 2 {
		t.Fatalf("expected at least 2 SendKeys calls, got %d", len(terminal.sendKeysCalls))
	}

	// Second call is the actual command
	call := terminal.sendKeysCalls[1]

	// Verify CLAUDE_CONFIG_DIR is included in command
	if !strings.Contains(call.text, "CLAUDE_CONFIG_DIR=/home/user/.multiclaude/claude-config/repo/agent") {
		t.Errorf("expected command to contain CLAUDE_CONFIG_DIR, got %q", call.text)
	}

	// Verify the env var is prepended (before the binary path)
	if !strings.HasPrefix(call.text, "CLAUDE_CONFIG_DIR=/home/user/.multiclaude/claude-config/repo/agent /path/to/claude") {
		t.Errorf("expected CLAUDE_CONFIG_DIR to be prepended before binary, got %q", call.text)
	}
}
