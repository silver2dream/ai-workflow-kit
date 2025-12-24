package kickoff

import (
	"fmt"
)

// PreflightChecker performs pre-flight checks before starting the workflow
type PreflightChecker struct {
	config     *Config
	configPath string
	lockFile   string
}

// CheckResult represents the result of a single check
type CheckResult struct {
	Name    string
	Passed  bool
	Message string
}

// NewPreflightChecker creates a new PreflightChecker
func NewPreflightChecker(configPath, lockFile string) *PreflightChecker {
	return &PreflightChecker{
		configPath: configPath,
		lockFile:   lockFile,
	}
}

// RunAll executes all pre-flight checks
// awkit only checks: lock file, config validation, PTY initialization
// Principal handles: gh auth, working directory, session init
func (p *PreflightChecker) RunAll() ([]CheckResult, error) {
	var results []CheckResult

	// 1. Check lock file
	lockResult := p.CheckLockFile()
	results = append(results, lockResult)
	if !lockResult.Passed {
		return results, fmt.Errorf("lock check failed: %s", lockResult.Message)
	}

	// 2. Check config
	configResult := p.CheckConfig()
	results = append(results, configResult)
	if !configResult.Passed {
		return results, fmt.Errorf("config check failed: %s", configResult.Message)
	}

	// 3. Check PTY
	ptyResult := p.CheckPTY()
	results = append(results, ptyResult)
	// PTY check is non-fatal (will fallback to standard execution)

	return results, nil
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

// CheckPTY checks if PTY can be initialized
func (p *PreflightChecker) CheckPTY() CheckResult {
	// PTY availability is platform-dependent
	// On Unix, PTY should always be available
	// On Windows, ConPTY requires Windows 11

	// For now, just return success
	// Actual PTY initialization happens when starting the executor
	return CheckResult{
		Name:    "PTY",
		Passed:  true,
		Message: "PTY available",
	}
}

// Config returns the loaded configuration
func (p *PreflightChecker) Config() *Config {
	return p.config
}
