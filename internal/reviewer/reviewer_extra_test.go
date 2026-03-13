package reviewer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// GetReviewSettings
// ---------------------------------------------------------------------------

func TestGetReviewSettings_Defaults_NoFile(t *testing.T) {
	dir := t.TempDir()
	settings := GetReviewSettings(dir)
	if settings.ScoreThreshold != 7 {
		t.Errorf("ScoreThreshold = %d, want 7 (default)", settings.ScoreThreshold)
	}
	if settings.MergeStrategy != "squash" {
		t.Errorf("MergeStrategy = %q, want 'squash' (default)", settings.MergeStrategy)
	}
}

func TestGetReviewSettings_FromConfig(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".ai", "config")
	os.MkdirAll(cfgDir, 0755)

	yaml := `review:
  score_threshold: 9
  merge_strategy: merge
`
	os.WriteFile(filepath.Join(cfgDir, "workflow.yaml"), []byte(yaml), 0644)

	settings := GetReviewSettings(dir)
	if settings.ScoreThreshold != 9 {
		t.Errorf("ScoreThreshold = %d, want 9", settings.ScoreThreshold)
	}
	if settings.MergeStrategy != "merge" {
		t.Errorf("MergeStrategy = %q, want 'merge'", settings.MergeStrategy)
	}
}

func TestGetReviewSettings_InvalidMergeStrategy(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".ai", "config")
	os.MkdirAll(cfgDir, 0755)

	yaml := `review:
  score_threshold: 5
  merge_strategy: invalid_strategy
`
	os.WriteFile(filepath.Join(cfgDir, "workflow.yaml"), []byte(yaml), 0644)

	settings := GetReviewSettings(dir)
	if settings.ScoreThreshold != 5 {
		t.Errorf("ScoreThreshold = %d, want 5", settings.ScoreThreshold)
	}
	// Invalid strategy should fall back to default "squash"
	if settings.MergeStrategy != "squash" {
		t.Errorf("MergeStrategy = %q, want 'squash' (default for invalid strategy)", settings.MergeStrategy)
	}
}

func TestGetReviewSettings_ValidStrategies(t *testing.T) {
	for _, strategy := range []string{"squash", "merge", "rebase"} {
		t.Run(strategy, func(t *testing.T) {
			dir := t.TempDir()
			cfgDir := filepath.Join(dir, ".ai", "config")
			os.MkdirAll(cfgDir, 0755)

			yaml := "review:\n  merge_strategy: " + strategy + "\n"
			os.WriteFile(filepath.Join(cfgDir, "workflow.yaml"), []byte(yaml), 0644)

			settings := GetReviewSettings(dir)
			if settings.MergeStrategy != strategy {
				t.Errorf("MergeStrategy = %q, want %q", settings.MergeStrategy, strategy)
			}
		})
	}
}

func TestGetReviewSettings_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".ai", "config")
	os.MkdirAll(cfgDir, 0755)
	os.WriteFile(filepath.Join(cfgDir, "workflow.yaml"), []byte("not: valid: yaml: ["), 0644)

	// Should return defaults on invalid YAML
	settings := GetReviewSettings(dir)
	if settings.ScoreThreshold != 7 {
		t.Errorf("ScoreThreshold = %d, want 7 (default)", settings.ScoreThreshold)
	}
}

// ---------------------------------------------------------------------------
// ReviewContext.FormatOutput
// ---------------------------------------------------------------------------

func TestReviewContext_FormatOutput_ContainsFields(t *testing.T) {
	rc := &ReviewContext{
		PRNumber:           42,
		IssueNumber:        10,
		PrincipalSessionID: "sess-abc",
		CIStatus:           "passed",
		WorktreePath:       "/tmp/worktrees/issue-10",
		TestCommand:        "go test ./...",
		Language:           "go",
		IssueJSON:          `{"title": "Fix bug"}`,
		CommitsJSON:        `[{"oid": "abc123"}]`,
	}

	out := rc.FormatOutput()

	checks := []string{
		"PR_NUMBER: 42",
		"ISSUE_NUMBER: 10",
		"PRINCIPAL_SESSION_ID: sess-abc",
		"CI_STATUS: passed",
		"WORKTREE_PATH: /tmp/worktrees/issue-10",
		"TEST_COMMAND: go test ./...",
		"LANGUAGE: go",
		"AWK PR REVIEW CONTEXT",
	}
	for _, c := range checks {
		if !strings.Contains(out, c) {
			t.Errorf("FormatOutput() missing %q", c)
		}
	}
}

func TestReviewContext_FormatOutput_WithTicket(t *testing.T) {
	rc := &ReviewContext{
		PRNumber:    5,
		IssueNumber: 2,
		Ticket:      "## Ticket\nFix the bug",
	}

	out := rc.FormatOutput()
	if !strings.Contains(out, "Fix the bug") {
		t.Errorf("FormatOutput() should include ticket content")
	}
	// When Ticket is set, IssueJSON should not be used
	if strings.Contains(out, rc.IssueJSON) && rc.IssueJSON != "" {
		t.Errorf("FormatOutput() should prefer Ticket over IssueJSON")
	}
}

func TestReviewContext_FormatOutput_WithTaskContent(t *testing.T) {
	rc := &ReviewContext{
		PRNumber:    3,
		IssueNumber: 1,
		TaskContent: "## Task\nDo this task",
	}

	out := rc.FormatOutput()
	if !strings.Contains(out, "TASK FILE") {
		t.Errorf("FormatOutput() should include TASK FILE section when TaskContent is set")
	}
	if !strings.Contains(out, "Do this task") {
		t.Errorf("FormatOutput() should include task content")
	}
}

func TestReviewContext_FormatOutput_WithoutTaskContent(t *testing.T) {
	rc := &ReviewContext{
		PRNumber:    3,
		IssueNumber: 1,
		TaskContent: "", // empty
	}

	out := rc.FormatOutput()
	if strings.Contains(out, "TASK FILE") {
		t.Errorf("FormatOutput() should NOT include TASK FILE section when TaskContent is empty")
	}
}

// ---------------------------------------------------------------------------
// ReviewContext.ToJSON
// ---------------------------------------------------------------------------

func TestReviewContext_ToJSON(t *testing.T) {
	rc := &ReviewContext{
		PRNumber:    99,
		IssueNumber: 50,
		CIStatus:    "passed",
		Language:    "rust",
	}

	json, err := rc.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() error = %v", err)
	}
	if !strings.Contains(json, `"pr_number": 99`) {
		t.Errorf("ToJSON() missing pr_number, got: %s", json)
	}
	if !strings.Contains(json, `"ci_status": "passed"`) {
		t.Errorf("ToJSON() missing ci_status, got: %s", json)
	}
	if !strings.Contains(json, `"language": "rust"`) {
		t.Errorf("ToJSON() missing language, got: %s", json)
	}
}

// ---------------------------------------------------------------------------
// isEpicMode
// ---------------------------------------------------------------------------

func TestIsEpicMode_NoConfig(t *testing.T) {
	dir := t.TempDir()
	// No config file → returns false (safe default)
	if isEpicMode(dir) {
		t.Error("isEpicMode with no config should return false")
	}
}

func TestIsEpicMode_TasksMdMode(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".ai", "config")
	os.MkdirAll(cfgDir, 0755)

	yaml := `specs:
  tracking:
    mode: tasks_md
`
	os.WriteFile(filepath.Join(cfgDir, "workflow.yaml"), []byte(yaml), 0644)

	if isEpicMode(dir) {
		t.Error("isEpicMode with tasks_md should return false")
	}
}

func TestIsEpicMode_GithubEpicMode(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".ai", "config")
	os.MkdirAll(cfgDir, 0755)

	yaml := `specs:
  tracking:
    mode: github_epic
`
	os.WriteFile(filepath.Join(cfgDir, "workflow.yaml"), []byte(yaml), 0644)

	if !isEpicMode(dir) {
		t.Error("isEpicMode with github_epic mode should return true")
	}
}

// ---------------------------------------------------------------------------
// cleanupWorktree (no actual git worktree, just tests the "not exists" path)
// ---------------------------------------------------------------------------

func TestCleanupWorktree_NonExistentWorktree(t *testing.T) {
	dir := t.TempDir()
	// Worktree doesn't exist — should return nil (no-op)
	err := cleanupWorktree(dir, 999)
	if err != nil {
		t.Errorf("cleanupWorktree with non-existent worktree should not error, got: %v", err)
	}
}
