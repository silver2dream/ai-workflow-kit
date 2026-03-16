package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// tailLines (backend_codex.go)
// ---------------------------------------------------------------------------

func TestTailLines_BasicRead(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.log")
	os.WriteFile(f, []byte("line1\nline2\nline3\n"), 0644)

	lines := tailLines(f, 10)
	if len(lines) != 3 {
		t.Errorf("tailLines() = %d lines, want 3", len(lines))
	}
	if lines[0] != "line1" {
		t.Errorf("lines[0] = %q, want line1", lines[0])
	}
}

func TestTailLines_MaxLinesRespected(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.log")
	// Write 10 lines
	content := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\n"
	os.WriteFile(f, []byte(content), 0644)

	lines := tailLines(f, 3)
	if len(lines) != 3 {
		t.Errorf("tailLines(maxLines=3) = %d lines, want 3", len(lines))
	}
	// Should be the last 3 lines
	if lines[2] != "line10" {
		t.Errorf("last line = %q, want line10", lines[2])
	}
}

func TestTailLines_NonExistentFile(t *testing.T) {
	lines := tailLines("/nonexistent/path.log", 10)
	if lines != nil {
		t.Errorf("tailLines for non-existent file should return nil, got %v", lines)
	}
}

func TestTailLines_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "empty.log")
	os.WriteFile(f, []byte(""), 0644)

	lines := tailLines(f, 10)
	if len(lines) != 0 {
		t.Errorf("tailLines on empty file should return 0 lines, got %d", len(lines))
	}
}

// ---------------------------------------------------------------------------
// writeSummary (backend_codex.go)
// ---------------------------------------------------------------------------

func TestWriteSummary_EmptyPath_NoOp(t *testing.T) {
	// Should not panic with empty path
	writeSummary("", "message")
}

func TestWriteSummary_WritesToFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "summary.txt")

	writeSummary(f, "hello\n")
	writeSummary(f, "world\n")

	data, err := os.ReadFile(f)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "hello") {
		t.Errorf("summary should contain 'hello', got: %s", string(data))
	}
	if !strings.Contains(string(data), "world") {
		t.Errorf("summary should contain 'world', got: %s", string(data))
	}
}

func TestWriteSummary_NonExistentDir_NoOp(t *testing.T) {
	// Should not panic even if directory doesn't exist
	writeSummary("/nonexistent/dir/summary.txt", "message")
}

// ---------------------------------------------------------------------------
// readFailureReason (backend_codex.go)
// ---------------------------------------------------------------------------

func TestReadFailureReason_NoFile(t *testing.T) {
	reason := readFailureReason("/nonexistent/log.txt", 1)
	if !strings.Contains(reason, "1") {
		t.Errorf("readFailureReason for non-existent file should return exit code message, got: %q", reason)
	}
}

func TestReadFailureReason_WithErrorLine(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "codex.log")
	os.WriteFile(f, []byte("some output\nERROR: compilation failed\nmore output\n"), 0644)

	reason := readFailureReason(f, 1)
	if !strings.Contains(reason, "ERROR") {
		t.Errorf("readFailureReason should return line with ERROR, got: %q", reason)
	}
}

func TestReadFailureReason_NoErrorLine(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "codex.log")
	os.WriteFile(f, []byte("normal output\neverything ok\n"), 0644)

	reason := readFailureReason(f, 42)
	if !strings.Contains(reason, "42") {
		t.Errorf("readFailureReason without error line should return exit code, got: %q", reason)
	}
}

// ---------------------------------------------------------------------------
// withOptionalTimeout (commit.go)
// ---------------------------------------------------------------------------

func TestWithOptionalTimeout_ZeroTimeout(t *testing.T) {
	ctx := context.Background()
	newCtx, cancel := withOptionalTimeout(ctx, 0)
	defer cancel()
	// Should return same context without deadline
	_, hasDeadline := newCtx.Deadline()
	if hasDeadline {
		t.Error("withOptionalTimeout(0) should not set a deadline")
	}
}

func TestWithOptionalTimeout_PositiveTimeout(t *testing.T) {
	ctx := context.Background()
	newCtx, cancel := withOptionalTimeout(ctx, 5*time.Second)
	defer cancel()
	// Should have a deadline
	_, hasDeadline := newCtx.Deadline()
	if !hasDeadline {
		t.Error("withOptionalTimeout(5s) should set a deadline")
	}
}

func TestWithOptionalTimeout_NegativeTimeout(t *testing.T) {
	ctx := context.Background()
	newCtx, cancel := withOptionalTimeout(ctx, -1*time.Second)
	defer cancel()
	// Negative timeout treated as 0 (no deadline)
	_, hasDeadline := newCtx.Deadline()
	if hasDeadline {
		t.Error("withOptionalTimeout(-1s) should not set a deadline")
	}
}

// ---------------------------------------------------------------------------
// NewCodexBackend / NewClaudeCodeBackend - constructor tests
// ---------------------------------------------------------------------------

func TestNewCodexBackend_ReturnsNonNil(t *testing.T) {
	b := NewCodexBackend()
	if b == nil {
		t.Error("NewCodexBackend should return non-nil")
	}
	if b.Name() != "codex" {
		t.Errorf("Name() = %q, want 'codex'", b.Name())
	}
}

func TestNewClaudeCodeBackend_ReturnsNonNil(t *testing.T) {
	b := NewClaudeCodeBackend("claude-sonnet-4-5", 10, false)
	if b == nil {
		t.Error("NewClaudeCodeBackend should return non-nil")
	}
	if b.Name() == "" {
		t.Error("Name() should not be empty")
	}
}

// ---------------------------------------------------------------------------
// SaveTicketFile / LoadTicketFile (ticket.go)
// ---------------------------------------------------------------------------

func TestSaveAndLoadTicketFile_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	content := "# [feat] add new feature\n\nSome ticket content here."

	if _, err := SaveTicketFile(dir, 5, content); err != nil {
		t.Fatalf("SaveTicketFile: %v", err)
	}

	loaded, err := LoadTicketFile(dir, 5)
	if err != nil {
		t.Fatalf("LoadTicketFile: %v", err)
	}
	if loaded != content {
		t.Errorf("loaded = %q, want %q", loaded, content)
	}
}

func TestLoadTicketFile_Missing(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadTicketFile(dir, 999)
	if err == nil {
		t.Error("LoadTicketFile for missing file should return error")
	}
}

func TestSaveTicketFile_ExtractsTitleLine(t *testing.T) {
	dir := t.TempDir()
	content := "# [fix] fix login bug\n\nDetails..."

	ticketPath, err := SaveTicketFile(dir, 10, content)
	if err != nil {
		t.Fatalf("SaveTicketFile: %v", err)
	}

	// File should exist
	if _, err := os.Stat(ticketPath); err != nil {
		t.Errorf("ticket file missing: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ParseVerificationCommands (ticket.go)
// ---------------------------------------------------------------------------

func TestParseVerificationCommands_Basic(t *testing.T) {
	content := `# Task

## Verification
- backend: ` + "`go test ./...`" + `
- frontend: ` + "`npm test`" + `
`
	cmds := ParseVerificationCommands(content)
	if len(cmds) == 0 {
		t.Error("ParseVerificationCommands should return at least one command")
	}
}

func TestParseVerificationCommands_EmptyContent(t *testing.T) {
	cmds := ParseVerificationCommands("")
	if len(cmds) != 0 {
		t.Errorf("ParseVerificationCommands('') should return empty slice, got %v", cmds)
	}
}

func TestParseVerificationCommands_NoVerificationSection(t *testing.T) {
	content := "# Task\n\n## Scope\n- Do something\n"
	cmds := ParseVerificationCommands(content)
	if len(cmds) != 0 {
		t.Errorf("ParseVerificationCommands with no verification section should return empty, got %v", cmds)
	}
}
