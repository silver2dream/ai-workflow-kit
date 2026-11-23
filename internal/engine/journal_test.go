package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func openJournal(t *testing.T, root string) *Journal {
	t.Helper()
	j, err := OpenJournal(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { j.Close() })
	return j
}

func record(t *testing.T, j *Journal, tr Transition) *Lane {
	t.Helper()
	if l := j.Lane(tr.Issue); l != nil {
		tr.From = l.State
	}
	lane, err := j.Record(tr)
	if err != nil {
		t.Fatalf("record %+v: %v", tr, err)
	}
	return lane
}

func eventLines(t *testing.T, root string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".ai", "state", "events", Session+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func TestJournal_RecordsAndRebuilds(t *testing.T) {
	root := t.TempDir()
	j := openJournal(t, root)
	if len(j.Lanes()) != 0 {
		t.Fatal("a new project has lanes")
	}
	record(t, j, Transition{Issue: 12, To: Ready, Cause: CauseOpened})
	record(t, j, Transition{Issue: 13, To: Blocked, Cause: CauseOpened})
	record(t, j, Transition{Issue: 12, To: Implementing, Cause: CauseDispatched})
	lane := record(t, j, Transition{Issue: 12, To: Reviewing, Cause: CauseImplemented, PR: 30})
	if lane.State != Reviewing || lane.PR != 30 || lane.Seq != 4 || lane.Since.IsZero() {
		t.Fatalf("lane = %+v", lane)
	}

	// The log has each transition, with the issue and the pull request.
	lines := eventLines(t, root)
	if len(lines) != 4 || !strings.Contains(lines[3], `"type":"lane_transition"`) || !strings.Contains(lines[3], `"issue_id":12`) || !strings.Contains(lines[3], `"pr_number":30`) || !strings.Contains(lines[3], `"cause":"implemented"`) {
		t.Errorf("log:\n%s", strings.Join(lines, "\n"))
	}

	// The snapshots are there for a reader.
	read, err := ReadLanes(root)
	if err != nil || len(read) != 2 || read[0].Issue != 12 || read[0].State != Reviewing || read[1].State != Blocked {
		t.Fatalf("snapshots: %+v, %v", read, err)
	}

	// Opened again, the journal reads the same lanes back from the log.
	j.Close()
	again := openJournal(t, root)
	got := again.Lane(12)
	if got == nil || got.State != Reviewing || got.PR != 30 || got.Seq != 4 {
		t.Errorf("rebuilt: %+v", got)
	}
	// And goes on numbering where it was.
	if l := record(t, again, Transition{Issue: 13, To: Ready, Cause: CauseUnblocked}); l.Seq != 5 {
		t.Errorf("seq after reopening = %d", l.Seq)
	}
}

func TestJournal_RefusesAndWritesNothing(t *testing.T) {
	root := t.TempDir()
	j := openJournal(t, root)
	record(t, j, Transition{Issue: 3, To: Ready, Cause: CauseOpened})
	if _, err := j.Record(Transition{Issue: 3, From: Ready, To: Merged, Cause: CauseMerged}); err == nil {
		t.Fatal("ready → merged was recorded")
	}
	// A decision taken on a state that moved since is refused.
	if _, err := j.Record(Transition{Issue: 3, From: Implementing, To: Reviewing, Cause: CauseImplemented}); err == nil {
		t.Fatal("a stale decision was recorded")
	}
	if n := len(eventLines(t, root)); n != 1 {
		t.Errorf("%d events, want only the one that applied", n)
	}
	if l := j.Lane(3); l.State != Ready {
		t.Errorf("lane = %+v", l)
	}
}

// The snapshots are what the log adds up to: one that is missing, old, or
// of a lane the log does not have is put right when the journal opens.
func TestJournal_SnapshotsFollowTheLog(t *testing.T) {
	root := t.TempDir()
	j := openJournal(t, root)
	record(t, j, Transition{Issue: 1, To: Ready, Cause: CauseOpened})
	record(t, j, Transition{Issue: 1, To: Implementing, Cause: CauseDispatched})
	record(t, j, Transition{Issue: 2, To: Ready, Cause: CauseOpened})
	j.Close()

	if err := os.Remove(snapshotPath(root, 2)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(snapshotPath(root, 1), []byte(`{"issue":1,"state":"ready","seq":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(snapshotPath(root, 77), []byte(`{"issue":77,"state":"reviewing","seq":9}`), 0o644); err != nil {
		t.Fatal(err)
	}
	openJournal(t, root)
	lanes, err := ReadLanes(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(lanes) != 2 || lanes[0].State != Implementing || lanes[0].Seq != 2 || lanes[1].Issue != 2 {
		t.Errorf("snapshots after opening: %+v", lanes)
	}
}

// A log whose transitions do not add up is not guessed at.
func TestRebuild_ALogThatDoesNotAddUp(t *testing.T) {
	root := t.TempDir()
	j := openJournal(t, root)
	record(t, j, Transition{Issue: 4, To: Ready, Cause: CauseOpened})
	j.Close()
	path := filepath.Join(root, ".ai", "state", "events", Session+".jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`{"seq":2,"ts":"2026-11-23T09:00:00Z","component":"engine","type":"lane_transition","level":"info","issue_id":4,"data":{"issue":4,"from":"reviewing","to":"merged","cause":"merged","at":"2026-11-23T09:00:00Z"}}` + "\n")
	f.Close()
	if _, err := OpenJournal(root); err == nil || !strings.Contains(err.Error(), "event 2") {
		t.Errorf("opened a log that does not add up: %v", err)
	}
}

func TestJournal_TimeOfARecord(t *testing.T) {
	j := openJournal(t, t.TempDir())
	at := time.Date(2026, 11, 23, 10, 30, 0, 0, time.UTC)
	j.now = func() time.Time { return at }
	if l := record(t, j, Transition{Issue: 6, To: Ready, Cause: CauseOpened}); !l.Since.Equal(at) {
		t.Errorf("since = %v", l.Since)
	}
}

func TestReadLanes_NoEngineYet(t *testing.T) {
	lanes, err := ReadLanes(t.TempDir())
	if err != nil || lanes == nil || len(lanes) != 0 {
		t.Errorf("lanes = %v, %v", lanes, err)
	}
}
