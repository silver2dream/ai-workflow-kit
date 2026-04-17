package worker

import (
	"strings"
	"testing"
)

func TestFormatPRReviewComments_Empty(t *testing.T) {
	result := formatPRReviewComments("")
	if result != "" {
		t.Errorf("expected empty string for empty input, got %q", result)
	}
}

func TestFormatPRReviewComments_WhitespaceOnly(t *testing.T) {
	result := formatPRReviewComments("   \n\n  ")
	if result != "" {
		t.Errorf("expected empty string for whitespace-only input, got %q", result)
	}
}

func TestFormatPRReviewComments_SingleComment(t *testing.T) {
	input := "internal/worker/runner.go\t145\tMissing error check"
	result := formatPRReviewComments(input)

	expected := "- **internal/worker/runner.go:145** → Missing error check\n"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestFormatPRReviewComments_MultipleComments(t *testing.T) {
	input := "internal/worker/runner.go\t145\tMissing error check\n" +
		"internal/worker/github.go\t30\tUse context timeout\n" +
		"cmd/awkit/main.go\t0\tRemove unused import"
	result := formatPRReviewComments(input)

	if !strings.Contains(result, "**internal/worker/runner.go:145**") {
		t.Error("expected first comment to be present")
	}
	if !strings.Contains(result, "**internal/worker/github.go:30**") {
		t.Error("expected second comment to be present")
	}
	if !strings.Contains(result, "**cmd/awkit/main.go:0**") {
		t.Error("expected third comment to be present")
	}

	lines := strings.Split(strings.TrimSpace(result), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lines))
	}
}

func TestFormatPRReviewComments_TruncatesLongBody(t *testing.T) {
	longBody := strings.Repeat("x", 300)
	input := "file.go\t10\t" + longBody
	result := formatPRReviewComments(input)

	if !strings.Contains(result, "...") {
		t.Error("expected truncation indicator '...'")
	}
	// Body should be truncated to 200 chars + "..."
	if strings.Contains(result, strings.Repeat("x", 201)) {
		t.Error("body should be truncated to 200 characters")
	}
}

func TestFormatPRReviewComments_CapsAt4000Chars(t *testing.T) {
	var builder strings.Builder
	for i := 0; i < 200; i++ {
		builder.WriteString("internal/worker/runner.go\t100\tSome review comment text here\n")
	}
	result := formatPRReviewComments(builder.String())

	if len(result) > 4000 {
		t.Errorf("expected output capped at 4000 chars, got %d", len(result))
	}
	if result == "" {
		t.Error("expected non-empty output")
	}
}

func TestFormatPRReviewComments_SkipsMalformedLines(t *testing.T) {
	input := "not-enough-tabs\n" +
		"internal/worker/runner.go\t145\tValid comment\n" +
		"also-bad\tbut-only-two"
	result := formatPRReviewComments(input)

	lines := strings.Split(strings.TrimSpace(result), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 valid line, got %d: %q", len(lines), result)
	}
	if !strings.Contains(result, "runner.go:145") {
		t.Error("expected valid comment to be present")
	}
}

func TestFormatPRReviewComments_SkipsEmptyPathOrBody(t *testing.T) {
	input := "\t145\tno path\n" +
		"file.go\t10\t\n" +
		"file.go\t20\tvalid body"
	result := formatPRReviewComments(input)

	lines := strings.Split(strings.TrimSpace(result), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 valid line, got %d: %q", len(lines), result)
	}
	if !strings.Contains(result, "file.go:20") {
		t.Error("expected valid comment to be present")
	}
}

func TestFormatPRReviewComments_CollapsesNewlinesInBody(t *testing.T) {
	input := "file.go\t10\tLine one\nstill part of body but new line"
	// Since split is on \n, the second part becomes a separate (malformed) line.
	// This tests that each line is processed independently.
	result := formatPRReviewComments(input)
	if !strings.Contains(result, "file.go:10") {
		t.Error("expected first line to be processed")
	}
}

func TestFormatPRReviewComments_MultilineBodyFromAPI(t *testing.T) {
	// When the API returns a body with literal \n in a single tab-delimited field,
	// the body gets split by Fields() to collapse whitespace
	input := "file.go\t10\tFirst line   second line   third line"
	result := formatPRReviewComments(input)

	if !strings.Contains(result, "First line second line third line") {
		t.Errorf("expected collapsed whitespace in body, got %q", result)
	}
}
