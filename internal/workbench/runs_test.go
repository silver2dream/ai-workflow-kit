package workbench

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/council"
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
	if w := do(t, s, http.MethodPost, base+"/run/stop", `{}`); w.Code != http.StatusNoContent {
		t.Fatalf("stop: %d %s", w.Code, w.Body)
	}
	if reason, present := stopmark.Read(root); !present || reason != stopmark.ByUser || len(killed) != 0 {
		t.Errorf("stop: marker %q %v, killed %v", reason, present, killed)
	}
	if w := do(t, s, http.MethodPost, base+"/run/stop", `{"now": true}`); w.Code != http.StatusNoContent {
		t.Fatalf("stop now: %d %s", w.Code, w.Body)
	}
	if len(killed) != 1 || killed[0] != os.Getpid() {
		t.Errorf("killed %v, want the run's process %d", killed, os.Getpid())
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
