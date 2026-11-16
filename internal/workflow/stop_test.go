package workflow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/stopmark"
)

// TestStopWorkflow_WritesStopMarker is the regression for the 50-session spin:
// StopWorkflow must drop the STOP marker the kickoff loop halts on — not just a
// report — or the multi-session loop keeps restarting after every stop request.
func TestStopWorkflow_WritesStopMarker(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".ai", "state"), 0755); err != nil {
		t.Fatal(err)
	}

	// Stats collection hits gh and is non-fatal; a tiny timeout keeps it fast.
	if _, err := StopWorkflow(context.Background(), StopWorkflowOptions{
		Reason:    "escalation_triggered",
		StateRoot: dir,
		GHTimeout: 1 * time.Millisecond,
	}); err != nil {
		t.Fatalf("StopWorkflow: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".ai", "state", "STOP")); err != nil {
		t.Fatalf("StopWorkflow must create the STOP marker the kickoff loop checks: %v", err)
	}
}

// The marker a stop leaves carries the reason, in the form the run and the
// decision engine read back.
func TestStopWorkflow_TheMarkerCarriesTheReason(t *testing.T) {
	dir := t.TempDir()
	if _, err := StopWorkflow(context.Background(), StopWorkflowOptions{
		Reason:    "needs_human_review",
		StateRoot: dir,
		GHTimeout: 1 * time.Millisecond,
	}); err != nil {
		t.Fatalf("StopWorkflow: %v", err)
	}
	if reason, present := stopmark.Read(dir); !present || reason != "needs_human_review" {
		t.Errorf("the marker says %q (present %v), want needs_human_review", reason, present)
	}
}

// How a run ended is said for every reason a run can stop with, and each one
// is either done or stopped short. Only "nothing left" and "a person asked"
// are done.
func TestEndingFor(t *testing.T) {
	done := map[string]bool{"all_tasks_complete": true, "user_stopped": true, "interrupted": true}
	for _, reason := range ValidExitReasons() {
		ending := EndingFor(reason)
		if ending.Summary == "" || ending.Next == "" {
			t.Errorf("EndingFor(%q) = %+v: it says nothing", reason, ending)
		}
		if ending.Done != done[reason] {
			t.Errorf("EndingFor(%q).Done = %v, want %v", reason, ending.Done, done[reason])
		}
		if !done[reason] && reason != "none" && strings.Contains(ending.Summary, "("+reason+")") && reason != "github_api_error" && reason != "loop_count_error" && reason != "error_exit" && reason != "contract_violation" {
			t.Errorf("EndingFor(%q) has no words of its own: %q", reason, ending.Summary)
		}
	}
	// A marker made by hand carries no reason: a person asked.
	if ending := EndingFor(""); !ending.Done {
		t.Errorf("EndingFor(\"\") = %+v, want the stop a person asked for", ending)
	}
	if ending := EndingFor("something_new"); ending.Done || !strings.Contains(ending.Summary, "something_new") {
		t.Errorf("EndingFor of a reason nobody knows = %+v, want it named and not called done", ending)
	}
	for reason, want := range map[string]string{
		"all_tasks_complete":      "Every ticket is done.",
		"needs_human_review":      "A ticket needs you.",
		"worker_failed":           "A ticket's Worker failed on every attempt.",
		"blocked_by_dependencies": "Every open ticket waits for an issue that is not done.",
		"invalid_ticket":          "A ticket cannot be read.",
	} {
		if got := EndingFor(reason).Summary; got != want {
			t.Errorf("EndingFor(%q).Summary = %q, want %q", reason, got, want)
		}
	}
}
