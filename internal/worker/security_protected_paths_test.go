package worker

import (
	"strings"
	"testing"
)

// The protected set is the workflow's own control files: what decides how
// work is dispatched and reviewed.
func TestFindProtectedChanges_ControlFiles(t *testing.T) {
	protected := []string{
		".ai/config/workflow.yaml",
		".ai/rules/_kit/git-workflow.md",
		".ai/skills/principal-workflow/phases/review.md",
		".claude/agents/pr-reviewer.md",
		"CLAUDE.md",
		"AGENTS.md",
	}
	for _, file := range protected {
		if got := findProtectedChanges([]string{file}, ""); len(got) != 1 {
			t.Errorf("findProtectedChanges(%q) = %v, want it reported", file, got)
		}
	}

	unprotected := []string{
		"backend/internal/engine.go",
		"docs/AGENTS.md",                 // only the root file is the Worker guide
		"backend/CLAUDE.md",              // same
		".claude/settings.local.json",    // machine-local, git-ignored
		".ai/specs/example/design.md",    // a spec is content, not control
		"internal/skills/skills.go",      // similar name, ordinary code
		"notes/.ai/config/workflow.yaml", // not at the repository root
	}
	if got := findProtectedChanges(unprotected, ""); len(got) != 0 {
		t.Errorf("findProtectedChanges(ordinary files) = %v, want none", got)
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
	for _, gone := range []string{".ai/scripts/", ".ai/commands/", "Req 3.4"} {
		if strings.Contains(prompt, gone) {
			t.Errorf("prompt still mentions %q", gone)
		}
	}
}
