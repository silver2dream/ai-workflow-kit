package main

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/config"
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

	Warnings []string
}

// listIssues asks GitHub for issues; the tests answer in its place.
var listIssues status.IssueLister = status.ListWithGh

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
	case v.Stopped:
		ending := workflow.EndingFor(v.StopReason)
		fmt.Fprintf(w, "%-9s not going. The last one: %s\n", "Run", ending.Summary)
	default:
		fmt.Fprintf(w, "%-9s not going\n", "Run")
	}
	if v.StaleLock {
		fmt.Fprintf(w, "%-9s a run died and left its lock; the next run clears it\n", "")
	}

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
