package worker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/layout"
)

func writePlanForTest(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plan.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// The Worker is told to write .ai/runs/issue-N/plan.md relative to its
// working directory, which is inside the worktree - not under the state root.
func TestResolvePlanContent_ReadsPlanFromWorktree(t *testing.T) {
	workDir := t.TempDir()
	runDir := layout.Run(t.TempDir(), 7)
	writePlanForTest(t, layout.Run(workDir, 7), "## Summary\nworktree plan")

	if got := resolvePlanContent(workDir, runDir, 7); got != "## Summary\nworktree plan" {
		t.Errorf("resolvePlanContent() = %q, want the plan written in the worktree", got)
	}
}

func TestResolvePlanContent_FallsBackToRunDir(t *testing.T) {
	workDir := t.TempDir()
	runDir := layout.Run(t.TempDir(), 7)
	writePlanForTest(t, runDir, "## Summary\nrun-dir plan")

	if got := resolvePlanContent(workDir, runDir, 7); got != "## Summary\nrun-dir plan" {
		t.Errorf("resolvePlanContent() = %q, want the run-dir plan", got)
	}
}

func TestResolvePlanContent_NoPlan(t *testing.T) {
	if got := resolvePlanContent(t.TempDir(), t.TempDir(), 7); got != "" {
		t.Errorf("resolvePlanContent() = %q, want empty", got)
	}
}
