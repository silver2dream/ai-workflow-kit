package jittest

import (
	"strings"
	"testing"
)

func TestResult_FormatComment_AllPassed(t *testing.T) {
	r := &Result{
		Generated: 3,
		Passed:    3,
		Failed:    0,
		Skipped:   0,
	}
	comment := r.FormatComment()
	if !strings.Contains(comment, "Generated: 3") {
		t.Error("comment should contain generated count")
	}
	if !strings.Contains(comment, "Passed: 3") {
		t.Error("comment should contain passed count")
	}
	if strings.Contains(comment, "**Failed:**") {
		t.Error("comment should not contain Failed section when none failed")
	}
}

func TestResult_FormatComment_WithFailures(t *testing.T) {
	r := &Result{
		Generated: 3,
		Passed:    2,
		Failed:    1,
		Tests: []TestOutcome{
			{Test: GeneratedTest{Filename: "a_jittest_test.go"}, Passed: true},
			{Test: GeneratedTest{Filename: "b_jittest_test.go"}, Passed: true},
			{Test: GeneratedTest{Filename: "c_jittest_test.go"}, Passed: false, Error: "expected error, got nil"},
		},
	}
	comment := r.FormatComment()
	if !strings.Contains(comment, "**Failed:**") {
		t.Error("comment should contain Failed section")
	}
	if !strings.Contains(comment, "c_jittest_test.go") {
		t.Error("comment should list failed test filename")
	}
	if !strings.Contains(comment, "expected error, got nil") {
		t.Error("comment should include error message")
	}
}

func TestResult_FormatComment_WithError(t *testing.T) {
	r := &Result{
		Error: "LLM timeout",
	}
	comment := r.FormatComment()
	if !strings.Contains(comment, "**Error:** LLM timeout") {
		t.Error("comment should contain error message")
	}
}

func TestResult_FormatComment_ContainsDisclaimer(t *testing.T) {
	r := &Result{}
	comment := r.FormatComment()
	if !strings.Contains(comment, "do not enter the codebase") {
		t.Error("comment should contain disclaimer about JiT tests being discarded")
	}
}

func TestResult_FormatFeedback_NoFailures(t *testing.T) {
	r := &Result{Failed: 0}
	if r.FormatFeedback() != "" {
		t.Error("FormatFeedback should return empty string when no failures")
	}
}

func TestResult_FormatFeedback_WithFailures(t *testing.T) {
	r := &Result{
		Failed: 1,
		Tests: []TestOutcome{
			{Test: GeneratedTest{Filename: "x_jittest_test.go"}, Passed: false, Error: "nil pointer"},
		},
	}
	feedback := r.FormatFeedback()
	if !strings.Contains(feedback, "Previous JiT test failures") {
		t.Error("feedback should contain header")
	}
	if !strings.Contains(feedback, "x_jittest_test.go: nil pointer") {
		t.Error("feedback should contain test name and error")
	}
}

func TestResult_IsBlocking(t *testing.T) {
	tests := []struct {
		name   string
		failed int
		policy string
		want   bool
	}{
		{"warn with failures", 1, "warn", false},
		{"block with failures", 1, "block", true},
		{"block with no failures", 0, "block", false},
		{"empty policy with failures", 1, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Result{Failed: tt.failed}
			if got := r.IsBlocking(tt.policy); got != tt.want {
				t.Errorf("IsBlocking(%q) = %v, want %v", tt.policy, got, tt.want)
			}
		})
	}
}
