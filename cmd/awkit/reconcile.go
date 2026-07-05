package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/silver2dream/ai-workflow-kit/internal/kickoff"
	"github.com/silver2dream/ai-workflow-kit/internal/selfheal"
	"github.com/silver2dream/ai-workflow-kit/internal/trace"
)

func usageReconcile() {
	fmt.Fprint(os.Stderr, `Reconcile a project's structural state back to a workable baseline

The workflow can be left in a recoverable-but-broken shape by earlier failures —
most commonly an integration branch that never received a repo's scaffold because
an early task false-completed, so every worker then branches from an empty tree.
reconcile detects such damage and self-corrects it (regenerate the missing
scaffold on an isolated worktree, commit, and push) so the workflow keeps going
instead of stalling.

This runs automatically before every analyze-next; the command exists so operators
can preview (--dry-run) or force a reconcile out of band.

Usage:
  awkit reconcile [options]

Options:
  --dry-run       Report what would change without pushing anything
  --json          Output the reconcile result as JSON
  --state-root    Override state root (default: current git root)
  --help          Show this help

Examples:
  awkit reconcile --dry-run
  awkit reconcile
`)
}

func cmdReconcile(args []string) int {
	fs := flag.NewFlagSet("reconcile", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usageReconcile

	dryRun := fs.Bool("dry-run", false, "")
	jsonOutput := fs.Bool("json", false, "")
	stateRoot := fs.String("state-root", "", "")
	showHelp := fs.Bool("help", false, "")
	showHelpShort := fs.Bool("h", false, "")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showHelp || *showHelpShort {
		usageReconcile()
		return 0
	}

	if *stateRoot == "" {
		root, err := resolveGitRoot()
		if err != nil {
			errorf("failed to resolve git root: %v\n", err)
			return 1
		}
		*stateRoot = root
	}

	res := runScaffoldReconcile(*stateRoot, *dryRun)

	if *jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		// ScaffoldReconcile carries an error value that does not marshal; project
		// it onto a serialisable view.
		_ = enc.Encode(map[string]any{
			"healthy": res.Healthy,
			"missing": res.Missing,
			"preset":  res.Preset,
			"fixed":   res.Fixed,
			"message": res.Message,
			"error":   errString(res.Err),
		})
	} else {
		fmt.Println(res.Message)
	}

	if res.Err != nil {
		return 1
	}
	return 0
}

// runScaffoldReconcile loads the project config and runs the integration-branch
// scaffold reconcile. Shared by the reconcile command and the automatic pass in
// analyze-next so both behave identically.
func runScaffoldReconcile(stateRoot string, dryRun bool) selfheal.ScaffoldReconcile {
	cfg, err := kickoff.LoadConfig(filepath.Join(stateRoot, ".ai", "config", "workflow.yaml"))
	if err != nil {
		return selfheal.ScaffoldReconcile{Healthy: true, Message: fmt.Sprintf("skipped: config not loadable (%v)", err)}
	}

	repos := make([]selfheal.Repo, 0, len(cfg.Repos))
	for _, r := range cfg.Repos {
		repos = append(repos, selfheal.Repo{Path: r.Path, Language: r.Language})
	}

	return selfheal.ReconcileIntegrationScaffold(stateRoot, cfg.Git.IntegrationBranch, repos, dryRun)
}

// reconcileStructuralState is the automatic self-heal pass run before the
// analyzer decides, so a repaired base is in place before any worker is
// dispatched. It applies fixes for real (dryRun=false). All output goes to
// stderr/trace only — never stdout — so it can't corrupt the analyze-next
// bash/JSON that the principal evals.
func reconcileStructuralState(stateRoot string) {
	res := runScaffoldReconcile(stateRoot, false)
	if res.Healthy {
		clearReconcileNotice(stateRoot) // reset dedupe so a future relapse re-reports
		return
	}

	level := trace.LevelInfo
	if res.Err != nil {
		level = trace.LevelWarn
	}
	trace.WriteEvent(trace.ComponentPrincipal, trace.TypeReconcile, level, trace.WithData(map[string]any{
		"kind":    "integration_scaffold",
		"missing": res.Missing,
		"preset":  res.Preset,
		"fixed":   res.Fixed,
		"message": res.Message,
	}))

	// Print to the console only when the situation changed, so a persistent
	// un-fixable state (e.g. a read-only token, or a preset that can't be
	// inferred) doesn't spam a line every single loop.
	if reconcileNoticeChanged(stateRoot, res.Message) {
		fmt.Fprintf(os.Stderr, "[reconcile] %s\n", res.Message)
	}
}

// reconcileNoticeChanged reports whether msg differs from the last reconcile
// notice printed, recording msg as the new last notice.
func reconcileNoticeChanged(stateRoot, msg string) bool {
	path := filepath.Join(stateRoot, ".ai", "state", "reconcile.last")
	if prev, err := os.ReadFile(path); err == nil && string(prev) == msg {
		return false
	}
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	_ = os.WriteFile(path, []byte(msg), 0644)
	return true
}

// clearReconcileNotice forgets the last notice once the state is healthy again,
// so a relapse is reported afresh rather than suppressed by a stale match.
func clearReconcileNotice(stateRoot string) {
	_ = os.Remove(filepath.Join(stateRoot, ".ai", "state", "reconcile.last"))
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
