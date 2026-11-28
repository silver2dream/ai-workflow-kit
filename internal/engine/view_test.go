package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/config"
	"github.com/silver2dream/ai-workflow-kit/internal/guard"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
	"github.com/silver2dream/ai-workflow-kit/internal/trace"
)

// writeView lays out what an engine leaves in a project: its run file, and
// the snapshot of each lane.
func writeView(t *testing.T, root string, run *RunInfo, lanes ...Lane) {
	t.Helper()
	if run != nil {
		data, _ := json.Marshal(run)
		if err := writeAtomic(filepath.Join(root, filepath.FromSlash(layout.StateDir), "engine"), "run.json", data); err != nil {
			t.Fatal(err)
		}
	}
	for _, l := range lanes {
		data, _ := json.Marshal(l)
		if err := writeAtomic(filepath.Join(root, filepath.FromSlash(LanesDir)), fmt.Sprintf("%d.json", l.Issue), data); err != nil {
			t.Fatal(err)
		}
	}
}

// writeEvents appends events to one of the project's event files.
func writeEvents(t *testing.T, root, session string, events ...trace.Event) string {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(layout.StateDir), "events")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, session+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, ev := range events {
		data, _ := json.Marshal(ev)
		f.Write(append(data, '\n'))
	}
	return path
}

func verdictEvent(at time.Time, issue int, v guard.Verdict) trace.Event {
	return trace.Event{Timestamp: at, Component: trace.ComponentGuard, Type: trace.TypeGuardrailVerdict, Level: trace.LevelWarn, IssueID: issue, Data: v}
}

func issues(lanes []LaneView) []int {
	out := make([]int, len(lanes))
	for i, l := range lanes {
		out[i] = l.Issue
	}
	return out
}

// A project the engine has not run shows nothing of it, and nothing is
// wrong with that.
func TestLook_NoEngine(t *testing.T) {
	v := Look(t.TempDir(), nil, time.Now())
	if v.Run != nil || v.Going || len(v.Lanes) != 0 || v.Earlier != 0 || v.Shadow != nil || len(v.Pending) != 0 || len(v.Replies) != 0 || len(v.Problems) != 0 {
		t.Errorf("view = %+v", v)
	}
	if v.Lanes == nil || v.Pending == nil || v.Replies == nil {
		t.Errorf("a list is null for the page: %+v", v)
	}
	if want := (Limits{Failures: 3, Reviews: analyzer.DefaultMaxReviewAttempts, Conflicts: analyzer.DefaultMaxReviewAttempts, Checks: analyzer.DefaultMaxReviewAttempts}); v.Limits != want {
		t.Errorf("limits = %+v, want %+v", v.Limits, want)
	}
}

// The lanes shown are those not over and those that ended in the last run,
// the ones the engine is on first, then the ones that need a person, the
// rest by number, and what ended last at the end, the latest first. A step is
// its lane's only while the run goes.
func TestLook_TheLanesOfTheLastRun(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UTC()
	started := now.Add(-time.Hour)
	run := &RunInfo{PID: os.Getpid(), Started: started, Beat: now, Steps: []StepInfo{{Action: analyzer.ActionReviewPR, Issue: 5, PR: 12, Since: now.Add(-time.Minute)}}}
	writeView(t, root, run,
		Lane{Issue: 3, State: Ready, Since: started.Add(time.Minute)},
		Lane{Issue: 5, State: Reviewing, PR: 12, Attempts: Attempts{Reviews: 1}, Since: now.Add(-2 * time.Minute)},
		Lane{Issue: 7, State: NeedsYou, PR: 14, Why: WhyReviewMaxRetries, Note: "the review of PR #14 was tried 3 times", Since: started.Add(2 * time.Minute)},
		Lane{Issue: 2, State: Merged, PR: 9, Since: started.Add(-time.Minute)}, // before the run
		Lane{Issue: 8, State: Merged, PR: 11, Since: started.Add(10 * time.Minute)},
		Lane{Issue: 9, State: Cancelled, Since: started.Add(20 * time.Minute)},
		Lane{Issue: 1, State: Blocked, Since: started.Add(-24 * time.Hour)}, // not over, however old
	)
	v := Look(root, nil, now)
	if !v.Going || v.Run == nil || len(v.Run.Steps) != 1 || v.Run.Steps[0].PR != 12 {
		t.Fatalf("view = %+v", v)
	}
	if got, want := fmt.Sprint(issues(v.Lanes)), "[5 7 1 3 9 8]"; got != want {
		t.Errorf("lanes = %s, want %s", got, want)
	}
	if v.Earlier != 1 {
		t.Errorf("earlier = %d, want 1", v.Earlier)
	}
	for _, l := range v.Lanes {
		if (l.Step != nil) != (l.Issue == 5) {
			t.Errorf("#%d: step %+v", l.Issue, l.Step)
		}
	}
	if l := v.Lanes[1]; l.Why != WhyReviewMaxRetries || l.Note == "" || l.PR != 14 {
		t.Errorf("the lane that needs a person = %+v", l)
	}
	if len(v.Problems) != 0 {
		t.Errorf("problems = %v", v.Problems)
	}

	// A run that ended, one whose process is gone, and one that no longer
	// beats are on no step; a decision that settles a lane on the way to the
	// end of the run is no step on it.
	for name, change := range map[string]func(r *RunInfo){
		"ended":    func(r *RunInfo) { r.Ended = now },
		"gone":     func(r *RunInfo) { r.PID = 1 << 30 },
		"no beat":  func(r *RunInfo) { r.Beat = now.Add(-time.Hour) },
		"settling": func(r *RunInfo) { r.Steps = []StepInfo{{Action: analyzer.ActionNone, Issue: 5}} },
		"the end":  func(r *RunInfo) { r.Steps = []StepInfo{{Action: analyzer.ActionAllComplete}} },
		"none":     func(r *RunInfo) { r.Steps = nil },
	} {
		r := *run
		change(&r)
		writeView(t, root, &r)
		v := Look(root, nil, now)
		for _, l := range v.Lanes {
			if l.Step != nil {
				t.Errorf("%s: #%d is on step %+v", name, l.Issue, l.Step)
			}
		}
		if got := fmt.Sprint(issues(v.Lanes)); got != "[7 1 3 5 9 8]" {
			t.Errorf("%s: lanes = %s", name, got)
		}
	}

	// The engine resolves and writes as well as reviews.
	for _, action := range []string{analyzer.ActionDispatchWorker, analyzer.ActionCheckResult} {
		r := *run
		r.Steps = []StepInfo{{Action: action, Issue: 3, Since: now}}
		writeView(t, root, &r)
		if v := Look(root, nil, now); v.Lanes[0].Issue != 3 || v.Lanes[0].Step == nil || v.Lanes[0].Step.Action != action {
			t.Errorf("%s: lanes = %v", action, issues(v.Lanes))
		}
	}

	// Lanes run side by side: each one the engine is on has its step, and
	// they come first, by number.
	r := *run
	r.Steps = []StepInfo{
		{Action: analyzer.ActionReviewPR, Issue: 5, PR: 12, Since: now.Add(-time.Minute)},
		{Action: analyzer.ActionDispatchWorker, Issue: 3, Since: now.Add(-3 * time.Minute)},
		{Action: analyzer.ActionCheckResult, Issue: 1, Since: now},
	}
	writeView(t, root, &r)
	v = Look(root, nil, now)
	if got := fmt.Sprint(issues(v.Lanes)); got != "[1 3 5 7 9 8]" {
		t.Errorf("three steps: lanes = %s", got)
	}
	for _, l := range v.Lanes[:3] {
		if l.Step == nil || l.Step.Issue != l.Issue {
			t.Errorf("three steps: #%d is on %+v", l.Issue, l.Step)
		}
	}
	if v.Lanes[1].Step.Action != analyzer.ActionDispatchWorker || v.Lanes[2].Step.PR != 12 {
		t.Errorf("three steps: the steps are not their lanes': %+v, %+v", v.Lanes[1].Step, v.Lanes[2].Step)
	}
}

// A run file an awkit before task 2.2b wrote names one step: it is read as
// the steps.
func TestReadRun_OneStepOfAnEarlierAwkit(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)
	dir := filepath.Join(root, filepath.FromSlash(layout.StateDir), "engine")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	earlier := fmt.Sprintf(`{"pid": 7, "host": "h", "started": %q, "beat": %q, "step": {"action": "review_pr", "issue": 5, "pr": 12, "since": %q}}`,
		now.Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err := os.WriteFile(filepath.Join(dir, "run.json"), []byte(earlier), 0o644); err != nil {
		t.Fatal(err)
	}
	r, ok := ReadRun(root)
	if !ok || r.Step != nil || len(r.Steps) != 1 || r.Steps[0] != (StepInfo{Action: analyzer.ActionReviewPR, Issue: 5, PR: 12, Since: now}) {
		t.Errorf("run = %+v (%v)", r, ok)
	}
	// One that names its steps keeps them, and none is none.
	both := fmt.Sprintf(`{"pid": 7, "steps": [{"action": "check_result", "issue": 3, "since": %q}], "step": {"action": "review_pr", "issue": 5, "since": %q}}`,
		now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err := os.WriteFile(filepath.Join(dir, "run.json"), []byte(both), 0o644); err != nil {
		t.Fatal(err)
	}
	if r, ok := ReadRun(root); !ok || r.Step != nil || len(r.Steps) != 1 || r.Steps[0].Issue != 3 {
		t.Errorf("run = %+v (%v)", r, ok)
	}
	if err := os.WriteFile(filepath.Join(dir, "run.json"), []byte(`{"pid": 7}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if r, ok := ReadRun(root); !ok || r.Steps != nil {
		t.Errorf("run = %+v (%v)", r, ok)
	}
}

// Without a run, every lane that is over ended before it.
// The lanes in the merge queue are shown with their place in it, and the
// merge that runs is a step on its lane.
func TestLook_TheMergeQueue(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UTC()
	run := &RunInfo{PID: os.Getpid(), Started: now.Add(-time.Hour), Beat: now, Steps: []StepInfo{{Action: ActionMerge, Issue: 6, PR: 16, Since: now.Add(-time.Minute)}}}
	writeView(t, root, run,
		Lane{Issue: 4, State: Queued, PR: 14, Since: now.Add(-3 * time.Minute)},
		Lane{Issue: 5, State: Queued, PR: 15, Since: now.Add(-2 * time.Minute)},
		Lane{Issue: 6, State: Queued, PR: 16, Since: now.Add(-9 * time.Minute)},
		Lane{Issue: 7, State: Reviewing, PR: 17, Since: now.Add(-9 * time.Minute)},
	)
	v := Look(root, nil, now)
	places := map[int]int{}
	for _, l := range v.Lanes {
		places[l.Issue] = l.Queue
		if (l.Step != nil) != (l.Issue == 6) {
			t.Errorf("#%d: step %+v", l.Issue, l.Step)
		}
	}
	if fmt.Sprint(places) != "map[4:2 5:3 6:1 7:0]" {
		t.Errorf("places = %v", places)
	}
}

func TestLook_LanesWithoutARun(t *testing.T) {
	root := t.TempDir()
	writeView(t, root, nil, Lane{Issue: 4, State: NeedsYou, Why: WhyPerson, Since: time.Now()}, Lane{Issue: 2, State: Merged, Since: time.Now()})
	v := Look(root, nil, time.Now())
	if v.Run != nil || v.Going || fmt.Sprint(issues(v.Lanes)) != "[4]" || v.Earlier != 1 {
		t.Errorf("view = %+v", v)
	}
}

// The limits are the engine's: the review and the conflicts by
// escalation.max_review_attempts, the Worker's failures by a dispatch's.
func TestLook_Limits(t *testing.T) {
	cfg := config.Default()
	cfg.Escalation.MaxReviewAttempts = 5
	if got, want := Look(t.TempDir(), cfg, time.Now()).Limits, (Limits{Failures: 3, Reviews: 5, Conflicts: 5, Checks: 5}); got != want {
		t.Errorf("limits = %+v, want %+v", got, want)
	}
	cfg.Escalation.MaxReviewAttempts = 0
	if got := LimitsOf(cfg); got.Reviews != analyzer.DefaultMaxReviewAttempts {
		t.Errorf("limits = %+v", got)
	}
	// The Worker's limit shown is the one its step holds it to.
	w := &WorkerSteps{}
	w.defaults()
	if got := LimitsOf(cfg).Failures; w.MaxRetries != got {
		t.Errorf("the Worker may fail %d times; the view says %d", w.MaxRetries, got)
	}
}

// A lane carries the latest verdict on its ticket in the last run that was
// not a pass, wherever it was written: the run's session, or the guard's own
// file. A verdict before the run, one of a ticket that has no lane shown,
// and a file not written since the run started are not looked at.
func TestLook_TheLatestVerdict(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UTC()
	started := now.Add(-time.Hour)
	writeView(t, root, &RunInfo{PID: os.Getpid(), Started: started, Beat: now},
		Lane{Issue: 5, State: Reviewing, PR: 12, Since: started},
		Lane{Issue: 7, State: NeedsYou, Why: WhyGuardBlocked, Since: started},
		Lane{Issue: 8, State: Rework, Since: started},
		Lane{Issue: 2, State: Merged, Since: started.Add(-time.Hour)},
	)
	flag := guard.Verdict{Result: guard.Flag, Class: guard.Security, Stage: guard.StageInput, Rule: "injection-rules/1:ignore-previous", Source: "issue #5 comment", Reason: "it reads as an instruction"}
	block := guard.Verdict{Result: guard.Block, Class: guard.Security, Stage: guard.StageOutput, Rule: "secret-scan/1", Reason: "a token"}
	writeEvents(t, root, "principal-20261124-102442-f44cb241",
		verdictEvent(started.Add(-time.Minute), 8, block), // before the run
		trace.Event{Timestamp: started.Add(time.Minute), Component: "worker", Type: "start", IssueID: 5, Data: map[string]any{"note": "guardrail_verdict is no verdict here"}},
		verdictEvent(started.Add(2*time.Minute), 5, flag),
		verdictEvent(started.Add(3*time.Minute), 5, guard.Verdict{Result: guard.Pass, Rule: "r"}),
		verdictEvent(started.Add(4*time.Minute), 2, block),  // its lane ended before the run
		verdictEvent(started.Add(5*time.Minute), 99, block), // no lane
		// Earlier than #7's block, in a file read after the guard's.
		verdictEvent(started.Add(30*time.Second), 7, flag),
	)
	writeEvents(t, root, guard.Session,
		verdictEvent(started.Add(6*time.Minute), 7, block),
		verdictEvent(started.Add(time.Minute), 5, block), // earlier than the flag
	)
	old := writeEvents(t, root, "principal-20261123-175640-f2021308", verdictEvent(started.Add(7*time.Minute), 8, block))
	if err := os.Chtimes(old, started.Add(-time.Hour), started.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, filepath.FromSlash(layout.StateDir), "events", "notes.txt"), []byte("guardrail_verdict"), 0o644)

	v := Look(root, nil, now)
	if len(v.Problems) != 0 {
		t.Fatalf("problems = %v", v.Problems)
	}
	got := map[int]*LaneVerdict{}
	for _, l := range v.Lanes {
		got[l.Issue] = l.Verdict
	}
	if g := got[5]; g == nil || g.Result != guard.Flag || g.Rule != flag.Rule || g.Source != flag.Source || !g.At.Equal(started.Add(2*time.Minute)) {
		t.Errorf("#5: verdict %+v", g)
	}
	if g := got[7]; g == nil || g.Result != guard.Block || g.Rule != block.Rule {
		t.Errorf("#7: verdict %+v", g)
	}
	if g := got[8]; g != nil {
		t.Errorf("#8: verdict %+v, want none", g)
	}
	if _, shown := got[2]; shown {
		t.Errorf("#2 is shown: %v", issues(v.Lanes))
	}

	// The JSON a page reads: the verdict's fields and its time beside the
	// lane's.
	data, _ := json.Marshal(v.Lanes[0])
	for _, want := range []string{`"issue":`, `"state":`, `"verdict":{"result":`, `"rule":`, `"at":`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("lane JSON %s has no %s", data, want)
		}
	}
}

// The engine's inbox and answers: what waits for an answer, oldest first,
// and the latest answers, newest first, as many as a view lists.
func TestLook_CommandsAndReplies(t *testing.T) {
	root := t.TempDir()
	at := time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC)
	for i := 0; i < MaxReplies+2; i++ {
		c, err := Send(root, Command{Kind: CommandStop, By: fmt.Sprintf("test %d", i), At: at.Add(time.Duration(i) * time.Second)})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := answer(root, c, Applied, fmt.Sprintf("answer %d", i), c.At); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if _, err := Send(root, Command{Kind: CommandStopNow, By: fmt.Sprintf("waiting %d", i), At: at.Add(time.Minute + time.Duration(i)*time.Second)}); err != nil {
			t.Fatal(err)
		}
	}
	v := Look(root, nil, at)
	if len(v.Replies) != MaxReplies || v.Replies[0].Reason != fmt.Sprintf("answer %d", MaxReplies+1) || v.Replies[MaxReplies-1].Reason != "answer 2" {
		t.Errorf("replies = %+v", v.Replies)
	}
	if len(v.Pending) != 2 || v.Pending[0].By != "waiting 0" || v.Pending[1].By != "waiting 1" {
		t.Errorf("pending = %+v", v.Pending)
	}
}

// The shadow of the last run is part of the view.
func TestLook_TheShadow(t *testing.T) {
	root := t.TempDir()
	j, err := OpenJournal(root)
	if err != nil {
		t.Fatal(err)
	}
	j.Note(TypeRun, trace.LevelInfo, 0, map[string]any{"event": "started"})
	j.Note(TypeShadow, trace.LevelInfo, 0, Comparison{Same: true})
	j.Note(TypeShadow, trace.LevelWarn, 0, Comparison{Engine: &analyzer.Decision{NextAction: analyzer.ActionReviewPR, IssueNumber: 5, PRNumber: 12}, Analyzer: &analyzer.Decision{NextAction: analyzer.ActionNone}})
	j.Close()
	v := Look(root, nil, time.Now())
	if v.Shadow == nil || v.Shadow.Steps != 2 || v.Shadow.Same != 1 || len(v.Shadow.Differences) != 1 {
		t.Errorf("shadow = %+v", v.Shadow)
	}

	// A run whose decisions were all the same has an empty list of
	// differences, not none: the page reads it as a list.
	j, err = OpenJournal(root)
	if err != nil {
		t.Fatal(err)
	}
	j.Note(TypeRun, trace.LevelInfo, 0, map[string]any{"event": "started"})
	j.Note(TypeShadow, trace.LevelInfo, 0, Comparison{Same: true})
	j.Close()
	data, _ := json.Marshal(Look(root, nil, time.Now()).Shadow)
	if !strings.Contains(string(data), `"differences":[]`) {
		t.Errorf("shadow = %s", data)
	}
}

// What cannot be read is said, and the rest is shown.
func TestLook_WhatCannotBeRead(t *testing.T) {
	root := t.TempDir()
	writeView(t, root, nil, Lane{Issue: 4, State: Ready})
	os.WriteFile(filepath.Join(root, filepath.FromSlash(LanesDir), "6.json"), []byte("{"), 0o644)
	Send(root, Command{Kind: CommandStop, By: "test"})
	v := Look(root, nil, time.Now())
	if len(v.Problems) != 1 || !strings.Contains(v.Problems[0], "lanes") {
		t.Errorf("problems = %v", v.Problems)
	}
	if fmt.Sprint(issues(v.Lanes)) != "[4]" {
		t.Errorf("lanes = %v", issues(v.Lanes))
	}
	if len(v.Pending) != 1 {
		t.Errorf("pending = %+v", v.Pending)
	}
}
