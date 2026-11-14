package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/config"
	status "github.com/silver2dream/ai-workflow-kit/internal/status"
	"github.com/silver2dream/ai-workflow-kit/internal/stopmark"
)

func statusText(v statusView) string {
	var out bytes.Buffer
	printStatus(&out, v)
	return out.String()
}

func ticketsOf(open ...status.Ticket) *status.Tickets {
	return &status.Tickets{Open: open, Done: 3}
}

// What a person wants from `awkit status`: where every ticket is, with what
// needs them first and the reason beside it, and what to do next.
func TestPrintStatus_Tickets(t *testing.T) {
	out := statusText(statusView{
		Project: "demo", Integration: "develop", Release: "main",
		Tickets: ticketsOf(
			status.Ticket{Number: 9, Title: "[feat] add login", Stage: status.StageNeedsYou, Why: "its Worker failed on every attempt; the issue has the last error"},
			status.Ticket{Number: 12, Title: "[feat] daily reward", Stage: status.StageInReview, Why: "its pull request waits for the review"},
			status.Ticket{Number: 13, Title: "[feat] shop page", Stage: status.StageInProgress, Why: "a Worker is on it"},
			status.Ticket{Number: 10, Title: "[feat] sign up", Stage: status.StageRework, Why: "its pull request conflicts with the branch it goes to"},
			status.Ticket{Number: 16, Title: "[feat] shop checkout", Stage: status.StageReady},
			status.Ticket{Number: 21, Title: "[feat] receipts", Stage: status.StageWaiting, Why: "waits for #16", WaitsFor: []int{16}},
		),
	})

	for _, want := range []string{
		"demo: pull requests go to develop, releases are cut from main\n",
		"Run       not going\n",
		"Tickets   6 open, 3 done\n",
		"\n  needs you\n    #9    [feat] add login\n          its Worker failed on every attempt; the issue has the last error\n",
		"\n  in review\n    #12   [feat] daily reward\n",
		"\n  in progress\n    #13   [feat] shop page\n",
		"\n  back to a Worker\n    #10   [feat] sign up\n          its pull request conflicts with the branch it goes to\n",
		"\n  ready\n    #16   [feat] shop checkout\n",
		"\n  waiting\n    #21   [feat] receipts\n          waits for #16\n",
		"Next      Look at #9 (gh issue view 9 --comments).",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("status does not show %q:\n%s", want, out)
		}
	}
	// The stage says it for a ticket in review or in progress.
	for _, noise := range []string{"waits for the review", "a Worker is on it", "STOP", "no_lock", "rules.kit"} {
		if strings.Contains(out, noise) {
			t.Errorf("status shows %q:\n%s", noise, out)
		}
	}
	// What needs a person comes first.
	if strings.Index(out, "needs you") > strings.Index(out, "in review") || strings.Index(out, "ready") > strings.Index(out, "waiting") {
		t.Errorf("the stages are out of order:\n%s", out)
	}
}

// A stage with many tickets lists the first ten and counts the rest; a title
// longer than the line is cut.
func TestPrintStatus_ManyTicketsAndLongTitles(t *testing.T) {
	var open []status.Ticket
	for n := 1; n <= 14; n++ {
		open = append(open, status.Ticket{Number: n, Title: fmt.Sprintf("ticket %d", n), Stage: status.StageReady})
	}
	open = append(open, status.Ticket{Number: 99, Title: strings.Repeat("長", 100), Stage: status.StageWaiting, Why: "waits for #1"})
	out := statusText(statusView{Project: "demo", Integration: "main", Release: "main", Tickets: &status.Tickets{Open: open}})

	if !strings.Contains(out, "    #10   ticket 10\n    … and 4 more\n") || strings.Contains(out, "ticket 11") {
		t.Errorf("the ready tickets beyond the tenth:\n%s", out)
	}
	if !strings.Contains(out, "#99   "+strings.Repeat("長", 71)+"…\n") {
		t.Errorf("a long title is not cut at 72 characters:\n%s", out)
	}
	if !strings.Contains(out, "demo: pull requests go to main\n") {
		t.Errorf("one trunk is named twice:\n%s", out)
	}
}

// The run: going, stopped for a reason, or neither.
func TestPrintStatus_Run(t *testing.T) {
	today := time.Now().Truncate(time.Minute)
	going := statusText(statusView{Project: "demo", Integration: "main", Running: true, PID: 4242, Since: today.UTC().Format(time.RFC3339), Tickets: ticketsOf()})
	if !strings.Contains(going, "Run       going since "+today.Local().Format("15:04")+" (pid 4242). awkit stop asks it to stop.\n") {
		t.Errorf("a run that is going:\n%s", going)
	}
	if !strings.Contains(going, "Next      Nothing: the run is going.") {
		t.Errorf("the next step while a run is going:\n%s", going)
	}

	stopped := statusText(statusView{Project: "demo", Integration: "main", Stopped: true, StopReason: "needs_human_review", Tickets: ticketsOf()})
	if !strings.Contains(stopped, "Run       not going. The last one: A ticket needs you.\n") {
		t.Errorf("a run that stopped for a reason:\n%s", stopped)
	}

	stale := statusText(statusView{Project: "demo", Integration: "main", StaleLock: true, Tickets: ticketsOf()})
	if !strings.Contains(stale, "a run died and left its lock; the next run clears it\n") {
		t.Errorf("a lock a dead run left:\n%s", stale)
	}

	for started, want := range map[string]string{
		"":                     "",
		"yesterday":            "",
		"2020-03-04T05:06:07Z": " since " + time.Date(2020, 3, 4, 5, 6, 7, 0, time.UTC).Local().Format("2006-01-02 15:04"),
	} {
		if got := since(started); got != want {
			t.Errorf("since(%q) = %q, want %q", started, got, want)
		}
	}
}

// The one thing to do next, for every state a project can be in.
func TestNextStep(t *testing.T) {
	project := func(v statusView) statusView {
		v.Project, v.Integration = "demo", "main"
		return v
	}
	for _, tc := range []struct {
		name string
		view statusView
		want string
	}{
		{"not set up", statusView{}, "awkit init sets this directory up."},
		{"a run is going", project(statusView{Running: true, Tickets: ticketsOf(status.Ticket{Number: 9, Stage: status.StageNeedsYou})}), "Nothing: the run is going."},
		{"tickets need a person", project(statusView{Tickets: ticketsOf(status.Ticket{Number: 9, Stage: status.StageNeedsYou}, status.Ticket{Number: 14, Stage: status.StageNeedsYou}, status.Ticket{Number: 16, Stage: status.StageReady})}),
			"Look at #9, #14 (gh issue view 9 --comments). Once it is dealt with: awkit reset --labels puts a failed ticket back in the queue, then awkit run."},
		{"work is waiting", project(statusView{Tickets: ticketsOf(status.Ticket{Number: 16, Stage: status.StageReady})}), "awkit run works through the open tickets."},
		{"everything is done", project(statusView{Tickets: &status.Tickets{Done: 5}}), `Every ticket is done. awkit plan "<what you want built>" plans more work.`},
		{"no ticket yet", project(statusView{Tickets: &status.Tickets{}}), `No ticket yet. awkit plan "<what you want built>" proposes some; awkit create-task adds one.`},
		{"GitHub was not asked", project(statusView{NoTickets: "not asked for (--offline)"}), "awkit doctor says whether a run can start."},
		{"GitHub was not asked, and the last run stopped", project(statusView{NoTickets: "x", Stopped: true, StopReason: "worker_failed"}), "awkit status shows which one; the issue has a comment with the last error. Fix the cause or the ticket, then awkit reset --labels and awkit run."},
	} {
		if got := nextStep(tc.view); !strings.HasPrefix(got, tc.want) {
			t.Errorf("%s: next step = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// replaceGitHub puts answers in the place of GitHub and of the remote check
// for the length of a test.
func replaceGitHub(t *testing.T, origin bool, lister status.IssueLister) {
	t.Helper()
	oldList, oldOrigin := listIssues, hasOrigin
	listIssues, hasOrigin = lister, func() bool { return origin }
	t.Cleanup(func() { listIssues, hasOrigin = oldList, oldOrigin })
}

// The view asks GitHub for the tickets by the project's labels; it does not
// ask when told not to, or when the project has no remote, and it says why
// there is no word on the tickets.
func TestCollectStatusView(t *testing.T) {
	inNewDir(t)
	cfg := &config.Config{}
	cfg.Project.Name = "demo"
	cfg.Git.IntegrationBranch, cfg.Git.ReleaseBranch = "develop", "main"
	cfg.GitHub.Labels = config.DefaultLabels()
	cfg.GitHub.Labels.Task = "work"

	var asked []string
	answer := func(_ context.Context, label, state, fields string) ([]byte, error) {
		asked = append(asked, label+"/"+state)
		if state == "open" {
			return []byte(`[{"number": 7, "title": "a ticket", "body": "", "labels": [{"name": "in-progress"}]}]`), nil
		}
		return []byte(`[]`), nil
	}

	replaceGitHub(t, true, answer)
	view := collectStatusView(context.Background(), status.Report{}, cfg, false)
	if view.Project != "demo" || view.Integration != "develop" || view.Release != "main" {
		t.Errorf("the project: %+v", view)
	}
	if view.Tickets == nil || len(view.Tickets.Open) != 1 || view.Tickets.Open[0].Stage != status.StageInProgress || view.NoTickets != "" {
		t.Errorf("the tickets: %+v (%q)", view.Tickets, view.NoTickets)
	}
	if len(asked) == 0 || asked[0] != "work/open" {
		t.Errorf("GitHub was asked %v, want the project's task label", asked)
	}

	asked = nil
	if view := collectStatusView(context.Background(), status.Report{}, cfg, true); view.Tickets != nil || !strings.Contains(view.NoTickets, "--offline") || len(asked) != 0 {
		t.Errorf("offline: %+v, asked %v", view, asked)
	}

	replaceGitHub(t, false, answer)
	if view := collectStatusView(context.Background(), status.Report{}, cfg, false); view.Tickets != nil || !strings.Contains(view.NoTickets, "no remote named origin") || len(asked) != 0 {
		t.Errorf("no remote: %+v, asked %v", view, asked)
	}

	replaceGitHub(t, true, func(context.Context, string, string, string) ([]byte, error) {
		return nil, errors.New("gh: not logged in")
	})
	if view := collectStatusView(context.Background(), status.Report{}, cfg, false); view.Tickets != nil || !strings.Contains(view.NoTickets, "gh: not logged in") || !strings.Contains(view.NoTickets, "awkit doctor") {
		t.Errorf("GitHub cannot be asked: %+v", view)
	}

	// The run: what the offline report and the marker say.
	if err := stopmark.Write(".", "stop-workflow", "worker_failed"); err != nil {
		t.Fatal(err)
	}
	report := status.Report{}
	report.Run.State, report.Run.PID, report.Run.StartTime = "running", 77, "2026-11-14T01:02:03Z"
	if view := collectStatusView(context.Background(), report, nil, true); !view.Running || view.PID != 77 || view.Since != "2026-11-14T01:02:03Z" || !view.Stopped || view.StopReason != "worker_failed" || view.Project != "" {
		t.Errorf("the run, without a configuration: %+v", view)
	}
	report.Run.State, report.Run.LockCorrupt, report.Run.LockFile = "lock_corrupt", true, filepath.Join(".ai", "state", "kickoff.lock")
	if view := collectStatusView(context.Background(), report, nil, true); len(view.Warnings) != 1 || !strings.Contains(view.Warnings[0], ".ai/state/kickoff.lock") || !strings.Contains(view.Warnings[0], "awkit reset --lock") {
		t.Errorf("a lock that cannot be read: %v", view.Warnings)
	}
	// The files of the issue that ran last are not this view's business.
	report = status.Report{Warnings: []string{"missing trace file: .ai/state/traces/issue-9.json"}}
	if view := collectStatusView(context.Background(), report, nil, true); len(view.Warnings) != 0 {
		t.Errorf("a warning about an issue's files is shown in the view: %v", view.Warnings)
	}
}

// The command, start to end, in a project with tickets.
func TestCmdStatus_ShowsTheTickets(t *testing.T) {
	dir := inNewDir(t)
	if err := os.MkdirAll(filepath.Join(dir, ".ai", "state"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".ai", "config.yaml"), []byte("project: {name: demo, type: single-repo}\nrepos: [{name: root, path: ./, type: root}]\ngit: {integration_branch: main, release_branch: main}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	asked := 0
	replaceGitHub(t, true, func(_ context.Context, label, state, _ string) ([]byte, error) {
		asked++
		if state == "open" {
			return []byte(`[{"number": 9, "title": "[feat] add login", "body": "", "labels": [{"name": "worker-failed"}]},
				{"number": 16, "title": "[feat] shop checkout", "body": "", "labels": []}]`), nil
		}
		return []byte(`[{"number": 1}]`), nil
	})

	out := captureStdout(t, func() {
		if code := cmdStatus(nil); code != 0 {
			t.Errorf("cmdStatus = %d, want 0", code)
		}
	})
	for _, want := range []string{"demo: pull requests go to main\n", "Tickets   2 open, 1 done\n", "  needs you\n    #9    [feat] add login\n", "  ready\n    #16   [feat] shop checkout\n", "Next      Look at #9"} {
		if !strings.Contains(out, want) {
			t.Errorf("awkit status does not show %q:\n%s", want, out)
		}
	}

	// As JSON, for a program: the report it always was, and the tickets.
	out = captureStdout(t, func() { cmdStatus([]string{"--json"}) })
	var parsed struct {
		Run     struct{ State string } `json:"run"`
		Tickets *status.Tickets        `json:"tickets"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("awkit status --json is not JSON: %v\n%s", err, out)
	}
	if parsed.Run.State != "no_lock" || parsed.Tickets == nil || len(parsed.Tickets.Open) != 2 || parsed.Tickets.Done != 1 {
		t.Errorf("awkit status --json: %+v", parsed)
	}

	// Offline, and for one issue's files, GitHub is not asked.
	asked = 0
	out = captureStdout(t, func() { cmdStatus([]string{"--offline"}) })
	if asked != 0 || !strings.Contains(out, "Tickets   not asked for (--offline)") {
		t.Errorf("awkit status --offline asked GitHub %d times:\n%s", asked, out)
	}
	out = captureStdout(t, func() { cmdStatus([]string{"--issue", "9"}) })
	if asked != 0 || !strings.Contains(out, "Artifacts(issue=9):") || strings.Contains(out, "Tickets") {
		t.Errorf("awkit status --issue 9 asked GitHub %d times:\n%s", asked, out)
	}
}

// A directory that is not set up is told how to set it up, and GitHub is not
// asked about a project that is not there.
func TestCmdStatus_NotSetUp(t *testing.T) {
	inNewDir(t)
	replaceGitHub(t, true, func(context.Context, string, string, string) ([]byte, error) {
		t.Error("GitHub was asked about a directory that is not set up")
		return nil, errors.New("no")
	})
	out := captureStdout(t, func() {
		if code := cmdStatus(nil); code != 0 {
			t.Errorf("cmdStatus = %d, want 0", code)
		}
	})
	if !strings.Contains(out, "Next      awkit init sets this directory up.") {
		t.Errorf("awkit status in a directory that is not set up:\n%s", out)
	}
}
