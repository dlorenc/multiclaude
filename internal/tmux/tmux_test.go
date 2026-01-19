package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// These tests verify that the internal/tmux package correctly re-exports
// functionality from pkg/tmux. The comprehensive tests are in pkg/tmux.

// TestMain ensures clean tmux environment for tests
func TestMain(m *testing.M) {
	// Skip tmux integration tests in CI environments unless TMUX_TESTS=1 is set
	if os.Getenv("CI") != "" && os.Getenv("TMUX_TESTS") != "1" {
		fmt.Fprintln(os.Stderr, "Skipping tmux tests in CI (set TMUX_TESTS=1 to enable)")
		os.Exit(0)
	}

	// Check if tmux is available
	if exec.Command("tmux", "-V").Run() != nil {
		fmt.Fprintln(os.Stderr, "Warning: tmux not available, skipping tmux tests")
		os.Exit(0)
	}

	// Verify we can actually create sessions
	testSession := fmt.Sprintf("test-tmux-probe-%d", time.Now().UnixNano())
	cmd := exec.Command("tmux", "new-session", "-d", "-s", testSession)
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "Warning: tmux cannot create sessions (no terminal?), skipping tmux tests")
		os.Exit(0)
	}
	exec.Command("tmux", "kill-session", "-t", testSession).Run()

	code := m.Run()
	cleanupTestSessions()
	os.Exit(code)
}

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

func uniqueSessionName() string {
	return fmt.Sprintf("test-tmux-%d", time.Now().UnixNano())
}

func waitForSession(client *Client, sessionName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	pollInterval := 10 * time.Millisecond

	for time.Now().Before(deadline) {
		exists, err := client.HasSession(sessionName)
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

func waitForNoSession(client *Client, sessionName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	pollInterval := 10 * time.Millisecond

	for time.Now().Before(deadline) {
		exists, err := client.HasSession(sessionName)
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

// TestReexport verifies that the re-exported types work correctly.
func TestReexport(t *testing.T) {
	client := NewClient()
	if client == nil {
		t.Fatal("NewClient() returned nil")
	}
	if !client.IsTmuxAvailable() {
		t.Error("Expected tmux to be available")
	}
}

// TestBasicOperations verifies core tmux operations through the re-export.
func TestBasicOperations(t *testing.T) {
	client := NewClient()
	sessionName := uniqueSessionName()

	// Create session
	if err := client.CreateSession(sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(sessionName)

	if err := waitForSession(client, sessionName, 2*time.Second); err != nil {
		t.Fatalf("Session not visible: %v", err)
	}

	// Create window
	windowName := "test-window"
	if err := client.CreateWindow(sessionName, windowName); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// Verify window exists
	exists, err := client.HasWindow(sessionName, windowName)
	if err != nil {
		t.Fatalf("HasWindow failed: %v", err)
	}
	if !exists {
		t.Error("Window should exist")
	}

	// Get PID
	pid, err := client.GetPanePID(sessionName, windowName)
	if err != nil {
		t.Fatalf("GetPanePID failed: %v", err)
	}
	if pid <= 0 {
		t.Errorf("Expected positive PID, got %d", pid)
	}
}

// TestSendKeys verifies send keys through the re-export.
func TestSendKeys(t *testing.T) {
	client := NewClient()
	sessionName := uniqueSessionName()

	if err := client.CreateSession(sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(sessionName)

	windowName := "test-window"
	if err := client.CreateWindow(sessionName, windowName); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// SendKeysLiteral should not error
	if err := client.SendKeysLiteral(sessionName, windowName, "echo test"); err != nil {
		t.Fatalf("Failed to send keys literal: %v", err)
	}

	// SendEnter should not error
	if err := client.SendEnter(sessionName, windowName); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}
}

// TestMultilineText verifies multiline text through the re-export.
func TestMultilineText(t *testing.T) {
	client := NewClient()
	sessionName := uniqueSessionName()

	if err := client.CreateSession(sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(sessionName)

	windowName := "test-window"
	if err := client.CreateWindow(sessionName, windowName); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// Test multiline text
	multiLineText := "line1\nline2\nline3"
	if err := client.SendKeysLiteral(sessionName, windowName, multiLineText); err != nil {
		t.Fatalf("Failed to send multi-line text: %v", err)
	}
}

// TestPipePane verifies pipe-pane through the re-export.
func TestPipePane(t *testing.T) {
	client := NewClient()
	session := uniqueSessionName()
	window := "testwindow"

	cmd := exec.Command("tmux", "new-session", "-d", "-s", session, "-n", window)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(session)

	tmpFile, err := os.CreateTemp("", "pipe-pane-test-*.log")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	// Start pipe-pane
	if err := client.StartPipePane(session, window, tmpFile.Name()); err != nil {
		t.Fatalf("StartPipePane failed: %v", err)
	}

	// Send output
	testMessage := "Hello from pipe-pane test"
	if err := client.SendKeys(session, window, fmt.Sprintf("echo '%s'", testMessage)); err != nil {
		t.Fatalf("Failed to send keys: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Read output
	content, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	if !strings.Contains(string(content), testMessage) {
		t.Errorf("Expected output to contain %q, got %q", testMessage, string(content))
	}

	// Stop pipe-pane
	if err := client.StopPipePane(session, window); err != nil {
		t.Fatalf("StopPipePane failed: %v", err)
	}
}

// TestSendKeysLiteralWithEnter_Atomic verifies that single-line messages are sent
// atomically with their Enter key in a single exec call.
func TestSendKeysLiteralWithEnter_Atomic(t *testing.T) {
	client := NewClient()
	sessionName := uniqueSessionName()

	// Create session with a window
	if err := client.CreateSession(sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(sessionName)

	windowName := "test-window"
	if err := client.CreateWindow(sessionName, windowName); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// Create a temp file to collect output
	testFile := fmt.Sprintf("/tmp/atomic-test-%d.log", time.Now().UnixNano())
	defer os.Remove(testFile)

	// Send the command that will be executed when Enter is received
	// The atomic method should send both text and Enter in one call
	if err := client.SendKeysLiteralWithEnter(sessionName, windowName, fmt.Sprintf("echo 'atomic-test' >> %s", testFile)); err != nil {
		t.Fatalf("SendKeysLiteralWithEnter failed: %v", err)
	}

	// Wait for execution
	timeout := 3 * time.Second
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if content, err := os.ReadFile(testFile); err == nil && strings.Contains(string(content), "atomic-test") {
			return // Success
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Error("SendKeysLiteralWithEnter did not execute command - Enter was not delivered atomically")
}

// TestSendKeysLiteralWithEnter_RapidMessages tests that 100 rapid messages are all
// delivered reliably using the atomic method. This is the core test for issue #63.
func TestSendKeysLiteralWithEnter_RapidMessages(t *testing.T) {
	client := NewClient()
	sessionName := uniqueSessionName()

	// Create session with a window
	if err := client.CreateSession(sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(sessionName)

	windowName := "test-window"
	if err := client.CreateWindow(sessionName, windowName); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// Create a temp file to collect output
	testFile := fmt.Sprintf("/tmp/rapid-test-%d.log", time.Now().UnixNano())
	defer os.Remove(testFile)

	// Send 100 messages rapidly using atomic method
	numMessages := 100
	for i := 0; i < numMessages; i++ {
		msg := fmt.Sprintf("echo 'MSG_%d' >> %s", i, testFile)
		if err := client.SendKeysLiteralWithEnter(sessionName, windowName, msg); err != nil {
			t.Fatalf("SendKeysLiteralWithEnter failed on message %d: %v", i, err)
		}
	}

	// Wait for all messages to be processed
	time.Sleep(5 * time.Second)

	// Read the file and count received messages
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read test file: %v", err)
	}

	receivedCount := 0
	for i := 0; i < numMessages; i++ {
		if strings.Contains(string(content), fmt.Sprintf("MSG_%d", i)) {
			receivedCount++
		}
	}

	if receivedCount != numMessages {
		t.Errorf("Only %d/%d messages were received. Atomic delivery may have failed.", receivedCount, numMessages)
	}
}

// TestSendKeysLiteralWithEnter_Multiline verifies that multiline text is sent atomically
// using tmux command chaining (set-buffer ; paste-buffer ; send-keys).
func TestSendKeysLiteralWithEnter_Multiline(t *testing.T) {
	client := NewClient()
	sessionName := uniqueSessionName()

	// Create session with a window
	if err := client.CreateSession(sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(sessionName)

	windowName := "test-window"
	if err := client.CreateWindow(sessionName, windowName); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// Create a temp file to collect output
	testFile := fmt.Sprintf("/tmp/multiline-test-%d.log", time.Now().UnixNano())
	defer os.Remove(testFile)

	// Prepare multiline text - a here-doc that writes multiple lines
	multilineMsg := fmt.Sprintf("cat >> %s << 'EOF'\nline1\nline2\nline3\nEOF", testFile)

	if err := client.SendKeysLiteralWithEnter(sessionName, windowName, multilineMsg); err != nil {
		t.Fatalf("SendKeysLiteralWithEnter (multiline) failed: %v", err)
	}

	// Wait for execution
	time.Sleep(3 * time.Second)

	// Read the file and verify all lines
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read test file: %v", err)
	}

	if !strings.Contains(string(content), "line1") ||
		!strings.Contains(string(content), "line2") ||
		!strings.Contains(string(content), "line3") {
		t.Errorf("Multiline content not fully delivered. Got: %s", string(content))
	}
}

// TestSendKeysLiteralWithEnter_MultilineRapid tests rapid multiline messages
func TestSendKeysLiteralWithEnter_MultilineRapid(t *testing.T) {
	client := NewClient()
	sessionName := uniqueSessionName()

	// Create session with a window
	if err := client.CreateSession(sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(sessionName)

	windowName := "test-window"
	if err := client.CreateWindow(sessionName, windowName); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// Create a temp file to collect output
	testFile := fmt.Sprintf("/tmp/multiline-rapid-%d.log", time.Now().UnixNano())
	defer os.Remove(testFile)

	// Send 20 multiline messages rapidly
	numMessages := 20
	for i := 0; i < numMessages; i++ {
		multilineMsg := fmt.Sprintf("cat >> %s << 'EOF'\nMSG_%d_LINE1\nMSG_%d_LINE2\nEOF", testFile, i, i)
		if err := client.SendKeysLiteralWithEnter(sessionName, windowName, multilineMsg); err != nil {
			t.Fatalf("SendKeysLiteralWithEnter (multiline) failed on message %d: %v", i, err)
		}
	}

	// Wait for all messages to be processed
	time.Sleep(5 * time.Second)

	// Read the file and count received messages
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read test file: %v", err)
	}

	receivedCount := 0
	for i := 0; i < numMessages; i++ {
		if strings.Contains(string(content), fmt.Sprintf("MSG_%d_LINE1", i)) &&
			strings.Contains(string(content), fmt.Sprintf("MSG_%d_LINE2", i)) {
			receivedCount++
		}
	}

	if receivedCount != numMessages {
		t.Errorf("Only %d/%d multiline messages were received. Atomic delivery may have failed.", receivedCount, numMessages)
	}
}

// TestOldApproach_RaceCondition documents the race condition bug in the old approach
// where SendKeysLiteral and SendEnter are called separately.
// This test simulates the race with an artificial delay to demonstrate why atomic
// sending is necessary. It may not always fail deterministically (race conditions
// are by nature non-deterministic), but documents the problematic pattern.
func TestOldApproach_RaceCondition(t *testing.T) {
	client := NewClient()
	sessionName := uniqueSessionName()

	// Create session with a window
	if err := client.CreateSession(sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(sessionName)

	windowName := "test-window"
	if err := client.CreateWindow(sessionName, windowName); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// Create a temp file to collect output
	testFile := fmt.Sprintf("/tmp/race-test-%d.log", time.Now().UnixNano())
	defer os.Remove(testFile)

	// This test documents the race condition by using the OLD pattern:
	// 1. SendKeysLiteral (text without Enter)
	// 2. Artificial delay (simulates process scheduling/context switch)
	// 3. SendEnter
	//
	// With a delay, the Enter might arrive at the wrong time, causing
	// message delivery failures. This is what issue #63 is about.

	numMessages := 50
	for i := 0; i < numMessages; i++ {
		msg := fmt.Sprintf("echo 'RACE_%d' >> %s", i, testFile)

		// OLD PATTERN (problematic):
		if err := client.SendKeysLiteral(sessionName, windowName, msg); err != nil {
			t.Fatalf("SendKeysLiteral failed: %v", err)
		}

		// Artificial delay simulating process scheduling
		// In real scenarios, this delay happens unpredictably due to:
		// - Process scheduling
		// - tmux internal state changes
		// - Claude CLI redrawing
		time.Sleep(10 * time.Millisecond)

		if err := client.SendEnter(sessionName, windowName); err != nil {
			t.Fatalf("SendEnter failed: %v", err)
		}
	}

	// Wait for all messages to be processed
	time.Sleep(5 * time.Second)

	// Read the file and count received messages
	content, err := os.ReadFile(testFile)
	if err != nil {
		// File might not exist if nothing was executed
		content = []byte{}
	}

	receivedCount := 0
	for i := 0; i < numMessages; i++ {
		if strings.Contains(string(content), fmt.Sprintf("RACE_%d", i)) {
			receivedCount++
		}
	}

	// Document the result - we don't fail the test because race conditions
	// are non-deterministic, but we log if any messages were lost
	if receivedCount < numMessages {
		t.Logf("RACE CONDITION DOCUMENTED: Only %d/%d messages received with old approach + delay", receivedCount, numMessages)
		t.Logf("This demonstrates why atomic sending (SendKeysLiteralWithEnter) is necessary")
	} else {
		t.Logf("All %d messages received (race condition didn't manifest in this run)", numMessages)
	}
}

// TestSendKeysLiteralWithEnter_ErrorHandling tests error handling
func TestSendKeysLiteralWithEnter_ErrorHandling(t *testing.T) {
	client := NewClient()

	// Test on non-existent session (single line)
	err := client.SendKeysLiteralWithEnter("nonexistent-session", "window", "test message")
	if err == nil {
		t.Error("SendKeysLiteralWithEnter on non-existent session should fail")
	}

	// Test on non-existent session (multiline)
	err = client.SendKeysLiteralWithEnter("nonexistent-session", "window", "line1\nline2")
	if err == nil {
		t.Error("SendKeysLiteralWithEnter (multiline) on non-existent session should fail")
	}
}

// TestSendKeysLiteralWithEnter_SpecialCharacters tests that special characters are handled correctly
func TestSendKeysLiteralWithEnter_SpecialCharacters(t *testing.T) {
	client := NewClient()
	sessionName := uniqueSessionName()

	// Create session with a window
	if err := client.CreateSession(sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(sessionName)

	windowName := "test-window"
	if err := client.CreateWindow(sessionName, windowName); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	testFile := fmt.Sprintf("/tmp/special-chars-%d.log", time.Now().UnixNano())
	defer os.Remove(testFile)

	// Test messages with special characters that might be interpreted by tmux
	testCases := []struct {
		name    string
		marker  string
		message string
	}{
		{"emoji", "EMOJI", "echo '📨 Message: test' >> " + testFile + " && echo 'EMOJI' >> " + testFile},
		{"quotes", "QUOTES", "echo \"double'quotes\" >> " + testFile + " && echo 'QUOTES' >> " + testFile},
		{"backslash", "BACKSLASH", "echo 'back\\slash' >> " + testFile + " && echo 'BACKSLASH' >> " + testFile},
	}

	for _, tc := range testCases {
		if err := client.SendKeysLiteralWithEnter(sessionName, windowName, tc.message); err != nil {
			t.Errorf("SendKeysLiteralWithEnter failed for %s: %v", tc.name, err)
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Wait for execution
	time.Sleep(2 * time.Second)

	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read test file: %v", err)
	}

	for _, tc := range testCases {
		if !strings.Contains(string(content), tc.marker) {
			t.Errorf("Message with %s was not delivered correctly", tc.name)
		}
	}
}
