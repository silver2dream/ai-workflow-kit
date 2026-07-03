package lessons

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testLesson(id, title string, status string, cats, scope []string) Lesson {
	return Lesson{
		ID: id, Title: title, Content: "- do the check", Kind: KindPitfall,
		Categories: cats, Scope: scope,
		Fingerprint: FingerprintOf(title, cats, scope),
		Status:      status, CreatedAt: "2026-06-01T00:00:00Z", Source: "human",
	}
}

// ---------------------------------------------------------------------------
// Store
// ---------------------------------------------------------------------------

func TestStore_LoadMissingReturnsEmpty(t *testing.T) {
	s, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Version != 1 || len(s.Lessons) != 0 {
		t.Errorf("expected empty v1 store, got %+v", s)
	}
}

func TestStore_SaveLoadRoundTrip(t *testing.T) {
	root := t.TempDir()
	s := &Store{Version: 1}
	s.Lessons = append(s.Lessons, testLesson("L-001", "sync schema on config change", StatusActive,
		[]string{"config"}, []string{"internal/analyzer/"}))
	s.Watermark.FeedbackLine = 42

	if err := Save(root, s); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Watermark.FeedbackLine != 42 || len(got.Lessons) != 1 || got.Lessons[0].ID != "L-001" {
		t.Errorf("round trip mismatch: %+v", got)
	}
}

func TestStore_LoadCorruptErrors(t *testing.T) {
	root := t.TempDir()
	path := StorePath(root)
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, []byte("{corrupt"), 0644)
	if _, err := Load(root); err == nil {
		t.Error("expected error for corrupt store")
	}
}

func TestStore_NextID(t *testing.T) {
	s := &Store{Lessons: []Lesson{{ID: "L-002"}, {ID: "L-010"}}}
	if got := s.NextID(); got != "L-011" {
		t.Errorf("NextID = %q, want L-011", got)
	}
	empty := &Store{}
	if got := empty.NextID(); got != "L-001" {
		t.Errorf("NextID on empty = %q, want L-001", got)
	}
}

func TestFingerprint_StableAndNormalized(t *testing.T) {
	a := FingerprintOf("Sync Schema on config change!", []string{"config"}, []string{"internal/analyzer/"})
	b := FingerprintOf("sync schema on CONFIG change", []string{"Config"}, []string{"internal\\analyzer\\"})
	if a != b {
		t.Errorf("fingerprints should normalize: %q vs %q", a, b)
	}
	c := FingerprintOf("different lesson entirely", []string{"test"}, nil)
	if a == c {
		t.Error("different lessons should not collide")
	}
}

func TestEnforceCaps_RetiresLowestScore(t *testing.T) {
	now := time.Now().UTC()
	s := &Store{Version: 1}
	for i := 0; i < 5; i++ {
		l := testLesson(fmt.Sprintf("L-%03d", i+1), fmt.Sprintf("lesson %d", i), StatusActive,
			[]string{"test"}, nil)
		l.Hits = i // L-001 has 0 hits (lowest score)
		l.LastHitAt = now.Format(time.RFC3339)
		s.Lessons = append(s.Lessons, l)
	}
	s.EnforceCaps(3, 10, now)

	retired := 0
	for _, l := range s.Lessons {
		if l.Status == StatusRetired {
			retired++
		}
	}
	if retired != 2 {
		t.Fatalf("expected 2 retired, got %d", retired)
	}
	if s.FindByID("L-001").Status != StatusRetired || s.FindByID("L-002").Status != StatusRetired {
		t.Error("lowest-hit lessons should be retired first")
	}
	if s.FindByID("L-005").Status != StatusActive {
		t.Error("highest-hit lesson should survive")
	}
}

// ---------------------------------------------------------------------------
// Select
// ---------------------------------------------------------------------------

func selectStore() *Store {
	s := &Store{Version: 1}
	s.Lessons = []Lesson{
		testLesson("L-001", "config schema sync", StatusActive, []string{"config"}, []string{"internal/analyzer/"}),
		testLesson("L-002", "test coverage for handlers", StatusActive, []string{"test"}, []string{"internal/worker/"}),
		testLesson("L-003", "security check inputs", StatusProven, []string{"security"}, nil),
		testLesson("L-004", "candidate one", StatusCandidate, []string{"config"}, []string{"internal/analyzer/"}),
		testLesson("L-005", "candidate two", StatusCandidate, []string{"config"}, []string{"internal/analyzer/"}),
		testLesson("L-006", "retired lesson", StatusRetired, []string{"config"}, []string{"internal/analyzer/"}),
	}
	return s
}

func TestSelect_ScopeAndCategoryRelevance(t *testing.T) {
	s := selectStore()
	q := Query{PathTokens: []string{"internal/analyzer/config.go"}, Categories: []string{"config"}}
	got := Select(s, q, 3, 800, time.Now().UTC())

	if len(got) == 0 {
		t.Fatal("expected selections")
	}
	if got[0].ID != "L-001" {
		t.Errorf("expected L-001 first (scope+category), got %s", got[0].ID)
	}
	for _, l := range got {
		if l.ID == "L-006" {
			t.Error("retired lesson must never be selected")
		}
		if l.ID == "L-002" {
			t.Error("irrelevant lesson (no scope/category match) selected")
		}
	}
}

func TestSelect_ZeroRelevanceReturnsNothing(t *testing.T) {
	s := selectStore()
	q := Query{PathTokens: []string{"frontend/src/ui.tsx"}, Categories: []string{"naming"}}
	if got := Select(s, q, 3, 800, time.Now().UTC()); len(got) != 0 {
		t.Errorf("expected no selections for irrelevant query, got %v", IDs(got))
	}
}

func TestSelect_AtMostOneCandidate(t *testing.T) {
	s := selectStore()
	q := Query{PathTokens: []string{"internal/analyzer/"}, Categories: []string{"config"}}
	got := Select(s, q, 3, 2000, time.Now().UTC())

	candidates := 0
	for _, l := range got {
		if l.Status == StatusCandidate {
			candidates++
		}
	}
	if candidates > 1 {
		t.Errorf("expected at most 1 candidate injected, got %d (%v)", candidates, IDs(got))
	}
}

func TestSelect_RespectsTopKAndBudget(t *testing.T) {
	s := selectStore()
	q := Query{PathTokens: []string{"internal/analyzer/"}, Categories: []string{"config", "security", "test"}}

	if got := Select(s, q, 1, 800, time.Now().UTC()); len(got) > 1 {
		t.Errorf("topK=1 violated: %v", IDs(got))
	}
	if got := Select(s, q, 5, 40, time.Now().UTC()); len(got) > 1 {
		t.Errorf("40-char budget should fit at most one line, got %v", IDs(got))
	}
}

func TestFormatForPrompt(t *testing.T) {
	sel := []Lesson{testLesson("L-001", "config schema sync", StatusActive, []string{"config"}, nil)}
	out := FormatForPrompt(sel)
	if !strings.Contains(out, "[L-001]") || !strings.Contains(out, "config schema sync") {
		t.Errorf("prompt format missing pieces:\n%s", out)
	}
	if FormatForPrompt(nil) != "" {
		t.Error("empty selection must render empty string")
	}
}

// ---------------------------------------------------------------------------
// Injection record + settlement
// ---------------------------------------------------------------------------

func TestSettle_MergedCountsHitsAndAdvances(t *testing.T) {
	root := t.TempDir()
	s := &Store{Version: 1}
	l := testLesson("L-001", "config schema sync", StatusCandidate, []string{"config"}, nil)
	l.Hits = 1 // one more clean outcome promotes to active
	s.Lessons = append(s.Lessons, l)
	if err := Save(root, s); err != nil {
		t.Fatal(err)
	}
	if err := RecordInjection(root, 7, []string{"L-001"}); err != nil {
		t.Fatal(err)
	}

	if err := Settle(root, 7, OutcomeMerged, ""); err != nil {
		t.Fatalf("Settle: %v", err)
	}

	got, _ := Load(root)
	gl := got.FindByID("L-001")
	if gl.Hits != 2 {
		t.Errorf("Hits = %d, want 2", gl.Hits)
	}
	if gl.Status != StatusActive {
		t.Errorf("Status = %s, want active (candidate promoted)", gl.Status)
	}
	if LoadInjection(root, 7) != nil {
		t.Error("injection record should be consumed after settlement")
	}
}

func TestSettle_MatchingRejectionCountsMiss(t *testing.T) {
	root := t.TempDir()
	s := &Store{Version: 1}
	l := testLesson("L-001", "config schema sync", StatusActive, []string{"config"}, nil)
	l.Misses = 2 // one more miss retires it
	s.Lessons = append(s.Lessons, l)
	Save(root, s)
	RecordInjection(root, 7, []string{"L-001"})

	err := Settle(root, 7, OutcomeChangesRequested, "the workflow.yaml schema was not updated")
	if err != nil {
		t.Fatalf("Settle: %v", err)
	}

	got, _ := Load(root)
	gl := got.FindByID("L-001")
	if gl.Misses != 3 {
		t.Errorf("Misses = %d, want 3", gl.Misses)
	}
	if gl.Status != StatusRetired {
		t.Errorf("Status = %s, want retired after 3 misses", gl.Status)
	}
}

func TestSettle_UnrelatedRejectionIsNoSignal(t *testing.T) {
	root := t.TempDir()
	s := &Store{Version: 1}
	s.Lessons = append(s.Lessons, testLesson("L-001", "config schema sync", StatusActive, []string{"config"}, nil))
	Save(root, s)
	RecordInjection(root, 7, []string{"L-001"})

	if err := Settle(root, 7, OutcomeChangesRequested, "variable naming is unclear, rename tmp"); err != nil {
		t.Fatal(err)
	}

	got, _ := Load(root)
	gl := got.FindByID("L-001")
	if gl.Hits != 0 || gl.Misses != 0 {
		t.Errorf("unrelated rejection must not move counters: hits=%d misses=%d", gl.Hits, gl.Misses)
	}
}

func TestSettle_NoInjectionIsNoop(t *testing.T) {
	root := t.TempDir()
	if err := Settle(root, 99, OutcomeMerged, ""); err != nil {
		t.Errorf("settle without injection record should be a silent noop: %v", err)
	}
}

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

// ---------------------------------------------------------------------------
// Distiller parsing (fail-closed)
// ---------------------------------------------------------------------------

func TestParseDistillOutput_Match(t *testing.T) {
	d, err := parseDistillOutput("Some preamble\nDECISION: MATCH L-007\n")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if d.Action != "MATCH" || d.MatchID != "L-007" {
		t.Errorf("got %+v", d)
	}
}

func TestParseDistillOutput_Noop(t *testing.T) {
	d, err := parseDistillOutput("DECISION: NOOP")
	if err != nil || d.Action != "NOOP" {
		t.Errorf("got %+v err=%v", d, err)
	}
}

func TestParseDistillOutput_NewFull(t *testing.T) {
	out := `DECISION: NEW
TITLE: sync schema when config struct changes
DESCRIPTION: schema drift caused validate failure
CONTENT:
- when editing internal/analyzer/config.go check workflow.schema.json
- update workflow.yaml comments
CATEGORIES: config, schema
SCOPE: internal/analyzer/, .ai/config/
`
	d, err := parseDistillOutput(out)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if d.Action != "NEW" || d.Title == "" || !strings.Contains(d.Content, "workflow.schema.json") {
		t.Errorf("got %+v", d)
	}
	if len(d.Categories) != 2 || len(d.Scope) != 2 {
		t.Errorf("categories/scope not parsed: %+v", d)
	}
}

func TestParseDistillOutput_FailClosed(t *testing.T) {
	cases := []string{
		"I think this is a new lesson about config",  // no DECISION
		"DECISION: NEW\nCONTENT:\n- check things\nCATEGORIES: config", // missing TITLE
		"DECISION: NEW\nTITLE: t\nCATEGORIES: config",                 // missing CONTENT
		"DECISION: NEW\nTITLE: t\nCONTENT:\n- c\n",                    // missing CATEGORIES
	}
	for i, c := range cases {
		if _, err := parseDistillOutput(c); err == nil {
			t.Errorf("case %d: expected fail-closed error", i)
		}
	}
}

// ---------------------------------------------------------------------------
// Distill end-to-end (stubbed LLM)
// ---------------------------------------------------------------------------

func writeFeedback(t *testing.T, root string, lines ...string) {
	t.Helper()
	path := filepath.Join(root, feedbackRelPath)
	os.MkdirAll(filepath.Dir(path), 0755)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestDistill_NewLessonCreated(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "internal", "analyzer"), 0755)
	writeFeedback(t, root,
		`{"timestamp":"2026-07-01T00:00:00Z","issue_id":42,"pr_number":9,"score":5,"categories":["config"],"summary":"schema not updated","paths":["internal/analyzer/config.go"]}`)

	orig := distillRunnerFunc
	distillRunnerFunc = func(ctx context.Context, prompt, model string) (string, error) {
		if !strings.Contains(prompt, "schema not updated") {
			t.Error("prompt missing rejection summary")
		}
		return `DECISION: NEW
TITLE: sync schema when config changes
DESCRIPTION: schema drift
CONTENT:
- check workflow.schema.json on config struct edits
CATEGORIES: config
SCOPE: internal/analyzer/, made/up/path/`, nil
	}
	defer func() { distillRunnerFunc = orig }()

	report, err := Distill(context.Background(), root, DistillOptions{})
	if err != nil {
		t.Fatalf("Distill: %v", err)
	}
	if len(report.Created) != 1 {
		t.Fatalf("expected 1 created, got %+v", report)
	}

	s, _ := Load(root)
	l := s.FindByID(report.Created[0])
	if l.Status != StatusCandidate {
		t.Errorf("new lesson must start as candidate, got %s", l.Status)
	}
	if len(l.Evidence) != 1 || l.Evidence[0].Issue != 42 {
		t.Errorf("evidence must be Go-written from the record: %+v", l.Evidence)
	}
	// Hallucinated path must be dropped; real path prefix kept.
	if len(l.Scope) != 1 || l.Scope[0] != "internal/analyzer/" {
		t.Errorf("scope validation failed: %v", l.Scope)
	}
	if s.Watermark.FeedbackLine != 1 {
		t.Errorf("watermark = %d, want 1", s.Watermark.FeedbackLine)
	}
}

func TestDistill_MatchUpvotes(t *testing.T) {
	root := t.TempDir()
	s := &Store{Version: 1}
	s.Lessons = append(s.Lessons, testLesson("L-001", "config schema sync", StatusActive, []string{"config"}, nil))
	Save(root, s)
	writeFeedback(t, root,
		`{"timestamp":"2026-07-01T00:00:00Z","issue_id":50,"pr_number":10,"score":5,"categories":["config"],"summary":"schema drift again"}`)

	orig := distillRunnerFunc
	distillRunnerFunc = func(ctx context.Context, prompt, model string) (string, error) {
		return "DECISION: MATCH L-001", nil
	}
	defer func() { distillRunnerFunc = orig }()

	report, err := Distill(context.Background(), root, DistillOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Matched) != 1 {
		t.Fatalf("expected 1 matched: %+v", report)
	}
	got, _ := Load(root)
	if got.FindByID("L-001").Hits != 1 {
		t.Errorf("MATCH should upvote hits")
	}
}

func TestDistill_DuplicateNewBecomesMatch(t *testing.T) {
	root := t.TempDir()
	existing := testLesson("L-001", "sync schema when config changes", StatusActive, []string{"config"}, nil)
	s := &Store{Version: 1, Lessons: []Lesson{existing}}
	Save(root, s)
	writeFeedback(t, root,
		`{"timestamp":"2026-07-01T00:00:00Z","issue_id":51,"pr_number":11,"score":4,"categories":["config"],"summary":"same problem"}`)

	orig := distillRunnerFunc
	distillRunnerFunc = func(ctx context.Context, prompt, model string) (string, error) {
		return `DECISION: NEW
TITLE: Sync Schema when CONFIG changes!
DESCRIPTION: dup
CONTENT:
- same check
CATEGORIES: config
SCOPE:`, nil
	}
	defer func() { distillRunnerFunc = orig }()

	report, err := Distill(context.Background(), root, DistillOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Created) != 0 || len(report.Matched) != 1 {
		t.Errorf("fingerprint-duplicate NEW should convert to MATCH: %+v", report)
	}
}

func TestDistill_GarbledOutputSkippedWatermarkAdvances(t *testing.T) {
	root := t.TempDir()
	writeFeedback(t, root,
		`{"timestamp":"2026-07-01T00:00:00Z","issue_id":60,"pr_number":12,"score":3,"categories":["test"],"summary":"missing tests"}`)

	orig := distillRunnerFunc
	distillRunnerFunc = func(ctx context.Context, prompt, model string) (string, error) {
		return "sure! I think you should add more tests :)", nil
	}
	defer func() { distillRunnerFunc = orig }()

	report, err := Distill(context.Background(), root, DistillOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Skipped != 1 || len(report.Created) != 0 {
		t.Errorf("garbled output must be skipped: %+v", report)
	}
	s, _ := Load(root)
	if s.Watermark.FeedbackLine != 1 {
		t.Errorf("watermark must advance past garbled entries, got %d", s.Watermark.FeedbackLine)
	}
}

func TestDistill_ApprovedEntriesNotDistilled(t *testing.T) {
	root := t.TempDir()
	writeFeedback(t, root,
		`{"timestamp":"2026-07-01T00:00:00Z","issue_id":61,"pr_number":13,"score":9,"categories":[],"summary":"merged fine","outcome":"approved"}`)

	called := false
	orig := distillRunnerFunc
	distillRunnerFunc = func(ctx context.Context, prompt, model string) (string, error) {
		called = true
		return "DECISION: NOOP", nil
	}
	defer func() { distillRunnerFunc = orig }()

	if _, err := Distill(context.Background(), root, DistillOptions{}); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Error("approved entries must not trigger the distiller LLM")
	}
	s, _ := Load(root)
	if s.Watermark.FeedbackLine != 1 {
		t.Errorf("watermark must still advance, got %d", s.Watermark.FeedbackLine)
	}
}

func TestDistill_AllRunnerFailuresPreserveWatermark(t *testing.T) {
	root := t.TempDir()
	writeFeedback(t, root,
		`{"timestamp":"2026-07-01T00:00:00Z","issue_id":70,"pr_number":14,"score":4,"categories":["test"],"summary":"missing tests"}`)

	orig := distillRunnerFunc
	distillRunnerFunc = func(ctx context.Context, prompt, model string) (string, error) {
		return "", fmt.Errorf("claude CLI not found in PATH")
	}
	defer func() { distillRunnerFunc = orig }()

	report, err := Distill(context.Background(), root, DistillOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.RunnerErrors != 1 {
		t.Errorf("RunnerErrors = %d, want 1", report.RunnerErrors)
	}
	s, _ := Load(root)
	if s.Watermark.FeedbackLine != 0 {
		t.Errorf("watermark must be preserved when every LLM call failed, got %d", s.Watermark.FeedbackLine)
	}
}
