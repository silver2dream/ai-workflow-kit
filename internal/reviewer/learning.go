package reviewer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/lessons"
)

// learningOutcome is what the learning loop records about one terminal
// review outcome.
type learningOutcome struct {
	// Outcome is lessons.OutcomeMerged, OutcomeChangesRequested or
	// OutcomeReviewBlocked.
	Outcome string
	// Categories is set for failures the system detected itself (CI,
	// verification, JiT tests). When nil they are read from Findings.
	Categories []string
	// Findings is what was objected to; empty for a merge.
	Findings string
}

// Size limits of a rejection reason in the feedback log.
const (
	maxSummaryChars  = 500
	maxFindingsChars = 2000
)

// closeLearningLoop records a terminal review outcome in the feedback log,
// settles the lessons that were in the Worker's prompt and, when the outcome
// is a merge, distills the cases the merge resolved. Every step is
// best-effort and none of them affects the review verdict.
func closeLearningLoop(ctx context.Context, opts SubmitReviewOptions, o learningOutcome) {
	merged := o.Outcome == lessons.OutcomeMerged
	findings := strings.TrimSpace(o.Findings)

	// The pull request's file list anchors the feedback entry to a scope and
	// tells settlement which lessons applied to this pull request.
	var changed []string
	diff, diffErr := fetchPRDiffFunc(ctx, opts.PRNumber, opts.GHTimeout)
	pathsKnown := diffErr == nil
	if pathsKnown {
		changed = ChangedPaths(diff)
	}

	var entry FeedbackEntry
	if merged {
		entry = FeedbackEntry{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			IssueID:   opts.IssueNumber,
			PRNumber:  opts.PRNumber,
			Score:     opts.Score,
			Outcome:   "approved",
		}
	} else {
		entry = BuildFeedbackEntry(opts.IssueNumber, opts.PRNumber, opts.Score, findings)
		if o.Categories != nil {
			entry.Categories = o.Categories
		}
		if len(findings) > maxSummaryChars {
			entry.Findings = truncateSummary(findings, maxFindingsChars)
		}
		// Read before settlement: which lessons the rejected attempt had.
		if rec := lessons.LoadInjection(opts.StateRoot, opts.IssueNumber); rec != nil {
			entry.InjectedLessons = rec.LessonIDs
		}
	}
	entry.HeadSHA = opts.headSHA
	entry.Paths = changed
	if len(entry.Paths) > maxFeedbackPaths {
		entry.Paths = entry.Paths[:maxFeedbackPaths]
	}
	_ = RecordFeedback(opts.StateRoot, entry)

	report, err := lessons.Settle(opts.StateRoot, lessons.SettleInput{
		IssueID:       opts.IssueNumber,
		Outcome:       o.Outcome,
		RejectionText: findings,
		ChangedPaths:  changed,
		PathsKnown:    pathsKnown,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "[REVIEW] warning: lesson settlement failed: %v\n", err)
	} else if len(report.Hits)+len(report.Misses)+len(report.NoSignal) > 0 {
		fmt.Printf("[REVIEW] Lessons settled: %d hit, %d miss, %d no signal\n",
			len(report.Hits), len(report.Misses), len(report.NoSignal))
		if !pathsKnown {
			fmt.Fprintf(os.Stderr, "[REVIEW] warning: PR file list unavailable (%v); lessons with a scope were not counted\n", diffErr)
		}
	}

	if !merged {
		// A rejection is distilled together with the approval that resolves
		// it: until then there is no accepted version to compare it with.
		return
	}

	cfg, err := analyzer.LoadConfig(filepath.Join(opts.StateRoot, ".ai", "config", "workflow.yaml"))
	if err != nil || !cfg.Lessons.IsEnabled() {
		return
	}
	timeout := 60 * time.Second
	if cfg.Lessons.Distiller.TimeoutSeconds > 0 {
		timeout = time.Duration(cfg.Lessons.Distiller.TimeoutSeconds) * time.Second
	}
	distilled, err := lessons.Distill(context.Background(), opts.StateRoot, lessons.DistillOptions{
		Model:      cfg.Lessons.Distiller.Model,
		Timeout:    timeout,
		MaxActive:  cfg.Lessons.MaxActive,
		MaxEntries: 3,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "[REVIEW] warning: lesson distillation failed: %v\n", err)
		return
	}
	if distilled.Processed > 0 {
		fmt.Printf("[REVIEW] Lessons: %d case(s) distilled (%d new, %d upvoted, %d skipped)\n",
			distilled.Processed, len(distilled.Created), len(distilled.Matched), distilled.Skipped)
	}
}

// rejectionFindings is what the reviewer objected to: the score reason and
// the findings. The rendered review opens with a walkthrough of every
// acceptance criterion, so its first lines do not say why it was rejected.
func rejectionFindings(opts SubmitReviewOptions) string {
	var parts []string
	if r := opts.Structured; r != nil {
		if summary := strings.TrimSpace(r.Summary); summary != "" {
			parts = append(parts, summary)
		}
		parts = append(parts, findingLines(r.Improvements)...)
	} else if sections := markdownSections(opts.ReviewBody, "Score Reason", "Suggested Improvements"); sections != "" {
		parts = append(parts, sections)
	}
	// A multi-model consensus can reject what the primary reviewer approved;
	// the objections are then in its section of the body.
	if i := strings.Index(opts.ReviewBody, consensusHeading); i >= 0 {
		parts = append(parts, strings.TrimSpace(opts.ReviewBody[i:]))
	}
	if len(parts) == 0 {
		return strings.TrimSpace(opts.ReviewBody)
	}
	return strings.Join(parts, "\n")
}

// findingLines renders a structured review's findings one per line. When
// there are blocking findings (critical, important) only those are returned:
// they are why a pull request is rejected, the rest is advice.
func findingLines(improvements []StructuredImprovement) []string {
	var blocking, advice []string
	for _, imp := range improvements {
		severity := strings.ToLower(strings.TrimSpace(imp.Severity))
		line := "- " + severityDisplay[severity] + ": "
		if location := strings.TrimSpace(imp.Location); location != "" {
			line += location + " — "
		}
		line += strings.TrimSpace(imp.Text)
		if severity == severityCritical || severity == severityImportant {
			blocking = append(blocking, line)
		} else {
			advice = append(advice, line)
		}
	}
	if len(blocking) > 0 {
		return blocking
	}
	return advice
}

// markdownSections returns the text under the named headings of a review
// written as markdown, in document order.
func markdownSections(body string, titles ...string) string {
	var lines []string
	keep := false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			heading := strings.ToLower(strings.TrimSpace(strings.TrimLeft(trimmed, "#")))
			keep = false
			for _, title := range titles {
				if strings.HasPrefix(heading, strings.ToLower(title)) {
					keep = true
					break
				}
			}
			continue
		}
		if keep && trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return strings.Join(lines, "\n")
}

// worktreeHeadFunc returns the commit checked out in a review worktree;
// replaced in tests.
var worktreeHeadFunc = worktreeHead

// worktreeHead returns the commit checked out in the worktree — the version
// a review judges — or "" when the path is not the root of a checkout.
func worktreeHead(worktreePath string) string {
	if worktreePath == "" {
		return ""
	}
	// A worktree root has a .git entry. Without this check git would answer
	// from whatever repository encloses the directory.
	if _, err := os.Stat(filepath.Join(worktreePath, ".git")); err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", worktreePath, "rev-parse", "--verify", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
