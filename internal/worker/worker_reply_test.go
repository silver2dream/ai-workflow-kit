package worker

import (
	"testing"
)

func TestParseCommentIDs(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect []int
	}{
		{
			name:   "single id",
			input:  "12345\n",
			expect: []int{12345},
		},
		{
			name:   "multiple ids",
			input:  "111\n222\n333\n",
			expect: []int{111, 222, 333},
		},
		{
			name:   "empty input",
			input:  "",
			expect: nil,
		},
		{
			name:   "whitespace only",
			input:  "  \n  \n",
			expect: nil,
		},
		{
			name:   "mixed valid and invalid lines",
			input:  "100\nnot_a_number\n200\n",
			expect: []int{100, 200},
		},
		{
			name:   "trailing whitespace on ids",
			input:  " 42 \n 99 \n",
			expect: []int{42, 99},
		},
		{
			name:   "no trailing newline",
			input:  "500",
			expect: []int{500},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCommentIDs(tt.input)
			if len(got) != len(tt.expect) {
				t.Fatalf("parseCommentIDs(%q) returned %d ids, want %d", tt.input, len(got), len(tt.expect))
			}
			for i := range got {
				if got[i] != tt.expect[i] {
					t.Errorf("parseCommentIDs(%q)[%d] = %d, want %d", tt.input, i, got[i], tt.expect[i])
				}
			}
		})
	}
}

func TestReplyToReviewComments_NoPRNumber(t *testing.T) {
	// replyToReviewComments should return early without error when
	// given a URL that doesn't contain a valid PR number.
	// This test verifies it doesn't panic on empty/invalid URLs.
	replyToReviewComments(nil, "", 0)
	replyToReviewComments(nil, "not-a-url", 0)
}

func TestReplyToReviewComments_PRNumberZero(t *testing.T) {
	// ExtractPRNumber returns 0 for PR URL ending in /pull/0,
	// which means the function returns early without making API calls.
	// This is a valid edge case since PR #0 does not exist.
	replyToReviewComments(nil, "https://github.com/owner/repo/pull/0", 0)
}
