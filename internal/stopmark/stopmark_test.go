package stopmark

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPath(t *testing.T) {
	if File != ".ai/state/STOP" {
		t.Errorf("File = %q", File)
	}
	if got, want := Path("project"), filepath.Join("project", ".ai", "state", "STOP"); got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}

// The marker says who asked for the stop and why, in the form the run reads
// back; a marker made by hand, or one an earlier awkit wrote with Windows
// line endings, is read too.
func TestWriteReadClear(t *testing.T) {
	root := t.TempDir()

	if reason, present := Read(root); present || reason != "" {
		t.Errorf("no marker: Read = %q, %v", reason, present)
	}
	if err := Clear(root); err != nil {
		t.Errorf("Clear without a marker: %v", err)
	}

	if err := Write(root, "awkit stop", ByUser); err != nil {
		t.Fatalf("Write: %v", err)
	}
	content, err := os.ReadFile(Path(root))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(content), "STOP requested by awkit stop\nReason: user_stopped\n"; got != want {
		t.Errorf("the marker is %q, want %q", got, want)
	}
	if reason, present := Read(root); !present || reason != "user_stopped" {
		t.Errorf("Read = %q, %v; want user_stopped, true", reason, present)
	}

	for content, want := range map[string]string{
		"": "",
		"STOP requested by stop-workflow\r\nReason: needs_human_review\r\n": "needs_human_review",
		"Reason:worker_failed": "worker_failed",
		"stop please\n":        "",
	} {
		if err := os.WriteFile(Path(root), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if reason, present := Read(root); !present || reason != want {
			t.Errorf("a marker of %q: Read = %q, %v; want %q, true", content, reason, present, want)
		}
	}

	if err := Clear(root); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, present := Read(root); present {
		t.Error("the marker is still there after Clear")
	}
}
