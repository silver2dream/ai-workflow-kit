package kickoff

import (
	"fmt"
	"path/filepath"
	"sync"
)

// FanInManager manages multiple log tailers and the fan-in channel
type FanInManager struct {
	channel         chan LogLine
	principalTailer *LogTailer
	workerTailer    *LogTailer
	currentIssueID  int
	logDir          string
	mu              sync.Mutex
	stopped         bool
	wg              sync.WaitGroup
}

// NewFanInManager creates a new FanInManager with a buffered channel
func NewFanInManager(bufferSize int) *FanInManager {
	return &FanInManager{
		channel: make(chan LogLine, bufferSize),
	}
}

// Channel returns the read-only fan-in channel
func (f *FanInManager) Channel() <-chan LogLine {
	return f.channel
}

// SetLogDir sets the log directory for tailers
func (f *FanInManager) SetLogDir(logDir string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logDir = logDir
}

// StartPrincipalTailer starts tailing the principal log file (Req 4.1)
func (f *FanInManager) StartPrincipalTailer(logDir string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.stopped {
		return
	}

	f.logDir = logDir
	path := filepath.Join(logDir, "principal.log")
	f.principalTailer = NewLogTailer(path, "principal", 0, f.channel)
	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		f.principalTailer.tailLoop()
	}()
}

// StartWorkerTailer starts tailing a worker log file (Req 4.2)
func (f *FanInManager) StartWorkerTailer(logDir string, issueID int) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.stopped {
		return
	}

	// Stop previous worker tailer if exists (Req 4.4)
	if f.workerTailer != nil {
		f.workerTailer.Stop()
		f.workerTailer = nil
	}

	f.currentIssueID = issueID
	path := filepath.Join(logDir, fmt.Sprintf("issue-%d.worker.log", issueID))
	f.workerTailer = NewLogTailer(path, "worker", issueID, f.channel)
	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		f.workerTailer.tailLoop()
	}()
}

// StopWorkerTailer stops the current worker tailer (Req 4.3)
func (f *FanInManager) StopWorkerTailer() {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.workerTailer != nil {
		f.workerTailer.Stop()
		f.workerTailer = nil
	}
	f.currentIssueID = 0
}

// CurrentIssueID returns the currently monitored issue ID
func (f *FanInManager) CurrentIssueID() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.currentIssueID
}

// SendClaudeLine sends a line from Claude stream to the channel
// Uses non-blocking send to prevent deadlock when channel buffer is full
func (f *FanInManager) SendClaudeLine(text string) {
	f.mu.Lock()
	stopped := f.stopped
	f.mu.Unlock()

	if stopped {
		return
	}

	// Non-blocking send to prevent deadlock when buffer is full during shutdown
	select {
	case f.channel <- LogLine{
		Source:  "claude",
		IssueID: 0,
		Text:    text,
	}:
	default:
		// Drop message if channel is full - prevents blocking during shutdown
	}
}

// Stop stops all tailers and closes the channel (Req 7.2, 7.3)
func (f *FanInManager) Stop() {
	f.mu.Lock()
	if f.stopped {
		f.mu.Unlock()
		return
	}
	f.stopped = true

	// Stop all tailers (Req 4.5)
	if f.principalTailer != nil {
		f.principalTailer.Stop()
	}
	if f.workerTailer != nil {
		f.workerTailer.Stop()
	}
	f.mu.Unlock()

	// Wait for all goroutines to finish (Req 7.4)
	f.wg.Wait()

	// Close channel after all producers finish (Req 7.2)
	close(f.channel)
}
