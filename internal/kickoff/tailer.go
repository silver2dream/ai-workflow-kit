package kickoff

import (
	"bufio"
	"io"
	"os"
	"sync"
	"time"
)

// LogLine represents a line from a log file with metadata
type LogLine struct {
	Source  string // "claude", "principal", "worker"
	IssueID int    // 0 for principal/claude, N for worker
	Text    string
}

// LogTailer monitors a log file and sends new lines to a channel
type LogTailer struct {
	path         string
	source       string
	issueID      int
	output       chan<- LogLine
	stopChan     chan struct{}
	doneChan     chan struct{}
	mu           sync.Mutex
	stopped      bool
	pollInterval time.Duration
}

// NewLogTailer creates a new LogTailer
func NewLogTailer(path, source string, issueID int, output chan<- LogLine) *LogTailer {
	return &LogTailer{
		path:         path,
		source:       source,
		issueID:      issueID,
		output:       output,
		stopChan:     make(chan struct{}),
		doneChan:     make(chan struct{}),
		pollInterval: 100 * time.Millisecond,
	}
}

// Start begins tailing the log file
// It waits for the file to be created if it doesn't exist
func (t *LogTailer) Start() {
	go t.tailLoop()
}

// Stop gracefully stops the tailer
func (t *LogTailer) Stop() {
	t.mu.Lock()
	if t.stopped {
		t.mu.Unlock()
		return
	}
	t.stopped = true
	t.mu.Unlock()

	close(t.stopChan)

	// Wait with timeout (1 second per requirement)
	select {
	case <-t.doneChan:
	case <-time.After(1 * time.Second):
		// Force timeout per Req 7.5
	}
}

// tailLoop is the main tailing loop
func (t *LogTailer) tailLoop() {
	defer close(t.doneChan)

	// Wait for file to exist (Req 2.4)
	var file *os.File
	var err error
	for {
		select {
		case <-t.stopChan:
			return
		default:
		}

		file, err = os.Open(t.path)
		if err == nil {
			break
		}

		// Wait and retry
		select {
		case <-t.stopChan:
			return
		case <-time.After(t.pollInterval):
		}
	}
	defer file.Close()

	// Seek to end of file (Req 2.3)
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		return
	}

	// Get initial file size for truncation detection
	var lastSize int64
	if info, err := file.Stat(); err == nil {
		lastSize = info.Size()
	}

	reader := bufio.NewReader(file)
	var lineBuffer string

	for {
		select {
		case <-t.stopChan:
			return
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				return
			}

			// Partial line without newline - buffer it
			if len(line) > 0 {
				lineBuffer += line
			}

			// Check for truncation (Req 2.6)
			if info, err := file.Stat(); err == nil {
				currentSize := info.Size()
				if currentSize < lastSize {
					// File was truncated, seek to beginning
					file.Seek(0, io.SeekStart)
					reader.Reset(file)
					lastSize = 0
					lineBuffer = ""
					continue
				}
				lastSize = currentSize
			}

			// Wait for more data
			select {
			case <-t.stopChan:
				return
			case <-time.After(t.pollInterval):
			}
			continue
		}

		// Complete line received
		fullLine := lineBuffer + line
		lineBuffer = ""

		// Trim trailing newline
		if len(fullLine) > 0 && fullLine[len(fullLine)-1] == '\n' {
			fullLine = fullLine[:len(fullLine)-1]
		}
		// Also trim \r for Windows CRLF
		if len(fullLine) > 0 && fullLine[len(fullLine)-1] == '\r' {
			fullLine = fullLine[:len(fullLine)-1]
		}

		// Update last known size
		if info, err := file.Stat(); err == nil {
			lastSize = info.Size()
		}

		// Send line to channel (Req 2.2)
		select {
		case t.output <- LogLine{
			Source:  t.source,
			IssueID: t.issueID,
			Text:    fullLine,
		}:
		case <-t.stopChan:
			return
		}
	}
}
