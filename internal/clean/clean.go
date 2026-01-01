package clean

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// CleanResult represents the result of a clean operation
type CleanResult struct {
	Name    string
	Success bool
	Message string
}

// Cleaner performs cleanup operations on the AWK project
type Cleaner struct {
	StateRoot string
	Timeout   time.Duration
	DryRun    bool
}

// New creates a new Cleaner
func New(stateRoot string) *Cleaner {
	if stateRoot == "" {
		stateRoot = "."
	}
	return &Cleaner{
		StateRoot: stateRoot,
		Timeout:   30 * time.Second,
	}
}

// SetDryRun enables dry-run mode
func (c *Cleaner) SetDryRun(dryRun bool) {
	c.DryRun = dryRun
}

// CleanAll cleans all state
func (c *Cleaner) CleanAll(ctx context.Context) []CleanResult {
	var results []CleanResult
	results = append(results, c.CleanState()...)
	results = append(results, c.CleanAttempts()...)
	results = append(results, c.CleanStop())
	results = append(results, c.CleanLock())
	results = append(results, c.CleanDeprecated()...)
	return results
}

// CleanState cleans loop_count and consecutive_failures
func (c *Cleaner) CleanState() []CleanResult {
	var results []CleanResult

	files := []string{
		filepath.Join(c.StateRoot, ".ai", "state", "loop_count"),
		filepath.Join(c.StateRoot, ".ai", "state", "consecutive_failures"),
	}

	for _, f := range files {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			continue
		}

		name := filepath.Base(f)
		if c.DryRun {
			results = append(results, CleanResult{
				Name:    name,
				Success: true,
				Message: fmt.Sprintf("Would delete %s", f),
			})
			continue
		}

		if err := os.Remove(f); err != nil {
			results = append(results, CleanResult{
				Name:    name,
				Success: false,
				Message: fmt.Sprintf("Failed to delete: %v", err),
			})
		} else {
			results = append(results, CleanResult{
				Name:    name,
				Success: true,
				Message: "Deleted",
			})
		}
	}

	return results
}

// CleanAttempts cleans attempt tracking files
func (c *Cleaner) CleanAttempts() []CleanResult {
	attemptsDir := filepath.Join(c.StateRoot, ".ai", "state", "attempts")

	if _, err := os.Stat(attemptsDir); os.IsNotExist(err) {
		return nil
	}

	if c.DryRun {
		return []CleanResult{{
			Name:    "attempts",
			Success: true,
			Message: fmt.Sprintf("Would delete %s/*", attemptsDir),
		}}
	}

	if err := os.RemoveAll(attemptsDir); err != nil {
		return []CleanResult{{
			Name:    "attempts",
			Success: false,
			Message: fmt.Sprintf("Failed to delete: %v", err),
		}}
	}

	return []CleanResult{{
		Name:    "attempts",
		Success: true,
		Message: "Deleted attempts directory",
	}}
}

// CleanStop removes the STOP marker
func (c *Cleaner) CleanStop() CleanResult {
	stopMarker := filepath.Join(c.StateRoot, ".ai", "state", "STOP")

	if _, err := os.Stat(stopMarker); os.IsNotExist(err) {
		return CleanResult{
			Name:    "STOP marker",
			Success: true,
			Message: "Not present",
		}
	}

	if c.DryRun {
		return CleanResult{
			Name:    "STOP marker",
			Success: true,
			Message: fmt.Sprintf("Would delete %s", stopMarker),
		}
	}

	if err := os.Remove(stopMarker); err != nil {
		return CleanResult{
			Name:    "STOP marker",
			Success: false,
			Message: fmt.Sprintf("Failed to delete: %v", err),
		}
	}

	return CleanResult{
		Name:    "STOP marker",
		Success: true,
		Message: "Deleted",
	}
}

// CleanLock removes the lock file
func (c *Cleaner) CleanLock() CleanResult {
	lockFile := filepath.Join(c.StateRoot, ".ai", "state", "principal.lock")

	if _, err := os.Stat(lockFile); os.IsNotExist(err) {
		return CleanResult{
			Name:    "Lock file",
			Success: true,
			Message: "Not present",
		}
	}

	if c.DryRun {
		return CleanResult{
			Name:    "Lock file",
			Success: true,
			Message: fmt.Sprintf("Would delete %s", lockFile),
		}
	}

	if err := os.Remove(lockFile); err != nil {
		return CleanResult{
			Name:    "Lock file",
			Success: false,
			Message: fmt.Sprintf("Failed to delete: %v", err),
		}
	}

	return CleanResult{
		Name:    "Lock file",
		Success: true,
		Message: "Deleted",
	}
}

// CleanDeprecated removes deprecated files
func (c *Cleaner) CleanDeprecated() []CleanResult {
	deprecatedFiles := []string{
		".ai/skills/principal-workflow/tasks/review-pr.md",
		".ai/docs/evaluate.md",
	}

	var results []CleanResult
	for _, f := range deprecatedFiles {
		path := filepath.Join(c.StateRoot, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}

		if c.DryRun {
			results = append(results, CleanResult{
				Name:    f,
				Success: true,
				Message: fmt.Sprintf("Would delete %s", path),
			})
			continue
		}

		if err := os.Remove(path); err != nil {
			results = append(results, CleanResult{
				Name:    f,
				Success: false,
				Message: fmt.Sprintf("Failed to delete: %v", err),
			})
		} else {
			results = append(results, CleanResult{
				Name:    f,
				Success: true,
				Message: "Deleted deprecated file",
			})
		}
	}

	return results
}

// CleanResults removes result files
func (c *Cleaner) CleanResults() []CleanResult {
	resultsDir := filepath.Join(c.StateRoot, ".ai", "results")

	entries, err := os.ReadDir(resultsDir)
	if err != nil {
		return nil
	}

	var results []CleanResult
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(resultsDir, entry.Name())

		if c.DryRun {
			results = append(results, CleanResult{
				Name:    entry.Name(),
				Success: true,
				Message: fmt.Sprintf("Would delete %s", path),
			})
			continue
		}

		if err := os.Remove(path); err != nil {
			results = append(results, CleanResult{
				Name:    entry.Name(),
				Success: false,
				Message: fmt.Sprintf("Failed to delete: %v", err),
			})
		} else {
			results = append(results, CleanResult{
				Name:    entry.Name(),
				Success: true,
				Message: "Deleted",
			})
		}
	}

	return results
}

// ResetGitHubLabel resets a label on issues
func (c *Cleaner) ResetGitHubLabel(ctx context.Context, fromLabel, toLabel string) []CleanResult {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	// Get issues with the label
	cmd := exec.CommandContext(ctx, "gh", "issue", "list",
		"--label", fromLabel,
		"--state", "open",
		"--json", "number",
		"--limit", "50")

	output, err := cmd.Output()
	if err != nil {
		return []CleanResult{{
			Name:    "GitHub labels",
			Success: false,
			Message: fmt.Sprintf("Failed to list issues: %v", err),
		}}
	}

	var issues []struct {
		Number int `json:"number"`
	}
	if err := json.Unmarshal(output, &issues); err != nil {
		return []CleanResult{{
			Name:    "GitHub labels",
			Success: false,
			Message: fmt.Sprintf("Failed to parse issues: %v", err),
		}}
	}

	if len(issues) == 0 {
		return []CleanResult{{
			Name:    "GitHub labels",
			Success: true,
			Message: fmt.Sprintf("No issues with '%s' label", fromLabel),
		}}
	}

	var results []CleanResult
	for _, issue := range issues {
		if c.DryRun {
			results = append(results, CleanResult{
				Name:    fmt.Sprintf("Issue #%d", issue.Number),
				Success: true,
				Message: fmt.Sprintf("Would change label from '%s' to '%s'", fromLabel, toLabel),
			})
			continue
		}

		// Remove old label, add new label
		editCmd := exec.CommandContext(ctx, "gh", "issue", "edit",
			fmt.Sprintf("%d", issue.Number),
			"--remove-label", fromLabel,
			"--add-label", toLabel)

		if err := editCmd.Run(); err != nil {
			results = append(results, CleanResult{
				Name:    fmt.Sprintf("Issue #%d", issue.Number),
				Success: false,
				Message: fmt.Sprintf("Failed to update labels: %v", err),
			})
		} else {
			results = append(results, CleanResult{
				Name:    fmt.Sprintf("Issue #%d", issue.Number),
				Success: true,
				Message: fmt.Sprintf("Changed label from '%s' to '%s'", fromLabel, toLabel),
			})
		}
	}

	return results
}
