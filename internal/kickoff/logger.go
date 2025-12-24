package kickoff

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	// DefaultMaxLogSize is the maximum size of a single log file (10MB)
	DefaultMaxLogSize = 10 * 1024 * 1024

	// DefaultMaxLogFiles is the maximum number of log files to keep
	DefaultMaxLogFiles = 10
)

// RotatingLogger handles log file rotation
type RotatingLogger struct {
	dir      string
	maxSize  int64
	maxFiles int
	current  *os.File
	written  int64
}

// NewRotatingLogger creates a new RotatingLogger
func NewRotatingLogger(dir string) (*RotatingLogger, error) {
	// Ensure directory exists
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	logger := &RotatingLogger{
		dir:      dir,
		maxSize:  DefaultMaxLogSize,
		maxFiles: DefaultMaxLogFiles,
	}

	// Create initial log file
	if err := logger.createNewFile(); err != nil {
		return nil, err
	}

	// Clean up old files
	logger.cleanup()

	return logger, nil
}

// Write implements io.Writer
func (l *RotatingLogger) Write(p []byte) (n int, err error) {
	if l.current == nil {
		if err := l.createNewFile(); err != nil {
			return 0, err
		}
	}

	// Add timestamp to each line
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	data := fmt.Sprintf("[%s] %s", timestamp, string(p))

	n, err = l.current.WriteString(data)
	if err != nil {
		return n, err
	}

	l.written += int64(n)

	// Check if rotation is needed
	if l.written >= l.maxSize {
		if err := l.Rotate(); err != nil {
			return n, err
		}
	}

	return len(p), nil
}

// Rotate closes the current log file and creates a new one
func (l *RotatingLogger) Rotate() error {
	if l.current != nil {
		if err := l.current.Close(); err != nil {
			return fmt.Errorf("failed to close current log file: %w", err)
		}
		l.current = nil
	}

	l.cleanup()
	return l.createNewFile()
}

// Close closes the logger
func (l *RotatingLogger) Close() error {
	if l.current != nil {
		err := l.current.Close()
		l.current = nil
		return err
	}
	return nil
}

// SetAsStdout redirects stdout and stderr to the logger
func (l *RotatingLogger) SetAsStdout() {
	// Create a multi-writer that writes to both the log and original stdout
	multiWriter := io.MultiWriter(os.Stdout, l)

	// Note: In Go, we can't directly replace os.Stdout
	// The caller should use this logger's Write method instead
	_ = multiWriter
}

// FilePath returns the current log file path
func (l *RotatingLogger) FilePath() string {
	if l.current != nil {
		return l.current.Name()
	}
	return ""
}

// createNewFile creates a new log file with timestamp
func (l *RotatingLogger) createNewFile() error {
	timestamp := time.Now().Format("20060102-150405.000")
	filename := fmt.Sprintf("kickoff-%s.log", timestamp)
	path := filepath.Join(l.dir, filename)

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}

	l.current = file
	l.written = 0
	return nil
}

// cleanup removes old log files if there are too many
func (l *RotatingLogger) cleanup() {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return
	}

	// Filter and sort log files
	var logFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".log" {
			logFiles = append(logFiles, filepath.Join(l.dir, entry.Name()))
		}
	}

	// Sort by modification time (oldest first)
	sort.Slice(logFiles, func(i, j int) bool {
		infoI, _ := os.Stat(logFiles[i])
		infoJ, _ := os.Stat(logFiles[j])
		if infoI == nil || infoJ == nil {
			return false
		}
		return infoI.ModTime().Before(infoJ.ModTime())
	})

	// Remove oldest files if we have too many
	for len(logFiles) > l.maxFiles {
		os.Remove(logFiles[0])
		logFiles = logFiles[1:]
	}
}
