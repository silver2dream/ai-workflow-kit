package doctor

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Doctor.New (doctor.go)
// ---------------------------------------------------------------------------

func TestNew_EmptyStateRoot_UsesDefault(t *testing.T) {
	d := New("")
	if d == nil {
		t.Fatal("New should return non-nil")
	}
	if d.StateRoot != "." {
		t.Errorf("StateRoot = %q, want '.'", d.StateRoot)
	}
}

func TestNew_NonEmptyStateRoot(t *testing.T) {
	d := New("/some/path")
	if d.StateRoot != "/some/path" {
		t.Errorf("StateRoot = %q, want '/some/path'", d.StateRoot)
	}
}

// ---------------------------------------------------------------------------
// Doctor.CheckAttempts (doctor.go)
// ---------------------------------------------------------------------------

func TestCheckAttempts_NoAttemptsDir(t *testing.T) {
	dir := t.TempDir()
	d := New(dir)
	results := d.CheckAttempts()
	if results != nil {
		t.Errorf("CheckAttempts with no dir should return nil, got %v", results)
	}
}

func TestCheckAttempts_EmptyAttemptsDir(t *testing.T) {
	dir := t.TempDir()
	attemptsDir := filepath.Join(dir, ".ai", "state", "attempts")
	os.MkdirAll(attemptsDir, 0755)

	d := New(dir)
	results := d.CheckAttempts()
	if len(results) != 0 {
		t.Errorf("CheckAttempts with empty dir = %v, want empty", results)
	}
}

func TestCheckAttempts_WithAttemptFile(t *testing.T) {
	dir := t.TempDir()
	attemptsDir := filepath.Join(dir, ".ai", "state", "attempts")
	os.MkdirAll(attemptsDir, 0755)
	os.WriteFile(filepath.Join(attemptsDir, "issue-42.txt"), []byte("2"), 0644)

	d := New(dir)
	results := d.CheckAttempts()
	if len(results) != 1 {
		t.Errorf("CheckAttempts with attempt file = %d results, want 1", len(results))
	}
	if results[0].Status != "warning" {
		t.Errorf("Status = %q, want 'warning'", results[0].Status)
	}
}

func TestCheckAttempts_ZeroCount(t *testing.T) {
	dir := t.TempDir()
	attemptsDir := filepath.Join(dir, ".ai", "state", "attempts")
	os.MkdirAll(attemptsDir, 0755)
	os.WriteFile(filepath.Join(attemptsDir, "issue-1.txt"), []byte("0"), 0644)

	d := New(dir)
	results := d.CheckAttempts()
	// Zero count should not trigger warning
	if len(results) != 0 {
		t.Errorf("CheckAttempts with zero count = %d results, want 0", len(results))
	}
}

// ---------------------------------------------------------------------------
// Doctor.CheckTempTickets (doctor.go)
// ---------------------------------------------------------------------------

func TestCheckTempTickets_NoTempDir(t *testing.T) {
	dir := t.TempDir()
	d := New(dir)
	results := d.CheckTempTickets()
	if results != nil {
		t.Errorf("CheckTempTickets with no dir should return nil, got %v", results)
	}
}

func TestCheckTempTickets_FewTickets(t *testing.T) {
	dir := t.TempDir()
	tempDir := filepath.Join(dir, ".ai", "temp")
	os.MkdirAll(tempDir, 0755)
	// Create fewer than threshold (10)
	for i := 0; i < 5; i++ {
		os.WriteFile(filepath.Join(tempDir, "ticket-00"+string(rune('0'+i))+".md"), []byte("content"), 0644)
	}

	d := New(dir)
	results := d.CheckTempTickets()
	// Under threshold — no warning
	if len(results) != 0 {
		t.Errorf("CheckTempTickets with few tickets = %v, want empty", results)
	}
}

func TestCheckTempTickets_ManyTickets(t *testing.T) {
	dir := t.TempDir()
	tempDir := filepath.Join(dir, ".ai", "temp")
	os.MkdirAll(tempDir, 0755)
	// Create more than threshold (10)
	for i := 0; i < 15; i++ {
		name := filepath.Join(tempDir, "ticket-"+string(rune('0'+(i/10)))+string(rune('0'+(i%10)))+".md")
		os.WriteFile(name, []byte("content"), 0644)
	}

	d := New(dir)
	results := d.CheckTempTickets()
	if len(results) != 1 {
		t.Errorf("CheckTempTickets with many tickets = %d results, want 1", len(results))
	}
	if results[0].Status != "warning" {
		t.Errorf("Status = %q, want 'warning'", results[0].Status)
	}
}

// ---------------------------------------------------------------------------
// Doctor.CheckSessionFiles (doctor.go)
// ---------------------------------------------------------------------------

func TestCheckSessionFiles_NoSessionDir(t *testing.T) {
	dir := t.TempDir()
	d := New(dir)
	results := d.CheckSessionFiles()
	if results != nil {
		t.Errorf("CheckSessionFiles with no dir should return nil, got %v", results)
	}
}

func TestCheckSessionFiles_FewSessions(t *testing.T) {
	dir := t.TempDir()
	sessDir := filepath.Join(dir, ".ai", "state", "principal", "sessions")
	os.MkdirAll(sessDir, 0755)
	for i := 0; i < 5; i++ {
		os.WriteFile(filepath.Join(sessDir, "session-00"+string(rune('0'+i))+".json"), []byte("{}"), 0644)
	}

	d := New(dir)
	results := d.CheckSessionFiles()
	if len(results) != 0 {
		t.Errorf("CheckSessionFiles with few sessions = %v, want empty", results)
	}
}

// ---------------------------------------------------------------------------
// Doctor.CheckClaudeSettings (doctor.go)
// ---------------------------------------------------------------------------

func TestCheckClaudeSettings_NoSettingsFile(t *testing.T) {
	dir := t.TempDir()
	d := New(dir)
	results := d.CheckClaudeSettings()
	if len(results) != 1 {
		t.Errorf("CheckClaudeSettings with no file = %d results, want 1", len(results))
	}
	if results[0].Status != "warning" {
		t.Errorf("Status = %q, want 'warning'", results[0].Status)
	}
}

// ---------------------------------------------------------------------------
// Doctor.CheckConfigVersion (doctor.go)
// ---------------------------------------------------------------------------

func TestCheckConfigVersion_NoConfigFile(t *testing.T) {
	dir := t.TempDir()
	d := New(dir)
	results := d.CheckConfigVersion()
	// No config file → no error
	if len(results) != 0 {
		t.Errorf("CheckConfigVersion with no config = %v, want empty", results)
	}
}

// ---------------------------------------------------------------------------
// Doctor.CheckConsecutiveFailures (doctor.go) — additional cases
// ---------------------------------------------------------------------------

func TestCheckConsecutiveFailures_NoFile(t *testing.T) {
	dir := t.TempDir()
	d := New(dir)
	result := d.CheckConsecutiveFailures()
	if result != nil {
		t.Errorf("CheckConsecutiveFailures with no file = %v, want nil", result)
	}
}

func TestCheckConsecutiveFailures_ZeroCount(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".ai", "state")
	os.MkdirAll(stateDir, 0755)
	os.WriteFile(filepath.Join(stateDir, "consecutive_failures"), []byte("0"), 0644)

	d := New(dir)
	result := d.CheckConsecutiveFailures()
	if result != nil {
		t.Errorf("CheckConsecutiveFailures with zero = %v, want nil", result)
	}
}

// ---------------------------------------------------------------------------
// Doctor.CheckOrphanTmpFiles (doctor.go) — no orphan case
// ---------------------------------------------------------------------------

func TestCheckOrphanTmpFiles_NoOrphans(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".ai", "state")
	resultsDir := filepath.Join(dir, ".ai", "results")
	os.MkdirAll(stateDir, 0755)
	os.MkdirAll(resultsDir, 0755)

	// Create a fresh .tmp file (not old enough to be orphaned)
	os.WriteFile(filepath.Join(stateDir, "fresh.tmp"), []byte("fresh"), 0644)

	d := New(dir)
	results := d.CheckOrphanTmpFiles()
	if len(results) != 0 {
		t.Errorf("CheckOrphanTmpFiles with fresh file = %v, want empty", results)
	}
}

func TestCheckOrphanTmpFiles_WithOldFile(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".ai", "state")
	os.MkdirAll(stateDir, 0755)
	oldFile := filepath.Join(stateDir, "old.tmp")
	os.WriteFile(oldFile, []byte("old"), 0644)

	// Set modification time to 2 hours ago
	oldTime := time.Now().Add(-2 * time.Hour)
	os.Chtimes(oldFile, oldTime, oldTime)

	d := New(dir)
	results := d.CheckOrphanTmpFiles()
	if len(results) != 1 {
		t.Errorf("CheckOrphanTmpFiles with old file = %d results, want 1", len(results))
	}
	if results[0].Status != "warning" {
		t.Errorf("Status = %q, want 'warning'", results[0].Status)
	}
}
