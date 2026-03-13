package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// FindSensitiveMatches additional cases (security.go)
// ---------------------------------------------------------------------------

func TestFindSensitiveMatches_GithubToken(t *testing.T) {
	diff := "+GITHUB_TOKEN=ghp_1234567890abcdef"
	matches := findSensitiveMatches(diff, nil)
	if len(matches) == 0 {
		t.Error("findSensitiveMatches should detect GITHUB_TOKEN pattern")
	}
}

func TestFindSensitiveMatches_RSAPrivateKey(t *testing.T) {
	diff := "+-----BEGIN RSA PRIVATE KEY-----"
	matches := findSensitiveMatches(diff, nil)
	if len(matches) == 0 {
		t.Error("findSensitiveMatches should detect RSA private key pattern")
	}
}

func TestFindSensitiveMatches_AWSSecretKey(t *testing.T) {
	diff := "+AWS_SECRET_ACCESS_KEY=abc123"
	matches := findSensitiveMatches(diff, nil)
	if len(matches) == 0 {
		t.Error("findSensitiveMatches should detect AWS_SECRET_ACCESS_KEY pattern")
	}
}

func TestFindSensitiveMatches_AccessToken(t *testing.T) {
	diff := `+access_token: "Bearer xyz123"`
	matches := findSensitiveMatches(diff, nil)
	if len(matches) == 0 {
		t.Error("findSensitiveMatches should detect access_token pattern")
	}
}

// ---------------------------------------------------------------------------
// NormalizePath additional cases (security.go)
// ---------------------------------------------------------------------------

func TestNormalizePath_MixedSlashes(t *testing.T) {
	got := normalizePath(`src\dir/file.go`)
	if strings.Contains(got, "\\") {
		t.Errorf("normalizePath should convert all backslashes, got: %q", got)
	}
}

func TestNormalizePath_MultipleTrailingSlashes(t *testing.T) {
	got := normalizePath("src/dir/")
	if strings.HasSuffix(got, "/") {
		t.Errorf("normalizePath should remove trailing slash, got: %q", got)
	}
}

// ---------------------------------------------------------------------------
// FindProtectedChanges additional cases (security.go)
// ---------------------------------------------------------------------------

func TestFindProtectedChanges_CommandsViolation(t *testing.T) {
	files := []string{".ai/commands/run.sh"}
	violations := findProtectedChanges(files, "")
	if len(violations) == 0 {
		t.Error("findProtectedChanges should detect .ai/commands/ path as violation")
	}
}

func TestFindProtectedChanges_PartialMatch(t *testing.T) {
	// File that partially matches but doesn't start with protected path
	files := []string{"other/.ai/scripts/deploy.sh"}
	violations := findProtectedChanges(files, "")
	// This should NOT be a violation (it doesn't start with .ai/scripts/)
	if len(violations) != 0 {
		t.Errorf("partial path match should not be a violation, got: %v", violations)
	}
}

// ---------------------------------------------------------------------------
// SplitLines additional cases (security.go)
// ---------------------------------------------------------------------------

func TestSplitLines_AllEmpty(t *testing.T) {
	input := "\n\n\n"
	got := splitLines(input)
	if len(got) != 0 {
		t.Errorf("splitLines with all empty lines should return empty, got: %v", got)
	}
}

func TestSplitLines_SingleLine(t *testing.T) {
	input := "single.go"
	got := splitLines(input)
	if len(got) != 1 {
		t.Errorf("splitLines single line = %v (len=%d), want 1", got, len(got))
	}
	if got[0] != "single.go" {
		t.Errorf("splitLines single = %q, want 'single.go'", got[0])
	}
}

// ---------------------------------------------------------------------------
// SaveTicketFile (ticket.go) — additional coverage
// ---------------------------------------------------------------------------

func TestSaveTicketFile_Success(t *testing.T) {
	dir := t.TempDir()
	body := "# Issue Title\n\nIssue body content."

	path, err := SaveTicketFile(dir, 42, body)
	if err != nil {
		t.Fatalf("SaveTicketFile: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("ticket file not created at %s: %v", path, err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != body {
		t.Errorf("ticket file content mismatch")
	}
}

func TestSaveTicketFile_ExpectedPath(t *testing.T) {
	dir := t.TempDir()
	path, err := SaveTicketFile(dir, 99, "body content")
	if err != nil {
		t.Fatalf("SaveTicketFile: %v", err)
	}

	expectedPath := filepath.Join(dir, ".ai", "temp", "ticket-99.md")
	if path != expectedPath {
		t.Errorf("path = %q, want %q", path, expectedPath)
	}
}

// ---------------------------------------------------------------------------
// ReadFailCount additional cases (result.go)
// ---------------------------------------------------------------------------

func TestReadFailCount_InvalidContent(t *testing.T) {
	dir := t.TempDir()
	runDir := filepath.Join(dir, ".ai", "runs", "issue-1")
	os.MkdirAll(runDir, 0755)
	os.WriteFile(filepath.Join(runDir, "fail_count.txt"), []byte("not-a-number"), 0644)

	got := ReadFailCount(dir, 1)
	if got != 0 {
		t.Errorf("ReadFailCount with invalid content = %d, want 0", got)
	}
}

func TestReadFailCount_ValidCount(t *testing.T) {
	dir := t.TempDir()
	runDir := filepath.Join(dir, ".ai", "runs", "issue-5")
	os.MkdirAll(runDir, 0755)
	os.WriteFile(filepath.Join(runDir, "fail_count.txt"), []byte("3"), 0644)

	got := ReadFailCount(dir, 5)
	if got != 3 {
		t.Errorf("ReadFailCount = %d, want 3", got)
	}
}

// ---------------------------------------------------------------------------
// WriteResultAtomic — new coverage
// ---------------------------------------------------------------------------

func TestWriteResultAtomic_CreatesResultDir(t *testing.T) {
	dir := t.TempDir()
	result := &IssueResult{
		IssueID: "77",
		Status:  "success",
	}

	if err := WriteResultAtomic(dir, 77, result); err != nil {
		t.Fatalf("WriteResultAtomic: %v", err)
	}

	resultPath := filepath.Join(dir, ".ai", "results", "issue-77.json")
	if _, err := os.Stat(resultPath); err != nil {
		t.Errorf("result file should exist at %s: %v", resultPath, err)
	}
}

// ---------------------------------------------------------------------------
// BuildCommitMessage additional cases (commit.go)
// ---------------------------------------------------------------------------

func TestBuildCommitMessage_AllCaps(t *testing.T) {
	got := BuildCommitMessage("[FEAT] ADD FEATURE")
	// type should be lowercase
	if strings.HasPrefix(got, "[FEAT]") {
		t.Errorf("BuildCommitMessage should normalize type to lowercase, got: %q", got)
	}
}

func TestBuildCommitMessage_OnlySpaces(t *testing.T) {
	got := BuildCommitMessage("   ")
	if got == "" {
		t.Error("BuildCommitMessage with spaces should not return empty")
	}
}

func TestBuildCommitMessage_WithDash(t *testing.T) {
	got := BuildCommitMessage("[fix] handle-edge-case")
	if !strings.Contains(got, "handle-edge-case") {
		t.Errorf("BuildCommitMessage should preserve dashes in subject, got: %q", got)
	}
}
