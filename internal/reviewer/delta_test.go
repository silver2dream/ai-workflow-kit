package reviewer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestComputeFindingID_Deterministic(t *testing.T) {
	id1 := ComputeFindingID("main.go", "security", "SQL injection risk")
	id2 := ComputeFindingID("main.go", "security", "SQL injection risk")

	if id1 != id2 {
		t.Errorf("same inputs should produce same ID: %q != %q", id1, id2)
	}

	if len(id1) != 16 {
		t.Errorf("expected ID length 16, got %d", len(id1))
	}
}

func TestComputeFindingID_DifferentInputs(t *testing.T) {
	id1 := ComputeFindingID("main.go", "security", "SQL injection risk")
	id2 := ComputeFindingID("main.go", "security", "XSS vulnerability")
	id3 := ComputeFindingID("other.go", "security", "SQL injection risk")

	if id1 == id2 {
		t.Error("different body should produce different ID")
	}
	if id1 == id3 {
		t.Error("different file should produce different ID")
	}
}

func TestComputeDelta_AllNew(t *testing.T) {
	current := []Finding{
		{File: "a.go", Category: "style", Body: "missing comment"},
		{File: "b.go", Category: "security", Body: "unsafe input"},
	}

	result := ComputeDelta(nil, current)

	if len(result.New) != 2 {
		t.Errorf("expected 2 new findings, got %d", len(result.New))
	}
	if len(result.Fixed) != 0 {
		t.Errorf("expected 0 fixed findings, got %d", len(result.Fixed))
	}
	if len(result.Persisting) != 0 {
		t.Errorf("expected 0 persisting findings, got %d", len(result.Persisting))
	}
}

func TestComputeDelta_AllFixed(t *testing.T) {
	previous := []Finding{
		{File: "a.go", Category: "style", Body: "missing comment"},
	}

	result := ComputeDelta(previous, nil)

	if len(result.New) != 0 {
		t.Errorf("expected 0 new findings, got %d", len(result.New))
	}
	if len(result.Fixed) != 1 {
		t.Errorf("expected 1 fixed finding, got %d", len(result.Fixed))
	}
	if len(result.Persisting) != 0 {
		t.Errorf("expected 0 persisting findings, got %d", len(result.Persisting))
	}
}

func TestComputeDelta_Mixed(t *testing.T) {
	shared := Finding{File: "a.go", Category: "style", Body: "missing comment"}
	onlyPrev := Finding{File: "b.go", Category: "security", Body: "unsafe input"}
	onlyCurr := Finding{File: "c.go", Category: "perf", Body: "slow loop"}

	previous := []Finding{shared, onlyPrev}
	current := []Finding{shared, onlyCurr}

	result := ComputeDelta(previous, current)

	if len(result.New) != 1 {
		t.Errorf("expected 1 new finding, got %d", len(result.New))
	}
	if len(result.Fixed) != 1 {
		t.Errorf("expected 1 fixed finding, got %d", len(result.Fixed))
	}
	if len(result.Persisting) != 1 {
		t.Errorf("expected 1 persisting finding, got %d", len(result.Persisting))
	}

	// Verify the correct findings are in each bucket
	if result.New[0].File != "c.go" {
		t.Errorf("expected new finding for c.go, got %s", result.New[0].File)
	}
	if result.Fixed[0].File != "b.go" {
		t.Errorf("expected fixed finding for b.go, got %s", result.Fixed[0].File)
	}
	if result.Persisting[0].File != "a.go" {
		t.Errorf("expected persisting finding for a.go, got %s", result.Persisting[0].File)
	}
}

func TestComputeDelta_WithPrecomputedIDs(t *testing.T) {
	id := ComputeFindingID("a.go", "style", "missing comment")

	previous := []Finding{
		{ID: id, File: "a.go", Category: "style", Body: "missing comment"},
	}
	current := []Finding{
		{ID: id, File: "a.go", Category: "style", Body: "missing comment"},
	}

	result := ComputeDelta(previous, current)

	if len(result.Persisting) != 1 {
		t.Errorf("expected 1 persisting with precomputed IDs, got %d", len(result.Persisting))
	}
	if len(result.New) != 0 {
		t.Errorf("expected 0 new, got %d", len(result.New))
	}
}

func TestComputeDelta_EmptyBoth(t *testing.T) {
	result := ComputeDelta(nil, nil)

	if len(result.New) != 0 || len(result.Fixed) != 0 || len(result.Persisting) != 0 {
		t.Error("expected all empty for nil inputs")
	}
}

func TestSaveAndLoadFindings(t *testing.T) {
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, ".ai", "state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}

	findings := []Finding{
		{ID: "abc123", File: "main.go", Category: "security", Body: "SQL injection"},
		{ID: "def456", File: "handler.go", Category: "style", Body: "missing docs"},
	}

	// Save
	if err := SaveFindings(tmpDir, 42, findings); err != nil {
		t.Fatalf("SaveFindings failed: %v", err)
	}

	// Load
	loaded, err := LoadPreviousFindings(tmpDir, 42)
	if err != nil {
		t.Fatalf("LoadPreviousFindings failed: %v", err)
	}

	if len(loaded) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(loaded))
	}

	if loaded[0].ID != "abc123" || loaded[0].File != "main.go" {
		t.Errorf("unexpected first finding: %+v", loaded[0])
	}
	if loaded[1].ID != "def456" || loaded[1].File != "handler.go" {
		t.Errorf("unexpected second finding: %+v", loaded[1])
	}
}

func TestLoadPreviousFindings_NotExists(t *testing.T) {
	tmpDir := t.TempDir()

	findings, err := LoadPreviousFindings(tmpDir, 999)
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if findings != nil {
		t.Errorf("expected nil findings for missing file, got %v", findings)
	}
}

func TestFormatDeltaReport_Empty(t *testing.T) {
	result := FormatDeltaReport(DeltaResult{})
	if result != "" {
		t.Errorf("expected empty string for empty delta, got: %q", result)
	}
}

func TestFormatDeltaReport_Mixed(t *testing.T) {
	delta := DeltaResult{
		New:        []Finding{{File: "new.go", Body: "new issue"}},
		Fixed:      []Finding{{File: "fixed.go", Body: "was broken"}},
		Persisting: []Finding{{File: "old.go", Body: "still there"}},
	}

	report := FormatDeltaReport(delta)

	for _, want := range []string{
		"[NEW]", "[FIXED]", "[PERSISTING]",
		"new.go", "fixed.go", "old.go",
		"Delta from Previous Review",
	} {
		if !strContains(report, want) {
			t.Errorf("report missing expected text: %q", want)
		}
	}
}
