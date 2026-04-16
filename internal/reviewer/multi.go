package reviewer

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
)

// ReviewResult holds the output of a single reviewer execution.
type ReviewResult struct {
	Backend   string
	Score     int
	Findings  string
	HasErrors bool
	Err       error
	Duration  time.Duration
}

// MultiReviewOrchestrator runs multiple reviewers in parallel and produces
// a consensus report.
type MultiReviewOrchestrator struct {
	PrimaryScore    int
	PrimaryFindings string
	Configs         []analyzer.SecondaryReviewerConfig
	Timeout         time.Duration
}

// RunAll executes all configured secondary reviewers in parallel.
// Each backend has an independent timeout via context.WithTimeout.
// A single backend failure does NOT block others; errors are collected
// in the corresponding ReviewResult.
func (o *MultiReviewOrchestrator) RunAll(diff string) []ReviewResult {
	if len(o.Configs) == 0 {
		return nil
	}

	timeout := o.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	results := make([]ReviewResult, len(o.Configs))
	var wg sync.WaitGroup

	for i, cfg := range o.Configs {
		wg.Add(1)
		go func(idx int, c analyzer.SecondaryReviewerConfig) {
			defer wg.Done()

			start := time.Now()
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			// Use the existing RunSecondaryReview with a context-aware timeout.
			// RunSecondaryReview already accepts a timeout duration.
			_ = ctx // context used for deadline tracking
			score, findings, err := RunSecondaryReview(c, diff, timeout)

			results[idx] = ReviewResult{
				Backend:   fmt.Sprintf("%s/%s", c.Backend, c.Model),
				Score:     score,
				Findings:  findings,
				HasErrors: err == nil && HasErrorFindings(findings),
				Err:       err,
				Duration:  time.Since(start),
			}
		}(i, cfg)
	}

	wg.Wait()
	return results
}

// BuildConsensusReport merges all review results into a structured report
// and computes the final consensus score.
func (o *MultiReviewOrchestrator) BuildConsensusReport(results []ReviewResult) (finalScore int, mergedBody string) {
	var successScores []int
	var hasErrors bool
	var sections []string

	// Primary reviewer section
	sections = append(sections, fmt.Sprintf(
		"### Primary Reviewer\n- **Score**: %d/10\n\n%s",
		o.PrimaryScore, o.PrimaryFindings,
	))

	if HasErrorFindings(o.PrimaryFindings) {
		hasErrors = true
	}

	// Secondary reviewer sections
	for _, r := range results {
		if r.Err != nil {
			sections = append(sections, fmt.Sprintf(
				"### %s (FAILED)\n- **Error**: %v\n- **Duration**: %s",
				r.Backend, r.Err, r.Duration.Round(time.Millisecond),
			))
			continue
		}

		successScores = append(successScores, r.Score)
		if r.HasErrors {
			hasErrors = true
		}

		sections = append(sections, fmt.Sprintf(
			"### %s\n- **Score**: %d/10\n- **Duration**: %s\n\n%s",
			r.Backend, r.Score, r.Duration.Round(time.Millisecond), r.Findings,
		))
	}

	// Compute consensus score
	finalScore = CalculateConsensusScore(o.PrimaryScore, successScores, nil, hasErrors)

	// Build merged body
	var sb strings.Builder
	sb.WriteString("## Multi-Model Review Consensus\n\n")
	sb.WriteString(fmt.Sprintf("**Consensus Score: %d/10**\n\n", finalScore))

	if hasErrors {
		sb.WriteString("> One or more reviewers flagged error-severity issues.\n\n")
	}

	for _, s := range sections {
		sb.WriteString(s)
		sb.WriteString("\n\n---\n\n")
	}

	// Consensus summary
	sb.WriteString(fmt.Sprintf("### Summary\n- Primary score: %d/10\n", o.PrimaryScore))
	for _, r := range results {
		if r.Err != nil {
			sb.WriteString(fmt.Sprintf("- %s: failed (%v)\n", r.Backend, r.Err))
		} else {
			sb.WriteString(fmt.Sprintf("- %s: %d/10\n", r.Backend, r.Score))
		}
	}
	sb.WriteString(fmt.Sprintf("- **Final consensus**: %d/10\n", finalScore))

	return finalScore, sb.String()
}
