package reset

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// New
// ---------------------------------------------------------------------------

func TestNew_DefaultStateRoot(t *testing.T) {
	r := New("")
	if r.StateRoot != "." {
		t.Errorf("New(\"\").StateRoot = %q, want %q", r.StateRoot, ".")
	}
}

func TestNew_CustomStateRoot(t *testing.T) {
	r := New("/tmp/test")
	if r.StateRoot != "/tmp/test" {
		t.Errorf("New(\"/tmp/test\").StateRoot = %q, want %q", r.StateRoot, "/tmp/test")
	}
}

func TestSetDryRun(t *testing.T) {
	r := New(".")
	if r.DryRun {
		t.Error("DryRun should default to false")
	}
	r.SetDryRun(true)
	if !r.DryRun {
		t.Error("DryRun should be true after SetDryRun(true)")
	}
}

// ---------------------------------------------------------------------------
// ResetState
// ---------------------------------------------------------------------------

func TestResetState_DeletesExistingFiles(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".ai", "state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}

	loopCount := filepath.Join(stateDir, "loop_count")
	failures := filepath.Join(stateDir, "consecutive_failures")
	if err := os.WriteFile(loopCount, []byte("5"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(failures, []byte("2"), 0644); err != nil {
		t.Fatal(err)
	}

	r := New(dir)
	results := r.ResetState()

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, res := range results {
		if !res.Success {
			t.Errorf("result %s failed: %s", res.Name, res.Message)
		}
	}

	if _, err := os.Stat(loopCount); !os.IsNotExist(err) {
		t.Error("loop_count should have been deleted")
	}
	if _, err := os.Stat(failures); !os.IsNotExist(err) {
		t.Error("consecutive_failures should have been deleted")
	}
}

func TestResetState_NoFiles_NoResults(t *testing.T) {
	dir := t.TempDir()
	r := New(dir)
	results := r.ResetState()
	if len(results) != 0 {
		t.Errorf("expected 0 results when files don't exist, got %d", len(results))
	}
}

func TestResetState_DryRun(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".ai", "state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	loopCount := filepath.Join(stateDir, "loop_count")
	if err := os.WriteFile(loopCount, []byte("3"), 0644); err != nil {
		t.Fatal(err)
	}

	r := New(dir)
	r.SetDryRun(true)
	results := r.ResetState()

	if len(results) == 0 {
		t.Fatal("expected at least 1 result in dry-run mode")
	}
	for _, res := range results {
		if !res.Success {
			t.Errorf("dry-run result should succeed: %s", res.Message)
		}
	}

	// File should NOT be deleted
	if _, err := os.Stat(loopCount); err != nil {
		t.Error("loop_count should NOT be deleted in dry-run mode")
	}
}

// ---------------------------------------------------------------------------
// ResetStop
// ---------------------------------------------------------------------------

func TestResetStop_RemovesMarker(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".ai", "state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}

	stopFile := filepath.Join(stateDir, "STOP")
	if err := os.WriteFile(stopFile, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	r := New(dir)
	result := r.ResetStop()

	if !result.Success {
		t.Errorf("ResetStop failed: %s", result.Message)
	}

	if _, err := os.Stat(stopFile); !os.IsNotExist(err) {
		t.Error("STOP file should have been deleted")
	}
}

func TestResetStop_NoMarker_ReturnsOK(t *testing.T) {
	dir := t.TempDir()
	r := New(dir)
	result := r.ResetStop()

	if !result.Success {
		t.Errorf("ResetStop when no file: %s", result.Message)
	}
	if result.Message != "Not present" {
		t.Errorf("Message = %q, want %q", result.Message, "Not present")
	}
}

func TestResetStop_DryRun(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".ai", "state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	stopFile := filepath.Join(stateDir, "STOP")
	if err := os.WriteFile(stopFile, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	r := New(dir)
	r.SetDryRun(true)
	result := r.ResetStop()

	if !result.Success {
		t.Errorf("dry-run ResetStop failed: %s", result.Message)
	}
	// File should NOT be deleted
	if _, err := os.Stat(stopFile); err != nil {
		t.Error("STOP file should NOT be deleted in dry-run mode")
	}
}

// ---------------------------------------------------------------------------
// ResetLock
// ---------------------------------------------------------------------------

func TestResetLock_RemovesLockFile(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".ai", "state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}

	lockFile := filepath.Join(stateDir, "kickoff.lock")
	if err := os.WriteFile(lockFile, []byte("12345"), 0644); err != nil {
		t.Fatal(err)
	}

	r := New(dir)
	result := r.ResetLock()

	if !result.Success {
		t.Errorf("ResetLock failed: %s", result.Message)
	}
	if _, err := os.Stat(lockFile); !os.IsNotExist(err) {
		t.Error("kickoff.lock should have been deleted")
	}
}

func TestResetLock_NoFile_ReturnsOK(t *testing.T) {
	dir := t.TempDir()
	r := New(dir)
	result := r.ResetLock()
	if !result.Success {
		t.Errorf("ResetLock when no file: %s", result.Message)
	}
}

func TestResetLock_DryRun(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".ai", "state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	lockFile := filepath.Join(stateDir, "kickoff.lock")
	if err := os.WriteFile(lockFile, []byte("12345"), 0644); err != nil {
		t.Fatal(err)
	}

	r := New(dir)
	r.SetDryRun(true)
	result := r.ResetLock()

	if !result.Success {
		t.Errorf("dry-run ResetLock failed: %s", result.Message)
	}
	if _, err := os.Stat(lockFile); err != nil {
		t.Error("lock file should NOT be deleted in dry-run")
	}
}

// ---------------------------------------------------------------------------
// ResetAttempts
// ---------------------------------------------------------------------------

func TestResetAttempts_RemovesRunsDir(t *testing.T) {
	dir := t.TempDir()
	runsDir := filepath.Join(dir, ".ai", "runs")
	if err := os.MkdirAll(filepath.Join(runsDir, "issue-1"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runsDir, "issue-1", "fail_count.txt"), []byte("2"), 0644); err != nil {
		t.Fatal(err)
	}

	r := New(dir)
	results := r.ResetAttempts()

	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	for _, res := range results {
		if !res.Success {
			t.Errorf("ResetAttempts result failed: %s", res.Message)
		}
	}

	if _, err := os.Stat(runsDir); !os.IsNotExist(err) {
		t.Error("runs dir should have been removed")
	}
}

func TestResetAttempts_NoDirs_NoResults(t *testing.T) {
	dir := t.TempDir()
	r := New(dir)
	results := r.ResetAttempts()
	if len(results) != 0 {
		t.Errorf("expected 0 results when dirs don't exist, got %d", len(results))
	}
}

func TestResetAttempts_DryRun(t *testing.T) {
	dir := t.TempDir()
	runsDir := filepath.Join(dir, ".ai", "runs", "issue-1")
	if err := os.MkdirAll(runsDir, 0755); err != nil {
		t.Fatal(err)
	}

	r := New(dir)
	r.SetDryRun(true)
	results := r.ResetAttempts()

	if len(results) == 0 {
		t.Fatal("expected at least 1 result in dry-run mode")
	}
	// Dir should still exist
	if _, err := os.Stat(filepath.Join(dir, ".ai", "runs")); err != nil {
		t.Error("runs dir should NOT be deleted in dry-run mode")
	}
}

// ---------------------------------------------------------------------------
// ResetDeprecated
// ---------------------------------------------------------------------------

func TestResetDeprecated_RemovesFiles(t *testing.T) {
	dir := t.TempDir()

	// Create one deprecated file
	deprecatedPath := filepath.Join(dir, ".ai", "skills", "principal-workflow", "tasks")
	if err := os.MkdirAll(deprecatedPath, 0755); err != nil {
		t.Fatal(err)
	}
	reviewFile := filepath.Join(deprecatedPath, "review-pr.md")
	if err := os.WriteFile(reviewFile, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}

	r := New(dir)
	results := r.ResetDeprecated()

	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	for _, res := range results {
		if !res.Success {
			t.Errorf("ResetDeprecated failed: %s", res.Message)
		}
	}

	if _, err := os.Stat(reviewFile); !os.IsNotExist(err) {
		t.Error("deprecated file should have been removed")
	}
}

func TestResetDeprecated_NoFiles_NoResults(t *testing.T) {
	dir := t.TempDir()
	r := New(dir)
	results := r.ResetDeprecated()
	if len(results) != 0 {
		t.Errorf("expected 0 results when no deprecated files, got %d", len(results))
	}
}

// ---------------------------------------------------------------------------
// ResetTraces
// ---------------------------------------------------------------------------

func TestResetTraces_RemovesDir(t *testing.T) {
	dir := t.TempDir()
	tracesDir := filepath.Join(dir, ".ai", "state", "traces")
	if err := os.MkdirAll(tracesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tracesDir, "issue-1.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	r := New(dir)
	results := r.ResetTraces()

	if len(results) == 0 {
		t.Fatal("expected 1 result")
	}
	if !results[0].Success {
		t.Errorf("ResetTraces failed: %s", results[0].Message)
	}
	if _, err := os.Stat(tracesDir); !os.IsNotExist(err) {
		t.Error("traces dir should have been removed")
	}
}

func TestResetTraces_NoDir_ReturnsNil(t *testing.T) {
	dir := t.TempDir()
	r := New(dir)
	results := r.ResetTraces()
	if len(results) != 0 {
		t.Errorf("expected nil when traces dir doesn't exist, got %d results", len(results))
	}
}

func TestResetTraces_DryRun(t *testing.T) {
	dir := t.TempDir()
	tracesDir := filepath.Join(dir, ".ai", "state", "traces")
	if err := os.MkdirAll(tracesDir, 0755); err != nil {
		t.Fatal(err)
	}

	r := New(dir)
	r.SetDryRun(true)
	results := r.ResetTraces()

	if len(results) == 0 {
		t.Fatal("expected 1 result in dry-run mode")
	}
	if _, err := os.Stat(tracesDir); err != nil {
		t.Error("traces dir should NOT be deleted in dry-run mode")
	}
}

// ---------------------------------------------------------------------------
// ResetEvents
// ---------------------------------------------------------------------------

func TestResetEvents_RemovesDir(t *testing.T) {
	dir := t.TempDir()
	eventsDir := filepath.Join(dir, ".ai", "state", "events")
	if err := os.MkdirAll(eventsDir, 0755); err != nil {
		t.Fatal(err)
	}

	r := New(dir)
	results := r.ResetEvents()

	if len(results) == 0 {
		t.Fatal("expected 1 result")
	}
	if !results[0].Success {
		t.Errorf("ResetEvents failed: %s", results[0].Message)
	}
	if _, err := os.Stat(eventsDir); !os.IsNotExist(err) {
		t.Error("events dir should have been removed")
	}
}

func TestResetEvents_NoDir_ReturnsNil(t *testing.T) {
	dir := t.TempDir()
	r := New(dir)
	results := r.ResetEvents()
	if len(results) != 0 {
		t.Errorf("expected nil when events dir doesn't exist, got %d results", len(results))
	}
}

// ---------------------------------------------------------------------------
// Results
// ---------------------------------------------------------------------------

func TestResults_DeletesResultFiles(t *testing.T) {
	dir := t.TempDir()
	resultsDir := filepath.Join(dir, ".ai", "results")
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resultsDir, "issue-1.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resultsDir, "issue-2.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	r := New(dir)
	results := r.Results()

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, res := range results {
		if !res.Success {
			t.Errorf("Results delete failed: %s", res.Message)
		}
	}
	if _, err := os.Stat(filepath.Join(resultsDir, "issue-1.json")); !os.IsNotExist(err) {
		t.Error("issue-1.json should have been deleted")
	}
}

func TestResults_NoDir_ReturnsNil(t *testing.T) {
	dir := t.TempDir()
	r := New(dir)
	results := r.Results()
	if len(results) != 0 {
		t.Errorf("expected nil when results dir doesn't exist, got %d", len(results))
	}
}

func TestResults_DryRun(t *testing.T) {
	dir := t.TempDir()
	resultsDir := filepath.Join(dir, ".ai", "results")
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		t.Fatal(err)
	}
	resultFile := filepath.Join(resultsDir, "issue-5.json")
	if err := os.WriteFile(resultFile, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	r := New(dir)
	r.SetDryRun(true)
	results := r.Results()

	if len(results) == 0 {
		t.Fatal("expected 1 result in dry-run mode")
	}
	if _, err := os.Stat(resultFile); err != nil {
		t.Error("result file should NOT be deleted in dry-run mode")
	}
}

// ---------------------------------------------------------------------------
// CleanAuditMilestones
// ---------------------------------------------------------------------------

func TestCleanAuditMilestones_RemovesFiles(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".ai", "state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}

	milestoneFile := filepath.Join(stateDir, "audit-milestone-myspec.txt")
	if err := os.WriteFile(milestoneFile, []byte("25"), 0644); err != nil {
		t.Fatal(err)
	}

	r := New(dir)
	results := r.CleanAuditMilestones()

	if len(results) == 0 {
		t.Fatal("expected 1 result")
	}
	if !results[0].Success {
		t.Errorf("CleanAuditMilestones failed: %s", results[0].Message)
	}
	if _, err := os.Stat(milestoneFile); !os.IsNotExist(err) {
		t.Error("audit milestone file should have been deleted")
	}
}

func TestCleanAuditMilestones_NoFiles_ReturnsNil(t *testing.T) {
	dir := t.TempDir()
	r := New(dir)
	results := r.CleanAuditMilestones()
	if len(results) != 0 {
		t.Errorf("expected nil when no audit milestone files, got %d results", len(results))
	}
}

func TestCleanAuditMilestones_DryRun(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".ai", "state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	milestoneFile := filepath.Join(stateDir, "audit-milestone-spec1.txt")
	if err := os.WriteFile(milestoneFile, []byte("50"), 0644); err != nil {
		t.Fatal(err)
	}

	r := New(dir)
	r.SetDryRun(true)
	results := r.CleanAuditMilestones()

	if len(results) == 0 {
		t.Fatal("expected 1 result in dry-run mode")
	}
	if _, err := os.Stat(milestoneFile); err != nil {
		t.Error("audit milestone file should NOT be deleted in dry-run mode")
	}
}

// ---------------------------------------------------------------------------
// ResetAll (reset.go)
// ---------------------------------------------------------------------------

func TestResetAll_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	r := New(dir)
	// Should not panic and should return no results (nothing to delete)
	results := r.ResetAll(nil)
	_ = results // just ensure no panic
}

func TestResetAll_DryRun_WithFiles(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".ai", "state")
	os.MkdirAll(stateDir, 0755)
	// Create a state file
	os.WriteFile(filepath.Join(stateDir, "loop_count"), []byte("5"), 0644)

	r := New(dir)
	r.SetDryRun(true)
	results := r.ResetAll(nil)
	// Dry-run: should report what would be done, file should still exist
	_ = results
	if _, err := os.Stat(filepath.Join(stateDir, "loop_count")); err != nil {
		t.Error("loop_count should still exist in dry-run mode")
	}
}

