package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeReviewFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "review.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// Validation runs before any side effects, so invalid submissions are safe
// to exercise end-to-end through the command entry point.

func TestSubmitReview_BodyFileInvalidJSONExits2(t *testing.T) {
	path := writeReviewFile(t, "{not json")
	code := cmdSubmitReview([]string{"--pr", "1", "--issue", "2", "--ci-status", "passed", "--body-file", path})
	if code != 2 {
		t.Errorf("exit code = %d, want 2 (in-session correction)", code)
	}
}

func TestSubmitReview_BodyFileFieldErrorsExit2(t *testing.T) {
	path := writeReviewFile(t, `{"score": 8, "criteria": []}`)
	code := cmdSubmitReview([]string{"--pr", "1", "--issue", "2", "--ci-status", "passed", "--body-file", path})
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

func TestSubmitReview_BodyFileMissingExits2(t *testing.T) {
	code := cmdSubmitReview([]string{"--pr", "1", "--issue", "2", "--ci-status", "passed", "--body-file", filepath.Join(t.TempDir(), "nope.json")})
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

func TestSubmitReview_BodyFileScoreConflictExits2(t *testing.T) {
	path := writeReviewFile(t, `{"score": 8, "criteria": [{"criterion": "All tests pass", "meta": true}]}`)
	code := cmdSubmitReview([]string{"--pr", "1", "--issue", "2", "--score", "5", "--ci-status", "passed", "--body-file", path})
	if code != 2 {
		t.Errorf("exit code = %d, want 2 for score conflict", code)
	}
}
