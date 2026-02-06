package kickoff

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/ghutil"
)

const (
	// PollInterval is the time between GitHub API polls
	PollInterval = 5 * time.Second

	// TimeoutDuration is the time without activity before showing timeout warning
	TimeoutDuration = 30 * time.Minute

	// AWKCommentPrefix is the marker for AWK comments
	AWKCommentPrefix = "<!-- AWK:session:"
)

// IssueMonitor monitors GitHub Issue comments for Worker progress
type IssueMonitor struct {
	issueID          int
	processedCount   int               // Track number of processed comments (G4 fix)
	seenCommentIDs   map[string]bool   // Track seen comment IDs to handle reordering
	lastActivity     time.Time
	startTime        time.Time
	spinner          *Spinner
	stopChan         chan struct{}
	doneChan         chan struct{}
	stopReason       string
	timedOut         bool
	mu               sync.Mutex
	backoff          time.Duration
	onComment        func(commentType string, prURL string)
}

// Comment represents a GitHub issue comment
type Comment struct {
	ID        string `json:"id"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
}

// IssueResponse represents the GitHub API response for issue view
type IssueResponse struct {
	Comments []Comment `json:"comments"`
	State    string    `json:"state"`
}

// NewIssueMonitor creates a new IssueMonitor for the given issue
func NewIssueMonitor(issueID int, spinner *Spinner) *IssueMonitor {
	return &IssueMonitor{
		issueID:        issueID,
		seenCommentIDs: make(map[string]bool),
		startTime:      time.Now(),
		lastActivity:   time.Now(),
		spinner:        spinner,
		stopChan:       make(chan struct{}),
		doneChan:       make(chan struct{}),
		backoff:        PollInterval,
	}
}

// SetCommentCallback sets the callback for when a new AWK comment is detected
func (m *IssueMonitor) SetCommentCallback(cb func(commentType string, prURL string)) {
	m.onComment = cb
}

// Start begins polling for issue comments
func (m *IssueMonitor) Start() {
	go m.pollLoop()
}

// Stop stops the monitor with the given reason.
// It waits up to 10 seconds for the poll loop to exit gracefully.
func (m *IssueMonitor) Stop(reason string) {
	m.mu.Lock()
	if m.stopReason != "" {
		m.mu.Unlock()
		return // Already stopped
	}
	m.stopReason = reason
	m.mu.Unlock()

	close(m.stopChan)
	// Wait for poll loop to exit with timeout to prevent indefinite blocking
	select {
	case <-m.doneChan:
		// Poll loop exited gracefully
	case <-time.After(10 * time.Second):
		// Timeout waiting for poll loop - it may be stuck in a GitHub API call
		// The poll loop will eventually exit when its current operation completes
	}
}

// StopReason returns the reason the monitor stopped
func (m *IssueMonitor) StopReason() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopReason
}

// pollLoop is the main polling loop
func (m *IssueMonitor) pollLoop() {
	defer close(m.doneChan)

	ticker := time.NewTicker(m.backoff)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopChan:
			return
		case <-ticker.C:
			if err := m.poll(); err != nil {
				// Exponential backoff on error
				m.backoff = min(m.backoff*2, 60*time.Second)
				ticker.Reset(m.backoff)
			} else {
				// Reset backoff on success
				m.backoff = PollInterval
				ticker.Reset(m.backoff)
			}

			// Check for timeout
			m.checkTimeout()
		}
	}
}

// poll fetches and processes new comments
func (m *IssueMonitor) poll() error {
	resp, err := m.fetchComments()
	if err != nil {
		return err
	}

	// Check if issue is closed
	if resp.State == "closed" {
		m.Stop("issue_closed")
		return nil
	}

	// Process new comments (G4 fix: use map instead of string comparison)
	for _, comment := range resp.Comments {
		// Skip already-processed comments
		if m.seenCommentIDs[comment.ID] {
			continue
		}

		// Mark as seen
		m.seenCommentIDs[comment.ID] = true
		m.processedCount++

		// Check for AWK marker
		if strings.Contains(comment.Body, AWKCommentPrefix) {
			commentType, prURL := m.parseAWKComment(comment.Body)
			m.lastActivity = time.Now()

			// Check for recovery after timeout
			if m.timedOut {
				m.timedOut = false
				// Recovery notification will be handled by caller
			}

			if m.onComment != nil {
				m.onComment(commentType, prURL)
			}

			// Check for worker_complete
			if commentType == "worker_complete" {
				m.Stop("worker_complete")
				return nil
			}
		}
	}

	return nil
}

// fetchComments calls gh issue view to get comments
func (m *IssueMonitor) fetchComments() (*IssueResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	output, err := ghutil.RunWithRetry(ctx, ghutil.DefaultRetryConfig(), "gh", "issue", "view",
		fmt.Sprintf("%d", m.issueID),
		"--json", "comments,state")
	if err != nil {
		return nil, err
	}

	var resp IssueResponse
	if err := json.Unmarshal(output, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// parseAWKComment extracts comment type and PR URL from AWK comment
func (m *IssueMonitor) parseAWKComment(body string) (commentType string, prURL string) {
	// Pattern: <!-- AWK:session:xxx:comment_type:... -->
	// or: <!-- AWK:session:xxx:comment_type:pr_url:... -->

	awkPattern := regexp.MustCompile(`<!-- AWK:session:[^:]+:([^:>]+)(?::([^>]+))? -->`)
	matches := awkPattern.FindStringSubmatch(body)

	if len(matches) > 1 {
		commentType = matches[1]
	}
	if len(matches) > 2 {
		prURL = matches[2]
	}

	return
}

// checkTimeout checks if the monitor has timed out
func (m *IssueMonitor) checkTimeout() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if time.Since(m.lastActivity) > TimeoutDuration && !m.timedOut {
		m.timedOut = true
		return true
	}
	return false
}

// IsTimedOut returns whether the monitor has timed out
func (m *IssueMonitor) IsTimedOut() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.timedOut
}

// Duration returns how long the monitor has been running
func (m *IssueMonitor) Duration() time.Duration {
	return time.Since(m.startTime)
}

// IssueID returns the issue ID being monitored
func (m *IssueMonitor) IssueID() int {
	return m.issueID
}
