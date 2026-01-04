package analyzer

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Constants for loop safety
const (
	MaxLoop                = 1000
	MaxConsecutiveFailures = 5
	MaxReviewAttempts      = 2
)

// Analyzer implements workflow decision logic
type Analyzer struct {
	StateRoot string
	Config    *Config
	GHClient  *GitHubClient
	GHTimeout time.Duration
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
	if err == nil && loopCount >= MaxLoop {
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
	inProgressIssues, err := a.GHClient.ListIssuesByLabel(ctx, labels.InProgress)
	if err == nil && len(inProgressIssues) > 0 {
		return &Decision{
			NextAction:  ActionCheckResult,
			IssueNumber: inProgressIssues[0].Number,
		}, nil
	}

	// Step 2: Check pr-ready issues
	prReadyIssues, err := a.GHClient.ListIssuesByLabel(ctx, labels.PRReady)
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
		// Can't extract PR number, remove pr-ready label
		_ = a.GHClient.RemoveLabel(ctx, issue.Number, labels.PRReady)
	}

	// Step 2.3: Check review-failed issues (retry with new subagent)
	reviewFailedIssues, err := a.GHClient.ListIssuesByLabel(ctx, labels.ReviewFailed)
	if err == nil && len(reviewFailedIssues) > 0 {
		issue := reviewFailedIssues[0]
		prNumber := a.extractPRNumberForIssue(issue.Number, issue.Body)

		if prNumber > 0 {
			attempts := a.getReviewAttempts(prNumber)
			if attempts < MaxReviewAttempts {
				// Allow new subagent to retry
				_ = a.GHClient.RemoveLabel(ctx, issue.Number, labels.ReviewFailed)
				_ = a.GHClient.AddLabel(ctx, issue.Number, labels.PRReady)
				a.incrementReviewAttempts(prNumber)
				return &Decision{
					NextAction:  ActionReviewPR,
					IssueNumber: issue.Number,
					PRNumber:    prNumber,
				}, nil
			}
			// Max retries exceeded, escalate to human
			_ = a.GHClient.RemoveLabel(ctx, issue.Number, labels.ReviewFailed)
			_ = a.GHClient.AddLabel(ctx, issue.Number, labels.NeedsHumanReview)
			return &Decision{
				NextAction:  ActionNone,
				IssueNumber: issue.Number,
				PRNumber:    prNumber,
				ExitReason:  ReasonReviewMaxRetries,
			}, nil
		}
	}

	// Step 2.5: Check merge-conflict label (Worker needs to fix conflict)
	// Note: Don't remove the label here - dispatch-worker will handle it
	// This ensures the label persists if dispatch fails or MergeIssue isn't passed correctly
	conflictIssues, err := a.GHClient.ListIssuesByLabel(ctx, labels.MergeConflict)
	if err == nil && len(conflictIssues) > 0 {
		issue := conflictIssues[0]
		prNumber := a.extractPRNumberForIssue(issue.Number, issue.Body)
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
	if err == nil && len(rebaseIssues) > 0 {
		issue := rebaseIssues[0]
		prNumber := a.extractPRNumberForIssue(issue.Number, issue.Body)
		return &Decision{
			NextAction:  ActionDispatchWorker,
			IssueNumber: issue.Number,
			PRNumber:    prNumber,
			MergeIssue:  MergeIssueRebase,
		}, nil
	}

	// Step 2.7: Check for blocking labels
	workerFailedIssues, err := a.GHClient.ListIssuesByLabel(ctx, labels.WorkerFailed)
	if err == nil && len(workerFailedIssues) > 0 {
		return &Decision{
			NextAction:  ActionNone,
			IssueNumber: workerFailedIssues[0].Number,
			ExitReason:  ReasonWorkerFailed,
		}, nil
	}

	needsReviewIssues, err := a.GHClient.ListIssuesByLabel(ctx, labels.NeedsHumanReview)
	if err == nil && len(needsReviewIssues) > 0 {
		return &Decision{
			NextAction:  ActionNone,
			IssueNumber: needsReviewIssues[0].Number,
			ExitReason:  ReasonNeedsHumanReview,
		}, nil
	}

	// Step 3: Check pending issues
	pendingIssues, err := a.GHClient.ListPendingIssues(ctx, labels)
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

	// Step 4: Check tasks.md for uncompleted tasks
	if decision := a.checkTasksFiles(); decision != nil {
		return decision, nil
	}

	// Step 5: Check if all complete
	openCount, err := a.GHClient.CountOpenIssues(ctx, labels.Task)
	if err == nil && openCount == 0 {
		return &Decision{
			NextAction: ActionAllComplete,
		}, nil
	}

	// Step 6: No actionable tasks
	return &Decision{
		NextAction: ActionNone,
		ExitReason: ReasonNoActionableTasks,
	}, nil
}

// updateLoopCount increments and returns the loop count
func (a *Analyzer) updateLoopCount() (int, error) {
	loopFile := filepath.Join(a.StateRoot, ".ai", "state", "loop_count")

	// Read current count
	data, err := os.ReadFile(loopFile)
	count := 0
	if err == nil {
		count, _ = strconv.Atoi(strings.TrimSpace(string(data)))
	}

	// Increment
	count++

	// Write back
	if err := os.MkdirAll(filepath.Dir(loopFile), 0755); err != nil {
		return count, err
	}
	if err := os.WriteFile(loopFile, []byte(strconv.Itoa(count)), 0644); err != nil {
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

// extractPRNumberForIssue extracts PR number from result file or issue body
func (a *Analyzer) extractPRNumberForIssue(issueNumber int, issueBody string) int {
	// Try result file first
	resultFile := filepath.Join(a.StateRoot, ".ai", "results", "issue-"+strconv.Itoa(issueNumber)+".json")
	if data, err := os.ReadFile(resultFile); err == nil {
		var result struct {
			PRURL string `json:"pr_url"`
		}
		if json.Unmarshal(data, &result) == nil && result.PRURL != "" {
			return ExtractPRNumber(result.PRURL)
		}
	}

	// Fallback to issue body
	return ExtractPRNumber(issueBody)
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
		file.Close()
	}

	return nil
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
func (a *Analyzer) incrementReviewAttempts(prNumber int) {
	attemptDir := filepath.Join(a.StateRoot, ".ai", "state", "attempts")
	_ = os.MkdirAll(attemptDir, 0755)
	attemptFile := filepath.Join(attemptDir, "review-pr-"+strconv.Itoa(prNumber))

	count := a.getReviewAttempts(prNumber) + 1
	_ = os.WriteFile(attemptFile, []byte(strconv.Itoa(count)), 0644)
}
