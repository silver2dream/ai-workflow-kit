package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/engine"
)

// awkit lane names one ticket, and what to do with it.
func TestCmdLane_WhatItTakes(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, c := range []struct {
		args []string
		code int
	}{
		{nil, 2},
		{[]string{"--help"}, 0},
		{[]string{"merge", "5"}, 2},
		{[]string{"approve"}, 2},
		{[]string{"approve", "five"}, 2},
		{[]string{"approve", "0"}, 2},
		{[]string{"approve", "5", "6"}, 2},
		{[]string{"approve", "5", "--note", "x"}, 2},
		{[]string{"reject", "--bogus", "5"}, 2},
	} {
		if code := cmdLane(c.args); code != c.code {
			t.Errorf("%v: %d, want %d", c.args, code, c.code)
		}
	}
	// With no run the command waits in the inbox, flags before the issue
	// or after it.
	for _, args := range [][]string{{"reject", "5", "--note", "Pin it."}, {"reject", "--note", "Pin it.", "5"}} {
		if code := cmdLane(args); code != 0 {
			t.Fatalf("%v: %d", args, code)
		}
	}
	pending, err := engine.Pending(".")
	if err != nil || len(pending) != 2 || pending[0].Kind != engine.CommandReject || pending[0].Issue != 5 || pending[0].Args["note"] != "Pin it." || pending[0].By != "awkit lane" {
		t.Errorf("pending: %+v, %v", pending, err)
	}
}

// What awkit lane says: the next run takes it when none goes; what the
// engine answered when one does, or that it has not answered yet.
func TestSendLane(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer
	if code := sendLane(&out, root, engine.Command{Kind: engine.CommandApprove, Issue: 7}, false, time.Second); code != 0 ||
		!strings.Contains(out.String(), "No run is going: the engine takes it") {
		t.Errorf("no run: %d %q", code, out.String())
	}
	out.Reset()
	if code := sendLane(&out, root, engine.Command{Kind: engine.CommandApprove, Issue: 7}, true, 50*time.Millisecond); code != 0 ||
		!strings.Contains(out.String(), "it has not answered yet") {
		t.Errorf("no answer: %d %q", code, out.String())
	}
}
