package kickoff

import (
	"testing"
	"time"
)

func TestIssueMonitor_ParseAWKComment(t *testing.T) {
	monitor := NewIssueMonitor(42, nil)

	tests := []struct {
		name         string
		body         string
		expectedType string
		expectedPR   string
	}{
		{
			name:         "worker_start comment",
			body:         "Starting work...\n<!-- AWK:session:abc123:worker_start -->",
			expectedType: "worker_start",
			expectedPR:   "",
		},
		{
			name:         "worker_complete with PR",
			body:         "Done!\n<!-- AWK:session:abc123:worker_complete:https://github.com/owner/repo/pull/123 -->",
			expectedType: "worker_complete",
			expectedPR:   "https://github.com/owner/repo/pull/123",
		},
		{
			name:         "worker_progress comment",
			body:         "<!-- AWK:session:xyz789:worker_progress -->",
			expectedType: "worker_progress",
			expectedPR:   "",
		},
		{
			name:         "no AWK marker",
			body:         "Just a regular comment",
			expectedType: "",
			expectedPR:   "",
		},
		{
			name:         "empty body",
			body:         "",
			expectedType: "",
			expectedPR:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commentType, prURL := monitor.parseAWKComment(tt.body)

			if commentType != tt.expectedType {
				t.Errorf("Expected type %q, got %q", tt.expectedType, commentType)
			}

			if prURL != tt.expectedPR {
				t.Errorf("Expected PR %q, got %q", tt.expectedPR, prURL)
			}
		})
	}
}

func TestIssueMonitor_Timeout(t *testing.T) {
	monitor := NewIssueMonitor(42, nil)

	// Initially should not be timed out
	if monitor.IsTimedOut() {
		t.Error("Monitor should not be timed out initially")
	}

	// Manually set lastActivity to past
	monitor.lastActivity = time.Now().Add(-31 * time.Minute)

	// Check timeout should return true
	if !monitor.checkTimeout() {
		t.Error("Monitor should detect timeout")
	}

	// IsTimedOut should now return true
	if !monitor.IsTimedOut() {
		t.Error("Monitor should be timed out")
	}

	// Calling checkTimeout again should return false (already timed out)
	if monitor.checkTimeout() {
		t.Error("checkTimeout should return false when already timed out")
	}
}

func TestIssueMonitor_StopReason(t *testing.T) {
	monitor := NewIssueMonitor(42, nil)

	// Start the monitor
	monitor.Start()

	// Initially no stop reason
	if reason := monitor.StopReason(); reason != "" {
		t.Errorf("Expected empty stop reason, got %q", reason)
	}

	// Stop with reason
	monitor.Stop("test_stop")

	// Check stop reason
	if reason := monitor.StopReason(); reason != "test_stop" {
		t.Errorf("Expected stop reason 'test_stop', got %q", reason)
	}

	// Stopping again should not change reason
	monitor.Stop("another_reason")
	if reason := monitor.StopReason(); reason != "test_stop" {
		t.Errorf("Stop reason should not change, got %q", reason)
	}
}

func TestIssueMonitor_Duration(t *testing.T) {
	monitor := NewIssueMonitor(42, nil)

	// Duration should be very small initially
	duration := monitor.Duration()
	if duration > time.Second {
		t.Errorf("Initial duration should be small, got %v", duration)
	}

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	// Duration should have increased
	newDuration := monitor.Duration()
	if newDuration <= duration {
		t.Error("Duration should increase over time")
	}
}

func TestIssueMonitor_IssueID(t *testing.T) {
	monitor := NewIssueMonitor(42, nil)

	if id := monitor.IssueID(); id != 42 {
		t.Errorf("Expected issue ID 42, got %d", id)
	}
}

func TestIssueMonitor_CommentCallback(t *testing.T) {
	monitor := NewIssueMonitor(42, nil)

	var receivedType, receivedPR string
	monitor.SetCommentCallback(func(commentType, prURL string) {
		receivedType = commentType
		receivedPR = prURL
	})

	// Simulate receiving a comment
	if monitor.onComment != nil {
		monitor.onComment("worker_start", "")
	}

	if receivedType != "worker_start" {
		t.Errorf("Expected type 'worker_start', got %q", receivedType)
	}

	if receivedPR != "" {
		t.Errorf("Expected empty PR, got %q", receivedPR)
	}
}

func TestNewIssueMonitor(t *testing.T) {
	spinner := NewSpinner(42, nil)
	monitor := NewIssueMonitor(42, spinner)

	if monitor.issueID != 42 {
		t.Errorf("Expected issue ID 42, got %d", monitor.issueID)
	}

	if monitor.spinner != spinner {
		t.Error("Spinner should be set")
	}

	if monitor.backoff != PollInterval {
		t.Errorf("Expected backoff %v, got %v", PollInterval, monitor.backoff)
	}

	if monitor.stopChan == nil {
		t.Error("stopChan should be initialized")
	}

	if monitor.doneChan == nil {
		t.Error("doneChan should be initialized")
	}
}
