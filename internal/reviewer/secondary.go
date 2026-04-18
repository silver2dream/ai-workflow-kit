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

	score, findings = parseSecondaryReviewOutput(output)
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
func parseSecondaryReviewOutput(output string) (int, string) {
	score := 5 // default if parsing fails

	// Extract score
	scoreRe := regexp.MustCompile(`(?im)^SCORE:\s*(\d+)`)
	if m := scoreRe.FindStringSubmatch(output); len(m) > 1 {
		if parsed, err := strconv.Atoi(m[1]); err == nil && parsed >= 1 && parsed <= 10 {
			score = parsed
		}
	}

	// Extract findings
	findings := ""
	findingsRe := regexp.MustCompile(`(?ims)^FINDINGS:\s*\n(.+)`)
	if m := findingsRe.FindStringSubmatch(output); len(m) > 1 {
		findings = strings.TrimSpace(m[1])
	} else {
		// Fallback: use the whole output minus the score line as findings
		findings = strings.TrimSpace(scoreRe.ReplaceAllString(output, ""))
	}

	return score, findings
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
