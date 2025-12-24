package kickoff

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
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
	issueID       int
	lastCommentID string
	lastActivity  time.Time
	startTime     time.Time
	spinner       *Spinner
	stopChan      chan struct{}
	doneChan      chan struct{}
	stopReason    string
	timedOut      bool
	mu            sync.Mutex
	backoff       time.Duration
	onComment     func(commentType string, prURL string)
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
		issueID:      issueID,
		startTime:    time.Now(),
		lastActivity: time.Now(),
		spinner:      spinner,
		stopChan:     make(chan struct{}),
		doneChan:     make(chan struct{}),
		backoff:      PollInterval,
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

// Stop stops the monitor with the given reason
func (m *IssueMonitor) Stop(reason string) {
	m.mu.Lock()
	if m.stopReason != "" {
		m.mu.Unlock()
		return // Already stopped
	}
	m.stopReason = reason
	m.mu.Unlock()

	close(m.stopChan)
	<-m.doneChan
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

	// Process new comments
	for _, comment := range resp.Comments {
		if comment.ID <= m.lastCommentID {
			continue
		}

		m.lastCommentID = comment.ID

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
	cmd := exec.Command("gh", "issue", "view",
		fmt.Sprintf("%d", m.issueID),
		"--json", "comments,state")

	output, err := cmd.Output()
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
