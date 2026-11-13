package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStopMarkerReason(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "STOP")

	if got := stopMarkerReason(marker); got != "" {
		t.Errorf("missing marker: reason = %q, want empty", got)
	}

	// The shape `awkit stop-workflow` writes (internal/workflow.writeStopMarker).
	content := "STOP requested by stop-workflow\nReason: risky_assumptions\n"
	if err := os.WriteFile(marker, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if got := stopMarkerReason(marker); got != "risky_assumptions" {
		t.Errorf("reason = %q, want risky_assumptions", got)
	}

	// A marker created by hand (`touch .ai/state/STOP`) carries no reason.
	if err := os.WriteFile(marker, nil, 0644); err != nil {
		t.Fatal(err)
	}
	if got := stopMarkerReason(marker); got != "" {
		t.Errorf("empty marker: reason = %q, want empty", got)
	}
}

func TestStopMarkerMessage(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "STOP")
	if err := os.WriteFile(marker, []byte("STOP requested by stop-workflow\r\nReason: needs_human_review\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := stopMarkerMessage(marker); !strings.Contains(got, "reason: needs_human_review") {
		t.Errorf("message = %q, want it to name the reason", got)
	}

	if err := os.WriteFile(marker, nil, 0644); err != nil {
		t.Fatal(err)
	}
	if got := stopMarkerMessage(marker); got != "Workflow stopped (STOP marker present)" {
		t.Errorf("message = %q", got)
	}
}

// The prompt is the only place a Principal session learns that nobody is
// there to answer a question.
func TestPrincipalPromptSaysTheRunIsUnattended(t *testing.T) {
	for _, want := range []string{"Run the AWK principal workflow.", "unattended", "awkit stop-workflow"} {
		if !strings.Contains(principalPrompt, want) {
			t.Errorf("principalPrompt is missing %q", want)
		}
	}
}
