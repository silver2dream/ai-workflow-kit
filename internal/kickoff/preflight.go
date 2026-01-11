package kickoff

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/upgrade"
)

// PreflightChecker performs pre-flight checks before starting the workflow
type PreflightChecker struct {
	config      *Config
	configPath  string
	lockFile    string
	stopMarker  string
	forceDelete bool // auto-delete STOP marker
}

// AuditSummary represents the summary from audit.json
type AuditSummary struct {
	P0 int `json:"p0"`
	P1 int `json:"p1"`
}

// AuditFinding represents a finding from audit.json
type AuditFinding struct {
	Severity string `json:"severity"`
	Title    string `json:"title"`
}

// AuditResult represents the audit.json structure
type AuditResult struct {
	Summary  AuditSummary   `json:"summary"`
	Findings []AuditFinding `json:"findings"`
}

// CheckResult represents the result of a single check
type CheckResult struct {
	Name    string
	Passed  bool
	Message string
	Warning bool // true if this is a warning (passed but with concerns)
}

// NewPreflightChecker creates a new PreflightChecker
func NewPreflightChecker(configPath, lockFile string) *PreflightChecker {
	return &PreflightChecker{
		configPath: configPath,
		lockFile:   lockFile,
		stopMarker: filepath.Join(".ai", "state", "STOP"),
	}
}

// SetForceDelete enables auto-deletion of STOP marker
func (p *PreflightChecker) SetForceDelete(force bool) {
	p.forceDelete = force
}

// RunAll executes all pre-flight checks
func (p *PreflightChecker) RunAll() ([]CheckResult, error) {
	var results []CheckResult

	// 1. Check gh CLI installed
	ghResult := p.CheckGhCLI()
	results = append(results, ghResult)
	if !ghResult.Passed {
		return results, fmt.Errorf("gh CLI check failed: %s", ghResult.Message)
	}

	// 2. Check gh auth
	ghAuthResult := p.CheckGhAuth()
	results = append(results, ghAuthResult)
	if !ghAuthResult.Passed {
		return results, fmt.Errorf("gh auth check failed: %s", ghAuthResult.Message)
	}

	// 3. Check claude CLI installed
	claudeResult := p.CheckClaudeCLI()
	results = append(results, claudeResult)
	if !claudeResult.Passed {
		return results, fmt.Errorf("claude CLI check failed: %s", claudeResult.Message)
	}

	// 4. Check codex CLI (warning only)
	codexResult := p.CheckCodexCLI()
	results = append(results, codexResult)
	// Non-fatal, just warning

	// 5. Check working directory is clean
	gitResult := p.CheckGitClean()
	results = append(results, gitResult)
	if !gitResult.Passed {
		return results, fmt.Errorf("git check failed: %s", gitResult.Message)
	}

	// 6. Check STOP marker
	stopResult := p.CheckStopMarker()
	results = append(results, stopResult)
	if !stopResult.Passed {
		return results, fmt.Errorf("stop marker check failed: %s", stopResult.Message)
	}

	// 7. Check lock file
	lockResult := p.CheckLockFile()
	results = append(results, lockResult)
	if !lockResult.Passed {
		return results, fmt.Errorf("lock check failed: %s", lockResult.Message)
	}

	// 8. Check config
	configResult := p.CheckConfig()
	results = append(results, configResult)
	if !configResult.Passed {
		return results, fmt.Errorf("config check failed: %s", configResult.Message)
	}

	// 9. Check PTY
	ptyResult := p.CheckPTY()
	results = append(results, ptyResult)
	// PTY check is non-fatal (will fallback to standard execution)

	// 10. Check permissions (warning only, non-fatal)
	permResult := p.CheckPermissions()
	results = append(results, permResult)
	// Permissions check is non-fatal but important warning

	return results, nil
}

// CheckGhCLI checks if gh CLI is installed
func (p *PreflightChecker) CheckGhCLI() CheckResult {
	_, err := exec.LookPath("gh")
	if err != nil {
		return CheckResult{
			Name:    "gh CLI",
			Passed:  false,
			Message: "gh CLI not installed. Run: brew install gh (macOS) or see https://cli.github.com/",
		}
	}
	return CheckResult{
		Name:    "gh CLI",
		Passed:  true,
		Message: "gh CLI installed",
	}
}

// CheckGhAuth checks if gh is authenticated
func (p *PreflightChecker) CheckGhAuth() CheckResult {
	cmd := exec.Command("gh", "auth", "status")
	if err := cmd.Run(); err != nil {
		return CheckResult{
			Name:    "gh Auth",
			Passed:  false,
			Message: "gh not authenticated. Run: gh auth login",
		}
	}
	return CheckResult{
		Name:    "gh Auth",
		Passed:  true,
		Message: "gh authenticated",
	}
}

// CheckClaudeCLI checks if claude CLI is installed
func (p *PreflightChecker) CheckClaudeCLI() CheckResult {
	_, err := exec.LookPath("claude")
	if err != nil {
		return CheckResult{
			Name:    "claude CLI",
			Passed:  false,
			Message: "claude CLI not installed. Ensure Claude Code is installed and in PATH.",
		}
	}
	return CheckResult{
		Name:    "claude CLI",
		Passed:  true,
		Message: "claude CLI installed",
	}
}

// CheckCodexCLI checks if codex CLI is installed (warning only)
func (p *PreflightChecker) CheckCodexCLI() CheckResult {
	_, err := exec.LookPath("codex")
	if err != nil {
		return CheckResult{
			Name:    "codex CLI",
			Passed:  true, // Non-fatal
			Message: "codex CLI not installed (worker execution may fail)",
			Warning: true,
		}
	}
	return CheckResult{
		Name:    "codex CLI",
		Passed:  true,
		Message: "codex CLI installed",
	}
}

// CheckGitClean checks if working directory is clean
func (p *PreflightChecker) CheckGitClean() CheckResult {
	cmd := exec.Command("git", "status", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return CheckResult{
			Name:    "Git Status",
			Passed:  false,
			Message: fmt.Sprintf("Failed to check git status: %v", err),
		}
	}

	if len(strings.TrimSpace(string(output))) > 0 {
		return CheckResult{
			Name:    "Git Status",
			Passed:  false,
			Message: "Working directory not clean. Please commit or stash changes.",
		}
	}

	return CheckResult{
		Name:    "Git Status",
		Passed:  true,
		Message: "Working directory clean",
	}
}

// CheckStopMarker checks if STOP marker exists
func (p *PreflightChecker) CheckStopMarker() CheckResult {
	if _, err := os.Stat(p.stopMarker); err == nil {
		// STOP marker exists
		if p.forceDelete {
			// Auto-delete in force mode
			if err := os.Remove(p.stopMarker); err != nil {
				return CheckResult{
					Name:    "Stop Marker",
					Passed:  false,
					Message: fmt.Sprintf("Failed to delete STOP marker: %v", err),
				}
			}
			return CheckResult{
				Name:    "Stop Marker",
				Passed:  true,
				Message: "STOP marker auto-deleted (--force mode)",
				Warning: true,
			}
		}
		return CheckResult{
			Name:    "Stop Marker",
			Passed:  false,
			Message: "Found stop marker .ai/state/STOP. Use --force to auto-delete, or manually delete and retry.",
		}
	}

	return CheckResult{
		Name:    "Stop Marker",
		Passed:  true,
		Message: "No stop marker",
	}
}

// CheckLockFile checks if another instance is running
func (p *PreflightChecker) CheckLockFile() CheckResult {
	lock := NewLockManager(p.lockFile)

	// Check if lock exists and is not stale
	if lock.IsStale() {
		return CheckResult{
			Name:    "Lock File",
			Passed:  true,
			Message: "Stale lock detected, will be cleaned up",
		}
	}

	// Try to acquire (this will fail if another instance is running)
	if err := lock.Acquire(); err != nil {
		return CheckResult{
			Name:    "Lock File",
			Passed:  false,
			Message: err.Error(),
		}
	}

	// Release immediately (actual acquisition happens in Run)
	lock.Release()

	return CheckResult{
		Name:    "Lock File",
		Passed:  true,
		Message: "No other instance running",
	}
}

// CheckConfig validates the workflow configuration
func (p *PreflightChecker) CheckConfig() CheckResult {
	config, err := LoadConfig(p.configPath)
	if err != nil {
		return CheckResult{
			Name:    "Config",
			Passed:  false,
			Message: err.Error(),
		}
	}

	// Validate required fields
	errors := config.Validate()
	if len(errors) > 0 {
		return CheckResult{
			Name:    "Config",
			Passed:  false,
			Message: errors[0].Error(),
		}
	}

	// Store config for later use
	p.config = config

	return CheckResult{
		Name:    "Config",
		Passed:  true,
		Message: fmt.Sprintf("Valid config for project: %s", config.Project.Name),
	}
}

// CheckPTY checks if PTY can be initialized (G12 fix: actual detection)
func (p *PreflightChecker) CheckPTY() CheckResult {
	switch runtime.GOOS {
	case "windows":
		// ConPTY requires Windows 10 version 1809+ or Windows 11
		// Check if conhost.exe exists (basic sanity check)
		conhost := filepath.Join(os.Getenv("SystemRoot"), "System32", "conhost.exe")
		if _, err := os.Stat(conhost); err != nil {
			return CheckResult{
				Name:    "PTY",
				Passed:  true,
				Warning: true,
				Message: "ConPTY may not be available (conhost.exe not found)",
			}
		}
		return CheckResult{
			Name:    "PTY",
			Passed:  true,
			Message: "PTY available (Windows ConPTY)",
		}

	case "linux", "darwin", "freebsd", "openbsd", "netbsd":
		// Unix systems have PTY support via /dev/ptmx or openpty()
		if _, err := os.Stat("/dev/ptmx"); err == nil {
			return CheckResult{
				Name:    "PTY",
				Passed:  true,
				Message: "PTY available (Unix /dev/ptmx)",
			}
		}
		// Fallback check for /dev/pty*
		matches, _ := filepath.Glob("/dev/pty*")
		if len(matches) > 0 {
			return CheckResult{
				Name:    "PTY",
				Passed:  true,
				Message: "PTY available (Unix /dev/pty*)",
			}
		}
		return CheckResult{
			Name:    "PTY",
			Passed:  true,
			Warning: true,
			Message: "PTY device not found, using fallback execution",
		}

	default:
		return CheckResult{
			Name:    "PTY",
			Passed:  true,
			Warning: true,
			Message: fmt.Sprintf("PTY support unknown for %s, using fallback", runtime.GOOS),
		}
	}
}

// CheckPermissions checks if settings.local.json has required Task tool permissions
func (p *PreflightChecker) CheckPermissions() CheckResult {
	// Get state root from config path: .ai/config/workflow.yaml -> .ai/config -> .ai -> root
	stateRoot := filepath.Dir(filepath.Dir(filepath.Dir(p.configPath)))

	missing := upgrade.CheckPermissions(stateRoot)
	if len(missing) == 0 {
		return CheckResult{
			Name:    "Permissions",
			Passed:  true,
			Message: "All required permissions present",
		}
	}

	return CheckResult{
		Name:    "Permissions",
		Passed:  true, // Non-fatal, workflow can continue
		Warning: true,
		Message: fmt.Sprintf("Missing: %v. Run 'awkit upgrade' to fix.", missing),
	}
}

// Config returns the loaded configuration
func (p *PreflightChecker) Config() *Config {
	return p.config
}
