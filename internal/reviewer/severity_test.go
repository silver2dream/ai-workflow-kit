package reviewer

import "testing"

func TestParseSeverityCounts_BoldForm(t *testing.T) {
	body := `### Suggested Improvements

- **Critical:** auth.go:42 — token comparison is non-constant-time.
- **Important:** engine.go:118 — no test for diagonal entry.
- **Nit:** engine.go:55 — variable tmp would read clearer as nextHead.
- **Optional:** consider extracting the helper.
- **FYI:** room.go:90 — same pattern duplicated in lobby.go:30.
`
	got := ParseSeverityCounts(body)
	want := SeverityCounts{Critical: 1, Important: 1, Nit: 1, Optional: 1, FYI: 1}
	if got != want {
		t.Errorf("ParseSeverityCounts = %+v, want %+v", got, want)
	}
}

func TestParseSeverityCounts_PlainAndVariants(t *testing.T) {
	body := `
- Critical: plain form without bold
* important: lowercase with star marker
1. **Consider:** numbered list counts as Optional
- **Critical**: colon outside the bold
`
	got := ParseSeverityCounts(body)
	if got.Critical != 2 {
		t.Errorf("Critical = %d, want 2", got.Critical)
	}
	if got.Important != 1 {
		t.Errorf("Important = %d, want 1", got.Important)
	}
	if got.Optional != 1 {
		t.Errorf("Optional (Consider) = %d, want 1", got.Optional)
	}
}

func TestParseSeverityCounts_IgnoresNonListLines(t *testing.T) {
	body := `
| **Critical:** | Blocks merge | Must fix |
Critical: prose without a list marker is not a finding
The word Important: mid-sentence should not count either.
None
`
	got := ParseSeverityCounts(body)
	if got.Total() != 0 {
		t.Errorf("expected 0 findings, got %+v", got)
	}
}

func TestValidateSeverityConsistency_ChangesRequestedNeedsCriticalOrImportant(t *testing.T) {
	body := `### Suggested Improvements
- **Nit:** rename tmp.
`
	err := ValidateSeverityConsistency(5, 7, body)
	if err == nil {
		t.Fatal("expected inconsistency error for changes_requested without Critical/Important")
	}
	if err.Code != 4 {
		t.Errorf("Code = %d, want 4", err.Code)
	}
}

func TestValidateSeverityConsistency_ChangesRequestedWithImportantOK(t *testing.T) {
	body := `- **Important:** engine.go:118 — missing test.`
	if err := ValidateSeverityConsistency(5, 7, body); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateSeverityConsistency_ApproveWithCriticalBlocked(t *testing.T) {
	body := `- **Critical:** auth.go:42 — timing side channel.`
	err := ValidateSeverityConsistency(8, 7, body)
	if err == nil {
		t.Fatal("expected inconsistency error for approve with Critical finding")
	}
	if err.Code != 4 {
		t.Errorf("Code = %d, want 4", err.Code)
	}
}

func TestValidateSeverityConsistency_ApproveCleanOK(t *testing.T) {
	body := `### Suggested Improvements
None
- **Nit:** cosmetic only.
- **FYI:** duplicated pattern elsewhere.
`
	if err := ValidateSeverityConsistency(9, 7, body); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateSeverityConsistency_ApproveWithImportantOK(t *testing.T) {
	// Important does not block approval by contract; only Critical does.
	body := `- **Important:** should improve logging before next release.`
	if err := ValidateSeverityConsistency(8, 7, body); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
