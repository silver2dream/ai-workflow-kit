//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/reset"
)

// stubGHClient is a minimal stub satisfying analyzer.GitHubClientInterface.
// All methods return empty/zero values so no real GitHub calls are made.
type stubGHClient struct{}

func (s *stubGHClient) ListIssuesByLabel(_ context.Context, _ string) ([]analyzer.Issue, error) {
	return nil, nil
}
func (s *stubGHClient) ListPendingIssues(_ context.Context, _ analyzer.LabelsConfig) ([]analyzer.Issue, error) {
	return nil, nil
}
func (s *stubGHClient) CountOpenIssues(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (s *stubGHClient) RemoveLabel(_ context.Context, _ int, _ string) error  { return nil }
func (s *stubGHClient) AddLabel(_ context.Context, _ int, _ string) error     { return nil }
func (s *stubGHClient) IsPRMerged(_ context.Context, _ int) (bool, error)     { return false, nil }
func (s *stubGHClient) CloseIssue(_ context.Context, _ int) error             { return nil }
func (s *stubGHClient) FindPRByBranch(_ context.Context, _ string) (int, error) { return 0, nil }
func (s *stubGHClient) GetIssueBody(_ context.Context, _ int) (string, error) { return "", nil }
func (s *stubGHClient) UpdateIssueBody(_ context.Context, _ int, _ string) error { return nil }

// setupProject creates a minimal .ai/ project structure in a temp directory
// and returns the temp dir path. The directory is automatically removed when
// the test ends.
func setupProject(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "awk_integration_*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	// Create directory skeleton.
	dirs := []string{
		".ai/config",
		".ai/state",
		".ai/state/attempts",
		".ai/results",
		".ai/specs/myfeature",
		".ai/temp",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(dir, d), 0755); err != nil {
			t.Fatalf("MkdirAll %s: %v", d, err)
		}
	}

	// Minimal workflow.yaml — epic mode, no active tasks.md specs.
	cfg := `version: "1.2"
project:
  name: test-project
  type: root
repos:
  - name: root
    type: root
    path: ./
    language: go
    verify:
      build: "go build ./..."
      test: "go test ./..."
git:
  integration_branch: develop
  release_branch: main
specs:
  base_path: .ai/specs
  active: []
  tracking:
    mode: github_epic
github:
  repo: owner/repo
  labels:
    pending: pending
    in_progress: in-progress
    pr_ready: pr-ready
    review_failed: review-failed
    worker_failed: worker-failed
    needs_human_review: needs-human-review
escalation:
  max_consecutive_failures: 5
`
	cfgPath := filepath.Join(dir, ".ai", "config", "workflow.yaml")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
		t.Fatalf("WriteFile workflow.yaml: %v", err)
	}

	return dir
}

// writeStateFile writes a string value to a state file.
func writeStateFile(t *testing.T, dir, name, value string) {
	t.Helper()
	p := filepath.Join(dir, ".ai", "state", name)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatalf("MkdirAll for state file: %v", err)
	}
	if err := os.WriteFile(p, []byte(value), 0644); err != nil {
		t.Fatalf("WriteFile state/%s: %v", name, err)
	}
}

// loadCfg is a helper that loads and validates workflow.yaml from dir.
func loadCfg(t *testing.T, dir string) *analyzer.Config {
	t.Helper()
	cfgPath := filepath.Join(dir, ".ai", "config", "workflow.yaml")
	cfg, err := analyzer.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	return cfg
}

// TestIntegration_AnalyzeNext_ReturnsValidDecision verifies that Decide()
// returns a structurally valid Decision for a freshly-created project that has
// no pending GitHub issues.
func TestIntegration_AnalyzeNext_ReturnsValidDecision(t *testing.T) {
	dir := setupProject(t)
	cfg := loadCfg(t, dir)

	a := analyzer.New(dir, cfg)
	a.GHClient = &stubGHClient{}

	decision, err := a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if decision == nil {
		t.Fatal("Decide returned nil decision")
	}

	// NextAction must be one of the known constants.
	knownActions := map[string]bool{
		analyzer.ActionGenerateTasks:  true,
		analyzer.ActionCreateTask:     true,
		analyzer.ActionDispatchWorker: true,
		analyzer.ActionCheckResult:    true,
		analyzer.ActionReviewPR:       true,
		analyzer.ActionAuditEpic:      true,
		analyzer.ActionAllComplete:    true,
		analyzer.ActionNone:           true,
	}
	if !knownActions[decision.NextAction] {
		t.Errorf("Decide returned unknown NextAction %q", decision.NextAction)
	}
}

// TestIntegration_FullWorkflowState verifies that state files drive Decision
// transitions correctly.
func TestIntegration_FullWorkflowState(t *testing.T) {
	t.Run("zero_loop_count_allows_progress", func(t *testing.T) {
		dir := setupProject(t)
		writeStateFile(t, dir, "loop_count", "0")

		a := analyzer.New(dir, loadCfg(t, dir))
		a.GHClient = &stubGHClient{}

		decision, err := a.Decide(context.Background())
		if err != nil {
			t.Fatalf("Decide: %v", err)
		}
		// A zero loop count must NOT cause the max-loop exit.
		if decision.ExitReason == analyzer.ReasonMaxLoopReached {
			t.Errorf("loop_count=0 should not trigger %s", analyzer.ReasonMaxLoopReached)
		}
	})

	t.Run("max_consecutive_failures_stops_workflow", func(t *testing.T) {
		dir := setupProject(t)
		// 5 equals MaxConsecutiveFailures constant in the analyzer package.
		writeStateFile(t, dir, "consecutive_failures", "5")

		a := analyzer.New(dir, loadCfg(t, dir))
		a.GHClient = &stubGHClient{}

		decision, err := a.Decide(context.Background())
		if err != nil {
			t.Fatalf("Decide: %v", err)
		}
		if decision.NextAction != analyzer.ActionNone {
			t.Errorf("NextAction = %q, want %q", decision.NextAction, analyzer.ActionNone)
		}
		if decision.ExitReason != analyzer.ReasonMaxConsecutiveFailures {
			t.Errorf("ExitReason = %q, want %q",
				decision.ExitReason, analyzer.ReasonMaxConsecutiveFailures)
		}
	})
}

// TestIntegration_ResetCleansAllState verifies that Resetter.ResetAll removes
// all known state files written to the project directory.
func TestIntegration_ResetCleansAllState(t *testing.T) {
	dir := setupProject(t)

	// Write state files that reset should clean up.
	writeStateFile(t, dir, "loop_count", "7")
	writeStateFile(t, dir, "consecutive_failures", "2")
	writeStateFile(t, dir, "STOP", "")

	// Create a lock file.
	lockPath := filepath.Join(dir, ".ai", "state", "kickoff.lock")
	if err := os.WriteFile(lockPath, []byte(`{"pid":0}`), 0644); err != nil {
		t.Fatalf("WriteFile lock: %v", err)
	}

	r := reset.New(dir)
	results := r.ResetAll(context.Background())

	// At least one operation must succeed.
	anySuccess := false
	for _, res := range results {
		if res.Success {
			anySuccess = true
		} else {
			t.Logf("reset op %q not successful: %s", res.Name, res.Message)
		}
	}
	if !anySuccess {
		t.Error("ResetAll returned no successful operations")
	}

	// loop_count and consecutive_failures should be deleted.
	for _, name := range []string{"loop_count", "consecutive_failures"} {
		p := filepath.Join(dir, ".ai", "state", name)
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("state file %q still exists after reset", name)
		}
	}

	// STOP marker should be gone.
	stopPath := filepath.Join(dir, ".ai", "state", "STOP")
	if _, err := os.Stat(stopPath); !os.IsNotExist(err) {
		t.Error("STOP marker still exists after reset")
	}
}
