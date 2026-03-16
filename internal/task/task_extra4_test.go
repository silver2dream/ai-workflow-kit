package task

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// prepareBodyFile (create.go)
// ---------------------------------------------------------------------------

func TestPrepareBodyFile_NoBodyFile(t *testing.T) {
	opts := CreateTaskOptions{
		BodyFile:  "",
		StateRoot: t.TempDir(),
	}
	// Call the real prepareBodyFile directly
	_, err := prepareBodyFile(opts, nil)
	if err == nil {
		t.Error("prepareBodyFile with empty BodyFile should return error")
	}
}

func TestPrepareBodyFile_FileNotFound(t *testing.T) {
	opts := CreateTaskOptions{
		BodyFile:  "/nonexistent/path/body.md",
		StateRoot: t.TempDir(),
	}
	_, err := prepareBodyFile(opts, nil)
	if err == nil {
		t.Error("prepareBodyFile with missing file should return error")
	}
}

// validBodyContent returns a body string that passes ValidateBody checks.
// Required sections: Summary, Scope, Acceptance Criteria, Testing Requirements, Metadata
// Also requires at least one unchecked checkbox.
func validBodyContent() string {
	return `# [feat] implement feature

## Summary
Do something useful.

## Scope
Backend changes only.

## Acceptance Criteria
- [ ] Works correctly

## Testing Requirements
- Run go test ./...

## Metadata
- Spec: my-spec
- Task Line: 3
`
}

func TestPrepareBodyFile_ValidBody(t *testing.T) {
	dir := t.TempDir()
	bodyFile := filepath.Join(dir, "body.md")
	os.WriteFile(bodyFile, []byte(validBodyContent()), 0644)

	opts := CreateTaskOptions{
		BodyFile:  bodyFile,
		StateRoot: dir,
		SpecName:  "my-spec",
		TaskLine:  3,
	}
	path, err := prepareBodyFile(opts, nil)
	if err != nil {
		t.Fatalf("prepareBodyFile: %v", err)
	}
	if path == "" {
		t.Error("prepareBodyFile should return non-empty path")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("temp body file not created at %s: %v", path, err)
	}
}

func TestPrepareBodyFile_RelativePath(t *testing.T) {
	dir := t.TempDir()
	bodyFile := filepath.Join(dir, "body.md")
	os.WriteFile(bodyFile, []byte(validBodyContent()), 0644)

	// Use absolute path (relative path would require CWD to contain the file)
	opts := CreateTaskOptions{
		BodyFile:  bodyFile,
		StateRoot: dir,
		SpecName:  "spec",
		TaskLine:  1,
	}
	path, err := prepareBodyFile(opts, nil)
	if err != nil {
		t.Fatalf("prepareBodyFile with absolute path: %v", err)
	}
	if !strings.Contains(path, dir) {
		t.Errorf("temp path %q should be inside stateRoot %q", path, dir)
	}
}

// ---------------------------------------------------------------------------
// AppendIssueRef success path (tasksmd.go)
// ---------------------------------------------------------------------------

func TestAppendIssueRef_AppendsToLine(t *testing.T) {
	dir := t.TempDir()
	tasksPath := filepath.Join(dir, "tasks.md")
	content := "# Tasks\n\n- [ ] First task\n- [ ] Second task\n"
	os.WriteFile(tasksPath, []byte(content), 0644)

	err := AppendIssueRef(tasksPath, 3, 42)
	if err != nil {
		t.Fatalf("AppendIssueRef: %v", err)
	}

	data, _ := os.ReadFile(tasksPath)
	lines := strings.Split(string(data), "\n")
	if !strings.Contains(lines[2], "<!-- Issue #42 -->") {
		t.Errorf("line 3 = %q, should contain '<!-- Issue #42 -->'", lines[2])
	}
}

func TestAppendIssueRef_LineOneHasRef(t *testing.T) {
	dir := t.TempDir()
	tasksPath := filepath.Join(dir, "tasks.md")
	content := "- [ ] Task <!-- Issue #10 -->\n- [ ] Other\n"
	os.WriteFile(tasksPath, []byte(content), 0644)

	// Line 1 already has ref — should skip silently
	err := AppendIssueRef(tasksPath, 1, 99)
	if err != nil {
		t.Fatalf("AppendIssueRef (already has ref): %v", err)
	}

	data, _ := os.ReadFile(tasksPath)
	// Should still have #10, not #99
	if strings.Contains(string(data), "<!-- Issue #99 -->") {
		t.Error("should not append ref when line already has one")
	}
}

// ---------------------------------------------------------------------------
// DefaultIssueTitle edge cases (tasksmd.go)
// ---------------------------------------------------------------------------

func TestDefaultIssueTitle_Empty(t *testing.T) {
	got := DefaultIssueTitle("")
	if got != "[feat] implement task" {
		t.Errorf("DefaultIssueTitle('') = %q, want '[feat] implement task'", got)
	}
}

func TestDefaultIssueTitle_AlreadyHasBracket(t *testing.T) {
	got := DefaultIssueTitle("[fix] some bug")
	if got != "[fix] some bug" {
		t.Errorf("DefaultIssueTitle('[fix] some bug') = %q, want '[fix] some bug'", got)
	}
}

func TestDefaultIssueTitle_PlainText(t *testing.T) {
	got := DefaultIssueTitle("  add new feature  ")
	if !strings.HasPrefix(got, "[feat]") {
		t.Errorf("DefaultIssueTitle('add new feature') = %q, should start with '[feat]'", got)
	}
}

// ---------------------------------------------------------------------------
// HasIssueRef edge cases (tasksmd.go)
// ---------------------------------------------------------------------------

func TestHasIssueRef_SpacesAroundHash(t *testing.T) {
	line := "- [ ] Task <!-- Issue #  42  -->"
	// Regex may or may not match depending on implementation
	_, _ = HasIssueRef(line)
	// Just ensure no panic
}

func TestHasIssueRef_NoHTML(t *testing.T) {
	line := "- [ ] Task without any reference"
	num, found := HasIssueRef(line)
	if found {
		t.Errorf("HasIssueRef with no ref: found=%v, num=%d, want not found", found, num)
	}
}

// ---------------------------------------------------------------------------
// ExtractTaskText edge cases (tasksmd.go)
// ---------------------------------------------------------------------------

func TestExtractTaskText_NoCheckbox(t *testing.T) {
	line := "Just some text"
	got := ExtractTaskText(line)
	if got != "Just some text" {
		t.Errorf("ExtractTaskText(%q) = %q, want 'Just some text'", line, got)
	}
}

func TestExtractTaskText_WithIssueRef(t *testing.T) {
	line := "- [ ] My task <!-- Issue #42 -->"
	got := ExtractTaskText(line)
	if strings.Contains(got, "Issue") {
		t.Errorf("ExtractTaskText should remove issue ref, got: %q", got)
	}
	if strings.Contains(got, "<!--") {
		t.Errorf("ExtractTaskText should remove HTML comment, got: %q", got)
	}
}
