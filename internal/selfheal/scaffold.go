// Package selfheal reconciles a project's structural state back to a workable
// baseline so the workflow can keep going instead of spinning or stalling on
// damage left by earlier failures. It is declarative: it compares the desired
// state (the integration branch has each repo's scaffold) to the actual state
// and, when they diverge, corrects it — safely, idempotently, and observably.
package selfheal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/install"
)

// Repo is the minimal repo shape selfheal needs: where the repo lives and what
// language it is — enough to know which base file proves its scaffold exists.
// Kept local so selfheal depends on neither analyzer nor kickoff (no import
// cycle, callers convert their own config).
type Repo struct {
	Path     string
	Language string
}

// ScaffoldReconcile is the outcome of reconciling the integration branch scaffold.
type ScaffoldReconcile struct {
	Healthy bool     // integration branch already had every repo's scaffold
	Missing []string // repo paths whose scaffold is absent on the integration branch
	Preset  string   // preset used (or inferred) for regeneration
	Fixed   bool     // a fix was actually applied (false in dry-run)
	Message string
	Err     error
}

// inferPreset derives the scaffold preset from the configured repos, so an
// existing project (whose config never recorded the preset) can still be
// reconciled. Returns "" when the repo mix matches no known preset.
func inferPreset(repos []Repo) string {
	var backend, frontend string
	for _, r := range repos {
		switch strings.ToLower(r.Language) {
		case "go":
			backend = "go"
		case "python":
			backend = "python"
		case "node", "react", "typescript", "javascript":
			frontend = "react"
		}
	}
	switch {
	case frontend == "react" && backend == "go":
		return "react-go"
	case frontend == "react" && backend == "python":
		return "react-python"
	case backend == "go" && frontend == "":
		return "go"
	case backend == "python" && frontend == "":
		return "python"
	case frontend == "react" && backend == "":
		return "node"
	default:
		return ""
	}
}

// repoBaseFile is the file whose presence proves a repo's scaffold exists.
func repoBaseFile(r Repo) string {
	path := strings.TrimSuffix(r.Path, "/")
	switch strings.ToLower(r.Language) {
	case "go":
		return join(path, "go.mod")
	case "node", "react", "typescript", "javascript":
		return join(path, "package.json")
	case "python":
		return join(path, "pyproject.toml")
	default:
		return ""
	}
}

func join(dir, file string) string {
	if dir == "" || dir == "." {
		return file
	}
	return dir + "/" + file
}

// missingRepoScaffold returns the repo paths whose base file is absent on the
// given git ref (uses forward-slash git paths, not filepath). Presence is probed
// through lsTree, which the caller binds to the repo root, so this stays pure and
// testable.
func missingRepoScaffold(ref string, repos []Repo, lsTree func(ref, path string) bool) []string {
	var missing []string
	for _, r := range repos {
		base := repoBaseFile(r)
		if base == "" {
			continue
		}
		if !lsTree(ref, base) {
			missing = append(missing, r.Path)
		}
	}
	return missing
}

// ReconcileIntegrationScaffold ensures the integration branch every worker
// branches from actually contains each repo's scaffold. If a repo's scaffold is
// missing (e.g. the first task false-completed so the scaffold never landed on
// main, leaving every worker to branch from an empty tree), it regenerates the
// scaffold from the current templates on an isolated worktree, commits, and
// pushes — so the project self-corrects instead of stalling. dryRun reports the
// plan without changing anything.
func ReconcileIntegrationScaffold(stateRoot, integrationBranch string, repos []Repo, dryRun bool) ScaffoldReconcile {
	if len(repos) == 0 {
		return ScaffoldReconcile{Healthy: true, Message: "no repos configured"}
	}
	branch := strings.TrimSpace(integrationBranch)
	if branch == "" {
		return ScaffoldReconcile{Healthy: true, Message: "no integration branch configured"}
	}
	ref := "origin/" + branch // workers branch from the remote integration branch

	lsTree := func(ref, path string) bool {
		out, err := exec.Command("git", "-C", stateRoot, "ls-tree", ref, "--", path).Output()
		return err == nil && strings.TrimSpace(string(out)) != ""
	}
	missing := missingRepoScaffold(ref, repos, lsTree)
	if len(missing) == 0 {
		return ScaffoldReconcile{Healthy: true, Message: fmt.Sprintf("%s has every repo's scaffold", ref)}
	}

	preset := inferPreset(repos)
	if preset == "" {
		return ScaffoldReconcile{
			Missing: missing,
			Message: fmt.Sprintf("%s is missing scaffold for %v but the preset can't be inferred; run `awkit init --preset <name>`", ref, missing),
		}
	}

	if dryRun {
		return ScaffoldReconcile{
			Missing: missing,
			Preset:  preset,
			Message: fmt.Sprintf("would regenerate scaffold (preset %s) for %v onto %s", preset, missing, branch),
		}
	}

	if err := applyScaffoldToBranch(stateRoot, branch, preset); err != nil {
		return ScaffoldReconcile{Missing: missing, Preset: preset, Err: err, Message: fmt.Sprintf("failed to reconcile scaffold: %v", err)}
	}
	return ScaffoldReconcile{
		Missing: missing,
		Preset:  preset,
		Fixed:   true,
		Message: fmt.Sprintf("regenerated scaffold (preset %s) onto %s and pushed", preset, branch),
	}
}

// applyScaffoldToBranch regenerates the scaffold on an isolated worktree of the
// integration branch and pushes it, so the main working tree is never disturbed
// and a failure leaves nothing behind.
func applyScaffoldToBranch(stateRoot, branch, preset string) error {
	wt, err := os.MkdirTemp("", "awkit-scaffold-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(wt)
	defer runGit(stateRoot, "worktree", "remove", "--force", wt) // best-effort cleanup

	if err := runGit(stateRoot, "fetch", "origin", branch); err != nil {
		return fmt.Errorf("fetch %s: %w", branch, err)
	}
	if err := runGit(stateRoot, "worktree", "add", "--detach", wt, "origin/"+branch); err != nil {
		return fmt.Errorf("worktree add: %w", err)
	}

	// SkipDeps: commit the source scaffold only. No npm install runs, so no
	// node_modules can exist to be committed — which is also why we deliberately
	// do NOT write our own .gitignore here. A .gitignore that differs even
	// slightly from the one `awkit init` commits becomes an add/add conflict
	// against any in-flight scaffold PR (this is exactly what stalled
	// tennis-arena's PR #8). The .gitignore is init's concern; reconcile restores
	// only what's needed to unblock workers branching from the integration branch.
	if _, err := install.Scaffold(wt, install.ScaffoldOptions{
		Preset:      preset,
		TargetDir:   wt,
		ProjectName: filepath.Base(strings.TrimSuffix(stateRoot, "/")),
		Force:       false, // never clobber existing files — only fill the gaps
		SkipDeps:    true,  // commit source only — never install/commit node_modules
	}); err != nil {
		return fmt.Errorf("scaffold: %w", err)
	}

	if err := runGit(wt, "add", "-A"); err != nil {
		return fmt.Errorf("git add: %w", err)
	}
	// Nothing staged -> the scaffold already matched; treat as success.
	if runGit(wt, "diff", "--cached", "--quiet") == nil {
		return nil
	}
	if err := runGit(wt, "commit", "-m", "[chore] self-heal: restore integration-branch scaffold"); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}
	if err := runGit(wt, "push", "origin", "HEAD:"+branch); err != nil {
		return fmt.Errorf("git push: %w", err)
	}
	return nil
}

// runGit runs a git command in dir. On failure it folds git's own stderr into
// the error so a rejected push (read-only token, non-fast-forward) is diagnosable
// rather than a bare "exit status 1". Callers that use it as a boolean (e.g.
// `diff --cached --quiet`) still work — they only test the error for nil.
func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if trimmed := strings.TrimSpace(string(out)); trimmed != "" {
			return fmt.Errorf("%w: %s", err, trimmed)
		}
		return err
	}
	return nil
}
