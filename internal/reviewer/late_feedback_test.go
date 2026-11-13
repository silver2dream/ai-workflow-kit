package reviewer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/lessons"
)

const (
	mergedAt    = "2026-11-13T10:00:00Z"
	beforeMerge = "2026-11-13T09:30:00Z"
	afterMerge  = "2026-11-13T18:45:00Z"
)

// ---------------------------------------------------------------------------
// Which pull requests and which feedback count
// ---------------------------------------------------------------------------

func TestWorkerIssue(t *testing.T) {
	tests := map[string]int{
		"feat/ai-issue-42":                   42,
		"feat/ai-issue-7":                    7,
		"feat/ai-issue-":                     0,
		"feat/ai-issue-42-retry":             0,
		"hotfix/feat/ai-issue-42":            0,
		"feature/login":                      0,
		"":                                   0,
		"feat/ai-issue-99999999999999999999": 0, // not a number an issue can have
	}
	for branch, want := range tests {
		if got := workerIssue(branch); got != want {
			t.Errorf("workerIssue(%q) = %d, want %d", branch, got, want)
		}
	}
}

func TestStampAfter(t *testing.T) {
	if !stampAfter(afterMerge, mergedAt) {
		t.Error("a later stamp is after")
	}
	if stampAfter(beforeMerge, mergedAt) || stampAfter(mergedAt, mergedAt) {
		t.Error("an earlier or equal stamp is not after")
	}
	// Different offsets, same instant order: compared as times, not as text.
	if !stampAfter("2026-11-13T11:00:00+02:00", "2026-11-13T08:59:59Z") || stampAfter("2026-11-13T11:00:00+02:00", "2026-11-13T09:00:01Z") {
		t.Error("stamps must be compared as points in time")
	}
	for _, bad := range [][2]string{{"", mergedAt}, {afterMerge, ""}, {"yesterday", mergedAt}} {
		if stampAfter(bad[0], bad[1]) {
			t.Errorf("stampAfter(%q, %q) = true; an unreadable stamp is never after", bad[0], bad[1])
		}
	}
}

func TestLateFeedback(t *testing.T) {
	early := humanFeedback{Ref: "comment:1", Body: "before the merge", At: beforeMerge}
	late := humanFeedback{Ref: "comment:2", Body: "after the merge", At: afterMerge}
	lateReview := humanFeedback{Ref: "review:3", Body: "also after", At: afterMerge}
	withdrawn := humanFeedback{Ref: "comment:4", Body: "never mind", At: afterMerge, Resolved: true}
	known := humanFeedback{Ref: "comment:5", Body: "already in the log", At: afterMerge}
	undated := humanFeedback{Ref: "comment:6", Body: "no date"}

	state := &prState{Merged: true, MergedAt: mergedAt, Feedback: []humanFeedback{early, late, lateReview, withdrawn, known, undated}}
	got := feedbackRefs(lateFeedback(state, map[string]bool{"comment:5": true}))
	if want := []string{"comment:2", "review:3"}; !reflect.DeepEqual(got, want) {
		t.Errorf("late feedback = %v, want %v", got, want)
	}

	open := &prState{Merged: false, Feedback: []humanFeedback{late}}
	if got := lateFeedback(open, nil); len(got) != 0 {
		t.Errorf("a pull request that is not merged has no late feedback, got %v", feedbackRefs(got))
	}
	unknownMerge := &prState{Merged: true, MergedAt: "", Feedback: []humanFeedback{late}}
	if got := lateFeedback(unknownMerge, nil); len(got) != 0 {
		t.Errorf("without the merge time nothing can be told to be late, got %v", feedbackRefs(got))
	}
}

func TestSelectHumanFeedback_LineCommentIsPublishedWithItsReview(t *testing.T) {
	reviews := []ghFeedback{
		{ID: 900, Login: "maintainer", UserType: "User", Association: "OWNER", State: "COMMENTED", Body: "", Commit: commitHead, At: afterMerge},
		{ID: 901, Login: "maintainer", UserType: "User", Association: "OWNER", State: "PENDING", Body: "", Commit: commitHead},
	}
	comments := []ghFeedback{
		// Drafted before the merge, submitted with review 900 after it.
		{ID: 11, Login: "maintainer", UserType: "User", Association: "OWNER", Body: "Wrap the error.", Path: "a.go", Line: 9,
			Commit: commitHead, At: beforeMerge, ReviewID: 900, Hunk: "@@ -1,2 +1,3 @@\n context\n+added"},
		// A single comment: published when it was written.
		{ID: 12, Login: "maintainer", UserType: "User", Association: "OWNER", Body: "Rename tmp.", Path: "a.go", Line: 3, Commit: commitHead, At: beforeMerge},
		// Its review has no submission time: the comment's own time stands.
		{ID: 13, Login: "maintainer", UserType: "User", Association: "OWNER", Body: "Why?", Path: "a.go", Line: 4, Commit: commitHead, At: beforeMerge, ReviewID: 901},
	}

	items, _ := selectHumanFeedback(reviews, comments, "", nil)

	byRef := map[string]humanFeedback{}
	for _, f := range items {
		byRef[f.Ref] = f
	}
	if got := byRef["comment:11"]; got.At != afterMerge || got.Hunk != "@@ -1,2 +1,3 @@\n context\n+added" {
		t.Errorf("comment 11 = %+v, want it published with its review and carrying its hunk", got)
	}
	if got := byRef["comment:12"].At; got != beforeMerge {
		t.Errorf("comment 12 published at %q, want its own time %q", got, beforeMerge)
	}
	if got := byRef["comment:13"].At; got != beforeMerge {
		t.Errorf("comment 13 published at %q, want its own time %q", got, beforeMerge)
	}
	// Oldest first, by publication.
	if last := items[len(items)-1].Ref; last != "comment:11" {
		t.Errorf("last item = %s, want the one published last (comment:11)", last)
	}
}

func TestLaterStamp(t *testing.T) {
	if got := laterStamp(beforeMerge, afterMerge); got != afterMerge {
		t.Errorf("got %q, want the later one", got)
	}
	if got := laterStamp(afterMerge, beforeMerge); got != afterMerge {
		t.Errorf("got %q, want the later one", got)
	}
	if got := laterStamp(beforeMerge, ""); got != beforeMerge {
		t.Errorf("got %q: an unreadable stamp loses", got)
	}
	if got := laterStamp("", afterMerge); got != afterMerge {
		t.Errorf("got %q: an unreadable stamp loses", got)
	}
}

func TestDecodeClosedPRs(t *testing.T) {
	out := []byte(`{"number":70,"updated":"2026-11-13T18:45:10Z","merged":true,"branch":"feat/ai-issue-7"}
{"number":71,"updated":"2026-11-13T12:00:00Z","merged":false,"branch":"feature/x"}
`)
	got, err := decodeClosedPRs(out)
	want := []closedPR{
		{Number: 70, Updated: "2026-11-13T18:45:10Z", Merged: true, Branch: "feat/ai-issue-7"},
		{Number: 71, Updated: "2026-11-13T12:00:00Z", Merged: false, Branch: "feature/x"},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("decoded %+v (err %v), want %+v", got, err, want)
	}
	if got, err := decodeClosedPRs(nil); err != nil || len(got) != 0 {
		t.Errorf("empty output: got %v, %v", got, err)
	}
	if _, err := decodeClosedPRs([]byte("gh: Not Found (HTTP 404)")); err == nil {
		t.Error("output that is not JSON must be an error, not an empty list")
	}
}

// ---------------------------------------------------------------------------
// The code a comment was written on
// ---------------------------------------------------------------------------

func TestHunkTail(t *testing.T) {
	if got := hunkTail(""); got != "" {
		t.Errorf("no hunk should give nothing, got %q", got)
	}
	if got := hunkTail(" \n\n"); got != "" {
		t.Errorf("a blank hunk should give nothing, got %q", got)
	}
	short := "@@ -1,2 +1,3 @@\r\n context\r\n+added\r\n"
	if got := hunkTail(short); got != "@@ -1,2 +1,3 @@\n context\n+added" {
		t.Errorf("short hunk = %q, want it whole with unix line ends", got)
	}

	var lines []string
	for i := 1; i <= 30; i++ {
		lines = append(lines, fmt.Sprintf("+line %d", i))
	}
	got := strings.Split(hunkTail(strings.Join(lines, "\n")), "\n")
	if len(got) != maxHunkLines || got[0] != "+line 19" || got[len(got)-1] != "+line 30" {
		t.Errorf("long hunk kept %d lines from %q to %q, want the last %d ending at the commented line", len(got), got[0], got[len(got)-1], maxHunkLines)
	}

	wide := hunkTail("+" + strings.Repeat("長", 400))
	if len(wide) > maxHunkLineLen || !strings.HasSuffix(wide, "...") || !strings.HasPrefix(wide, "+長") {
		t.Errorf("a very long line must be cut on a character boundary, got %d bytes", len(wide))
	}
}

func TestFeedbackContext(t *testing.T) {
	items := []humanFeedback{
		{Path: "internal/config/load.go", Line: 9, Hunk: "@@ -1,3 +1,9 @@\n+\tif err != nil {\n+\t\treturn &Config{}"},
		{Body: "a review body has no code"},
		{Path: "README.md", Hunk: "+one line"},
	}
	want := "internal/config/load.go:9\n@@ -1,3 +1,9 @@\n+\tif err != nil {\n+\t\treturn &Config{}\n\nREADME.md\n+one line"
	if got := feedbackContext(items); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	if got := feedbackContext([]humanFeedback{{Body: "no code at all"}}); got != "" {
		t.Errorf("feedback without code should give no context, got %q", got)
	}

	// Many comments: whole blocks only, within the budget.
	var many []humanFeedback
	for i := 0; i < 40; i++ {
		many = append(many, humanFeedback{Path: fmt.Sprintf("pkg/file%02d.go", i), Line: 1, Hunk: "+" + strings.Repeat("x", 180)})
	}
	got := feedbackContext(many)
	if len(got) > maxContextChars {
		t.Errorf("context is %d bytes, over the budget of %d", len(got), maxContextChars)
	}
	if !strings.HasPrefix(got, "pkg/file00.go:1\n") || strings.Count(got, "pkg/file") < 10 {
		t.Errorf("context should keep the first comments whole, got %d block(s)", strings.Count(got, "pkg/file"))
	}
	if last := got[strings.LastIndex(got, "\n")+1:]; len(last) != 181 {
		t.Errorf("the last block is cut short (%d bytes): blocks are kept whole or left out", len(last))
	}
}

// ---------------------------------------------------------------------------
// Syncing
// ---------------------------------------------------------------------------

// stubClosedPRs replaces the listing of closed pull requests; calls counts
// how often GitHub was asked.
func stubClosedPRs(t *testing.T, listing *[]closedPR, err error) *int {
	t.Helper()
	calls := 0
	orig := listClosedPRsFunc
	listClosedPRsFunc = func(ctx context.Context, timeout time.Duration) ([]closedPR, error) {
		calls++
		if err != nil {
			return nil, err
		}
		return *listing, nil
	}
	t.Cleanup(func() { listClosedPRsFunc = orig })
	return &calls
}

// mergedLog records a merge by the workflow for each issue → pull request pair.
func mergedLog(t *testing.T, root string, issueToPR map[int]int) {
	t.Helper()
	for issue := 1; issue <= 200; issue++ {
		pr, ok := issueToPR[issue]
		if !ok {
			continue
		}
		if err := RecordFeedback(root, FeedbackEntry{IssueID: issue, PRNumber: pr, Score: 9, Outcome: outcomeApproved, HeadSHA: commitHead}); err != nil {
			t.Fatal(err)
		}
	}
}

func scanStamps(t *testing.T, root string) map[string]string {
	t.Helper()
	return loadLateScan(root)
}

func TestSyncLateFeedback(t *testing.T) {
	root := t.TempDir()
	// Feedback the merge gate saw while the pull request was open, then the merges.
	recordHumanFeedback(root, 7, 70, []humanFeedback{{Ref: "comment:1", Author: "maintainer", Commit: commitOld, Body: "seen before the merge"}})
	// Issue 9 was merged through an earlier pull request; its second one was
	// closed without merging.
	mergedLog(t, root, map[int]int{7: 70, 8: 80, 9: 89})
	// A damaged log line must not make every branch look like the workflow's.
	if err := RecordFeedback(root, FeedbackEntry{Outcome: outcomeApproved}); err != nil {
		t.Fatal(err)
	}
	// A stamp from long ago, for a pull request that is no longer listed.
	saveLateScan(root, map[string]string{"999": "2026-01-01T00:00:00Z"})

	listing := []closedPR{
		{Number: 70, Updated: "2026-11-13T18:45:10Z", Merged: true, Branch: "feat/ai-issue-7"},
		{Number: 80, Updated: "2026-11-13T10:00:05Z", Merged: true, Branch: "feat/ai-issue-8"},
		{Number: 90, Updated: "2026-11-13T17:00:00Z", Merged: true, Branch: "feature/by-a-person"},
		{Number: 91, Updated: "2026-11-13T16:00:00Z", Merged: false, Branch: "feat/ai-issue-9"}, // closed without merging
		{Number: 92, Updated: "2026-11-13T15:00:00Z", Merged: true, Branch: "feat/ai-issue-12"}, // not merged by this workflow
	}
	calls := stubClosedPRs(t, &listing, nil)
	states := map[int]*prState{
		70: {HeadSHA: commitHead, Merged: true, MergedAt: mergedAt, Feedback: []humanFeedback{
			{Ref: "comment:1", Author: "maintainer", Commit: commitOld, Body: "seen before the merge", At: beforeMerge},
			{Ref: "comment:2", Author: "maintainer", Commit: commitHead, Body: "written before the merge, never seen", At: beforeMerge},
			{Ref: "comment:3", Author: "maintainer", Commit: commitHead, Path: "internal/config/load.go", Line: 9,
				Body: "Only fs.ErrNotExist means defaults. Return every other error.", At: afterMerge,
				Hunk: "@@ -1,3 +1,9 @@\n+\tif err != nil {\n+\t\treturn &Config{}"},
			{Ref: "review:4", Author: "maintainer", Commit: commitHead, Body: "Never drop an error in this package.", At: afterMerge},
		}},
		80: {HeadSHA: commitHead, Merged: true, MergedAt: mergedAt},
	}
	asked := stubPRState(t, states, nil)

	if got := SyncLateFeedback(context.Background(), root, time.Second); got != 2 {
		t.Fatalf("recorded %d item(s), want the 2 published after the merge", got)
	}
	if !reflect.DeepEqual(*asked, []int{70, 80}) {
		t.Errorf("read pull requests %v, want only the workflow's merged ones (70, 80)", *asked)
	}

	entries, _ := LoadFeedback(root)
	if len(entries) != 6 {
		t.Fatalf("got %d entries, want the 5 that were there and 1 new entry", len(entries))
	}
	entry := entries[5]
	if entry.IssueID != 7 || entry.PRNumber != 70 || !entry.AfterMerge || entry.Source != sourceHuman || entry.Outcome != "" {
		t.Errorf("entry = %+v, want a maintainer's objection to issue 7 / PR 70 marked as written after the merge", entry)
	}
	if !reflect.DeepEqual(entry.Refs, []string{"comment:3", "review:4"}) {
		t.Errorf("Refs = %v, want the two late items", entry.Refs)
	}
	for _, want := range []string{"Only fs.ErrNotExist means defaults.", "Never drop an error in this package.", "internal/config/load.go:9"} {
		if !strings.Contains(entry.Summary, want) {
			t.Errorf("Summary is missing %q: %s", want, entry.Summary)
		}
	}
	if strings.Contains(entry.Summary, "never seen") {
		t.Error("feedback published before the merge must not be recorded as late")
	}
	if entry.Context != "internal/config/load.go:9\n@@ -1,3 +1,9 @@\n+\tif err != nil {\n+\t\treturn &Config{}" {
		t.Errorf("Context = %q, want the code the line comment is on", entry.Context)
	}
	if entry.HeadSHA != commitHead || !reflect.DeepEqual(entry.Paths, []string{"internal/config/load.go"}) {
		t.Errorf("HeadSHA = %q, Paths = %v", entry.HeadSHA, entry.Paths)
	}
	if want := map[string]string{"70": "2026-11-13T18:45:10Z", "80": "2026-11-13T10:00:05Z"}; !reflect.DeepEqual(scanStamps(t, root), want) {
		t.Errorf("stamps = %v, want %v: the pull requests read, and none that is no longer listed", scanStamps(t, root), want)
	}
	// A late objection does not leave the issue waiting for an approval.
	if pending, err := lessons.PendingIssues(root); err != nil || len(pending) != 0 {
		t.Errorf("pending = %+v (err %v), want none", pending, err)
	}

	// Nothing happened on GitHub since: one listing call, no pull request read.
	*asked = nil
	if got := SyncLateFeedback(context.Background(), root, time.Second); got != 0 || len(*asked) != 0 {
		t.Errorf("second sync recorded %d and read %v, want nothing", got, *asked)
	}
	if *calls != 2 {
		t.Errorf("listing calls = %d, want one per sync", *calls)
	}

	// The maintainer adds a comment: only that pull request is read again,
	// and only the new item is recorded.
	listing[0].Updated = "2026-11-14T08:00:00Z"
	states[70].Feedback = append(states[70].Feedback, humanFeedback{Ref: "comment:5", Author: "maintainer", Commit: commitHead, Body: "And add a test for the unreadable file.", At: "2026-11-14T08:00:00Z"})
	if got := SyncLateFeedback(context.Background(), root, time.Second); got != 1 {
		t.Fatalf("third sync recorded %d item(s), want 1", got)
	}
	if !reflect.DeepEqual(*asked, []int{70}) {
		t.Errorf("third sync read %v, want only the updated pull request", *asked)
	}
	entries, _ = LoadFeedback(root)
	if last := entries[len(entries)-1]; len(entries) != 7 || !reflect.DeepEqual(last.Refs, []string{"comment:5"}) || !last.AfterMerge {
		t.Errorf("got %d entries, last %+v; want one more entry with the new item only", len(entries), last)
	}
}

func TestSyncLateFeedback_AsksGitHubOnlyWhenThereIsSomethingToFind(t *testing.T) {
	listing := []closedPR{{Number: 70, Updated: "2026-11-13T18:45:10Z", Merged: true, Branch: "feat/ai-issue-7"}}

	t.Run("no merge in the log", func(t *testing.T) {
		root := t.TempDir()
		if err := RecordFeedback(root, FeedbackEntry{IssueID: 7, PRNumber: 70, Score: 4, Summary: "rejected, not merged"}); err != nil {
			t.Fatal(err)
		}
		calls := stubClosedPRs(t, &listing, nil)
		if got := SyncLateFeedback(context.Background(), root, time.Second); got != 0 || *calls != 0 {
			t.Errorf("recorded %d with %d listing call(s), want no call at all", got, *calls)
		}
	})

	t.Run("switched off", func(t *testing.T) {
		root := t.TempDir()
		mergedLog(t, root, map[int]int{7: 70})
		writeWorkflowConfig(t, root, "review:\n  human_feedback: false\n")
		calls := stubClosedPRs(t, &listing, nil)
		asked := stubPRState(t, map[int]*prState{70: {Merged: true, MergedAt: mergedAt, Feedback: []humanFeedback{{Ref: "comment:3", Body: "x", At: afterMerge}}}}, nil)
		if got := SyncLateFeedback(context.Background(), root, time.Second); got != 0 || *calls != 0 || len(*asked) != 0 {
			t.Errorf("review.human_feedback: false must switch this off: recorded %d, %d listing call(s), read %v", got, *calls, *asked)
		}
	})

	t.Run("listing fails", func(t *testing.T) {
		root := t.TempDir()
		mergedLog(t, root, map[int]int{7: 70})
		saveLateScan(root, map[string]string{"70": "2026-11-12T00:00:00Z"})
		stubClosedPRs(t, &listing, errors.New("gh: API rate limit exceeded"))
		asked := stubPRState(t, nil, nil)
		if got := SyncLateFeedback(context.Background(), root, time.Second); got != 0 || len(*asked) != 0 {
			t.Errorf("recorded %d and read %v, want nothing", got, *asked)
		}
		if want := map[string]string{"70": "2026-11-12T00:00:00Z"}; !reflect.DeepEqual(scanStamps(t, root), want) {
			t.Errorf("stamps = %v: a failed listing must leave them as they were", scanStamps(t, root))
		}
	})
}

func TestSyncLateFeedback_UnreadPullRequestIsReadNextTime(t *testing.T) {
	root := t.TempDir()
	mergedLog(t, root, map[int]int{7: 70})
	saveLateScan(root, map[string]string{"70": "2026-11-13T10:00:05Z"})
	listing := []closedPR{{Number: 70, Updated: "2026-11-13T18:45:10Z", Merged: true, Branch: "feat/ai-issue-7"}}
	stubClosedPRs(t, &listing, nil)

	failing := stubPRState(t, nil, errors.New("gh: timeout"))
	if got := SyncLateFeedback(context.Background(), root, time.Second); got != 0 || len(*failing) != 1 {
		t.Fatalf("recorded %d after %d read(s), want 0 after 1 failed read", got, len(*failing))
	}
	if got := scanStamps(t, root)["70"]; got != "2026-11-13T10:00:05Z" {
		t.Errorf("stamp = %q: a pull request that could not be read keeps its old stamp", got)
	}

	working := stubPRState(t, map[int]*prState{70: {HeadSHA: commitHead, Merged: true, MergedAt: mergedAt,
		Feedback: []humanFeedback{{Ref: "review:4", Author: "maintainer", Body: "Never drop an error.", At: afterMerge}}}}, nil)
	if got := SyncLateFeedback(context.Background(), root, time.Second); got != 1 || len(*working) != 1 {
		t.Errorf("recorded %d after %d read(s), want the item on the retry", got, len(*working))
	}
}

func TestSyncLateFeedback_ReadsAreBounded(t *testing.T) {
	root := t.TempDir()
	issues := map[int]int{}
	var listing []closedPR
	total := maxLateReads + 3
	for i := 1; i <= total; i++ {
		issues[i] = 100 + i
		listing = append(listing, closedPR{Number: 100 + i, Updated: "2026-11-13T18:00:00Z", Merged: true, Branch: fmt.Sprintf("feat/ai-issue-%d", i)})
	}
	mergedLog(t, root, issues)
	stubClosedPRs(t, &listing, nil)
	asked := stubPRState(t, nil, nil)

	SyncLateFeedback(context.Background(), root, time.Second)
	if len(*asked) != maxLateReads {
		t.Fatalf("first sync read %d pull request(s), want the budget of %d", len(*asked), maxLateReads)
	}
	if got := len(scanStamps(t, root)); got != maxLateReads {
		t.Errorf("%d stamp(s) saved, want only those of the pull requests that were read", got)
	}

	*asked = nil
	SyncLateFeedback(context.Background(), root, time.Second)
	if len(*asked) != 3 {
		t.Errorf("second sync read %d pull request(s), want the 3 left over", len(*asked))
	}
	if got := len(scanStamps(t, root)); got != total {
		t.Errorf("%d stamp(s) saved, want all %d", got, total)
	}
}

func TestLoadLateScan_UnreadableFileMeansNothingRead(t *testing.T) {
	root := t.TempDir()
	if got := loadLateScan(root); len(got) != 0 {
		t.Errorf("no file: got %v", got)
	}
	path := filepath.Join(root, filepath.FromSlash(lateScanFile))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := loadLateScan(root); got == nil || len(got) != 0 {
		t.Errorf("garbled file: got %v, want an empty map", got)
	}
	if err := os.WriteFile(path, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := loadLateScan(root); got == nil {
		t.Error("a file without stamps must still give a usable map")
	}
}

// ---------------------------------------------------------------------------
// The learning loop
// ---------------------------------------------------------------------------

func TestCloseLearningLoop_MergeKeepsTheLessonsTheAttemptHad(t *testing.T) {
	root := learningRoot(t, 7, activeLesson("L-001", []string{"error-handling"}, nil))
	stubPRDiff(t, diffTouching("internal/config/load.go"), nil)

	closeLearningLoop(context.Background(), SubmitReviewOptions{
		StateRoot: root, IssueNumber: 7, PRNumber: 70, Score: 9, headSHA: commitHead,
	}, learningOutcome{Outcome: lessons.OutcomeMerged})

	entry := onlyFeedbackEntry(t, root)
	if entry.Outcome != outcomeApproved || !reflect.DeepEqual(entry.InjectedLessons, []string{"L-001"}) {
		t.Errorf("entry = %+v, want the merge to remember the lessons its attempt had", entry)
	}
}

func TestCloseLearningLoop_MergeTakesInWhatMaintainersDidMeanwhile(t *testing.T) {
	root := learningRoot(t, 8)
	writeWorkflowConfig(t, root, "lessons:\n  enabled: true\n")
	mergedLog(t, root, map[int]int{7: 70}) // merged earlier in the run
	stubPRDiff(t, diffTouching("internal/config/load.go"), nil)
	listing := []closedPR{{Number: 70, Updated: "2026-11-13T18:45:10Z", Merged: true, Branch: "feat/ai-issue-7"}}
	stubClosedPRs(t, &listing, nil)
	stubPRState(t, map[int]*prState{70: {HeadSHA: commitHead, Merged: true, MergedAt: mergedAt,
		Feedback: []humanFeedback{{Ref: "review:4", Author: "maintainer", Body: "Never drop an error.", At: afterMerge}}}}, nil)
	// No distiller on the PATH: no model is called from a test.
	t.Setenv("PATH", "")

	closeLearningLoop(context.Background(), SubmitReviewOptions{
		StateRoot: root, IssueNumber: 8, PRNumber: 80, Score: 9, headSHA: commitHead,
	}, learningOutcome{Outcome: lessons.OutcomeMerged})

	entries, _ := LoadFeedback(root)
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want the earlier merge, this merge and the maintainer's review of the earlier one", len(entries))
	}
	if merge := entries[1]; merge.IssueID != 8 || merge.Outcome != outcomeApproved {
		t.Errorf("second entry = %+v, want this merge", merge)
	}
	if late := entries[2]; late.IssueID != 7 || !late.AfterMerge || !reflect.DeepEqual(late.Refs, []string{"review:4"}) {
		t.Errorf("third entry = %+v, want the review written on the pull request merged earlier", late)
	}
}

func TestSyncMaintainerActivity(t *testing.T) {
	root := t.TempDir()
	mergedLog(t, root, map[int]int{7: 70})
	// Issue 8 was rejected; its pull request was then merged by hand.
	if err := RecordFeedback(root, FeedbackEntry{IssueID: 8, PRNumber: 80, Score: 4, Summary: "no tests", HeadSHA: commitOld}); err != nil {
		t.Fatal(err)
	}
	listing := []closedPR{
		{Number: 70, Updated: "2026-11-13T18:45:10Z", Merged: true, Branch: "feat/ai-issue-7"},
		{Number: 80, Updated: "2026-11-13T12:00:00Z", Merged: true, Branch: "feat/ai-issue-8"},
	}
	stubClosedPRs(t, &listing, nil)
	stubPRState(t, map[int]*prState{
		70: {HeadSHA: commitHead, Merged: true, MergedAt: mergedAt, Feedback: []humanFeedback{{Ref: "review:4", Author: "maintainer", Body: "Never drop an error.", At: afterMerge}}},
		80: {HeadSHA: commitHead, Merged: true, MergedAt: mergedAt},
	}, nil)
	origFetch := fetchPRHeadFunc
	fetchPRHeadFunc = func(stateRoot string, prNumber int) {}
	t.Cleanup(func() { fetchPRHeadFunc = origFetch })

	handMerged, lateItems := SyncMaintainerActivity(context.Background(), root, time.Second)

	if handMerged != 1 || lateItems != 1 {
		t.Errorf("got %d merged by hand and %d late item(s), want 1 and 1", handMerged, lateItems)
	}
}

func TestLearnBeforeDispatch(t *testing.T) {
	late := map[int]*prState{70: {HeadSHA: commitHead, Merged: true, MergedAt: mergedAt,
		Feedback: []humanFeedback{{Ref: "review:4", Author: "maintainer", Body: "Never drop an error.", At: afterMerge}}}}
	listing := []closedPR{{Number: 70, Updated: "2026-11-13T18:45:10Z", Merged: true, Branch: "feat/ai-issue-7"}}
	storePath := func(root string) string { return lessons.StorePath(root) }

	t.Run("lessons switched off", func(t *testing.T) {
		root := t.TempDir()
		mergedLog(t, root, map[int]int{7: 70})
		writeWorkflowConfig(t, root, "lessons:\n  enabled: false\n")
		calls := stubClosedPRs(t, &listing, nil)
		var log bytes.Buffer

		LearnBeforeDispatch(context.Background(), root, time.Second, &log)

		if *calls != 0 || log.Len() != 0 {
			t.Errorf("lessons.enabled: false: %d listing call(s), log %q; want nothing", *calls, log.String())
		}
	})

	t.Run("nothing new", func(t *testing.T) {
		root := t.TempDir()
		mergedLog(t, root, map[int]int{7: 70})
		writeWorkflowConfig(t, root, "lessons:\n  enabled: true\n")
		stubClosedPRs(t, &listing, nil)
		stubPRState(t, map[int]*prState{70: {HeadSHA: commitHead, Merged: true, MergedAt: mergedAt}}, nil)
		var log bytes.Buffer

		LearnBeforeDispatch(context.Background(), root, time.Second, &log)

		if log.Len() != 0 {
			t.Errorf("nothing new should be silent, got %q", log.String())
		}
		if _, err := os.Stat(storePath(root)); !os.IsNotExist(err) {
			t.Errorf("the distiller must not run when the sync found nothing (store written: %v)", err)
		}
	})

	t.Run("a maintainer reviewed a merged pull request", func(t *testing.T) {
		root := t.TempDir()
		mergedLog(t, root, map[int]int{7: 70})
		writeWorkflowConfig(t, root, "lessons:\n  enabled: true\n")
		stubClosedPRs(t, &listing, nil)
		stubPRState(t, late, nil)
		// No distiller on the PATH: the run is attempted and reported, and no
		// model is called from a test.
		t.Setenv("PATH", "")
		var log bytes.Buffer

		LearnBeforeDispatch(context.Background(), root, time.Second, &log)

		for _, want := range []string{"0 pull request(s) merged by hand, 1 review item(s) on merged pull requests", "1 case(s) distilled (0 new, 0 upvoted, 1 skipped)"} {
			if !strings.Contains(log.String(), want) {
				t.Errorf("log is missing %q:\n%s", want, log.String())
			}
		}
		entries, _ := LoadFeedback(root)
		if last := entries[len(entries)-1]; !last.AfterMerge || !reflect.DeepEqual(last.Refs, []string{"review:4"}) {
			t.Errorf("the review should be in the feedback log, last entry %+v", last)
		}
		// The distiller could not run, so the case is kept for the next run.
		store, err := lessons.Load(root)
		if err != nil || store.Watermark.FeedbackLine != 0 {
			t.Errorf("watermark = %d (err %v), want 0: an attempt that could not run is retried", store.Watermark.FeedbackLine, err)
		}
	})
}
