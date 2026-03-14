package epicaudit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
)

// mockGHClient is a minimal implementation of analyzer.GitHubClientInterface for testing.
type mockGHClient struct {
	body string
	err  error
}

func (m *mockGHClient) GetIssueBody(_ context.Context, _ int) (string, error) {
	return m.body, m.err
}
func (m *mockGHClient) ListIssuesByLabel(_ context.Context, _ string) ([]analyzer.Issue, error) {
	return nil, nil
}
func (m *mockGHClient) ListPendingIssues(_ context.Context, _ analyzer.LabelsConfig) ([]analyzer.Issue, error) {
	return nil, nil
}
func (m *mockGHClient) CountOpenIssues(_ context.Context, _ string) (int, error) { return 0, nil }
func (m *mockGHClient) RemoveLabel(_ context.Context, _ int, _ string) error      { return nil }
func (m *mockGHClient) AddLabel(_ context.Context, _ int, _ string) error         { return nil }
func (m *mockGHClient) IsPRMerged(_ context.Context, _ int) (bool, error)         { return false, nil }
func (m *mockGHClient) CloseIssue(_ context.Context, _ int) error                 { return nil }
func (m *mockGHClient) FindPRByBranch(_ context.Context, _ string) (int, error)   { return 0, nil }
func (m *mockGHClient) UpdateIssueBody(_ context.Context, _ int, _ string) error  { return nil }

// compile-time check
var _ analyzer.GitHubClientInterface = (*mockGHClient)(nil)

// buildTestStateRoot creates a minimal state root with a workflow.yaml configured
// for github_epic tracking with the given spec and epic issue number.
func buildTestStateRoot(t *testing.T, specName string, epicIssue int) string {
	t.Helper()
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".ai", "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	cfg := `specs:
  base_path: .ai/specs
  tracking:
    mode: github_epic
    epic_issues:
      ` + specName + `: ` + itoa(epicIssue) + `
github:
  repo: owner/repo
repos:
  - name: backend
  - name: frontend
`
	if err := os.WriteFile(filepath.Join(configDir, "workflow.yaml"), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 10)
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}

// --- RunAudit: missing config ---

func TestRunAudit_MissingConfig(t *testing.T) {
	dir := t.TempDir() // no workflow.yaml

	_, err := RunAudit(context.Background(), AuditOptions{
		SpecName:  "myspec",
		StateRoot: dir,
	}, &mockGHClient{})
	if err == nil {
		t.Fatal("expected error when config is missing")
	}
}

// --- RunAudit: empty spec name ---

func TestRunAudit_EmptySpecName(t *testing.T) {
	_, err := RunAudit(context.Background(), AuditOptions{
		SpecName:  "",
		StateRoot: t.TempDir(),
	}, &mockGHClient{})
	if err == nil {
		t.Fatal("expected error for empty spec name")
	}
}

// --- RunAudit: non-epic tracking mode ---

func TestRunAudit_NonEpicMode(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".ai", "config")
	os.MkdirAll(configDir, 0755)
	cfg := `specs:
  base_path: .ai/specs
  tracking:
    mode: tasks_md
`
	os.WriteFile(filepath.Join(configDir, "workflow.yaml"), []byte(cfg), 0644)

	_, err := RunAudit(context.Background(), AuditOptions{
		SpecName:  "myspec",
		StateRoot: dir,
	}, &mockGHClient{})
	if err == nil {
		t.Fatal("expected error for non-epic mode")
	}
}

// --- RunAudit: no epic issue configured ---

func TestRunAudit_NoEpicIssue(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".ai", "config")
	os.MkdirAll(configDir, 0755)
	cfg := `specs:
  base_path: .ai/specs
  tracking:
    mode: github_epic
`
	os.WriteFile(filepath.Join(configDir, "workflow.yaml"), []byte(cfg), 0644)

	_, err := RunAudit(context.Background(), AuditOptions{
		SpecName:  "myspec",
		StateRoot: dir,
	}, &mockGHClient{})
	if err == nil {
		t.Fatal("expected error when no epic issue configured")
	}
}

// --- RunAudit: design file missing returns ok ---

func TestRunAudit_NoDesignFile(t *testing.T) {
	dir := buildTestStateRoot(t, "myspec", 10)

	report, err := RunAudit(context.Background(), AuditOptions{
		SpecName:  "myspec",
		StateRoot: dir,
	}, &mockGHClient{})
	if err != nil {
		t.Fatalf("RunAudit() error = %v", err)
	}
	if report.DesignExists {
		t.Error("DesignExists should be false when design.md is missing")
	}
	if report.SuggestedAction != "ok" {
		t.Errorf("SuggestedAction = %q, want ok", report.SuggestedAction)
	}
}

// --- RunAudit: GitHub API error ---

func TestRunAudit_GitHubError(t *testing.T) {
	dir := buildTestStateRoot(t, "myspec", 10)

	// Create a design.md so we proceed past the file-read step
	specDir := filepath.Join(dir, ".ai", "specs", "myspec")
	os.MkdirAll(specDir, 0755)
	os.WriteFile(filepath.Join(specDir, "design.md"), []byte("## Overview\nTest"), 0644)

	ghErr := errors.New("API error")
	_, err := RunAudit(context.Background(), AuditOptions{
		SpecName:  "myspec",
		StateRoot: dir,
	}, &mockGHClient{err: ghErr})
	if err == nil {
		t.Fatal("expected error from GitHub API failure")
	}
}

// --- RunAudit: full happy path ---

func TestRunAudit_HappyPath(t *testing.T) {
	dir := buildTestStateRoot(t, "myspec", 10)

	// Create design.md with sections and requirements
	specDir := filepath.Join(dir, ".ai", "specs", "myspec")
	os.MkdirAll(specDir, 0755)
	designContent := `## Overview
R1: WebSocket connections
R2: Game loop

## Architecture
Details about backend and frontend.

## Testing
Test plan.
`
	os.WriteFile(filepath.Join(specDir, "design.md"), []byte(designContent), 0644)

	// Epic body with two tasks (using #N format for issue references)
	epicBody := `## Tasks

- [ ] #11 Implement R1 backend WebSocket
- [x] #12 Implement R2 frontend game loop
`
	report, err := RunAudit(context.Background(), AuditOptions{
		SpecName:  "myspec",
		StateRoot: dir,
	}, &mockGHClient{body: epicBody})
	if err != nil {
		t.Fatalf("RunAudit() error = %v", err)
	}

	if report.SpecName != "myspec" {
		t.Errorf("SpecName = %q, want myspec", report.SpecName)
	}
	if report.EpicIssue != 10 {
		t.Errorf("EpicIssue = %d, want 10", report.EpicIssue)
	}
	if !report.DesignExists {
		t.Error("DesignExists should be true")
	}
	if report.TotalTasks != 2 {
		t.Errorf("TotalTasks = %d, want 2", report.TotalTasks)
	}
	if report.CompletedTasks != 1 {
		t.Errorf("CompletedTasks = %d, want 1", report.CompletedTasks)
	}
	if report.PendingTasks != 1 {
		t.Errorf("PendingTasks = %d, want 1", report.PendingTasks)
	}
	if len(report.DesignSections) == 0 {
		t.Error("DesignSections should not be empty")
	}
}

// --- RunAudit: with result files ---

func TestRunAudit_WithResultFiles(t *testing.T) {
	dir := buildTestStateRoot(t, "myspec", 10)

	// Create design.md
	specDir := filepath.Join(dir, ".ai", "specs", "myspec")
	os.MkdirAll(specDir, 0755)
	os.WriteFile(filepath.Join(specDir, "design.md"), []byte("## Overview\nTest"), 0644)

	// Create a result file for issue 11
	resultsDir := filepath.Join(dir, ".ai", "results")
	os.MkdirAll(resultsDir, 0755)
	os.WriteFile(filepath.Join(resultsDir, "issue-11.json"), []byte(`{"status":"merged"}`), 0644)

	epicBody := `## Tasks

- [x] #11 Implement backend
`
	report, err := RunAudit(context.Background(), AuditOptions{
		SpecName:  "myspec",
		StateRoot: dir,
	}, &mockGHClient{body: epicBody})
	if err != nil {
		t.Fatalf("RunAudit() error = %v", err)
	}

	if len(report.CompletedResults) == 0 {
		t.Error("expected CompletedResults to be non-empty when result file exists")
	}
	foundResult := false
	for _, r := range report.Tasks {
		if r.IssueNumber == 11 && r.HasResult {
			foundResult = true
		}
	}
	if !foundResult {
		t.Error("expected task with IssueNumber=11 to have HasResult=true")
	}
}
