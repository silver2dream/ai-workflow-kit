package engine

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/config"
	"github.com/silver2dream/ai-workflow-kit/internal/guard"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
	"github.com/silver2dream/ai-workflow-kit/internal/trace"
	"github.com/silver2dream/ai-workflow-kit/internal/worker"
)

// What a reader that is not the engine is shown of it: the workbench's run
// page and `awkit status` (task 1.8, design.md §13). Both read it here, so
// that they say the same.

// View is the engine's state of a project, as a person is shown it.
type View struct {
	// Run is the engine's last run; nil when the project has had none.
	Run *RunInfo `json:"run,omitempty"`
	// Going says that run is going now (RunInfo.Alive).
	Going bool `json:"going"`
	// Lanes are the lanes worth showing, in the order they are shown: each
	// that is not over, and each that ended in the last run.
	Lanes []LaneView `json:"lanes"`
	// Earlier counts the lanes that ended before the last run: not listed.
	Earlier int `json:"earlier"`
	// Limits are the counts at which a lane is handed to a person.
	Limits Limits `json:"limits"`
	// Shadow is how the last run's decisions compared with the Principal's
	// workflow's, when it had a shadow (AWKIT_ENGINE=shadow).
	Shadow *ShadowRun `json:"shadow,omitempty"`
	// Pending are the commands the engine has not answered, oldest first;
	// Replies are its latest answers, newest first.
	Pending []Command `json:"pending"`
	Replies []Reply   `json:"replies"`
	// Problems say what of the state could not be read; the rest is shown.
	Problems []string `json:"problems,omitempty"`
}

// LaneView is a lane as it is shown.
type LaneView struct {
	Lane
	// Step is what the engine does on the lane now: set while the run goes
	// and one of the steps that run is this lane's.
	Step *StepInfo `json:"step,omitempty"`
	// Queue is the lane's place in the merge queue, 1 the next to be
	// merged; 0 is not in it.
	Queue int `json:"queue,omitempty"`
	// Verdict is the latest guardrail verdict on the lane's ticket in the
	// last run that was not a pass.
	Verdict *LaneVerdict `json:"verdict,omitempty"`
}

// LaneVerdict is a guardrail verdict, and when it was made.
type LaneVerdict struct {
	guard.Verdict
	At time.Time `json:"at"`
}

// Limits are the counts of a lane (Attempts) at which it is handed to a
// person.
type Limits struct {
	Failures  int `json:"failures"`
	Reviews   int `json:"reviews"`
	Conflicts int `json:"conflicts"`
	// Checks bounds how often the merge queue's checks may fail on a
	// lane's change.
	Checks int `json:"checks"`
}

// LimitsOf are the limits a project's lanes are held to. The review is tried
// again, and a conflict sent to be resolved, as often as
// escalation.max_review_attempts says (as the analyzer bounds both); the
// Worker may fail as often as a dispatch allows by default, as the
// Principal's dispatches did.
func LimitsOf(cfg *config.Config) Limits {
	reviews := analyzer.DefaultMaxReviewAttempts
	if cfg != nil && cfg.Escalation.MaxReviewAttempts > 0 {
		reviews = cfg.Escalation.MaxReviewAttempts
	}
	return Limits{Failures: worker.DefaultMaxRetries, Reviews: reviews, Conflicts: reviews, Checks: reviews}
}

// MaxReplies is how many of the engine's answers a view lists.
const MaxReplies = 5

// working says whether a decision is a step on its lane: one that writes,
// reviews, resolves or merges it. The others end the run, or settle a lane
// on the way to ending it.
func working(action string) bool {
	switch action {
	case analyzer.ActionDispatchWorker, analyzer.ActionCheckResult, analyzer.ActionReviewPR, ActionMerge:
		return true
	}
	return false
}

// Look reads the engine's state of the project at root. cfg gives the
// limits (nil is the defaults); now says whether the run is alive. What
// cannot be read is said in Problems, and the rest is shown.
func Look(root string, cfg *config.Config, now time.Time) View {
	v := View{Lanes: []LaneView{}, Limits: LimitsOf(cfg), Pending: []Command{}, Replies: []Reply{}}
	var started time.Time
	if run, ok := ReadRun(root); ok {
		v.Run, v.Going, started = &run, run.Alive(now), run.Started
	}

	lanes, err := ReadLanes(root)
	if err != nil {
		v.Problems = append(v.Problems, "the lanes cannot be read: "+err.Error())
	}
	for _, l := range lanes {
		if l.State.Terminal() && (started.IsZero() || l.Since.Before(started)) {
			v.Earlier++
			continue
		}
		lv := LaneView{Lane: l}
		if v.Going {
			for _, s := range v.Run.Steps {
				if s.Issue == l.Issue && working(s.Action) {
					step := s
					lv.Step = &step
					break
				}
			}
		}
		v.Lanes = append(v.Lanes, lv)
	}
	for place, l := range queue(lanes) {
		for i := range v.Lanes {
			if v.Lanes[i].Issue == l.Issue {
				v.Lanes[i].Queue = place + 1
			}
		}
	}
	rank := func(l LaneView) int {
		switch {
		case l.Step != nil:
			return 0
		case l.State == NeedsYou:
			return 1
		case !l.State.Terminal():
			return 2
		}
		return 3
	}
	sort.SliceStable(v.Lanes, func(i, j int) bool {
		a, b := v.Lanes[i], v.Lanes[j]
		if rank(a) != rank(b) {
			return rank(a) < rank(b)
		}
		if rank(a) == 3 {
			// What ended last first.
			return a.Since.After(b.Since)
		}
		return a.Issue < b.Issue
	})

	if !started.IsZero() && len(v.Lanes) > 0 {
		if err := latestVerdicts(root, started, v.Lanes); err != nil {
			v.Problems = append(v.Problems, "the guardrails' verdicts cannot be read: "+err.Error())
		}
	}

	if shadow, ok, err := LastShadow(root); err != nil {
		v.Problems = append(v.Problems, "how the last run compared with the Principal's workflow cannot be read: "+err.Error())
	} else if ok {
		v.Shadow = &shadow
	}

	if pending, err := Pending(root); err != nil {
		v.Problems = append(v.Problems, "the engine's inbox cannot be read: "+err.Error())
	} else if pending != nil {
		v.Pending = pending
	}
	if replies, err := latestReplies(root, MaxReplies); err != nil {
		v.Problems = append(v.Problems, "the engine's answers cannot be read: "+err.Error())
	} else {
		v.Replies = replies
	}
	return v
}

// latestVerdicts gives each lane the latest verdict on its ticket since the
// run started that was not a pass. Only the event files written since then
// are read: a verdict of the run is in one of them, wherever it was made
// (the run's session, the guard's own file for a hook or a Worker).
func latestVerdicts(root string, since time.Time, lanes []LaneView) error {
	dir := filepath.Join(root, filepath.FromSlash(layout.StateDir), "events")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	at := map[int]int{}
	for i, l := range lanes {
		at[l.Issue] = i
	}
	needles := [][]byte{[]byte(trace.TypeGuardrailVerdict)}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().Before(since) {
			continue
		}
		err = scanEvents(filepath.Join(dir, entry.Name()), needles, func(ev trace.Event) {
			i, ok := at[ev.IssueID]
			if !ok || ev.Type != trace.TypeGuardrailVerdict || ev.Timestamp.Before(since) {
				return
			}
			var verdict guard.Verdict
			if decode(ev.Data, &verdict) != nil || verdict.Result == "" || verdict.Result == guard.Pass {
				return
			}
			if have := lanes[i].Verdict; have == nil || !ev.Timestamp.Before(have.At) {
				lanes[i].Verdict = &LaneVerdict{Verdict: verdict, At: ev.Timestamp}
			}
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// scanEvents reads, in order, the events of a file whose line holds one of
// the needles: a line without one is not decoded, so a long file is read
// quickly for the few events wanted. A line that is not an event is passed
// over, as the event reader does.
func scanEvents(path string, needles [][]byte, each func(trace.Event)) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		found := false
		for _, n := range needles {
			if bytes.Contains(line, n) {
				found = true
				break
			}
		}
		if !found {
			continue
		}
		var ev trace.Event
		if json.Unmarshal(line, &ev) != nil {
			continue
		}
		each(ev)
	}
	return scanner.Err()
}

// latestReplies are the engine's latest answers, newest first.
func latestReplies(root string, n int) ([]Reply, error) {
	dir := filepath.Join(root, filepath.FromSlash(OutboxDir))
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return []Reply{}, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".json") {
			continue
		}
		ids = append(ids, strings.TrimSuffix(name, ".json"))
	}
	// An id sorts in the order its command was written.
	sort.Sort(sort.Reverse(sort.StringSlice(ids)))
	replies := []Reply{}
	for _, id := range ids {
		if len(replies) == n {
			break
		}
		if r, ok := ReplyOf(root, id); ok {
			replies = append(replies, r)
		}
	}
	return replies, nil
}
