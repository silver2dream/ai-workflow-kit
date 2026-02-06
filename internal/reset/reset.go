package reset

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"
)

// Result represents the result of a reset operation
type Result struct {
	Name    string
	Success bool
	Message string
}

// Resetter performs reset operations on the AWK project
type Resetter struct {
	StateRoot string
	Timeout   time.Duration
	DryRun    bool
}

// New creates a new Resetter
func New(stateRoot string) *Resetter {
	if stateRoot == "" {
		stateRoot = "."
	}
	return &Resetter{
		StateRoot: stateRoot,
		Timeout:   30 * time.Second,
	}
}

// SetDryRun enables dry-run mode
func (c *Resetter) SetDryRun(dryRun bool) {
	c.DryRun = dryRun
}

// ResetAll cleans all state
func (c *Resetter) ResetAll(ctx context.Context) []Result {
	var results []Result
	results = append(results, c.ResetState()...)
	results = append(results, c.ResetAttempts()...)
	results = append(results, c.ResetStop())
	results = append(results, c.ResetLock())
	results = append(results, c.ResetDeprecated()...)
	results = append(results, c.ResetTraces()...)
	results = append(results, c.ResetEvents()...)
	results = append(results, c.CleanTemp()...)
	results = append(results, c.CleanSessions()...)
	results = append(results, c.CleanReports()...)
	results = append(results, c.CleanOrphans()...)
	return results
}

// ResetState cleans loop_count and consecutive_failures
func (c *Resetter) ResetState() []Result {
	var results []Result

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
			results = append(results, Result{
				Name:    name,
				Success: true,
				Message: fmt.Sprintf("Would delete %s", f),
			})
			continue
		}

		if err := os.Remove(f); err != nil {
			results = append(results, Result{
				Name:    name,
				Success: false,
				Message: fmt.Sprintf("Failed to delete: %v", err),
			})
		} else {
			results = append(results, Result{
				Name:    name,
				Success: true,
				Message: "Deleted",
			})
		}
	}

	return results
}

// ResetAttempts cleans attempt tracking files
func (c *Resetter) ResetAttempts() []Result {
	var results []Result

	// Clean legacy attempts directory
	attemptsDir := filepath.Join(c.StateRoot, ".ai", "state", "attempts")
	if _, err := os.Stat(attemptsDir); err == nil {
		if c.DryRun {
			results = append(results, Result{
				Name:    "attempts",
				Success: true,
				Message: fmt.Sprintf("Would delete %s/*", attemptsDir),
			})
		} else if err := os.RemoveAll(attemptsDir); err != nil {
			results = append(results, Result{
				Name:    "attempts",
				Success: false,
				Message: fmt.Sprintf("Failed to delete: %v", err),
			})
		} else {
			results = append(results, Result{
				Name:    "attempts",
				Success: true,
				Message: "Deleted attempts directory",
			})
		}
	}

	// Clean runs directory (contains fail_count.txt per issue)
	runsDir := filepath.Join(c.StateRoot, ".ai", "runs")
	if _, err := os.Stat(runsDir); err == nil {
		if c.DryRun {
			results = append(results, Result{
				Name:    "runs",
				Success: true,
				Message: fmt.Sprintf("Would delete %s/*", runsDir),
			})
		} else if err := os.RemoveAll(runsDir); err != nil {
			results = append(results, Result{
				Name:    "runs",
				Success: false,
				Message: fmt.Sprintf("Failed to delete: %v", err),
			})
		} else {
			results = append(results, Result{
				Name:    "runs",
				Success: true,
				Message: "Deleted runs directory",
			})
		}
	}

	return results
}

// ResetStop removes the STOP marker
func (c *Resetter) ResetStop() Result {
	stopMarker := filepath.Join(c.StateRoot, ".ai", "state", "STOP")

	if _, err := os.Stat(stopMarker); os.IsNotExist(err) {
		return Result{
			Name:    "STOP marker",
			Success: true,
			Message: "Not present",
		}
	}

	if c.DryRun {
		return Result{
			Name:    "STOP marker",
			Success: true,
			Message: fmt.Sprintf("Would delete %s", stopMarker),
		}
	}

	if err := os.Remove(stopMarker); err != nil {
		return Result{
			Name:    "STOP marker",
			Success: false,
			Message: fmt.Sprintf("Failed to delete: %v", err),
		}
	}

	return Result{
		Name:    "STOP marker",
		Success: true,
		Message: "Deleted",
	}
}

// ResetLock removes the lock file
func (c *Resetter) ResetLock() Result {
	lockFile := filepath.Join(c.StateRoot, ".ai", "state", "kickoff.lock")

	if _, err := os.Stat(lockFile); os.IsNotExist(err) {
		return Result{
			Name:    "Lock file",
			Success: true,
			Message: "Not present",
		}
	}

	if c.DryRun {
		return Result{
			Name:    "Lock file",
			Success: true,
			Message: fmt.Sprintf("Would delete %s", lockFile),
		}
	}

	if err := os.Remove(lockFile); err != nil {
		return Result{
			Name:    "Lock file",
			Success: false,
			Message: fmt.Sprintf("Failed to delete: %v", err),
		}
	}

	return Result{
		Name:    "Lock file",
		Success: true,
		Message: "Deleted",
	}
}

// ResetDeprecated removes deprecated files
func (c *Resetter) ResetDeprecated() []Result {
	deprecatedFiles := []string{
		".ai/skills/principal-workflow/tasks/review-pr.md",
		".ai/docs/evaluate.md",
	}

	var results []Result
	for _, f := range deprecatedFiles {
		path := filepath.Join(c.StateRoot, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}

		if c.DryRun {
			results = append(results, Result{
				Name:    f,
				Success: true,
				Message: fmt.Sprintf("Would delete %s", path),
			})
			continue
		}

		if err := os.Remove(path); err != nil {
			results = append(results, Result{
				Name:    f,
				Success: false,
				Message: fmt.Sprintf("Failed to delete: %v", err),
			})
		} else {
			results = append(results, Result{
				Name:    f,
				Success: true,
				Message: "Deleted deprecated file",
			})
		}
	}

	return results
}

// ResetTraces removes old trace files (.ai/state/traces/)
// This is part of the migration to the unified event stream
func (c *Resetter) ResetTraces() []Result {
	tracesDir := filepath.Join(c.StateRoot, ".ai", "state", "traces")

	if _, err := os.Stat(tracesDir); os.IsNotExist(err) {
		return nil
	}

	if c.DryRun {
		return []Result{{
			Name:    "traces",
			Success: true,
			Message: fmt.Sprintf("Would delete %s/*", tracesDir),
		}}
	}

	if err := os.RemoveAll(tracesDir); err != nil {
		return []Result{{
			Name:    "traces",
			Success: false,
			Message: fmt.Sprintf("Failed to delete: %v", err),
		}}
	}

	return []Result{{
		Name:    "traces",
		Success: true,
		Message: "Deleted traces directory (migrated to unified event stream)",
	}}
}

// ResetEvents removes event stream files (.ai/state/events/)
func (c *Resetter) ResetEvents() []Result {
	eventsDir := filepath.Join(c.StateRoot, ".ai", "state", "events")

	if _, err := os.Stat(eventsDir); os.IsNotExist(err) {
		return nil
	}

	if c.DryRun {
		return []Result{{
			Name:    "events",
			Success: true,
			Message: fmt.Sprintf("Would delete %s/*", eventsDir),
		}}
	}

	if err := os.RemoveAll(eventsDir); err != nil {
		return []Result{{
			Name:    "events",
			Success: false,
			Message: fmt.Sprintf("Failed to delete: %v", err),
		}}
	}

	return []Result{{
		Name:    "events",
		Success: true,
		Message: "Deleted events directory",
	}}
}

// Results removes result files
func (c *Resetter) Results() []Result {
	resultsDir := filepath.Join(c.StateRoot, ".ai", "results")

	entries, err := os.ReadDir(resultsDir)
	if err != nil {
		return nil
	}

	var results []Result
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(resultsDir, entry.Name())

		if c.DryRun {
			results = append(results, Result{
				Name:    entry.Name(),
				Success: true,
				Message: fmt.Sprintf("Would delete %s", path),
			})
			continue
		}

		if err := os.Remove(path); err != nil {
			results = append(results, Result{
				Name:    entry.Name(),
				Success: false,
				Message: fmt.Sprintf("Failed to delete: %v", err),
			})
		} else {
			results = append(results, Result{
				Name:    entry.Name(),
				Success: true,
				Message: "Deleted",
			})
		}
	}

	return results
}

// cleanGlob finds files matching a glob pattern and removes them.
// In dry-run mode it only reports what would be deleted.
func (c *Resetter) cleanGlob(pattern, label string) []Result {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return []Result{{
			Name:    label,
			Success: false,
			Message: fmt.Sprintf("Glob error: %v", err),
		}}
	}

	if len(matches) == 0 {
		return nil
	}

	if c.DryRun {
		return []Result{{
			Name:    label,
			Success: true,
			Message: fmt.Sprintf("Would delete %d file(s)", len(matches)),
		}}
	}

	var removed int
	for _, m := range matches {
		if err := os.Remove(m); err == nil {
			removed++
		}
	}

	return []Result{{
		Name:    label,
		Success: true,
		Message: fmt.Sprintf("Deleted %d file(s)", removed),
	}}
}

// CleanTemp removes ticket temp files (.ai/temp/ticket-*.md)
func (c *Resetter) CleanTemp() []Result {
	pattern := filepath.Join(c.StateRoot, ".ai", "temp", "ticket-*.md")
	return c.cleanGlob(pattern, "temp tickets")
}

// CleanSessions removes old session files, keeping the last 5
func (c *Resetter) CleanSessions() []Result {
	dir := filepath.Join(c.StateRoot, ".ai", "state", "principal", "sessions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil // directory doesn't exist, nothing to do
	}

	// Collect .json files with their mod times
	type fileInfo struct {
		path    string
		modTime time.Time
	}
	var files []fileInfo
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, fileInfo{
			path:    filepath.Join(dir, entry.Name()),
			modTime: info.ModTime(),
		})
	}

	const keepCount = 5
	if len(files) <= keepCount {
		return nil
	}

	// Sort by mod time descending (newest first)
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})

	toRemove := files[keepCount:]

	if c.DryRun {
		return []Result{{
			Name:    "sessions",
			Success: true,
			Message: fmt.Sprintf("Would delete %d old session file(s), keeping last %d", len(toRemove), keepCount),
		}}
	}

	var removed int
	for _, f := range toRemove {
		if err := os.Remove(f.path); err == nil {
			removed++
		}
	}

	return []Result{{
		Name:    "sessions",
		Success: true,
		Message: fmt.Sprintf("Deleted %d old session file(s), kept last %d", removed, keepCount),
	}}
}

// CleanReports removes old workflow report files (.ai/state/workflow-report-*.md)
func (c *Resetter) CleanReports() []Result {
	pattern := filepath.Join(c.StateRoot, ".ai", "state", "workflow-report-*.md")
	return c.cleanGlob(pattern, "workflow reports")
}

// CleanOrphans removes orphaned .tmp files in .ai/state/ and .ai/results/
// that are older than 1 hour (to avoid removing active writes)
func (c *Resetter) CleanOrphans() []Result {
	dirs := []string{
		filepath.Join(c.StateRoot, ".ai", "state"),
		filepath.Join(c.StateRoot, ".ai", "results"),
	}

	cutoff := time.Now().Add(-1 * time.Hour)
	var orphans []string

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".tmp" {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if info.ModTime().Before(cutoff) {
				orphans = append(orphans, filepath.Join(dir, entry.Name()))
			}
		}
	}

	if len(orphans) == 0 {
		return nil
	}

	if c.DryRun {
		return []Result{{
			Name:    "orphan .tmp files",
			Success: true,
			Message: fmt.Sprintf("Would delete %d orphaned .tmp file(s) older than 1 hour", len(orphans)),
		}}
	}

	var removed int
	for _, f := range orphans {
		if err := os.Remove(f); err == nil {
			removed++
		}
	}

	return []Result{{
		Name:    "orphan .tmp files",
		Success: true,
		Message: fmt.Sprintf("Deleted %d orphaned .tmp file(s)", removed),
	}}
}

// ResetGitHubLabel resets a label on issues
func (c *Resetter) ResetGitHubLabel(ctx context.Context, fromLabel, toLabel string) []Result {
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
		return []Result{{
			Name:    "GitHub labels",
			Success: false,
			Message: fmt.Sprintf("Failed to list issues: %v", err),
		}}
	}

	var issues []struct {
		Number int `json:"number"`
	}
	if err := json.Unmarshal(output, &issues); err != nil {
		return []Result{{
			Name:    "GitHub labels",
			Success: false,
			Message: fmt.Sprintf("Failed to parse issues: %v", err),
		}}
	}

	if len(issues) == 0 {
		return []Result{{
			Name:    "GitHub labels",
			Success: true,
			Message: fmt.Sprintf("No issues with '%s' label", fromLabel),
		}}
	}

	var results []Result
	for _, issue := range issues {
		if c.DryRun {
			results = append(results, Result{
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
			results = append(results, Result{
				Name:    fmt.Sprintf("Issue #%d", issue.Number),
				Success: false,
				Message: fmt.Sprintf("Failed to update labels: %v", err),
			})
		} else {
			results = append(results, Result{
				Name:    fmt.Sprintf("Issue #%d", issue.Number),
				Success: true,
				Message: fmt.Sprintf("Changed label from '%s' to '%s'", fromLabel, toLabel),
			})
		}
	}

	return results
}
