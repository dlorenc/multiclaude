package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// canCreateSessions indicates whether the environment supports creating tmux sessions.
// This is set during TestMain and checked by tests that need to create sessions.
var canCreateSessions bool

// TestMain ensures clean tmux environment for tests
func TestMain(m *testing.M) {
	// Fail loudly in CI environments unless TMUX_TESTS=1 is set
	// CI environments (like GitHub Actions) often have tmux installed but without
	// proper terminal support, causing flaky session creation failures
	if os.Getenv("CI") != "" && os.Getenv("TMUX_TESTS") != "1" {
		fmt.Fprintln(os.Stderr, "FAIL: tmux is required for these tests but TMUX_TESTS=1 is not set in CI")
		os.Exit(1)
	}

	// Check if tmux is available
	if exec.Command("tmux", "-V").Run() != nil {
		fmt.Fprintln(os.Stderr, "FAIL: tmux is required for these tests but not available")
		os.Exit(1)
	}

	// Check if we can actually create sessions (not just that tmux is installed)
	// Some environments have tmux installed but unable to create sessions (headless CI)
	testSession := fmt.Sprintf("test-tmux-probe-%d", time.Now().UnixNano())
	cmd := exec.Command("tmux", "new-session", "-d", "-s", testSession)
	if err := cmd.Run(); err != nil {
		// Session creation failed - tests that need sessions will skip
		canCreateSessions = false
	} else {
		canCreateSessions = true
		// Clean up probe session
		exec.Command("tmux", "kill-session", "-t", testSession).Run()
	}

	// Run tests
	code := m.Run()

	// Cleanup any test sessions that might have leaked
	cleanupTestSessions()

	os.Exit(code)
}

// skipIfCannotCreateSessions skips the test if the environment cannot create tmux sessions.
// Use this at the start of any test that needs to create tmux sessions.
func skipIfCannotCreateSessions(t *testing.T) {
	t.Helper()
	if !canCreateSessions {
		t.Skip("tmux cannot create sessions in this environment (headless CI?)")
	}
}

// cleanupTestSessions removes any test sessions that leaked
func cleanupTestSessions() {
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	output, err := cmd.Output()
	if err != nil {
		return
	}

	sessions := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, session := range sessions {
		if strings.HasPrefix(session, "test-") {
			exec.Command("tmux", "kill-session", "-t", session).Run()
		}
	}
}

// uniqueSessionName generates a unique test session name
func uniqueSessionName() string {
	return fmt.Sprintf("test-tmux-%d", time.Now().UnixNano())
}

// waitForSession polls until a session exists or timeout is reached.
// This handles the race condition where tmux reports success but the session
// isn't immediately visible in subsequent queries.
func waitForSession(ctx context.Context, client *Client, sessionName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	pollInterval := 10 * time.Millisecond

	for time.Now().Before(deadline) {
		exists, err := client.HasSession(ctx, sessionName)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
		time.Sleep(pollInterval)
	}
	return fmt.Errorf("session %s did not appear within %v", sessionName, timeout)
}

// waitForNoSession polls until a session no longer exists or timeout is reached.
func waitForNoSession(ctx context.Context, client *Client, sessionName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	pollInterval := 10 * time.Millisecond

	for time.Now().Before(deadline) {
		exists, err := client.HasSession(ctx, sessionName)
		if err != nil {
			return err
		}
		if !exists {
			return nil
		}
		time.Sleep(pollInterval)
	}
	return fmt.Errorf("session %s still exists after %v", sessionName, timeout)
}

func TestNewClient(t *testing.T) {
	client := NewClient()
	if client == nil {
		t.Fatal("NewClient() returned nil")
	}
	if client.tmuxPath != "tmux" {
		t.Errorf("expected default tmuxPath to be 'tmux', got %q", client.tmuxPath)
	}
}

func TestNewClientWithOptions(t *testing.T) {
	client := NewClient(WithTmuxPath("/custom/path/tmux"))
	if client.tmuxPath != "/custom/path/tmux" {
		t.Errorf("expected tmuxPath to be '/custom/path/tmux', got %q", client.tmuxPath)
	}
}

func TestIsTmuxAvailable(t *testing.T) {
	client := NewClient()
	if !client.IsTmuxAvailable() {
		t.Error("Expected tmux to be available")
	}
}

func TestHasSession(t *testing.T) {
	skipIfCannotCreateSessions(t)
	ctx := context.Background()
	client := NewClient()
	sessionName := uniqueSessionName()

	// Session should not exist initially
	exists, err := client.HasSession(ctx, sessionName)
	if err != nil {
		t.Fatalf("HasSession failed: %v", err)
	}
	if exists {
		t.Error("Session should not exist initially")
	}

	// Create session
	if err := client.CreateSession(ctx, sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(ctx, sessionName)

	// Wait for session to be visible (handles tmux timing race)
	if err := waitForSession(ctx, client, sessionName, 2*time.Second); err != nil {
		t.Fatalf("Session not visible after creation: %v", err)
	}

	// Session should now exist
	exists, err = client.HasSession(ctx, sessionName)
	if err != nil {
		t.Fatalf("HasSession failed: %v", err)
	}
	if !exists {
		t.Error("Session should exist after creation")
	}
}

func TestCreateSession(t *testing.T) {
	skipIfCannotCreateSessions(t)
	ctx := context.Background()
	client := NewClient()
	sessionName := uniqueSessionName()

	// Create detached session
	if err := client.CreateSession(ctx, sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(ctx, sessionName)

	// Wait for session to be visible (handles tmux timing race)
	if err := waitForSession(ctx, client, sessionName, 2*time.Second); err != nil {
		t.Fatalf("Session not visible after creation: %v", err)
	}

	// Verify session exists
	exists, err := client.HasSession(ctx, sessionName)
	if err != nil {
		t.Fatalf("HasSession failed: %v", err)
	}
	if !exists {
		t.Error("Session should exist after creation")
	}

	// Creating duplicate session should fail
	err = client.CreateSession(ctx, sessionName, true)
	if err == nil {
		t.Error("Creating duplicate session should fail")
	}
}

func TestCreateWindow(t *testing.T) {
	skipIfCannotCreateSessions(t)
	ctx := context.Background()
	client := NewClient()
	sessionName := uniqueSessionName()

	// Create session first
	if err := client.CreateSession(ctx, sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(ctx, sessionName)

	// Create window
	windowName := "test-window"
	if err := client.CreateWindow(ctx, sessionName, windowName); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// Verify window exists
	exists, err := client.HasWindow(ctx, sessionName, windowName)
	if err != nil {
		t.Fatalf("HasWindow failed: %v", err)
	}
	if !exists {
		t.Error("Window should exist after creation")
	}
}

func TestHasWindow(t *testing.T) {
	skipIfCannotCreateSessions(t)
	ctx := context.Background()
	client := NewClient()
	sessionName := uniqueSessionName()

	// Create session
	if err := client.CreateSession(ctx, sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(ctx, sessionName)

	// Non-existent window should return false
	exists, err := client.HasWindow(ctx, sessionName, "nonexistent")
	if err != nil {
		t.Fatalf("HasWindow failed: %v", err)
	}
	if exists {
		t.Error("Non-existent window should return false")
	}

	// Create window
	windowName := "test-window"
	if err := client.CreateWindow(ctx, sessionName, windowName); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// Window should now exist
	exists, err = client.HasWindow(ctx, sessionName, windowName)
	if err != nil {
		t.Fatalf("HasWindow failed: %v", err)
	}
	if !exists {
		t.Error("Window should exist after creation")
	}
}

func TestHasWindowExactMatch(t *testing.T) {
	skipIfCannotCreateSessions(t)
	ctx := context.Background()
	client := NewClient()
	sessionName := uniqueSessionName()

	// Create session
	if err := client.CreateSession(ctx, sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(ctx, sessionName)

	// Create window named "test"
	if err := client.CreateWindow(ctx, sessionName, "test"); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// Create window named "test-longer"
	if err := client.CreateWindow(ctx, sessionName, "test-longer"); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// "test" should exist
	exists, err := client.HasWindow(ctx, sessionName, "test")
	if err != nil {
		t.Fatalf("HasWindow failed: %v", err)
	}
	if !exists {
		t.Error("Window 'test' should exist")
	}

	// "test-longer" should exist
	exists, err = client.HasWindow(ctx, sessionName, "test-longer")
	if err != nil {
		t.Fatalf("HasWindow failed: %v", err)
	}
	if !exists {
		t.Error("Window 'test-longer' should exist")
	}

	// "test-long" should NOT exist (exact match required)
	exists, err = client.HasWindow(ctx, sessionName, "test-long")
	if err != nil {
		t.Fatalf("HasWindow failed: %v", err)
	}
	if exists {
		t.Error("Window 'test-long' should NOT exist - exact match required")
	}

	// "tes" should NOT exist (exact match required)
	exists, err = client.HasWindow(ctx, sessionName, "tes")
	if err != nil {
		t.Fatalf("HasWindow failed: %v", err)
	}
	if exists {
		t.Error("Window 'tes' should NOT exist - exact match required")
	}
}

func TestKillWindow(t *testing.T) {
	skipIfCannotCreateSessions(t)
	ctx := context.Background()
	client := NewClient()
	sessionName := uniqueSessionName()

	// Create session
	if err := client.CreateSession(ctx, sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(ctx, sessionName)

	// Create two windows (we need at least 2 to kill one)
	if err := client.CreateWindow(ctx, sessionName, "window1"); err != nil {
		t.Fatalf("Failed to create window1: %v", err)
	}
	if err := client.CreateWindow(ctx, sessionName, "window2"); err != nil {
		t.Fatalf("Failed to create window2: %v", err)
	}

	// Kill window1
	if err := client.KillWindow(ctx, sessionName, "window1"); err != nil {
		t.Fatalf("Failed to kill window: %v", err)
	}

	// Verify window1 no longer exists
	exists, err := client.HasWindow(ctx, sessionName, "window1")
	if err != nil {
		t.Fatalf("HasWindow failed: %v", err)
	}
	if exists {
		t.Error("Window should not exist after killing")
	}

	// Verify window2 still exists
	exists, err = client.HasWindow(ctx, sessionName, "window2")
	if err != nil {
		t.Fatalf("HasWindow failed: %v", err)
	}
	if !exists {
		t.Error("Window2 should still exist")
	}
}

func TestKillSession(t *testing.T) {
	skipIfCannotCreateSessions(t)
	ctx := context.Background()
	client := NewClient()
	sessionName := uniqueSessionName()

	// Create session
	if err := client.CreateSession(ctx, sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Wait for session to be visible before killing
	if err := waitForSession(ctx, client, sessionName, 2*time.Second); err != nil {
		t.Fatalf("Session not visible after creation: %v", err)
	}

	// Kill session
	if err := client.KillSession(ctx, sessionName); err != nil {
		t.Fatalf("Failed to kill session: %v", err)
	}

	// Wait for session to be gone (handles tmux timing race)
	if err := waitForNoSession(ctx, client, sessionName, 2*time.Second); err != nil {
		t.Fatalf("Session still visible after killing: %v", err)
	}

	// Verify session no longer exists
	exists, err := client.HasSession(ctx, sessionName)
	if err != nil {
		t.Fatalf("HasSession failed: %v", err)
	}
	if exists {
		t.Error("Session should not exist after killing")
	}
}

func TestSendKeys(t *testing.T) {
	skipIfCannotCreateSessions(t)
	ctx := context.Background()
	client := NewClient()
	sessionName := uniqueSessionName()

	// Create session with a window running a shell
	if err := client.CreateSession(ctx, sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(ctx, sessionName)

	// Create a window
	windowName := "test-window"
	if err := client.CreateWindow(ctx, sessionName, windowName); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// Send keys to create a file (this tests that send-keys works)
	testFile := fmt.Sprintf("/tmp/tmux-test-%d", time.Now().UnixNano())
	defer os.Remove(testFile)

	if err := client.SendKeys(ctx, sessionName, windowName, fmt.Sprintf("touch %s", testFile)); err != nil {
		t.Fatalf("Failed to send keys: %v", err)
	}

	// Poll for the file to be created with timeout
	// CI environments may be slow, so we use a generous timeout with polling
	timeout := 5 * time.Second
	pollInterval := 50 * time.Millisecond
	deadline := time.Now().Add(timeout)

	fileCreated := false
	for time.Now().Before(deadline) {
		if _, err := os.Stat(testFile); err == nil {
			fileCreated = true
			break
		}
		time.Sleep(pollInterval)
	}

	// Verify the file was created (proves send-keys worked)
	if !fileCreated {
		t.Error("SendKeys did not execute command - file was not created within timeout")
	}
}

func TestSendKeysLiteral(t *testing.T) {
	skipIfCannotCreateSessions(t)
	ctx := context.Background()
	client := NewClient()
	sessionName := uniqueSessionName()

	// Create session
	if err := client.CreateSession(ctx, sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(ctx, sessionName)

	// Create a window
	windowName := "test-window"
	if err := client.CreateWindow(ctx, sessionName, windowName); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// SendKeysLiteral should not execute (no Enter key)
	// We can't easily verify this without reading pane content,
	// but we can at least verify it doesn't error
	if err := client.SendKeysLiteral(ctx, sessionName, windowName, "echo test"); err != nil {
		t.Fatalf("Failed to send keys literal: %v", err)
	}
}

func TestSendKeysLiteralWithNewlines(t *testing.T) {
	skipIfCannotCreateSessions(t)
	ctx := context.Background()
	client := NewClient()
	sessionName := uniqueSessionName()

	// Create session
	if err := client.CreateSession(ctx, sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(ctx, sessionName)

	// Create a window
	windowName := "test-window"
	if err := client.CreateWindow(ctx, sessionName, windowName); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// Test sending text with newlines - should not error
	multiLineText := "line1\nline2\nline3"
	if err := client.SendKeysLiteral(ctx, sessionName, windowName, multiLineText); err != nil {
		t.Fatalf("Failed to send multi-line text: %v", err)
	}

	// Test with empty lines
	textWithEmptyLines := "first\n\nlast"
	if err := client.SendKeysLiteral(ctx, sessionName, windowName, textWithEmptyLines); err != nil {
		t.Fatalf("Failed to send text with empty lines: %v", err)
	}

	// Test with trailing newline
	textWithTrailingNewline := "content\n"
	if err := client.SendKeysLiteral(ctx, sessionName, windowName, textWithTrailingNewline); err != nil {
		t.Fatalf("Failed to send text with trailing newline: %v", err)
	}
}

func TestSendEnter(t *testing.T) {
	skipIfCannotCreateSessions(t)
	ctx := context.Background()
	client := NewClient()
	sessionName := uniqueSessionName()

	// Create session
	if err := client.CreateSession(ctx, sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(ctx, sessionName)

	// Create a window
	windowName := "test-window"
	if err := client.CreateWindow(ctx, sessionName, windowName); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// SendEnter should work without error
	if err := client.SendEnter(ctx, sessionName, windowName); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}
}

func TestSendKeysLiteralWithEnter(t *testing.T) {
	skipIfCannotCreateSessions(t)
	ctx := context.Background()
	client := NewClient()
	sessionName := uniqueSessionName()

	// Create session with a window running a shell
	if err := client.CreateSession(ctx, sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(ctx, sessionName)

	// Create a window
	windowName := "test-window"
	if err := client.CreateWindow(ctx, sessionName, windowName); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// Send keys atomically to create a file
	testFile := fmt.Sprintf("/tmp/tmux-test-atomic-%d", time.Now().UnixNano())
	defer os.Remove(testFile)

	if err := client.SendKeysLiteralWithEnter(ctx, sessionName, windowName, fmt.Sprintf("touch %s", testFile)); err != nil {
		t.Fatalf("Failed to send keys atomically: %v", err)
	}

	// Poll for the file to be created with timeout
	timeout := 5 * time.Second
	pollInterval := 50 * time.Millisecond
	deadline := time.Now().Add(timeout)

	fileCreated := false
	for time.Now().Before(deadline) {
		if _, err := os.Stat(testFile); err == nil {
			fileCreated = true
			break
		}
		time.Sleep(pollInterval)
	}

	if !fileCreated {
		t.Error("SendKeysLiteralWithEnter did not execute command - file was not created within timeout")
	}
}

func TestListSessions(t *testing.T) {
	skipIfCannotCreateSessions(t)
	ctx := context.Background()
	client := NewClient()

	// Create a test session
	sessionName := uniqueSessionName()
	if err := client.CreateSession(ctx, sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(ctx, sessionName)

	// Wait for session to be visible (handles tmux timing race)
	if err := waitForSession(ctx, client, sessionName, 2*time.Second); err != nil {
		t.Fatalf("Session not visible after creation: %v", err)
	}

	// List sessions
	sessions, err := client.ListSessions(ctx)
	if err != nil {
		t.Fatalf("Failed to list sessions: %v", err)
	}

	// Our test session should be in the list
	// Note: We don't check exact count because external processes may create/delete
	// sessions concurrently, making count-based assertions flaky
	found := false
	for _, s := range sessions {
		if s == sessionName {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Session %s not found in list: %v", sessionName, sessions)
	}
}

func TestListWindows(t *testing.T) {
	skipIfCannotCreateSessions(t)
	ctx := context.Background()
	client := NewClient()
	sessionName := uniqueSessionName()

	// Create session
	if err := client.CreateSession(ctx, sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(ctx, sessionName)

	// List windows (should have default window)
	windows, err := client.ListWindows(ctx, sessionName)
	if err != nil {
		t.Fatalf("Failed to list windows: %v", err)
	}
	if len(windows) == 0 {
		t.Error("Expected at least one default window")
	}

	// Create additional windows
	if err := client.CreateWindow(ctx, sessionName, "window1"); err != nil {
		t.Fatalf("Failed to create window1: %v", err)
	}
	if err := client.CreateWindow(ctx, sessionName, "window2"); err != nil {
		t.Fatalf("Failed to create window2: %v", err)
	}

	// List windows again
	windows, err = client.ListWindows(ctx, sessionName)
	if err != nil {
		t.Fatalf("Failed to list windows: %v", err)
	}

	// Should have at least 3 windows (default + 2 created)
	if len(windows) < 3 {
		t.Errorf("Expected at least 3 windows, got %d: %v", len(windows), windows)
	}

	// Verify our windows are in the list
	foundWindow1 := false
	foundWindow2 := false
	for _, w := range windows {
		if w == "window1" {
			foundWindow1 = true
		}
		if w == "window2" {
			foundWindow2 = true
		}
	}
	if !foundWindow1 || !foundWindow2 {
		t.Errorf("Created windows not found in list: %v", windows)
	}
}

func TestGetPanePID(t *testing.T) {
	skipIfCannotCreateSessions(t)
	ctx := context.Background()
	client := NewClient()
	sessionName := uniqueSessionName()

	// Create session
	if err := client.CreateSession(ctx, sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(ctx, sessionName)

	// Create a window
	windowName := "test-window"
	if err := client.CreateWindow(ctx, sessionName, windowName); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// Get pane PID
	pid, err := client.GetPanePID(ctx, sessionName, windowName)
	if err != nil {
		t.Fatalf("Failed to get pane PID: %v", err)
	}

	// PID should be positive
	if pid <= 0 {
		t.Errorf("Expected positive PID, got %d", pid)
	}

	// Verify PID corresponds to a running process
	process, err := os.FindProcess(pid)
	if err != nil {
		t.Errorf("Failed to find process with PID %d: %v", pid, err)
	}
	if process == nil {
		t.Errorf("Process with PID %d not found", pid)
	}
}

func TestMultipleSessions(t *testing.T) {
	skipIfCannotCreateSessions(t)
	ctx := context.Background()
	client := NewClient()

	// Create multiple test sessions with unique names
	session1 := fmt.Sprintf("test-tmux-%d-1", time.Now().UnixNano())
	time.Sleep(1 * time.Millisecond)
	session2 := fmt.Sprintf("test-tmux-%d-2", time.Now().UnixNano())
	time.Sleep(1 * time.Millisecond)
	session3 := fmt.Sprintf("test-tmux-%d-3", time.Now().UnixNano())

	if err := client.CreateSession(ctx, session1, true); err != nil {
		t.Fatalf("Failed to create session1: %v", err)
	}
	defer client.KillSession(ctx, session1)

	if err := client.CreateSession(ctx, session2, true); err != nil {
		t.Fatalf("Failed to create session2: %v", err)
	}
	defer client.KillSession(ctx, session2)

	if err := client.CreateSession(ctx, session3, true); err != nil {
		t.Fatalf("Failed to create session3: %v", err)
	}
	defer client.KillSession(ctx, session3)

	// Verify all sessions exist
	sessions, err := client.ListSessions(ctx)
	if err != nil {
		t.Fatalf("Failed to list sessions: %v", err)
	}

	sessionMap := make(map[string]bool)
	for _, s := range sessions {
		sessionMap[s] = true
	}

	if !sessionMap[session1] || !sessionMap[session2] || !sessionMap[session3] {
		t.Error("Not all created sessions found in list")
	}

	// Create windows in different sessions
	if err := client.CreateWindow(ctx, session1, "win1"); err != nil {
		t.Fatalf("Failed to create window in session1: %v", err)
	}
	if err := client.CreateWindow(ctx, session2, "win2"); err != nil {
		t.Fatalf("Failed to create window in session2: %v", err)
	}

	// Verify windows are in correct sessions
	hasWin1, _ := client.HasWindow(ctx, session1, "win1")
	hasWin2, _ := client.HasWindow(ctx, session2, "win2")
	hasWin1InSession2, _ := client.HasWindow(ctx, session2, "win1")

	if !hasWin1 {
		t.Error("win1 should exist in session1")
	}
	if !hasWin2 {
		t.Error("win2 should exist in session2")
	}
	if hasWin1InSession2 {
		t.Error("win1 should not exist in session2")
	}
}

func TestErrorHandling(t *testing.T) {
	ctx := context.Background()
	client := NewClient()

	// Test operations on non-existent session
	err := client.CreateWindow(ctx, "nonexistent-session", "window")
	if err == nil {
		t.Error("CreateWindow on non-existent session should fail")
	}
	// Verify it's a CommandError
	if _, ok := err.(*CommandError); !ok {
		t.Errorf("Expected CommandError, got %T", err)
	}

	err = client.KillWindow(ctx, "nonexistent-session", "window")
	if err == nil {
		t.Error("KillWindow on non-existent session should fail")
	}

	_, err = client.GetPanePID(ctx, "nonexistent-session", "window")
	if err == nil {
		t.Error("GetPanePID on non-existent session should fail")
	}

	// Test ListWindows on non-existent session
	_, err = client.ListWindows(ctx, "nonexistent-session")
	if err == nil {
		t.Error("ListWindows on non-existent session should fail")
	}
}

func TestContextCancellation(t *testing.T) {
	client := NewClient()
	sessionName := uniqueSessionName()

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Operations should fail with context error
	_, err := client.HasSession(ctx, sessionName)
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}

	err = client.CreateSession(ctx, sessionName, true)
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

func TestPipePane(t *testing.T) {
	skipIfCannotCreateSessions(t)
	ctx := context.Background()
	client := NewClient()
	session := uniqueSessionName()
	window := "testwindow"

	// Create session with a named window using tmux directly
	cmd := exec.Command("tmux", "new-session", "-d", "-s", session, "-n", window)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(ctx, session)

	// Create a temp file to capture output
	tmpFile, err := os.CreateTemp("", "pipe-pane-test-*.log")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	// Start pipe-pane
	if err := client.StartPipePane(ctx, session, window, tmpFile.Name()); err != nil {
		t.Fatalf("StartPipePane failed: %v", err)
	}

	// Send some output to the pane
	testMessage := "Hello from pipe-pane test"
	if err := client.SendKeys(ctx, session, window, fmt.Sprintf("echo '%s'", testMessage)); err != nil {
		t.Fatalf("Failed to send keys: %v", err)
	}

	// Wait for output to be captured
	time.Sleep(500 * time.Millisecond)

	// Read the captured output
	content, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	// Verify the output was captured
	if !strings.Contains(string(content), testMessage) {
		t.Errorf("Expected output to contain %q, got %q", testMessage, string(content))
	}

	// Stop pipe-pane
	if err := client.StopPipePane(ctx, session, window); err != nil {
		t.Fatalf("StopPipePane failed: %v", err)
	}

	// Send more output after stopping
	if err := client.SendKeys(ctx, session, window, "echo 'This should not be captured'"); err != nil {
		t.Fatalf("Failed to send keys: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	// Verify the file size hasn't changed much (new output shouldn't be captured)
	content2, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	// The file might have grown a bit due to the echo command appearing in the prompt
	// but the actual "This should not be captured" message should not appear
	if strings.Contains(string(content2), "This should not be captured") {
		t.Error("Output was captured after StopPipePane was called")
	}
}

func TestPipePaneErrorHandling(t *testing.T) {
	ctx := context.Background()
	client := NewClient()

	// Test StartPipePane on non-existent session
	err := client.StartPipePane(ctx, "nonexistent-session", "window", "/tmp/test.log")
	if err == nil {
		t.Error("StartPipePane on non-existent session should fail")
	}

	// Test StopPipePane on non-existent session
	err = client.StopPipePane(ctx, "nonexistent-session", "window")
	if err == nil {
		t.Error("StopPipePane on non-existent session should fail")
	}
}

func TestCustomErrorTypes(t *testing.T) {
	// Test SessionNotFoundError
	sessionErr := &SessionNotFoundError{Name: "test-session"}
	if sessionErr.Error() != "tmux session not found: test-session" {
		t.Errorf("Unexpected error message: %s", sessionErr.Error())
	}
	if !IsSessionNotFound(sessionErr) {
		t.Error("IsSessionNotFound should return true")
	}
	if IsWindowNotFound(sessionErr) {
		t.Error("IsWindowNotFound should return false for SessionNotFoundError")
	}

	// Test WindowNotFoundError
	windowErr := &WindowNotFoundError{Session: "test-session", Window: "test-window"}
	if windowErr.Error() != "tmux window not found: test-window in session test-session" {
		t.Errorf("Unexpected error message: %s", windowErr.Error())
	}
	if !IsWindowNotFound(windowErr) {
		t.Error("IsWindowNotFound should return true")
	}
	if IsSessionNotFound(windowErr) {
		t.Error("IsSessionNotFound should return false for WindowNotFoundError")
	}

	// Test CommandError
	cmdErr := &CommandError{Op: "send-keys", Session: "sess", Window: "win", Err: fmt.Errorf("underlying error")}
	expected := "tmux send-keys failed for sess:win: underlying error"
	if cmdErr.Error() != expected {
		t.Errorf("Expected %q, got %q", expected, cmdErr.Error())
	}
	if cmdErr.Unwrap() == nil {
		t.Error("CommandError.Unwrap should return underlying error")
	}
}

func TestSessionNotFoundErrorIs(t *testing.T) {
	err1 := &SessionNotFoundError{Name: "session1"}
	err2 := &SessionNotFoundError{Name: "session2"}

	// Is should return true for same error type regardless of name
	if !err1.Is(err2) {
		t.Error("SessionNotFoundError.Is should return true for another SessionNotFoundError")
	}

	// Is should return false for different error types
	windowErr := &WindowNotFoundError{Session: "sess", Window: "win"}
	if err1.Is(windowErr) {
		t.Error("SessionNotFoundError.Is should return false for WindowNotFoundError")
	}

	// Is should return false for generic errors
	genericErr := fmt.Errorf("some error")
	if err1.Is(genericErr) {
		t.Error("SessionNotFoundError.Is should return false for generic error")
	}
}

func TestWindowNotFoundErrorIs(t *testing.T) {
	err1 := &WindowNotFoundError{Session: "sess1", Window: "win1"}
	err2 := &WindowNotFoundError{Session: "sess2", Window: "win2"}

	// Is should return true for same error type regardless of session/window
	if !err1.Is(err2) {
		t.Error("WindowNotFoundError.Is should return true for another WindowNotFoundError")
	}

	// Is should return false for different error types
	sessionErr := &SessionNotFoundError{Name: "sess"}
	if err1.Is(sessionErr) {
		t.Error("WindowNotFoundError.Is should return false for SessionNotFoundError")
	}

	// Is should return false for generic errors
	genericErr := fmt.Errorf("some error")
	if err1.Is(genericErr) {
		t.Error("WindowNotFoundError.Is should return false for generic error")
	}
}

func TestCommandErrorVariants(t *testing.T) {
	// Test CommandError with only operation (no session or window)
	cmdErrNoSession := &CommandError{Op: "list-sessions", Err: fmt.Errorf("failed")}
	expected := "tmux list-sessions failed: failed"
	if cmdErrNoSession.Error() != expected {
		t.Errorf("Expected %q, got %q", expected, cmdErrNoSession.Error())
	}

	// Test CommandError with session but no window
	cmdErrWithSession := &CommandError{Op: "kill-session", Session: "test-session", Err: fmt.Errorf("not found")}
	expected = "tmux kill-session failed for session test-session: not found"
	if cmdErrWithSession.Error() != expected {
		t.Errorf("Expected %q, got %q", expected, cmdErrWithSession.Error())
	}

	// Test CommandError with session and window (already covered in TestCustomErrorTypes)
	cmdErrFull := &CommandError{Op: "send-keys", Session: "sess", Window: "win", Err: fmt.Errorf("error")}
	expected = "tmux send-keys failed for sess:win: error"
	if cmdErrFull.Error() != expected {
		t.Errorf("Expected %q, got %q", expected, cmdErrFull.Error())
	}
}

func TestIsHelperFunctionsWithNil(t *testing.T) {
	// Test helper functions with nil
	if IsSessionNotFound(nil) {
		t.Error("IsSessionNotFound(nil) should return false")
	}
	if IsWindowNotFound(nil) {
		t.Error("IsWindowNotFound(nil) should return false")
	}
}

func TestIsHelperFunctionsWithGenericErrors(t *testing.T) {
	genericErr := fmt.Errorf("some generic error")

	if IsSessionNotFound(genericErr) {
		t.Error("IsSessionNotFound should return false for generic error")
	}
	if IsWindowNotFound(genericErr) {
		t.Error("IsWindowNotFound should return false for generic error")
	}
}

// BenchmarkSendKeys measures the performance of sending keys to a tmux pane.
func BenchmarkSendKeys(b *testing.B) {
	if !canCreateSessions {
		b.Skip("tmux cannot create sessions in this environment (headless CI?)")
	}
	ctx := context.Background()
	client := NewClient()
	sessionName := fmt.Sprintf("bench-tmux-%d", time.Now().UnixNano())

	if err := client.CreateSession(ctx, sessionName, true); err != nil {
		b.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(ctx, sessionName)

	windowName := "bench-window"
	if err := client.CreateWindow(ctx, sessionName, windowName); err != nil {
		b.Fatalf("Failed to create window: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client.SendKeysLiteral(ctx, sessionName, windowName, "test message")
	}
}

// BenchmarkSendKeysMultiline measures sending multiline text via paste-buffer.
func BenchmarkSendKeysMultiline(b *testing.B) {
	if !canCreateSessions {
		b.Skip("tmux cannot create sessions in this environment (headless CI?)")
	}
	ctx := context.Background()
	client := NewClient()
	sessionName := fmt.Sprintf("bench-tmux-%d", time.Now().UnixNano())

	if err := client.CreateSession(ctx, sessionName, true); err != nil {
		b.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(ctx, sessionName)

	windowName := "bench-window"
	if err := client.CreateWindow(ctx, sessionName, windowName); err != nil {
		b.Fatalf("Failed to create window: %v", err)
	}

	multilineText := "line1\nline2\nline3\nline4\nline5"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client.SendKeysLiteral(ctx, sessionName, windowName, multilineText)
	}
}

func TestKillSessionNonExistent(t *testing.T) {
	ctx := context.Background()
	client := NewClient()

	// Killing a non-existent session should return an error
	err := client.KillSession(ctx, "nonexistent-session-kill-test")
	if err == nil {
		t.Error("KillSession on non-existent session should fail")
	}

	// Verify it's a CommandError
	if _, ok := err.(*CommandError); !ok {
		t.Errorf("Expected CommandError, got %T: %v", err, err)
	}
}

func TestSendKeysOnNonExistentSession(t *testing.T) {
	ctx := context.Background()
	client := NewClient()

	// SendKeys on non-existent session should return an error
	err := client.SendKeys(ctx, "nonexistent-session-sendkeys-test", "window", "test")
	if err == nil {
		t.Error("SendKeys on non-existent session should fail")
	}

	// Verify it's a CommandError
	if _, ok := err.(*CommandError); !ok {
		t.Errorf("Expected CommandError, got %T: %v", err, err)
	}
}

func TestSendKeysLiteralOnNonExistentSession(t *testing.T) {
	ctx := context.Background()
	client := NewClient()

	// SendKeysLiteral on non-existent session should return an error
	err := client.SendKeysLiteral(ctx, "nonexistent-session-literal-test", "window", "test")
	if err == nil {
		t.Error("SendKeysLiteral on non-existent session should fail")
	}

	// Verify it's a CommandError
	if _, ok := err.(*CommandError); !ok {
		t.Errorf("Expected CommandError, got %T: %v", err, err)
	}
}

func TestSendEnterOnNonExistentSession(t *testing.T) {
	ctx := context.Background()
	client := NewClient()

	// SendEnter on non-existent session should return an error
	err := client.SendEnter(ctx, "nonexistent-session-enter-test", "window")
	if err == nil {
		t.Error("SendEnter on non-existent session should fail")
	}

	// Verify it's a CommandError
	if _, ok := err.(*CommandError); !ok {
		t.Errorf("Expected CommandError, got %T: %v", err, err)
	}
}

func TestSendKeysLiteralWithEnterOnNonExistentSession(t *testing.T) {
	ctx := context.Background()
	client := NewClient()

	// SendKeysLiteralWithEnter on non-existent session should return an error
	err := client.SendKeysLiteralWithEnter(ctx, "nonexistent-session-atomic-test", "window", "test")
	if err == nil {
		t.Error("SendKeysLiteralWithEnter on non-existent session should fail")
	}

	// Verify it's a CommandError
	if _, ok := err.(*CommandError); !ok {
		t.Errorf("Expected CommandError, got %T: %v", err, err)
	}
}

func TestHasWindowOnNonExistentSession(t *testing.T) {
	ctx := context.Background()
	client := NewClient()

	// HasWindow on non-existent session should return an error
	_, err := client.HasWindow(ctx, "nonexistent-session-haswindow-test", "window")
	if err == nil {
		t.Error("HasWindow on non-existent session should fail")
	}

	// Verify it's a CommandError
	if _, ok := err.(*CommandError); !ok {
		t.Errorf("Expected CommandError, got %T: %v", err, err)
	}
}

func TestListSessionsNoSessions(t *testing.T) {
	skipIfCannotCreateSessions(t)
	// This test verifies behavior when listing sessions and no sessions exist
	// Since we need at least one session for the test framework, we can't easily
	// test the "no sessions" case, but we test that the exit code 1 case is handled
	ctx := context.Background()
	client := NewClient()

	// Create and immediately kill a session to ensure we're in a state
	// where we know what sessions exist
	sessionName := uniqueSessionName()
	if err := client.CreateSession(ctx, sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Kill session
	if err := client.KillSession(ctx, sessionName); err != nil {
		t.Fatalf("Failed to kill session: %v", err)
	}

	// Wait for session to be gone
	if err := waitForNoSession(ctx, client, sessionName, 2*time.Second); err != nil {
		t.Fatalf("Session still visible after killing: %v", err)
	}

	// List sessions should work (might have 0 or more sessions from other tests)
	sessions, err := client.ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}

	// The killed session should not be in the list
	for _, s := range sessions {
		if s == sessionName {
			t.Errorf("Killed session %s should not be in session list", sessionName)
		}
	}
}

func TestListSessionsContextCancellation(t *testing.T) {
	client := NewClient()

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// ListSessions should return context error
	_, err := client.ListSessions(ctx)
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

func TestSendKeysLiteralMultilineContextCancellation(t *testing.T) {
	client := NewClient()

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// SendKeysLiteral with multiline should return context error
	multiline := "line1\nline2"
	err := client.SendKeysLiteral(ctx, "session", "window", multiline)
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled for multiline, got %v", err)
	}

	// SendKeysLiteral with single line should also return context error
	err = client.SendKeysLiteral(ctx, "session", "window", "single line")
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled for single line, got %v", err)
	}
}

func TestCreateWindowContextCancellation(t *testing.T) {
	client := NewClient()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := client.CreateWindow(ctx, "session", "window")
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

func TestKillWindowContextCancellation(t *testing.T) {
	client := NewClient()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := client.KillWindow(ctx, "session", "window")
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

func TestListWindowsContextCancellation(t *testing.T) {
	client := NewClient()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.ListWindows(ctx, "session")
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

func TestGetPanePIDContextCancellation(t *testing.T) {
	client := NewClient()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.GetPanePID(ctx, "session", "window")
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

func TestStartPipePaneContextCancellation(t *testing.T) {
	client := NewClient()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := client.StartPipePane(ctx, "session", "window", "/tmp/test.log")
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

func TestStopPipePaneContextCancellation(t *testing.T) {
	client := NewClient()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := client.StopPipePane(ctx, "session", "window")
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}
