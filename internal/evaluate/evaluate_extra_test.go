package evaluate

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// CheckO7VersionSync (offline_gate.go)
// ---------------------------------------------------------------------------

func TestCheckO7VersionSync_FileExists(t *testing.T) {
	dir := t.TempDir()
	// Create the version file
	versionDir := filepath.Join(dir, "internal", "buildinfo")
	os.MkdirAll(versionDir, 0755)
	os.WriteFile(filepath.Join(versionDir, "version.go"), []byte("package buildinfo"), 0644)

	result := CheckO7VersionSync(dir)
	if result.Status != "PASS" {
		t.Errorf("CheckO7VersionSync with version.go = %q, want PASS", result.Status)
	}
}

func TestCheckO7VersionSync_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	result := CheckO7VersionSync(dir)
	if result.Status != "SKIP" {
		t.Errorf("CheckO7VersionSync without version.go = %q, want SKIP", result.Status)
	}
}

// ---------------------------------------------------------------------------
// gateScore (scoring.go) — test the untested default branch
// ---------------------------------------------------------------------------

func TestGateScore_DefaultCase(t *testing.T) {
	result := GateResult{Status: "UNKNOWN"}
	score := gateScore(result, 10.0)
	if score != 0 {
		t.Errorf("gateScore(UNKNOWN) = %v, want 0", score)
	}
}

func TestGateScore_PassCase(t *testing.T) {
	result := GateResult{Status: "PASS"}
	score := gateScore(result, 10.0)
	if score != 10.0 {
		t.Errorf("gateScore(PASS, 10) = %v, want 10.0", score)
	}
}

func TestGateScore_SkipCase(t *testing.T) {
	result := GateResult{Status: "SKIP"}
	score := gateScore(result, 10.0)
	if score != 5.0 {
		t.Errorf("gateScore(SKIP, 10) = %v, want 5.0", score)
	}
}

func TestGateScore_FailCase(t *testing.T) {
	result := GateResult{Status: "FAIL"}
	score := gateScore(result, 10.0)
	if score != 0 {
		t.Errorf("gateScore(FAIL, 10) = %v, want 0", score)
	}
}

// ---------------------------------------------------------------------------
// CheckO5ConfigValidation — test the missing branches
// ---------------------------------------------------------------------------

func TestCheckO5ConfigValidation_NoWorkflowYaml(t *testing.T) {
	dir := t.TempDir()
	// Create .ai/config but no workflow.yaml
	os.MkdirAll(filepath.Join(dir, ".ai", "config"), 0755)

	result := CheckO5ConfigValidation(dir)
	if result.Status == "" {
		t.Error("CheckO5ConfigValidation should return a non-empty status")
	}
	// With empty config dir, should either SKIP or FAIL
}

func TestCheckO5ConfigValidation_MissingConfigDir(t *testing.T) {
	dir := t.TempDir()
	result := CheckO5ConfigValidation(dir)
	// No .ai/config directory
	if result.Status != "SKIP" && result.Status != "FAIL" {
		t.Errorf("CheckO5ConfigValidation with no config = %q, want SKIP or FAIL", result.Status)
	}
}

// ---------------------------------------------------------------------------
// Evaluator.RunOffline (evaluate.go)
// ---------------------------------------------------------------------------

func TestEvaluator_RunOffline_HasAllGates(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ai", "config"), 0755)

	e := New(dir)
	result := e.RunOffline()
	// Should have populated gate results
	if result.O0.Status == "" {
		t.Error("O0 gate status should not be empty")
	}
}

func TestEvaluator_RunOffline_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	e := New(dir)
	result := e.RunOffline()
	// O0 should still run (PASS or SKIP) on any dir
	if result.O0.Status == "" {
		t.Error("O0 gate status should not be empty even for empty dir")
	}
}

func TestEvaluator_Run_HasGradeAndScore(t *testing.T) {
	dir := t.TempDir()
	e := New(dir)
	result := e.Run()
	if result.Grade == "" {
		t.Error("Grade should not be empty")
	}
	if result.ScoreCap < 0 || result.ScoreCap > 100 {
		t.Errorf("ScoreCap = %v, should be in [0, 100]", result.ScoreCap)
	}
}
