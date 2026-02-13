package analyzer

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Constants for loop safety
const (
	MaxLoop                      = 1000
	MaxConsecutiveFailures       = 5
	DefaultMaxReviewAttempts     = 3
)

// Analyzer implements workflow decision logic
type Analyzer struct {
	StateRoot string
	Config    *Config
	GHClient  GitHubClientInterface
	GHTimeout time.Duration
	mu        sync.Mutex // protects loop counter updates
}

// New creates a new Analyzer
func New(stateRoot string, config *Config) *Analyzer {
	if stateRoot == "" {
		stateRoot = "."
	}
	timeout := 30 * time.Second
	return &Analyzer{
		StateRoot: stateRoot,
		Config:    config,
		GHClient:  NewGitHubClient(timeout),
		GHTimeout: timeout,
	}
}

// Decide determines the next action based on workflow state
func (a *Analyzer) Decide(ctx context.Context) (*Decision, error) {
	// Check for config
	if a.Config == nil {
		configPath := filepath.Join(a.StateRoot, ".ai", "config", "workflow.yaml")
		config, err := LoadConfig(configPath)
		if err != nil {
			return &Decision{
				NextAction: ActionNone,
				ExitReason: ReasonConfigNotFound,
			}, nil
		}
		a.Config = config
	}

	labels := a.Config.GitHub.Labels

	// Step 0: Loop safety check
	loopCount, err := a.updateLoopCount()
	if err != nil {
		// If we can't update/read loop count, stop to prevent infinite loop
		return &Decision{
			NextAction: ActionNone,
			ExitReason: ReasonLoopCountError,
		}, nil
	}
	if loopCount >= MaxLoop {
		return &Decision{
			NextAction: ActionNone,
			ExitReason: ReasonMaxLoopReached,
		}, nil
	}

	consecutiveFailures := a.readConsecutiveFailures()
	if consecutiveFailures >= MaxConsecutiveFailures {
		return &Decision{
			NextAction: ActionNone,
			ExitReason: ReasonMaxConsecutiveFailures,
		}, nil
	}

	// Step 1: Check in-progress issues
	// CRITICAL: This check MUST succeed. If it fails, we might double-dispatch a worker
	// to an issue that already has one running.
	inProgressIssues, err := a.GHClient.ListIssuesByLabel(ctx, labels.InProgress)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[analyzer] error: failed to check in-progress issues: %v\n", err)
		return &Decision{
			NextAction: ActionNone,
			ExitReason: ReasonGitHubAPIError,
		}, nil
	}
	if len(inProgressIssues) > 0 {
		return &Decision{
			NextAction:  ActionCheckResult,
			IssueNumber: inProgressIssues[0].Number,
		}, nil
	}

	// Track API errors for subsequent non-critical checks
	var apiErrors int

	// Step 2: Check pr-ready issues
	prReadyIssues, err := a.GHClient.ListIssuesByLabel(ctx, labels.PRReady)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[analyzer] warning: failed to check pr-ready issues: %v\n", err)
		apiErrors++
	}
	if err == nil && len(prReadyIssues) > 0 {
		issue := prReadyIssues[0]
		prNumber := a.extractPRNumberForIssue(issue.Number, issue.Body)

		if prNumber > 0 {
			return &Decision{
				NextAction:  ActionReviewPR,
				IssueNumber: issue.Number,
				PRNumber:    prNumber,
			}, nil
		}
		// Can't extract PR number - mark as needs-human-review to prevent infinite loop
		a.updateIssueLabels(ctx, issue.Number, []string{labels.NeedsHumanReview}, []string{labels.PRReady})
		return &Decision{
			NextAction:  ActionNone,
			IssueNumber: issue.Number,
			ExitReason:  ReasonNeedsHumanReview,
		}, nil
	}

	// Step 2.3: Check review-failed issues (retry with new subagent)
	reviewFailedIssues, err := a.GHClient.ListIssuesByLabel(ctx, labels.ReviewFailed)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[analyzer] warning: failed to check review-failed issues: %v\n", err)
		apiErrors++
	}
	if err == nil && len(reviewFailedIssues) > 0 {
		issue := reviewFailedIssues[0]
		prNumber := a.extractPRNumberForIssue(issue.Number, issue.Body)

		if prNumber > 0 {
			attempts := a.getReviewAttempts(prNumber)
			if attempts < a.maxReviewAttempts() {
				// Try to persist the incremented attempt count BEFORE allowing retry
				// This prevents infinite loops if we can't track attempts
				if err := a.incrementReviewAttempts(prNumber); err != nil {
					// Cannot persist attempt count - escalate to human to prevent infinite loop
					a.updateIssueLabels(ctx, issue.Number, []string{labels.NeedsHumanReview}, []string{labels.ReviewFailed})
					return &Decision{
						NextAction:  ActionNone,
						IssueNumber: issue.Number,
						PRNumber:    prNumber,
						ExitReason:  ReasonNeedsHumanReview,
					}, nil
				}
				// Allow new subagent to retry
				a.updateIssueLabels(ctx, issue.Number, []string{labels.PRReady}, []string{labels.ReviewFailed})
				return &Decision{
					NextAction:  ActionReviewPR,
					IssueNumber: issue.Number,
					PRNumber:    prNumber,
				}, nil
			}
			// Max retries exceeded, escalate to human
			a.updateIssueLabels(ctx, issue.Number, []string{labels.NeedsHumanReview}, []string{labels.ReviewFailed})
			return &Decision{
				NextAction:  ActionNone,
				IssueNumber: issue.Number,
				PRNumber:    prNumber,
				ExitReason:  ReasonReviewMaxRetries,
			}, nil
		}
		// Can't extract PR number - mark as needs-human-review to prevent infinite loop
		a.updateIssueLabels(ctx, issue.Number, []string{labels.NeedsHumanReview}, []string{labels.ReviewFailed})
		return &Decision{
			NextAction:  ActionNone,
			IssueNumber: issue.Number,
			ExitReason:  ReasonNeedsHumanReview,
		}, nil
	}

	// Step 2.5: Check merge-conflict label (Worker needs to fix conflict)
	// Note: Don't remove the label here - dispatch-worker will handle it
	// This ensures the label persists if dispatch fails or MergeIssue isn't passed correctly
	conflictIssues, err := a.GHClient.ListIssuesByLabel(ctx, labels.MergeConflict)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[analyzer] warning: failed to check merge-conflict issues: %v\n", err)
		apiErrors++
	}
	if err == nil && len(conflictIssues) > 0 {
		issue := conflictIssues[0]
		prNumber := a.extractPRNumberForIssue(issue.Number, issue.Body)
		if prNumber == 0 {
			// Can't resolve conflict without PR number - mark as needs-human-review
			a.updateIssueLabels(ctx, issue.Number, []string{labels.NeedsHumanReview}, []string{labels.MergeConflict})
			return &Decision{
				NextAction:  ActionNone,
				IssueNumber: issue.Number,
				ExitReason:  ReasonNeedsHumanReview,
			}, nil
		}
		return &Decision{
			NextAction:  ActionDispatchWorker,
			IssueNumber: issue.Number,
			PRNumber:    prNumber,
			MergeIssue:  MergeIssueConflict,
		}, nil
	}

	// Step 2.6: Check needs-rebase label (Worker needs to rebase)
	// Note: Don't remove the label here - dispatch-worker will handle it
	rebaseIssues, err := a.GHClient.ListIssuesByLabel(ctx, labels.NeedsRebase)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[analyzer] warning: failed to check needs-rebase issues: %v\n", err)
		apiErrors++
	}
	if err == nil && len(rebaseIssues) > 0 {
		issue := rebaseIssues[0]
		prNumber := a.extractPRNumberForIssue(issue.Number, issue.Body)
		if prNumber == 0 {
			// Can't rebase without PR number - mark as needs-human-review
			a.updateIssueLabels(ctx, issue.Number, []string{labels.NeedsHumanReview}, []string{labels.NeedsRebase})
			return &Decision{
				NextAction:  ActionNone,
				IssueNumber: issue.Number,
				ExitReason:  ReasonNeedsHumanReview,
			}, nil
		}
		return &Decision{
			NextAction:  ActionDispatchWorker,
			IssueNumber: issue.Number,
			PRNumber:    prNumber,
			MergeIssue:  MergeIssueRebase,
		}, nil
	}

	// Step 2.7: Check for blocking labels
	workerFailedIssues, err := a.GHClient.ListIssuesByLabel(ctx, labels.WorkerFailed)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[analyzer] warning: failed to check worker-failed issues: %v\n", err)
		apiErrors++
	}
	if err == nil && len(workerFailedIssues) > 0 {
		return &Decision{
			NextAction:  ActionNone,
			IssueNumber: workerFailedIssues[0].Number,
			ExitReason:  ReasonWorkerFailed,
		}, nil
	}

	needsReviewIssues, err := a.GHClient.ListIssuesByLabel(ctx, labels.NeedsHumanReview)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[analyzer] warning: failed to check needs-human-review issues: %v\n", err)
		apiErrors++
	}
	if err == nil && len(needsReviewIssues) > 0 {
		return &Decision{
			NextAction:  ActionNone,
			IssueNumber: needsReviewIssues[0].Number,
			ExitReason:  ReasonNeedsHumanReview,
		}, nil
	}

	// Step 3: Check pending issues
	pendingIssues, err := a.GHClient.ListPendingIssues(ctx, labels)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[analyzer] warning: failed to check pending issues: %v\n", err)
		apiErrors++
	}
	if err == nil && len(pendingIssues) > 0 {
		// Check each pending issue for merged PR (cleanup orphaned issues)
		for _, issue := range pendingIssues {
			prNumber := a.extractPRNumberForIssue(issue.Number, issue.Body)
			if prNumber > 0 {
				// Check if PR is merged
				if merged, err := a.GHClient.IsPRMerged(ctx, prNumber); err == nil && merged {
					// PR is merged but issue is still open - close it
					_ = a.GHClient.CloseIssue(ctx, issue.Number)
					continue // Check next issue
				}
			}
			// This issue is truly pending (no merged PR)
			return &Decision{
				NextAction:  ActionDispatchWorker,
				IssueNumber: issue.Number,
			}, nil
		}
	}

	// Step 4: Check for uncompleted tasks (mode-dependent)
	if a.Config != nil && a.Config.IsEpicMode() {
		if decision := a.checkEpicProgress(ctx); decision != nil {
			return decision, nil
		}
		// In epic mode, if checkEpicProgress returned nil, all tasks are done
		return &Decision{
			NextAction: ActionAllComplete,
		}, nil
	}

	if decision := a.checkTasksFiles(); decision != nil {
		return decision, nil
	}

	// Step 5: Check if all complete (tasks_md mode)
	openCount, err := a.GHClient.CountOpenIssues(ctx, labels.Task)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[analyzer] warning: failed to count open issues: %v\n", err)
		apiErrors++
	}
	if err == nil && openCount == 0 {
		return &Decision{
			NextAction: ActionAllComplete,
		}, nil
	}

	// Step 6: No actionable tasks
	// Distinguish between "no tasks found" and "couldn't check due to API errors"
	if apiErrors > 0 {
		return &Decision{
			NextAction: ActionNone,
			ExitReason: ReasonGitHubAPIError,
		}, nil
	}
	return &Decision{
		NextAction: ActionNone,
		ExitReason: ReasonNoActionableTasks,
	}, nil
}

// writeFileAtomic writes data to a file atomically using tmp+rename pattern
// This prevents file corruption if the process crashes during write
// Note: On Windows, os.Rename cannot overwrite existing files, so we remove first
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmpFile := path + ".tmp"
	if err := os.WriteFile(tmpFile, data, perm); err != nil {
		return err
	}

	// Remove target file first for Windows compatibility
	// On Windows, os.Rename fails if destination exists
	_ = os.Remove(path)

	if err := os.Rename(tmpFile, path); err != nil {
		os.Remove(tmpFile) // cleanup on failure
		return err
	}
	return nil
}

// updateLoopCount increments and returns the loop count
// This method is safe for concurrent use.
func (a *Analyzer) updateLoopCount() (int, error) {
	// Lock to prevent concurrent read-modify-write race conditions
	a.mu.Lock()
	defer a.mu.Unlock()

	loopFile := filepath.Join(a.StateRoot, ".ai", "state", "loop_count")

	// Read current count
	data, err := os.ReadFile(loopFile)
	count := 0
	if err == nil {
		count, _ = strconv.Atoi(strings.TrimSpace(string(data)))
	}

	// Increment
	count++

	// Write back atomically
	if err := writeFileAtomic(loopFile, []byte(strconv.Itoa(count)), 0644); err != nil {
		return count, err
	}

	return count, nil
}

// readConsecutiveFailures reads the consecutive failures count
func (a *Analyzer) readConsecutiveFailures() int {
	failFile := filepath.Join(a.StateRoot, ".ai", "state", "consecutive_failures")
	data, err := os.ReadFile(failFile)
	if err != nil {
		return 0
	}
	count, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	return count
}

// extractPRNumberForIssue extracts PR number from result file, issue body, or branch lookup
func (a *Analyzer) extractPRNumberForIssue(issueNumber int, issueBody string) int {
	// Try result file first (most reliable: has pr_url from worker)
	resultFile := filepath.Join(a.StateRoot, ".ai", "results", "issue-"+strconv.Itoa(issueNumber)+".json")
	if data, err := os.ReadFile(resultFile); err == nil {
		var result struct {
			PRURL    string `json:"pr_url"`
			PRNumber int    `json:"pr_number"`
		}
		if json.Unmarshal(data, &result) == nil {
			// Try pr_number field first (integer, more reliable)
			if result.PRNumber > 0 {
				return result.PRNumber
			}
			// Fallback to pr_url string parsing
			if result.PRURL != "" {
				if num := ExtractPRNumber(result.PRURL); num > 0 {
					return num
				}
			}
		}
	}

	// Try issue body
	if num := ExtractPRNumber(issueBody); num > 0 {
		return num
	}

	// Fallback: look up PR by branch name convention (feat/ai-issue-{N})
	branchName := fmt.Sprintf("feat/ai-issue-%d", issueNumber)
	if prNum, err := a.GHClient.FindPRByBranch(context.Background(), branchName); err == nil && prNum > 0 {
		return prNum
	}

	return 0
}

// checkTasksFiles checks tasks.md files for uncompleted tasks
func (a *Analyzer) checkTasksFiles() *Decision {
	if a.Config == nil || len(a.Config.Specs.Active) == 0 {
		return nil
	}

	basePath := a.Config.Specs.BasePath
	if basePath == "" {
		basePath = ".ai/specs"
	}

	for _, spec := range a.Config.Specs.Active {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}

		tasksFile := filepath.Join(a.StateRoot, basePath, spec, "tasks.md")
		designFile := filepath.Join(a.StateRoot, basePath, spec, "design.md")

		// Check if tasks.md exists
		if _, err := os.Stat(tasksFile); os.IsNotExist(err) {
			// Check if design.md exists (need to generate tasks)
			if _, err := os.Stat(designFile); err == nil {
				return &Decision{
					NextAction: ActionGenerateTasks,
					SpecName:   spec,
				}
			}
			continue
		}

		// Find uncompleted task without Issue reference
		file, err := os.Open(tasksFile)
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(file)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()

			// Check for uncompleted task
			if strings.HasPrefix(line, "- [ ]") && !strings.Contains(line, "<!-- Issue #") {
				file.Close()
				return &Decision{
					NextAction: ActionCreateTask,
					SpecName:   spec,
					TaskLine:   lineNum,
				}
			}
		}
		// Check for scanner errors (e.g., I/O errors during reading)
		if err := scanner.Err(); err != nil {
			file.Close()
			continue // Skip this spec file on read error
		}
		file.Close()
	}

	return nil
}

// checkEpicProgress checks GitHub tracking issues for uncompleted tasks.
// Returns a Decision if there's work to do, or nil if all tasks are complete.
func (a *Analyzer) checkEpicProgress(ctx context.Context) *Decision {
	if a.Config == nil || len(a.Config.Specs.Active) == 0 {
		return nil
	}

	for _, spec := range a.Config.Specs.Active {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}

		epicIssue := a.Config.GetEpicIssue(spec)
		if epicIssue == 0 {
			fmt.Fprintf(os.Stderr, "[analyzer] warning: epic mode but no epic_issue configured for spec %q\n", spec)
			continue
		}

		body, err := a.GHClient.GetIssueBody(ctx, epicIssue)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[analyzer] warning: failed to read epic #%d body: %v\n", epicIssue, err)
			continue
		}

		tasks := ParseEpicBody(body)
		for _, task := range tasks {
			if task.Completed {
				continue
			}

			// Unchecked task with issue ref — check if issue is still open (in-progress)
			if task.IssueNumber > 0 {
				// This task has a child issue already; it's being worked on or pending
				// The standard issue-label checks in Steps 1-3 handle these
				continue
			}

			// Unchecked task without issue ref — needs a new child issue
			return &Decision{
				NextAction: ActionCreateTask,
				SpecName:   spec,
				EpicIssue:  epicIssue,
				TaskText:   task.Text,
			}
		}
	}

	return nil
}

// maxReviewAttempts returns the configured max review attempts, falling back to default.
func (a *Analyzer) maxReviewAttempts() int {
	if a.Config != nil && a.Config.Escalation.MaxReviewAttempts > 0 {
		return a.Config.Escalation.MaxReviewAttempts
	}
	return DefaultMaxReviewAttempts
}

// getReviewAttempts returns the number of review attempts for a PR
func (a *Analyzer) getReviewAttempts(prNumber int) int {
	attemptFile := filepath.Join(a.StateRoot, ".ai", "state", "attempts", "review-pr-"+strconv.Itoa(prNumber))
	data, err := os.ReadFile(attemptFile)
	if err != nil {
		return 0
	}
	count, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	return count
}

// incrementReviewAttempts increments the review attempt count for a PR
// Returns error if the attempt count cannot be persisted (which could cause infinite retry loops)
func (a *Analyzer) incrementReviewAttempts(prNumber int) error {
	attemptFile := filepath.Join(a.StateRoot, ".ai", "state", "attempts", "review-pr-"+strconv.Itoa(prNumber))

	count := a.getReviewAttempts(prNumber) + 1
	// Use atomic write to prevent corruption
	if err := writeFileAtomic(attemptFile, []byte(strconv.Itoa(count)), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "[analyzer] error: failed to persist review attempt count for PR #%d: %v\n", prNumber, err)
		return err
	}
	return nil
}

// updateIssueLabels is a helper that logs warnings if label operations fail.
// Label operations are non-critical - we log warnings but don't fail the workflow.
func (a *Analyzer) updateIssueLabels(ctx context.Context, issueNumber int, addLabels, removeLabels []string) {
	for _, label := range removeLabels {
		if err := a.GHClient.RemoveLabel(ctx, issueNumber, label); err != nil {
			fmt.Fprintf(os.Stderr, "[analyzer] warning: failed to remove label %q from issue #%d: %v\n", label, issueNumber, err)
		}
	}
	for _, label := range addLabels {
		if err := a.GHClient.AddLabel(ctx, issueNumber, label); err != nil {
			fmt.Fprintf(os.Stderr, "[analyzer] warning: failed to add label %q to issue #%d: %v\n", label, issueNumber, err)
		}
	}
}
