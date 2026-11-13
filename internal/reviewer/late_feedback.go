package reviewer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/config"
	"github.com/silver2dream/ai-workflow-kit/internal/ghutil"
	"github.com/silver2dream/ai-workflow-kit/internal/lessons"
)

// The workflow merges a pull request as soon as the automated review and CI
// pass. It never waits for a person, so a maintainer usually reads a pull
// request after it was merged. What they write then is still a correction.
// It is recorded in the feedback log as a case of its own (after_merge) and
// distilled before the next Worker is dispatched, so that Worker already
// works from it.
//
// The merged code is left alone: nothing is reopened and no ticket is made.
// Feedback on a pull request that is still open is the merge gate's business
// (human_feedback.go).

const (
	// lateScanFile remembers the update stamp GitHub showed for each merged
	// pull request when its review feedback was last read. A pull request is
	// read again only when that stamp has changed.
	lateScanFile = ".ai/state/late_feedback_scan.json"

	maxLateListing = 30 // closed pull requests listed per sync, most recently updated first
	maxLateReads   = 10 // pull requests read in full per sync

	maxContextChars = 3000 // code quoted in one feedback entry
	maxHunkLines    = 12   // lines kept of the diff hunk a comment is attached to
	maxHunkLineLen  = 200

	// maxSyncDuration bounds the GitHub lookups made before a dispatch.
	maxSyncDuration = 2 * time.Minute
)

// workerBranchRe matches the branch a Worker opens its pull request from and
// captures the issue number. The branch name is what ties a pull request to
// its issue, in whichever repository of the project it was opened.
var workerBranchRe = regexp.MustCompile(`^feat/ai-issue-(\d+)$`)

// workerIssue returns the issue a Worker's branch belongs to, or 0 for any
// other branch.
func workerIssue(branch string) int {
	m := workerBranchRe.FindStringSubmatch(branch)
	if m == nil {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	return n
}

// closedPR is one row of the listing of recently updated pull requests.
type closedPR struct {
	Number  int    `json:"number"`
	Updated string `json:"updated"` // GitHub's updated_at
	Merged  bool   `json:"merged"`
	Branch  string `json:"branch"`
}

const closedPRsJQ = `.[] | {number, updated: .updated_at, merged: (.merged_at != null), branch: .head.ref}`

// listClosedPRsFunc lists the closed pull requests GitHub updated most
// recently; replaced in tests.
var listClosedPRsFunc = listClosedPRs

func listClosedPRs(ctx context.Context, timeout time.Duration) ([]closedPR, error) {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	endpoint := fmt.Sprintf("repos/{owner}/{repo}/pulls?state=closed&sort=updated&direction=desc&per_page=%d", maxLateListing)
	out, err := ghutil.RunWithRetry(ctx, ghutil.DefaultRetryConfig(), "gh", "api", endpoint, "--jq", closedPRsJQ)
	if err != nil {
		return nil, err
	}
	return decodeClosedPRs(out)
}

// decodeClosedPRs reads the one-object-per-line output of the listing.
func decodeClosedPRs(output []byte) ([]closedPR, error) {
	var prs []closedPR
	dec := json.NewDecoder(bytes.NewReader(output))
	for dec.More() {
		var pr closedPR
		if err := dec.Decode(&pr); err != nil {
			return nil, fmt.Errorf("closed pull requests: %w", err)
		}
		prs = append(prs, pr)
	}
	return prs, nil
}

// lateScan is the content of lateScanFile.
type lateScan struct {
	// Seen maps a pull request number to its update stamp at the last read.
	Seen map[string]string `json:"seen"`
}

// loadLateScan returns the stamps of the last sync. A missing or unreadable
// file means nothing was read yet.
func loadLateScan(stateRoot string) map[string]string {
	data, err := os.ReadFile(filepath.Join(stateRoot, filepath.FromSlash(lateScanFile)))
	if err != nil {
		return map[string]string{}
	}
	var scan lateScan
	if err := json.Unmarshal(data, &scan); err != nil || scan.Seen == nil {
		return map[string]string{}
	}
	return scan.Seen
}

func saveLateScan(stateRoot string, seen map[string]string) {
	path := filepath.Join(stateRoot, filepath.FromSlash(lateScanFile))
	data, err := json.MarshalIndent(lateScan{Seen: seen}, "", "  ")
	if err == nil {
		if err = os.MkdirAll(filepath.Dir(path), 0755); err == nil {
			err = os.WriteFile(path, append(data, '\n'), 0644)
		}
	}
	if err != nil {
		// Without the stamps the next sync reads the same pull requests
		// again; every item is still recorded only once.
		fmt.Fprintf(os.Stderr, "[REVIEW] warning: could not save %s: %v\n", lateScanFile, err)
	}
}

// mergedIssues returns the issues the feedback log has a merge of.
func mergedIssues(stateRoot string) map[int]bool {
	merged := map[int]bool{}
	entries, err := LoadFeedback(stateRoot)
	if err != nil {
		return merged
	}
	for _, e := range entries {
		if e.Outcome == outcomeApproved {
			merged[e.IssueID] = true
		}
	}
	return merged
}

// stampAfter reports whether one GitHub timestamp is later than another.
// Both come from GitHub's clock, so the local clock plays no part. A stamp
// that cannot be read is never "after".
func stampAfter(at, ref string) bool {
	a, errA := time.Parse(time.RFC3339, at)
	r, errR := time.Parse(time.RFC3339, ref)
	return errA == nil && errR == nil && a.After(r)
}

// lateFeedback picks the maintainer feedback that was published after the
// pull request was merged and is not in the feedback log yet.
//
// Feedback published before the merge is not picked up here even when the
// log lacks it: whether the Worker went on to address it cannot be told any
// more. A conversation the maintainer resolved is left out too; on a merged
// pull request that is how a remark is taken back. A pull request that is
// not merged has no merge time, so nothing on it is late.
func lateFeedback(state *prState, recorded map[string]bool) []humanFeedback {
	var late []humanFeedback
	for _, f := range state.Feedback {
		if recorded[f.Ref] || f.Resolved || !stampAfter(f.At, state.MergedAt) {
			continue
		}
		late = append(late, f)
	}
	return late
}

// hunkTail returns the last lines of the diff hunk a comment is attached to:
// the commented line and what leads up to it.
func hunkTail(hunk string) string {
	hunk = strings.TrimRight(strings.ReplaceAll(hunk, "\r\n", "\n"), "\n")
	if strings.TrimSpace(hunk) == "" {
		return ""
	}
	lines := strings.Split(hunk, "\n")
	if len(lines) > maxHunkLines {
		lines = lines[len(lines)-maxHunkLines:]
	}
	for i, line := range lines {
		lines[i] = truncateSummary(line, maxHunkLineLen)
	}
	return strings.Join(lines, "\n")
}

// feedbackContext renders the code the line comments were written on, one
// block per comment: where it is, then the end of its diff hunk. That is
// what the maintainer was looking at.
func feedbackContext(items []humanFeedback) string {
	var b strings.Builder
	for _, f := range items {
		code := hunkTail(f.Hunk)
		if code == "" {
			continue
		}
		where := f.Path
		if f.Line > 0 {
			where += ":" + strconv.Itoa(f.Line)
		}
		block := where + "\n" + code + "\n\n"
		if b.Len()+len(block) > maxContextChars {
			break
		}
		b.WriteString(block)
	}
	return strings.TrimRight(b.String(), "\n")
}

// SyncLateFeedback records the review feedback maintainers published on the
// workflow's pull requests after they were merged, and returns how many
// items it recorded. Each item is recorded once.
//
// One listing call finds the pull requests worth reading: merged, opened
// from a Worker's branch for an issue this workflow merged, and updated on
// GitHub since they were last read.
func SyncLateFeedback(ctx context.Context, stateRoot string, ghTimeout time.Duration) int {
	if !humanFeedbackEnabled(stateRoot) {
		return 0
	}
	merged := mergedIssues(stateRoot)
	if len(merged) == 0 {
		return 0
	}
	listing, err := listClosedPRsFunc(ctx, ghTimeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[REVIEW] warning: could not list merged pull requests to look for review feedback: %v\n", err)
		return 0
	}

	seen := loadLateScan(stateRoot)
	next := make(map[string]string, len(listing))
	recorded, reads := 0, 0
	for _, pr := range listing {
		issue := workerIssue(pr.Branch)
		if !pr.Merged || issue == 0 || !merged[issue] {
			continue
		}
		key := strconv.Itoa(pr.Number)
		// keepStamp leaves the pull request as it was, so a later sync
		// decides about it again.
		keepStamp := func() {
			if stamp, ok := seen[key]; ok {
				next[key] = stamp
			}
		}
		if stamp, ok := seen[key]; ok && stamp == pr.Updated {
			next[key] = stamp // nothing happened on it since it was read
			continue
		}
		if reads >= maxLateReads {
			keepStamp()
			continue
		}
		reads++
		state, err := fetchPRStateFunc(ctx, pr.Number, ghTimeout)
		if err != nil {
			keepStamp()
			continue
		}
		late := lateFeedback(state, recordedFeedbackRefs(stateRoot, issue))
		if len(late) > 0 {
			entry := humanFeedbackEntry(issue, pr.Number, late)
			entry.AfterMerge = true
			entry.HeadSHA = state.HeadSHA
			entry.Context = feedbackContext(late)
			if err := RecordFeedback(stateRoot, entry); err != nil {
				fmt.Fprintf(os.Stderr, "[REVIEW] warning: could not record maintainer feedback on merged PR #%d: %v\n", pr.Number, err)
				keepStamp()
				continue
			}
			recorded += len(late)
		}
		next[key] = pr.Updated
	}
	saveLateScan(stateRoot, next)
	return recorded
}

// SyncMaintainerActivity brings the feedback log up to date with what
// maintainers did on GitHub outside the workflow's own review rounds: pull
// requests they merged themselves, and review feedback they published on
// pull requests that were already merged.
func SyncMaintainerActivity(ctx context.Context, stateRoot string, ghTimeout time.Duration) (handMerged, lateItems int) {
	handMerged = SyncHumanResolutions(ctx, stateRoot, ghTimeout)
	lateItems = SyncLateFeedback(ctx, stateRoot, ghTimeout)
	return handMerged, lateItems
}

// distillOptions translates the lessons configuration into the options of a
// distillation run.
func distillOptions(cfg analyzer.LessonsConfig, maxEntries int) lessons.DistillOptions {
	timeout := 60 * time.Second
	if cfg.Distiller.TimeoutSeconds > 0 {
		timeout = time.Duration(cfg.Distiller.TimeoutSeconds) * time.Second
	}
	return lessons.DistillOptions{
		Model:      cfg.Distiller.Model,
		Timeout:    timeout,
		MaxActive:  cfg.MaxActive,
		MaxEntries: maxEntries,
	}
}

// LearnBeforeDispatch takes in what maintainers did since the workflow last
// looked and distills it, so the Worker about to be dispatched already works
// from it. This is how a correction made at any time — also after the merge
// it is about — reaches the workflow without the workflow waiting for anyone.
//
// Best effort: progress goes to log (the caller's stdout is parsed), and no
// failure here stops the dispatch.
func LearnBeforeDispatch(ctx context.Context, stateRoot string, ghTimeout time.Duration, log io.Writer) {
	cfg, err := analyzer.LoadConfig(config.Path(stateRoot))
	if err != nil || !cfg.Lessons.IsEnabled() {
		return
	}

	syncCtx, cancel := context.WithTimeout(ctx, maxSyncDuration)
	handMerged, lateItems := SyncMaintainerActivity(syncCtx, stateRoot, ghTimeout)
	cancel()
	if handMerged+lateItems == 0 {
		return
	}
	fmt.Fprintf(log, "[LESSONS] Maintainers since the last look: %d pull request(s) merged by hand, %d review item(s) on merged pull requests\n", handMerged, lateItems)

	report, err := lessons.Distill(ctx, stateRoot, distillOptions(cfg.Lessons, 3))
	if err != nil {
		fmt.Fprintf(log, "[LESSONS] warning: lesson distillation failed: %v\n", err)
		return
	}
	fmt.Fprintf(log, "[LESSONS] %d case(s) distilled (%d new, %d upvoted, %d skipped)\n",
		report.Processed, len(report.Created), len(report.Matched), report.Skipped)
}
