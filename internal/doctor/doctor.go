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

	"github.com/silver2dream/ai-workflow-kit/internal/config"
	"github.com/silver2dream/ai-workflow-kit/internal/jittest"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
	"github.com/silver2dream/ai-workflow-kit/internal/migrate"
	"github.com/silver2dream/ai-workflow-kit/internal/stopmark"
	"github.com/silver2dream/ai-workflow-kit/internal/upgrade"
)

const (
	ticketFileWarnThreshold  = 10
	sessionFileWarnThreshold = 20
	orphanTmpMaxAge          = time.Hour
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

	// 0. Is this a project awkit can work in at all
	results = append(results, d.CheckLayout()...)

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

	// 4. Check state file accumulation
	results = append(results, d.CheckTempTickets()...)
	results = append(results, d.CheckSessionFiles()...)
	results = append(results, d.CheckOrphanTmpFiles()...)

	// 5. Check config version
	results = append(results, d.CheckConfigVersion()...)

	// 6. Check Claude settings permissions
	results = append(results, d.CheckClaudeSettings()...)

	// 7. Check JiTTest calibration suggestion
	results = append(results, d.CheckJiTTestCalibration()...)

	return results
}

// CheckLayout says when the project still keeps files where an earlier awkit
// put them. Nothing is read from there, so most commands do not work until
// `awkit upgrade` has moved them.
func (d *Doctor) CheckLayout() []CheckResult {
	if !migrate.UsesOldLayout(d.StateRoot) {
		return nil
	}
	return []CheckResult{{
		Name:    "Layout",
		Status:  "error",
		Message: "the project keeps files where an earlier awkit put them; run 'awkit upgrade' to move them ('awkit upgrade --dry-run' shows every step first)",
	}}
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
	if reason, present := stopmark.Read(d.StateRoot); present {
		message := "a STOP marker is there; the next run clears it"
		if reason != "" {
			message = fmt.Sprintf("the last run stopped (%s); the next run clears its STOP marker", reason)
		}
		return CheckResult{
			Name:     "Stop Marker",
			Status:   "ok",
			Message:  message,
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
	lockFile := filepath.Join(d.StateRoot, ".ai", "state", "kickoff.lock")
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
		name        string
		desc        string
		canReset    bool
		resetTo     string
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
		layout.SkillsDir + "/principal-workflow/tasks/review-pr.md",
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

// CheckTempTickets warns if there are too many ticket files in .ai/temp/
func (d *Doctor) CheckTempTickets() []CheckResult {
	pattern := filepath.Join(layout.Temp(d.StateRoot), "ticket-*.md")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return nil
	}

	if len(matches) > ticketFileWarnThreshold {
		return []CheckResult{{
			Name:     "Temp Tickets",
			Status:   "warning",
			Message:  fmt.Sprintf("%d ticket file(s) in "+layout.TempDir+"/ (run 'awkit reset --temp' to clean)", len(matches)),
			CanClean: true,
			CleanKey: "temp",
		}}
	}
	return nil
}

// CheckSessionFiles warns if there are too many session files
func (d *Doctor) CheckSessionFiles() []CheckResult {
	dir := filepath.Join(d.StateRoot, ".ai", "state", "principal", "sessions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var count int
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			count++
		}
	}

	if count > sessionFileWarnThreshold {
		return []CheckResult{{
			Name:     "Session Files",
			Status:   "warning",
			Message:  fmt.Sprintf("%d session file(s) in .ai/state/principal/sessions/ (run 'awkit reset --sessions' to clean)", count),
			CanClean: true,
			CleanKey: "sessions",
		}}
	}
	return nil
}

// CheckOrphanTmpFiles warns if there are orphaned .tmp files older than 1 hour
func (d *Doctor) CheckOrphanTmpFiles() []CheckResult {
	dirs := []string{
		filepath.Join(d.StateRoot, ".ai", "state"),
		layout.Results(d.StateRoot),
	}

	cutoff := time.Now().Add(-orphanTmpMaxAge)
	var count int

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
				count++
			}
		}
	}

	if count > 0 {
		return []CheckResult{{
			Name:     "Orphan .tmp Files",
			Status:   "warning",
			Message:  fmt.Sprintf("%d orphaned .tmp file(s) older than 1 hour (run 'awkit reset --orphans' to clean)", count),
			CanClean: true,
			CleanKey: "orphans",
		}}
	}
	return nil
}

// CheckClaudeSettings checks if settings.local.json has required Task tool permissions
func (d *Doctor) CheckClaudeSettings() []CheckResult {
	settingsPath := filepath.Join(d.StateRoot, ".claude", "settings.local.json")
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		return []CheckResult{{
			Name:     "Claude Settings",
			Status:   "warning",
			Message:  ".claude/settings.local.json not found (run 'awkit generate')",
			CanClean: false,
		}}
	}

	missing := upgrade.CheckPermissions(d.StateRoot)
	if len(missing) > 0 {
		return []CheckResult{{
			Name:     "Claude Settings",
			Status:   "warning",
			Message:  fmt.Sprintf("missing permissions: %s (run 'awkit upgrade')", strings.Join(missing, ", ")),
			CanClean: true,
			CleanKey: "upgrade",
		}}
	}

	return nil
}

// CheckJiTTestCalibration checks if JiTTest stats suggest upgrading failure_policy to "block"
func (d *Doctor) CheckJiTTestCalibration() []CheckResult {
	stats, err := jittest.LoadStats(d.StateRoot)
	if err != nil || stats.TotalRuns == 0 {
		return nil
	}

	if stats.ShouldSuggestBlock() {
		return []CheckResult{{
			Name:   "JiTTest Calibration",
			Status: "warning",
			Message: fmt.Sprintf(
				"JiTTest FP rate %.1f%% across %d runs. Consider upgrading failure_policy to \"block\" in "+layout.ConfigFile+".",
				stats.FalsePositiveRate(), stats.TotalRuns,
			),
		}}
	}
	return nil
}

// CheckConfigVersion checks if workflow.yaml needs migration to a newer version
func (d *Doctor) CheckConfigVersion() []CheckResult {
	configPath := config.Path(d.StateRoot)
	currentVersion, needed, err := migrate.NeedsMigration(configPath)
	if err != nil {
		// Config not found or unreadable — not an error for doctor
		return nil
	}
	if !needed {
		return nil
	}

	return []CheckResult{{
		Name:   "Config Version",
		Status: "warning",
		Message: fmt.Sprintf(
			layout.ConfigFile+" is version %s (latest: %s). Run 'awkit upgrade' to migrate.",
			currentVersion, migrate.LatestVersion,
		),
	}}
}
