package trace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC)

func at(minutes int) time.Time { return t0.Add(time.Duration(minutes) * time.Minute) }

// writeSession writes the events of a session's file, named by their type.
func writeSession(t *testing.T, root, session string, events ...Event) {
	t.Helper()
	dir := filepath.Join(root, ".ai", "state", "events")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, e := range events {
		data, _ := json.Marshal(e)
		b.Write(data)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(dir, session+".jsonl"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func current(t *testing.T, root, session string) {
	t.Helper()
	dir := filepath.Join(root, ".ai", "state", "principal")
	os.MkdirAll(dir, 0o755)
	if err := os.WriteFile(filepath.Join(dir, "session.json"), []byte(`{"session_id":"`+session+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
}

func ev(minute int, component, typ string) Event {
	return Event{Timestamp: at(minute), Component: component, Type: typ, Level: LevelInfo}
}

func types(events []Event) string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = e.Type
	}
	return strings.Join(out, " ")
}

const (
	run1 = "principal-20261127-090000-aaaa1111"
	run2 = "principal-20261127-091000-bbbb2222"
	run3 = "principal-20261127-092000-cccc3333"
)

// A run's events are its session's and what the side files got while it
// went, in the order they happened: from its first event to its end, or,
// for the run that goes on, to now.
func TestReadRun(t *testing.T) {
	root := t.TempDir()
	writeSession(t, root, run1, ev(0, ComponentPrincipal, TypeSessionStart), ev(2, "reviewer", "review_start"), ev(5, ComponentPrincipal, TypeSessionEnd))
	writeSession(t, root, run2, ev(10, ComponentPrincipal, TypeSessionStart), ev(12, "worker", "dispatch_start"))
	current(t, root, run2)
	writeSession(t, root, SessionEngine,
		ev(-1, "engine", "before"), ev(3, "engine", "e-run1"), ev(6, "engine", "between"), ev(11, "engine", "e-run2"), ev(13, "engine", "e-after"))
	writeSession(t, root, SessionGuard, ev(4, ComponentGuard, "g-run1"), ev(14, ComponentGuard, "g-run2"))
	r := NewEventReader(root)

	got, err := r.ReadRun(run1, EventFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if want := "session_start review_start e-run1 g-run1 session_end"; types(got) != want {
		t.Errorf("the run that ended:\n got %s\nwant %s", types(got), want)
	}
	got, _ = r.ReadRun(run2, EventFilter{})
	if want := "session_start e-run2 dispatch_start e-after g-run2"; types(got) != want {
		t.Errorf("the run that goes on:\n got %s\nwant %s", types(got), want)
	}
	// Filtered, then the last of them.
	if got, _ := r.ReadRun(run2, EventFilter{Component: "engine"}); types(got) != "e-run2 e-after" {
		t.Errorf("the engine's: %s", types(got))
	}
	if got, _ := r.ReadRun(run2, EventFilter{Last: 2}); types(got) != "e-after g-run2" {
		t.Errorf("the last two: %s", types(got))
	}
	// A side file named is read alone, all of it.
	if got, _ := r.ReadRun(SessionEngine, EventFilter{}); types(got) != "before e-run1 between e-run2 e-after" {
		t.Errorf("the engine's file: %s", types(got))
	}
	// A run with no events has none of the side files' either.
	writeSession(t, root, run3)
	if got, err := r.ReadRun(run3, EventFilter{}); err != nil || len(got) != 0 {
		t.Errorf("an empty run: %v %v", types(got), err)
	}
	if _, err := r.ReadRun("principal-nothing", EventFilter{}); err == nil {
		t.Error("a run that is not there")
	}
}

// A run whose process died said no end: it ended, at the latest, when the
// next run started.
func TestReadRun_ARunThatDied(t *testing.T) {
	root := t.TempDir()
	writeSession(t, root, run1, ev(0, ComponentPrincipal, TypeSessionStart), ev(1, "worker", "dispatch_start"))
	writeSession(t, root, run2, ev(20, ComponentPrincipal, TypeSessionStart))
	current(t, root, run2)
	writeSession(t, root, SessionEngine, ev(3, "engine", "e-run1"), ev(19, "engine", "e-late"), ev(21, "engine", "e-run2"))
	r := NewEventReader(root)
	if got, _ := r.ReadRun(run1, EventFilter{}); types(got) != "session_start dispatch_start e-run1 e-late" {
		t.Errorf("the run that died: %s", types(got))
	}
	// The newest run, though not the current one, goes on.
	current(t, root, run1)
	if got, _ := r.ReadRun(run2, EventFilter{}); types(got) != "session_start e-run2" {
		t.Errorf("the newest run: %s", types(got))
	}
}

// A side file is read from its end, in pieces smaller than its lines, and
// reading stops at the first event older than the window.
func TestReadWindow(t *testing.T) {
	defer func(n int) { windowChunk = n }(windowChunk)
	windowChunk = 7
	root := t.TempDir()
	long := Event{Timestamp: at(8), Component: "engine", Type: "long", Data: map[string]any{"note": strings.Repeat("x", 300)}}
	writeSession(t, root, SessionEngine, ev(1, "engine", "old"), ev(5, "engine", "in-but-past-an-older-one"), ev(2, "engine", "older"), ev(6, "engine", "a"), long, ev(9, "engine", "b"), ev(12, "engine", "after"))
	path := NewEventReader(root).GetSessionFilePath(SessionEngine)
	got, err := readWindow(path, at(4), at(10), 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if types(got) != "a long b" {
		t.Errorf("the window: %s", types(got))
	}
	if got[1].Data.(map[string]any)["note"] != long.Data.(map[string]any)["note"] {
		t.Error("a line read in pieces is not whole")
	}
	if got, _ := readWindow(path, at(4), time.Time{}, 2, nil); types(got) != "b after" {
		t.Errorf("the latest two, no end: %s", types(got))
	}
	if got, _ := readWindow(path, at(4), time.Time{}, 0, func(e Event) bool { return e.Type != "long" }); types(got) != "a b after" {
		t.Errorf("kept: %s", types(got))
	}
	if got, err := readWindow(filepath.Join(root, "none.jsonl"), at(0), time.Time{}, 0, nil); got != nil || err != nil {
		t.Errorf("no file: %v %v", got, err)
	}
	// Every size of the pieces reads the same.
	for _, n := range []int{1, 2, 64, 1 << 16} {
		windowChunk = n
		if got, _ := readWindow(path, at(0), time.Time{}, 0, nil); types(got) != "old in-but-past-an-older-one older a long b after" {
			t.Errorf("pieces of %d: %s", n, types(got))
		}
	}
}

// The run shown when none is asked for: the current one, else the newest
// run, never a side file.
func TestLatestRun(t *testing.T) {
	root := t.TempDir()
	r := NewEventReader(root)
	writeSession(t, root, SessionGuard, ev(0, ComponentGuard, "g"))
	writeSession(t, root, SessionEngine, ev(0, "engine", "e"))
	if got := r.LatestRun(); got != "" {
		t.Errorf("only side files: %q", got)
	}
	writeSession(t, root, run1, ev(0, ComponentPrincipal, TypeSessionStart))
	writeSession(t, root, run2, ev(10, ComponentPrincipal, TypeSessionStart))
	if got := r.LatestRun(); got != run2 {
		t.Errorf("the newest: %q", got)
	}
	current(t, root, run1)
	if got := r.LatestRun(); got != run1 {
		t.Errorf("the current: %q", got)
	}
	current(t, root, "principal-gone")
	if got := r.LatestRun(); got != run2 {
		t.Errorf("a current one that left no file: %q", got)
	}
	for _, s := range []string{SessionEngine, SessionGuard, SessionAgent} {
		if !IsSide(s) {
			t.Errorf("%s is no side file", s)
		}
	}
	if IsSide(run1) {
		t.Errorf("%s is a side file", run1)
	}
}
