package workflow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteStopMarker(t *testing.T) {
	dir := t.TempDir()
	if err := writeStopMarker(dir, "escalation_triggered"); err != nil {
		t.Fatalf("writeStopMarker: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".ai", "state", "STOP"))
	if err != nil {
		t.Fatalf("STOP marker not created: %v", err)
	}
	if !strings.Contains(string(data), "escalation_triggered") {
		t.Errorf("STOP marker missing reason, got %q", string(data))
	}
}

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
