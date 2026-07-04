package reviewer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// StructuredReview is the agent-friendly review submission contract
// (ACI: the reviewer writes structured data; humans get markdown RENDERED
// from it — nothing is regex-parsed back out of prose).
//
// Submitted as JSON via `awkit submit-review --body-file review.json`.
type StructuredReview struct {
	Score   int    `json:"score"`             // 1-10
	Summary string `json:"summary,omitempty"` // score reason / overall assessment

	Criteria []StructuredCriterion `json:"criteria"`

	Improvements []StructuredImprovement `json:"improvements,omitempty"`

	Risks string `json:"risks,omitempty"`
}

// StructuredCriterion maps one acceptance criterion to its evidence.
type StructuredCriterion struct {
	Criterion      string `json:"criterion"`                // acceptance criterion text (verbatim from ticket)
	Implementation string `json:"implementation,omitempty"` // where/how implemented (function, file:line, behavior)
	TestName       string `json:"test_name,omitempty"`      // exact test function name from test output
	Assertion      string `json:"assertion,omitempty"`      // key assertion copied verbatim from the test file
	Meta           bool   `json:"meta,omitempty"`           // true for meta-criteria ("all tests pass")
}

// StructuredImprovement is one severity-tagged finding.
type StructuredImprovement struct {
	Severity string `json:"severity"`           // critical | important | nit | optional | fyi
	Location string `json:"location,omitempty"` // file:line
	Text     string `json:"text"`
}

// ValidationError describes one schema problem in an agent-actionable way:
// which field, what is wrong, and how to fix it.
type ValidationError struct {
	Field   string `json:"field"`
	Problem string `json:"problem"`
	Hint    string `json:"hint,omitempty"`
}

func (e ValidationError) String() string {
	s := e.Field + ": " + e.Problem
	if e.Hint != "" {
		s += " — " + e.Hint
	}
	return s
}

// validSeverities for improvements.
var validSeverities = map[string]bool{
	severityCritical: true, severityImportant: true, severityNit: true,
	severityOptional: true, severityConsider: true, severityFYI: true,
}

// ParseStructuredReview decodes and validates a structured review
// submission. Unknown fields are rejected (catches typos like "tests_name"
// at the interface instead of silently dropping evidence). Returns the
// parsed review or a list of actionable validation errors — never both.
func ParseStructuredReview(data []byte) (*StructuredReview, []ValidationError) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var r StructuredReview
	if err := dec.Decode(&r); err != nil {
		return nil, []ValidationError{{
			Field:   "(document)",
			Problem: fmt.Sprintf("invalid JSON: %v", err),
			Hint:    "the file must be a single JSON object; check for trailing commas, unquoted keys, or misspelled field names",
		}}
	}
	// Reject trailing content after the object.
	if dec.More() {
		return nil, []ValidationError{{
			Field:   "(document)",
			Problem: "trailing content after the JSON object",
			Hint:    "the file must contain exactly one JSON object",
		}}
	}

	var errs []ValidationError
	if r.Score < 1 || r.Score > 10 {
		errs = append(errs, ValidationError{
			Field:   "score",
			Problem: fmt.Sprintf("must be 1-10, got %d", r.Score),
		})
	}
	if len(r.Criteria) == 0 {
		errs = append(errs, ValidationError{
			Field:   "criteria",
			Problem: "must contain one entry per acceptance criterion in the ticket",
			Hint:    "copy each ticket criterion verbatim into criteria[].criterion",
		})
	}
	for i, c := range r.Criteria {
		prefix := fmt.Sprintf("criteria[%d]", i)
		if strings.TrimSpace(c.Criterion) == "" {
			errs = append(errs, ValidationError{
				Field:   prefix + ".criterion",
				Problem: "empty",
				Hint:    "copy the acceptance criterion text verbatim from the ticket",
			})
		}
		if c.Meta {
			continue // meta-criteria are verified by the overall test pass
		}
		if len(strings.TrimSpace(c.Implementation)) < minImplementationChars {
			errs = append(errs, ValidationError{
				Field:   prefix + ".implementation",
				Problem: fmt.Sprintf("must be at least %d characters describing the actual implementation", minImplementationChars),
				Hint:    "name the function and file:line, e.g. \"HandleCollision() at engine.go:145 sets game.State = GameOver\"",
			})
		}
		if strings.TrimSpace(c.TestName) == "" {
			errs = append(errs, ValidationError{
				Field:   prefix + ".test_name",
				Problem: "empty",
				Hint:    "the exact test function name that verifies this criterion (or set \"meta\": true for criteria like \"all tests pass\")",
			})
		}
		if strings.TrimSpace(c.Assertion) == "" {
			errs = append(errs, ValidationError{
				Field:   prefix + ".assertion",
				Problem: "empty",
				Hint:    "copy the key assertion line verbatim from the test file",
			})
		}
	}
	for i, imp := range r.Improvements {
		prefix := fmt.Sprintf("improvements[%d]", i)
		if !validSeverities[strings.ToLower(strings.TrimSpace(imp.Severity))] {
			errs = append(errs, ValidationError{
				Field:   prefix + ".severity",
				Problem: fmt.Sprintf("unknown severity %q", imp.Severity),
				Hint:    "one of: critical, important, nit, optional, fyi",
			})
		}
		if strings.TrimSpace(imp.Text) == "" {
			errs = append(errs, ValidationError{
				Field:   prefix + ".text",
				Problem: "empty",
			})
		}
	}

	if len(errs) > 0 {
		return nil, errs
	}
	return &r, nil
}

// minImplementationChars mirrors ValidateCompleteness's minimum.
const minImplementationChars = 20

// ToVerifications converts the structured criteria directly into the
// verification entries the evidence gate consumes — no markdown parsing,
// no fuzzy recovery needed.
func (r *StructuredReview) ToVerifications() []CriteriaVerification {
	out := make([]CriteriaVerification, 0, len(r.Criteria))
	for _, c := range r.Criteria {
		out = append(out, CriteriaVerification{
			Criteria:       strings.TrimSpace(c.Criterion),
			Implementation: strings.TrimSpace(c.Implementation),
			TestName:       strings.TrimSpace(c.TestName),
			Assertion:      strings.TrimSpace(c.Assertion),
			IsMeta:         c.Meta,
		})
	}
	return out
}

// SeverityCounts tallies improvements directly — the structured counterpart
// of ParseSeverityCounts, with no bullet-prefix parsing.
func (r *StructuredReview) SeverityCounts() SeverityCounts {
	var counts SeverityCounts
	for _, imp := range r.Improvements {
		switch strings.ToLower(strings.TrimSpace(imp.Severity)) {
		case severityCritical:
			counts.Critical++
		case severityImportant:
			counts.Important++
		case severityNit:
			counts.Nit++
		case severityOptional, severityConsider:
			counts.Optional++
		case severityFYI:
			counts.FYI++
		}
	}
	return counts
}

// severityDisplay maps lowercase severities to their display prefix.
var severityDisplay = map[string]string{
	severityCritical:  "Critical",
	severityImportant: "Important",
	severityNit:       "Nit",
	severityOptional:  "Optional",
	severityConsider:  "Consider",
	severityFYI:       "FYI",
}

// RenderMarkdown produces the human-readable review body from the
// structured data, matching the established review format so PR comments,
// feedback categorization, and humans all see the familiar shape.
func (r *StructuredReview) RenderMarkdown() string {
	var b strings.Builder

	b.WriteString("### Implementation Review\n\n")
	for i, c := range r.Criteria {
		b.WriteString(fmt.Sprintf("#### %d. %s\n", i+1, strings.TrimSpace(c.Criterion)))
		impl := strings.TrimSpace(c.Implementation)
		if c.Meta && impl == "" {
			impl = "(meta) verified by the overall test pass"
		}
		b.WriteString(fmt.Sprintf("**Implementation**: %s\n\n", impl))
	}

	b.WriteString("### Test Review\n\n")
	b.WriteString("| Criteria | Test | Key Assertion |\n")
	b.WriteString("|----------|------|---------------|\n")
	for _, c := range r.Criteria {
		test := strings.TrimSpace(c.TestName)
		assertion := strings.TrimSpace(c.Assertion)
		if c.Meta {
			if test == "" {
				test = "(meta)"
			}
			if assertion == "" {
				assertion = "overall test pass"
			}
		}
		b.WriteString(fmt.Sprintf("| %s | %s | %s |\n",
			tableCell(c.Criterion), tableCell(test), tableCell(assertion)))
	}
	b.WriteString("\n")

	if r.Summary != "" {
		b.WriteString("### Score Reason\n\n")
		b.WriteString(strings.TrimSpace(r.Summary))
		b.WriteString("\n\n")
	}

	b.WriteString("### Suggested Improvements\n\n")
	if len(r.Improvements) == 0 {
		b.WriteString("None\n")
	} else {
		for _, imp := range r.Improvements {
			display := severityDisplay[strings.ToLower(strings.TrimSpace(imp.Severity))]
			line := "- **" + display + ":**"
			if loc := strings.TrimSpace(imp.Location); loc != "" {
				line += " `" + loc + "` —"
			}
			line += " " + strings.TrimSpace(imp.Text)
			b.WriteString(line + "\n")
		}
	}
	b.WriteString("\n")

	if r.Risks != "" {
		b.WriteString("### Potential Risks\n\n")
		b.WriteString(strings.TrimSpace(r.Risks))
		b.WriteString("\n")
	}

	return b.String()
}

// tableCell makes a string safe for a markdown table cell.
func tableCell(s string) string {
	s = strings.Join(strings.Fields(s), " ") // collapse newlines
	return strings.ReplaceAll(s, "|", "\\|")
}

// SubmissionContract renders the JSON schema skeleton and submission
// instructions shown to the reviewer by prepare-review (ACI: the interface
// states its own contract instead of relying on prompt memory).
func SubmissionContract(prNumber, issueNumber int) string {
	return fmt.Sprintf(`Write your review as JSON to .ai/state/reviews/pr-%d/review.json:

{
  "score": <1-10>,
  "summary": "<why this score>",
  "criteria": [
    {
      "criterion": "<acceptance criterion copied verbatim from the ticket>",
      "implementation": "<function + file:line + behavior, >=20 chars>",
      "test_name": "<exact test function name>",
      "assertion": "<key assertion copied verbatim from the test file>"
    },
    { "criterion": "All tests pass", "meta": true }
  ],
  "improvements": [
    { "severity": "critical|important|nit|optional|fyi", "location": "file.go:42", "text": "<finding>" }
  ],
  "risks": "<or omit>"
}

Then submit:
  awkit submit-review --pr %d --issue %d --ci-status <passed|failed> --body-file .ai/state/reviews/pr-%d/review.json

If the command exits with SUBMISSION INVALID, fix the listed fields in the
JSON and resubmit IN THIS SESSION (format errors are yours to correct now).
If the result is review_blocked, STOP — a new session will re-review.`,
		prNumber, prNumber, issueNumber, prNumber)
}
