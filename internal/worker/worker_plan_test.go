package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// readPlanFile
// ---------------------------------------------------------------------------

func TestReadPlanFile_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "plan.md")
	content := "## Summary\nRefactor the module.\n\n## Files to modify\n- `foo.go` — update logic\n"
	if err := os.WriteFile(planPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	got := readPlanFile(dir)
	if got == "" {
		t.Fatal("expected non-empty plan content")
	}
	if !strings.Contains(got, "Refactor the module") {
		t.Errorf("plan content should contain expected text, got: %s", got)
	}
}

func TestReadPlanFile_MissingFile(t *testing.T) {
	dir := t.TempDir()
	got := readPlanFile(dir)
	if got != "" {
		t.Errorf("expected empty string for missing file, got: %s", got)
	}
}

func TestReadPlanFile_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "plan.md")
	if err := os.WriteFile(planPath, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	got := readPlanFile(dir)
	if got != "" {
		t.Errorf("expected empty string for empty file, got: %s", got)
	}
}

func TestReadPlanFile_WhitespaceOnly(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "plan.md")
	if err := os.WriteFile(planPath, []byte("   \n\n  \n"), 0644); err != nil {
		t.Fatal(err)
	}

	got := readPlanFile(dir)
	if got != "" {
		t.Errorf("expected empty string for whitespace-only file, got: %s", got)
	}
}

// ---------------------------------------------------------------------------
// buildPRBody
// ---------------------------------------------------------------------------

func TestBuildPRBody_WithPlan(t *testing.T) {
	body := buildPRBody(42, "[feat] add widget", "## Summary\nAdd a new widget.\n")

	if !strings.Contains(body, "Closes #42") {
		t.Error("body should contain Closes #42")
	}
	if !strings.Contains(body, "## Implementation Plan") {
		t.Error("body should contain Implementation Plan section")
	}
	if !strings.Contains(body, "Add a new widget") {
		t.Error("body should contain plan content")
	}
	if !strings.Contains(body, "## Changes") {
		t.Error("body should contain Changes section")
	}
	if !strings.Contains(body, "[feat] add widget") {
		t.Error("body should contain title in Changes section")
	}
}

func TestBuildPRBody_WithoutPlan(t *testing.T) {
	body := buildPRBody(99, "[fix] resolve crash", "")

	if !strings.Contains(body, "Closes #99") {
		t.Error("body should contain Closes #99")
	}
	if strings.Contains(body, "## Implementation Plan") {
		t.Error("body should NOT contain Implementation Plan when plan is empty")
	}
	if !strings.Contains(body, "## Changes") {
		t.Error("body should contain Changes section")
	}
	if !strings.Contains(body, "[fix] resolve crash") {
		t.Error("body should contain title")
	}
}

func TestBuildPRBody_PlanBeforeChanges(t *testing.T) {
	body := buildPRBody(1, "title", "plan content here")

	planIdx := strings.Index(body, "## Implementation Plan")
	changesIdx := strings.Index(body, "## Changes")
	closesIdx := strings.Index(body, "Closes #1")

	if closesIdx < 0 || planIdx < 0 || changesIdx < 0 {
		t.Fatal("body missing expected sections")
	}
	if closesIdx >= planIdx {
		t.Error("Closes should come before Implementation Plan")
	}
	if planIdx >= changesIdx {
		t.Error("Implementation Plan should come before Changes")
	}
}
