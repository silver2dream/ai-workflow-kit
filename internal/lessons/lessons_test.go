package lessons

import (
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
