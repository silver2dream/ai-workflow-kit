package reviewer

import (
	"fmt"
	"strings"
)

// Severity levels recognized in review "Suggested Improvements" bullets.
// The pr-reviewer agent contract requires each suggestion line to start
// with one of these prefixes (e.g. "- **Critical:** auth.go:42 — ...").
const (
	severityCritical  = "critical"
	severityImportant = "important"
	severityNit       = "nit"
	severityOptional  = "optional"
	severityConsider  = "consider"
	severityFYI       = "fyi"
)

// SeverityCounts holds the number of severity-prefixed findings in a review body.
type SeverityCounts struct {
	Critical  int
	Important int
	Nit       int
	Optional  int // includes Consider:
	FYI       int
}

// Total returns the total number of severity-tagged findings.
func (s SeverityCounts) Total() int {
	return s.Critical + s.Important + s.Nit + s.Optional + s.FYI
}

// ParseSeverityCounts scans a review body for list items that start with a
// severity prefix. Both bold ("**Critical:**") and plain ("Critical:") forms
// are accepted; matching is case-insensitive. Only list items are counted so
// that prose and the severity legend table are ignored.
func ParseSeverityCounts(reviewBody string) SeverityCounts {
	var counts SeverityCounts
	for _, line := range strings.Split(reviewBody, "\n") {
		switch sev, ok := severityOfLine(line); {
		case !ok:
		case sev == severityCritical:
			counts.Critical++
		case sev == severityImportant:
			counts.Important++
		case sev == severityNit:
			counts.Nit++
		case sev == severityOptional, sev == severityConsider:
			counts.Optional++
		case sev == severityFYI:
			counts.FYI++
		}
	}
	return counts
}

// severityOfLine reports the severity prefix of a markdown list item, if any.
func severityOfLine(line string) (string, bool) {
	s := strings.TrimSpace(line)

	// Require a list marker so plain prose and table rows are not counted.
	marker := false
	for _, prefix := range []string{"- ", "* ", "+ "} {
		if strings.HasPrefix(s, prefix) {
			s = strings.TrimSpace(s[len(prefix):])
			marker = true
			break
		}
	}
	if !marker {
		// Tolerate numbered lists ("1. " / "2) ").
		i := 0
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i > 0 && i < len(s) && (s[i] == '.' || s[i] == ')') {
			s = strings.TrimSpace(s[i+1:])
			marker = true
		}
	}
	if !marker {
		return "", false
	}

	// Strip emphasis markers before the prefix ("**Critical:**", "`Nit:`").
	s = strings.TrimLeft(s, "*_`")
	lower := strings.ToLower(s)
	for _, sev := range []string{severityCritical, severityImportant, severityNit, severityOptional, severityConsider, severityFYI} {
		rest, ok := strings.CutPrefix(lower, sev)
		if !ok {
			continue
		}
		// Allow closing emphasis between the word and the colon, and the
		// colon inside the bold form ("**Critical:**" → rest = ":**").
		rest = strings.TrimLeft(rest, "*_` ")
		if strings.HasPrefix(rest, ":") || strings.HasPrefix(rest, "：") {
			return sev, true
		}
	}
	return "", false
}

// ValidateSeverityConsistency cross-checks the numeric score against the
// severity-tagged findings in the review body. It is the system-enforced
// counterpart of the pr-reviewer prompt contract:
//   - a failing score (score < threshold → changes_requested) must be
//     justified by at least one Critical or Important finding;
//   - a passing score (approve) must not carry unresolved Critical findings.
//
// Returns nil when consistent.
func ValidateSeverityConsistency(score, threshold int, reviewBody string) *EvidenceError {
	return ValidateSeverityCounts(score, threshold, ParseSeverityCounts(reviewBody))
}

// ValidateSeverityCounts is the counts-based core of the severity gate,
// used directly by the structured submission path (no prose parsing).
func ValidateSeverityCounts(score, threshold int, counts SeverityCounts) *EvidenceError {
	if score < threshold && counts.Critical == 0 && counts.Important == 0 {
		return &EvidenceError{
			Code: 4,
			Message: fmt.Sprintf(
				"inconsistent review: score %d is below threshold %d (changes_requested) but no Critical/Important finding was listed",
				score, threshold),
			Details: []string{
				"every changes_requested review must contain at least one suggestion line starting with **Critical:** or **Important:**",
				fmt.Sprintf("severity-tagged findings found: %d (critical=0, important=0)", counts.Total()),
			},
		}
	}

	if score >= threshold && counts.Critical > 0 {
		return &EvidenceError{
			Code: 4,
			Message: fmt.Sprintf(
				"inconsistent review: score %d meets threshold %d (approve) but %d Critical finding(s) remain listed",
				score, threshold, counts.Critical),
			Details: []string{
				"Critical findings block merge; score below the threshold, or downgrade the finding if it is actually resolved",
			},
		}
	}

	return nil
}
