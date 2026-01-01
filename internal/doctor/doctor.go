package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// CheckResult represents the result of a single check
type CheckResult struct {
	Name     string
	Status   string // "ok", "warning", "error"
	Message  string
	CanClean bool   // true if this can be cleaned up
	CleanKey string // key for clean command
}

// Doctor performs health checks on the AWK project
type Doctor struct {
	StateRoot string
	Timeout   time.Duration
}

// New creates a new Doctor
func New(stateRoot string) *Doctor {
	if stateRoot == "" {
		stateRoot = "."
	}
	return &Doctor{
		StateRoot: stateRoot,
		Timeout:   30 * time.Second,
	}
}

// RunAll executes all health checks
func (d *Doctor) RunAll(ctx context.Context) []CheckResult {
	var results []CheckResult

	// 1. Check local state files
	results = append(results, d.CheckLoopCount()...)
	results = append(results, d.CheckConsecutiveFailures()...)
	results = append(results, d.CheckAttempts()...)
	results = append(results, d.CheckStopMarker())
	results = append(results, d.CheckLockFile())

	// 2. Check GitHub labels
	results = append(results, d.CheckGitHubLabels(ctx)...)

	// 3. Check deprecated files
	results = append(results, d.CheckDeprecatedFiles()...)

	return results
}

// CheckLoopCount checks the loop count state
func (d *Doctor) CheckLoopCount() []CheckResult {
	loopFile := filepath.Join(d.StateRoot, ".ai", "state", "loop_count")
	data, err := os.ReadFile(loopFile)
	if err != nil {
		return nil // file doesn't exist, ok
	}

	count, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	if count > 0 {
		return []CheckResult{{
			Name:     "Loop Count",
			Status:   "warning",
			Message:  fmt.Sprintf("loop_count = %d (previous session state)", count),
			CanClean: true,
			CleanKey: "state",
		}}
	}
	return nil
}

// CheckConsecutiveFailures checks the consecutive failures count
func (d *Doctor) CheckConsecutiveFailures() []CheckResult {
	failFile := filepath.Join(d.StateRoot, ".ai", "state", "consecutive_failures")
	data, err := os.ReadFile(failFile)
	if err != nil {
		return nil
	}

	count, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	if count > 0 {
		return []CheckResult{{
			Name:     "Consecutive Failures",
			Status:   "warning",
			Message:  fmt.Sprintf("consecutive_failures = %d", count),
			CanClean: true,
			CleanKey: "state",
		}}
	}
	return nil
}

// CheckAttempts checks for attempt tracking files
func (d *Doctor) CheckAttempts() []CheckResult {
	attemptsDir := filepath.Join(d.StateRoot, ".ai", "state", "attempts")
	entries, err := os.ReadDir(attemptsDir)
	if err != nil {
		return nil
	}

	var results []CheckResult
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(attemptsDir, entry.Name()))
		if err != nil {
			continue
		}
		count, _ := strconv.Atoi(strings.TrimSpace(string(data)))
		if count > 0 {
			results = append(results, CheckResult{
				Name:     "Attempt Tracking",
				Status:   "warning",
				Message:  fmt.Sprintf("%s = %d attempts", entry.Name(), count),
				CanClean: true,
				CleanKey: "attempts",
			})
		}
	}
	return results
}

// CheckStopMarker checks for STOP marker
func (d *Doctor) CheckStopMarker() CheckResult {
	stopMarker := filepath.Join(d.StateRoot, ".ai", "state", "STOP")
	if _, err := os.Stat(stopMarker); err == nil {
		return CheckResult{
			Name:     "Stop Marker",
			Status:   "warning",
			Message:  "STOP marker exists (workflow was stopped)",
			CanClean: true,
			CleanKey: "stop",
		}
	}
	return CheckResult{
		Name:    "Stop Marker",
		Status:  "ok",
		Message: "No stop marker",
	}
}

// CheckLockFile checks for stale lock file
func (d *Doctor) CheckLockFile() CheckResult {
	lockFile := filepath.Join(d.StateRoot, ".ai", "state", "principal.lock")
	info, err := os.Stat(lockFile)
	if err != nil {
		return CheckResult{
			Name:    "Lock File",
			Status:  "ok",
			Message: "No lock file",
		}
	}

	// Check if lock is stale (older than 1 hour)
	if time.Since(info.ModTime()) > time.Hour {
		return CheckResult{
			Name:     "Lock File",
			Status:   "warning",
			Message:  fmt.Sprintf("Stale lock file (age: %v)", time.Since(info.ModTime()).Round(time.Minute)),
			CanClean: true,
			CleanKey: "lock",
		}
	}

	return CheckResult{
		Name:    "Lock File",
		Status:  "error",
		Message: "Lock file exists (another instance may be running)",
	}
}

// CheckGitHubLabels checks for issues with problematic labels
func (d *Doctor) CheckGitHubLabels(ctx context.Context) []CheckResult {
	var results []CheckResult

	labels := []struct {
		name       string
		desc       string
		canReset   bool
		resetTo    string
		humanAction string
	}{
		{"needs-human-review", "Issues requiring human intervention", false, "", "manually review and close/merge"},
		{"review-failed", "Issues with failed review", true, "pr-ready", ""},
		{"worker-failed", "Issues with failed worker", false, "", "investigate and retry or close"},
		{"in-progress", "Issues currently in progress", false, "", "wait for completion or manually reset"},
	}

	for _, label := range labels {
		count, issues := d.countIssuesWithLabel(ctx, label.name)
		if count > 0 {
			status := "warning"
			message := fmt.Sprintf("%d issue(s) with '%s' label", count, label.name)
			if len(issues) > 0 && len(issues) <= 3 {
				message += fmt.Sprintf(": #%s", strings.Join(issues, ", #"))
			}

			if label.canReset {
				message += fmt.Sprintf(" [can reset: awkit reset --labels]")
			} else if label.humanAction != "" {
				message += fmt.Sprintf(" [action: %s]", label.humanAction)
			}

			results = append(results, CheckResult{
				Name:     "GitHub: " + label.desc,
				Status:   status,
				Message:  message,
				CanClean: label.canReset,
				CleanKey: "label:" + label.name,
			})
		}
	}

	return results
}

func (d *Doctor) countIssuesWithLabel(ctx context.Context, label string) (int, []string) {
	ctx, cancel := context.WithTimeout(ctx, d.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "issue", "list",
		"--label", label,
		"--state", "open",
		"--json", "number",
		"--limit", "10")

	output, err := cmd.Output()
	if err != nil {
		return 0, nil
	}

	var issues []struct {
		Number int `json:"number"`
	}
	if err := json.Unmarshal(output, &issues); err != nil {
		return 0, nil
	}

	var nums []string
	for _, issue := range issues {
		nums = append(nums, strconv.Itoa(issue.Number))
	}

	return len(issues), nums
}

// CheckDeprecatedFiles checks for deprecated files that should be removed
func (d *Doctor) CheckDeprecatedFiles() []CheckResult {
	deprecatedFiles := []string{
		".ai/skills/principal-workflow/tasks/review-pr.md",
		".ai/docs/evaluate.md",
	}

	var results []CheckResult
	for _, f := range deprecatedFiles {
		path := filepath.Join(d.StateRoot, f)
		if _, err := os.Stat(path); err == nil {
			results = append(results, CheckResult{
				Name:     "Deprecated File",
				Status:   "warning",
				Message:  fmt.Sprintf("%s (should be removed)", f),
				CanClean: true,
				CleanKey: "deprecated",
			})
		}
	}

	return results
}
