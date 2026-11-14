package main

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/doctor"
	"github.com/silver2dream/ai-workflow-kit/internal/kickoff"
)

const workerBackendCheck = "codex CLI (worker backend: codex)"

// fakeProbe answers the doctor's questions from a table, and writes down
// which ones were asked.
type fakeProbe struct {
	failing  map[string]string // the checks that fail, by name, with their message
	asked    []string
	unclean  bool
	noWorker bool
}

func (f *fakeProbe) answer(name string) kickoff.CheckResult {
	f.asked = append(f.asked, name)
	if message, ok := f.failing[name]; ok {
		return kickoff.CheckResult{Name: name, Message: message}
	}
	return kickoff.CheckResult{Name: name, Passed: true, Message: "fine"}
}

func (f *fakeProbe) CheckGhCLI() kickoff.CheckResult     { return f.answer("gh CLI") }
func (f *fakeProbe) CheckGhAuth() kickoff.CheckResult    { return f.answer("gh Auth") }
func (f *fakeProbe) CheckClaudeCLI() kickoff.CheckResult { return f.answer("claude CLI") }
func (f *fakeProbe) CheckConfig() kickoff.CheckResult    { return f.answer("Config") }
func (f *fakeProbe) CheckRepoPaths() kickoff.CheckResult { return f.answer("Repo Paths") }
func (f *fakeProbe) CheckIntegrationBranch() kickoff.CheckResult {
	return f.answer("Integration Branch")
}
func (f *fakeProbe) CheckGitHubRepo() kickoff.CheckResult { return f.answer("GitHub Repo") }
func (f *fakeProbe) CheckRepoWritePermission() kickoff.CheckResult {
	return f.answer("Repo Write Access")
}
func (f *fakeProbe) CheckPermissions() kickoff.CheckResult { return f.answer("Permissions") }
func (f *fakeProbe) CheckAgents() kickoff.CheckResult      { return f.answer("Agents") }
func (f *fakeProbe) CheckGitRemote() kickoff.CheckResult   { return f.answer("Git Remote") }

func (f *fakeProbe) CheckCodexCLI() kickoff.CheckResult {
	f.asked = append(f.asked, "worker backend")
	if f.noWorker {
		return kickoff.CheckResult{Name: workerBackendCheck, Passed: true, Warning: true, Message: "codex not installed (worker execution may fail)"}
	}
	return kickoff.CheckResult{Name: workerBackendCheck, Passed: true, Message: "codex installed"}
}

func (f *fakeProbe) CheckGitClean() kickoff.CheckResult {
	f.asked = append(f.asked, "Git Status")
	if f.unclean {
		return kickoff.CheckResult{Name: "Git Status", Message: "Working directory not clean. Please commit or stash changes."}
	}
	return kickoff.CheckResult{Name: "Git Status", Passed: true, Message: "Working directory clean"}
}

func statusOf(results []doctor.CheckResult) map[string]string {
	status := map[string]string{}
	for _, r := range results {
		status[r.Name] = r.Status
	}
	return status
}

// A run asks these questions one at a time and stops at the first that
// fails. The doctor asks them all, so that everything that is wrong is said
// at once.
func TestSetupChecks_AsksEverything(t *testing.T) {
	probe := &fakeProbe{}
	results := setupChecks(probe)

	want := []string{"gh CLI", "gh Auth", "claude CLI", "Config", "worker backend", "Repo Paths", "Permissions", "Agents", "Git Remote", "Integration Branch", "GitHub Repo", "Repo Write Access", "Git Status"}
	if !reflect.DeepEqual(probe.asked, want) {
		t.Errorf("asked %v\nwant  %v", probe.asked, want)
	}
	for _, r := range results {
		if r.Status != "ok" {
			t.Errorf("%s: %s (%s), want ok", r.Name, r.Status, r.Message)
		}
	}
	// A clean working tree is not worth a line.
	if _, said := statusOf(results)["Git Status"]; said {
		t.Error("a clean working tree is reported")
	}
	if errors, warnings := doctorTotals(results); errors != 0 || warnings != 0 {
		t.Errorf("totals = %d errors, %d warnings", errors, warnings)
	}
}

// Several things wrong at once are all said, and a question whose answer
// depends on something that failed is not asked.
func TestSetupChecks_SaysAllThatIsWrong(t *testing.T) {
	probe := &fakeProbe{
		failing: map[string]string{
			"gh Auth":            "gh not authenticated. Run: gh auth login",
			"claude CLI":         "claude CLI not installed",
			"Integration Branch": "the branch does not exist",
		},
		unclean:  true,
		noWorker: true,
	}
	results := setupChecks(probe)
	status := statusOf(results)

	for name, want := range map[string]string{
		"gh CLI":             "ok",
		"gh Auth":            "error",
		"claude CLI":         "error",
		"Config":             "ok",
		"Integration Branch": "error",
		"Git Status":         "warning", // no fault of the project: a run only wants it clean
		workerBackendCheck:   "error",   // every ticket fails without it; a run only warns
	} {
		if status[name] != want {
			t.Errorf("%s is %q, want %q", name, status[name], want)
		}
	}
	// Without a login there is no word on the GitHub side.
	for _, skipped := range []string{"GitHub Repo", "Repo Write Access"} {
		if _, said := status[skipped]; said {
			t.Errorf("%s was asked though gh is not logged in", skipped)
		}
	}
	for _, r := range results {
		switch r.Name {
		case workerBackendCheck:
			if !strings.Contains(r.Message, "the Worker runs it for every ticket") || !strings.Contains(r.Message, "worker.backend") || strings.Contains(r.Message, "may fail") {
				t.Errorf("the missing worker backend: %q", r.Message)
			}
		case "Git Status":
			if !strings.Contains(r.Message, "git add -A && git commit") {
				t.Errorf("the working tree: %q, want the command that cleans it", r.Message)
			}
		}
	}
	if errors, warnings := doctorTotals(results); errors != 4 || warnings != 1 {
		t.Errorf("totals = %d errors, %d warnings; want 4 and 1", errors, warnings)
	}
}

// What cannot be answered is not asked: without gh nothing about GitHub,
// without a configuration nothing about what it names, without a remote
// nothing about what is on it.
func TestSetupChecks_SkipsWhatCannotBeAnswered(t *testing.T) {
	for _, tc := range []struct {
		name    string
		failing string
		skipped []string
		asked   []string
	}{
		{"no gh", "gh CLI", []string{"gh Auth", "GitHub Repo", "Repo Write Access"}, []string{"claude CLI", "Config", "Git Remote", "Integration Branch"}},
		{"no configuration", "Config", []string{"Repo Paths", "Permissions", "Agents", "Integration Branch"}, []string{"gh Auth", "Git Remote", "GitHub Repo", "Repo Write Access", "worker backend"}},
		{"no remote", "Git Remote", []string{"Integration Branch", "GitHub Repo", "Repo Write Access"}, []string{"gh Auth", "Config", "Repo Paths"}},
	} {
		probe := &fakeProbe{failing: map[string]string{tc.failing: "not there"}}
		setupChecks(probe)
		asked := map[string]bool{}
		for _, name := range probe.asked {
			asked[name] = true
		}
		for _, name := range tc.skipped {
			if asked[name] {
				t.Errorf("%s: %s was asked", tc.name, name)
			}
		}
		for _, name := range tc.asked {
			if !asked[name] {
				t.Errorf("%s: %s was not asked", tc.name, name)
			}
		}
	}
}

// The doctor's last line is what a person does next, and its exit code says
// whether a run can start.
func TestPrintDoctor(t *testing.T) {
	setup := []doctor.CheckResult{
		{Name: "gh CLI", Status: "ok", Message: "gh CLI installed"},
		{Name: "Git Remote", Status: "error", Message: "the repository has no remote named origin"},
		{Name: "Git Status", Status: "warning", Message: "the working tree has changes"},
	}
	state := []doctor.CheckResult{{Name: "Loop Count", Status: "warning", Message: "loop_count = 9", CanClean: true}}

	var out bytes.Buffer
	if code := printDoctor(&out, setup, state); code != 1 {
		t.Errorf("exit code = %d, want 1: something has to be fixed before a run", code)
	}
	for _, want := range []string{
		"What a run needs\n",
		"  ✓ gh CLI: gh CLI installed\n",
		"  ✗ Git Remote: the repository has no remote named origin\n",
		"  ! Git Status: the working tree has changes\n",
		"What earlier runs left behind\n",
		"  ! Loop Count: loop_count = 9\n",
		"1 problem to fix before a run, and 2 warnings.\n",
		"awkit reset",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the doctor does not say %q:\n%s", want, out.String())
		}
	}

	out.Reset()
	if code := printDoctor(&out, setup[:1], nil); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "Nothing stands in the way of a run.\n") || !strings.Contains(out.String(), "What earlier runs left behind\n  nothing\n") || strings.Contains(out.String(), "awkit reset") {
		t.Errorf("a project with nothing wrong:\n%s", out.String())
	}

	for _, tc := range []struct {
		errors, warnings int
		want             string
	}{
		{0, 0, "Nothing stands in the way of a run."},
		{0, 1, "Nothing stands in the way of a run. 1 warning."},
		{0, 3, "Nothing stands in the way of a run. 3 warnings."},
		{1, 0, "1 problem to fix before a run."},
		{2, 0, "2 problems to fix before a run."},
		{2, 1, "2 problems to fix before a run, and 1 warning."},
	} {
		if got := doctorVerdict(tc.errors, tc.warnings); got != tc.want {
			t.Errorf("doctorVerdict(%d, %d) = %q, want %q", tc.errors, tc.warnings, got, tc.want)
		}
	}
}
