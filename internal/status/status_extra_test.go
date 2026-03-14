package status

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// readIntFile (status.go)
// ---------------------------------------------------------------------------

func TestReadIntFile_Valid(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "count.txt")
	os.WriteFile(f, []byte("42"), 0644)

	val, err := readIntFile(f)
	if err != nil {
		t.Fatalf("readIntFile: %v", err)
	}
	if val != 42 {
		t.Errorf("readIntFile = %d, want 42", val)
	}
}

func TestReadIntFile_NotFound(t *testing.T) {
	_, err := readIntFile("/nonexistent/count.txt")
	if err == nil {
		t.Error("readIntFile for missing file should return error")
	}
}

func TestReadIntFile_Empty(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "empty.txt")
	os.WriteFile(f, []byte(""), 0644)

	_, err := readIntFile(f)
	if err == nil {
		t.Error("readIntFile for empty file should return error")
	}
}

func TestReadIntFile_Invalid(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "bad.txt")
	os.WriteFile(f, []byte("notanumber"), 0644)

	_, err := readIntFile(f)
	if err == nil {
		t.Error("readIntFile for non-numeric file should return error")
	}
}

// ---------------------------------------------------------------------------
// parseIssueIDFromFilename (status.go)
// ---------------------------------------------------------------------------

func TestParseIssueIDFromFilename_Valid(t *testing.T) {
	id := parseIssueIDFromFilename("issue-42.json")
	if id != 42 {
		t.Errorf("parseIssueIDFromFilename('issue-42.json') = %d, want 42", id)
	}
}

func TestParseIssueIDFromFilename_Invalid(t *testing.T) {
	id := parseIssueIDFromFilename("not-an-issue.txt")
	if id != 0 {
		t.Errorf("parseIssueIDFromFilename(invalid) = %d, want 0", id)
	}
}

func TestParseIssueIDFromFilename_LargeNumber(t *testing.T) {
	id := parseIssueIDFromFilename("issue-1234.json")
	if id != 1234 {
		t.Errorf("parseIssueIDFromFilename('issue-1234.json') = %d, want 1234", id)
	}
}

// ---------------------------------------------------------------------------
// readSessionLog (status.go)
// ---------------------------------------------------------------------------

func TestReadSessionLog_Valid(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "session.log.json")

	log := rawSessionLog{
		SessionID: "session-123",
		Actions:   []SessionAction{{Type: "message"}},
	}
	data, _ := json.Marshal(log)
	os.WriteFile(f, data, 0644)

	result, err := readSessionLog(f)
	if err != nil {
		t.Fatalf("readSessionLog: %v", err)
	}
	if result.SessionID != "session-123" {
		t.Errorf("SessionID = %q, want 'session-123'", result.SessionID)
	}
	if len(result.Actions) != 1 {
		t.Errorf("Actions count = %d, want 1", len(result.Actions))
	}
}

func TestReadSessionLog_NotFound(t *testing.T) {
	_, err := readSessionLog("/nonexistent/session.json")
	if err == nil {
		t.Error("readSessionLog for missing file should return error")
	}
}

func TestReadSessionLog_NilActions_Initialized(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "session.json")
	// No Actions field in the JSON
	os.WriteFile(f, []byte(`{"session_id":"s1"}`), 0644)

	result, err := readSessionLog(f)
	if err != nil {
		t.Fatalf("readSessionLog: %v", err)
	}
	if result.Actions == nil {
		t.Error("Actions should be initialized to empty slice when nil in JSON")
	}
}

// ---------------------------------------------------------------------------
// collectConfigInfo (status.go)
// ---------------------------------------------------------------------------

func TestCollectConfigInfo_NoFile(t *testing.T) {
	dir := t.TempDir()
	info := collectConfigInfo(dir)
	// Should return empty ConfigInfo without error
	if len(info.RulesKit) != 0 || len(info.RulesCustom) != 0 {
		t.Errorf("collectConfigInfo(no file) should return empty info, got: %+v", info)
	}
}

func TestCollectConfigInfo_ValidYaml(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".ai", "config")
	os.MkdirAll(configDir, 0755)
	yaml := `rules:
  kit:
    - git-workflow
  custom:
    - backend-go
agents:
  builtin:
    - pr-reviewer
  custom:
    - name: my-agent
`
	os.WriteFile(filepath.Join(configDir, "workflow.yaml"), []byte(yaml), 0644)

	info := collectConfigInfo(dir)
	if len(info.RulesKit) != 1 || info.RulesKit[0] != "git-workflow" {
		t.Errorf("RulesKit = %v, want [git-workflow]", info.RulesKit)
	}
	if len(info.RulesCustom) != 1 || info.RulesCustom[0] != "backend-go" {
		t.Errorf("RulesCustom = %v, want [backend-go]", info.RulesCustom)
	}
	if len(info.AgentsBuiltin) != 1 || info.AgentsBuiltin[0] != "pr-reviewer" {
		t.Errorf("AgentsBuiltin = %v, want [pr-reviewer]", info.AgentsBuiltin)
	}
	if len(info.AgentsCustom) != 1 || info.AgentsCustom[0] != "my-agent" {
		t.Errorf("AgentsCustom = %v, want [my-agent]", info.AgentsCustom)
	}
}

func TestCollectConfigInfo_InvalidYaml(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".ai", "config")
	os.MkdirAll(configDir, 0755)
	os.WriteFile(filepath.Join(configDir, "workflow.yaml"), []byte("[invalid yaml"), 0644)

	info := collectConfigInfo(dir)
	// Should return empty ConfigInfo on parse error
	if len(info.RulesKit) != 0 {
		t.Errorf("collectConfigInfo(invalid yaml) should return empty info, got: %+v", info)
	}
}

// ---------------------------------------------------------------------------
// fileExists (status.go)
// ---------------------------------------------------------------------------

func TestFileExists_Status_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.txt")
	os.WriteFile(f, []byte("content"), 0644)
	if !fileExists(f) {
		t.Error("fileExists should return true for existing file")
	}
}

func TestFileExists_Status_Missing(t *testing.T) {
	if fileExists("/nonexistent/path/file.txt") {
		t.Error("fileExists should return false for missing file")
	}
}
