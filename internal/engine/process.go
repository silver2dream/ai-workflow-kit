package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/kickoff"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
	"github.com/silver2dream/ai-workflow-kit/internal/stopmark"
	"github.com/silver2dream/ai-workflow-kit/internal/trace"
)

// The engine as the project's run (design.md §5.1). It runs alone: whoever
// starts it holds the run's lock (`awkit run`, the kickoff lock the
// Principal's workflow holds too, so that the two never run at once). It
// says that it is alive and what it is doing in its run file, beat by beat:
// the lease on the lanes it is at, the steps that run on them, which `awkit
// status` and the workbench read, and which an engine started after a crash
// finds stale. What it was
// doing when it stopped is taken up again from the lanes (Next): a Worker's
// result is read, a review or a resolution is done again from the start.
// It takes its commands from the inbox between beats, and a stop at once
// while a step runs.

// RunFile is the engine's run file, under the project.
const RunFile = layout.StateDir + "/engine/run.json"

// BeatInterval is how often a running engine says it is alive.
const BeatInterval = 15 * time.Second

// TypeRun is the event of a run's start and end.
const TypeRun = "engine_run"

// RunInfo is what the run file says.
type RunInfo struct {
	PID     int       `json:"pid"`
	Host    string    `json:"host"`
	Started time.Time `json:"started"`
	Beat    time.Time `json:"beat"`
	// Only is the one ticket the run is limited to.
	Only int `json:"only,omitempty"`
	// Steps are what the engine is doing: the steps that run, each the
	// decision it acts on, and since when; the oldest first.
	Steps []StepInfo `json:"steps,omitempty"`
	// Step is the one step a run file of an awkit before task 2.2b names:
	// ReadRun takes it as Steps.
	Step *StepInfo `json:"step,omitempty"`
	// Ended and Reason say that the run is over, and why.
	Ended  time.Time `json:"ended,omitzero"`
	Reason string    `json:"reason,omitempty"`
}

// StepInfo is a decision an engine acts on: a step that runs.
type StepInfo struct {
	Action string    `json:"action"`
	Issue  int       `json:"issue,omitempty"`
	PR     int       `json:"pr,omitempty"`
	Merge  string    `json:"merge,omitempty"`
	Since  time.Time `json:"since"`
}

// ReadRun is a project's run file, when it has one.
func ReadRun(root string) (RunInfo, bool) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(RunFile)))
	if err != nil {
		return RunInfo{}, false
	}
	var r RunInfo
	if json.Unmarshal(data, &r) != nil {
		return RunInfo{}, false
	}
	if len(r.Steps) == 0 && r.Step != nil {
		r.Steps = []StepInfo{*r.Step}
	}
	r.Step = nil
	return r, true
}

// Alive says whether the run is going: not ended, its process there, and
// its last beat recent. A run file of a process that died says otherwise.
func (r RunInfo) Alive(now time.Time) bool {
	return r.Ended.IsZero() && r.PID > 0 && kickoff.ProcessAlive(r.PID) && now.Sub(r.Beat) < 3*BeatInterval
}

// ServeOptions are how often a served engine beats and looks at its inbox;
// zero is BeatInterval and two seconds.
type ServeOptions struct {
	Beat time.Duration
	Poll time.Duration
}

// Serve runs the engine as the project's run, and says how it ended (Run).
// The caller holds the run's lock.
func Serve(ctx context.Context, e *Engine, o ServeOptions) (string, error) {
	if o.Beat == 0 {
		o.Beat = BeatInterval
	}
	if o.Poll == 0 {
		o.Poll = 2 * time.Second
	}
	host, _ := os.Hostname()
	started := e.clock()
	info := RunInfo{PID: os.Getpid(), Host: host, Started: started, Beat: started, Only: e.Only}
	var mu sync.Mutex
	write := func(change func(*RunInfo)) {
		mu.Lock()
		defer mu.Unlock()
		if change != nil {
			change(&info)
		}
		data, err := json.MarshalIndent(info, "", "  ")
		if err == nil {
			err = writeAtomic(filepath.Join(e.Root, filepath.FromSlash(layout.StateDir), "engine"), "run.json", data)
		}
		if err != nil {
			e.logf("the run file could not be written: %v", err)
		}
	}
	write(nil)
	_ = e.Journal.Note(TypeRun, trace.LevelInfo, e.Only, map[string]any{"event": "started", "pid": info.PID, "host": host})

	ctx, stopNow := context.WithCancel(ctx)
	defer stopNow()
	onSteps := e.OnSteps
	e.OnSteps = func(steps []StepInfo) {
		write(func(r *RunInfo) { r.Steps = steps })
		if onSteps != nil {
			onSteps(steps)
		}
	}
	defer func() { e.OnSteps = onSteps }()

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		beat := time.NewTicker(o.Beat)
		poll := time.NewTicker(o.Poll)
		defer beat.Stop()
		defer poll.Stop()
		for {
			select {
			case <-done:
				return
			case <-beat.C:
				write(func(r *RunInfo) { r.Beat = e.clock() })
			case <-poll.C:
				e.takeCommands(stopNow)
			}
		}
	}()

	reason, err := e.Run(ctx)
	close(done)
	wg.Wait()
	// A command given as the run ended is answered all the same.
	e.takeCommands(nil)

	// The steps were taken out as each ended: a run ends when none runs.
	write(func(r *RunInfo) { r.Ended, r.Reason, r.Beat = e.clock(), reason, e.clock() })
	data := map[string]any{"event": "ended", "reason": reason}
	if err != nil {
		data["error"] = err.Error()
	}
	_ = e.Journal.Note(TypeRun, trace.LevelInfo, e.Only, data)
	return reason, err
}

// takeCommands answers what is in the inbox, in order. stopNow ends the run
// at once; nil when it has ended.
func (e *Engine) takeCommands(stopNow func()) {
	commands, err := Pending(e.Root)
	if err != nil {
		e.logf("the inbox could not be read: %v", err)
		return
	}
	for _, c := range commands {
		result, reason := Applied, ""
		switch c.Kind {
		case CommandStop:
			by := c.By
			if by == "" {
				by = "the engine's inbox"
			}
			if err := stopmark.Write(e.Root, by, stopmark.ByUser); err != nil {
				result, reason = Rejected, "the stop could not be written: "+err.Error()
			} else {
				reason = "the run stops when the step it is on is done"
			}
		case CommandStopNow:
			if stopNow != nil {
				stopNow()
				reason = "the run stops at once, and what is running with it"
			} else {
				reason = "the run had ended"
			}
		case "":
			result, reason = Rejected, "the file is not a command"
		default:
			result, reason = Rejected, fmt.Sprintf("the engine takes %q from task 2.6 on; until then only %s and %s", c.Kind, CommandStop, CommandStopNow)
		}
		r, err := answer(e.Root, c, result, reason, e.clock())
		if err != nil {
			e.logf("command %s could not be answered: %v", c.ID, err)
			continue
		}
		level := trace.LevelInfo
		if result == Rejected {
			level = trace.LevelWarn
		}
		_ = e.Journal.Note(TypeCommand, level, c.Issue, map[string]any{"command": c, "reply": r})
		e.logf("command %s (%s, by %s): %s%s", c.ID, c.Kind, c.By, result, noteOf(reason))
	}
}
