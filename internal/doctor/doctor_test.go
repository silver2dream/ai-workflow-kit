package doctor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// makeDir creates a directory and returns cleanup function.
func makeDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "doctor_test_*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// writeFile writes content to path, creating all parent directories.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}

func TestCheckLoopCount_NoFile_ReturnsNil(t *testing.T) {
	dir := makeDir(t)
	d := New(dir)
	results := d.CheckLoopCount()
	if len(results) != 0 {
		t.Errorf("expected nil results when loop_count file absent, got %v", results)
	}
}

func TestCheckLoopCount_ZeroCount_ReturnsNil(t *testing.T) {
	dir := makeDir(t)
	writeFile(t, filepath.Join(dir, ".ai", "state", "loop_count"), "0\n")
	d := New(dir)
	results := d.CheckLoopCount()
	if len(results) != 0 {
		t.Errorf("expected nil results for loop_count=0, got %v", results)
	}
}

func TestCheckLoopCount_NonZeroCount_ReturnsWarning(t *testing.T) {
	dir := makeDir(t)
	writeFile(t, filepath.Join(dir, ".ai", "state", "loop_count"), "42\n")
	d := New(dir)
	results := d.CheckLoopCount()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "warning" {
		t.Errorf("status = %q, want %q", results[0].Status, "warning")
	}
	if !results[0].CanClean {
		t.Error("expected CanClean=true")
	}
}

func TestCheckStopMarker_NoMarker_ReturnsOK(t *testing.T) {
	dir := makeDir(t)
	d := New(dir)
	result := d.CheckStopMarker()
	if result.Status != "ok" {
		t.Errorf("status = %q, want %q", result.Status, "ok")
	}
}

func TestCheckStopMarker_MarkerExists_ReturnsWarning(t *testing.T) {
	dir := makeDir(t)
	writeFile(t, filepath.Join(dir, ".ai", "state", "STOP"), "")
	d := New(dir)
	result := d.CheckStopMarker()
	if result.Status != "warning" {
		t.Errorf("status = %q, want %q", result.Status, "warning")
	}
	if !result.CanClean {
		t.Error("expected CanClean=true")
	}
}

func TestCheckLockFile_NoLock_ReturnsOK(t *testing.T) {
	dir := makeDir(t)
	d := New(dir)
	result := d.CheckLockFile()
	if result.Status != "ok" {
		t.Errorf("status = %q, want %q", result.Status, "ok")
	}
}

func TestCheckLockFile_StaleLock_ReturnsWarning(t *testing.T) {
	dir := makeDir(t)
	lockPath := filepath.Join(dir, ".ai", "state", "kickoff.lock")
	writeFile(t, lockPath, "pid=12345\n")

	// Set mtime to 2 hours ago to make it stale.
	oldTime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(lockPath, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	d := New(dir)
	result := d.CheckLockFile()
	if result.Status != "warning" {
		t.Errorf("status = %q, want %q", result.Status, "warning")
	}
}

func TestCheckLockFile_FreshLock_ReturnsError(t *testing.T) {
	dir := makeDir(t)
	writeFile(t, filepath.Join(dir, ".ai", "state", "kickoff.lock"), "pid=12345\n")
	d := New(dir)
	result := d.CheckLockFile()
	if result.Status != "error" {
		t.Errorf("status = %q, want %q", result.Status, "error")
	}
}

func TestCheckConsecutiveFailures_NonZero_ReturnsWarning(t *testing.T) {
	dir := makeDir(t)
	writeFile(t, filepath.Join(dir, ".ai", "state", "consecutive_failures"), "3\n")
	d := New(dir)
	results := d.CheckConsecutiveFailures()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "warning" {
		t.Errorf("status = %q, want %q", results[0].Status, "warning")
	}
}

func TestCheckOrphanTmpFiles_OldFile_ReturnsWarning(t *testing.T) {
	dir := makeDir(t)
	stateDir := filepath.Join(dir, ".ai", "state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	tmpPath := filepath.Join(stateDir, "leftover.tmp")
	if err := os.WriteFile(tmpPath, []byte("stale"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// Make file look old (2 hours ago).
	oldTime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(tmpPath, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	d := New(dir)
	results := d.CheckOrphanTmpFiles()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "warning" {
		t.Errorf("status = %q, want %q", results[0].Status, "warning")
	}
}

func TestRunAll_EmptyDir_NoErrors(t *testing.T) {
	dir := makeDir(t)
	d := New(dir)
	// RunAll includes GitHub label checks which call exec. Run with a short
	// timeout and cancelled context so the exec call returns quickly.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	results := d.RunAll(ctx)
	// We expect no error-status results for an empty dir.
	for _, r := range results {
		if r.Status == "error" {
			t.Errorf("unexpected error result: %+v", r)
		}
	}
}
