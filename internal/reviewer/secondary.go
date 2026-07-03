package reviewer

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
)

// SecondaryReviewResult holds the output of a secondary reviewer.
type SecondaryReviewResult struct {
	FocusArea string
	Score     int
	Findings  string
	Error     error
}

// claudeRunnerFunc is a function variable for invoking the Claude CLI.
// Replaced in tests to avoid real LLM calls.
var claudeRunnerFunc = runClaudeCLI

// RunSecondaryReview invokes a secondary AI reviewer on the given diff.
// It follows the jittest pattern: call `claude --print --model <model> --max-turns 1`
// with a review-focused prompt, then parse the output for a score and findings.
func RunSecondaryReview(cfg analyzer.SecondaryReviewerConfig, diff string, timeout time.Duration) (score int, findings string, err error) {
	if diff == "" {
		return 0, "", fmt.Errorf("empty diff, nothing to review")
	}
	if cfg.Model == "" {
		return 0, "", fmt.Errorf("model is required for secondary review")
	}

	prompt := buildSecondaryReviewPrompt(cfg.FocusArea, diff)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	output, err := claudeRunnerFunc(ctx, prompt, cfg.Model)
	if err != nil {
		return 0, "", fmt.Errorf("secondary review failed (model=%s, focus=%s): %w", cfg.Model, cfg.FocusArea, err)
	}

	score, findings, err = parseSecondaryReviewOutput(output)
	if err != nil {
		return 0, "", fmt.Errorf("secondary review unparseable (model=%s, focus=%s): %w", cfg.Model, cfg.FocusArea, err)
	}
	return score, findings, nil
}

// buildSecondaryReviewPrompt constructs a focused review prompt for the secondary reviewer.
func buildSecondaryReviewPrompt(focusArea, diff string) string {
	focusInstruction := "general code quality"
	if focusArea != "" {
		focusInstruction = focusArea
	}

	return fmt.Sprintf(`You are a code reviewer focused on: %s

Review the following diff and provide:
1. A score from 1-10 (where 10 is perfect)
2. Key findings related to your focus area

Format your response EXACTLY as:
SCORE: <number>
FINDINGS:
<your findings here>

If you find any severity=error issues (critical bugs, security vulnerabilities, data loss risks),
prefix the finding with [ERROR].

Diff:
%s`, focusInstruction, diff)
}

// parseSecondaryReviewOutput extracts score and findings from the LLM output.
// A missing or out-of-range SCORE line is an error, not a default: a garbled
// reviewer must be excluded from consensus rather than contribute a
// fabricated middle score.
func parseSecondaryReviewOutput(output string) (int, string, error) {
	scoreRe := regexp.MustCompile(`(?im)^SCORE:\s*(\d+)`)
	m := scoreRe.FindStringSubmatch(output)
	if len(m) < 2 {
		return 0, "", fmt.Errorf("no SCORE line in reviewer output")
	}
	score, err := strconv.Atoi(m[1])
	if err != nil || score < 1 || score > 10 {
		return 0, "", fmt.Errorf("SCORE %q out of range 1-10", m[1])
	}

	// Extract findings
	findings := ""
	findingsRe := regexp.MustCompile(`(?ims)^FINDINGS:\s*\n(.+)`)
	if fm := findingsRe.FindStringSubmatch(output); len(fm) > 1 {
		findings = strings.TrimSpace(fm[1])
	} else {
		// Fallback: use the whole output minus the score line as findings
		findings = strings.TrimSpace(scoreRe.ReplaceAllString(output, ""))
	}

	return score, findings, nil
}

// HasErrorFindings returns true if the findings contain any [ERROR] tagged items.
func HasErrorFindings(findings string) bool {
	return strings.Contains(findings, "[ERROR]")
}

// runClaudeCLI invokes `claude --print --model <model> --max-turns 1` with the prompt via stdin.
func runClaudeCLI(ctx context.Context, prompt, model string) (string, error) {
	if _, err := exec.LookPath("claude"); err != nil {
		return "", fmt.Errorf("claude CLI not found in PATH")
	}

	args := []string{
		"--print",
		"--model", model,
		"--max-turns", "1",
	}

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Stdin = strings.NewReader(prompt)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("claude CLI timed out")
		}
		return "", fmt.Errorf("claude exited with error: %w (stderr: %s)", err, stderr.String())
	}

	return stdout.String(), nil
}
