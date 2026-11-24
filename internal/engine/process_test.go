package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/stopmark"
)

func TestInbox(t *testing.T) {
	root := t.TempDir()
	first, err := Send(root, Command{Kind: CommandStop, By: "awkit stop"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Send(root, Command{Kind: "pause", Issue: 12, By: "workbench", At: first.At.Add(time.Millisecond)})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, filepath.FromSlash(InboxDir), "zz-not-a-command.json"), "{")
	writeFile(t, filepath.Join(root, filepath.FromSlash(InboxDir), ".write-123"), "half")

	pending, err := Pending(root)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range pending {
		got = append(got, c.Kind+"/"+c.By)
	}
	if strings.Join(got, " ") != "stop/awkit stop pause/workbench /" || pending[0].ID != first.ID || pending[1].ID != second.ID {
		t.Fatalf("pending: %v", got)
	}

	r, err := answer(root, first, Applied, "stopping", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if back, ok := ReplyOf(root, first.ID); !ok || back.Result != Applied || back.Kind != CommandStop || back != r {
		t.Errorf("reply: %+v, %v", back, ok)
	}
	if pending, _ := Pending(root); len(pending) != 2 {
		t.Errorf("the answered command is still in the inbox: %+v", pending)
	}
	if _, err := Send(root, Command{ID: "../out", Kind: CommandStop}); err == nil {
		t.Error("a command whose id names a path")
	}
}

// stopAnalyzer stops when the stop marker says so, as the analyzer does, and
// dispatches #12 otherwise.
type stopAnalyzer struct{ root string }

func (a stopAnalyzer) Precheck() *analyzer.Decision {
	if reason, stopped := stopmark.Read(a.root); stopped {
		return &analyzer.Decision{NextAction: analyzer.ActionNone, ExitReason: reason}
	}
	return nil
}
func (a stopAnalyzer) NextTicket(ctx context.Context) (*analyzer.Decision, error) {
	return dispatch(12), nil
}
func (a stopAnalyzer) PRNumber(int, string) int { return 0 }

// serveFixture is an engine whose Worker takes its time, served with quick
// beats and polls.
func serveFixture(t *testing.T, work func(ctx context.Context) error) *engineFixture {
	t.Helper()
	f := newEngineFixture(t, task(12))
	f.e.Analyzer = stopAnalyzer{root: f.e.Root}
	f.e.Implement = func(ctx context.Context, l Lane) (Transition, error) {
		if err := work(ctx); err != nil {
			return Transition{}, err
		}
		return Transition{Issue: l.Issue, From: l.State, To: Rework, Cause: CauseImplementFail}, nil
	}
	return f
}

var quick = ServeOptions{Beat: 20 * time.Millisecond, Poll: 10 * time.Millisecond}

// A stop at once ends what is running, and the run, and is answered.
func TestServe_StopNow(t *testing.T) {
	started := make(chan struct{})
	var once sync.Once
	f := serveFixture(t, func(ctx context.Context) error {
		once.Do(func() { close(started) })
		<-ctx.Done()
		return ctx.Err()
	})
	sent := make(chan Command, 1)
	go func() {
		<-started
		// The run file says what it is at.
		if r, ok := ReadRun(f.e.Root); !ok || r.Step == nil || r.Step.Action != analyzer.ActionDispatchWorker || r.Step.Issue != 12 || !r.Alive(f.e.clock()) {
			t.Errorf("run file while at work: %+v, %v", r, ok)
		}
		c, _ := Send(f.e.Root, Command{Kind: CommandStopNow, By: "awkit stop --now"})
		sent <- c
	}()
	end, err := Serve(context.Background(), f.e, quick)
	if end != EndInterrupted || err != nil {
		t.Fatalf("ended %q, %v", end, err)
	}
	c := <-sent
	if r, ok := ReplyOf(f.e.Root, c.ID); !ok || r.Result != Applied {
		t.Errorf("reply: %+v, %v", r, ok)
	}
	r, _ := ReadRun(f.e.Root)
	if r.Ended.IsZero() || r.Reason != EndInterrupted || r.Step != nil || r.Alive(f.e.clock()) {
		t.Errorf("run file after: %+v", r)
	}
	if l := f.lane(12); l.State != Implementing {
		t.Errorf("the lane the stop caught at work: %+v (it is taken up from there)", l)
	}
	if len(eventsWith(t, f.e.Root, `"type":"engine_run"`)) != 2 || len(eventsWith(t, f.e.Root, `"type":"engine_command"`)) != 1 {
		t.Error("the run and the command are not in the event log")
	}
}

// A stop lets the step that is going finish, then ends the run.
func TestServe_Stop(t *testing.T) {
	steps := 0
	var c Command
	var f *engineFixture
	f = serveFixture(t, func(ctx context.Context) error {
		steps++
		if steps == 1 {
			c, _ = Send(f.e.Root, Command{Kind: CommandStop, By: "awkit stop"})
			deadline := time.Now().Add(5 * time.Second)
			for {
				if _, ok := ReplyOf(f.e.Root, c.ID); ok || time.Now().After(deadline) {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
		}
		return nil
	})
	end, err := Serve(context.Background(), f.e, quick)
	if end != stopmark.ByUser || err != nil || steps != 1 {
		t.Fatalf("ended %q, %v after %d steps", end, err, steps)
	}
	if r, ok := ReplyOf(f.e.Root, c.ID); !ok || r.Result != Applied {
		t.Errorf("reply: %+v", r)
	}
	if l := f.lane(12); l.State != Rework {
		t.Errorf("the step that was going did not finish: %+v", l)
	}
}

// What comes with the workbench's controls is refused until then, and said
// why.
func TestServe_RejectsWhatComesLater(t *testing.T) {
	f := serveFixture(t, func(ctx context.Context) error { return nil })
	c, _ := Send(f.e.Root, Command{Kind: "pause", Issue: 12, By: "workbench"})
	stop, _ := Send(f.e.Root, Command{Kind: CommandStop, By: "test", At: c.At.Add(time.Millisecond)})
	if end, err := Serve(context.Background(), f.e, quick); err != nil || end != stopmark.ByUser {
		t.Fatalf("ended %q, %v", end, err)
	}
	if r, _ := ReplyOf(f.e.Root, c.ID); r.Result != Rejected || !strings.Contains(r.Reason, "2.6") {
		t.Errorf("pause: %+v", r)
	}
	if r, _ := ReplyOf(f.e.Root, stop.ID); r.Result != Applied {
		t.Errorf("stop: %+v", r)
	}
}

func TestRunInfo_Alive(t *testing.T) {
	now := time.Now()
	live := RunInfo{PID: os.Getpid(), Beat: now}
	if !live.Alive(now) {
		t.Error("a beating run is not alive")
	}
	for name, r := range map[string]RunInfo{
		"ended":      {PID: os.Getpid(), Beat: now, Ended: now},
		"stale beat": {PID: os.Getpid(), Beat: now.Add(-time.Hour)},
		"no process": {PID: 0, Beat: now},
	} {
		if r.Alive(now) {
			t.Errorf("%s: alive", name)
		}
	}
	if _, ok := ReadRun(t.TempDir()); ok {
		t.Error("a run file where there is none")
	}
}
