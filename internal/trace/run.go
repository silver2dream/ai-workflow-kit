package trace

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"time"
)

// A run's events are in more than one file. Its session's file has what
// `awkit run` and the code in its process write (the engine's review and
// resolve steps among them). Processes that have no stream of the run write
// to files of their own, the side files, whatever run is going: ReadRun
// takes from them what they got while the run went.
const (
	// SessionEngine is the engine's file: its lanes' transitions, its runs'
	// starts and ends, its commands, its shadow.
	SessionEngine = "engine"
	// SessionGuard is where a verdict goes that is made where there is no
	// stream: a hook (`awkit guard tool`), a Worker's `awkit run-issue`.
	SessionGuard = "guard"
	// SessionAgent is where an agent that runs outside a run says it ran.
	SessionAgent = "agent"
)

var sideSessions = []string{SessionEngine, SessionGuard, SessionAgent}

// IsSide reports whether a session is one of the side files, not a run's.
func IsSide(session string) bool {
	return slices.Contains(sideSessions, session)
}

// windowChunk is how much of a side file is read at a time, from its end.
var windowChunk = 64 * 1024

// ReadRun reads the events of a run: those of its session, and those the
// side files got while it went, in the order they happened, then filtered.
// The run went from its session's first event to its session_end; a session
// with no end goes on while it is the current one, and otherwise ended when
// the next run started. A side file is named for itself.
func (r *EventReader) ReadRun(sessionID string, filter EventFilter) ([]Event, error) {
	if IsSide(sessionID) {
		return r.ReadSessionFiltered(sessionID, filter)
	}
	events, err := r.ReadSession(sessionID)
	if err != nil {
		return nil, err
	}
	last := filter.Last
	filter.Last = 0
	out := r.applyFilter(events, filter)
	if len(events) > 0 {
		since, until := events[0].Timestamp, r.endOf(sessionID, events)
		for _, side := range sideSessions {
			got, err := readWindow(r.GetSessionFilePath(side), since, until, last, func(e Event) bool { return r.matchesFilter(e, filter) })
			if err != nil {
				return nil, err
			}
			out = append(out, got...)
		}
		sort.SliceStable(out, func(i, j int) bool { return out[i].Timestamp.Before(out[j].Timestamp) })
	}
	if last > 0 && len(out) > last {
		out = out[len(out)-last:]
	}
	return out, nil
}

// endOf says when a run whose session has these events ended: zero while
// it goes on.
func (r *EventReader) endOf(sessionID string, events []Event) time.Time {
	for _, e := range events {
		if e.Type == TypeSessionEnd {
			return events[len(events)-1].Timestamp
		}
	}
	if current, err := r.getCurrentSessionID(); err == nil && current == sessionID {
		return time.Time{}
	}
	// A run that did not say it ended, and is not the current one: its
	// process is gone. It ended, at the latest, when the next run started.
	sessions, _ := r.ListSessions()
	next := ""
	for _, s := range sessions {
		if !IsSide(s) && s > sessionID && (next == "" || s < next) {
			next = s
		}
	}
	if next != "" {
		if e, ok := firstEvent(r.GetSessionFilePath(next)); ok {
			return e.Timestamp
		}
	}
	return time.Time{}
}

// LatestRun names the run whose events are shown when none is asked for: the
// current session, when its file is there, else the newest session that is a
// run's. "" when there is none.
func (r *EventReader) LatestRun() string {
	if id, err := r.getCurrentSessionID(); err == nil {
		if _, err := os.Stat(r.GetSessionFilePath(id)); err == nil {
			return id
		}
	}
	sessions, err := r.ListSessions()
	if err != nil {
		return ""
	}
	for _, s := range sessions { // the newest first
		if !IsSide(s) {
			return s
		}
	}
	return ""
}

// firstEvent is the first event of a file.
func firstEvent(path string) (Event, bool) {
	f, err := os.Open(path)
	if err != nil {
		return Event{}, false
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		var e Event
		if json.Unmarshal(scanner.Bytes(), &e) == nil {
			return e, true
		}
	}
	return Event{}, false
}

// readWindow reads the events of a file written from since up to until (zero
// for no end) that keep takes, at most max of them (0 for all), the latest
// kept, in the order they were written. It reads from the file's end and
// stops at the first event older than since: an event file is written in
// time, so a long one is read only as far back as the window goes.
func readWindow(path string, since, until time.Time, max int, keep func(Event) bool) ([]Event, error) {
	f, err := os.Open(filepath.Clean(path))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	var newest []Event
	// tail is the end of a line whose start is in the part not read yet.
	var tail []byte
	for pos := info.Size(); pos > 0; {
		n := int64(windowChunk)
		if n > pos {
			n = pos
		}
		pos -= n
		chunk := make([]byte, n, int(n)+len(tail))
		if _, err := f.ReadAt(chunk, pos); err != nil {
			return nil, err
		}
		lines := bytes.Split(append(chunk, tail...), []byte{'\n'})
		first := 0
		if pos > 0 {
			// The first line goes on in the part before: read with it.
			tail, first = lines[0], 1
		}
		for i := len(lines) - 1; i >= first; i-- {
			line := bytes.TrimSpace(lines[i])
			if len(line) == 0 {
				continue
			}
			var e Event
			if json.Unmarshal(line, &e) != nil {
				continue
			}
			if e.Timestamp.Before(since) {
				slices.Reverse(newest)
				return newest, nil
			}
			if (!until.IsZero() && e.Timestamp.After(until)) || (keep != nil && !keep(e)) {
				continue
			}
			newest = append(newest, e)
			if max > 0 && len(newest) == max {
				slices.Reverse(newest)
				return newest, nil
			}
		}
	}
	slices.Reverse(newest)
	return newest, nil
}
