package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/layout"
	"github.com/silver2dream/ai-workflow-kit/internal/trace"
)

// The engine's history is its transitions, in the project's event log: the
// engine's own file of it (.ai/state/events/engine.jsonl), which awkit events
// reads with the rest. Each lane's state is also kept as a snapshot
// (.ai/state/engine/lanes/<issue>.json) for whoever reads it (the workbench,
// awkit status); a snapshot is what the log adds up to, and is written again
// from it whenever they could differ.

// Session is the event log file of the engine.
const Session = "engine"

// Component and event type of a transition in the event log.
const (
	Component      = "engine"
	TypeTransition = "lane_transition"
)

// LanesDir is where the snapshots are, under the project.
const LanesDir = layout.StateDir + "/engine/lanes"

// Journal records a project's lane transitions: in the event log first, then
// in the lane's snapshot. One engine holds a project's journal at a time
// (the engine's lock, task 1.6).
type Journal struct {
	root   string
	writer *trace.EventWriter
	now    func() time.Time

	mu    sync.Mutex
	lanes map[int]*Lane
}

// OpenJournal opens the journal of the project at root: it reads the lanes
// back from the event log and writes their snapshots again. A transition in
// the log that does not apply to its lane is an error: the log is the lanes'
// history, and one that does not add up is not guessed at.
func OpenJournal(root string) (*Journal, error) {
	lanes, err := Rebuild(root)
	if err != nil {
		return nil, err
	}
	writer, err := trace.NewEventWriter(root, Session)
	if err != nil {
		return nil, err
	}
	j := &Journal{root: root, writer: writer, now: func() time.Time { return time.Now().UTC() }, lanes: lanes}
	if err := j.writeSnapshots(); err != nil {
		writer.Close()
		return nil, err
	}
	return j, nil
}

// Close closes the event log.
func (j *Journal) Close() error {
	return j.writer.Close()
}

// Lane is a copy of the lane of an issue; nil when there is none.
func (j *Journal) Lane(issue int) *Lane {
	j.mu.Lock()
	defer j.mu.Unlock()
	if l, ok := j.lanes[issue]; ok {
		c := *l
		return &c
	}
	return nil
}

// Lanes are copies of every lane, by issue.
func (j *Journal) Lanes() []Lane {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]Lane, 0, len(j.lanes))
	for _, l := range j.lanes {
		out = append(out, *l)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Issue < out[b].Issue })
	return out
}

// Record moves a lane: the transition is checked against the lane, written
// to the event log, and then to the lane's snapshot. Its From must be the
// lane's state now, so that a decision taken on a state that has moved since
// is refused, not applied to the new one; At is the time now when it has
// none. A transition that does not apply is refused and nothing is written.
func (j *Journal) Record(t Transition) (*Lane, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if t.At.IsZero() {
		t.At = j.now()
	}
	next, err := Apply(j.lanes[t.Issue], t)
	if err != nil {
		return nil, err
	}
	level := trace.LevelInfo
	if t.To == NeedsYou {
		level = trace.LevelWarn
	}
	opts := []trace.EventOption{trace.WithIssue(t.Issue), trace.WithData(t)}
	if next.PR != 0 {
		opts = append(opts, trace.WithPR(next.PR))
	}
	if err := j.writer.Write(Component, TypeTransition, level, opts...); err != nil {
		return nil, fmt.Errorf("the transition of lane #%d was not recorded: %w", t.Issue, err)
	}
	next.Seq = j.writer.Seq()
	j.lanes[t.Issue] = next
	// The log has it: a snapshot that cannot be written now is written
	// again from the log when the journal is next opened.
	if err := writeSnapshot(j.root, next); err != nil {
		fmt.Fprintf(os.Stderr, "[engine] warning: the snapshot of lane #%d was not written: %v\n", t.Issue, err)
	}
	c := *next
	return &c, nil
}

// Rebuild reads the lanes back from the event log of the project at root.
func Rebuild(root string) (map[int]*Lane, error) {
	lanes := map[int]*Lane{}
	path := filepath.Join(root, layout.StateDir, "events", Session+".jsonl")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return lanes, nil
	}
	events, err := trace.NewEventReader(root).ReadSession(Session)
	if err != nil {
		return nil, err
	}
	for _, e := range events {
		if e.Component != Component || e.Type != TypeTransition {
			continue
		}
		t, err := transitionOf(e)
		if err != nil {
			return nil, fmt.Errorf("event %d of %s: %w", e.Seq, path, err)
		}
		next, err := Apply(lanes[t.Issue], t)
		if err != nil {
			return nil, fmt.Errorf("event %d of %s: %w", e.Seq, path, err)
		}
		next.Seq = e.Seq
		lanes[t.Issue] = next
	}
	return lanes, nil
}

func transitionOf(e trace.Event) (Transition, error) {
	var t Transition
	data, err := json.Marshal(e.Data)
	if err != nil {
		return t, err
	}
	if err := json.Unmarshal(data, &t); err != nil {
		return t, fmt.Errorf("not a transition: %w", err)
	}
	if t.Issue <= 0 || !t.To.Known() {
		return t, fmt.Errorf("not a transition: %s", data)
	}
	return t, nil
}

// writeSnapshots writes every lane's snapshot, and removes those of lanes
// the log does not have.
func (j *Journal) writeSnapshots() error {
	dir := filepath.Join(j.root, filepath.FromSlash(LanesDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, l := range j.lanes {
		if err := writeSnapshot(j.root, l); err != nil {
			return err
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		issue, err := strconv.Atoi(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if _, ok := j.lanes[issue]; !ok {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
	return nil
}

func snapshotPath(root string, issue int) string {
	return filepath.Join(root, filepath.FromSlash(LanesDir), strconv.Itoa(issue)+".json")
}

// writeSnapshot writes a lane's snapshot whole or not at all.
func writeSnapshot(root string, l *Lane) error {
	path := snapshotPath(root, l.Issue)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".lane-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// ReadLanes reads the snapshots of the project at root, for a reader that
// is not the engine: what the engine last wrote, by issue. A snapshot that
// cannot be read is named in the error, and the others are given all the
// same.
func ReadLanes(root string) ([]Lane, error) {
	dir := filepath.Join(root, filepath.FromSlash(LanesDir))
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return []Lane{}, nil
	}
	if err != nil {
		return nil, err
	}
	lanes := []Lane{}
	var bad []error
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			bad = append(bad, err)
			continue
		}
		var l Lane
		if err := json.Unmarshal(data, &l); err != nil {
			bad = append(bad, fmt.Errorf("%s: %w", entry.Name(), err))
			continue
		}
		lanes = append(lanes, l)
	}
	sort.Slice(lanes, func(a, b int) bool { return lanes[a].Issue < lanes[b].Issue })
	return lanes, errors.Join(bad...)
}

// Note writes an event of the engine that is no transition (a command, a
// run's start and end) to its log, beside the transitions. Rebuild passes
// it over.
func (j *Journal) Note(eventType, level string, issue int, data any) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	opts := []trace.EventOption{trace.WithData(data)}
	if issue != 0 {
		opts = append(opts, trace.WithIssue(issue))
	}
	return j.writer.Write(Component, eventType, level, opts...)
}
