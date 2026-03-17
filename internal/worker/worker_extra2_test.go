package worker

import (
	"os"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// NewAttemptGuard
// ---------------------------------------------------------------------------

func TestNewAttemptGuard_Defaults(t *testing.T) {
	t.Setenv("AI_MAX_ATTEMPTS", "")
	dir := t.TempDir()
	g := NewAttemptGuard(dir, 42)
	if g.IssueID != 42 {
		t.Errorf("IssueID = %d, want 42", g.IssueID)
	}
	if g.StateRoot != dir {
		t.Errorf("StateRoot = %q, want %q", g.StateRoot, dir)
	}
	if g.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3 (default)", g.MaxAttempts)
	}
}

func TestNewAttemptGuard_FromEnv(t *testing.T) {
	t.Setenv("AI_MAX_ATTEMPTS", "5")
	dir := t.TempDir()
	g := NewAttemptGuard(dir, 1)
	if g.MaxAttempts != 5 {
		t.Errorf("MaxAttempts = %d, want 5 from env", g.MaxAttempts)
	}
}

func TestNewAttemptGuard_InvalidEnv(t *testing.T) {
	t.Setenv("AI_MAX_ATTEMPTS", "not-a-number")
	dir := t.TempDir()
	g := NewAttemptGuard(dir, 1)
	if g.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3 (default when env is invalid)", g.MaxAttempts)
	}
}

func TestNewAttemptGuard_ZeroEnv(t *testing.T) {
	t.Setenv("AI_MAX_ATTEMPTS", "0")
	dir := t.TempDir()
	g := NewAttemptGuard(dir, 1)
	// 0 is not > 0, so default applies
	if g.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3 (default when env is 0)", g.MaxAttempts)
	}
}

// ---------------------------------------------------------------------------
// security.go pure functions: splitLines, findProtectedChanges, findSensitiveMatches, normalizePath
// ---------------------------------------------------------------------------

func TestSplitLines_Basic(t *testing.T) {
	lines := splitLines("a\nb\nc")
	if len(lines) != 3 {
		t.Fatalf("len = %d, want 3", len(lines))
	}
	if lines[0] != "a" || lines[1] != "b" || lines[2] != "c" {
		t.Errorf("lines = %v, want [a b c]", lines)
	}
}

func TestSplitLines_TrimsAndFiltersEmpty(t *testing.T) {
	lines := splitLines("  a  \n\n  b  \n")
	if len(lines) != 2 {
		t.Fatalf("len = %d, want 2", len(lines))
	}
	if lines[0] != "a" || lines[1] != "b" {
		t.Errorf("lines = %v, want [a b]", lines)
	}
}

func TestSplitLines_Empty(t *testing.T) {
	lines := splitLines("")
	if len(lines) != 0 {
		t.Errorf("len = %d, want 0", len(lines))
	}
}

func TestSplitLines_OnlyNewlines(t *testing.T) {
	lines := splitLines("\n\n\n")
	if len(lines) != 0 {
		t.Errorf("len = %d, want 0", len(lines))
	}
}

func TestFindProtectedChanges_DetectsProtected(t *testing.T) {
	files := []string{
		".ai/scripts/run.sh",
		"backend/main.go",
	}
	violations := findProtectedChanges(files, "")
	if len(violations) != 1 {
		t.Fatalf("len = %d, want 1", len(violations))
	}
	if violations[0] != ".ai/scripts/run.sh" {
		t.Errorf("violation = %q, want .ai/scripts/run.sh", violations[0])
	}
}

func TestFindProtectedChanges_WhitelistedSkipped(t *testing.T) {
	files := []string{".ai/scripts/run.sh"}
	violations := findProtectedChanges(files, ".ai/scripts/run.sh")
	if len(violations) != 0 {
		t.Errorf("expected 0 violations with whitelist, got %v", violations)
	}
}

func TestFindProtectedChanges_NormalFile(t *testing.T) {
	files := []string{"backend/service.go", "frontend/ui.js"}
	violations := findProtectedChanges(files, "")
	if len(violations) != 0 {
		t.Errorf("expected 0 violations for normal files, got %v", violations)
	}
}

func TestFindProtectedChanges_EmptyFile_Skipped(t *testing.T) {
	// Empty string in file list should be skipped
	files := []string{"", ".ai/commands/cmd.sh", ""}
	violations := findProtectedChanges(files, "")
	if len(violations) != 1 {
		t.Fatalf("len = %d, want 1", len(violations))
	}
}

func TestFindSensitiveMatches_DetectsPassword(t *testing.T) {
	diff := `+password: "my-secret-pass"`
	matches := findSensitiveMatches(diff, nil)
	if len(matches) == 0 {
		t.Error("expected to detect password pattern")
	}
}

func TestFindSensitiveMatches_DetectsAPIKey(t *testing.T) {
	diff := `+api_key = "sk-abc123"`
	matches := findSensitiveMatches(diff, nil)
	if len(matches) == 0 {
		t.Error("expected to detect API key pattern")
	}
}

func TestFindSensitiveMatches_NoMatch(t *testing.T) {
	diff := `+name: "john"`
	matches := findSensitiveMatches(diff, nil)
	if len(matches) != 0 {
		t.Errorf("expected 0 matches for benign diff, got %v", matches)
	}
}

func TestFindSensitiveMatches_CustomPattern(t *testing.T) {
	diff := `+MY_SECRET_THING=12345`
	matches := findSensitiveMatches(diff, []string{`MY_SECRET_THING=\d+`})
	if len(matches) == 0 {
		t.Error("expected custom pattern to match")
	}
}

func TestFindSensitiveMatches_InvalidCustomPattern(t *testing.T) {
	// Invalid regex should be silently skipped
	diff := `+name: "ok"`
	matches := findSensitiveMatches(diff, []string{"[invalid(regex"})
	if len(matches) != 0 {
		t.Errorf("expected 0 matches for invalid regex, got %v", matches)
	}
}

func TestFindSensitiveMatches_EmptyCustomPattern(t *testing.T) {
	// Empty pattern string should be skipped
	diff := `+name: "ok"`
	matches := findSensitiveMatches(diff, []string{""})
	if len(matches) != 0 {
		t.Errorf("expected 0 matches for empty pattern, got %v", matches)
	}
}

func TestNormalizePath_BackslashToForward(t *testing.T) {
	path := normalizePath(`.ai\scripts\run.sh`)
	if strings.Contains(path, `\`) {
		t.Errorf("normalizePath should convert backslashes, got %q", path)
	}
	if !strings.HasPrefix(path, ".ai/") {
		t.Errorf("normalizePath = %q, should start with .ai/", path)
	}
}

func TestNormalizePath_TrailingSlashRemoved(t *testing.T) {
	path := normalizePath("backend/")
	if strings.HasSuffix(path, "/") {
		t.Errorf("normalizePath should remove trailing slash, got %q", path)
	}
}

// ---------------------------------------------------------------------------
// commit.go: BuildCommitMessage (already has tests but let's boost edge cases)
// ---------------------------------------------------------------------------

func TestBuildCommitMessage_TypeSubject(t *testing.T) {
	msg := BuildCommitMessage("[feat] add feature")
	if !strings.HasPrefix(msg, "[feat] add feature") {
		t.Errorf("BuildCommitMessage = %q, should start with [feat] add feature", msg)
	}
}

func TestBuildCommitMessage_WithBody(t *testing.T) {
	msg := BuildCommitMessage("[fix] fix bug\n\nDetails here")
	if !strings.Contains(msg, "[fix] fix bug") {
		t.Errorf("BuildCommitMessage = %q, should contain [fix] fix bug", msg)
	}
}

// ---------------------------------------------------------------------------
// WriteFileAtomic covers the remove-then-write path
// ---------------------------------------------------------------------------

func TestWriteFileAtomic_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/output.txt"
	data := []byte("hello world")
	if err := WriteFileAtomic(path, data, 0644); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Errorf("content = %q, want %q", got, data)
	}
}

func TestWriteFileAtomic_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/output.txt"
	os.WriteFile(path, []byte("old"), 0644)

	if err := WriteFileAtomic(path, []byte("new"), 0644); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "new" {
		t.Errorf("content = %q, want %q", got, "new")
	}
}
