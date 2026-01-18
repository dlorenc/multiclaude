package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestMain ensures clean tmux environment for tests
func TestMain(m *testing.M) {
	// Check if tmux is available
	if exec.Command("tmux", "-V").Run() != nil {
		fmt.Fprintln(os.Stderr, "Warning: tmux not available, skipping tmux tests")
		os.Exit(0)
	}

	// Run tests
	code := m.Run()

	// Cleanup any test sessions that might have leaked
	cleanupTestSessions()

	os.Exit(code)
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

func TestIsTmuxAvailable(t *testing.T) {
	client := NewClient()
	if !client.IsTmuxAvailable() {
		t.Error("Expected tmux to be available")
	}
}

func TestHasSession(t *testing.T) {
	client := NewClient()
	sessionName := uniqueSessionName()

	// Session should not exist initially
	exists, err := client.HasSession(sessionName)
	if err != nil {
		t.Fatalf("HasSession failed: %v", err)
	}
	if exists {
		t.Error("Session should not exist initially")
	}

	// Create session
	if err := client.CreateSession(sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(sessionName)

	// Session should now exist
	exists, err = client.HasSession(sessionName)
	if err != nil {
		t.Fatalf("HasSession failed: %v", err)
	}
	if !exists {
		t.Error("Session should exist after creation")
	}
}

func TestCreateSession(t *testing.T) {
	client := NewClient()
	sessionName := uniqueSessionName()

	// Create detached session
	if err := client.CreateSession(sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(sessionName)

	// Verify session exists
	exists, err := client.HasSession(sessionName)
	if err != nil {
		t.Fatalf("HasSession failed: %v", err)
	}
	if !exists {
		t.Error("Session should exist after creation")
	}

	// Creating duplicate session should fail
	err = client.CreateSession(sessionName, true)
	if err == nil {
		t.Error("Creating duplicate session should fail")
	}
}

func TestCreateWindow(t *testing.T) {
	client := NewClient()
	sessionName := uniqueSessionName()

	// Create session first
	if err := client.CreateSession(sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(sessionName)

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
		t.Error("Window should exist after creation")
	}
}

func TestHasWindow(t *testing.T) {
	client := NewClient()
	sessionName := uniqueSessionName()

	// Create session
	if err := client.CreateSession(sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(sessionName)

	// Non-existent window should return false
	exists, err := client.HasWindow(sessionName, "nonexistent")
	if err != nil {
		t.Fatalf("HasWindow failed: %v", err)
	}
	if exists {
		t.Error("Non-existent window should return false")
	}

	// Create window
	windowName := "test-window"
	if err := client.CreateWindow(sessionName, windowName); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// Window should now exist
	exists, err = client.HasWindow(sessionName, windowName)
	if err != nil {
		t.Fatalf("HasWindow failed: %v", err)
	}
	if !exists {
		t.Error("Window should exist after creation")
	}
}

func TestKillWindow(t *testing.T) {
	client := NewClient()
	sessionName := uniqueSessionName()

	// Create session
	if err := client.CreateSession(sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(sessionName)

	// Create two windows (we need at least 2 to kill one)
	if err := client.CreateWindow(sessionName, "window1"); err != nil {
		t.Fatalf("Failed to create window1: %v", err)
	}
	if err := client.CreateWindow(sessionName, "window2"); err != nil {
		t.Fatalf("Failed to create window2: %v", err)
	}

	// Kill window1
	if err := client.KillWindow(sessionName, "window1"); err != nil {
		t.Fatalf("Failed to kill window: %v", err)
	}

	// Verify window1 no longer exists
	exists, err := client.HasWindow(sessionName, "window1")
	if err != nil {
		t.Fatalf("HasWindow failed: %v", err)
	}
	if exists {
		t.Error("Window should not exist after killing")
	}

	// Verify window2 still exists
	exists, err = client.HasWindow(sessionName, "window2")
	if err != nil {
		t.Fatalf("HasWindow failed: %v", err)
	}
	if !exists {
		t.Error("Window2 should still exist")
	}
}

func TestKillSession(t *testing.T) {
	client := NewClient()
	sessionName := uniqueSessionName()

	// Create session
	if err := client.CreateSession(sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Kill session
	if err := client.KillSession(sessionName); err != nil {
		t.Fatalf("Failed to kill session: %v", err)
	}

	// Verify session no longer exists
	exists, err := client.HasSession(sessionName)
	if err != nil {
		t.Fatalf("HasSession failed: %v", err)
	}
	if exists {
		t.Error("Session should not exist after killing")
	}
}

func TestSendKeys(t *testing.T) {
	client := NewClient()
	sessionName := uniqueSessionName()

	// Create session with a window running a shell
	if err := client.CreateSession(sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(sessionName)

	// Create a window
	windowName := "test-window"
	if err := client.CreateWindow(sessionName, windowName); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// Send keys to create a file (this tests that send-keys works)
	testFile := fmt.Sprintf("/tmp/tmux-test-%d", time.Now().UnixNano())
	defer os.Remove(testFile)

	if err := client.SendKeys(sessionName, windowName, fmt.Sprintf("touch %s", testFile)); err != nil {
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
	client := NewClient()
	sessionName := uniqueSessionName()

	// Create session
	if err := client.CreateSession(sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(sessionName)

	// Create a window
	windowName := "test-window"
	if err := client.CreateWindow(sessionName, windowName); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// SendKeysLiteral should not execute (no Enter key)
	// We can't easily verify this without reading pane content,
	// but we can at least verify it doesn't error
	if err := client.SendKeysLiteral(sessionName, windowName, "echo test"); err != nil {
		t.Fatalf("Failed to send keys literal: %v", err)
	}
}

func TestListSessions(t *testing.T) {
	client := NewClient()

	// Get initial sessions
	initialSessions, err := client.ListSessions()
	if err != nil {
		t.Fatalf("Failed to list sessions: %v", err)
	}

	// Create a test session
	sessionName := uniqueSessionName()
	if err := client.CreateSession(sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(sessionName)

	// List sessions again
	sessions, err := client.ListSessions()
	if err != nil {
		t.Fatalf("Failed to list sessions: %v", err)
	}

	// Should have one more session
	if len(sessions) != len(initialSessions)+1 {
		t.Errorf("Expected %d sessions, got %d", len(initialSessions)+1, len(sessions))
	}

	// Our session should be in the list
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
	client := NewClient()
	sessionName := uniqueSessionName()

	// Create session
	if err := client.CreateSession(sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(sessionName)

	// List windows (should have default window)
	windows, err := client.ListWindows(sessionName)
	if err != nil {
		t.Fatalf("Failed to list windows: %v", err)
	}
	if len(windows) == 0 {
		t.Error("Expected at least one default window")
	}

	// Create additional windows
	if err := client.CreateWindow(sessionName, "window1"); err != nil {
		t.Fatalf("Failed to create window1: %v", err)
	}
	if err := client.CreateWindow(sessionName, "window2"); err != nil {
		t.Fatalf("Failed to create window2: %v", err)
	}

	// List windows again
	windows, err = client.ListWindows(sessionName)
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
	client := NewClient()
	sessionName := uniqueSessionName()

	// Create session
	if err := client.CreateSession(sessionName, true); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer client.KillSession(sessionName)

	// Create a window
	windowName := "test-window"
	if err := client.CreateWindow(sessionName, windowName); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// Get pane PID
	pid, err := client.GetPanePID(sessionName, windowName)
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
	client := NewClient()

	// Create multiple test sessions with unique names
	session1 := fmt.Sprintf("test-tmux-%d-1", time.Now().UnixNano())
	time.Sleep(1 * time.Millisecond)
	session2 := fmt.Sprintf("test-tmux-%d-2", time.Now().UnixNano())
	time.Sleep(1 * time.Millisecond)
	session3 := fmt.Sprintf("test-tmux-%d-3", time.Now().UnixNano())

	if err := client.CreateSession(session1, true); err != nil {
		t.Fatalf("Failed to create session1: %v", err)
	}
	defer client.KillSession(session1)

	if err := client.CreateSession(session2, true); err != nil {
		t.Fatalf("Failed to create session2: %v", err)
	}
	defer client.KillSession(session2)

	if err := client.CreateSession(session3, true); err != nil {
		t.Fatalf("Failed to create session3: %v", err)
	}
	defer client.KillSession(session3)

	// Verify all sessions exist
	sessions, err := client.ListSessions()
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
	if err := client.CreateWindow(session1, "win1"); err != nil {
		t.Fatalf("Failed to create window in session1: %v", err)
	}
	if err := client.CreateWindow(session2, "win2"); err != nil {
		t.Fatalf("Failed to create window in session2: %v", err)
	}

	// Verify windows are in correct sessions
	hasWin1, _ := client.HasWindow(session1, "win1")
	hasWin2, _ := client.HasWindow(session2, "win2")
	hasWin1InSession2, _ := client.HasWindow(session2, "win1")

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
	client := NewClient()

	// Test operations on non-existent session
	err := client.CreateWindow("nonexistent-session", "window")
	if err == nil {
		t.Error("CreateWindow on non-existent session should fail")
	}

	err = client.KillWindow("nonexistent-session", "window")
	if err == nil {
		t.Error("KillWindow on non-existent session should fail")
	}

	_, err = client.GetPanePID("nonexistent-session", "window")
	if err == nil {
		t.Error("GetPanePID on non-existent session should fail")
	}

	// Test ListWindows on non-existent session
	_, err = client.ListWindows("nonexistent-session")
	if err == nil {
		t.Error("ListWindows on non-existent session should fail")
	}
}
