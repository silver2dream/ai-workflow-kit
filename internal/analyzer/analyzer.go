package analyzer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/config"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
	"github.com/silver2dream/ai-workflow-kit/internal/stopmark"
	"github.com/silver2dream/ai-workflow-kit/internal/workflow"
)

// Constants for loop safety
const (
	MaxLoop                  = 1000
	MaxConsecutiveFailures   = 5
	DefaultMaxReviewAttempts = 3
)

// Analyzer implements workflow decision logic
type Analyzer struct {
	StateRoot string
	Config    *Config
	GHClient  GitHubClientInterface
	GHTimeout time.Duration
	// ReadOnly decides without acting on the decision: no label is
	// changed and no attempt is counted. The engine's shadow decides so
	// (LabelDecision; design.md §5.1).
	ReadOnly bool
	// AttemptsOf, when it is set, counts the attempts in the place of the
	// state files: the engine's shadow gives the counts of its lanes. ok
	// false is no count there, and the file is read.
	AttemptsOf func(kind string, pr int) (count int, ok bool)
	only       int        // the one issue the run is limited to, 0 for every ticket (see LimitTo)
	mu         sync.Mutex // protects loop counter updates
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

// Decide determines the next action based on workflow state. It is
// Precheck, then the labels of the tickets in the workflow's hands, then
// NextTicket: the engine (internal/engine) asks Precheck and NextTicket the
// same, and reads its own lanes in the place of the labels.
func (a *Analyzer) Decide(ctx context.Context) (*Decision, error) {
	if d := a.Precheck(); d != nil {
		return d, nil
	}
	d, apiErrors, err := a.labelDecision(ctx)
	if d != nil || err != nil {
		return d, err
	}
	return a.pickTicket(ctx, apiErrors)
}

// LabelDecision is what Decide decides from the labels of the tickets in
// the workflow's hands: a ticket a Worker is at work on, then one in review,
// one whose review is tried again, one to resolve, one that needs a person.
// nil is none in its hands: the next ticket is picked (NextTicket). It is
// what the engine's shadow compares with the engine's lanes.
func (a *Analyzer) LabelDecision(ctx context.Context) (*Decision, error) {
	d, _, err := a.labelDecision(ctx)
	return d, err
}

// labelDecision is LabelDecision, and the lookups of it that failed (which
// picking a ticket is told).
func (a *Analyzer) labelDecision(ctx context.Context) (*Decision, int, error) {
	labels := a.Config.GitHub.Labels

	// Step 1: Check in-progress issues
	// CRITICAL: This check MUST succeed. If it fails, we might double-dispatch a worker
	// to an issue that already has one running.
	inProgressIssues, err := a.GHClient.ListIssuesByLabel(ctx, labels.InProgress)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[analyzer] error: failed to check in-progress issues: %v\n", err)
		return &Decision{
			NextAction: ActionNone,
			ExitReason: ReasonGitHubAPIError,
		}, 0, nil
	}
	if len(inProgressIssues) > 0 {
		return &Decision{
			NextAction:  ActionCheckResult,
			IssueNumber: inProgressIssues[0].Number,
		}, 0, nil
	}

	return a.decideFromLabels(ctx)
}

// Precheck is what is decided before any ticket is looked at: a
// configuration that cannot be read, a stop that was asked for, too many
// rounds, too many failures in a row. nil is none of them. It counts the
// round.
func (a *Analyzer) Precheck() *Decision {
	// Check for config
	if a.Config == nil {
		configPath := config.Path(a.StateRoot)
		config, err := LoadConfig(configPath)
		if err != nil {
			// A file that is not there is created; a file that cannot be used
			// is corrected. The run stops either way, and says which.
			reason := ReasonConfigNotFound
			if !os.IsNotExist(err) {
				reason = ReasonConfigInvalid
				fmt.Fprintf(os.Stderr, "[analyzer] error: %v\n", err)
			}
			return &Decision{
				NextAction: ActionNone,
				ExitReason: reason,
			}
		}
		a.Config = config
	}

	// A stop was asked for: nothing more is handed out. This is how a stop
	// reaches a Principal in the middle of its session; the run's own loop
	// looks for the marker only between sessions. The reason is the marker's
	// when `awkit stop-workflow` knows it, and the user's otherwise.
	if reason, stopped := stopmark.Read(a.StateRoot); stopped {
		if !workflow.IsValidExitReason(reason) {
			reason = ReasonStopRequested
		}
		return &Decision{NextAction: ActionNone, ExitReason: reason}
	}

	// Step 0: Loop safety check
	loopCount, err := a.updateLoopCount()
	if err != nil {
		// If we can't update/read loop count, stop to prevent infinite loop
		return &Decision{
			NextAction: ActionNone,
			ExitReason: ReasonLoopCountError,
		}
	}
	if loopCount >= MaxLoop {
		return &Decision{
			NextAction: ActionNone,
			ExitReason: ReasonMaxLoopReached,
		}
	}

	consecutiveFailures := a.readConsecutiveFailures()
	if consecutiveFailures >= MaxConsecutiveFailures {
		return &Decision{
			NextAction: ActionNone,
			ExitReason: ReasonMaxConsecutiveFailures,
		}
	}

	return nil
}

// decideFromLabels reads the labels of the tickets in the workflow's hands
// (steps 2 to 2.7). nil is none in its hands; the lookups that failed are
// counted for picking the next ticket (NextTicket).
func (a *Analyzer) decideFromLabels(ctx context.Context) (*Decision, int, error) {
	labels := a.Config.GitHub.Labels

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
			// A pr-ready PR that conflicts with its base can't be merged, so
			// reviewing it just stalls the loop (the reviewer approves, the merge
			// fails, the run escalates). Route it to automatic conflict resolution
			// instead: dispatch_worker with a conflict merge-issue rebases the PR
			// branch onto its base (and escalates to conflict-resolver if the
			// worker can't). Only a definitive CONFLICTING is actionable — a
			// transient UNKNOWN or an API error falls through to normal review.
			// Bounded by an attempt counter so a genuinely unresolvable conflict
			// escalates to a human rather than looping forever.
			if status, mErr := a.GHClient.PRMergeable(ctx, prNumber); mErr == nil && status == "CONFLICTING" {
				if a.getAttempts("conflict", prNumber) >= a.maxReviewAttempts() {
					a.updateIssueLabels(ctx, issue.Number, []string{labels.NeedsHumanReview}, []string{labels.PRReady})
					return &Decision{
						NextAction:  ActionNone,
						IssueNumber: issue.Number,
						PRNumber:    prNumber,
						ExitReason:  ReasonNeedsHumanReview,
					}, apiErrors, nil
				}
				if err := a.incrementAttempts("conflict", prNumber); err != nil {
					a.updateIssueLabels(ctx, issue.Number, []string{labels.NeedsHumanReview}, []string{labels.PRReady})
					return &Decision{
						NextAction:  ActionNone,
						IssueNumber: issue.Number,
						PRNumber:    prNumber,
						ExitReason:  ReasonNeedsHumanReview,
					}, apiErrors, nil
				}
				return &Decision{
					NextAction:  ActionDispatchWorker,
					IssueNumber: issue.Number,
					PRNumber:    prNumber,
					MergeIssue:  MergeIssueConflict,
				}, apiErrors, nil
			}
			return &Decision{
				NextAction:  ActionReviewPR,
				IssueNumber: issue.Number,
				PRNumber:    prNumber,
			}, apiErrors, nil
		}
		// Can't extract PR number - mark as needs-human-review to prevent infinite loop
		a.updateIssueLabels(ctx, issue.Number, []string{labels.NeedsHumanReview}, []string{labels.PRReady})
		return &Decision{
			NextAction:  ActionNone,
			IssueNumber: issue.Number,
			ExitReason:  ReasonNeedsHumanReview,
		}, apiErrors, nil
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
					}, apiErrors, nil
				}
				// Allow new subagent to retry
				a.updateIssueLabels(ctx, issue.Number, []string{labels.PRReady}, []string{labels.ReviewFailed})
				return &Decision{
					NextAction:  ActionReviewPR,
					IssueNumber: issue.Number,
					PRNumber:    prNumber,
				}, apiErrors, nil
			}
			// Max retries exceeded, escalate to human
			a.updateIssueLabels(ctx, issue.Number, []string{labels.NeedsHumanReview}, []string{labels.ReviewFailed})
			return &Decision{
				NextAction:  ActionNone,
				IssueNumber: issue.Number,
				PRNumber:    prNumber,
				ExitReason:  ReasonReviewMaxRetries,
			}, apiErrors, nil
		}
		// Can't extract PR number - mark as needs-human-review to prevent infinite loop
		a.updateIssueLabels(ctx, issue.Number, []string{labels.NeedsHumanReview}, []string{labels.ReviewFailed})
		return &Decision{
			NextAction:  ActionNone,
			IssueNumber: issue.Number,
			ExitReason:  ReasonNeedsHumanReview,
		}, apiErrors, nil
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
			}, apiErrors, nil
		}
		return &Decision{
			NextAction:  ActionDispatchWorker,
			IssueNumber: issue.Number,
			PRNumber:    prNumber,
			MergeIssue:  MergeIssueConflict,
		}, apiErrors, nil
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
			}, apiErrors, nil
		}
		return &Decision{
			NextAction:  ActionDispatchWorker,
			IssueNumber: issue.Number,
			PRNumber:    prNumber,
			MergeIssue:  MergeIssueRebase,
		}, apiErrors, nil
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
		}, apiErrors, nil
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
		}, apiErrors, nil
	}

	return nil, apiErrors, nil
}

// NextTicket is the decision once no ticket is in the workflow's hands: the
// next pending ticket to dispatch, a run that is complete, or none and why.
// It may change GitHub as Decide does: close an issue whose pull request is
// merged, reopen one closed without its work landing, hand a ticket that
// cannot be used to a person.
func (a *Analyzer) NextTicket(ctx context.Context) (*Decision, error) {
	return a.pickTicket(ctx, 0)
}

// Candidates is up to max pending tickets that can start, in the order to
// start them, passing over those skip says to (nil passes over none); and
// what was decided past them or instead (Picks). It looks at the tickets as
// NextTicket does, and may change GitHub as it does, for the tickets it
// looks at: with max 1 and nothing passed over, its one ticket, or else its
// Then, is NextTicket's decision.
func (a *Analyzer) Candidates(ctx context.Context, max int, skip Skip) (Picks, error) {
	if max < 1 {
		return Picks{}, fmt.Errorf("candidates: max is %d; at least one ticket is asked for", max)
	}
	return a.candidates(ctx, 0, max, skip), nil
}

// pickTicket is steps 3 to 5; apiErrors counts the lookups that failed
// before it.
func (a *Analyzer) pickTicket(ctx context.Context, apiErrors int) (*Decision, error) {
	picks := a.candidates(ctx, apiErrors, 1, nil)
	if len(picks.Tickets) > 0 {
		return picks.Tickets[0], nil
	}
	return picks.Then, nil
}

// candidates is steps 3 to 5 for up to max tickets.
func (a *Analyzer) candidates(ctx context.Context, apiErrors, max int, skip Skip) Picks {
	labels := a.Config.GitHub.Labels

	// Step 3: Pick the next tickets among the pending ones: the lowest
	// numbers whose dependencies are done.
	lookup := a.newTicketLookup()
	pendingIssues, err := a.GHClient.ListPendingIssues(ctx, labels)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[analyzer] warning: failed to check pending issues: %v\n", err)
		apiErrors++
	}
	if err == nil {
		picks, lookupErrors := a.walk(ctx, pendingIssues, lookup, max, skip)
		apiErrors += lookupErrors
		if len(picks.Tickets) > 0 || picks.Then != nil || len(picks.Skipped) > 0 {
			return picks
		}
	}

	// Step 4: Nothing is left to dispatch. The run is complete once no task
	// issue is open, and no ticket was closed without its work landing.
	openCount, err := a.GHClient.CountOpenIssues(ctx, labels.Task)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[analyzer] warning: failed to count open issues: %v\n", err)
		apiErrors++
	}
	if err == nil && openCount == 0 {
		redo, err := a.falseCompletion(ctx, lookup)
		switch {
		case err != nil:
			fmt.Fprintf(os.Stderr, "[analyzer] warning: failed to check that the closed tickets landed: %v\n", err)
			apiErrors++
		case redo != nil:
			return Picks{Then: redo}
		default:
			return Picks{Then: &Decision{NextAction: ActionAllComplete}}
		}
	}

	// Step 5: No actionable tasks
	// Distinguish between "no tasks found" and "couldn't check due to API errors"
	if apiErrors > 0 {
		return Picks{Then: &Decision{NextAction: ActionNone, ExitReason: ReasonGitHubAPIError}}
	}
	return Picks{Then: &Decision{NextAction: ActionNone, ExitReason: ReasonNoActionableTasks}}
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
// PRNumber is the pull request of a ticket: the one its Worker's result
// names, or the one its body links; 0 when there is none.
func (a *Analyzer) PRNumber(issue int, body string) int {
	return a.extractPRNumberForIssue(issue, body)
}

func (a *Analyzer) extractPRNumberForIssue(issueNumber int, issueBody string) int {
	// Try result file first (most reliable: has pr_url from worker)
	resultFile := layout.Result(a.StateRoot, issueNumber)
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

// maxReviewAttempts returns the configured max review attempts, falling back to default.
func (a *Analyzer) maxReviewAttempts() int {
	if a.Config != nil && a.Config.Escalation.MaxReviewAttempts > 0 {
		return a.Config.Escalation.MaxReviewAttempts
	}
	return DefaultMaxReviewAttempts
}

// attemptFile is the state file tracking how many times a bounded action (kind,
// e.g. "review" or "conflict") has been attempted for a PR.
func (a *Analyzer) attemptFile(kind string, prNumber int) string {
	return filepath.Join(a.StateRoot, ".ai", "state", "attempts", kind+"-pr-"+strconv.Itoa(prNumber))
}

// getAttempts returns the recorded attempt count for (kind, PR).
func (a *Analyzer) getAttempts(kind string, prNumber int) int {
	if a.AttemptsOf != nil {
		if count, ok := a.AttemptsOf(kind, prNumber); ok {
			return count
		}
	}
	data, err := os.ReadFile(a.attemptFile(kind, prNumber))
	if err != nil {
		return 0
	}
	count, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	return count
}

// incrementAttempts increments the attempt count for (kind, PR). Returns an error
// if it cannot be persisted, so the caller can escalate rather than risk an
// unbounded retry loop.
func (a *Analyzer) incrementAttempts(kind string, prNumber int) error {
	if a.ReadOnly {
		return nil
	}
	count := a.getAttempts(kind, prNumber) + 1
	if err := writeFileAtomic(a.attemptFile(kind, prNumber), []byte(strconv.Itoa(count)), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "[analyzer] error: failed to persist %s attempt count for PR #%d: %v\n", kind, prNumber, err)
		return err
	}
	return nil
}

// getReviewAttempts returns the number of review attempts for a PR
func (a *Analyzer) getReviewAttempts(prNumber int) int {
	return a.getAttempts("review", prNumber)
}

// incrementReviewAttempts increments the review attempt count for a PR
// Returns error if the attempt count cannot be persisted (which could cause infinite retry loops)
func (a *Analyzer) incrementReviewAttempts(prNumber int) error {
	return a.incrementAttempts("review", prNumber)
}

// updateIssueLabels is a helper that logs warnings if label operations fail.
// Label operations are non-critical - we log warnings but don't fail the workflow.
func (a *Analyzer) updateIssueLabels(ctx context.Context, issueNumber int, addLabels, removeLabels []string) {
	if a.ReadOnly {
		return
	}
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
