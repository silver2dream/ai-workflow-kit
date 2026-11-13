package reviewer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/ghutil"
	"github.com/silver2dream/ai-workflow-kit/internal/lessons"
)

// A maintainer's review of a pull request is a correction, and a correction
// from a person is the strongest signal this workflow gets. It outranks the
// automated verdict — the pull request is not merged over it — and it is
// recorded for the learning loop as human evidence.
//
// Maintainer feedback is what an account with write access wrote through
// GitHub's review interface: the body of a review and comments on lines of
// the diff. Conversation comments are left for discussion.
//
// This file is about feedback on a pull request that is still open. The
// workflow does not wait for it — a pull request with no open feedback
// merges as soon as the automated review and CI pass — so much of it arrives
// after the merge; late_feedback.go takes that in.
//
// The maintainer and the workflow often share one GitHub account, so the
// author of a comment cannot tell them apart. The text can: everything AWK
// posts carries a marker.

// AddressedReplyBody is the reply the Worker posts under review comments
// after pushing a fix. It is AWK's own text, not maintainer feedback.
const AddressedReplyBody = "Addressed in the latest push. Please re-review."

// sourceHuman marks feedback log entries that record a maintainer's action.
const sourceHuman = "human"

// maxFeedbackItemChars caps one piece of maintainer feedback when quoted.
const maxFeedbackItemChars = 800

// humanFeedback is one piece of review feedback written by a maintainer.
type humanFeedback struct {
	Ref      string // "review:<id>" or "comment:<id>"; each is recorded once
	Author   string
	Commit   string // the commit it was written against
	Path     string // line comments only
	Line     int
	Body     string
	Resolved bool   // the maintainer resolved the conversation
	At       string // when it was published, RFC 3339
	Hunk     string // line comments only: the diff it is attached to, ending at its line
}

// prState is what the merge gate and the learning loop need to know about a
// pull request.
type prState struct {
	HeadSHA  string
	Merged   bool
	MergedAt string          // RFC 3339; "" while the pull request is not merged
	Feedback []humanFeedback // maintainer feedback, oldest first
	Ignored  int             // review text left out because of who wrote it
}

// ghFeedback is one review or line comment, as projected by the jq filters
// in fetchPRState.
type ghFeedback struct {
	ID          int64  `json:"id"`
	Login       string `json:"login"`
	UserType    string `json:"type"`
	Association string `json:"association"`
	State       string `json:"state"` // reviews only
	Body        string `json:"body"`
	Path        string `json:"path"` // line comments only
	Line        int    `json:"line"`
	Commit      string `json:"commit"`
	At          string `json:"at"`
	Hunk        string `json:"hunk"`   // line comments only
	ReviewID    int64  `json:"review"` // line comments only: the review it was submitted with
}

// awkGenerated reports whether review text was written by AWK itself.
func awkGenerated(body string) bool {
	return strings.Contains(body, "AWK Review") || strings.Contains(body, "<!-- AWK:") ||
		strings.TrimSpace(body) == AddressedReplyBody
}

// writeAccess reports whether a GitHub author association implies write
// access to the repository.
func writeAccess(association string) bool {
	switch strings.ToUpper(strings.TrimSpace(association)) {
	case "OWNER", "MEMBER", "COLLABORATOR":
		return true
	}
	return false
}

// selectHumanFeedback keeps the review text a maintainer wrote. Left out:
// bots, AWK's own text, empty bodies, reviews that are drafts, dismissed or
// plain approvals, and — counted in ignored — text from accounts without
// write access. viewerLogin is the account running the workflow ("" when
// unknown); resolved holds the ids of line comments whose conversation the
// maintainer resolved.
func selectHumanFeedback(reviews, comments []ghFeedback, viewerLogin string, resolved map[int64]bool) (items []humanFeedback, ignored int) {
	// A line comment drafted in a review is published when the review is
	// submitted, which can be long after it was written.
	submitted := make(map[int64]string, len(reviews))
	for _, r := range reviews {
		submitted[r.ID] = r.At
	}
	keep := func(kind string, f ghFeedback) {
		if strings.EqualFold(f.UserType, "Bot") || strings.TrimSpace(f.Body) == "" || awkGenerated(f.Body) {
			return
		}
		byViewer := viewerLogin != "" && strings.EqualFold(f.Login, viewerLogin)
		if !byViewer && !writeAccess(f.Association) {
			ignored++
			return
		}
		item := humanFeedback{
			Ref:    fmt.Sprintf("%s:%d", kind, f.ID),
			Author: f.Login,
			Commit: f.Commit,
			Path:   f.Path,
			Line:   f.Line,
			Body:   strings.TrimSpace(f.Body),
			At:     f.At,
		}
		if kind == "comment" {
			item.Resolved = resolved[f.ID]
			item.Hunk = f.Hunk
			item.At = laterStamp(f.At, submitted[f.ReviewID])
		}
		items = append(items, item)
	}
	for _, r := range reviews {
		switch strings.ToUpper(r.State) {
		case "PENDING", "DISMISSED", "APPROVED":
			continue
		}
		keep("review", r)
	}
	for _, c := range comments {
		keep("comment", c)
	}
	sort.SliceStable(items, func(a, b int) bool { return items[a].At < items[b].At })
	return items, ignored
}

// laterStamp returns the later of two GitHub timestamps. One that cannot be
// read loses to the other.
func laterStamp(a, b string) string {
	ta, errA := time.Parse(time.RFC3339, a)
	tb, errB := time.Parse(time.RFC3339, b)
	switch {
	case errB != nil:
		return a
	case errA != nil || tb.After(ta):
		return b
	}
	return a
}

// needsViewerLogin reports whether any text comes from an account whose
// association alone does not show write access (an app token, for instance),
// so that the viewer's own login is needed to vouch for it.
func needsViewerLogin(lists ...[]ghFeedback) bool {
	for _, list := range lists {
		for _, f := range list {
			if !writeAccess(f.Association) {
				return true
			}
		}
	}
	return false
}

// decodeFeedback reads the one-object-per-line output of `gh api --paginate --jq`.
func decodeFeedback(output []byte) ([]ghFeedback, error) {
	var out []ghFeedback
	dec := json.NewDecoder(bytes.NewReader(output))
	for dec.More() {
		var f ghFeedback
		if err := dec.Decode(&f); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

// jq projections of the REST review and review-comment objects. A line
// comment is tied to the commit the maintainer was looking at when writing
// it (original_commit_id); GitHub moves commit_id along with later pushes.
const (
	reviewsJQ  = `.[] | {id, login: .user.login, type: .user.type, association: .author_association, state, body, commit: .commit_id, at: .submitted_at}`
	commentsJQ = `.[] | {id, login: .user.login, type: .user.type, association: .author_association, body, path, line: (.original_line // .line), commit: (.original_commit_id // .commit_id), at: .created_at, hunk: .diff_hunk, review: .pull_request_review_id}`

	resolvedThreadsQuery = `query($owner: String!, $repo: String!, $number: Int!) { repository(owner: $owner, name: $repo) { pullRequest(number: $number) { reviewThreads(first: 100) { nodes { isResolved comments(first: 100) { nodes { databaseId } } } } } } }`
	resolvedThreadsJQ    = `.data.repository.pullRequest.reviewThreads.nodes[] | select(.isResolved) | .comments.nodes[].databaseId`
)

// fetchPRStateFunc reads a pull request's state and maintainer feedback from
// GitHub; replaced in tests.
var fetchPRStateFunc = fetchPRState

func fetchPRState(ctx context.Context, prNumber int, timeout time.Duration) (*prState, error) {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	gh := func(args ...string) ([]byte, error) {
		callCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return ghutil.RunWithRetry(callCtx, ghutil.DefaultRetryConfig(), "gh", args...)
	}
	pull := fmt.Sprintf("repos/{owner}/{repo}/pulls/%d", prNumber)

	out, err := gh("api", pull, "--jq", "{head: .head.sha, merged: .merged, merged_at: .merged_at}")
	if err != nil {
		return nil, err
	}
	var meta struct {
		Head     string `json:"head"`
		Merged   bool   `json:"merged"`
		MergedAt string `json:"merged_at"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out), &meta); err != nil {
		return nil, fmt.Errorf("pull request #%d: %w", prNumber, err)
	}

	out, err = gh("api", pull+"/reviews", "--paginate", "--jq", reviewsJQ)
	if err != nil {
		return nil, err
	}
	reviews, err := decodeFeedback(out)
	if err != nil {
		return nil, fmt.Errorf("reviews of pull request #%d: %w", prNumber, err)
	}

	out, err = gh("api", pull+"/comments", "--paginate", "--jq", commentsJQ)
	if err != nil {
		return nil, err
	}
	comments, err := decodeFeedback(out)
	if err != nil {
		return nil, fmt.Errorf("review comments of pull request #%d: %w", prNumber, err)
	}

	// The two lookups below refine the answer; without them every author is
	// judged by association alone and no conversation counts as resolved.
	viewer := ""
	if needsViewerLogin(reviews, comments) {
		if out, err := gh("api", "graphql", "-f", "query=query{viewer{login}}", "--jq", ".data.viewer.login"); err == nil {
			viewer = strings.TrimSpace(string(out))
		}
	}
	resolved := map[int64]bool{}
	if len(comments) > 0 {
		if out, err := gh("api", "graphql", "-F", "owner={owner}", "-F", "repo={repo}",
			"-F", "number="+strconv.Itoa(prNumber), "-f", "query="+resolvedThreadsQuery,
			"--jq", resolvedThreadsJQ); err == nil {
			for _, field := range strings.Fields(string(out)) {
				if id, err := strconv.ParseInt(field, 10, 64); err == nil {
					resolved[id] = true
				}
			}
		}
	}

	state := &prState{HeadSHA: meta.Head, Merged: meta.Merged, MergedAt: meta.MergedAt}
	state.Feedback, state.Ignored = selectHumanFeedback(reviews, comments, viewer, resolved)
	return state, nil
}

// humanFeedbackEnabled reports whether maintainer review feedback is read
// and acted on (review.human_feedback, default: true).
func humanFeedbackEnabled(stateRoot string) bool {
	cfg, err := analyzer.LoadConfig(filepath.Join(stateRoot, ".ai", "config", "workflow.yaml"))
	if err != nil {
		return true
	}
	return cfg.Review.HumanFeedbackEnabled()
}

// formatHumanFeedback renders maintainer feedback one item per line: who,
// where, what. Logins are written without "@" so that quoting them in a
// GitHub comment does not notify anyone.
func formatHumanFeedback(items []humanFeedback) string {
	var b strings.Builder
	for _, f := range items {
		where := "review"
		if f.Path != "" {
			where = f.Path
			if f.Line > 0 {
				where += ":" + strconv.Itoa(f.Line)
			}
		}
		body := truncateSummary(strings.Join(strings.Fields(f.Body), " "), maxFeedbackItemChars)
		fmt.Fprintf(&b, "- %s, %s: %s\n", f.Author, where, body)
	}
	return strings.TrimRight(b.String(), "\n")
}

// feedbackGroup is the maintainer feedback written against one commit.
type feedbackGroup struct {
	Commit string
	Items  []humanFeedback
}

// groupByCommit groups feedback by the commit it was written against, in
// order of first appearance.
func groupByCommit(items []humanFeedback) []feedbackGroup {
	var groups []feedbackGroup
	index := map[string]int{}
	for _, f := range items {
		i, ok := index[f.Commit]
		if !ok {
			i = len(groups)
			index[f.Commit] = i
			groups = append(groups, feedbackGroup{Commit: f.Commit})
		}
		groups[i].Items = append(groups[i].Items, f)
	}
	return groups
}

func feedbackRefs(items []humanFeedback) []string {
	refs := make([]string, len(items))
	for i, f := range items {
		refs[i] = f.Ref
	}
	return refs
}

// feedbackPaths lists the files the line comments are on, without repeats.
func feedbackPaths(items []humanFeedback) []string {
	var paths []string
	seen := map[string]bool{}
	for _, f := range items {
		if f.Path != "" && !seen[f.Path] && len(paths) < maxFeedbackPaths {
			seen[f.Path] = true
			paths = append(paths, f.Path)
		}
	}
	return paths
}

// recordedFeedbackRefs returns the maintainer feedback already in the
// feedback log for an issue, so each item is recorded once.
func recordedFeedbackRefs(stateRoot string, issueID int) map[string]bool {
	recorded := map[string]bool{}
	entries, err := LoadFeedback(stateRoot)
	if err != nil {
		return recorded
	}
	for _, e := range entries {
		if e.IssueID != issueID {
			continue
		}
		for _, ref := range e.Refs {
			recorded[ref] = true
		}
	}
	return recorded
}

// recordHumanFeedback writes maintainer feedback to the feedback log as
// human rejections, one entry per commit it was written against. This is the
// record for the learning loop only; it settles nothing.
func recordHumanFeedback(stateRoot string, issueID, prNumber int, items []humanFeedback) {
	for _, group := range groupByCommit(items) {
		entry := humanFeedbackEntry(issueID, prNumber, group.Items)
		entry.HeadSHA = group.Commit
		if err := RecordFeedback(stateRoot, entry); err != nil {
			fmt.Fprintf(os.Stderr, "[REVIEW] warning: could not record maintainer feedback: %v\n", err)
		}
	}
}

// humanFeedbackEntry builds the feedback log entry for maintainer feedback:
// a rejection that quotes it and carries the ids of the items it records.
func humanFeedbackEntry(issueID, prNumber int, items []humanFeedback) FeedbackEntry {
	text := formatHumanFeedback(items)
	entry := BuildFeedbackEntry(issueID, prNumber, 0, text)
	if len(text) > maxSummaryChars {
		entry.Findings = truncateSummary(text, maxFindingsChars)
	}
	entry.Source = sourceHuman
	entry.Refs = feedbackRefs(items)
	entry.Paths = feedbackPaths(items)
	return entry
}

// humanReview is the maintainer feedback found on the pull request under
// review, sorted for the merge gate.
type humanReview struct {
	// open was written against the pull request's current head and its
	// conversation is not resolved: nothing pushed since can have addressed it.
	open []humanFeedback
	// fresh is not in the feedback log yet.
	fresh []humanFeedback
}

// sortHumanFeedback splits a pull request's maintainer feedback for the gate.
func sortHumanFeedback(state *prState, recorded map[string]bool) humanReview {
	var h humanReview
	for _, f := range state.Feedback {
		if !recorded[f.Ref] {
			h.fresh = append(h.fresh, f)
		}
		if !f.Resolved && f.Commit != "" && f.Commit == state.HeadSHA {
			h.open = append(h.open, f)
		}
	}
	return h
}

// freshOpen is the open feedback the Worker has not been sent back with yet.
func (h humanReview) freshOpen() []humanFeedback {
	fresh := map[string]bool{}
	for _, f := range h.fresh {
		fresh[f.Ref] = true
	}
	var out []humanFeedback
	for _, f := range h.open {
		if fresh[f.Ref] {
			out = append(out, f)
		}
	}
	return out
}

// earlier is the fresh feedback that is not open: written against an earlier
// commit, or already resolved by the maintainer.
func (h humanReview) earlier() []humanFeedback {
	open := map[string]bool{}
	for _, f := range h.open {
		open[f.Ref] = true
	}
	var out []humanFeedback
	for _, f := range h.fresh {
		if !open[f.Ref] {
			out = append(out, f)
		}
	}
	return out
}

// loadHumanReview reads the maintainer feedback on the pull request under
// review. A failure to read it is reported and treated as "none": the gate is
// an added safeguard and must not stop the workflow when GitHub hiccups.
func loadHumanReview(ctx context.Context, opts SubmitReviewOptions) humanReview {
	if !humanFeedbackEnabled(opts.StateRoot) {
		return humanReview{}
	}
	state, err := fetchPRStateFunc(ctx, opts.PRNumber, opts.GHTimeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[REVIEW] warning: could not read maintainer review feedback on PR #%d: %v\n", opts.PRNumber, err)
		return humanReview{}
	}
	if state.Ignored > 0 {
		fmt.Fprintf(os.Stderr, "[REVIEW] ignored %d review comment(s) on PR #%d from accounts without write access\n", state.Ignored, opts.PRNumber)
	}
	return sortHumanFeedback(state, recordedFeedbackRefs(opts.StateRoot, opts.IssueNumber))
}

// Gate decisions for a review the automated reviewer would approve.
const (
	gateProceed  = "proceed"   // nothing open: approve and merge as usual
	gateSendBack = "send_back" // open feedback the Worker has not seen through the gate yet
	gateEscalate = "escalate"  // open feedback the Worker was already sent back with, and nothing was pushed
)

// decide says what to do with a pull request the automated review would
// approve. Open feedback always wins over the approval; whether the Worker
// gets another round depends on whether it already had one for this feedback.
func (h humanReview) decide() string {
	switch {
	case len(h.open) == 0:
		return gateProceed
	case len(h.freshOpen()) > 0:
		return gateSendBack
	default:
		return gateEscalate
	}
}

// applyHumanGate reads the maintainer feedback on the pull request under
// review, records what was written against earlier commits for the learning
// loop, and — when the automated review would approve — holds the pull
// request if feedback is still open on the current commit. held is the
// result to return instead of approving; nil means carry on.
//
// When the automated review rejects, nothing is held: the Worker is sent
// back anyway, and sectionForRejection puts the open feedback in that round.
func applyHumanGate(ctx context.Context, opts SubmitReviewOptions) (human humanReview, held *SubmitReviewResult) {
	human = loadHumanReview(ctx, opts)
	recordHumanFeedback(opts.StateRoot, opts.IssueNumber, opts.PRNumber, human.earlier())
	if opts.Score < opts.ScoreThreshold {
		return human, nil
	}
	switch human.decide() {
	case gateSendBack:
		return human, sendBackForHumanFeedback(ctx, opts, human)
	case gateEscalate:
		return human, escalateOpenHumanFeedback(ctx, opts, human)
	}
	return human, nil
}

// sectionForRejection is the part of the rejection comment that carries the
// open maintainer feedback to the Worker ("" when there is none). The
// feedback not yet in the log is recorded on the way.
func (h humanReview) sectionForRejection(opts SubmitReviewOptions) string {
	if len(h.open) == 0 {
		return ""
	}
	recordHumanFeedback(opts.StateRoot, opts.IssueNumber, opts.PRNumber, h.freshOpen())
	return "### 維護者的審查意見（優先處理）\n\n" + formatHumanFeedback(h.open) + "\n\n"
}

// Side effects of the gate; replaced in tests so no test edits a real issue.
var (
	labelIssueFunc   = editIssueLabels
	commentIssueFunc = postIssueComment
)

// sendBackForHumanFeedback hands open maintainer feedback to the Worker: the
// issue goes back to the task queue with the feedback quoted in a comment the
// Worker prompt picks up.
func sendBackForHumanFeedback(ctx context.Context, opts SubmitReviewOptions, h humanReview) *SubmitReviewResult {
	fmt.Printf("[REVIEW] Maintainer review feedback is open on the current commit — not merging, sending the Worker back\n")

	if err := labelIssueFunc(ctx, opts.IssueNumber, []string{"ai-task"}, []string{"pr-ready"}, opts.GHTimeout); err != nil {
		fmt.Fprintf(os.Stderr, "[REVIEW] warning: failed to edit issue labels: %v\n", err)
	}
	comment := fmt.Sprintf(`## AWK Review: 維護者的審查意見待處理

PR #%d 目前的 commit 上有維護者留下的審查意見。它們優先於自動審查的結論，所以這個 PR 不會合併。

%s

---
**Worker 請根據以上意見修改後重新提交。**
PR: #%d`, opts.PRNumber, formatHumanFeedback(h.open), opts.PRNumber)
	if err := commentIssueFunc(ctx, opts.IssueNumber, comment, opts.GHTimeout); err != nil {
		fmt.Fprintf(os.Stderr, "[REVIEW] warning: failed to post issue comment: %v\n", err)
	}

	fresh := h.freshOpen()
	closeLearningLoop(ctx, opts, learningOutcome{
		Outcome:  lessons.OutcomeChangesRequested,
		Findings: formatHumanFeedback(fresh),
		Source:   sourceHuman,
		Refs:     feedbackRefs(fresh),
	})
	return &SubmitReviewResult{Result: "changes_requested", Reason: "maintainer review feedback is open on the current commit"}
}

// escalateOpenHumanFeedback stops short of merging when the Worker was
// already sent back with the open feedback and pushed nothing: another round
// would repeat itself, and merging would overrule the maintainer.
func escalateOpenHumanFeedback(ctx context.Context, opts SubmitReviewOptions, h humanReview) *SubmitReviewResult {
	fmt.Printf("[REVIEW] Maintainer review feedback is still open and nothing was pushed since — needs a person\n")

	if err := labelIssueFunc(ctx, opts.IssueNumber, []string{"needs-human-review"}, []string{"pr-ready"}, opts.GHTimeout); err != nil {
		fmt.Fprintf(os.Stderr, "[REVIEW] warning: failed to edit issue labels: %v\n", err)
	}
	comment := fmt.Sprintf(`## AWK Review: 維護者的審查意見仍未處理（需要人工介入）

PR #%d 目前的 commit 上仍有維護者的審查意見，而 Worker 上一輪沒有為它推送任何修改。AWK 不會在這個狀態下合併。

%s

請擇一處理，然後移除 needs-human-review 標籤：
- 補充說明（在 PR 上再留一則審查意見），Worker 會再處理一輪
- 把該則對話標為已解決（Resolve conversation）或撤銷該次 review，AWK 會照自動審查的結論繼續
- 自行推送修正，或直接合併

PR: #%d`, opts.PRNumber, formatHumanFeedback(h.open), opts.PRNumber)
	if err := commentIssueFunc(ctx, opts.IssueNumber, comment, opts.GHTimeout); err != nil {
		fmt.Fprintf(os.Stderr, "[REVIEW] warning: failed to post issue comment: %v\n", err)
	}
	return &SubmitReviewResult{Result: "merge_failed", Reason: "maintainer review feedback is still open on the current commit"}
}

// maintainerFeedbackForReviewer renders a pull request's maintainer feedback
// for the review context: every item that is not resolved, with the commit it
// was written against, so the reviewer can check it against the current code.
func maintainerFeedbackForReviewer(state *prState) string {
	var b strings.Builder
	for _, group := range groupByCommit(state.Feedback) {
		var open []humanFeedback
		for _, f := range group.Items {
			if !f.Resolved {
				open = append(open, f)
			}
		}
		if len(open) == 0 {
			continue
		}
		commit := group.Commit
		if len(commit) > 7 {
			commit = commit[:7]
		}
		when := "an earlier commit"
		if group.Commit != "" && group.Commit == state.HeadSHA {
			when = "the current commit"
		}
		fmt.Fprintf(&b, "Written on %s (%s):\n%s\n\n", when, commit, formatHumanFeedback(open))
	}
	return strings.TrimRight(b.String(), "\n")
}

// fetchPRHeadFunc downloads a pull request's head commit so its changes can
// be compared locally; replaced in tests.
var fetchPRHeadFunc = fetchPRHead

func fetchPRHead(stateRoot string, prNumber int) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	// Best effort: without the objects the case is distilled from the
	// objections alone.
	_ = exec.CommandContext(ctx, "git", "-C", stateRoot, "fetch", "-q", "origin",
		fmt.Sprintf("pull/%d/head", prNumber)).Run()
}
