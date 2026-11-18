package council

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// What a meeting tells while it goes on, for whoever follows it from outside
// the process that holds it (the workbench): where it is, in live.json, and
// what was said, a line at a time, in transcript.jsonl. Both are in the
// plan's state folder; neither is read back by the meeting.

// StateDir is the folder where a plan keeps what is not one of its documents:
// every prompt and answer, what the owner said, how far it went.
func StateDir(root, id string) string {
	return filepath.Join(root, ".ai", "state", "council", id)
}

// The states of a meeting, as live.json says them.
const (
	// LiveMeeting: the meeting goes on.
	LiveMeeting = "meeting"
	// LiveAgenda: the agenda is set, and waits for the owner to look at it.
	LiveAgenda = "agenda"
	// LiveQuestions: the questions of the agenda wait for the owner's answers.
	LiveQuestions = "questions"
	// LivePaused: the owner asked it to stop, and it stopped after a step.
	LivePaused = "paused"
	// LiveFinished: the work list is laid out.
	LiveFinished = "finished"
	// LiveFailed: it stopped on an error.
	LiveFailed = "failed"
)

// Live is where a meeting is, as the process holding it last said.
type Live struct {
	// PID is the process that holds the meeting; whoever reads this checks
	// that it still runs while State is LiveMeeting.
	PID   int    `json:"pid"`
	State string `json:"state"`
	// Note says why it stopped, when it stopped on an error.
	Note string `json:"note,omitempty"`
	// Topic is the topic it is at, from 1; 0 before the first and after the
	// last. Topics is how many the agenda has.
	Topic  int `json:"topic"`
	Topics int `json:"topics"`
	// Step is what it is at; Speaker the role that is asked, when one is.
	Step    string `json:"step,omitempty"`
	Speaker string `json:"speaker,omitempty"`
	// Calls is how many model calls this run made, of the Limit a run has.
	Calls int `json:"calls"`
	Limit int `json:"limit"`
	// Started is when this run started; Updated when this was written.
	Started time.Time `json:"started"`
	Updated time.Time `json:"updated"`
}

// LiveFile is the name of the meeting's state in its state folder.
const LiveFile = "live.json"

// ReadLive reads where a meeting is. A plan that was never held by a meeting
// that says so (one held before this was written) has none: os.ErrNotExist.
func ReadLive(root, id string) (*Live, error) {
	if !planID.MatchString(id) {
		return nil, fmt.Errorf("%q is not the name of a plan", id)
	}
	data, err := os.ReadFile(filepath.Join(StateDir(root, id), LiveFile))
	if err != nil {
		return nil, err
	}
	var live Live
	if err := json.Unmarshal(data, &live); err != nil {
		return nil, fmt.Errorf("%s cannot be read: %w", LiveFile, err)
	}
	return &live, nil
}

// Entry is one thing said at a meeting: a proposal, a speech, a ruling, what
// the owner said. Data holds what was said, in the form of its Kind.
type Entry struct {
	Seq int       `json:"seq"`
	At  time.Time `json:"at"`
	// Step is the step it was said at; a step that is said again (a meeting
	// that went on asks again where it failed) replaces what it said before.
	Step  string `json:"step"`
	Topic string `json:"topic,omitempty"`
	Kind  string `json:"kind"`
	Role  string `json:"role,omitempty"`
	Data  any    `json:"data,omitempty"`
}

// The kinds of an entry.
const (
	EntryAgenda    = "agenda"    // Data: Agenda
	EntryQuestions = "questions" // Data: []string, put to the owner
	EntryAnswers   = "answers"   // Data: []OwnerAnswer
	EntryProposal  = "proposal"  // Data: Speech
	EntrySpeech    = "speech"    // Data: Speech
	EntryExchange  = "exchange"  // Data: ExchangeEntry
	EntrySummary   = "summary"   // Data: Summary
	EntryRuling    = "ruling"    // Data: RulingEntry
	EntryDocuments = "documents" // Data: DocumentsEntry
	EntryWord      = "word"      // Data: OwnerWord
	EntryReview    = "review"    // Data: Review
	EntryWorkList  = "worklist"  // Data: WorkList
	EntryStopped   = "stopped"   // Data: Live, when the meeting stopped
)

// ExchangeEntry is one round of a dispute.
type ExchangeEntry struct {
	Challenger string   `json:"challenger"`
	Defender   string   `json:"defender"`
	Point      string   `json:"point"`
	Round      int      `json:"round"`
	Exchange   Exchange `json:"exchange"`
}

// RulingEntry is what the deciding role ruled on a topic.
type RulingEntry struct {
	Decisions   []Decision   `json:"decisions"`
	Assumptions []Assumption `json:"assumptions"`
}

// DocumentsEntry is what a role wrote, and what reading it against the rules
// found.
type DocumentsEntry struct {
	Documents []WrittenDocument `json:"documents"`
	Check     *RuleCheck        `json:"check,omitempty"`
}

// TranscriptFile is the name of what was said in the state folder.
const TranscriptFile = "transcript.jsonl"

// transcript appends what is said to the plan's transcript.
type transcript struct {
	mu   sync.Mutex
	file string
	seq  int
	now  func() time.Time
}

func newTranscript(dir string, now func() time.Time) *transcript {
	t := &transcript{file: filepath.Join(dir, TranscriptFile), now: now}
	// The seq counts on from what an earlier run said.
	if entries, err := readTranscript(t.file, 0); err == nil {
		for _, entry := range entries {
			if entry.Seq > t.seq {
				t.seq = entry.Seq
			}
		}
	}
	return t
}

func (t *transcript) add(entry Entry) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.seq++
	entry.Seq, entry.At = t.seq, t.now()
	line, err := json.Marshal(entry)
	if err != nil {
		return
	}
	f, err := os.OpenFile(t.file, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	// What is told is told as well as it can be: a line that cannot be
	// written is left out, and the meeting goes on.
	_, _ = f.Write(append(line, '\n'))
	if err := f.Close(); err != nil {
		return
	}
}

// ReadTranscript returns what was said at a plan's meeting after the entry
// numbered since, in order. A step said again keeps only its last entry.
func ReadTranscript(root, id string, since int) ([]Entry, error) {
	if !planID.MatchString(id) {
		return nil, fmt.Errorf("%q is not the name of a plan", id)
	}
	return readTranscript(filepath.Join(StateDir(root, id), TranscriptFile), since)
}

func readTranscript(file string, since int) ([]Entry, error) {
	f, err := os.Open(file)
	if errors.Is(err, os.ErrNotExist) {
		return []Entry{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var all []Entry
	last := map[string]int{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry Entry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue // a line cut short by a crash says nothing
		}
		if at, ok := last[entry.Step]; ok && entry.Step != "" {
			all[at].Seq = -1 // said again: the earlier one is dropped
		}
		last[entry.Step] = len(all)
		all = append(all, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	out := []Entry{}
	for _, entry := range all {
		if entry.Seq > since {
			out = append(out, entry)
		}
	}
	return out, nil
}

// liveWriter keeps live.json up to date.
type liveWriter struct {
	mu   sync.Mutex
	file string
	live Live
	now  func() time.Time
}

func newLiveWriter(dir string, limit int, now func() time.Time) *liveWriter {
	return &liveWriter{file: filepath.Join(dir, LiveFile), now: now,
		live: Live{PID: os.Getpid(), State: LiveMeeting, Limit: limit, Started: now()}}
}

// set changes what live.json says, and writes it.
func (w *liveWriter) set(change func(*Live)) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	change(&w.live)
	w.live.Updated = w.now()
	data, err := json.MarshalIndent(w.live, "", "  ")
	if err != nil {
		return
	}
	// Written whole, so that a reader never sees half of it.
	tmp := w.file + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	if err := os.Rename(tmp, w.file); err != nil {
		// Windows refuses while a reader has the file open: written in place.
		_ = os.WriteFile(w.file, data, 0o644)
		_ = os.Remove(tmp)
	}
}

func (w *liveWriter) get() Live {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.live
}
