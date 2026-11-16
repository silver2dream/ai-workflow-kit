package worker

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/config"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
)

// A project that has not been upgraded has its configuration at the earlier
// place. The Worker does not go on from the defaults there, which would
// ignore what the project says: it stops, and says what to do.
func TestRunIssue_AProjectThatHasNotBeenUpgraded(t *testing.T) {
	dir := t.TempDir()
	earlier := filepath.Join(dir, ".ai", "config", "workflow.yaml")
	if err := os.MkdirAll(filepath.Dir(earlier), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(earlier, []byte("repos:\n  - {name: backend, path: backend/}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ticket := filepath.Join(dir, "ticket.md")
	if err := os.WriteFile(ticket, []byte("# [feat] a task\n\n```yaml awk-ticket\nschema: 1\nrepo: root\n```\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A run that went on would start git and gh. With a context that is
	// already done, no command it starts ever runs.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := RunIssue(ctx, RunIssueOptions{IssueID: 7, TicketFile: ticket, StateRoot: dir})

	var old *config.OldLayoutError
	if result != nil || !errors.As(err, &old) || !strings.Contains(err.Error(), "awkit upgrade") {
		t.Fatalf("result %+v, err %v; want the run to stop and say to upgrade", result, err)
	}
	// It stopped before it wrote anything of a run.
	for _, made := range []string{layout.Run(dir, 7), layout.Logs(dir), layout.Results(dir)} {
		if _, err := os.Stat(made); !os.IsNotExist(err) {
			t.Errorf("the run went on to make %s (err %v)", made, err)
		}
	}
}

// What a run leaves behind never goes into the Worker's commit, whatever the
// project's ignore file says. The project's configuration and what is under
// .claude are staged like any other file: a change to them is then checked
// like a change to any control file. All of .ai used to be kept out of the
// commit, and a change to the configuration was dropped without a word.
func TestStageChanges_KeepsWhatARunLeavesBehindOutOfTheCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	git("init", "-q")
	for _, rel := range []string{
		"main.go",
		".ai/config.yaml",
		".ai/roster.yaml",
		".claude/rules/backend.md",
		".ai/state/runs/issue-7/plan.md",
		".ai/state/logs/issue-7.worker.log",
		".worktrees/issue-8/main.go",
	} {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := stageChanges(context.Background(), dir, "root", 30*time.Second); err != nil {
		t.Fatalf("stageChanges: %v", err)
	}

	staged := strings.Fields(git("diff", "--cached", "--name-only"))
	want := []string{".ai/config.yaml", ".ai/roster.yaml", ".claude/rules/backend.md", "main.go"}
	if !reflect.DeepEqual(staged, want) {
		t.Errorf("staged %v, want %v", staged, want)
	}
	// And what is staged of the control files is what the check then sees.
	if got := findProtectedChanges(staged, ""); !reflect.DeepEqual(got, want[:3]) {
		t.Errorf("the check sees %v as changes to control files, want %v", got, want[:3])
	}
}
