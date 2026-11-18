package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/council"
)

func planProject(t *testing.T) string {
	t.Helper()
	origModel, origInput := planModel, planInput
	t.Cleanup(func() { planModel, planInput = origModel, origInput })
	dir := t.TempDir()
	writePlanConfig(t, dir, "repos:\n  - {name: backend, path: backend/, language: go}\n")
	return dir
}

// An owner who asks to see the agenda first is shown it; going on is their
// word that they have, and the agenda is checked before the meeting goes on.
func TestCmdPlan_TheAgendaWaitsForTheOwner(t *testing.T) {
	dir := planProject(t)
	planModel = &scriptedCouncil{}
	var code int
	out := captureStdout(t, func() {
		code = cmdPlan([]string{"--review-agenda", "--name", "p-one", "Add a health endpoint.", "--state-root", dir})
	})
	if code != 0 {
		t.Fatalf("code = %d:\n%s", code, out)
	}
	for _, want := range []string{"The agenda is set: docs/plans/p-one/agenda.yaml", "  1. What is built (", "Change it if you want", "awkit plan --resume p-one"} {
		if !strings.Contains(out, want) {
			t.Errorf("the output should have %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Plan p-one:") {
		t.Errorf("the meeting went on past the agenda:\n%s", out)
	}

	// An agenda the table cannot hold is not gone on with.
	agendaFile := filepath.Join(dir, "docs", "plans", "p-one", "agenda.yaml")
	good, err := os.ReadFile(agendaFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agendaFile, []byte(strings.Replace(string(good), "lead: ", "lead: nobody-", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = captureStdout(t, func() { code = cmdPlan([]string{"--resume", "p-one", "--state-root", dir}) })
	if code != 1 {
		t.Errorf("going on with a broken agenda = %d, want 1", code)
	}
	if err := os.WriteFile(agendaFile, good, 0o644); err != nil {
		t.Fatal(err)
	}
	out = captureStdout(t, func() { code = cmdPlan([]string{"--resume", "p-one", "--state-root", dir}) })
	if code != 0 || !strings.Contains(out, "Plan p-one: 1 topics") {
		t.Errorf("going on = %d:\n%s", code, out)
	}
}

// Questions put to the owner in the workbench stop the meeting; the person
// at the terminal answers them here when they go on with --ask.
func TestCmdPlan_QuestionsForTheWorkbench(t *testing.T) {
	dir := planProject(t)
	planModel = &scriptedCouncil{curious: true}
	var code int
	out := captureStdout(t, func() {
		code = cmdPlan([]string{"--ask-in-workbench", "--name", "p-q", "Add a health endpoint.", "--state-root", dir})
	})
	if code != 0 {
		t.Fatalf("code = %d:\n%s", code, out)
	}
	for _, want := range []string{"The council has 2 question(s) only you can answer:", "  1. Who calls the endpoint?", "awkit ui", "awkit plan --resume p-q --ask"} {
		if !strings.Contains(out, want) {
			t.Errorf("the output should have %q:\n%s", want, out)
		}
	}
	planInput = strings.NewReader("A readiness probe.\n\n")
	out = captureStdout(t, func() { code = cmdPlan([]string{"--resume", "p-q", "--ask", "--state-root", dir}) })
	if code != 0 || !strings.Contains(out, "1 answered, 1 left to the council to assume") || !strings.Contains(out, "Plan p-q:") {
		t.Errorf("going on with --ask = %d:\n%s", code, out)
	}
}

// What the owner says is put to the plan's meeting; a meeting that is not
// going on takes it in when it goes on.
func TestCmdPlanSayAndPause(t *testing.T) {
	dir := planProject(t)
	planModel = &scriptedCouncil{}
	var code int
	_ = captureStdout(t, func() {
		code = cmdPlan([]string{"--review-agenda", "--name", "p-one", "Add a health endpoint.", "--state-root", dir})
	})
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	out := captureStdout(t, func() {
		code = cmdPlan([]string{"say", "p-one", "Phones", "first.", "--to", "Who uses it?", "--state-root", dir})
	})
	if code != 0 || !strings.Contains(out, "Said to plan p-one (W1). The meeting is not going on") || !strings.Contains(out, "awkit plan --resume p-one") {
		t.Errorf("say = %d:\n%s", code, out)
	}
	words, err := council.Words(dir, "p-one")
	if err != nil || len(words) != 1 || words[0].Text != "Phones first." || words[0].Question != "Who uses it?" {
		t.Errorf("words = %+v, %v", words, err)
	}
	_ = captureStdout(t, func() { code = cmdPlan([]string{"say", "nothing", "hello", "--state-root", dir}) })
	if code != 1 {
		t.Errorf("say to a plan that is not there = %d, want 1", code)
	}
	_ = captureStdout(t, func() { code = cmdPlan([]string{"say", "p-one", "--state-root", dir}) })
	if code != 2 {
		t.Errorf("say nothing = %d, want 2", code)
	}
	_ = captureStdout(t, func() { code = cmdPlan([]string{"pause", "p-one", "--state-root", dir}) })
	if code != 1 {
		t.Errorf("pause a meeting that is not going on = %d, want 1", code)
	}

	// The word is taken in when the meeting goes on.
	out = captureStdout(t, func() { code = cmdPlan([]string{"--resume", "p-one", "--state-root", dir}) })
	if code != 0 || !strings.Contains(out, "The owner said (W1): Phones first.") {
		t.Errorf("going on = %d:\n%s", code, out)
	}
}

// Options that cannot go together are refused before anything is asked.
func TestCmdPlan_OptionsThatDoNotGoTogether(t *testing.T) {
	dir := planProject(t)
	planModel = &scriptedCouncil{}
	for _, args := range [][]string{
		{"--resume", "p1", "--name", "p2"},
		{"--ask", "--ask-in-workbench", "Add a health endpoint."},
	} {
		var code int
		_ = captureStdout(t, func() { code = cmdPlan(append(args, "--state-root", dir)) })
		if code != 2 {
			t.Errorf("%v = %d, want 2", args, code)
		}
	}
}

// awkit detach is how the workbench starts work; it takes its arguments in
// one form only.
func TestCmdDetach_Arguments(t *testing.T) {
	for _, args := range [][]string{nil, {"plan"}, {"--log", "x.log", "plan"}, {"--log", "x.log", "--"}} {
		var code int
		_ = captureStdout(t, func() { code = cmdDetach(args) })
		if code != 2 {
			t.Errorf("detach %v = %d, want 2", args, code)
		}
	}
}
