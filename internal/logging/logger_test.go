package logging

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(buf)

	if logger == nil {
		t.Fatal("New() returned nil")
	}

	if logger.writer != buf {
		t.Error("New() did not set writer correctly")
	}

	if logger.logger == nil {
		t.Error("New() did not initialize internal logger")
	}
}

func TestNewFile(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	logger, err := NewFile(logPath)
	if err != nil {
		t.Fatalf("NewFile() error = %v", err)
	}

	if logger == nil {
		t.Fatal("NewFile() returned nil logger")
	}

	// Verify file was created
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Error("NewFile() did not create log file")
	}

	// Clean up
	logger.Close()
}

func TestNewFileError(t *testing.T) {
	// Try to create file in non-existent directory
	_, err := NewFile("/nonexistent/path/test.log")
	if err == nil {
		t.Error("NewFile() should return error for invalid path")
	}
}

func TestLoggerInfo(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(buf)

	logger.Info("test message %d", 42)

	output := buf.String()
	if !strings.Contains(output, "[INFO]") {
		t.Errorf("Info() output = %q, missing [INFO] prefix", output)
	}
	if !strings.Contains(output, "test message 42") {
		t.Errorf("Info() output = %q, missing message content", output)
	}
}

func TestLoggerWarn(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(buf)

	logger.Warn("warning message %s", "test")

	output := buf.String()
	if !strings.Contains(output, "[WARN]") {
		t.Errorf("Warn() output = %q, missing [WARN] prefix", output)
	}
	if !strings.Contains(output, "warning message test") {
		t.Errorf("Warn() output = %q, missing message content", output)
	}
}

func TestLoggerError(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(buf)

	logger.Error("error message: %v", "something went wrong")

	output := buf.String()
	if !strings.Contains(output, "[ERROR]") {
		t.Errorf("Error() output = %q, missing [ERROR] prefix", output)
	}
	if !strings.Contains(output, "error message: something went wrong") {
		t.Errorf("Error() output = %q, missing message content", output)
	}
}

func TestLoggerDebug(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(buf)

	logger.Debug("debug info: x=%d, y=%d", 1, 2)

	output := buf.String()
	if !strings.Contains(output, "[DEBUG]") {
		t.Errorf("Debug() output = %q, missing [DEBUG] prefix", output)
	}
	if !strings.Contains(output, "debug info: x=1, y=2") {
		t.Errorf("Debug() output = %q, missing message content", output)
	}
}

func TestLoggerClose(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	logger, err := NewFile(logPath)
	if err != nil {
		t.Fatalf("NewFile() error = %v", err)
	}

	// Write something
	logger.Info("test message")

	// Close should succeed
	err = logger.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestLoggerCloseNonFile(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(buf)

	// Close on non-file writer should succeed without error
	err := logger.Close()
	if err != nil {
		t.Errorf("Close() error = %v, expected nil for non-file writer", err)
	}
}

func TestLoggerMultipleWrites(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(buf)

	logger.Info("message 1")
	logger.Warn("message 2")
	logger.Error("message 3")
	logger.Debug("message 4")

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) != 4 {
		t.Errorf("Expected 4 log lines, got %d", len(lines))
	}
}

func TestLoggerThreadSafety(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(buf)

	// Concurrent writes should not panic
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(n int) {
			for j := 0; j < 100; j++ {
				logger.Info("goroutine %d, iteration %d", n, j)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify we got all messages
	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 1000 {
		t.Errorf("Expected 1000 log lines, got %d", len(lines))
	}
}
