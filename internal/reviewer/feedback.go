package reviewer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FeedbackEntry represents a single review feedback record.
type FeedbackEntry struct {
	Timestamp  string   `json:"timestamp"`
	IssueID    int      `json:"issue_id"`
	PRNumber   int      `json:"pr_number"`
	Score      int      `json:"score"`
	Categories []string `json:"categories"`
	Summary    string   `json:"summary"`
	Attempt    int      `json:"attempt"`
}

// feedbackFile is the JSONL file path relative to state root.
const feedbackFile = ".ai/state/review_feedback.jsonl"

// RecordFeedback appends a feedback entry to the JSONL file.
func RecordFeedback(stateRoot string, entry FeedbackEntry) (err error) {
	path := filepath.Join(stateRoot, feedbackFile)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create feedback dir: %w", err)
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	_, err = f.WriteString(string(data) + "\n")
	return err
}

// LoadFeedback reads all feedback entries from the JSONL file.
func LoadFeedback(stateRoot string) ([]FeedbackEntry, error) {
	path := filepath.Join(stateRoot, feedbackFile)
	return readFeedbackFile(path)
}

// LoadRecentFeedback reads the last N feedback entries.
func LoadRecentFeedback(stateRoot string, limit int) ([]FeedbackEntry, error) {
	entries, err := LoadFeedback(stateRoot)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit >= len(entries) {
		return entries, nil
	}
	return entries[len(entries)-limit:], nil
}

func readFeedbackFile(path string) ([]FeedbackEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var entries []FeedbackEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry FeedbackEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue // skip malformed lines
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return entries, err
	}
	return entries, nil
}

// rejectionCategories maps keywords to category names.
var rejectionCategories = map[string][]string{
	"test":          {"test", "testing", "unit test", "integration test", "test coverage"},
	"error-handling": {"error handling", "error", "panic", "recover", "exception"},
	"naming":        {"naming", "variable name", "function name", "rename"},
	"architecture":  {"architecture", "structure", "layer", "separation of concerns", "dependency"},
	"security":      {"security", "vulnerability", "injection", "auth", "credential"},
	"performance":   {"performance", "slow", "optimize", "memory", "cpu", "latency"},
	"scope":         {"scope", "out of scope", "non-goal", "beyond scope"},
	"style":         {"style", "formatting", "lint", "convention"},
}

// ExtractCategories scans a review body for common rejection patterns.
func ExtractCategories(reviewBody string) []string {
	lower := strings.ToLower(reviewBody)
	var found []string
	seen := make(map[string]bool)

	for category, keywords := range rejectionCategories {
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				if !seen[category] {
					seen[category] = true
					found = append(found, category)
				}
				break
			}
		}
	}
	return found
}

// FormatFeedbackForPrompt formats feedback entries as concise text for Worker prompt injection.
// Output is capped at maxChars characters.
func FormatFeedbackForPrompt(entries []FeedbackEntry, maxChars int) string {
	if len(entries) == 0 {
		return ""
	}
	if maxChars <= 0 {
		maxChars = 2000
	}

	var b strings.Builder
	b.WriteString("The following patterns were found in previous review rejections. Avoid repeating these issues:\n\n")

	for i, e := range entries {
		line := fmt.Sprintf("%d. Issue #%d (score: %d/10)", i+1, e.IssueID, e.Score)
		if len(e.Categories) > 0 {
			line += " [" + strings.Join(e.Categories, ", ") + "]"
		}
		if e.Summary != "" {
			summary := e.Summary
			if len(summary) > 200 {
				summary = summary[:200] + "..."
			}
			line += ": " + summary
		}
		line += "\n"

		if b.Len()+len(line) > maxChars {
			break
		}
		b.WriteString(line)
	}
	return b.String()
}

// truncateSummary truncates a string to maxLen, adding "..." if truncated.
func truncateSummary(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// BuildFeedbackEntry creates a FeedbackEntry with common defaults.
func BuildFeedbackEntry(issueID, prNumber, score int, reviewBody string) FeedbackEntry {
	return FeedbackEntry{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		IssueID:    issueID,
		PRNumber:   prNumber,
		Score:      score,
		Categories: ExtractCategories(reviewBody),
		Summary:    truncateSummary(reviewBody, 500),
	}
}
