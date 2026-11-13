package reviewer

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/silver2dream/ai-workflow-kit/internal/lessons"
)

// ---------------------------------------------------------------------------
// Changed paths
// ---------------------------------------------------------------------------

func TestChangedPaths_ReadsEveryKindOfChange(t *testing.T) {
	diff := strings.Join([]string{
		"diff --git a/internal/modified.go b/internal/modified.go",
		"index 1111111..2222222 100644",
		"--- a/internal/modified.go",
		"+++ b/internal/modified.go",
		"@@ -1,3 +1,3 @@",
		" package internal",
		"+++ b/not/a/header.go", // hunk content: an added line whose text is "++ b/not/a/header.go"
		"--- a/nor/is/this.go",  // hunk content: a removed line whose text is "-- a/nor/is/this.go"
		"diff --git a/internal/added.go b/internal/added.go",
		"new file mode 100644",
		"--- /dev/null",
		"+++ b/internal/added.go",
		"@@ -0,0 +1 @@",
		"+package internal",
		"diff --git a/internal/deleted.go b/internal/deleted.go",
		"deleted file mode 100644",
		"--- a/internal/deleted.go",
		"+++ /dev/null",
		"@@ -1 +0,0 @@",
		"-package internal",
		"diff --git a/docs/old name.md b/docs/new name.md",
		"similarity index 100%",
		"rename from docs/old name.md",
		"rename to docs/new name.md",
		"diff --git a/assets/logo.png b/assets/logo.png",
		"index 3333333..4444444 100644",
		"Binary files a/assets/logo.png and b/assets/logo.png differ",
		"diff --git a/scripts/run.sh b/scripts/run.sh",
		"old mode 100644",
		"new mode 100755",
		`diff --git "a/docs/\346\225\231\350\250\223.md" "b/docs/\346\225\231\350\250\223.md"`,
		"index 5555555..6666666 100644",
		`--- "a/docs/\346\225\231\350\250\223.md"`,
		`+++ "b/docs/\346\225\231\350\250\223.md"`,
		"@@ -1 +1 @@",
		"-old",
		"+new",
		`diff --git "a/docs/\345\210\252.md" "b/docs/\345\210\252.md"`,
		"deleted file mode 100644",
		`--- "a/docs/\345\210\252.md"`,
		"+++ /dev/null",
		"@@ -1 +0,0 @@",
		"-old",
		"diff --git a/with space.txt b/with space.txt",
		"--- a/with space.txt\t",
		"+++ b/with space.txt\t",
		"@@ -1 +1 @@",
		"-old",
		"+new",
		"",
	}, "\r\n") // a diff may arrive with either line ending

	got := ChangedPaths(diff)
	want := []string{
		"internal/modified.go",
		"internal/added.go",
		"internal/deleted.go",
		"docs/old name.md",
		"docs/new name.md",
		"assets/logo.png",
		"scripts/run.sh",
		"docs/教訓.md",
		"docs/刪.md",
		"with space.txt",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ChangedPaths =\n  %q\nwant\n  %q", got, want)
	}
}

func TestChangedPaths_EmptyDiff(t *testing.T) {
	if got := ChangedPaths(""); len(got) != 0 {
		t.Errorf("ChangedPaths(\"\") = %v, want none", got)
	}
}

// ---------------------------------------------------------------------------
// What the reviewer objected to
// ---------------------------------------------------------------------------

func TestRejectionFindings_StructuredReviewGivesReasonAndBlockingFindings(t *testing.T) {
	review := &StructuredReview{
		Score:   5,
		Summary: "The retry path loses the original error.",
		Criteria: []StructuredCriterion{{
			Criterion:      "Requests are retried three times",
			Implementation: "Retry() at client.go:40 loops over attempts",
			TestName:       "TestRetry",
			Assertion:      "assert.Equal(t, 3, calls)",
		}},
		Improvements: []StructuredImprovement{
			{Severity: "critical", Location: "client.go:52", Text: "the last error is dropped, callers get nil"},
			{Severity: "Important", Text: "no test covers the exhausted-retries path"},
			{Severity: "nit", Text: "rename tmp to attempt"},
		},
	}
	opts := SubmitReviewOptions{Structured: review, ReviewBody: review.RenderMarkdown()}

	got := rejectionFindings(opts)

	want := strings.Join([]string{
		"The retry path loses the original error.",
		"- Critical: client.go:52 — the last error is dropped, callers get nil",
		"- Important: no test covers the exhausted-retries path",
	}, "\n")
	if got != want {
		t.Errorf("rejectionFindings =\n%s\nwant\n%s", got, want)
	}
	// The rendered body opens with the criterion walkthrough: its first 500
	// bytes are not the reason, which is why the body is not used as is.
	if strings.Contains(got, "Retry() at client.go:40") {
		t.Error("the criterion walkthrough is not an objection")
	}
}

func TestRejectionFindings_AdviceIsUsedWhenNothingBlocks(t *testing.T) {
	opts := SubmitReviewOptions{Structured: &StructuredReview{
		Improvements: []StructuredImprovement{{Severity: "nit", Text: "rename tmp to attempt"}},
	}}
	if got := rejectionFindings(opts); got != "- Nit: rename tmp to attempt" {
		t.Errorf("rejectionFindings = %q", got)
	}
}

func TestRejectionFindings_MarkdownReviewUsesItsReasonSections(t *testing.T) {
	body := `### Implementation Review

#### 1. Requests are retried
**Implementation**: Retry() loops over attempts

### Test Review

| Criteria | Test | Key Assertion |
|----------|------|---------------|
| Requests are retried | TestRetry | assert.Equal(t, 3, calls) |

### Score Reason

The retry path loses the original error.

### Suggested Improvements

- **Critical:** the last error is dropped

### Potential Risks

None
`
	got := rejectionFindings(SubmitReviewOptions{ReviewBody: body})
	want := "The retry path loses the original error.\n- **Critical:** the last error is dropped"
	if got != want {
		t.Errorf("rejectionFindings =\n%s\nwant\n%s", got, want)
	}
}

func TestRejectionFindings_IncludesTheConsensusSection(t *testing.T) {
	review := &StructuredReview{Score: 8, Summary: "Looks complete."}
	consensus := consensusHeading + "\n\n**Consensus Score: 6/10**\n\n### codex\n- **Score**: 4/10\n\n[ERROR] the migration is not reversible"
	opts := SubmitReviewOptions{Structured: review, ReviewBody: review.RenderMarkdown() + "\n\n" + consensus}

	got := rejectionFindings(opts)

	if !strings.HasPrefix(got, "Looks complete.\n"+consensusHeading) || !strings.Contains(got, "the migration is not reversible") {
		t.Errorf("the consensus that rejected the pull request should be part of the findings:\n%s", got)
	}
}

func TestRejectionFindings_FallsBackToTheBody(t *testing.T) {
	if got := rejectionFindings(SubmitReviewOptions{ReviewBody: "  needs work: no tests  \n"}); got != "needs work: no tests" {
		t.Errorf("rejectionFindings = %q", got)
	}
}

func TestTruncateSummary_KeepsCharactersWhole(t *testing.T) {
	text := strings.Repeat("審查", 20) // 3 bytes per character
	for max := 4; max < len(text); max++ {
		got := truncateSummary(text, max)
		if !utf8.ValidString(got) {
			t.Fatalf("truncateSummary(_, %d) split a character: %q", max, got)
		}
		if len(got) > max {
			t.Fatalf("truncateSummary(_, %d) returned %d bytes", max, len(got))
		}
	}
}

// ---------------------------------------------------------------------------
// closeLearningLoop
// ---------------------------------------------------------------------------

// stubPRDiff replaces the GitHub diff fetch for one test.
func stubPRDiff(t *testing.T, diff string, err error) {
	t.Helper()
	orig := fetchPRDiffFunc
	fetchPRDiffFunc = func(ctx context.Context, prNumber int, timeout time.Duration) (string, error) {
		return diff, err
	}
	t.Cleanup(func() { fetchPRDiffFunc = orig })
}

func diffTouching(paths ...string) string {
	var b strings.Builder
	for _, p := range paths {
		b.WriteString("diff --git a/" + p + " b/" + p + "\n--- a/" + p + "\n+++ b/" + p + "\n@@ -1 +1 @@\n-old\n+new\n")
	}
	return b.String()
}

// learningRoot saves the given lessons and records them as injected into the issue's run.
func learningRoot(t *testing.T, issue int, stored ...lessons.Lesson) string {
	t.Helper()
	root := t.TempDir()
	if err := lessons.Save(root, &lessons.Store{Version: 1, Lessons: stored}); err != nil {
		t.Fatal(err)
	}
	if err := lessons.RecordInjection(root, issue, lessons.IDs(stored)); err != nil {
		t.Fatal(err)
	}
	return root
}

func activeLesson(id string, categories, scope []string) lessons.Lesson {
	return lessons.Lesson{
		ID: id, Title: "lesson " + id, Content: "- do the check", Kind: lessons.KindPitfall,
		Categories: categories, Scope: scope,
		Fingerprint: lessons.FingerprintOf("lesson "+id, categories, scope),
		Status:      lessons.StatusActive, CreatedAt: "2026-06-01T00:00:00Z", Source: "human",
	}
}

func lessonCounts(t *testing.T, root, id string) (hits, misses int) {
	t.Helper()
	store, err := lessons.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	l := store.FindByID(id)
	if l == nil {
		t.Fatalf("lesson %s not in store", id)
	}
	return l.Hits, l.Misses
}

func onlyFeedbackEntry(t *testing.T, root string) FeedbackEntry {
	t.Helper()
	entries, err := LoadFeedback(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d feedback entries, want 1", len(entries))
	}
	return entries[0]
}

func TestCloseLearningLoop_RejectionRecordsTheEvidenceForTheCase(t *testing.T) {
	root := learningRoot(t, 7,
		activeLesson("L-001", []string{"error-handling"}, []string{"internal/client/"}),
		activeLesson("L-002", []string{"error-handling"}, []string{"frontend/"}),
	)
	// The file under L-001's scope comes after more files than an entry keeps.
	var changed []string
	for i := 0; i < maxFeedbackPaths+2; i++ {
		changed = append(changed, "docs/page-"+string(rune('a'+i))+".md")
	}
	changed = append(changed, "internal/client/retry.go")
	stubPRDiff(t, diffTouching(changed...), nil)
	findings := "The retry path loses the original error. " + strings.Repeat("Callers get nil. ", 60)

	closeLearningLoop(context.Background(), SubmitReviewOptions{
		StateRoot: root, IssueNumber: 7, PRNumber: 70, Score: 5, headSHA: "abc1234",
	}, learningOutcome{Outcome: lessons.OutcomeChangesRequested, Findings: findings})

	entry := onlyFeedbackEntry(t, root)
	if entry.Outcome != "" || entry.IssueID != 7 || entry.PRNumber != 70 || entry.Score != 5 {
		t.Errorf("entry = %+v, want a rejection of issue 7 / PR 70", entry)
	}
	if entry.HeadSHA != "abc1234" {
		t.Errorf("HeadSHA = %q, want the reviewed commit", entry.HeadSHA)
	}
	if !reflect.DeepEqual(entry.InjectedLessons, []string{"L-001", "L-002"}) {
		t.Errorf("InjectedLessons = %v, want the lessons of the rejected attempt", entry.InjectedLessons)
	}
	if len(entry.Paths) != maxFeedbackPaths || entry.Paths[0] != "docs/page-a.md" {
		t.Errorf("entry keeps %d paths (%v), want the first %d", len(entry.Paths), entry.Paths, maxFeedbackPaths)
	}
	if len(entry.Summary) > maxSummaryChars || !strings.HasPrefix(entry.Summary, "The retry path loses") {
		t.Errorf("Summary = %q", entry.Summary)
	}
	if len(entry.Findings) <= maxSummaryChars || len(entry.Findings) > maxFindingsChars {
		t.Errorf("Findings has %d bytes, want the longer form up to %d", len(entry.Findings), maxFindingsChars)
	}
	if !reflect.DeepEqual(entry.Categories, []string{"error-handling"}) {
		t.Errorf("Categories = %v, want them read from the findings", entry.Categories)
	}

	// Settlement saw the whole file list, not the capped one, and counted
	// only the lesson whose scope the pull request touched.
	if _, misses := lessonCounts(t, root, "L-001"); misses != 1 {
		t.Errorf("L-001 misses = %d, want 1", misses)
	}
	if _, misses := lessonCounts(t, root, "L-002"); misses != 0 {
		t.Errorf("L-002 misses = %d, want 0: frontend/ was not touched", misses)
	}
}

func TestCloseLearningLoop_ShortFindingsAreNotStoredTwice(t *testing.T) {
	root := t.TempDir()
	stubPRDiff(t, "", nil)

	closeLearningLoop(context.Background(), SubmitReviewOptions{StateRoot: root, IssueNumber: 7, PRNumber: 70},
		learningOutcome{Outcome: lessons.OutcomeChangesRequested, Categories: []string{"ci-failure"}, Findings: "CI failed"})

	entry := onlyFeedbackEntry(t, root)
	if entry.Summary != "CI failed" || entry.Findings != "" {
		t.Errorf("summary=%q findings=%q, want the reason once, in the summary", entry.Summary, entry.Findings)
	}
	if !reflect.DeepEqual(entry.Categories, []string{"ci-failure"}) {
		t.Errorf("Categories = %v, want the category the caller set", entry.Categories)
	}
}

func TestCloseLearningLoop_MergeCountsOnlyLessonsThatApplied(t *testing.T) {
	root := learningRoot(t, 7,
		activeLesson("L-001", []string{"config"}, []string{"internal/client/"}),
		activeLesson("L-002", []string{"config"}, []string{"frontend/"}),
		activeLesson("L-003", []string{"config"}, nil),
	)
	stubPRDiff(t, diffTouching("internal/client/retry.go"), nil)

	closeLearningLoop(context.Background(), SubmitReviewOptions{
		StateRoot: root, IssueNumber: 7, PRNumber: 70, Score: 9, headSHA: "def5678",
	}, learningOutcome{Outcome: lessons.OutcomeMerged})

	entry := onlyFeedbackEntry(t, root)
	if entry.Outcome != "approved" || entry.HeadSHA != "def5678" || !reflect.DeepEqual(entry.Paths, []string{"internal/client/retry.go"}) {
		t.Errorf("entry = %+v, want an approval of def5678 with its changed path", entry)
	}
	for id, wantHits := range map[string]int{"L-001": 1, "L-002": 0, "L-003": 1} {
		if hits, _ := lessonCounts(t, root, id); hits != wantHits {
			t.Errorf("%s hits = %d, want %d", id, hits, wantHits)
		}
	}
	if lessons.LoadInjection(root, 7) != nil {
		t.Error("the merge should end the issue's attribution window")
	}
}

func TestCloseLearningLoop_UnreadableDiffLeavesScopedLessonsUncounted(t *testing.T) {
	root := learningRoot(t, 7,
		activeLesson("L-001", []string{"config"}, []string{"internal/client/"}),
		activeLesson("L-002", []string{"config"}, nil),
	)
	stubPRDiff(t, "", errors.New("gh: diff too large"))

	closeLearningLoop(context.Background(), SubmitReviewOptions{StateRoot: root, IssueNumber: 7, PRNumber: 70},
		learningOutcome{Outcome: lessons.OutcomeMerged})

	if hits, _ := lessonCounts(t, root, "L-001"); hits != 0 {
		t.Errorf("scoped lesson hits = %d, want 0 when the file list is unknown", hits)
	}
	if hits, _ := lessonCounts(t, root, "L-002"); hits != 1 {
		t.Errorf("unscoped lesson hits = %d, want 1", hits)
	}
	if entry := onlyFeedbackEntry(t, root); len(entry.Paths) != 0 {
		t.Errorf("Paths = %v, want none", entry.Paths)
	}
}

// ---------------------------------------------------------------------------
// Reviewed commit
// ---------------------------------------------------------------------------

func TestWorktreeHead(t *testing.T) {
	if got := worktreeHead(""); got != "" {
		t.Errorf("worktreeHead(\"\") = %q, want empty", got)
	}
	// A plain directory, even inside a repository, is not a checkout root.
	if got := worktreeHead(t.TempDir()); got != "" {
		t.Errorf("worktreeHead(plain dir) = %q, want empty", got)
	}
	if got := worktreeHead("."); got != "" {
		t.Errorf("worktreeHead(package dir) = %q, want empty: it has no .git entry of its own", got)
	}

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repo := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		full := append([]string{"-C", repo,
			"-c", "user.name=test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false"}, args...)
		out, err := exec.Command("git", full...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q")
	if got := worktreeHead(repo); got != "" {
		t.Errorf("worktreeHead(repo without commits) = %q, want empty", got)
	}
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("commit", "-q", "-m", "first")
	if got, want := worktreeHead(repo), git("rev-parse", "HEAD"); got != want || len(got) < 40 {
		t.Errorf("worktreeHead = %q, want %q", got, want)
	}
}
