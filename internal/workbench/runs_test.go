package workbench

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/council"
	"github.com/silver2dream/ai-workflow-kit/internal/engine"
	"github.com/silver2dream/ai-workflow-kit/internal/kickoff"
	"github.com/silver2dream/ai-workflow-kit/internal/stopmark"
)

// runningLock says, as a run would, that one works on the project: this
// process stands in for it.
func runningLock(t *testing.T, root string) {
	t.Helper()
	lock, _ := json.Marshal(kickoff.LockInfo{PID: os.Getpid(), StartTime: time.Now(), Hostname: "here"})
	write(t, root, ".ai/state/kickoff.lock", string(lock))
}

// A run started from the page is `awkit run` in a process of its own, maybe
// for one ticket; not while one works on the project, nor while a meeting
// writes the documents or a plan is being approved.
func TestRuns_Start(t *testing.T) {
	s, root, base, spawn, _ := meetingProject(t, council.LiveFinished)
	write(t, root, ".ai/state/logs/workbench-run.log", "what the last run said\n")
	if w := do(t, s, http.MethodPost, base+"/run", `{}`); w.Code != http.StatusAccepted {
		t.Fatalf("start: %d %s", w.Code, w.Body)
	}
	dir, args := spawn.last()
	if dir != root || strings.Join(args, " ") != "run" || spawn.calls[0].log != runLog(root) {
		t.Errorf("spawned in %s: %v, log %s", dir, args, spawn.calls[0].log)
	}
	if _, err := os.Stat(runLog(root)); !os.IsNotExist(err) {
		t.Error("what the last run said is still there")
	}
	if w := do(t, s, http.MethodPost, base+"/run", `{"issue": 12}`); w.Code != http.StatusAccepted {
		t.Fatalf("start for one ticket: %d %s", w.Code, w.Body)
	}
	if _, args := spawn.last(); strings.Join(args, " ") != "run --issue 12" {
		t.Errorf("spawned %v", args)
	}
	if w := do(t, s, http.MethodPost, base+"/run", `{"issue": -1}`); w.Code != http.StatusBadRequest {
		t.Errorf("a ticket of no number: %d", w.Code)
	}

	calls := len(spawn.calls)
	runningLock(t, root)
	if w := do(t, s, http.MethodPost, base+"/run", `{}`); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "already") {
		t.Errorf("start while a run goes: %d %s", w.Code, w.Body)
	}
	if err := os.Remove(filepath.Join(root, ".ai", "state", "kickoff.lock")); err != nil {
		t.Fatal(err)
	}
	setLive(t, root, "p1", council.LiveMeeting)
	if w := do(t, s, http.MethodPost, base+"/run", `{}`); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "meeting of plan p1") {
		t.Errorf("start while a meeting goes: %d %s", w.Code, w.Body)
	}
	setLive(t, root, "p1", council.LiveFinished)
	if err := council.WriteApproval(root, "p1", &council.ApprovalRecord{PID: 4242, State: council.ApprovalWorking}); err != nil {
		t.Fatal(err)
	}
	if w := do(t, s, http.MethodPost, base+"/run", `{}`); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "being approved") {
		t.Errorf("start while a plan is approved: %d %s", w.Code, w.Body)
	}
	if len(spawn.calls) != calls {
		t.Errorf("a run was started though it was refused")
	}
}

// A run stops once the step it is on is done, as `awkit stop` asks; or at
// once, which ends it and what it started. The marker is left either way.
func TestRuns_Stop(t *testing.T) {
	s, root, base, _, _ := meetingProject(t, "")
	var killed []int
	s.opts.Kill = func(pid int) error {
		killed = append(killed, pid)
		return nil
	}
	if w := do(t, s, http.MethodPost, base+"/run/stop", `{}`); w.Code != http.StatusConflict {
		t.Errorf("stop with no run: %d %s", w.Code, w.Body)
	}
	if _, present := stopmark.Read(root); present {
		t.Error("a marker was left with no run to stop")
	}

	runningLock(t, root)
	w := do(t, s, http.MethodPost, base+"/run/stop", `{}`)
	if w.Code != http.StatusOK {
		t.Fatalf("stop: %d %s", w.Code, w.Body)
	}
	if v := decode[StopView](t, w); v.Engine || v.Ended {
		t.Errorf("stop: %+v", v)
	}
	if reason, present := stopmark.Read(root); !present || reason != stopmark.ByUser || len(killed) != 0 {
		t.Errorf("stop: marker %q %v, killed %v", reason, present, killed)
	}
	w = do(t, s, http.MethodPost, base+"/run/stop", `{"now": true}`)
	if w.Code != http.StatusOK {
		t.Fatalf("stop now: %d %s", w.Code, w.Body)
	}
	if v := decode[StopView](t, w); v.Engine || !v.Ended {
		t.Errorf("stop now: %+v", v)
	}
	if len(killed) != 1 || killed[0] != os.Getpid() {
		t.Errorf("killed %v, want the run's process %d", killed, os.Getpid())
	}
	if pending, _ := engine.Pending(root); len(pending) != 0 {
		t.Errorf("the Principal's run was given %+v", pending)
	}
}

// engineAt stands in for an engine that runs the project: its run file says
// it is alive, and, while answer is set, each command in its inbox is
// answered with what answer says, as the engine answers it.
type engineAt struct {
	root   string
	mu     sync.Mutex
	answer func(engine.Command) (result, reason string)
	given  []engine.Command
}

func runningEngine(t *testing.T, root string) *engineAt {
	t.Helper()
	runningLock(t, root)
	now := time.Now().UTC()
	run, _ := json.Marshal(engine.RunInfo{PID: os.Getpid(), Host: "here", Started: now, Beat: now})
	write(t, root, filepath.ToSlash(engine.RunFile), string(run))
	e := &engineAt{root: root, answer: func(engine.Command) (string, string) { return engine.Applied, "taken" }}
	done := make(chan struct{})
	t.Cleanup(func() { close(done) })
	go func() {
		for {
			select {
			case <-done:
				return
			case <-time.After(10 * time.Millisecond):
			}
			e.mu.Lock()
			answer := e.answer
			e.mu.Unlock()
			if answer == nil {
				continue
			}
			pending, _ := engine.Pending(root)
			for _, c := range pending {
				result, reason := answer(c)
				data, _ := json.Marshal(engine.Reply{ID: c.ID, Kind: c.Kind, Result: result, Reason: reason, At: time.Now().UTC()})
				out := filepath.Join(root, filepath.FromSlash(engine.OutboxDir))
				os.MkdirAll(out, 0o755)
				os.WriteFile(filepath.Join(out, c.ID+".json"), data, 0o644)
				os.Remove(filepath.Join(root, filepath.FromSlash(engine.InboxDir), c.ID+".json"))
				e.mu.Lock()
				e.given = append(e.given, c)
				e.mu.Unlock()
			}
		}
	}()
	return e
}

func (e *engineAt) set(answer func(engine.Command) (string, string)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.answer = answer
}

func (e *engineAt) last() engine.Command {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.given) == 0 {
		return engine.Command{}
	}
	return e.given[len(e.given)-1]
}

// The engine is given a stop through its inbox, as `awkit stop` gives it,
// and the page is told what it answered. It ends what it runs itself: its
// process is ended from here only when it does not answer a stop at once.
func TestRuns_StopTheEngine(t *testing.T) {
	s, root, base, _, _ := meetingProject(t, "")
	var killed []int
	s.opts.Kill = func(pid int) error {
		killed = append(killed, pid)
		return nil
	}
	s.opts.StopWait = 2 * time.Second
	e := runningEngine(t, root)

	w := do(t, s, http.MethodPost, base+"/run/stop", `{}`)
	if w.Code != http.StatusOK {
		t.Fatalf("stop: %d %s", w.Code, w.Body)
	}
	if v := decode[StopView](t, w); !v.Engine || !v.Answered || v.Reply == nil || v.Reply.Kind != engine.CommandStop || v.Reply.Reason != "taken" || v.Ended {
		t.Errorf("stop: %+v", v)
	}
	if c := e.last(); c.Kind != engine.CommandStop || c.By != "the workbench" {
		t.Errorf("the engine was given %+v", c)
	}
	if _, present := stopmark.Read(root); !present {
		t.Error("no marker: a run the engine outlives is not stopped")
	}

	w = do(t, s, http.MethodPost, base+"/run/stop", `{"now": true}`)
	if v := decode[StopView](t, w); w.Code != http.StatusOK || !v.Engine || !v.Answered || v.Reply.Kind != engine.CommandStopNow || v.Ended {
		t.Errorf("stop now: %d %+v", w.Code, v)
	}
	if len(killed) != 0 {
		t.Errorf("the engine's process was ended from here: %v", killed)
	}

	// A stop the engine refuses is said.
	e.set(func(engine.Command) (string, string) { return engine.Rejected, "the stop could not be written" })
	if w := do(t, s, http.MethodPost, base+"/run/stop", `{}`); w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), "the stop could not be written") {
		t.Errorf("refused: %d %s", w.Code, w.Body)
	}

	// An engine that does not answer: a stop waits for its next step, and
	// a stop at once ends its process.
	e.set(nil)
	s.opts.StopWait = 50 * time.Millisecond
	w = do(t, s, http.MethodPost, base+"/run/stop", `{}`)
	if v := decode[StopView](t, w); w.Code != http.StatusOK || !v.Engine || v.Answered || v.Ended || len(killed) != 0 {
		t.Errorf("stop, no answer: %d %+v, killed %v", w.Code, v, killed)
	}
	w = do(t, s, http.MethodPost, base+"/run/stop", `{"now": true}`)
	if v := decode[StopView](t, w); w.Code != http.StatusOK || !v.Engine || v.Answered || !v.Ended || len(killed) != 1 || killed[0] != os.Getpid() {
		t.Errorf("stop now, no answer: %d %+v, killed %v", w.Code, v, killed)
	}
}

// The page reads the engine's state as `awkit status` does: its run, its
// lanes with the limits of the project's configuration, its answers.
func TestEngineView(t *testing.T) {
	s, root, base, _, _ := meetingProject(t, "")
	w := do(t, s, http.MethodGet, base+"/engine", "")
	if w.Code != http.StatusOK {
		t.Fatalf("engine: %d %s", w.Code, w.Body)
	}
	for _, want := range []string{`"going":false`, `"lanes":[]`, `"pending":[]`, `"replies":[]`, `"limits":{"failures":3,"reviews":`} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("a project the engine has not run: %s has no %s", w.Body, want)
		}
	}

	runningEngine(t, root)
	now := time.Now().UTC()
	lane, _ := json.Marshal(engine.Lane{Issue: 5, State: engine.Reviewing, PR: 12, Attempts: engine.Attempts{Reviews: 1}, Since: now})
	write(t, root, filepath.ToSlash(engine.LanesDir)+"/5.json", string(lane))
	v := decode[engine.View](t, do(t, s, http.MethodGet, base+"/engine", ""))
	if !v.Going || v.Run == nil || len(v.Lanes) != 1 || v.Lanes[0].PR != 12 || v.Lanes[0].Attempts.Reviews != 1 {
		t.Errorf("engine = %+v", v)
	}

	// The limits are the configuration's; without one that can be read,
	// the defaults, and the engine's files are shown all the same.
	write(t, root, ".ai/config.yaml", testConfig+"escalation:\n  max_review_attempts: 5\n")
	if v := decode[engine.View](t, do(t, s, http.MethodGet, base+"/engine", "")); v.Limits.Reviews != 5 || v.Limits.Conflicts != 5 {
		t.Errorf("limits = %+v, want the configuration's 5", v.Limits)
	}
	write(t, root, ".ai/config.yaml", "version: [")
	w = do(t, s, http.MethodGet, base+"/engine", "")
	if v := decode[engine.View](t, w); w.Code != http.StatusOK || v.Limits.Reviews != 3 || len(v.Lanes) != 1 {
		t.Errorf("a configuration that cannot be read: %d %+v", w.Code, v)
	}
}

// The page reads the logs of a run a piece at a time: what was written since
// it read last, the end of a long log, from the start of one that started
// over; without the escapes of a terminal.
func TestRuns_Output(t *testing.T) {
	s, root, base, _, _ := meetingProject(t, "")
	write(t, root, ".ai/state/logs/principal.log", "\x1b[32m[PRINCIPAL]\x1b[0m dispatch #12\r\n")
	write(t, root, ".ai/state/logs/issue-12.worker.log", "writing the test\n")
	write(t, root, ".ai/state/logs/issue-9.worker.log", "old\n")
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(root, ".ai", "state", "logs", "issue-9.worker.log"), old, old); err != nil {
		t.Fatal(err)
	}

	logs := decode[map[string][]RunLogView](t, do(t, s, http.MethodGet, base+"/run/logs", ""))["logs"]
	var names []string
	for _, log := range logs {
		names = append(names, log.Name)
	}
	if strings.Join(names, ",") != "principal,issue-12,issue-9" {
		t.Errorf("logs = %v", names)
	}

	out := decode[RunOutput](t, do(t, s, http.MethodGet, base+"/run/output?log=principal", ""))
	if !out.Exists || out.Text != "[PRINCIPAL] dispatch #12\n" || out.Fresh {
		t.Errorf("output = %+v", out)
	}
	f, err := os.OpenFile(filepath.Join(root, ".ai", "state", "logs", "principal.log"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("worker done\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	more := decode[RunOutput](t, do(t, s, http.MethodGet, base+"/run/output?log=principal&since="+itoa(out.Next), ""))
	if more.Text != "worker done\n" || more.Fresh {
		t.Errorf("more = %+v", more)
	}
	// A log shorter than where the page was: it started over.
	again := decode[RunOutput](t, do(t, s, http.MethodGet, base+"/run/output?log=issue-9&since=999", ""))
	if again.Text != "old\n" || !again.Fresh {
		t.Errorf("again = %+v", again)
	}
	// A long log: its end.
	write(t, root, ".ai/state/logs/workbench-run.log", strings.Repeat("a line of the run\n", 10000))
	long := decode[RunOutput](t, do(t, s, http.MethodGet, base+"/run/output?log=run", ""))
	if !long.Fresh || len(long.Text) > maxOutput+64 || !strings.HasPrefix(long.Text, "…") || !strings.HasSuffix(long.Text, "a line of the run\n") {
		t.Errorf("long = %d bytes, fresh %v, starts %q", len(long.Text), long.Fresh, long.Text[:20])
	}
	if none := decode[RunOutput](t, do(t, s, http.MethodGet, base+"/run/output?log=issue-77", "")); none.Exists {
		t.Errorf("a log that is not there: %+v", none)
	}
	for _, bad := range []string{"", "issue-../../x", "issue-0", "../config"} {
		if w := do(t, s, http.MethodGet, base+"/run/output?log="+bad, ""); w.Code != http.StatusBadRequest {
			t.Errorf("log %q: %d", bad, w.Code)
		}
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
