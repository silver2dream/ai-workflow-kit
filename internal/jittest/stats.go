package jittest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const statsFile = ".ai/state/jittest-stats.json"

// Stats holds cumulative JiTTest statistics.
type Stats struct {
	TotalRuns           int                    `json:"total_runs"`
	TotalGenerated      int                    `json:"total_generated"`
	TotalPassed         int                    `json:"total_passed"`
	TotalFailed         int                    `json:"total_failed"`
	TotalSkipped        int                    `json:"total_skipped"`
	FalsePositiveMarked int                    `json:"false_positive_marked"`
	ByLanguage          map[string]*LangStats  `json:"by_language"`
	LastUpdated         string                 `json:"last_updated"`
}

// LangStats holds per-language statistics.
type LangStats struct {
	Runs      int `json:"runs"`
	Generated int `json:"generated"`
	Passed    int `json:"passed"`
	Failed    int `json:"failed"`
}

// FalsePositiveRate returns the false positive rate as a percentage.
// Returns 0 if there are no failures.
func (s *Stats) FalsePositiveRate() float64 {
	if s.TotalFailed == 0 {
		return 0
	}
	return float64(s.FalsePositiveMarked) / float64(s.TotalFailed) * 100
}

// ShouldSuggestBlock returns true if stats indicate the system is reliable
// enough to suggest upgrading failure_policy from "warn" to "block".
func (s *Stats) ShouldSuggestBlock() bool {
	return s.TotalRuns >= 20 && s.FalsePositiveRate() < 10.0
}

// LoadStats reads the stats file. Returns empty Stats if not found.
func LoadStats(stateRoot string) (*Stats, error) {
	path := filepath.Join(stateRoot, statsFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Stats{ByLanguage: make(map[string]*LangStats)}, nil
		}
		return nil, fmt.Errorf("failed to read stats: %w", err)
	}

	var stats Stats
	if err := json.Unmarshal(data, &stats); err != nil {
		return nil, fmt.Errorf("failed to parse stats: %w", err)
	}
	if stats.ByLanguage == nil {
		stats.ByLanguage = make(map[string]*LangStats)
	}
	return &stats, nil
}

// SaveStats writes stats to file using atomic write (tmp + rename).
func SaveStats(stateRoot string, stats *Stats) error {
	path := filepath.Join(stateRoot, statsFile)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create stats dir: %w", err)
	}

	stats.LastUpdated = time.Now().UTC().Format(time.RFC3339)

	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal stats: %w", err)
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write stats tmp: %w", err)
	}

	// Atomic rename (Windows: remove first)
	os.Remove(path)
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to rename stats: %w", err)
	}

	return nil
}

// RecordRun updates stats with the results of a JiTTest run.
func RecordRun(stateRoot, language string, result *Result) error {
	stats, err := LoadStats(stateRoot)
	if err != nil {
		stats = &Stats{ByLanguage: make(map[string]*LangStats)}
	}

	stats.TotalRuns++
	stats.TotalGenerated += result.Generated
	stats.TotalPassed += result.Passed
	stats.TotalFailed += result.Failed
	stats.TotalSkipped += result.Skipped

	// Per-language stats
	if language != "" {
		lang, ok := stats.ByLanguage[language]
		if !ok {
			lang = &LangStats{}
			stats.ByLanguage[language] = lang
		}
		lang.Runs++
		lang.Generated += result.Generated
		lang.Passed += result.Passed
		lang.Failed += result.Failed
	}

	return SaveStats(stateRoot, stats)
}

// MarkFalsePositive increments the false positive counter.
func MarkFalsePositive(stateRoot string) error {
	stats, err := LoadStats(stateRoot)
	if err != nil {
		return err
	}
	stats.FalsePositiveMarked++
	return SaveStats(stateRoot, stats)
}

// FormatStats returns a human-readable summary of JiTTest statistics.
func FormatStats(stats *Stats) string {
	if stats.TotalRuns == 0 {
		return "No JiTTest runs recorded.\n"
	}

	s := fmt.Sprintf("JiTTest Statistics:\n")
	s += fmt.Sprintf("  Total runs:        %d\n", stats.TotalRuns)
	s += fmt.Sprintf("  Tests generated:   %d\n", stats.TotalGenerated)
	s += fmt.Sprintf("  Tests passed:      %d\n", stats.TotalPassed)
	s += fmt.Sprintf("  Tests failed:      %d\n", stats.TotalFailed)
	s += fmt.Sprintf("  Tests skipped:     %d\n", stats.TotalSkipped)
	s += fmt.Sprintf("  False positives:   %d\n", stats.FalsePositiveMarked)
	s += fmt.Sprintf("  FP rate:           %.1f%%\n", stats.FalsePositiveRate())

	if len(stats.ByLanguage) > 0 {
		s += "\n  By language:\n"
		for lang, ls := range stats.ByLanguage {
			s += fmt.Sprintf("    %s: %d runs, %d generated, %d passed, %d failed\n",
				lang, ls.Runs, ls.Generated, ls.Passed, ls.Failed)
		}
	}

	if stats.ShouldSuggestBlock() {
		s += fmt.Sprintf("\n  Suggestion: FP rate %.1f%% (<%d runs). Consider upgrading failure_policy to \"block\".\n",
			stats.FalsePositiveRate(), stats.TotalRuns)
	}

	return s
}
