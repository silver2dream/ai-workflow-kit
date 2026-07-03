package reviewer

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
)

func TestMultiReviewOrchestrator_RunAll_Parallel(t *testing.T) {
	// Replace claude runner with a fake that returns quickly.
	// The short sleep keeps Duration measurable on Windows, where the
	// clock granularity would otherwise round an instant return to 0.
	origRunner := claudeRunnerFunc
	claudeRunnerFunc = func(ctx context.Context, prompt, model string) (string, error) {
		time.Sleep(2 * time.Millisecond)
		return fmt.Sprintf("SCORE: 8\nFINDINGS:\nLooks good from %s", model), nil
	}
	defer func() { claudeRunnerFunc = origRunner }()

	o := &MultiReviewOrchestrator{
		PrimaryScore:    7,
		PrimaryFindings: "Primary review findings",
		Configs: []analyzer.SecondaryReviewerConfig{
			{Backend: "claude", Model: "opus", FocusArea: "security"},
			{Backend: "claude", Model: "sonnet", FocusArea: "performance"},
		},
		Timeout: 30 * time.Second,
	}

	results := o.RunAll("diff --git a/main.go\n+added line")

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	for i, r := range results {
		if r.Err != nil {
			t.Errorf("result[%d] unexpected error: %v", i, r.Err)
		}
		if r.Score != 8 {
			t.Errorf("result[%d] expected score 8, got %d", i, r.Score)
		}
		if r.Duration == 0 {
			t.Errorf("result[%d] duration should be non-zero", i)
		}
	}
}

func TestMultiReviewOrchestrator_RunAll_SingleFailure(t *testing.T) {
	origRunner := claudeRunnerFunc
	claudeRunnerFunc = func(ctx context.Context, prompt, model string) (string, error) {
		if model == "fail-model" {
			return "", fmt.Errorf("backend unavailable")
		}
		return "SCORE: 9\nFINDINGS:\nAll good", nil
	}
	defer func() { claudeRunnerFunc = origRunner }()

	o := &MultiReviewOrchestrator{
		PrimaryScore:    7,
		PrimaryFindings: "Primary findings",
		Configs: []analyzer.SecondaryReviewerConfig{
			{Backend: "claude", Model: "fail-model", FocusArea: "security"},
			{Backend: "claude", Model: "sonnet", FocusArea: "performance"},
		},
		Timeout: 30 * time.Second,
	}

	results := o.RunAll("diff content")

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// First should have an error
	if results[0].Err == nil {
		t.Error("expected error for fail-model, got nil")
	}

	// Second should succeed
	if results[1].Err != nil {
		t.Errorf("expected success for sonnet, got error: %v", results[1].Err)
	}
	if results[1].Score != 9 {
		t.Errorf("expected score 9 for sonnet, got %d", results[1].Score)
	}
}

func TestMultiReviewOrchestrator_RunAll_EmptyConfigs(t *testing.T) {
	o := &MultiReviewOrchestrator{
		PrimaryScore: 7,
		Configs:      nil,
	}

	results := o.RunAll("diff content")
	if results != nil {
		t.Errorf("expected nil results for empty configs, got %v", results)
	}
}

func TestMultiReviewOrchestrator_RunAll_ErrorFindings(t *testing.T) {
	origRunner := claudeRunnerFunc
	claudeRunnerFunc = func(ctx context.Context, prompt, model string) (string, error) {
		return "SCORE: 4\nFINDINGS:\n[ERROR] SQL injection vulnerability in query builder", nil
	}
	defer func() { claudeRunnerFunc = origRunner }()

	o := &MultiReviewOrchestrator{
		PrimaryScore:    8,
		PrimaryFindings: "Looks fine",
		Configs: []analyzer.SecondaryReviewerConfig{
			{Backend: "claude", Model: "opus", FocusArea: "security"},
		},
		Timeout: 30 * time.Second,
	}

	results := o.RunAll("diff content")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if !results[0].HasErrors {
		t.Error("expected HasErrors=true for [ERROR] findings")
	}
}

func TestMultiReviewOrchestrator_BuildConsensusReport(t *testing.T) {
	o := &MultiReviewOrchestrator{
		PrimaryScore:    7,
		PrimaryFindings: "Primary review looks good",
	}

	results := []ReviewResult{
		{
			Backend:  "claude/opus",
			Score:    8,
			Findings: "Security check passed",
			Duration: 2 * time.Second,
		},
		{
			Backend:  "claude/sonnet",
			Score:    6,
			Findings: "Performance could be improved",
			Duration: 1500 * time.Millisecond,
		},
	}

	score, body := o.BuildConsensusReport(results)

	// Consensus score with default weights: 0.7*7 + 0.15*8 + 0.15*6 = 4.9+1.2+0.9 = 7.0
	if score != 7 {
		t.Errorf("expected consensus score 7, got %d", score)
	}

	if body == "" {
		t.Error("expected non-empty report body")
	}

	// Check report contains key sections
	for _, want := range []string{
		"Multi-Model Review Consensus",
		"Primary Reviewer",
		"claude/opus",
		"claude/sonnet",
		"Consensus Score: 7/10",
	} {
		if !strContains(body, want) {
			t.Errorf("report body missing expected text: %q", want)
		}
	}
}

func TestMultiReviewOrchestrator_BuildConsensusReport_WithErrors(t *testing.T) {
	o := &MultiReviewOrchestrator{
		PrimaryScore:    8,
		PrimaryFindings: "Looks good",
	}

	results := []ReviewResult{
		{
			Backend:   "claude/opus",
			Score:     9,
			Findings:  "[ERROR] Critical vulnerability found",
			HasErrors: true,
			Duration:  1 * time.Second,
		},
	}

	score, body := o.BuildConsensusReport(results)

	// With error findings, score should be capped at ErrorFindingsCap (6)
	if score > ErrorFindingsCap {
		t.Errorf("expected score capped at %d with errors, got %d", ErrorFindingsCap, score)
	}

	if !strContains(body, "error-severity") {
		t.Error("report should mention error-severity issues")
	}
}

func TestMultiReviewOrchestrator_BuildConsensusReport_WithFailedBackend(t *testing.T) {
	o := &MultiReviewOrchestrator{
		PrimaryScore:    7,
		PrimaryFindings: "Primary findings",
	}

	results := []ReviewResult{
		{
			Backend:  "claude/opus",
			Score:    8,
			Findings: "Good",
			Duration: 1 * time.Second,
		},
		{
			Backend:  "claude/fail",
			Err:      fmt.Errorf("timeout"),
			Duration: 5 * time.Second,
		},
	}

	score, body := o.BuildConsensusReport(results)

	// Only one successful secondary, so: 0.7*7 + 0.3*8 = 4.9+2.4 = 7.3 -> 7
	if score != 7 {
		t.Errorf("expected consensus score 7, got %d", score)
	}

	if !strContains(body, "FAILED") {
		t.Error("report should indicate failed backend")
	}
	if !strContains(body, "timeout") {
		t.Error("report should include error message")
	}
}

func strContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
