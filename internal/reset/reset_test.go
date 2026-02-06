package reset

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanTemp(t *testing.T) {
	dir := t.TempDir()

	// Create .ai/temp/ directory
	tempDir := filepath.Join(dir, ".ai", "temp")
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create ticket files and a non-ticket file
	ticketFiles := []string{"ticket-1.md", "ticket-2.md", "ticket-100.md"}
	for _, name := range ticketFiles {
		if err := os.WriteFile(filepath.Join(tempDir, name), []byte("test"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	otherFile := filepath.Join(tempDir, "notes.txt")
	if err := os.WriteFile(otherFile, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := New(dir)
	results := r.CleanTemp()

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Success {
		t.Fatalf("expected success, got failure: %s", results[0].Message)
	}

	// Verify ticket files are gone
	for _, name := range ticketFiles {
		if _, err := os.Stat(filepath.Join(tempDir, name)); !os.IsNotExist(err) {
			t.Errorf("ticket file %s should have been deleted", name)
		}
	}

	// Verify non-ticket file is still there
	if _, err := os.Stat(otherFile); err != nil {
		t.Errorf("notes.txt should not have been deleted: %v", err)
	}
}

func TestCleanTemp_NoFiles(t *testing.T) {
	dir := t.TempDir()

	r := New(dir)
	results := r.CleanTemp()

	if len(results) != 0 {
		t.Fatalf("expected no results when no temp files exist, got %d", len(results))
	}
}

func TestCleanSessions_KeepsLast5(t *testing.T) {
	dir := t.TempDir()

	sessionsDir := filepath.Join(dir, ".ai", "state", "principal", "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create 8 session files with different mod times
	baseTime := time.Now().Add(-10 * time.Hour)
	for i := 0; i < 8; i++ {
		name := filepath.Join(sessionsDir, "session-"+string(rune('a'+i))+".json")
		if err := os.WriteFile(name, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		modTime := baseTime.Add(time.Duration(i) * time.Hour)
		if err := os.Chtimes(name, modTime, modTime); err != nil {
			t.Fatal(err)
		}
	}

	r := New(dir)
	results := r.CleanSessions()

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Success {
		t.Fatalf("expected success: %s", results[0].Message)
	}

	// Count remaining files
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		t.Fatal(err)
	}
	remaining := 0
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".json" {
			remaining++
		}
	}
	if remaining != 5 {
		t.Errorf("expected 5 remaining sessions, got %d", remaining)
	}

	// Verify the oldest 3 were removed (a, b, c) and newest 5 remain (d, e, f, g, h)
	for _, name := range []string{"session-d.json", "session-e.json", "session-f.json", "session-g.json", "session-h.json"} {
		if _, err := os.Stat(filepath.Join(sessionsDir, name)); err != nil {
			t.Errorf("expected %s to still exist", name)
		}
	}
	for _, name := range []string{"session-a.json", "session-b.json", "session-c.json"} {
		if _, err := os.Stat(filepath.Join(sessionsDir, name)); !os.IsNotExist(err) {
			t.Errorf("expected %s to be deleted", name)
		}
	}
}

func TestCleanSessions_LessThan5(t *testing.T) {
	dir := t.TempDir()

	sessionsDir := filepath.Join(dir, ".ai", "state", "principal", "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create only 3 session files
	for i := 0; i < 3; i++ {
		name := filepath.Join(sessionsDir, "session-"+string(rune('a'+i))+".json")
		if err := os.WriteFile(name, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	r := New(dir)
	results := r.CleanSessions()

	if len(results) != 0 {
		t.Fatalf("expected no results when <= 5 sessions, got %d", len(results))
	}
}

func TestCleanOrphans_RemovesOldTmpFiles(t *testing.T) {
	dir := t.TempDir()

	stateDir := filepath.Join(dir, ".ai", "state")
	resultsDir := filepath.Join(dir, ".ai", "results")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create an old .tmp file (2 hours ago)
	oldTmp := filepath.Join(stateDir, "write-abc123.tmp")
	if err := os.WriteFile(oldTmp, []byte("partial"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(oldTmp, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	// Create a recent .tmp file (5 minutes ago) - should NOT be removed
	recentTmp := filepath.Join(resultsDir, "write-def456.tmp")
	if err := os.WriteFile(recentTmp, []byte("partial"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a non-.tmp file - should NOT be removed
	normalFile := filepath.Join(stateDir, "loop_count")
	if err := os.WriteFile(normalFile, []byte("5"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := New(dir)
	results := r.CleanOrphans()

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Success {
		t.Fatalf("expected success: %s", results[0].Message)
	}

	// Old .tmp should be gone
	if _, err := os.Stat(oldTmp); !os.IsNotExist(err) {
		t.Error("old .tmp file should have been deleted")
	}

	// Recent .tmp should still exist
	if _, err := os.Stat(recentTmp); err != nil {
		t.Error("recent .tmp file should NOT have been deleted")
	}

	// Normal file should still exist
	if _, err := os.Stat(normalFile); err != nil {
		t.Error("loop_count should NOT have been deleted")
	}
}

func TestCleanOrphans_NoOrphans(t *testing.T) {
	dir := t.TempDir()

	stateDir := filepath.Join(dir, ".ai", "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}

	r := New(dir)
	results := r.CleanOrphans()

	if len(results) != 0 {
		t.Fatalf("expected no results when no orphans, got %d", len(results))
	}
}

func TestCleanReports(t *testing.T) {
	dir := t.TempDir()

	stateDir := filepath.Join(dir, ".ai", "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create workflow report files
	reportFiles := []string{
		"workflow-report-2025-01-01.md",
		"workflow-report-2025-01-02.md",
	}
	for _, name := range reportFiles {
		if err := os.WriteFile(filepath.Join(stateDir, name), []byte("# Report"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Create a non-report file
	otherFile := filepath.Join(stateDir, "loop_count")
	if err := os.WriteFile(otherFile, []byte("0"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := New(dir)
	results := r.CleanReports()

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Success {
		t.Fatalf("expected success: %s", results[0].Message)
	}

	// Reports should be gone
	for _, name := range reportFiles {
		if _, err := os.Stat(filepath.Join(stateDir, name)); !os.IsNotExist(err) {
			t.Errorf("report %s should have been deleted", name)
		}
	}

	// Other file should remain
	if _, err := os.Stat(otherFile); err != nil {
		t.Error("loop_count should NOT have been deleted")
	}
}

func TestDryRun_DoesNotDelete(t *testing.T) {
	dir := t.TempDir()

	// Set up temp tickets
	tempDir := filepath.Join(dir, ".ai", "temp")
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ticketFile := filepath.Join(tempDir, "ticket-1.md")
	if err := os.WriteFile(ticketFile, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Set up old orphan
	stateDir := filepath.Join(dir, ".ai", "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	orphanFile := filepath.Join(stateDir, "write.tmp")
	if err := os.WriteFile(orphanFile, []byte("partial"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(orphanFile, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	// Set up sessions (7 files)
	sessionsDir := filepath.Join(dir, ".ai", "state", "principal", "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	baseTime := time.Now().Add(-10 * time.Hour)
	for i := 0; i < 7; i++ {
		name := filepath.Join(sessionsDir, "session-"+string(rune('a'+i))+".json")
		if err := os.WriteFile(name, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		modTime := baseTime.Add(time.Duration(i) * time.Hour)
		if err := os.Chtimes(name, modTime, modTime); err != nil {
			t.Fatal(err)
		}
	}

	// Set up workflow reports
	reportFile := filepath.Join(stateDir, "workflow-report-test.md")
	if err := os.WriteFile(reportFile, []byte("# Report"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := New(dir)
	r.SetDryRun(true)

	// Run all cleanup methods
	r.CleanTemp()
	r.CleanOrphans()
	r.CleanSessions()
	r.CleanReports()

	// Verify nothing was deleted
	if _, err := os.Stat(ticketFile); err != nil {
		t.Error("ticket file should NOT have been deleted in dry-run mode")
	}
	if _, err := os.Stat(orphanFile); err != nil {
		t.Error("orphan file should NOT have been deleted in dry-run mode")
	}
	if _, err := os.Stat(reportFile); err != nil {
		t.Error("report file should NOT have been deleted in dry-run mode")
	}

	// All 7 session files should still exist
	entries, _ := os.ReadDir(sessionsDir)
	jsonCount := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".json" {
			jsonCount++
		}
	}
	if jsonCount != 7 {
		t.Errorf("expected 7 session files in dry-run mode, got %d", jsonCount)
	}
}
