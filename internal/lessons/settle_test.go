package lessons

import (
	"reflect"
	"testing"
)

// settleRoot saves a store holding the given lessons and returns its root.
func settleRoot(t *testing.T, stored ...Lesson) string {
	t.Helper()
	root := t.TempDir()
	if err := Save(root, &Store{Version: 1, Lessons: stored}); err != nil {
		t.Fatal(err)
	}
	return root
}

func inject(t *testing.T, root string, issue int, ids ...string) {
	t.Helper()
	if err := RecordInjection(root, issue, ids); err != nil {
		t.Fatal(err)
	}
}

func settle(t *testing.T, root string, in SettleInput) *SettleReport {
	t.Helper()
	report, err := Settle(root, in)
	if err != nil {
		t.Fatalf("Settle: %v", err)
	}
	return report
}

func mergedPR(issue int, paths ...string) SettleInput {
	return SettleInput{IssueID: issue, Outcome: OutcomeMerged, ChangedPaths: paths, PathsKnown: true}
}

func rejectedPR(issue int, findings string, paths ...string) SettleInput {
	return SettleInput{
		IssueID: issue, Outcome: OutcomeChangesRequested, RejectionText: findings,
		ChangedPaths: paths, PathsKnown: true,
	}
}

func storedLesson(t *testing.T, root, id string) *Lesson {
	t.Helper()
	s, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	l := s.FindByID(id)
	if l == nil {
		t.Fatalf("lesson %s not in store", id)
	}
	return l
}

// ---------------------------------------------------------------------------
// Hits and misses
// ---------------------------------------------------------------------------

func TestSettle_MergedCountsHitsAndAdvances(t *testing.T) {
	l := testLesson("L-001", "config schema sync", StatusCandidate, []string{"config"}, nil)
	l.Hits = 1 // one more clean outcome promotes to active
	root := settleRoot(t, l)
	inject(t, root, 7, "L-001")

	report := settle(t, root, mergedPR(7, "internal/analyzer/config.go"))

	got := storedLesson(t, root, "L-001")
	if got.Hits != 2 {
		t.Errorf("Hits = %d, want 2", got.Hits)
	}
	if got.Status != StatusActive {
		t.Errorf("Status = %s, want active (candidate promoted)", got.Status)
	}
	if !reflect.DeepEqual(report.Hits, []string{"L-001"}) {
		t.Errorf("report.Hits = %v, want [L-001]", report.Hits)
	}
	if LoadInjection(root, 7) != nil {
		t.Error("the merge ends the attribution window: the record should be gone")
	}
}

func TestSettle_MatchingRejectionCountsMiss(t *testing.T) {
	l := testLesson("L-001", "config schema sync", StatusActive, []string{"config"}, nil)
	l.Misses = 2 // one more miss retires it
	root := settleRoot(t, l)
	inject(t, root, 7, "L-001")

	report := settle(t, root, rejectedPR(7, "the workflow.yaml schema was not updated"))

	got := storedLesson(t, root, "L-001")
	if got.Misses != 3 {
		t.Errorf("Misses = %d, want 3", got.Misses)
	}
	if got.Status != StatusRetired {
		t.Errorf("Status = %s, want retired after 3 misses", got.Status)
	}
	if !reflect.DeepEqual(report.Misses, []string{"L-001"}) {
		t.Errorf("report.Misses = %v, want [L-001]", report.Misses)
	}
}

func TestSettle_UnrelatedRejectionIsNoSignal(t *testing.T) {
	root := settleRoot(t, testLesson("L-001", "config schema sync", StatusActive, []string{"config"}, nil))
	inject(t, root, 7, "L-001")

	report := settle(t, root, rejectedPR(7, "variable naming is unclear, rename tmp"))

	got := storedLesson(t, root, "L-001")
	if got.Hits != 0 || got.Misses != 0 {
		t.Errorf("unrelated rejection must not move counters: hits=%d misses=%d", got.Hits, got.Misses)
	}
	if !reflect.DeepEqual(report.NoSignal, []string{"L-001"}) {
		t.Errorf("report.NoSignal = %v, want [L-001]", report.NoSignal)
	}
}

func TestSettle_NoInjectionIsNoop(t *testing.T) {
	report, err := Settle(t.TempDir(), mergedPR(99))
	if err != nil {
		t.Errorf("settle without injection record should be a silent noop: %v", err)
	}
	if report == nil || len(report.Hits)+len(report.Misses)+len(report.NoSignal) != 0 {
		t.Errorf("report = %+v, want an empty report", report)
	}
}

// ---------------------------------------------------------------------------
// A lesson counts only where it applied
// ---------------------------------------------------------------------------

func TestSettle_ScopedLessonCountsOnlyWhenItsScopeChanged(t *testing.T) {
	tests := []struct {
		name    string
		scope   []string
		changed []string
		wantHit bool
	}{
		{"file under the scope", []string{"internal/analyzer/"}, []string{"internal/analyzer/config.go"}, true},
		{"one of several files", []string{"internal/analyzer/"}, []string{"README.md", "internal/analyzer/config.go"}, true},
		{"second scope prefix", []string{"docs/", "internal/analyzer/"}, []string{"internal/analyzer/config.go"}, true},
		{"scope names the file", []string{"internal/analyzer/config.go"}, []string{"internal/analyzer/config.go"}, true},
		{"windows separators and case", []string{`Internal\Analyzer\`}, []string{"internal/analyzer/config.go"}, true},
		{"leading ./", []string{"./internal/analyzer/"}, []string{"internal/analyzer/config.go"}, true},
		{"scope is the repository root", []string{"./"}, []string{"anything.go"}, true},
		{"submodule pointer above the scope", []string{"backend/internal/"}, []string{"backend"}, true},
		{"elsewhere in the repository", []string{"internal/analyzer/"}, []string{"frontend/src/app.tsx"}, false},
		{"sibling directory with the same stem", []string{"internal/analyzer/"}, []string{"internal/analyzer_v2/config.go"}, false},
		{"no files changed", []string{"internal/analyzer/"}, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := settleRoot(t, testLesson("L-001", "config schema sync", StatusActive, []string{"config"}, tt.scope))
			inject(t, root, 7, "L-001")

			settle(t, root, mergedPR(7, tt.changed...))

			if got := storedLesson(t, root, "L-001").Hits; (got == 1) != tt.wantHit {
				t.Errorf("Hits = %d, want hit=%v", got, tt.wantHit)
			}
		})
	}
}

func TestSettle_RejectionOutsideTheScopeIsNotAMiss(t *testing.T) {
	root := settleRoot(t, testLesson("L-001", "config schema sync", StatusActive, []string{"config"}, []string{"internal/analyzer/"}))
	inject(t, root, 7, "L-001")

	settle(t, root, rejectedPR(7, "the config schema was not updated", "frontend/src/app.tsx"))

	if got := storedLesson(t, root, "L-001").Misses; got != 0 {
		t.Errorf("Misses = %d, want 0: the pull request never touched the lesson's scope", got)
	}
}

func TestSettle_UnknownFileListCountsOnlyUnscopedLessons(t *testing.T) {
	root := settleRoot(t,
		testLesson("L-001", "scoped lesson", StatusActive, []string{"config"}, []string{"internal/analyzer/"}),
		testLesson("L-002", "repository-wide lesson", StatusActive, []string{"config"}, nil),
	)
	inject(t, root, 7, "L-001", "L-002")

	report := settle(t, root, SettleInput{IssueID: 7, Outcome: OutcomeMerged, PathsKnown: false})

	if got := storedLesson(t, root, "L-001").Hits; got != 0 {
		t.Errorf("scoped lesson Hits = %d, want 0 when the file list is unknown", got)
	}
	if got := storedLesson(t, root, "L-002").Hits; got != 1 {
		t.Errorf("unscoped lesson Hits = %d, want 1", got)
	}
	if !reflect.DeepEqual(report.NoSignal, []string{"L-001"}) || !reflect.DeepEqual(report.Hits, []string{"L-002"}) {
		t.Errorf("report = %+v, want L-001 no signal and L-002 hit", report)
	}
}

// ---------------------------------------------------------------------------
// One issue is one sample
// ---------------------------------------------------------------------------

func TestSettle_OneMissPerIssue(t *testing.T) {
	root := settleRoot(t, testLesson("L-001", "config schema sync", StatusActive, []string{"config"}, nil))
	inject(t, root, 7, "L-001")

	settle(t, root, rejectedPR(7, "the config schema was not updated"))
	settle(t, root, rejectedPR(7, "the config schema is still not updated"))

	if got := storedLesson(t, root, "L-001").Misses; got != 1 {
		t.Errorf("Misses = %d, want 1: two rejections of one issue are one sample", got)
	}
}

func TestSettle_MergeAfterAMissIsNotAHit(t *testing.T) {
	root := settleRoot(t, testLesson("L-001", "config schema sync", StatusActive, []string{"config"}, nil))
	inject(t, root, 7, "L-001")
	settle(t, root, rejectedPR(7, "the config schema was not updated"))

	// The Worker is dispatched again with the same lesson and the review
	// feedback, and this time the pull request merges.
	inject(t, root, 7, "L-001")
	report := settle(t, root, mergedPR(7))

	got := storedLesson(t, root, "L-001")
	if got.Hits != 0 || got.Misses != 1 {
		t.Errorf("hits=%d misses=%d, want 0 and 1: the retry was steered by the review, not the lesson", got.Hits, got.Misses)
	}
	if !reflect.DeepEqual(report.NoSignal, []string{"L-001"}) {
		t.Errorf("report.NoSignal = %v, want [L-001]", report.NoSignal)
	}
	if LoadInjection(root, 7) != nil {
		t.Error("the merge should remove the record, missed list included")
	}
}

func TestSettle_RejectionKeepsAttributionForAReReview(t *testing.T) {
	root := settleRoot(t, testLesson("L-001", "config schema sync", StatusActive, []string{"config"}, nil))
	inject(t, root, 7, "L-001")

	// The review could not be verified. No Worker runs before the next
	// review, so the lessons of the reviewed attempt must still be on record.
	settle(t, root, SettleInput{
		IssueID: 7, Outcome: OutcomeReviewBlocked, RejectionText: "severity and score disagree", PathsKnown: true,
	})
	if LoadInjection(root, 7) == nil {
		t.Fatal("a rejection must not consume the injection record")
	}
	settle(t, root, mergedPR(7))

	if got := storedLesson(t, root, "L-001").Hits; got != 1 {
		t.Errorf("Hits = %d, want 1: the lesson was in the prompt of the merged attempt", got)
	}
}

func TestSettle_TheLessonsOwnSourceIssueIsNotAHit(t *testing.T) {
	l := testLesson("L-001", "config schema sync", StatusCandidate, []string{"config"}, nil)
	l.Evidence = []Evidence{{Issue: 7, PR: 70}}
	root := settleRoot(t, l)

	inject(t, root, 7, "L-001")
	settle(t, root, mergedPR(7))
	if got := storedLesson(t, root, "L-001").Hits; got != 0 {
		t.Errorf("Hits = %d, want 0: issue 7 is where the lesson came from", got)
	}

	inject(t, root, 8, "L-001")
	settle(t, root, mergedPR(8))
	if got := storedLesson(t, root, "L-001").Hits; got != 1 {
		t.Errorf("Hits = %d, want 1 from an unrelated issue", got)
	}
}

// ---------------------------------------------------------------------------
// Injection record
// ---------------------------------------------------------------------------

func TestRecordInjection_KeepsTheMissedListAcrossDispatches(t *testing.T) {
	root := settleRoot(t,
		testLesson("L-001", "config schema sync", StatusActive, []string{"config"}, nil),
		testLesson("L-002", "cover handlers with tests", StatusActive, []string{"test"}, nil),
	)
	inject(t, root, 7, "L-001")
	settle(t, root, rejectedPR(7, "the config schema was not updated"))

	// The next dispatch selects a different lesson.
	inject(t, root, 7, "L-002")
	rec := LoadInjection(root, 7)
	if rec == nil || !reflect.DeepEqual(rec.LessonIDs, []string{"L-002"}) || !reflect.DeepEqual(rec.Missed, []string{"L-001"}) {
		t.Fatalf("record = %+v, want lessons [L-002] and missed [L-001]", rec)
	}

	// A dispatch that selects nothing still remembers the miss.
	inject(t, root, 7)
	rec = LoadInjection(root, 7)
	if rec == nil || len(rec.LessonIDs) != 0 || !reflect.DeepEqual(rec.Missed, []string{"L-001"}) {
		t.Fatalf("record = %+v, want no lessons and missed [L-001]", rec)
	}
}

func TestRecordInjection_NothingToRememberRemovesTheRecord(t *testing.T) {
	root := t.TempDir()
	inject(t, root, 7, "L-001")
	inject(t, root, 7)
	if rec := LoadInjection(root, 7); rec != nil {
		t.Errorf("record = %+v, want none", rec)
	}
}

// ---------------------------------------------------------------------------
// State machine
// ---------------------------------------------------------------------------

func TestAdvanceStatus_ActiveToProven(t *testing.T) {
	l := &Lesson{Status: StatusActive, Hits: 5, Misses: 0}
	advanceStatus(l)
	if l.Status != StatusProven {
		t.Errorf("Status = %s, want proven", l.Status)
	}
	// High miss rate blocks proven.
	l2 := &Lesson{Status: StatusActive, Hits: 5, Misses: 2}
	advanceStatus(l2)
	if l2.Status != StatusActive {
		t.Errorf("Status = %s, want active (miss rate too high)", l2.Status)
	}
}
