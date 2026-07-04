package reviewer

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
)

func TestParseSecondaryReviewOutput_ValidOutput(t *testing.T) {
	output := `SCORE: 8
FINDINGS:
The architecture follows clean separation of concerns.
No critical issues found.`

	score, findings, err := parseSecondaryReviewOutput(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score != 8 {
		t.Errorf("expected score 8, got %d", score)
	}
	if findings == "" {
		t.Error("expected non-empty findings")
	}
	if !strings.Contains(findings, "clean separation") {
		t.Errorf("expected findings to contain 'clean separation', got: %s", findings)
	}
}

func TestParseSecondaryReviewOutput_ScoreOutOfRange(t *testing.T) {
	output := "SCORE: 15\nFINDINGS:\nSome findings"
	_, _, err := parseSecondaryReviewOutput(output)
	if err == nil {
		t.Error("expected error for out-of-range score (fail-closed, no default)")
	}
}

func TestParseSecondaryReviewOutput_NoScore(t *testing.T) {
	output := "No structured output here, just text."
	_, _, err := parseSecondaryReviewOutput(output)
	if err == nil {
		t.Error("expected error when SCORE line is missing (fail-closed, no default)")
	}
}

func TestParseSecondaryReviewOutput_ErrorFindings(t *testing.T) {
	output := `SCORE: 4
FINDINGS:
[ERROR] SQL injection vulnerability in user input handling.
Minor style issues in naming conventions.`

	score, findings, err := parseSecondaryReviewOutput(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score != 4 {
		t.Errorf("expected score 4, got %d", score)
	}
	if !HasErrorFindings(findings) {
		t.Error("expected HasErrorFindings to return true")
	}
}

func TestHasErrorFindings(t *testing.T) {
	tests := []struct {
		findings string
		want     bool
	}{
		{"[ERROR] critical bug found", true},
		{"Minor issues only", false},
		{"", false},
		{"Some text [ERROR] in the middle", true},
	}

	for _, tt := range tests {
		got := HasErrorFindings(tt.findings)
		if got != tt.want {
			t.Errorf("HasErrorFindings(%q) = %v, want %v", tt.findings, got, tt.want)
		}
	}
}

func TestRunSecondaryReview_EmptyDiff(t *testing.T) {
	cfg := analyzer.SecondaryReviewerConfig{Model: "sonnet"}
	_, _, err := RunSecondaryReview(cfg, "", 30*time.Second)
	if err == nil {
		t.Error("expected error for empty diff")
	}
}

func TestRunSecondaryReview_EmptyModel(t *testing.T) {
	cfg := analyzer.SecondaryReviewerConfig{Backend: "claude"}
	_, _, err := RunSecondaryReview(cfg, "some diff", 30*time.Second)
	if err == nil {
		t.Error("expected error for empty model")
	}
}

func TestRunSecondaryReview_MockedCLI(t *testing.T) {
	// Replace the claudeRunner with a mock
	origRunner := claudeRunnerFunc
	defer func() { claudeRunnerFunc = origRunner }()

	claudeRunnerFunc = func(ctx context.Context, prompt, model string) (string, error) {
		return "SCORE: 7\nFINDINGS:\nLooks good overall. Minor naming issues.", nil
	}

	cfg := analyzer.SecondaryReviewerConfig{
		Backend:   "claude",
		Model:     "sonnet",
		FocusArea: "architecture",
	}
	score, findings, err := RunSecondaryReview(cfg, "diff --git a/file.go b/file.go\n+some code", 30*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score != 7 {
		t.Errorf("expected score 7, got %d", score)
	}
	if findings == "" {
		t.Error("expected non-empty findings")
	}
}

func TestRunSecondaryReview_CLIError(t *testing.T) {
	origRunner := claudeRunnerFunc
	defer func() { claudeRunnerFunc = origRunner }()

	claudeRunnerFunc = func(ctx context.Context, prompt, model string) (string, error) {
		return "", fmt.Errorf("connection refused")
	}

	cfg := analyzer.SecondaryReviewerConfig{
		Backend: "claude",
		Model:   "sonnet",
	}
	_, _, err := RunSecondaryReview(cfg, "some diff", 30*time.Second)
	if err == nil {
		t.Error("expected error from CLI failure")
	}
}

func TestBuildSecondaryReviewPrompt(t *testing.T) {
	prompt := buildSecondaryReviewPrompt("security", "diff content here")
	if !strings.Contains(prompt, "security") {
		t.Error("expected prompt to contain focus area")
	}
	if !strings.Contains(prompt, "SCORE:") {
		t.Error("expected prompt to contain SCORE format instruction")
	}
	if !strings.Contains(prompt, "diff content here") {
		t.Error("expected prompt to contain diff")
	}
}

func TestBuildSecondaryReviewPrompt_EmptyFocusArea(t *testing.T) {
	prompt := buildSecondaryReviewPrompt("", "diff content")
	if !strings.Contains(prompt, "general code quality") {
		t.Error("expected default focus area in prompt")
	}
}
