package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// findProtectedChanges (security.go) — additional cases
// ---------------------------------------------------------------------------

func TestFindProtectedChanges_NoViolations(t *testing.T) {
	files := []string{"main.go", "README.md", "internal/pkg/foo.go"}
	violations := findProtectedChanges(files, "")
	if len(violations) != 0 {
		t.Errorf("findProtectedChanges(normal files) = %v, want empty", violations)
	}
}

func TestFindProtectedChanges_CommandsPath(t *testing.T) {
	files := []string{".ai/commands/run.sh"}
	violations := findProtectedChanges(files, "")
	if len(violations) != 1 {
		t.Errorf("findProtectedChanges(.ai/commands/) = %d violations, want 1", len(violations))
	}
}

func TestFindProtectedChanges_WithViolation(t *testing.T) {
	files := []string{".ai/scripts/setup.sh", "main.go"}
	violations := findProtectedChanges(files, "")
	if len(violations) != 1 {
		t.Errorf("findProtectedChanges(protected file) = %d violations, want 1", len(violations))
	}
	if violations[0] != ".ai/scripts/setup.sh" {
		t.Errorf("violation = %q, want .ai/scripts/setup.sh", violations[0])
	}
}

// ---------------------------------------------------------------------------
// normalizePath (security.go) — additional cases
// ---------------------------------------------------------------------------

func TestNormalizePath_BackslashToSlash(t *testing.T) {
	result := normalizePath(".ai\\scripts\\setup.sh")
	if strings.Contains(result, "\\") {
		t.Errorf("normalizePath should convert backslash to slash, got: %q", result)
	}
}

// ---------------------------------------------------------------------------
// splitLines (security.go) — additional cases
// ---------------------------------------------------------------------------

func TestSplitLines_SkipsEmpty(t *testing.T) {
	lines := splitLines("line1\n\nline2\n   \nline3\n")
	if len(lines) != 3 {
		t.Errorf("splitLines (with empty) = %d lines, want 3", len(lines))
	}
}

// ---------------------------------------------------------------------------
// BackendRegistry (registry.go)
// ---------------------------------------------------------------------------

func TestNewBackendRegistry_Empty(t *testing.T) {
	reg := NewBackendRegistry()
	if reg == nil {
		t.Fatal("NewBackendRegistry should return non-nil")
	}
	names := reg.Names()
	if len(names) != 0 {
		t.Errorf("Names() = %v, want empty", names)
	}
}

func TestBackendRegistry_RegisterAndGet(t *testing.T) {
	reg := NewBackendRegistry()
	b := NewCodexBackend()
	reg.Register(b)

	got, err := reg.Get("codex")
	if err != nil {
		t.Fatalf("Get('codex'): %v", err)
	}
	if got.Name() != "codex" {
		t.Errorf("got.Name() = %q, want 'codex'", got.Name())
	}
}

func TestBackendRegistry_GetUnknown_Error(t *testing.T) {
	reg := NewBackendRegistry()
	_, err := reg.Get("nonexistent")
	if err == nil {
		t.Error("Get('nonexistent') should return error")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention backend name, got: %q", err.Error())
	}
}

func TestBackendRegistry_Names_Sorted(t *testing.T) {
	reg := NewBackendRegistry()
	reg.Register(NewCodexBackend())
	reg.Register(NewClaudeCodeBackend("claude-sonnet-4-5", 10, false))

	names := reg.Names()
	if len(names) < 2 {
		t.Fatalf("Names() = %v, want at least 2", names)
	}
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Errorf("Names() not sorted: %v", names)
		}
	}
}

func TestDefaultRegistry_HasBothBackends(t *testing.T) {
	reg := DefaultRegistry("claude-sonnet-4-5", 10, false)
	names := reg.Names()
	if len(names) < 2 {
		t.Errorf("DefaultRegistry names = %v, want at least 2", names)
	}
}

// ---------------------------------------------------------------------------
// AttemptGuard (attempt.go)
// ---------------------------------------------------------------------------

func TestAttemptGuard_Check_Increments(t *testing.T) {
	dir := t.TempDir()
	g := &AttemptGuard{StateRoot: dir, IssueID: 1, MaxAttempts: 3}
	result, err := g.Check()
	if err != nil {
		t.Fatalf("Check(): %v", err)
	}
	if !result.CanProceed {
		t.Error("Check() should allow proceeding on first attempt")
	}
	if result.AttemptNumber != 1 {
		t.Errorf("AttemptNumber = %d, want 1", result.AttemptNumber)
	}
	if result.Reason != "ok" {
		t.Errorf("Reason = %q, want 'ok'", result.Reason)
	}
}

func TestAttemptGuard_Check_MaxAttempts(t *testing.T) {
	dir := t.TempDir()
	g := &AttemptGuard{StateRoot: dir, IssueID: 2, MaxAttempts: 2}

	// Use up all attempts
	for i := 0; i < 2; i++ {
		_, err := g.Check()
		if err != nil {
			t.Fatalf("Check() attempt %d: %v", i+1, err)
		}
	}

	// This attempt should be denied
	result, err := g.Check()
	if err != nil {
		t.Fatalf("Check() after max: %v", err)
	}
	if result.CanProceed {
		t.Error("Check() should not allow proceeding after max attempts")
	}
	if result.Reason != "max_attempts" {
		t.Errorf("Reason = %q, want 'max_attempts'", result.Reason)
	}
}

func TestAttemptGuard_RecordFailure(t *testing.T) {
	dir := t.TempDir()
	g := &AttemptGuard{StateRoot: dir, IssueID: 3, MaxAttempts: 3}

	// Create dirs via Check
	_, err := g.Check()
	if err != nil {
		t.Fatalf("Check(): %v", err)
	}

	if err := g.RecordFailure("timeout", true); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}

	historyFile := filepath.Join(dir, ".ai", "state", "failure_history.jsonl")
	data, err := os.ReadFile(historyFile)
	if err != nil {
		t.Fatalf("ReadFile(failure_history): %v", err)
	}
	if !strings.Contains(string(data), "timeout") {
		t.Errorf("failure_history should contain 'timeout', got: %s", string(data))
	}
}

func TestAttemptGuard_Reset(t *testing.T) {
	dir := t.TempDir()
	g := &AttemptGuard{StateRoot: dir, IssueID: 4, MaxAttempts: 3}

	// Create dirs and increment
	_, err := g.Check()
	if err != nil {
		t.Fatalf("Check(): %v", err)
	}

	if err := g.Reset(); err != nil {
		t.Fatalf("Reset(): %v", err)
	}

	// After reset, count should be 0 → next check returns attempt 1
	result, err := g.Check()
	if err != nil {
		t.Fatalf("Check() after reset: %v", err)
	}
	if result.AttemptNumber != 1 {
		t.Errorf("AttemptNumber after reset = %d, want 1", result.AttemptNumber)
	}
}

// ---------------------------------------------------------------------------
// result.go — WriteFileAtomic additional test
// ---------------------------------------------------------------------------

func TestWriteFileAtomic_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "nested", "test.txt")

	if err := WriteFileAtomic(path, []byte("nested"), 0644); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file should exist after atomic write: %v", err)
	}
}

// ---------------------------------------------------------------------------
// NewTraceRecorder (trace.go)
// ---------------------------------------------------------------------------

func TestNewTraceRecorder_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	rec, err := NewTraceRecorder(dir, 10, "owner/repo", "feat/test", "main", 12345, now)
	if err != nil {
		t.Fatalf("NewTraceRecorder: %v", err)
	}
	if rec == nil {
		t.Fatal("NewTraceRecorder returned nil")
	}

	tracePath := filepath.Join(dir, ".ai", "state", "traces", "issue-10.json")
	if _, err := os.Stat(tracePath); err != nil {
		t.Errorf("trace file not created: %v", err)
	}
}

func TestTraceRecorder_StepStartEnd(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	rec, err := NewTraceRecorder(dir, 20, "owner/repo", "feat/test", "main", 0, now)
	if err != nil {
		t.Fatalf("NewTraceRecorder: %v", err)
	}

	rec.StepStart("setup")
	if err := rec.StepEnd("success", "", nil); err != nil {
		t.Fatalf("StepEnd: %v", err)
	}

	tracePath := filepath.Join(dir, ".ai", "state", "traces", "issue-20.json")
	data, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "setup") {
		t.Errorf("trace should contain 'setup' step, got: %s", string(data))
	}
}

func TestTraceRecorder_Finalize_Success(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	rec, err := NewTraceRecorder(dir, 30, "owner/repo", "feat/test", "main", 0, now)
	if err != nil {
		t.Fatalf("NewTraceRecorder: %v", err)
	}

	if err := rec.Finalize(nil); err != nil {
		t.Fatalf("Finalize(nil): %v", err)
	}

	tracePath := filepath.Join(dir, ".ai", "state", "traces", "issue-30.json")
	data, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), `"success"`) {
		t.Errorf("trace should contain 'success' status, got: %s", string(data))
	}
}

func TestTraceRecorder_Finalize_WithError(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	rec, err := NewTraceRecorder(dir, 40, "owner/repo", "feat/test", "main", 0, now)
	if err != nil {
		t.Fatalf("NewTraceRecorder: %v", err)
	}

	if err := rec.Finalize(os.ErrNotExist); err != nil {
		t.Fatalf("Finalize(err): %v", err)
	}

	tracePath := filepath.Join(dir, ".ai", "state", "traces", "issue-40.json")
	data, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "failed") {
		t.Errorf("trace should contain 'failed' status, got: %s", string(data))
	}
}

func TestTraceRecorder_StepEnd_NoCurrentStep(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	rec, err := NewTraceRecorder(dir, 50, "owner/repo", "feat/test", "main", 0, now)
	if err != nil {
		t.Fatalf("NewTraceRecorder: %v", err)
	}

	// StepEnd without StepStart should be a no-op
	if err := rec.StepEnd("success", "", nil); err != nil {
		t.Fatalf("StepEnd (no current step): %v", err)
	}
}

func TestTraceRecorder_StepEnd_WithError(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	rec, err := NewTraceRecorder(dir, 60, "owner/repo", "feat/test", "main", 0, now)
	if err != nil {
		t.Fatalf("NewTraceRecorder: %v", err)
	}

	rec.StepStart("build")
	if err := rec.StepEnd("failed", "compilation error", nil); err != nil {
		t.Fatalf("StepEnd with error: %v", err)
	}

	tracePath := filepath.Join(dir, ".ai", "state", "traces", "issue-60.json")
	data, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "compilation error") {
		t.Errorf("trace should contain error message, got: %s", string(data))
	}
}
