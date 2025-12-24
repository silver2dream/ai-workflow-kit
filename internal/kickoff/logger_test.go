package kickoff

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRotatingLogger_Write tests basic write functionality
func TestRotatingLogger_Write(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "logger-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logger, err := NewRotatingLogger(tmpDir)
	if err != nil {
		t.Fatalf("NewRotatingLogger failed: %v", err)
	}
	defer logger.Close()

	// Write some data
	testData := "test log message"
	n, err := logger.Write([]byte(testData))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if n != len(testData) {
		t.Errorf("Write returned %d, want %d", n, len(testData))
	}

	// Verify file was created
	if logger.FilePath() == "" {
		t.Error("FilePath() returned empty string")
	}
}

// TestRotatingLogger_Timestamp tests Property 16: Log completeness (timestamps)
func TestRotatingLogger_Timestamp(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "logger-timestamp-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logger, err := NewRotatingLogger(tmpDir)
	if err != nil {
		t.Fatalf("NewRotatingLogger failed: %v", err)
	}

	// Get file path before closing
	filePath := logger.FilePath()

	// Write test message
	logger.Write([]byte("test message\n"))
	logger.Close()

	// Read log file
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	// Property 16: Log should contain timestamp
	// Format: [2006-01-02 15:04:05]
	if !strings.Contains(string(content), "[") || !strings.Contains(string(content), "]") {
		t.Errorf("Log should contain timestamp brackets, got: %s", string(content))
	}

	// Check timestamp format (YYYY-MM-DD)
	year := time.Now().Format("2006")
	if !strings.Contains(string(content), year) {
		t.Errorf("Log should contain current year %s, got: %s", year, string(content))
	}
}

// TestRotatingLogger_Rotation tests Property 17: Log rotation (10MB, 10 files)
func TestRotatingLogger_Rotation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "logger-rotation-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logger, err := NewRotatingLogger(tmpDir)
	if err != nil {
		t.Fatalf("NewRotatingLogger failed: %v", err)
	}

	// Verify default settings match Property 17
	if logger.maxSize != DefaultMaxLogSize {
		t.Errorf("maxSize = %d, want %d (10MB)", logger.maxSize, DefaultMaxLogSize)
	}

	if logger.maxFiles != DefaultMaxLogFiles {
		t.Errorf("maxFiles = %d, want %d", logger.maxFiles, DefaultMaxLogFiles)
	}

	// Property 17: 10MB = 10 * 1024 * 1024
	expectedSize := int64(10 * 1024 * 1024)
	if DefaultMaxLogSize != expectedSize {
		t.Errorf("DefaultMaxLogSize = %d, want %d (10MB)", DefaultMaxLogSize, expectedSize)
	}

	logger.Close()
}

// TestRotatingLogger_RotationTrigger tests rotation when size exceeded
func TestRotatingLogger_RotationTrigger(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "logger-trigger-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logger, err := NewRotatingLogger(tmpDir)
	if err != nil {
		t.Fatalf("NewRotatingLogger failed: %v", err)
	}

	// Set small max size for testing
	logger.maxSize = 50

	// Write enough data to trigger rotation
	for i := 0; i < 10; i++ {
		logger.Write([]byte("this is a test log message\n"))
	}

	logger.Close()

	// Check that rotation occurred (new file created)
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read dir: %v", err)
	}

	logCount := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".log") {
			logCount++
		}
	}

	if logCount < 2 {
		t.Errorf("Expected rotation to create multiple files, got %d", logCount)
	}
}

// TestRotatingLogger_Cleanup tests old file cleanup
func TestRotatingLogger_Cleanup(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "logger-cleanup-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create more than maxFiles log files
	for i := 0; i < 15; i++ {
		filename := filepath.Join(tmpDir, "kickoff-old-"+string(rune('a'+i))+".log")
		os.WriteFile(filename, []byte("old log"), 0644)
		time.Sleep(10 * time.Millisecond) // Different mod times
	}

	// Create logger (should trigger cleanup)
	logger, err := NewRotatingLogger(tmpDir)
	if err != nil {
		t.Fatalf("NewRotatingLogger failed: %v", err)
	}
	logger.Close()

	// Count remaining log files
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read dir: %v", err)
	}

	logCount := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".log") {
			logCount++
		}
	}

	// Property 17: Should keep at most 10 files
	if logCount > DefaultMaxLogFiles {
		t.Errorf("Expected at most %d log files, got %d", DefaultMaxLogFiles, logCount)
	}
}

// TestRotatingLogger_DirectoryCreation tests auto directory creation
func TestRotatingLogger_DirectoryCreation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "logger-mkdir-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Use nested directory that doesn't exist
	logDir := filepath.Join(tmpDir, "nested", "log", "dir")

	logger, err := NewRotatingLogger(logDir)
	if err != nil {
		t.Fatalf("NewRotatingLogger failed: %v", err)
	}
	defer logger.Close()

	// Verify directory was created
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		t.Error("Log directory was not created")
	}
}

// TestRotatingLogger_Close tests proper cleanup
func TestRotatingLogger_Close(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "logger-close-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logger, err := NewRotatingLogger(tmpDir)
	if err != nil {
		t.Fatalf("NewRotatingLogger failed: %v", err)
	}

	logger.Write([]byte("test"))

	// Close should not error
	if err := logger.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Double close should not error
	if err := logger.Close(); err != nil {
		t.Errorf("Double close failed: %v", err)
	}
}

// TestRotatingLogger_Rotate tests manual rotation
func TestRotatingLogger_Rotate(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "logger-rotate-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logger, err := NewRotatingLogger(tmpDir)
	if err != nil {
		t.Fatalf("NewRotatingLogger failed: %v", err)
	}
	defer logger.Close()

	firstFile := logger.FilePath()
	logger.Write([]byte("first file content"))

	// Wait to ensure different timestamp
	time.Sleep(1100 * time.Millisecond)

	// Manual rotation
	if err := logger.Rotate(); err != nil {
		t.Fatalf("Rotate failed: %v", err)
	}

	secondFile := logger.FilePath()

	// Files should be different
	if firstFile == secondFile {
		t.Error("Rotation should create new file")
	}

	// Both files should exist
	if _, err := os.Stat(firstFile); os.IsNotExist(err) {
		t.Error("First file should still exist")
	}
	if _, err := os.Stat(secondFile); os.IsNotExist(err) {
		t.Error("Second file should exist")
	}
}

// TestNewRotatingLogger tests logger creation
func TestNewRotatingLogger(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "logger-new-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logger, err := NewRotatingLogger(tmpDir)
	if err != nil {
		t.Fatalf("NewRotatingLogger failed: %v", err)
	}
	defer logger.Close()

	if logger.dir != tmpDir {
		t.Errorf("dir = %q, want %q", logger.dir, tmpDir)
	}

	if logger.current == nil {
		t.Error("current file should not be nil")
	}
}

// TestRotatingLogger_FilePath tests file path retrieval
func TestRotatingLogger_FilePath(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "logger-path-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logger, err := NewRotatingLogger(tmpDir)
	if err != nil {
		t.Fatalf("NewRotatingLogger failed: %v", err)
	}
	defer logger.Close()

	path := logger.FilePath()

	// Should be in the log directory
	if !strings.HasPrefix(path, tmpDir) {
		t.Errorf("FilePath %q should be in %q", path, tmpDir)
	}

	// Should have .log extension
	if !strings.HasSuffix(path, ".log") {
		t.Errorf("FilePath %q should have .log extension", path)
	}

	// Should contain "kickoff"
	if !strings.Contains(path, "kickoff") {
		t.Errorf("FilePath %q should contain 'kickoff'", path)
	}
}
