package reviewer

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Finding represents a single review finding with a stable identity.
type Finding struct {
	ID       string `json:"id"`       // hash of file+category+body
	File     string `json:"file"`
	Category string `json:"category"` // e.g. "security", "performance", "style"
	Body     string `json:"body"`
}

// DeltaResult holds the delta between two sets of findings.
type DeltaResult struct {
	New        []Finding `json:"new"`        // not in previous review
	Fixed      []Finding `json:"fixed"`      // was in previous, not in current
	Persisting []Finding `json:"persisting"` // in both
}

// ComputeFindingID generates a stable hash ID for a finding based on its
// file, category, and body. This allows tracking findings across reviews.
func ComputeFindingID(file, category, body string) string {
	h := sha256.New()
	h.Write([]byte(file))
	h.Write([]byte("\x00"))
	h.Write([]byte(category))
	h.Write([]byte("\x00"))
	h.Write([]byte(body))
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

// ComputeDelta compares previous and current findings to determine
// which are new, fixed, or persisting.
func ComputeDelta(previous, current []Finding) DeltaResult {
	prevMap := make(map[string]Finding, len(previous))
	for _, f := range previous {
		id := f.ID
		if id == "" {
			id = ComputeFindingID(f.File, f.Category, f.Body)
		}
		prevMap[id] = f
	}

	currMap := make(map[string]Finding, len(current))
	for _, f := range current {
		id := f.ID
		if id == "" {
			id = ComputeFindingID(f.File, f.Category, f.Body)
		}
		currMap[id] = f
	}

	var result DeltaResult

	// Check current findings against previous
	for id, f := range currMap {
		if _, existed := prevMap[id]; existed {
			result.Persisting = append(result.Persisting, f)
		} else {
			result.New = append(result.New, f)
		}
	}

	// Check previous findings not in current (fixed)
	for id, f := range prevMap {
		if _, exists := currMap[id]; !exists {
			result.Fixed = append(result.Fixed, f)
		}
	}

	return result
}

// findingsFilePath returns the path for storing review findings for a PR.
func findingsFilePath(stateRoot string, prNumber int) string {
	return filepath.Join(stateRoot, ".ai", "state", fmt.Sprintf("review-findings-%d.json", prNumber))
}

// LoadPreviousFindings loads the previously saved findings for a PR.
// Returns nil if no previous findings exist.
func LoadPreviousFindings(stateRoot string, prNumber int) ([]Finding, error) {
	path := findingsFilePath(stateRoot, prNumber)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read previous findings: %w", err)
	}

	var findings []Finding
	if err := json.Unmarshal(data, &findings); err != nil {
		return nil, fmt.Errorf("failed to parse previous findings: %w", err)
	}

	return findings, nil
}

// SaveFindings persists the current findings for a PR for future delta comparison.
func SaveFindings(stateRoot string, prNumber int, findings []Finding) error {
	path := findingsFilePath(stateRoot, prNumber)

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	data, err := json.MarshalIndent(findings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal findings: %w", err)
	}

	// Atomic write: write to tmp then rename
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write findings file: %w", err)
	}

	// On Windows, remove target before rename
	_ = os.Remove(path)
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to rename findings file: %w", err)
	}

	return nil
}

// FormatDeltaReport formats the delta result into a markdown section
// suitable for inclusion in a review report.
func FormatDeltaReport(delta DeltaResult) string {
	if len(delta.New) == 0 && len(delta.Fixed) == 0 && len(delta.Persisting) == 0 {
		return ""
	}

	var s string
	s += "### Delta from Previous Review\n\n"

	if len(delta.Fixed) > 0 {
		s += fmt.Sprintf("**Fixed (%d)**:\n", len(delta.Fixed))
		for _, f := range delta.Fixed {
			s += fmt.Sprintf("- [FIXED] %s: %s\n", f.File, f.Body)
		}
		s += "\n"
	}

	if len(delta.New) > 0 {
		s += fmt.Sprintf("**New (%d)**:\n", len(delta.New))
		for _, f := range delta.New {
			s += fmt.Sprintf("- [NEW] %s: %s\n", f.File, f.Body)
		}
		s += "\n"
	}

	if len(delta.Persisting) > 0 {
		s += fmt.Sprintf("**Persisting (%d)**:\n", len(delta.Persisting))
		for _, f := range delta.Persisting {
			s += fmt.Sprintf("- [PERSISTING] %s: %s\n", f.File, f.Body)
		}
		s += "\n"
	}

	return s
}
