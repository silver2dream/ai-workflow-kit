package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/kickoff"
	"github.com/silver2dream/ai-workflow-kit/internal/stopmark"
)

// The prompt is the only place a Principal session learns that nobody is
// there to answer a question.
func TestPrincipalPromptSaysTheRunIsUnattended(t *testing.T) {
	for _, want := range []string{"Run the AWK principal workflow.", "unattended", "awkit stop-workflow"} {
		if !strings.Contains(principalPrompt, want) {
			t.Errorf("principalPrompt is missing %q", want)
		}
	}
}

// A run says how it ended in words, and ends with 1 when it stopped short of
// its work: for every stop it used to print "Workflow stopped (STOP marker
// present, reason: ...)" and end with 0, a ticket that needs a person
// included.
func TestEndedByMarker(t *testing.T) {
	dir := inNewDir(t)
	output := kickoff.NewOutputFormatter(os.Stdout)

	if code, ended := endedByMarker(output); ended || code != 0 {
		t.Errorf("no marker: ended = %v, code = %d", ended, code)
	}

	for _, tc := range []struct {
		reason string
		code   int
		says   []string
	}{
		{"all_tasks_complete", 0, []string{"Every ticket is done."}},
		{"user_stopped", 0, []string{"The run was stopped, as asked.", "awkit run goes on"}},
		{"needs_human_review", 1, []string{"A ticket needs you.", "awkit status"}},
		{"worker_failed", 1, []string{"Worker failed on every attempt", "awkit reset --labels"}},
		{"blocked_by_dependencies", 1, []string{"waits for an issue that is not done"}},
	} {
		if err := stopmark.Write(dir, "stop-workflow", tc.reason); err != nil {
			t.Fatal(err)
		}
		var code int
		var ended bool
		out := captureStdout(t, func() { code, ended = endedByMarker(kickoff.NewOutputFormatter(os.Stdout)) })
		if !ended || code != tc.code {
			t.Errorf("%s: ended = %v, code = %d; want true, %d", tc.reason, ended, code, tc.code)
		}
		for _, want := range tc.says {
			if !strings.Contains(out, want) {
				t.Errorf("%s: the run does not say %q:\n%s", tc.reason, want, out)
			}
		}
		if strings.Contains(out, "STOP marker") {
			t.Errorf("%s: the run talks about its marker to a person:\n%s", tc.reason, out)
		}
	}

	// A marker made by hand carries no reason: a person asked for the stop.
	if err := os.WriteFile(stopmark.Path(dir), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	var code int
	out := captureStdout(t, func() { code, _ = endedByMarker(kickoff.NewOutputFormatter(os.Stdout)) })
	if code != 0 || !strings.Contains(out, "as asked") {
		t.Errorf("a marker made by hand: code = %d, says:\n%s", code, out)
	}
}

// `awkit stop` asks the run that is going to stop, and leaves no marker where
// no run is going: the next run would only have to clear it.
func TestStopRun(t *testing.T) {
	dir := inNewDir(t)

	var out bytes.Buffer
	if code := stopRun(&out, dir, false); code != 0 || !strings.Contains(out.String(), "No run is going") {
		t.Errorf("no run: code = %d, says %q", code, out.String())
	}
	if _, present := stopmark.Read(dir); present {
		t.Error("a marker was written though no run is going")
	}

	out.Reset()
	if code := stopRun(&out, dir, true); code != 0 || !strings.Contains(out.String(), "Asked the run to stop") || !strings.Contains(out.String(), "finishes first") {
		t.Errorf("a run is going: code = %d, says %q", code, out.String())
	}
	if reason, present := stopmark.Read(dir); !present || reason != "user_stopped" {
		t.Errorf("the marker says %q (present %v), want user_stopped", reason, present)
	}

	// The command itself, in a project where nothing runs.
	if err := stopmark.Clear(dir); err != nil {
		t.Fatal(err)
	}
	said := captureStdout(t, func() {
		if code := cmdStop(nil); code != 0 {
			t.Errorf("cmdStop = %d, want 0", code)
		}
	})
	if !strings.Contains(said, "No run is going") {
		t.Errorf("awkit stop without a run says %q", said)
	}
}
