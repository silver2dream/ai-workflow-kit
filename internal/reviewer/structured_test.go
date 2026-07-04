package reviewer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validReviewJSON = `{
  "score": 8,
  "summary": "solid implementation with good coverage",
  "criteria": [
    {
      "criterion": "Wall collision ends game",
      "implementation": "HandleCollision() at engine.go:145 sets game.State = GameOver on boundary hit",
      "test_name": "TestCollisionScenarios",
      "assertion": "assert.Equal(t, GameOver, game.State)"
    },
    { "criterion": "All tests pass", "meta": true }
  ],
  "improvements": [
    { "severity": "nit", "location": "engine.go:55", "text": "rename tmp to nextHead" }
  ],
  "risks": "None identified"
}`

func TestParseStructuredReview_Valid(t *testing.T) {
	r, errs := ParseStructuredReview([]byte(validReviewJSON))
	if len(errs) > 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}
	if r.Score != 8 || len(r.Criteria) != 2 || len(r.Improvements) != 1 {
		t.Errorf("parsed wrong: %+v", r)
	}
	if !r.Criteria[1].Meta {
		t.Error("meta flag not parsed")
	}
}

func TestParseStructuredReview_InvalidJSON(t *testing.T) {
	_, errs := ParseStructuredReview([]byte("{not json"))
	if len(errs) != 1 || !strings.Contains(errs[0].Problem, "invalid JSON") {
		t.Errorf("expected invalid-JSON error, got %v", errs)
	}
}

func TestParseStructuredReview_UnknownFieldCaught(t *testing.T) {
	// Typo "tests_name" must be rejected, not silently dropped.
	bad := `{"score": 8, "criteria": [{"criterion": "c", "implementation": "long enough description here", "tests_name": "TestX", "assertion": "a"}]}`
	_, errs := ParseStructuredReview([]byte(bad))
	if len(errs) == 0 {
		t.Fatal("expected error for unknown field typo")
	}
	if !strings.Contains(errs[0].Problem, "unknown field") && !strings.Contains(errs[0].Problem, "tests_name") {
		t.Errorf("error should mention the typo: %v", errs)
	}
}

func TestParseStructuredReview_FieldErrorsWithHints(t *testing.T) {
	bad := `{
	  "score": 0,
	  "criteria": [
	    { "criterion": "", "implementation": "short", "test_name": "", "assertion": "" }
	  ],
	  "improvements": [ { "severity": "blocker", "text": "" } ]
	}`
	_, errs := ParseStructuredReview([]byte(bad))
	if len(errs) < 5 {
		t.Fatalf("expected multiple field errors, got %d: %v", len(errs), errs)
	}

	fields := make(map[string]bool)
	hints := 0
	for _, e := range errs {
		fields[e.Field] = true
		if e.Hint != "" {
			hints++
		}
	}
	for _, want := range []string{"score", "criteria[0].criterion", "criteria[0].implementation", "criteria[0].test_name", "criteria[0].assertion", "improvements[0].severity"} {
		if !fields[want] {
			t.Errorf("missing error for field %s (got %v)", want, errs)
		}
	}
	if hints == 0 {
		t.Error("validation errors should carry actionable hints")
	}
}

func TestParseStructuredReview_MetaSkipsEvidenceFields(t *testing.T) {
	ok := `{"score": 7, "criteria": [{"criterion": "All tests pass", "meta": true}]}`
	r, errs := ParseStructuredReview([]byte(ok))
	if len(errs) > 0 {
		t.Fatalf("meta criteria must not require evidence fields: %v", errs)
	}
	if !r.Criteria[0].Meta {
		t.Error("meta not set")
	}
}

func TestStructuredReview_ToVerifications(t *testing.T) {
	r, _ := ParseStructuredReview([]byte(validReviewJSON))
	vs := r.ToVerifications()
	if len(vs) != 2 {
		t.Fatalf("expected 2 verifications, got %d", len(vs))
	}
	if vs[0].TestName != "TestCollisionScenarios" || vs[0].Assertion != "assert.Equal(t, GameOver, game.State)" {
		t.Errorf("verification mapping wrong: %+v", vs[0])
	}
	if !vs[1].IsMeta {
		t.Error("meta flag must map to IsMeta")
	}
}

func TestStructuredReview_SeverityCounts(t *testing.T) {
	j := `{"score": 5, "criteria": [{"criterion": "c", "meta": true}], "improvements": [
	  {"severity": "critical", "text": "a"},
	  {"severity": "Important", "text": "b"},
	  {"severity": "consider", "text": "c"}
	]}`
	r, errs := ParseStructuredReview([]byte(j))
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	counts := r.SeverityCounts()
	if counts.Critical != 1 || counts.Important != 1 || counts.Optional != 1 {
		t.Errorf("counts wrong: %+v", counts)
	}
}

func TestStructuredReview_RenderMarkdown(t *testing.T) {
	r, _ := ParseStructuredReview([]byte(validReviewJSON))
	md := r.RenderMarkdown()

	for _, want := range []string{
		"### Implementation Review",
		"#### 1. Wall collision ends game",
		"**Implementation**: HandleCollision()",
		"### Test Review",
		"| Wall collision ends game | TestCollisionScenarios | assert.Equal(t, GameOver, game.State) |",
		"### Score Reason",
		"### Suggested Improvements",
		"- **Nit:** `engine.go:55` — rename tmp to nextHead",
		"### Potential Risks",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("rendered markdown missing %q:\n%s", want, md)
		}
	}
}

func TestStructuredReview_RenderMarkdown_EscapesPipes(t *testing.T) {
	j := `{"score": 7, "criteria": [{"criterion": "supports a|b syntax", "implementation": "parse alternation in matcher.go:10 with split", "test_name": "TestAlternation", "assertion": "require.True(t, ok)"}]}`
	r, errs := ParseStructuredReview([]byte(j))
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	md := r.RenderMarkdown()
	if !strings.Contains(md, `a\|b`) {
		t.Errorf("pipe in criterion must be escaped in table:\n%s", md)
	}
}

// TestVerifyTestEvidence_DirectVerificationsBypassParsing proves the
// structured path skips markdown parsing entirely: the review body is
// garbage, but direct verifications succeed.
func TestVerifyTestEvidence_DirectVerificationsBypassParsing(t *testing.T) {
	tmpDir := t.TempDir()

	testContent := `package game
import "testing"
func TestCollisionScenarios(t *testing.T) {
	// assert.Equal(t, GameOver, game.State)
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "engine_test.go"), []byte(testContent), 0644); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(tmpDir, "run-test.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho '--- PASS: TestCollisionScenarios'\necho 'ok'\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}

	err := VerifyTestEvidence(context.Background(), VerifyOptions{
		Ticket:       "## Acceptance Criteria\n- [ ] Wall collision ends game",
		ReviewBody:   "THIS IS NOT PARSEABLE MARKDOWN AT ALL",
		WorktreePath: tmpDir,
		TestCommand:  "sh " + filepath.ToSlash(script),
		Language:     "go",
		Verifications: []CriteriaVerification{{
			Criteria:       "Wall collision ends game",
			Implementation: "HandleCollision() at engine.go:145 sets game.State = GameOver",
			TestName:       "TestCollisionScenarios",
			Assertion:      "assert.Equal(t, GameOver, game.State)",
		}},
	})
	if err != nil {
		t.Errorf("direct verifications should bypass body parsing, got: code=%d msg=%s details=%v", err.Code, err.Message, err.Details)
	}
}
