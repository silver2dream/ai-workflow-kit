package reviewer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	// Paths anchors the entry to the PR's changed files so distilled
	// lessons can carry a scope.
	Paths []string `json:"paths,omitempty"`
	// Outcome distinguishes approvals ("approved") from rejections (empty
	// or the rejection kind). Approved entries feed settlement denominators
	// and are never distilled into pitfalls.
	Outcome string `json:"outcome,omitempty"`
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

// CategoryCount holds a category name and its occurrence count.
type CategoryCount struct {
	Name  string
	Count int
}

// TopCategories returns the top N most frequent rejection categories sorted by count descending.
func TopCategories(entries []FeedbackEntry, n int) []CategoryCount {
	if len(entries) == 0 || n <= 0 {
		return nil
	}

	counts := make(map[string]int)
	for _, e := range entries {
		for _, c := range e.Categories {
			counts[c]++
		}
	}

	sorted := make([]CategoryCount, 0, len(counts))
	for name, count := range counts {
		sorted = append(sorted, CategoryCount{Name: name, Count: count})
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Count != sorted[j].Count {
			return sorted[i].Count > sorted[j].Count
		}
		return sorted[i].Name < sorted[j].Name
	})

	if n > len(sorted) {
		n = len(sorted)
	}
	return sorted[:n]
}

// CategoryTrend represents how a category's frequency has changed between recent and overall entries.
type CategoryTrend struct {
	Name       string
	RecentPct  float64 // percentage in recent entries
	OverallPct float64 // percentage in all entries
	Direction  string  // "increasing", "decreasing", or "stable"
}

// AnalyzeTrends compares category distribution of the last recentN entries vs all entries.
// Returns trends sorted by absolute change descending, only including categories that changed.
func AnalyzeTrends(entries []FeedbackEntry, recentN int) []CategoryTrend {
	if len(entries) == 0 || recentN <= 0 {
		return nil
	}

	// Build category counts for all entries
	allCounts := make(map[string]int)
	for _, e := range entries {
		for _, c := range e.Categories {
			allCounts[c]++
		}
	}

	// Build category counts for recent entries
	recentStart := len(entries) - recentN
	if recentStart < 0 {
		recentStart = 0
	}
	recent := entries[recentStart:]
	recentCounts := make(map[string]int)
	for _, e := range recent {
		for _, c := range e.Categories {
			recentCounts[c]++
		}
	}

	// Calculate percentages and detect direction
	allTotal := 0
	for _, v := range allCounts {
		allTotal += v
	}
	recentTotal := 0
	for _, v := range recentCounts {
		recentTotal += v
	}

	if allTotal == 0 {
		return nil
	}

	// Collect all category names
	allNames := make(map[string]bool)
	for k := range allCounts {
		allNames[k] = true
	}
	for k := range recentCounts {
		allNames[k] = true
	}

	const threshold = 5.0 // minimum percentage point change to report
	var trends []CategoryTrend
	for name := range allNames {
		overallPct := float64(allCounts[name]) / float64(allTotal) * 100
		recentPct := 0.0
		if recentTotal > 0 {
			recentPct = float64(recentCounts[name]) / float64(recentTotal) * 100
		}

		diff := recentPct - overallPct
		direction := "stable"
		if diff > threshold {
			direction = "increasing"
		} else if diff < -threshold {
			direction = "decreasing"
		} else {
			continue // skip stable categories
		}

		trends = append(trends, CategoryTrend{
			Name:       name,
			RecentPct:  recentPct,
			OverallPct: overallPct,
			Direction:  direction,
		})
	}

	// Sort by absolute change descending
	sort.Slice(trends, func(i, j int) bool {
		absI := trends[i].RecentPct - trends[i].OverallPct
		if absI < 0 {
			absI = -absI
		}
		absJ := trends[j].RecentPct - trends[j].OverallPct
		if absJ < 0 {
			absJ = -absJ
		}
		return absI > absJ
	})

	return trends
}

// FormatTopCategoriesForPrompt formats the top categories as a prompt injection section.
func FormatTopCategoriesForPrompt(entries []FeedbackEntry, n int) string {
	top := TopCategories(entries, n)
	if len(top) == 0 {
		return ""
	}

	descriptions := map[string]string{
		"test":           "ensure adequate test coverage",
		"error-handling": "handle all error paths",
		"naming":         "use descriptive names",
		"architecture":   "follow layered architecture patterns",
		"security":       "address security concerns",
		"performance":    "optimize for performance",
		"scope":          "stay within ticket scope",
		"style":          "follow code style conventions",
		"jittest":        "write independent, non-flaky tests",
	}

	var b strings.Builder
	b.WriteString("COMMON REJECTION PATTERNS (avoid these mistakes):\n")
	for i, c := range top {
		desc := descriptions[c.Name]
		if desc == "" {
			desc = "pay attention to " + c.Name
		}
		b.WriteString(fmt.Sprintf("%d. %s (seen %d times) — %s\n", i+1, c.Name, c.Count, desc))
	}
	return b.String()
}

// rejectionCategories maps keywords to category names.
var rejectionCategories = map[string][]string{
	"test":           {"test", "testing", "unit test", "integration test", "test coverage"},
	"error-handling": {"error handling", "error", "panic", "recover", "exception"},
	"naming":         {"naming", "variable name", "function name", "rename"},
	"architecture":   {"architecture", "structure", "layer", "separation of concerns", "dependency"},
	"security":       {"security", "vulnerability", "injection", "auth", "credential"},
	"performance":    {"performance", "slow", "optimize", "memory", "cpu", "latency"},
	"scope":          {"scope", "out of scope", "non-goal", "beyond scope"},
	"style":          {"style", "formatting", "lint", "convention"},
	"jittest":        {"jit test", "jittest", "jit-test", "independent test"},
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

// maxFeedbackPaths caps how many changed-file paths a feedback entry keeps.
const maxFeedbackPaths = 10

// DiffPaths extracts changed-file paths from a unified diff ("+++ b/" lines),
// capped at maxFeedbackPaths.
func DiffPaths(diff string) []string {
	var paths []string
	seen := make(map[string]bool)
	for _, line := range strings.Split(diff, "\n") {
		path, ok := strings.CutPrefix(line, "+++ b/")
		if !ok || path == "/dev/null" {
			continue
		}
		path = strings.TrimSpace(path)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		paths = append(paths, path)
		if len(paths) >= maxFeedbackPaths {
			break
		}
	}
	return paths
}
