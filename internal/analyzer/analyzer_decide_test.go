package analyzer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// MockGitHubClient is a mock implementation of GitHubClientInterface for testing
type MockGitHubClient struct {
	// Mock data for responses
	IssuesByLabel       map[string][]Issue
	IssuesByLabelError  map[string]error // Per-label errors for ListIssuesByLabel
	PendingIssues       []Issue
	PendingIssuesError  error // Specific error for ListPendingIssues
	OpenIssueCount      int
	PRMerged            map[int]bool
	PRMergedError       map[int]error
	PRByBranch          map[string]int // branch name -> PR number
	ClosedIssues        []int
	AddedLabels         map[int][]string
	RemovedLabels       map[int][]string
	ListIssuesError     error
	CountError          error
	AddLabelError       error
	RemoveLabelError    error
	CloseIssueError     error
	IssueBodies         map[int]string // issue number -> body
	GetIssueBodyError   error
	UpdateIssueBodyError error
	UpdatedBodies       map[int]string // tracks UpdateIssueBody calls
}

func NewMockGitHubClient() *MockGitHubClient {
	return &MockGitHubClient{
		IssuesByLabel:      make(map[string][]Issue),
		IssuesByLabelError: make(map[string]error),
		PRMerged:           make(map[int]bool),
		PRMergedError:      make(map[int]error),
		PRByBranch:         make(map[string]int),
		AddedLabels:        make(map[int][]string),
		RemovedLabels:      make(map[int][]string),
		IssueBodies:        make(map[int]string),
		UpdatedBodies:      make(map[int]string),
	}
}

func (m *MockGitHubClient) ListIssuesByLabel(ctx context.Context, label string) ([]Issue, error) {
	// Check per-label errors first
	if err, ok := m.IssuesByLabelError[label]; ok && err != nil {
		return nil, err
	}
	if m.ListIssuesError != nil {
		return nil, m.ListIssuesError
	}
	return m.IssuesByLabel[label], nil
}

func (m *MockGitHubClient) ListPendingIssues(ctx context.Context, labels LabelsConfig) ([]Issue, error) {
	if m.PendingIssuesError != nil {
		return nil, m.PendingIssuesError
	}
	if m.ListIssuesError != nil {
		return nil, m.ListIssuesError
	}
	return m.PendingIssues, nil
}

func (m *MockGitHubClient) CountOpenIssues(ctx context.Context, taskLabel string) (int, error) {
	if m.CountError != nil {
		return 0, m.CountError
	}
	return m.OpenIssueCount, nil
}

func (m *MockGitHubClient) RemoveLabel(ctx context.Context, issueNumber int, label string) error {
	if m.RemoveLabelError != nil {
		return m.RemoveLabelError
	}
	m.RemovedLabels[issueNumber] = append(m.RemovedLabels[issueNumber], label)
	return nil
}

func (m *MockGitHubClient) AddLabel(ctx context.Context, issueNumber int, label string) error {
	if m.AddLabelError != nil {
		return m.AddLabelError
	}
	m.AddedLabels[issueNumber] = append(m.AddedLabels[issueNumber], label)
	return nil
}

func (m *MockGitHubClient) IsPRMerged(ctx context.Context, prNumber int) (bool, error) {
	if err, ok := m.PRMergedError[prNumber]; ok && err != nil {
		return false, err
	}
	return m.PRMerged[prNumber], nil
}

func (m *MockGitHubClient) CloseIssue(ctx context.Context, issueNumber int) error {
	if m.CloseIssueError != nil {
		return m.CloseIssueError
	}
	m.ClosedIssues = append(m.ClosedIssues, issueNumber)
	return nil
}

func (m *MockGitHubClient) FindPRByBranch(ctx context.Context, branchName string) (int, error) {
	if num, ok := m.PRByBranch[branchName]; ok {
		return num, nil
	}
	return 0, nil
}

func (m *MockGitHubClient) GetIssueBody(ctx context.Context, issueNumber int) (string, error) {
	if m.GetIssueBodyError != nil {
		return "", m.GetIssueBodyError
	}
	body, ok := m.IssueBodies[issueNumber]
	if !ok {
		return "", fmt.Errorf("issue #%d not found", issueNumber)
	}
	return body, nil
}

func (m *MockGitHubClient) UpdateIssueBody(ctx context.Context, issueNumber int, body string) error {
	if m.UpdateIssueBodyError != nil {
		return m.UpdateIssueBodyError
	}
	m.IssueBodies[issueNumber] = body
	m.UpdatedBodies[issueNumber] = body
	return nil
}

// Helper function to create test analyzer with mock client
func newTestAnalyzer(tmpDir string, config *Config, mockClient *MockGitHubClient) *Analyzer {
	a := New(tmpDir, config)
	a.GHClient = mockClient
	return a
}

// Helper function to create default test config
func defaultTestConfig() *Config {
	return &Config{
		GitHub: GitHubConfig{
			Labels: DefaultLabels(),
		},
	}
}

func TestDecide_ConfigNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()

	// Create analyzer without config - it will try to load from file
	a := newTestAnalyzer(tmpDir, nil, mockClient)

	decision, err := a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	if decision.NextAction != ActionNone {
		t.Errorf("NextAction = %q, want %q", decision.NextAction, ActionNone)
	}
	if decision.ExitReason != ReasonConfigNotFound {
		t.Errorf("ExitReason = %q, want %q", decision.ExitReason, ReasonConfigNotFound)
	}
}

func TestDecide_MaxLoopReached(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := defaultTestConfig()

	// Set loop count to max
	stateDir := filepath.Join(tmpDir, ".ai", "state")
	os.MkdirAll(stateDir, 0755)
	os.WriteFile(filepath.Join(stateDir, "loop_count"), []byte("1000"), 0644)

	a := newTestAnalyzer(tmpDir, config, mockClient)
	decision, err := a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	if decision.NextAction != ActionNone {
		t.Errorf("NextAction = %q, want %q", decision.NextAction, ActionNone)
	}
	if decision.ExitReason != ReasonMaxLoopReached {
		t.Errorf("ExitReason = %q, want %q", decision.ExitReason, ReasonMaxLoopReached)
	}
}

func TestDecide_MaxConsecutiveFailures(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := defaultTestConfig()

	// Set consecutive failures to max
	stateDir := filepath.Join(tmpDir, ".ai", "state")
	os.MkdirAll(stateDir, 0755)
	os.WriteFile(filepath.Join(stateDir, "consecutive_failures"), []byte("5"), 0644)

	a := newTestAnalyzer(tmpDir, config, mockClient)
	decision, err := a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	if decision.NextAction != ActionNone {
		t.Errorf("NextAction = %q, want %q", decision.NextAction, ActionNone)
	}
	if decision.ExitReason != ReasonMaxConsecutiveFailures {
		t.Errorf("ExitReason = %q, want %q", decision.ExitReason, ReasonMaxConsecutiveFailures)
	}
}

func TestDecide_InProgressIssue(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := defaultTestConfig()

	// Add in-progress issue
	mockClient.IssuesByLabel[config.GitHub.Labels.InProgress] = []Issue{
		{Number: 42, Body: "test issue"},
	}

	a := newTestAnalyzer(tmpDir, config, mockClient)
	decision, err := a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	if decision.NextAction != ActionCheckResult {
		t.Errorf("NextAction = %q, want %q", decision.NextAction, ActionCheckResult)
	}
	if decision.IssueNumber != 42 {
		t.Errorf("IssueNumber = %d, want 42", decision.IssueNumber)
	}
}

func TestDecide_PRReadyWithPRNumber(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := defaultTestConfig()

	// Add pr-ready issue with PR number in body
	mockClient.IssuesByLabel[config.GitHub.Labels.PRReady] = []Issue{
		{Number: 10, Body: "PR ready - see https://github.com/owner/repo/pull/100"},
	}

	a := newTestAnalyzer(tmpDir, config, mockClient)
	decision, err := a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	if decision.NextAction != ActionReviewPR {
		t.Errorf("NextAction = %q, want %q", decision.NextAction, ActionReviewPR)
	}
	if decision.IssueNumber != 10 {
		t.Errorf("IssueNumber = %d, want 10", decision.IssueNumber)
	}
	if decision.PRNumber != 100 {
		t.Errorf("PRNumber = %d, want 100", decision.PRNumber)
	}
}

func TestDecide_PRReadyFromResultFile(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := defaultTestConfig()

	// Add pr-ready issue without PR in body
	mockClient.IssuesByLabel[config.GitHub.Labels.PRReady] = []Issue{
		{Number: 15, Body: "PR ready"},
	}

	// Create result file with PR URL
	resultDir := filepath.Join(tmpDir, ".ai", "results")
	os.MkdirAll(resultDir, 0755)
	resultData, _ := json.Marshal(map[string]string{"pr_url": "https://github.com/owner/repo/pull/150"})
	os.WriteFile(filepath.Join(resultDir, "issue-15.json"), resultData, 0644)

	a := newTestAnalyzer(tmpDir, config, mockClient)
	decision, err := a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	if decision.NextAction != ActionReviewPR {
		t.Errorf("NextAction = %q, want %q", decision.NextAction, ActionReviewPR)
	}
	if decision.PRNumber != 150 {
		t.Errorf("PRNumber = %d, want 150", decision.PRNumber)
	}
}

func TestDecide_PRReadyNoPRNumber(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := defaultTestConfig()

	// Add pr-ready issue without PR number
	mockClient.IssuesByLabel[config.GitHub.Labels.PRReady] = []Issue{
		{Number: 20, Body: "PR ready but no link"},
	}

	a := newTestAnalyzer(tmpDir, config, mockClient)
	decision, err := a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	if decision.NextAction != ActionNone {
		t.Errorf("NextAction = %q, want %q", decision.NextAction, ActionNone)
	}
	if decision.ExitReason != ReasonNeedsHumanReview {
		t.Errorf("ExitReason = %q, want %q", decision.ExitReason, ReasonNeedsHumanReview)
	}

	// Check label was added
	if len(mockClient.AddedLabels[20]) == 0 || mockClient.AddedLabels[20][0] != config.GitHub.Labels.NeedsHumanReview {
		t.Errorf("Expected needs-human-review label to be added")
	}
}

func TestDecide_ReviewFailedRetry(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := defaultTestConfig()

	// Add review-failed issue
	mockClient.IssuesByLabel[config.GitHub.Labels.ReviewFailed] = []Issue{
		{Number: 25, Body: "https://github.com/owner/repo/pull/250"},
	}

	a := newTestAnalyzer(tmpDir, config, mockClient)
	decision, err := a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	if decision.NextAction != ActionReviewPR {
		t.Errorf("NextAction = %q, want %q", decision.NextAction, ActionReviewPR)
	}
	if decision.PRNumber != 250 {
		t.Errorf("PRNumber = %d, want 250", decision.PRNumber)
	}
}

func TestDecide_ReviewFailedMaxRetries(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := defaultTestConfig()

	// Add review-failed issue
	mockClient.IssuesByLabel[config.GitHub.Labels.ReviewFailed] = []Issue{
		{Number: 30, Body: "https://github.com/owner/repo/pull/300"},
	}

	// Set review attempts to max (default is 3)
	attemptDir := filepath.Join(tmpDir, ".ai", "state", "attempts")
	os.MkdirAll(attemptDir, 0755)
	os.WriteFile(filepath.Join(attemptDir, "review-pr-300"), []byte(strconv.Itoa(DefaultMaxReviewAttempts)), 0644)

	a := newTestAnalyzer(tmpDir, config, mockClient)
	decision, err := a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	if decision.NextAction != ActionNone {
		t.Errorf("NextAction = %q, want %q", decision.NextAction, ActionNone)
	}
	if decision.ExitReason != ReasonReviewMaxRetries {
		t.Errorf("ExitReason = %q, want %q", decision.ExitReason, ReasonReviewMaxRetries)
	}
}

func TestDecide_MergeConflict(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := defaultTestConfig()

	// Add merge-conflict issue
	mockClient.IssuesByLabel[config.GitHub.Labels.MergeConflict] = []Issue{
		{Number: 35, Body: "https://github.com/owner/repo/pull/350"},
	}

	a := newTestAnalyzer(tmpDir, config, mockClient)
	decision, err := a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	if decision.NextAction != ActionDispatchWorker {
		t.Errorf("NextAction = %q, want %q", decision.NextAction, ActionDispatchWorker)
	}
	if decision.MergeIssue != MergeIssueConflict {
		t.Errorf("MergeIssue = %q, want %q", decision.MergeIssue, MergeIssueConflict)
	}
	if decision.PRNumber != 350 {
		t.Errorf("PRNumber = %d, want 350", decision.PRNumber)
	}
}

func TestDecide_NeedsRebase(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := defaultTestConfig()

	// Add needs-rebase issue
	mockClient.IssuesByLabel[config.GitHub.Labels.NeedsRebase] = []Issue{
		{Number: 40, Body: "https://github.com/owner/repo/pull/400"},
	}

	a := newTestAnalyzer(tmpDir, config, mockClient)
	decision, err := a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	if decision.NextAction != ActionDispatchWorker {
		t.Errorf("NextAction = %q, want %q", decision.NextAction, ActionDispatchWorker)
	}
	if decision.MergeIssue != MergeIssueRebase {
		t.Errorf("MergeIssue = %q, want %q", decision.MergeIssue, MergeIssueRebase)
	}
}

func TestDecide_WorkerFailed(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := defaultTestConfig()

	// Add worker-failed issue
	mockClient.IssuesByLabel[config.GitHub.Labels.WorkerFailed] = []Issue{
		{Number: 45, Body: "worker failed"},
	}

	a := newTestAnalyzer(tmpDir, config, mockClient)
	decision, err := a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	if decision.NextAction != ActionNone {
		t.Errorf("NextAction = %q, want %q", decision.NextAction, ActionNone)
	}
	if decision.ExitReason != ReasonWorkerFailed {
		t.Errorf("ExitReason = %q, want %q", decision.ExitReason, ReasonWorkerFailed)
	}
}

func TestDecide_NeedsHumanReview(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := defaultTestConfig()

	// Add needs-human-review issue
	mockClient.IssuesByLabel[config.GitHub.Labels.NeedsHumanReview] = []Issue{
		{Number: 50, Body: "needs human review"},
	}

	a := newTestAnalyzer(tmpDir, config, mockClient)
	decision, err := a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	if decision.NextAction != ActionNone {
		t.Errorf("NextAction = %q, want %q", decision.NextAction, ActionNone)
	}
	if decision.ExitReason != ReasonNeedsHumanReview {
		t.Errorf("ExitReason = %q, want %q", decision.ExitReason, ReasonNeedsHumanReview)
	}
}

func TestDecide_PendingIssue(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := defaultTestConfig()

	// Add pending issue
	mockClient.PendingIssues = []Issue{
		{Number: 55, Body: "pending task"},
	}

	a := newTestAnalyzer(tmpDir, config, mockClient)
	decision, err := a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	if decision.NextAction != ActionDispatchWorker {
		t.Errorf("NextAction = %q, want %q", decision.NextAction, ActionDispatchWorker)
	}
	if decision.IssueNumber != 55 {
		t.Errorf("IssueNumber = %d, want 55", decision.IssueNumber)
	}
}

func TestDecide_PendingIssueWithMergedPR(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := defaultTestConfig()

	// Add pending issues - first one has merged PR, second is truly pending
	mockClient.PendingIssues = []Issue{
		{Number: 60, Body: "https://github.com/owner/repo/pull/600"},
		{Number: 61, Body: "truly pending"},
	}
	mockClient.PRMerged[600] = true

	a := newTestAnalyzer(tmpDir, config, mockClient)
	decision, err := a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	// Should close first issue and dispatch to second
	if len(mockClient.ClosedIssues) != 1 || mockClient.ClosedIssues[0] != 60 {
		t.Errorf("Expected issue 60 to be closed")
	}
	if decision.NextAction != ActionDispatchWorker {
		t.Errorf("NextAction = %q, want %q", decision.NextAction, ActionDispatchWorker)
	}
	if decision.IssueNumber != 61 {
		t.Errorf("IssueNumber = %d, want 61", decision.IssueNumber)
	}
}

func TestDecide_AllComplete(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := defaultTestConfig()

	// No issues and no open tasks
	mockClient.OpenIssueCount = 0

	a := newTestAnalyzer(tmpDir, config, mockClient)
	decision, err := a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	if decision.NextAction != ActionAllComplete {
		t.Errorf("NextAction = %q, want %q", decision.NextAction, ActionAllComplete)
	}
}

func TestDecide_NoActionableTasks(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := defaultTestConfig()

	// No pending issues but has open task issues
	mockClient.OpenIssueCount = 5

	a := newTestAnalyzer(tmpDir, config, mockClient)
	decision, err := a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	if decision.NextAction != ActionNone {
		t.Errorf("NextAction = %q, want %q", decision.NextAction, ActionNone)
	}
	if decision.ExitReason != ReasonNoActionableTasks {
		t.Errorf("ExitReason = %q, want %q", decision.ExitReason, ReasonNoActionableTasks)
	}
}

func TestDecide_GenerateTasks(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := &Config{
		GitHub: GitHubConfig{
			Labels: DefaultLabels(),
		},
		Specs: SpecsConfig{
			BasePath: ".ai/specs",
			Active:   []string{"my-feature"},
		},
	}

	// Create design.md but no tasks.md
	specDir := filepath.Join(tmpDir, ".ai", "specs", "my-feature")
	os.MkdirAll(specDir, 0755)
	os.WriteFile(filepath.Join(specDir, "design.md"), []byte("# Design"), 0644)

	a := newTestAnalyzer(tmpDir, config, mockClient)
	decision, err := a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	if decision.NextAction != ActionGenerateTasks {
		t.Errorf("NextAction = %q, want %q", decision.NextAction, ActionGenerateTasks)
	}
	if decision.SpecName != "my-feature" {
		t.Errorf("SpecName = %q, want %q", decision.SpecName, "my-feature")
	}
}

func TestDecide_CreateTask(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := &Config{
		GitHub: GitHubConfig{
			Labels: DefaultLabels(),
		},
		Specs: SpecsConfig{
			BasePath: ".ai/specs",
			Active:   []string{"my-feature"},
		},
	}

	// Create tasks.md with uncompleted task
	specDir := filepath.Join(tmpDir, ".ai", "specs", "my-feature")
	os.MkdirAll(specDir, 0755)
	tasksContent := `# Tasks
- [ ] First task
- [x] Completed task <!-- Issue #1 -->
`
	os.WriteFile(filepath.Join(specDir, "tasks.md"), []byte(tasksContent), 0644)

	a := newTestAnalyzer(tmpDir, config, mockClient)
	decision, err := a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	if decision.NextAction != ActionCreateTask {
		t.Errorf("NextAction = %q, want %q", decision.NextAction, ActionCreateTask)
	}
	if decision.SpecName != "my-feature" {
		t.Errorf("SpecName = %q, want %q", decision.SpecName, "my-feature")
	}
	if decision.TaskLine != 2 { // Line 2 is the uncompleted task
		t.Errorf("TaskLine = %d, want 2", decision.TaskLine)
	}
}

func TestDecide_MergeConflictNoPR(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := defaultTestConfig()

	// Add merge-conflict issue without PR number
	mockClient.IssuesByLabel[config.GitHub.Labels.MergeConflict] = []Issue{
		{Number: 70, Body: "conflict but no PR link"},
	}

	a := newTestAnalyzer(tmpDir, config, mockClient)
	decision, err := a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	if decision.NextAction != ActionNone {
		t.Errorf("NextAction = %q, want %q", decision.NextAction, ActionNone)
	}
	if decision.ExitReason != ReasonNeedsHumanReview {
		t.Errorf("ExitReason = %q, want %q", decision.ExitReason, ReasonNeedsHumanReview)
	}
}

func TestDecide_NeedsRebaseNoPR(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := defaultTestConfig()

	// Add needs-rebase issue without PR number
	mockClient.IssuesByLabel[config.GitHub.Labels.NeedsRebase] = []Issue{
		{Number: 75, Body: "rebase needed but no PR link"},
	}

	a := newTestAnalyzer(tmpDir, config, mockClient)
	decision, err := a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	if decision.NextAction != ActionNone {
		t.Errorf("NextAction = %q, want %q", decision.NextAction, ActionNone)
	}
	if decision.ExitReason != ReasonNeedsHumanReview {
		t.Errorf("ExitReason = %q, want %q", decision.ExitReason, ReasonNeedsHumanReview)
	}
}

func TestDecide_ReviewFailedNoPR(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := defaultTestConfig()

	// Add review-failed issue without PR number
	mockClient.IssuesByLabel[config.GitHub.Labels.ReviewFailed] = []Issue{
		{Number: 80, Body: "review failed but no PR link"},
	}

	a := newTestAnalyzer(tmpDir, config, mockClient)
	decision, err := a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	if decision.NextAction != ActionNone {
		t.Errorf("NextAction = %q, want %q", decision.NextAction, ActionNone)
	}
	if decision.ExitReason != ReasonNeedsHumanReview {
		t.Errorf("ExitReason = %q, want %q", decision.ExitReason, ReasonNeedsHumanReview)
	}
}

func TestDecide_InProgressAPIError(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := defaultTestConfig()

	// Simulate in-progress API failure - MUST return github_api_error to prevent double dispatch
	mockClient.IssuesByLabelError[config.GitHub.Labels.InProgress] = fmt.Errorf("API timeout")

	a := newTestAnalyzer(tmpDir, config, mockClient)
	decision, err := a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	if decision.NextAction != ActionNone {
		t.Errorf("NextAction = %q, want %q", decision.NextAction, ActionNone)
	}
	if decision.ExitReason != ReasonGitHubAPIError {
		t.Errorf("ExitReason = %q, want %q", decision.ExitReason, ReasonGitHubAPIError)
	}
}

func TestDecide_AllAPIErrors(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := defaultTestConfig()

	// In-progress succeeds (returns empty), but all other API calls fail
	mockClient.IssuesByLabelError[config.GitHub.Labels.PRReady] = fmt.Errorf("rate limited")
	mockClient.IssuesByLabelError[config.GitHub.Labels.ReviewFailed] = fmt.Errorf("rate limited")
	mockClient.IssuesByLabelError[config.GitHub.Labels.MergeConflict] = fmt.Errorf("rate limited")
	mockClient.IssuesByLabelError[config.GitHub.Labels.NeedsRebase] = fmt.Errorf("rate limited")
	mockClient.IssuesByLabelError[config.GitHub.Labels.WorkerFailed] = fmt.Errorf("rate limited")
	mockClient.IssuesByLabelError[config.GitHub.Labels.NeedsHumanReview] = fmt.Errorf("rate limited")
	mockClient.PendingIssuesError = fmt.Errorf("rate limited")
	mockClient.CountError = fmt.Errorf("rate limited")

	a := newTestAnalyzer(tmpDir, config, mockClient)
	decision, err := a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	// Should return github_api_error, NOT no_actionable_tasks
	if decision.NextAction != ActionNone {
		t.Errorf("NextAction = %q, want %q", decision.NextAction, ActionNone)
	}
	if decision.ExitReason != ReasonGitHubAPIError {
		t.Errorf("ExitReason = %q, want %q (should not be %q)", decision.ExitReason, ReasonGitHubAPIError, ReasonNoActionableTasks)
	}
}

func TestDecide_PRReadyFromBranchLookup(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := defaultTestConfig()

	// Add pr-ready issue without PR in body or result file
	mockClient.IssuesByLabel[config.GitHub.Labels.PRReady] = []Issue{
		{Number: 85, Body: "PR ready but no link anywhere"},
	}

	// Set up branch lookup fallback
	mockClient.PRByBranch["feat/ai-issue-85"] = 850

	a := newTestAnalyzer(tmpDir, config, mockClient)
	decision, err := a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	if decision.NextAction != ActionReviewPR {
		t.Errorf("NextAction = %q, want %q", decision.NextAction, ActionReviewPR)
	}
	if decision.PRNumber != 850 {
		t.Errorf("PRNumber = %d, want 850", decision.PRNumber)
	}
}

func TestExtractPRNumberForIssue_PRNumberField(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := defaultTestConfig()

	a := newTestAnalyzer(tmpDir, config, mockClient)

	// Create result file with pr_number integer field
	resultDir := filepath.Join(tmpDir, ".ai", "results")
	os.MkdirAll(resultDir, 0755)
	resultData, _ := json.Marshal(map[string]any{"pr_number": 999})
	os.WriteFile(filepath.Join(resultDir, "issue-90.json"), resultData, 0644)

	prNumber := a.extractPRNumberForIssue(90, "no PR link")
	if prNumber != 999 {
		t.Errorf("PRNumber = %d, want 999", prNumber)
	}
}
