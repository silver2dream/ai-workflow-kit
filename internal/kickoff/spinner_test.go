package kickoff

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestSpinner_NonTTY(t *testing.T) {
	// Use a buffer (non-TTY)
	buf := &bytes.Buffer{}
	spinner := NewSpinner(42, buf)

	// Should detect non-TTY
	if spinner.IsTTY() {
		t.Error("Buffer should not be detected as TTY")
	}

	// Start spinner
	spinner.Start()

	// Wait for the static message to be printed
	time.Sleep(50 * time.Millisecond)

	// Stop spinner
	spinner.Stop("")

	// Check output - should have static message
	output := buf.String()
	if !strings.Contains(output, "[#42] Worker running...") {
		t.Errorf("Expected static message, got %q", output)
	}

	// Should only print once (no animation)
	count := strings.Count(output, "[#42]")
	if count != 1 {
		t.Errorf("Expected message to appear once, appeared %d times", count)
	}
}

func TestSpinner_Duration(t *testing.T) {
	buf := &bytes.Buffer{}
	spinner := NewSpinner(42, buf)

	// Duration should be very small initially
	duration := spinner.Duration()
	if duration > time.Second {
		t.Errorf("Initial duration should be small, got %v", duration)
	}

	// Start spinner
	spinner.Start()
	time.Sleep(20 * time.Millisecond)
	spinner.Stop("")

	// Duration should have increased
	newDuration := spinner.Duration()
	if newDuration <= duration {
		t.Error("Duration should increase over time")
	}
}

func TestSpinner_PauseResume(t *testing.T) {
	buf := &bytes.Buffer{}
	spinner := NewSpinner(42, buf)

	spinner.Start()

	// Pause
	spinner.Pause()
	if !spinner.paused {
		t.Error("Spinner should be paused")
	}

	// Resume
	spinner.Resume()
	if spinner.paused {
		t.Error("Spinner should not be paused after resume")
	}

	spinner.Stop("")
}

func TestSpinner_DoubleStart(t *testing.T) {
	buf := &bytes.Buffer{}
	spinner := NewSpinner(42, buf)

	// Start twice should not panic
	spinner.Start()
	spinner.Start() // Should be no-op

	spinner.Stop("")
}

func TestSpinner_DoubleStop(t *testing.T) {
	buf := &bytes.Buffer{}
	spinner := NewSpinner(42, buf)

	spinner.Start()

	// Stop twice should not panic
	spinner.Stop("first")
	spinner.Stop("second") // Should be no-op
}

func TestSpinner_StopWithMessage(t *testing.T) {
	buf := &bytes.Buffer{}
	spinner := NewSpinner(42, buf)

	spinner.Start()
	time.Sleep(20 * time.Millisecond)
	spinner.Stop("✓ [#42] Worker 完成 (1s)")

	output := buf.String()
	if !strings.Contains(output, "✓ [#42] Worker 完成") {
		t.Errorf("Expected final message in output, got %q", output)
	}
}

func TestSpinner_ClearLine(t *testing.T) {
	buf := &bytes.Buffer{}
	spinner := NewSpinner(42, buf)

	// For non-TTY, ClearLine should do nothing
	spinner.ClearLine()

	if buf.Len() != 0 {
		t.Error("ClearLine should not write to non-TTY")
	}
}

func TestNewSpinner(t *testing.T) {
	buf := &bytes.Buffer{}
	spinner := NewSpinner(42, buf)

	if spinner.issueID != 42 {
		t.Errorf("Expected issue ID 42, got %d", spinner.issueID)
	}

	if spinner.writer != buf {
		t.Error("Writer should be set")
	}

	if len(spinner.frames) == 0 {
		t.Error("Frames should be initialized")
	}

	if spinner.stopChan == nil {
		t.Error("stopChan should be initialized")
	}

	if spinner.doneChan == nil {
		t.Error("doneChan should be initialized")
	}
}

func TestSpinner_Frames(t *testing.T) {
	// Verify default frames are braille dots
	expected := []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}

	if len(defaultFrames) != len(expected) {
		t.Errorf("Expected %d frames, got %d", len(expected), len(defaultFrames))
	}

	for i, frame := range defaultFrames {
		if frame != expected[i] {
			t.Errorf("Frame %d: expected %c, got %c", i, expected[i], frame)
		}
	}
}
