package doctor

import (
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/kickoff"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
)

// SetupProbe is what the doctor asks about a project's surroundings: the
// tools a run starts, the configuration, the remote. The pre-flight checker
// of `awkit run` answers.
//
// A run asks the same questions and stops at the first one that fails, so a
// person learned of a missing tool, then of an invalid configuration, then
// of a branch that is not there, one run at a time. The doctor asks them all
// and says everything that is wrong at once.
type SetupProbe interface {
	CheckGhCLI() kickoff.CheckResult
	CheckGhAuth() kickoff.CheckResult
	CheckClaudeCLI() kickoff.CheckResult
	CheckCodexCLI() kickoff.CheckResult
	CheckConfig() kickoff.CheckResult
	CheckRepoPaths() kickoff.CheckResult
	CheckIntegrationBranch() kickoff.CheckResult
	CheckGitHubRepo() kickoff.CheckResult
	CheckRepoWritePermission() kickoff.CheckResult
	CheckPermissions() kickoff.CheckResult
	CheckAgents() kickoff.CheckResult
	CheckGitClean() kickoff.CheckResult
	CheckGitRemote() kickoff.CheckResult
}

// SetupProbeOf is the probe of the project in root: the pre-flight checker of
// a run, asking git and gh in root.
func SetupProbeOf(root string) SetupProbe {
	return kickoff.NewPreflightChecker(layout.Config(root), layout.State(root, "kickoff.lock")).In(root)
}

// Setup asks the probe everything a run needs and returns what it said. A
// question that cannot be answered because an earlier one failed is not
// asked: without `gh` there is no word on its login, and without a
// configuration none on the branch the configuration names.
//
// It is the first half of `awkit doctor`, "What a run needs"; RunAll is the
// second, "What earlier runs left behind".
func Setup(p SetupProbe) []CheckResult {
	var results []CheckResult
	add := func(r kickoff.CheckResult) bool {
		results = append(results, fromPreflight(r))
		return r.Passed
	}

	// The tools.
	gh := add(p.CheckGhCLI())
	if gh {
		gh = add(p.CheckGhAuth())
	}
	add(p.CheckClaudeCLI())

	// The configuration, and what it names. The Worker's tool is one of
	// them: every ticket fails without it, so here it is not the warning a
	// run makes of it.
	configured := add(p.CheckConfig())
	backend := fromPreflight(p.CheckCodexCLI())
	if backend.Status == "warning" {
		backend.Status = "error"
		backend.Message = strings.TrimSuffix(backend.Message, " (worker execution may fail)") +
			": the Worker runs it for every ticket. Install it, or name the other backend (worker.backend) in " + layout.ConfigFile
	}
	results = append(results, backend)
	if configured {
		add(p.CheckRepoPaths())
		add(p.CheckPermissions())
		add(p.CheckAgents())
	}

	// The remote, and what is asked of it: the branch pull requests go to,
	// the GitHub repository behind it, and the right to write there.
	remote := add(p.CheckGitRemote())
	if remote && configured {
		add(p.CheckIntegrationBranch())
	}
	if remote && gh {
		add(p.CheckGitHubRepo())
		add(p.CheckRepoWritePermission())
	}

	// A run starts from a clean working tree. That is no fault of the
	// project, so it is a warning here.
	if clean := p.CheckGitClean(); !clean.Passed {
		results = append(results, CheckResult{
			Name:    clean.Name,
			Status:  "warning",
			Message: "the working tree has changes; a run starts from a clean one. Commit them (git add -A && git commit) or stash them",
		})
	}
	return results
}

// fromPreflight says a run's check in the doctor's terms.
func fromPreflight(r kickoff.CheckResult) CheckResult {
	status := "ok"
	switch {
	case !r.Passed:
		status = "error"
	case r.Warning:
		status = "warning"
	}
	return CheckResult{Name: r.Name, Status: status, Message: r.Message}
}
