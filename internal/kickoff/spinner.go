package kickoff

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"golang.org/x/term"
)

const (
	// SpinnerInterval is the time between spinner frame updates
	SpinnerInterval = 100 * time.Millisecond
)

// Note: colorGreen and colorReset are defined in output.go

// Spinner displays an animated spinner during Worker execution
type Spinner struct {
	issueID    int
	startTime  time.Time
	frames     []rune
	frameIndex int
	isTTY      bool
	mu         sync.Mutex
	active     bool
	paused     bool
	stopChan   chan struct{}
	doneChan   chan struct{}
	writer     io.Writer
	printed    bool // For non-TTY mode: only print once
}

// Default spinner frames (braille dots)
var defaultFrames = []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}

// NewSpinner creates a new Spinner for the given issue
func NewSpinner(issueID int, writer io.Writer) *Spinner {
	if writer == nil {
		writer = os.Stdout
	}

	// Detect if stdout is a TTY
	isTTY := false
	if f, ok := writer.(*os.File); ok {
		isTTY = term.IsTerminal(int(f.Fd()))
	}

	return &Spinner{
		issueID:   issueID,
		startTime: time.Now(),
		frames:    defaultFrames,
		isTTY:     isTTY,
		stopChan:  make(chan struct{}),
		doneChan:  make(chan struct{}),
		writer:    writer,
	}
}

// Start begins the spinner animation
func (s *Spinner) Start() {
	s.mu.Lock()
	if s.active {
		s.mu.Unlock()
		return
	}
	s.active = true
	s.startTime = time.Now()
	s.mu.Unlock()

	go s.loop()
}

// Stop stops the spinner and displays the final message
func (s *Spinner) Stop(finalMessage string) {
	s.mu.Lock()
	if !s.active {
		s.mu.Unlock()
		return
	}
	s.active = false
	s.mu.Unlock()

	close(s.stopChan)
	<-s.doneChan

	// Clear line and print final message
	if s.isTTY {
		s.ClearLine()
	}
	if finalMessage != "" {
		fmt.Fprintln(s.writer, finalMessage)
	}
}

// Pause temporarily stops the spinner animation
func (s *Spinner) Pause() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paused = true
}

// Resume continues the spinner animation
func (s *Spinner) Resume() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paused = false
}

// ClearLine clears the current line (for TTY mode)
func (s *Spinner) ClearLine() {
	if s.isTTY {
		fmt.Fprint(s.writer, "\r\033[K")
	}
}

// IsTTY returns whether the output is a TTY
func (s *Spinner) IsTTY() bool {
	return s.isTTY
}

// loop is the main animation loop
func (s *Spinner) loop() {
	defer close(s.doneChan)

	// Non-TTY mode: print once and return
	if !s.isTTY {
		s.mu.Lock()
		if !s.printed {
			fmt.Fprintf(s.writer, "[#%d] Worker running...\n", s.issueID)
			s.printed = true
		}
		s.mu.Unlock()

		// Wait for stop signal
		<-s.stopChan
		return
	}

	// TTY mode: animate
	ticker := time.NewTicker(SpinnerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.render()
		}
	}
}

// render draws the current spinner frame
func (s *Spinner) render() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.paused || !s.active {
		return
	}

	elapsed := time.Since(s.startTime)
	minutes := int(elapsed.Minutes())
	seconds := int(elapsed.Seconds()) % 60

	frame := s.frames[s.frameIndex]
	s.frameIndex = (s.frameIndex + 1) % len(s.frames)

	// Format: ⠋ [#N] Worker running... (M:SS) with green spinner
	fmt.Fprintf(s.writer, "\r%s%c%s [#%d] Worker running... (%d:%02d)",
		colorGreen, frame, colorReset, s.issueID, minutes, seconds)
}

// Duration returns how long the spinner has been running
func (s *Spinner) Duration() time.Duration {
	return time.Since(s.startTime)
}
