package worker

import (
	"strings"
	"testing"
)

// The protected set is the workflow's own control files: what decides how
// work is dispatched and reviewed, and what the agents that do it are told
// and allowed.
func TestFindProtectedChanges_ControlFiles(t *testing.T) {
	protected := []string{
		".ai/config.yaml",
		".ai/roster.yaml",
		".claude/rules/_kit/git-workflow.md",
		".claude/rules/backend.md", // a rule of the project's own binds the review as much
		".claude/skills/principal-workflow/phases/review.md",
		".claude/agents/pr-reviewer.md",
		// Not AWK's files, and control files all the same: they grant
		// permissions and run commands in the Principal's session.
		".claude/settings.json",
		".claude/settings.local.json",
		".claude/commands/deploy.md",
		".claude/hooks/pre-tool.sh",
		".mcp.json",
		"CLAUDE.md",
		"AGENTS.md",
		// On a file system that ignores case these are the same files.
		".Claude/settings.json",
		".CLAUDE/agents/pr-reviewer.md",
		".AI/Config.yaml",
		"claude.md",
		"Agents.md",
	}
	for _, file := range protected {
		if got := findProtectedChanges([]string{file}, ""); len(got) != 1 || got[0] != file {
			t.Errorf("findProtectedChanges(%q) = %v, want it reported", file, got)
		}
	}

	unprotected := []string{
		"backend/internal/engine.go",
		"docs/AGENTS.md",                 // only the root file is the Worker guide
		"backend/CLAUDE.md",              // same
		"backend/.claude/settings.json",  // only the root directory is the project's
		"notes/.ai/config.yaml",          // not at the repository root
		"tools/.mcp.json",                // same
		".ai/state/runs/issue-7/plan.md", // what a run leaves behind
		".claudeignore",                  // a name that starts alike
		".ai/config.yaml.bak",            // same
		"internal/skills/skills.go",      // similar name, ordinary code
		".github/workflows/ci.yml",
	}
	if got := findProtectedChanges(unprotected, ""); len(got) != 0 {
		t.Errorf("findProtectedChanges(ordinary files) = %v, want none", got)
	}
}

// Where an earlier awkit kept its control files is nowhere now: an upgrade
// moves a project away from there, and nothing reads those paths.
func TestFindProtectedChanges_TheEarlierPlacesAreOrdinary(t *testing.T) {
	earlier := []string{
		".ai/config/workflow.yaml",
		".ai/rules/_kit/git-workflow.md",
		".ai/skills/principal-workflow/SKILL.md",
		".ai/specs/example/design.md",
	}
	if got := findProtectedChanges(earlier, ""); len(got) != 0 {
		t.Errorf("findProtectedChanges(the earlier places) = %v, want none", got)
	}
}

func TestFindProtectedChanges_WhitelistAllowsRootFile(t *testing.T) {
	if got := findProtectedChanges([]string{"AGENTS.md"}, "AGENTS.md"); len(got) != 0 {
		t.Errorf("a whitelisted file must not be reported, got %v", got)
	}
}

// The paths removed with the bash-era workflow are no longer special.
func TestFindProtectedChanges_LegacyPathsAreOrdinary(t *testing.T) {
	if got := findProtectedChanges([]string{".ai/scripts/run.sh", ".ai/commands/start-work.md"}, ""); len(got) != 0 {
		t.Errorf("legacy paths should not be protected any more, got %v", got)
	}
}

// The prompt tells the Worker exactly what the runner enforces.
func TestWorkerPromptListsProtectedPaths(t *testing.T) {
	prompt := buildWorkerPrompt(workerPromptInputs{Ticket: "# Task"})
	for _, path := range append(append([]string{}, protectedPaths...), protectedFiles...) {
		if !strings.Contains(prompt, path) {
			t.Errorf("prompt does not name protected path %q", path)
		}
	}
	for _, want := range []string{".claude/", ".ai/config.yaml", ".ai/roster.yaml", "CLAUDE.md", "AGENTS.md", ".mcp.json"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt does not name %q", want)
		}
	}
	for _, gone := range []string{".ai/scripts/", ".ai/commands/", ".ai/rules/", ".ai/skills/", ".ai/config/", "Req 3.4"} {
		if strings.Contains(prompt, gone) {
			t.Errorf("prompt still mentions %q", gone)
		}
	}
}
