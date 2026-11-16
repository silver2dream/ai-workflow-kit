package main

import (
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/stopmark"
)

// Before it decides, analyze-next repairs the project: it restores a skeleton
// the integration branch lost, which is a commit and a push. A run that was
// asked to stop changes nothing more, so with the STOP marker there the
// repair is left out and the answer is "none".
func TestCmdAnalyzeNext_AStopSkipsTheRepair(t *testing.T) {
	repairs := 0
	previous := repairStructuralState
	repairStructuralState = func(string) { repairs++ }
	t.Cleanup(func() { repairStructuralState = previous })

	decide := func(dir string) string {
		t.Helper()
		var code int
		out := captureStdout(t, func() { code = cmdAnalyzeNext([]string{"--state-root", dir}) })
		if code != 0 {
			t.Fatalf("analyze-next = %d\n%s", code, out)
		}
		return out
	}

	// No marker: the repair comes first. (Without a configuration the
	// decision is made before GitHub is asked anything.)
	if out := decide(t.TempDir()); repairs != 1 || !strings.Contains(out, "EXIT_REASON=config_not_found") {
		t.Errorf("without a marker: %d repair(s), want 1\n%s", repairs, out)
	}

	stopped := t.TempDir()
	writePlanConfig(t, stopped, "")
	if err := stopmark.Write(stopped, "awkit stop", stopmark.ByUser); err != nil {
		t.Fatal(err)
	}
	repairs = 0
	if out := decide(stopped); repairs != 0 || !strings.Contains(out, "NEXT_ACTION=none") || !strings.Contains(out, "EXIT_REASON=user_stopped") {
		t.Errorf("with the marker: %d repair(s), want none\n%s", repairs, out)
	}
}
