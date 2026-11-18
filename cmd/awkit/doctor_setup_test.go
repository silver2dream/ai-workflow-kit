package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/doctor"
)

// The doctor's last line is what a person does next, and its exit code says
// whether a run can start.
func TestPrintDoctor(t *testing.T) {
	setup := []doctor.CheckResult{
		{Name: "gh CLI", Status: "ok", Message: "gh CLI installed"},
		{Name: "Git Remote", Status: "error", Message: "the repository has no remote named origin"},
		{Name: "Git Status", Status: "warning", Message: "the working tree has changes"},
	}
	state := []doctor.CheckResult{{Name: "Loop Count", Status: "warning", Message: "loop_count = 9", CanClean: true}}

	var out bytes.Buffer
	if code := printDoctor(&out, setup, state); code != 1 {
		t.Errorf("exit code = %d, want 1: something has to be fixed before a run", code)
	}
	for _, want := range []string{
		"What a run needs\n",
		"  ✓ gh CLI: gh CLI installed\n",
		"  ✗ Git Remote: the repository has no remote named origin\n",
		"  ! Git Status: the working tree has changes\n",
		"What earlier runs left behind\n",
		"  ! Loop Count: loop_count = 9\n",
		"1 problem to fix before a run, and 2 warnings.\n",
		"awkit reset",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the doctor does not say %q:\n%s", want, out.String())
		}
	}

	out.Reset()
	if code := printDoctor(&out, setup[:1], nil); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "Nothing stands in the way of a run.\n") || !strings.Contains(out.String(), "What earlier runs left behind\n  nothing\n") || strings.Contains(out.String(), "awkit reset") {
		t.Errorf("a project with nothing wrong:\n%s", out.String())
	}

	// A reset is offered for what a reset clears, and for nothing else.
	out.Reset()
	printDoctor(&out, setup[:1], []doctor.CheckResult{{Name: "Stop Marker", Status: "ok", Message: "No stop marker"}})
	if !strings.Contains(out.String(), "  ✓ Stop Marker: No stop marker\n") || strings.Contains(out.String(), "awkit reset") {
		t.Errorf("a project whose earlier runs left nothing to clear:\n%s", out.String())
	}

	for _, tc := range []struct {
		errors, warnings int
		want             string
	}{
		{0, 0, "Nothing stands in the way of a run."},
		{0, 1, "Nothing stands in the way of a run. 1 warning."},
		{0, 3, "Nothing stands in the way of a run. 3 warnings."},
		{1, 0, "1 problem to fix before a run."},
		{2, 0, "2 problems to fix before a run."},
		{2, 1, "2 problems to fix before a run, and 1 warning."},
	} {
		if got := doctorVerdict(tc.errors, tc.warnings); got != tc.want {
			t.Errorf("doctorVerdict(%d, %d) = %q, want %q", tc.errors, tc.warnings, got, tc.want)
		}
	}
}
