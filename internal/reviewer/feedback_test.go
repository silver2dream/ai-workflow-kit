package reviewer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecordFeedback(t *testing.T) {
	tmpDir := t.TempDir()

	entry := FeedbackEntry{
		Timestamp:  "2026-02-26T10:00:00Z",
		IssueID:    42,
		PRNumber:   100,
		Score:      4,
		Categories: []string{"test", "naming"},
		Summary:    "Missing unit tests for new function",
		Attempt:    1,
	}

	if err := RecordFeedback(tmpDir, entry); err != nil {
		t.Fatalf("RecordFeedback failed: %v", err)
	}

	// Verify file exists
	path := filepath.Join(tmpDir, feedbackFile)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("feedback file should exist after recording")
	}

	// Read back
	entries, err := LoadFeedback(tmpDir)
	if err != nil {
		t.Fatalf("LoadFeedback failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].IssueID != 42 {
		t.Errorf("IssueID = %d, want 42", entries[0].IssueID)
	}
	if entries[0].Score != 4 {
		t.Errorf("Score = %d, want 4", entries[0].Score)
	}
	if len(entries[0].Categories) != 2 {
		t.Errorf("Categories len = %d, want 2", len(entries[0].Categories))
	}
}

func TestRecordFeedback_Multiple(t *testing.T) {
	tmpDir := t.TempDir()

	for i := 0; i < 5; i++ {
		entry := FeedbackEntry{
			Timestamp: "2026-02-26T10:00:00Z",
			IssueID:   i + 1,
			PRNumber:  i + 10,
			Score:     3 + i,
		}
		if err := RecordFeedback(tmpDir, entry); err != nil {
			t.Fatalf("RecordFeedback #%d failed: %v", i, err)
		}
	}

	entries, err := LoadFeedback(tmpDir)
	if err != nil {
		t.Fatalf("LoadFeedback failed: %v", err)
	}
	if len(entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(entries))
	}
}

func TestLoadFeedback_Empty(t *testing.T) {
	tmpDir := t.TempDir()

	entries, err := LoadFeedback(tmpDir)
	if err != nil {
		t.Fatalf("LoadFeedback failed: %v", err)
	}
	if entries != nil {
		t.Errorf("expected nil entries for non-existent file, got %v", entries)
	}
}

func TestLoadRecentFeedback(t *testing.T) {
	tmpDir := t.TempDir()

	for i := 0; i < 10; i++ {
		entry := FeedbackEntry{
			Timestamp: "2026-02-26T10:00:00Z",
			IssueID:   i + 1,
			Score:     3,
		}
		if err := RecordFeedback(tmpDir, entry); err != nil {
			t.Fatalf("RecordFeedback #%d failed: %v", i, err)
		}
	}

	// Limit to 3
	entries, err := LoadRecentFeedback(tmpDir, 3)
	if err != nil {
		t.Fatalf("LoadRecentFeedback failed: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	// Should be the last 3 (IssueID 8, 9, 10)
	if entries[0].IssueID != 8 {
		t.Errorf("first entry IssueID = %d, want 8", entries[0].IssueID)
	}

	// Limit larger than total
	entries, err = LoadRecentFeedback(tmpDir, 20)
	if err != nil {
		t.Fatalf("LoadRecentFeedback failed: %v", err)
	}
	if len(entries) != 10 {
		t.Fatalf("expected 10 entries, got %d", len(entries))
	}

	// Limit 0 returns all
	entries, err = LoadRecentFeedback(tmpDir, 0)
	if err != nil {
		t.Fatalf("LoadRecentFeedback failed: %v", err)
	}
	if len(entries) != 10 {
		t.Fatalf("expected 10 entries, got %d", len(entries))
	}
}

func TestExtractCategories(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantAny    []string // at least these categories should be present
		wantNone   []string // these categories should NOT be present
	}{
		{
			name:    "test issues",
			body:    "Missing unit test coverage for the new handler function",
			wantAny: []string{"test"},
		},
		{
			name:    "error handling",
			body:    "Error handling is not implemented correctly, panic may occur",
			wantAny: []string{"error-handling"},
		},
		{
			name:    "security concern",
			body:    "This code has a potential SQL injection vulnerability",
			wantAny: []string{"security"},
		},
		{
			name:    "multiple categories",
			body:    "Performance is slow and the variable naming is inconsistent. Also missing tests.",
			wantAny: []string{"performance", "naming", "test"},
		},
		{
			name:     "no matches",
			body:     "Looks good overall, minor cosmetic change",
			wantNone: []string{"test", "error-handling", "security"},
		},
		{
			name:    "scope issue",
			body:    "This change is out of scope for the ticket",
			wantAny: []string{"scope"},
		},
		{
			name:    "architecture issue",
			body:    "The layer separation of concerns is violated here",
			wantAny: []string{"architecture"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			categories := ExtractCategories(tt.body)
			catSet := make(map[string]bool)
			for _, c := range categories {
				catSet[c] = true
			}

			for _, want := range tt.wantAny {
				if !catSet[want] {
					t.Errorf("expected category %q in %v", want, categories)
				}
			}
			for _, notWant := range tt.wantNone {
				if catSet[notWant] {
					t.Errorf("did not expect category %q in %v", notWant, categories)
				}
			}
		})
	}
}

func TestFormatFeedbackForPrompt(t *testing.T) {
	entries := []FeedbackEntry{
		{IssueID: 1, Score: 3, Categories: []string{"test"}, Summary: "Missing tests"},
		{IssueID: 2, Score: 5, Categories: []string{"naming", "style"}, Summary: "Inconsistent naming convention"},
	}

	result := FormatFeedbackForPrompt(entries, 2000)

	if result == "" {
		t.Fatal("expected non-empty result")
	}
	if !strings.Contains(result, "Issue #1") {
		t.Error("should contain Issue #1")
	}
	if !strings.Contains(result, "Issue #2") {
		t.Error("should contain Issue #2")
	}
	if !strings.Contains(result, "test") {
		t.Error("should contain category 'test'")
	}
	if !strings.Contains(result, "Missing tests") {
		t.Error("should contain summary text")
	}
}

func TestFormatFeedbackForPrompt_Empty(t *testing.T) {
	result := FormatFeedbackForPrompt(nil, 2000)
	if result != "" {
		t.Errorf("expected empty result for nil entries, got %q", result)
	}
}

func TestFormatFeedbackForPrompt_Truncation(t *testing.T) {
	// Create entries that exceed maxChars
	var entries []FeedbackEntry
	for i := 0; i < 50; i++ {
		entries = append(entries, FeedbackEntry{
			IssueID:    i + 1,
			Score:      3,
			Categories: []string{"test"},
			Summary:    strings.Repeat("x", 100),
		})
	}

	result := FormatFeedbackForPrompt(entries, 500)
	if len(result) > 600 { // some tolerance for the header
		t.Errorf("result too long: %d chars, expected <= 600", len(result))
	}
}

func TestTruncateSummary(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"short", 10, "short"},
		{"hello world", 5, "he..."},
		{"ab", 2, "ab"},
		{"abc", 3, "abc"},
		{"abcd", 3, "abc"},
	}

	for _, tt := range tests {
		got := truncateSummary(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncateSummary(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}

func TestBuildFeedbackEntry(t *testing.T) {
	entry := BuildFeedbackEntry(42, 100, 5, "Missing tests and error handling is broken")

	if entry.IssueID != 42 {
		t.Errorf("IssueID = %d, want 42", entry.IssueID)
	}
	if entry.PRNumber != 100 {
		t.Errorf("PRNumber = %d, want 100", entry.PRNumber)
	}
	if entry.Score != 5 {
		t.Errorf("Score = %d, want 5", entry.Score)
	}
	if entry.Timestamp == "" {
		t.Error("Timestamp should not be empty")
	}
	if len(entry.Categories) == 0 {
		t.Error("Categories should not be empty for review body with known patterns")
	}
}
