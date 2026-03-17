package jittest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadStats_NoFile(t *testing.T) {
	stats, err := LoadStats(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.TotalRuns != 0 {
		t.Errorf("expected 0 runs, got %d", stats.TotalRuns)
	}
	if stats.ByLanguage == nil {
		t.Error("ByLanguage should be initialized")
	}
}

func TestSaveAndLoadStats(t *testing.T) {
	dir := t.TempDir()
	stats := &Stats{
		TotalRuns:      10,
		TotalGenerated: 50,
		TotalPassed:    45,
		TotalFailed:    5,
		ByLanguage: map[string]*LangStats{
			"go": {Runs: 10, Generated: 50, Passed: 45, Failed: 5},
		},
	}

	if err := SaveStats(dir, stats); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := LoadStats(dir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if loaded.TotalRuns != 10 {
		t.Errorf("expected 10 runs, got %d", loaded.TotalRuns)
	}
	if loaded.TotalPassed != 45 {
		t.Errorf("expected 45 passed, got %d", loaded.TotalPassed)
	}
	if loaded.ByLanguage["go"] == nil {
		t.Fatal("expected go language stats")
	}
	if loaded.ByLanguage["go"].Runs != 10 {
		t.Errorf("expected 10 go runs, got %d", loaded.ByLanguage["go"].Runs)
	}
	if loaded.LastUpdated == "" {
		t.Error("LastUpdated should be set")
	}
}

func TestSaveStats_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	stats := &Stats{TotalRuns: 1, ByLanguage: make(map[string]*LangStats)}

	if err := SaveStats(dir, stats); err != nil {
		t.Fatal(err)
	}

	// Verify no .tmp file remains
	tmpPath := filepath.Join(dir, statsFile+".tmp")
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("tmp file should not remain after save")
	}

	// Verify actual file exists
	path := filepath.Join(dir, statsFile)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("stats file should exist after save")
	}
}

func TestRecordRun(t *testing.T) {
	dir := t.TempDir()
	result := &Result{Generated: 5, Passed: 4, Failed: 1, Skipped: 0}

	if err := RecordRun(dir, "go", result); err != nil {
		t.Fatalf("record failed: %v", err)
	}

	stats, _ := LoadStats(dir)
	if stats.TotalRuns != 1 {
		t.Errorf("expected 1 run, got %d", stats.TotalRuns)
	}
	if stats.TotalGenerated != 5 {
		t.Errorf("expected 5 generated, got %d", stats.TotalGenerated)
	}
	if stats.TotalFailed != 1 {
		t.Errorf("expected 1 failed, got %d", stats.TotalFailed)
	}
	if stats.ByLanguage["go"].Runs != 1 {
		t.Errorf("expected 1 go run, got %d", stats.ByLanguage["go"].Runs)
	}
}

func TestRecordRun_Multiple(t *testing.T) {
	dir := t.TempDir()

	RecordRun(dir, "go", &Result{Generated: 3, Passed: 3})
	RecordRun(dir, "typescript", &Result{Generated: 2, Passed: 1, Failed: 1})
	RecordRun(dir, "go", &Result{Generated: 4, Passed: 2, Failed: 2})

	stats, _ := LoadStats(dir)
	if stats.TotalRuns != 3 {
		t.Errorf("expected 3 runs, got %d", stats.TotalRuns)
	}
	if stats.ByLanguage["go"].Runs != 2 {
		t.Errorf("expected 2 go runs, got %d", stats.ByLanguage["go"].Runs)
	}
	if stats.ByLanguage["typescript"].Runs != 1 {
		t.Errorf("expected 1 ts run, got %d", stats.ByLanguage["typescript"].Runs)
	}
}

func TestMarkFalsePositive(t *testing.T) {
	dir := t.TempDir()
	RecordRun(dir, "go", &Result{Generated: 1, Failed: 1})

	if err := MarkFalsePositive(dir); err != nil {
		t.Fatal(err)
	}

	stats, _ := LoadStats(dir)
	if stats.FalsePositiveMarked != 1 {
		t.Errorf("expected 1 FP marked, got %d", stats.FalsePositiveMarked)
	}
}

func TestFalsePositiveRate(t *testing.T) {
	tests := []struct {
		name   string
		stats  Stats
		want   float64
	}{
		{"no failures", Stats{TotalFailed: 0, FalsePositiveMarked: 0}, 0},
		{"no FP", Stats{TotalFailed: 10, FalsePositiveMarked: 0}, 0},
		{"50% FP", Stats{TotalFailed: 10, FalsePositiveMarked: 5}, 50},
		{"100% FP", Stats{TotalFailed: 2, FalsePositiveMarked: 2}, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.stats.FalsePositiveRate()
			if got != tt.want {
				t.Errorf("FalsePositiveRate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldSuggestBlock(t *testing.T) {
	tests := []struct {
		name  string
		stats Stats
		want  bool
	}{
		{"not enough runs", Stats{TotalRuns: 10, TotalFailed: 1, FalsePositiveMarked: 0}, false},
		{"enough runs low FP", Stats{TotalRuns: 25, TotalFailed: 10, FalsePositiveMarked: 0}, true},
		{"enough runs high FP", Stats{TotalRuns: 25, TotalFailed: 10, FalsePositiveMarked: 5}, false},
		{"enough runs borderline FP", Stats{TotalRuns: 20, TotalFailed: 100, FalsePositiveMarked: 9}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.stats.ShouldSuggestBlock(); got != tt.want {
				t.Errorf("ShouldSuggestBlock() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatStats_NoRuns(t *testing.T) {
	stats := &Stats{}
	out := FormatStats(stats)
	if !strings.Contains(out, "No JiTTest runs") {
		t.Error("should indicate no runs")
	}
}

func TestFormatStats_WithData(t *testing.T) {
	stats := &Stats{
		TotalRuns:      25,
		TotalGenerated: 100,
		TotalPassed:    90,
		TotalFailed:    10,
		ByLanguage: map[string]*LangStats{
			"go": {Runs: 25, Generated: 100, Passed: 90, Failed: 10},
		},
	}
	out := FormatStats(stats)
	if !strings.Contains(out, "Total runs:        25") {
		t.Error("should show total runs")
	}
	if !strings.Contains(out, "go:") {
		t.Error("should show language breakdown")
	}
}

func TestFormatStats_SuggestsBlock(t *testing.T) {
	stats := &Stats{
		TotalRuns:           25,
		TotalFailed:         10,
		FalsePositiveMarked: 0,
		ByLanguage:          make(map[string]*LangStats),
	}
	out := FormatStats(stats)
	if !strings.Contains(out, "Consider upgrading") {
		t.Error("should suggest upgrading to block when FP rate is low")
	}
}
