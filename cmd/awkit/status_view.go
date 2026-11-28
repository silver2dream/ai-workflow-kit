package main

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/config"
	"github.com/silver2dream/ai-workflow-kit/internal/engine"
	"github.com/silver2dream/ai-workflow-kit/internal/guard"
	status "github.com/silver2dream/ai-workflow-kit/internal/status"
	"github.com/silver2dream/ai-workflow-kit/internal/stopmark"
	"github.com/silver2dream/ai-workflow-kit/internal/workflow"
)

// statusView is what `awkit status` shows a person: whether a run is going
// and how the last one ended, where every ticket is, and what to do next.
// It used to print the lock's state, the rule names of the configuration and
// "STOP=absent", and in a new project nothing else.
type statusView struct {
	Project     string
	Integration string
	Release     string

	Running   bool
	Since     string // when the run that is going started, as the lock says it
	PID       int
	StaleLock bool

	Stopped    bool   // the last run left its marker
	StopReason string // and this is why it stopped

	Tickets   *status.Tickets
	NoTickets string // why there is no word on the tickets

	// Engine is the engine's state: its last run, its lanes, its shadow.
	Engine engine.View
	// Shadow is how the last run of the engine compared its decisions with
	// the Principal's workflow's, when it had a shadow (AWKIT_ENGINE=shadow).
	Shadow *engine.ShadowRun

	Warnings []string
}

// listIssues asks GitHub for issues; the tests answer in its place.
var listIssues status.IssueLister = status.ListWithGh

// statusNow tells the time a run's beat is compared with; the tests set it.
var statusNow = time.Now

// hasOrigin reports whether the project has a remote named origin. Without
// one there is no GitHub to ask, and `gh` takes its time to say so.
var hasOrigin = func() bool {
	out, err := exec.Command("git", "remote", "get-url", "origin").Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
}

// collectStatusView puts the view together from the offline report, the
// configuration and, unless it is told not to ask, GitHub.
func collectStatusView(ctx context.Context, report status.Report, cfg *config.Config, offline bool) statusView {
	view := statusView{
		Running:   report.Run.State == "running",
		PID:       report.Run.PID,
		Since:     report.Run.StartTime,
		StaleLock: report.Run.State == "stale_lock",
	}
	if report.Run.LockCorrupt {
		view.Warnings = append(view.Warnings, "the lock of a run cannot be read ("+filepath.ToSlash(report.Run.LockFile)+"); awkit reset --lock removes it")
	}
	view.StopReason, view.Stopped = stopmark.Read(".")
	view.Engine = engine.Look(".", cfg, statusNow())
	view.Shadow = view.Engine.Shadow
	view.Warnings = append(view.Warnings, view.Engine.Problems...)

	if cfg == nil {
		return view
	}
	view.Project = cfg.Project.Name
	view.Integration, view.Release = cfg.Git.IntegrationBranch, cfg.Git.ReleaseBranch

	switch {
	case offline:
		view.NoTickets = "not asked for (--offline)"
	case !hasOrigin():
		view.NoTickets = "the project has no remote named origin yet; awkit doctor says what a run needs"
	default:
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		tickets, err := status.CollectTickets(ctx, cfg.GitHub.Labels, listIssues)
		if err != nil {
			view.NoTickets = fmt.Sprintf("GitHub could not be asked (%v); awkit doctor checks the tools and the remote", err)
		} else {
			view.Tickets = tickets
		}
	}
	return view
}

// maxShownPerStage is how many tickets of a stage are listed before the rest
// are counted.
const maxShownPerStage = 10

// printStatus writes the view.
func printStatus(w io.Writer, v statusView) {
	if v.Project != "" {
		where := "pull requests go to " + v.Integration
		if v.Release != "" && v.Release != v.Integration {
			where += ", releases are cut from " + v.Release
		}
		fmt.Fprintf(w, "%s: %s\n\n", bold(v.Project), where)
	}

	// The run.
	switch {
	case v.Running:
		fmt.Fprintf(w, "%-9s going%s (pid %d). awkit stop asks it to stop.\n", "Run", since(v.Since), v.PID)
		if v.Engine.Going {
			// A line for each step that runs: lanes run side by side.
			for _, s := range v.Engine.Run.Steps {
				if doing := stepWords(s); doing != "" {
					fmt.Fprintf(w, "%-9s the engine: %s%s\n", "", doing, since(s.Since.Format(time.RFC3339)))
				}
			}
		}
	case v.Stopped:
		ending := workflow.EndingFor(v.StopReason)
		fmt.Fprintf(w, "%-9s not going. The last one: %s\n", "Run", ending.Summary)
	default:
		fmt.Fprintf(w, "%-9s not going\n", "Run")
	}
	if v.StaleLock {
		fmt.Fprintf(w, "%-9s a run died and left its lock; the next run clears it\n", "")
	}
	printShadow(w, v.Shadow)
	printLanes(w, v)

	// The tickets.
	switch {
	case v.Tickets == nil && v.NoTickets != "":
		fmt.Fprintf(w, "%-9s %s\n", "Tickets", v.NoTickets)
	case v.Tickets != nil:
		fmt.Fprintf(w, "%-9s %d open, %d done\n", "Tickets", len(v.Tickets.Open), v.Tickets.Done)
		for _, stage := range status.Stages {
			tickets := v.Tickets.In(stage.Stage)
			if len(tickets) == 0 {
				continue
			}
			fmt.Fprintf(w, "\n  %s\n", stage.Title)
			for i, t := range tickets {
				if i == maxShownPerStage {
					fmt.Fprintf(w, "    … and %d more\n", len(tickets)-maxShownPerStage)
					break
				}
				fmt.Fprintf(w, "    #%-4d %s\n", t.Number, shortened(t.Title, 72))
				if t.Why != "" && (stage.Stage == status.StageNeedsYou || stage.Stage == status.StageWaiting || stage.Stage == status.StageRework) {
					fmt.Fprintf(w, "          %s\n", t.Why)
				}
			}
		}
	}

	for _, warning := range v.Warnings {
		fmt.Fprintf(w, "\n%s %s\n", yellow("!"), warning)
	}

	fmt.Fprintf(w, "\n%-9s %s\n", "Next", nextStep(v))
}

// maxShownDifferences is how many of the shadow's differences are listed,
// the latest, before the rest are counted.
const maxShownDifferences = 5

// printShadow says how the last run's decisions compared with the
// Principal's workflow's.
func printShadow(w io.Writer, run *engine.ShadowRun) {
	if run == nil {
		return
	}
	parts := []string{fmt.Sprintf("%d the same", run.Same)}
	if run.Known > 0 {
		parts = append(parts, fmt.Sprintf("%d as is known", run.Known))
	}
	if len(run.Differences) > 0 {
		parts = append(parts, fmt.Sprintf("%d different", len(run.Differences)))
	}
	if run.Failed > 0 {
		parts = append(parts, fmt.Sprintf("%d not compared", run.Failed))
	}
	fmt.Fprintf(w, "%-9s the last run compared %d of the engine's decisions with the Principal's workflow's: %s\n", "Shadow", run.Steps, strings.Join(parts, ", "))
	shown := run.Differences
	if len(shown) > maxShownDifferences {
		fmt.Fprintf(w, "%-9s %d earlier differences are in awkit events\n", "", len(shown)-maxShownDifferences)
		shown = shown[len(shown)-maxShownDifferences:]
	}
	for _, d := range shown {
		fmt.Fprintf(w, "%-9s %s the engine: %s\n", "", d.At.Local().Format("15:04"), engine.Decided(d.Comparison.Engine))
		fmt.Fprintf(w, "%-9s       the Principal's workflow: %s\n", "", engine.Decided(d.Comparison.Analyzer))
	}
}

// laneStates say where a lane is, as `awkit status` says it.
var laneStates = map[engine.State]string{
	engine.Blocked:          "waits for a ticket it depends on",
	engine.Ready:            "ready",
	engine.Implementing:     "being written",
	engine.Checking:         "being checked",
	engine.Reviewing:        "in review",
	engine.ReviewRetry:      "to be reviewed again",
	engine.Rework:           "back to a Worker",
	engine.AwaitingApproval: "waits for your approval",
	engine.Queued:           "waits its turn to be merged",
	engine.Resolving:        "to be resolved",
	engine.Merged:           "merged",
	engine.Completed:        "done: nothing to change",
	engine.NeedsYou:         "needs you",
	engine.Paused:           "paused",
	engine.Cancelled:        "cancelled",
}

// laneState says where a lane is, with what resolving does to it.
func laneState(l engine.Lane) string {
	switch {
	case l.State == engine.Resolving && l.Resolve == engine.ResolveConflict:
		return "conflicts with its base"
	case l.State == engine.Resolving && l.Resolve == engine.ResolveRebase:
		return "behind its base"
	case l.State == engine.Implementing && l.Resolve == engine.ResolveConflict:
		return "its conflict is being resolved"
	case l.State == engine.Implementing && l.Resolve == engine.ResolveRebase:
		return "being brought up to its base"
	}
	if words, ok := laneStates[l.State]; ok {
		return words
	}
	return string(l.State)
}

// queuePlace says a lane's place in the merge queue: "the next to be
// merged", "2nd in the merge queue".
func queuePlace(place int) string {
	if place == 1 {
		return "the next to be merged"
	}
	suffix := "th"
	switch {
	case place%100 >= 11 && place%100 <= 13:
	case place%10 == 1:
		suffix = "st"
	case place%10 == 2:
		suffix = "nd"
	case place%10 == 3:
		suffix = "rd"
	}
	return fmt.Sprintf("%d%s in the merge queue", place, suffix)
}

// stepWords say what the engine does in a step, "" for a step that ends
// the run.
func stepWords(s engine.StepInfo) string {
	switch {
	case s.Action == engine.ActionMerge:
		return fmt.Sprintf("the merge queue merges PR #%d of #%d", s.PR, s.Issue)
	case s.Action == analyzer.ActionReviewPR:
		return fmt.Sprintf("the reviewer reviews PR #%d of #%d", s.PR, s.Issue)
	case s.Action == analyzer.ActionDispatchWorker && s.Merge == analyzer.MergeIssueConflict:
		return fmt.Sprintf("the resolver resolves the conflict of PR #%d of #%d", s.PR, s.Issue)
	case s.Action == analyzer.ActionDispatchWorker && s.Merge == analyzer.MergeIssueRebase:
		return fmt.Sprintf("PR #%d of #%d is brought up to its base", s.PR, s.Issue)
	case s.Action == analyzer.ActionDispatchWorker, s.Action == analyzer.ActionCheckResult:
		return fmt.Sprintf("a Worker writes #%d", s.Issue)
	}
	return ""
}

// printLanes lists the engine's lanes: the tickets in its hands, and those
// its last run finished.
func printLanes(w io.Writer, v statusView) {
	e := v.Engine
	if len(e.Lanes) == 0 {
		return
	}
	title := func(issue int) string {
		if v.Tickets == nil {
			return ""
		}
		for _, t := range v.Tickets.Open {
			if t.Number == issue {
				return t.Title
			}
		}
		return v.Tickets.DoneTitles[issue]
	}
	inHand, finished := 0, 0
	for _, l := range e.Lanes {
		if l.State.Terminal() {
			finished++
		} else {
			inHand++
		}
	}
	summary := fmt.Sprintf("%d in the engine's hands", inHand)
	if finished > 0 {
		summary += fmt.Sprintf(", %d finished in its last run", finished)
	}
	fmt.Fprintf(w, "%-9s %s\n\n", "Lanes", summary)
	for _, l := range e.Lanes {
		parts := []string{laneState(l.Lane)}
		if l.Queue > 0 {
			parts = append(parts, queuePlace(l.Queue))
		}
		if l.PR > 0 {
			parts = append(parts, fmt.Sprintf("PR #%d", l.PR))
		}
		for _, count := range []struct {
			name      string
			used, max int
		}{
			{"failed", l.Attempts.Failures, e.Limits.Failures},
			{"reviewed again", l.Attempts.Reviews, e.Limits.Reviews},
			{"conflicts sent to be resolved", l.Attempts.Conflicts, e.Limits.Conflicts},
			{"checks failed in the merge queue", l.Attempts.Checks, e.Limits.Checks},
		} {
			if count.used > 0 {
				parts = append(parts, fmt.Sprintf("%s %d of %d", count.name, count.used, count.max))
			}
		}
		if !l.Since.IsZero() {
			parts = append(parts, strings.TrimSpace(since(l.Since.Format(time.RFC3339))))
		}
		fmt.Fprintf(w, "    #%-4d %s\n", l.Issue, strings.Join(parts, ", "))
		if t := title(l.Issue); t != "" {
			fmt.Fprintf(w, "          %s\n", shortened(t, 72))
		}
		if l.Step != nil {
			if doing := stepWords(*l.Step); doing != "" {
				fmt.Fprintf(w, "          now: %s\n", doing)
			}
		}
		if l.State == engine.NeedsYou {
			why := engine.WhyText(l.Why)
			// What a guardrail stopped is said by its verdict, below.
			guarded := l.Verdict != nil && strings.HasPrefix(l.Why, "guard_")
			if l.Note != "" && l.Note != why && !guarded {
				why += ": " + l.Note
			}
			fmt.Fprintf(w, "          %s\n", why)
		}
		if l.State == engine.AwaitingApproval {
			if l.Note != "" {
				fmt.Fprintf(w, "          %s\n", l.Note)
			}
			fmt.Fprintf(w, "          awkit lane approve %d sends it to the merge queue; awkit lane reject %d --note \"…\" returns it\n", l.Issue, l.Issue)
		}
		if l.Verdict != nil {
			what := "flagged"
			if l.Verdict.Result == guard.Block {
				what = "blocked"
			}
			where := l.Verdict.Rule
			if l.Verdict.Source != "" {
				where += " in " + l.Verdict.Source
			}
			fmt.Fprintf(w, "          a guardrail %s %s at %s: %s\n", what, where, l.Verdict.At.Local().Format("15:04"), l.Verdict.Reason)
		}
	}
	if e.Earlier > 0 {
		fmt.Fprintf(w, "    %d more ended before the last run; awkit events --issue N shows one\n", e.Earlier)
	}
	fmt.Fprintln(w)
}

// nextStep is the one thing a person does next.
func nextStep(v statusView) string {
	if v.Project == "" {
		return "awkit init sets this directory up."
	}
	if v.Running {
		return "Nothing: the run is going. awkit status again later; awkit stop to end it."
	}
	if v.Tickets == nil {
		if v.Stopped {
			return workflow.EndingFor(v.StopReason).Next
		}
		return "awkit doctor says whether a run can start."
	}

	needsYou := v.Tickets.In(status.StageNeedsYou)
	switch {
	case len(needsYou) > 0:
		numbers := make([]string, 0, len(needsYou))
		for _, t := range needsYou {
			numbers = append(numbers, fmt.Sprintf("#%d", t.Number))
		}
		return fmt.Sprintf("Look at %s (gh issue view %d --comments). Once it is dealt with: awkit reset --labels puts a failed ticket back in the queue, then awkit run.",
			strings.Join(numbers, ", "), needsYou[0].Number)
	case len(v.Tickets.Open) > 0:
		return "awkit run works through the open tickets."
	case v.Tickets.Done > 0:
		return `Every ticket is done. awkit plan "<what you want built>" plans more work.`
	default:
		return `No ticket yet. awkit plan "<what you want built>" proposes some; awkit create-task adds one.`
	}
}

// since says when a run started, in the local time of the person reading:
// " since 14:02", or with the day when it was not today.
func since(started string) string {
	t, err := time.Parse(time.RFC3339, started)
	if err != nil {
		return ""
	}
	t = t.Local()
	if now := time.Now(); t.Year() == now.Year() && t.YearDay() == now.YearDay() {
		return " since " + t.Format("15:04")
	}
	return " since " + t.Format("2006-01-02 15:04")
}

// shortened cuts a title that is longer than a line has room for.
func shortened(s string, max int) string {
	runes := []rune(strings.TrimSpace(s))
	if len(runes) <= max {
		return string(runes)
	}
	return string(runes[:max-1]) + "…"
}
