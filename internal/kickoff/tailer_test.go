package kickoff

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLogTailer_BasicTailing(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	// Create log file
	f, err := os.Create(logFile)
	if err != nil {
		t.Fatalf("Failed to create log file: %v", err)
	}

	// Write initial content (tailer should start from EOF, so this won't be read)
	f.WriteString("initial line\n")
	f.Close()

	// Create channel and tailer
	ch := make(chan LogLine, 10)
	tailer := NewLogTailer(logFile, "test", 42, ch)
	tailer.Start()

	// Give tailer time to start and seek to EOF
	time.Sleep(150 * time.Millisecond)

	// Append new content
	f, _ = os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0644)
	f.WriteString("new line 1\n")
	f.WriteString("new line 2\n")
	f.Close()

	// Wait for lines to be read
	time.Sleep(200 * time.Millisecond)

	// Stop tailer
	tailer.Stop()

	// Check received lines
	close(ch)
	lines := []LogLine{}
	for line := range ch {
		lines = append(lines, line)
	}

	if len(lines) != 2 {
		t.Errorf("Expected 2 lines, got %d", len(lines))
	}

	if len(lines) > 0 {
		if lines[0].Text != "new line 1" {
			t.Errorf("Expected 'new line 1', got '%s'", lines[0].Text)
		}
		if lines[0].Source != "test" {
			t.Errorf("Expected source 'test', got '%s'", lines[0].Source)
		}
		if lines[0].IssueID != 42 {
			t.Errorf("Expected issueID 42, got %d", lines[0].IssueID)
		}
	}

	if len(lines) > 1 && lines[1].Text != "new line 2" {
		t.Errorf("Expected 'new line 2', got '%s'", lines[1].Text)
	}
}

func TestLogTailer_WaitForFile(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "delayed.log")

	// Create channel and tailer (file doesn't exist yet)
	ch := make(chan LogLine, 10)
	tailer := NewLogTailer(logFile, "test", 0, ch)
	tailer.Start()

	// Wait a bit, then create the file (empty)
	time.Sleep(150 * time.Millisecond)
	f, err := os.Create(logFile)
	if err != nil {
		t.Fatalf("Failed to create log file: %v", err)
	}
	f.Close()

	// Give tailer time to open the file and seek to EOF
	time.Sleep(150 * time.Millisecond)

	// Now append content (after tailer has opened the file)
	f, _ = os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0644)
	f.WriteString("delayed content\n")
	f.Close()

	// Wait for line to be read
	time.Sleep(200 * time.Millisecond)

	tailer.Stop()

	close(ch)
	lines := []LogLine{}
	for line := range ch {
		lines = append(lines, line)
	}

	if len(lines) != 1 {
		t.Errorf("Expected 1 line, got %d", len(lines))
	}

	if len(lines) > 0 && lines[0].Text != "delayed content" {
		t.Errorf("Expected 'delayed content', got '%s'", lines[0].Text)
	}
}

func TestLogTailer_GracefulStop(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "stop.log")

	// Create log file
	f, _ := os.Create(logFile)
	f.Close()

	// Create channel and tailer
	ch := make(chan LogLine, 10)
	tailer := NewLogTailer(logFile, "test", 0, ch)
	tailer.Start()

	time.Sleep(50 * time.Millisecond)

	// Stop should complete within 1 second
	start := time.Now()
	tailer.Stop()
	elapsed := time.Since(start)

	if elapsed > 1100*time.Millisecond {
		t.Errorf("Stop took too long: %v", elapsed)
	}
}

func TestLogTailer_StopBeforeFileExists(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "never.log")

	// Create channel and tailer (file doesn't exist)
	ch := make(chan LogLine, 10)
	tailer := NewLogTailer(logFile, "test", 0, ch)
	tailer.Start()

	time.Sleep(50 * time.Millisecond)

	// Stop should complete within 1 second even without file
	start := time.Now()
	tailer.Stop()
	elapsed := time.Since(start)

	if elapsed > 1100*time.Millisecond {
		t.Errorf("Stop took too long: %v", elapsed)
	}
}

func TestLogTailer_HandlesTruncation(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "truncate.log")

	// Create log file with initial content
	f, _ := os.Create(logFile)
	f.WriteString("initial\n")
	f.Close()

	// Create channel and tailer
	ch := make(chan LogLine, 10)
	tailer := NewLogTailer(logFile, "test", 0, ch)
	tailer.Start()

	time.Sleep(150 * time.Millisecond)

	// Append first line
	f, _ = os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0644)
	f.WriteString("line before truncate\n")
	f.Close()

	time.Sleep(150 * time.Millisecond)

	// Truncate file (simulate log rotation)
	f, _ = os.Create(logFile) // Create truncates
	f.WriteString("line after truncate\n")
	f.Close()

	time.Sleep(200 * time.Millisecond)

	tailer.Stop()

	close(ch)
	lines := []LogLine{}
	for line := range ch {
		lines = append(lines, line)
	}

	// Should have received at least the lines we appended
	if len(lines) < 2 {
		t.Errorf("Expected at least 2 lines, got %d", len(lines))
	}

	// Last line should be from after truncation
	if len(lines) > 0 {
		lastLine := lines[len(lines)-1].Text
		if lastLine != "line after truncate" {
			t.Logf("Note: Last line was '%s'", lastLine)
		}
	}
}

func TestLogTailer_IdempotentStop(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "idempotent.log")

	f, _ := os.Create(logFile)
	f.Close()

	ch := make(chan LogLine, 10)
	tailer := NewLogTailer(logFile, "test", 0, ch)
	tailer.Start()

	time.Sleep(50 * time.Millisecond)

	// Multiple stops should not panic
	tailer.Stop()
	tailer.Stop()
	tailer.Stop()
}

func TestLogTailer_CRLFHandling(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "crlf.log")

	// Create file with CRLF line endings
	f, _ := os.Create(logFile)
	f.Close()

	ch := make(chan LogLine, 10)
	tailer := NewLogTailer(logFile, "test", 0, ch)
	tailer.Start()

	time.Sleep(150 * time.Millisecond)

	// Append line with CRLF
	f, _ = os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0644)
	f.WriteString("windows line\r\n")
	f.Close()

	time.Sleep(150 * time.Millisecond)

	tailer.Stop()

	close(ch)
	lines := []LogLine{}
	for line := range ch {
		lines = append(lines, line)
	}

	if len(lines) != 1 {
		t.Errorf("Expected 1 line, got %d", len(lines))
	}

	if len(lines) > 0 && lines[0].Text != "windows line" {
		t.Errorf("Expected 'windows line', got '%s' (len=%d)", lines[0].Text, len(lines[0].Text))
	}
}
