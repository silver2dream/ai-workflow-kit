package kickoff

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/config"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
	"github.com/silver2dream/ai-workflow-kit/internal/stopmark"
	"github.com/silver2dream/ai-workflow-kit/internal/upgrade"
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
	Warning bool // true if this is a warning (passed but with concerns)
}

// NewPreflightChecker creates a new PreflightChecker
func NewPreflightChecker(configPath, lockFile string) *PreflightChecker {
	return &PreflightChecker{
		configPath: configPath,
		lockFile:   lockFile,
	}
}

// preflightStep is one check of a run: what is checked, and whether the run
// stops when the check fails. What stops it is named in the error.
type preflightStep struct {
	name  string
	check func() CheckResult
	fatal bool
}

// steps lists the checks of a run in the order they are made. A check after
// one that stops the run is not made: without `gh` there is no asking it who
// is logged in.
func (p *PreflightChecker) steps() []preflightStep {
	return []preflightStep{
		{"gh CLI", p.CheckGhCLI, true},
		{"gh auth", p.CheckGhAuth, true},
		{"claude CLI", p.CheckClaudeCLI, true},
		// The Worker's backend: a warning here. `awkit doctor` calls it what
		// it is, a problem.
		{"worker backend", p.CheckCodexCLI, false},
		// A repository with a remote: the workflow lives in GitHub issues and
		// pull requests, and a Worker pushes its branch to origin.
		{"git remote", p.CheckGitRemote, true},
		{"git", p.CheckGitClean, true},
		// The marker of the last run: said, and cleared when the run starts.
		{"stop marker", p.CheckStopMarker, false},
		{"lock", p.CheckLockFile, true},
		{"config", p.CheckConfig, true},
		{"repo paths", p.CheckRepoPaths, true},
		// Every ticket is branched from the integration branch and its pull
		// request is opened against it.
		{"integration branch", p.CheckIntegrationBranch, true},
		{"GitHub repo", p.CheckGitHubRepo, false},
		// A read-only token passes `gh auth status` but 403s on every
		// state-changing call — create issue, approve/merge PR, move labels —
		// which silently spins the workflow loop for dozens of sessions.
		{"repo write access", p.CheckRepoWritePermission, true},
		// Without a PTY the run falls back to standard execution.
		{"PTY", p.CheckPTY, false},
		{"permissions", p.CheckPermissions, false},
		{"agents", p.CheckAgents, false},
	}
}

// RunAll makes the checks of a run and returns what each one said. It stops
// at the first check that fails and stops the run, and says which.
func (p *PreflightChecker) RunAll() ([]CheckResult, error) {
	return runSteps(p.steps())
}

func runSteps(steps []preflightStep) ([]CheckResult, error) {
	var results []CheckResult
	for _, step := range steps {
		result := step.check()
		results = append(results, result)
		if step.fatal && !result.Passed {
			return results, fmt.Errorf("%s check failed: %s", step.name, result.Message)
		}
	}
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

// CheckCodexCLI checks if the configured worker backend CLI is installed (warning only).
// It reads worker.backend from config and checks the corresponding binary.
func (p *PreflightChecker) CheckCodexCLI() CheckResult {
	// This check runs before the one of the configuration. A file that
	// cannot be read is reported there; here it only fails to name a backend,
	// and the default one is checked.
	backend := config.Default().Worker.Backend
	if cfg := p.config; cfg != nil {
		backend = cfg.Worker.Backend
	} else if cfg, err := config.LoadFile(p.configPath); err == nil {
		backend = cfg.Worker.Backend
	}

	var binaryName, displayName string
	switch backend {
	case "claude-code":
		binaryName = "claude"
		displayName = "claude CLI (worker backend: claude-code)"
	default:
		binaryName = "codex"
		displayName = "codex CLI (worker backend: codex)"
	}

	_, err := exec.LookPath(binaryName)
	if err != nil {
		return CheckResult{
			Name:    displayName,
			Passed:  true, // Non-fatal
			Message: fmt.Sprintf("%s not installed (worker execution may fail)", binaryName),
			Warning: true,
		}
	}
	return CheckResult{
		Name:    displayName,
		Passed:  true,
		Message: fmt.Sprintf("%s installed", binaryName),
	}
}

// CheckGitRemote checks that the project is a git repository with a remote
// named origin. Every ticket's branch is pushed there and its pull request is
// opened there.
func (p *PreflightChecker) CheckGitRemote() CheckResult {
	const name = "Git Remote"
	if err := exec.Command("git", "rev-parse", "--git-dir").Run(); err != nil {
		return CheckResult{
			Name:    name,
			Message: "this is not a git repository. Make it one (git init), commit, and add the GitHub repository as its remote: git remote add origin <url>",
		}
	}
	out, err := exec.Command("git", "remote", "get-url", "origin").Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		return CheckResult{
			Name:    name,
			Message: "the repository has no remote named origin. Tickets are GitHub issues and a Worker pushes its branch there: git remote add origin <url of the GitHub repository>",
		}
	}
	return CheckResult{Name: name, Passed: true, Message: "origin is " + strings.TrimSpace(string(out))}
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

// CheckStopMarker says when the last run left its STOP marker behind. The
// marker ends the run that is going. It does not stand in the way of the next
// one: every run ends with it, a finished one too, and a run that refused to
// start over it had to be forced each time. The run clears it when it starts;
// the check changes nothing, so that a dry run leaves the project as it is.
func (p *PreflightChecker) CheckStopMarker() CheckResult {
	reason, present := stopmark.Read(".")
	switch {
	case !present:
		return CheckResult{Name: "Stop Marker", Passed: true, Message: "No stop marker"}
	case reason == "":
		return CheckResult{Name: "Stop Marker", Passed: true, Warning: true, Message: "a STOP marker is there; a run clears it when it starts"}
	default:
		return CheckResult{Name: "Stop Marker", Passed: true, Warning: true, Message: fmt.Sprintf("the last run stopped (%s); a run clears its STOP marker when it starts", reason)}
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

// CheckConfig validates the workflow configuration. A configuration that is
// valid and holds keys nothing reads passes, and the check says that there
// are such keys: whoever set one takes it to be in force.
func (p *PreflightChecker) CheckConfig() CheckResult {
	cfg, err := LoadConfig(p.configPath)
	if err != nil {
		return CheckResult{
			Name:    "Config",
			Passed:  false,
			Message: err.Error(),
		}
	}

	// Validate required fields
	errors := cfg.Validate()
	if len(errors) > 0 {
		return CheckResult{
			Name:    "Config",
			Passed:  false,
			Message: errors[0].Error(),
		}
	}

	// Store config for later use
	p.config = cfg

	message := fmt.Sprintf("Valid config for project: %s", cfg.Project.Name)
	if unread, err := config.UnreadKeys(p.configPath); err == nil && len(unread) > 0 {
		keys := "keys are"
		if len(unread) == 1 {
			keys = "key is"
		}
		message += fmt.Sprintf(" (%d %s read by nothing: awkit validate names them)", len(unread), keys)
	}
	return CheckResult{
		Name:    "Config",
		Passed:  true,
		Message: message,
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
	stateRoot := layout.ProjectOf(p.configPath)

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

// CheckAgents checks if .claude/agents/ has required agent definitions
func (p *PreflightChecker) CheckAgents() CheckResult {
	stateRoot := layout.ProjectOf(p.configPath)

	missing := upgrade.CheckAgents(stateRoot)
	if len(missing) == 0 {
		return CheckResult{
			Name:    "Agents",
			Passed:  true,
			Message: "All required agent definitions present",
		}
	}

	return CheckResult{
		Name:    "Agents",
		Passed:  true, // Non-fatal, workflow can continue but Task tool won't work
		Warning: true,
		Message: fmt.Sprintf("Missing: %v. Run 'awkit upgrade' to fix.", missing),
	}
}

// baseDir returns the root of the project the configuration belongs to.
func (p *PreflightChecker) baseDir() string {
	d := layout.ProjectOf(p.configPath)
	if d == "." || d == "" {
		d, _ = os.Getwd()
	}
	return d
}

// CheckRepoPaths validates that all configured repo paths exist on disk
func (p *PreflightChecker) CheckRepoPaths() CheckResult {
	if p.config == nil {
		return CheckResult{
			Name:    "Repo Paths",
			Passed:  false,
			Message: "config not loaded (run CheckConfig first)",
		}
	}

	errs := p.config.ValidatePaths(p.baseDir())
	if len(errs) > 0 {
		msgs := make([]string, len(errs))
		for i, e := range errs {
			msgs[i] = e.Error()
		}
		return CheckResult{
			Name:    "Repo Paths",
			Passed:  false,
			Message: fmt.Sprintf("repo path errors: %s", strings.Join(msgs, "; ")),
		}
	}

	return CheckResult{
		Name:    "Repo Paths",
		Passed:  true,
		Message: "all repo paths exist",
	}
}

// remoteHasBranch asks origin whether it has a branch. known is false when it
// could not be asked: no remote of that name, or no way to reach it.
var remoteHasBranch = func(branch string) (found, known bool) {
	err := exec.Command("git", "ls-remote", "--exit-code", "--heads", "origin", "refs/heads/"+branch).Run()
	if err == nil {
		return true, true
	}
	// git ls-remote --exit-code ends with 2 when the remote answered and has
	// no such ref.
	if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 2 {
		return false, true
	}
	return false, false
}

// CheckIntegrationBranch checks that the branch pull requests go to is on the
// remote. A Worker's worktree is branched from it and the Worker's pull
// request is opened against it, so every ticket fails while it is not there.
func (p *PreflightChecker) CheckIntegrationBranch() CheckResult {
	const name = "Integration Branch"
	if p.config == nil {
		return CheckResult{
			Name:    name,
			Passed:  true,
			Warning: true,
			Message: "config not loaded, skipping branch check",
		}
	}

	branch := p.config.Git.IntegrationBranch
	if branch == "" {
		return CheckResult{
			Name:    name,
			Passed:  true,
			Warning: true,
			Message: "no integration branch configured",
		}
	}

	here := exec.Command("git", "rev-parse", "--verify", "--quiet", "refs/heads/"+branch).Run() == nil
	found, known := remoteHasBranch(branch)
	switch {
	case found:
		return CheckResult{Name: name, Passed: true, Message: fmt.Sprintf("pull requests go to %q", branch)}
	case known && here:
		return CheckResult{
			Name:    name,
			Message: fmt.Sprintf("branch %q is not on origin, and a ticket's pull request is opened against it. Push it: git push -u origin %s", branch, branch),
		}
	case known:
		return CheckResult{
			Name: name,
			Message: fmt.Sprintf("branch %q does not exist, and every ticket is branched from it. Create it (git branch %s <the branch to start from> && git push -u origin %s), or set git.integration_branch in %s to the branch pull requests go to",
				branch, branch, branch, layout.ConfigFile),
		}
	case here:
		return CheckResult{
			Name:    name,
			Passed:  true,
			Warning: true,
			Message: fmt.Sprintf("origin could not be asked for branch %q; it is here, and has to be on origin before a ticket is worked on", branch),
		}
	default:
		return CheckResult{
			Name:    name,
			Passed:  true,
			Warning: true,
			Message: fmt.Sprintf("branch %q is not here, and origin could not be asked for it; it has to be on origin before a ticket is worked on", branch),
		}
	}
}

// CheckGitHubRepo checks if a GitHub remote is accessible
func (p *PreflightChecker) CheckGitHubRepo() CheckResult {
	// Check if gh CLI is available (already verified earlier, but be safe)
	if _, err := exec.LookPath("gh"); err != nil {
		return CheckResult{
			Name:    "GitHub Repo",
			Passed:  true,
			Warning: true,
			Message: "gh CLI not available, skipping repo check",
		}
	}

	cmd := exec.Command("gh", "repo", "view", "--json", "name", "-q", ".name")
	output, err := cmd.Output()
	if err != nil {
		return CheckResult{
			Name:    "GitHub Repo",
			Passed:  true,
			Warning: true,
			Message: "could not detect GitHub repo (offline or no remote configured)",
		}
	}

	repoName := strings.TrimSpace(string(output))
	return CheckResult{
		Name:    "GitHub Repo",
		Passed:  true,
		Message: fmt.Sprintf("GitHub repo: %s", repoName),
	}
}

// CheckRepoWritePermission verifies the authenticated gh identity can actually
// WRITE to the target repo. `gh auth status` only proves a token is logged in;
// the whole workflow (create issues, approve/merge PRs, move labels) needs write
// access, and a read-only token 403s on every one of those — passing preflight
// but then spinning the loop. This asks GitHub what the current identity can do
// and fails fast when it can't push.
func (p *PreflightChecker) CheckRepoWritePermission() CheckResult {
	const name = "Repo Write Access"

	if _, err := exec.LookPath("gh"); err != nil {
		return CheckResult{Name: name, Passed: true, Warning: true, Message: "gh CLI not available, skipping"}
	}

	// Resolve OWNER/REPO for the current directory.
	repoOut, err := exec.Command("gh", "repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner").Output()
	repo := strings.TrimSpace(string(repoOut))
	if err != nil || repo == "" {
		return CheckResult{Name: name, Passed: true, Warning: true, Message: "could not detect repo (offline or no remote), skipping write-access check"}
	}

	// Ask GitHub what the *current* identity may do on this repo.
	permOut, err := exec.Command("gh", "api", "repos/"+repo, "--jq", ".permissions.push").Output()
	if err != nil {
		return CheckResult{Name: name, Passed: true, Warning: true, Message: fmt.Sprintf("could not verify write access to %s (offline?), continuing", repo)}
	}
	canPush := strings.TrimSpace(string(permOut)) == "true"

	// Best-effort: name the account so the fix is obvious.
	login := "the current gh account"
	if out, uerr := exec.Command("gh", "api", "user", "--jq", ".login").Output(); uerr == nil {
		if l := strings.TrimSpace(string(out)); l != "" {
			login = l
		}
	}

	return writePermissionResult(repo, canPush, login)
}

// writePermissionResult builds the CheckResult from resolved facts. Pure, so the
// pass/fail decision and the guidance message stay unit-testable.
func writePermissionResult(repo string, canPush bool, login string) CheckResult {
	const name = "Repo Write Access"
	if canPush {
		return CheckResult{
			Name:    name,
			Passed:  true,
			Message: fmt.Sprintf("%s can write to %s", login, repo),
		}
	}
	return CheckResult{
		Name:   name,
		Passed: false,
		Message: fmt.Sprintf(
			"%s has read-only access to %s. The workflow must create issues, approve/merge PRs and move labels — a read-only token passes auth but 403s on every state change and spins the loop. Fix: point GH_TOKEN at an account with write access (e.g. $env:GH_TOKEN = (gh auth token --user <writer>)), or add %s as a collaborator.",
			login, repo, login),
	}
}

// Config returns the loaded configuration
func (p *PreflightChecker) Config() *Config {
	return p.config
}
