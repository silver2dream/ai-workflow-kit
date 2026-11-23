package workbench

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/council"
	projectsettings "github.com/silver2dream/ai-workflow-kit/internal/settings"
)

// The page reads the settings as the files have them, with what a value left
// empty gets, what keeps them from being changed now, and a name for the
// files as they were read.
func TestSettings_Read(t *testing.T) {
	s, root, base, _, alive := meetingProject(t, "")
	view := decode[SettingsView](t, do(t, s, http.MethodGet, base+"/settings", ""))
	if view.Version == "" || view.Roster == nil || view.RosterMissing || view.RosterProblem != "" || view.Config == nil || view.ConfigProblem != "" {
		t.Fatalf("view = %+v", view)
	}
	if view.Config.Name != "rally" || len(view.Config.Repos) != 1 || len(view.Roster.Roles) != 3 {
		t.Errorf("config %+v, roster %+v", view.Config, view.Roster)
	}
	if view.Defaults.Roster.Model == "" || view.Defaults.Config.ScoreThreshold != 7 || view.Busy != nil || view.Apply != nil || view.Origin {
		t.Errorf("defaults %+v, busy %+v, apply %+v, origin %v", view.Defaults, view.Busy, view.Apply, view.Origin)
	}

	// A roster the council cannot meet with is read all the same, and the
	// page is told why.
	write(t, root, ".ai/roster.yaml", strings.Replace(meetingRoster, "decides: true", "decides: false", 1))
	view = decode[SettingsView](t, do(t, s, http.MethodGet, base+"/settings", ""))
	if view.Roster == nil || !strings.Contains(view.RosterProblem, "decide") {
		t.Errorf("problem = %q", view.RosterProblem)
	}
	// A project without a roster is shown the one it would start with.
	if err := os.Remove(filepath.Join(root, ".ai", "roster.yaml")); err != nil {
		t.Fatal(err)
	}
	view = decode[SettingsView](t, do(t, s, http.MethodGet, base+"/settings", ""))
	if !view.RosterMissing || view.Roster == nil || len(view.Roster.Roles) < 2 {
		t.Errorf("missing roster: %+v", view)
	}

	// A change whose process is gone is said to have stopped.
	if err := projectsettings.WriteRecord(root, &projectsettings.Record{PID: 4242, State: projectsettings.Working, Step: "checks"}); err != nil {
		t.Fatal(err)
	}
	if view := decode[SettingsView](t, do(t, s, http.MethodGet, base+"/settings", "")); view.Busy == nil || view.Busy.Kind != "settings" {
		t.Errorf("busy = %+v", view.Busy)
	}
	*alive = false
	view = decode[SettingsView](t, do(t, s, http.MethodGet, base+"/settings", ""))
	if view.Apply == nil || view.Apply.State != applyStopped || view.Busy != nil {
		t.Errorf("apply %+v, busy %+v", view.Apply, view.Busy)
	}
}

func settingsBody(t *testing.T, view SettingsView, change func(*projectsettings.Roster, *projectsettings.Config)) string {
	t.Helper()
	change(view.Roster, view.Config)
	data, _ := json.Marshal(map[string]any{"version": view.Version, "roster": view.Roster, "config": view.Config})
	return string(data)
}

// A change is checked here and made by `awkit settings apply` in a process
// of its own, told what the page changed and what it was changed from.
func TestSettings_Save(t *testing.T) {
	s, root, base, spawn, _ := meetingProject(t, "")
	read := func() SettingsView { return decode[SettingsView](t, do(t, s, http.MethodGet, base+"/settings", "")) }

	stale := read()
	if err := projectsettings.WriteRecord(root, &projectsettings.Record{State: projectsettings.Failed, Why: "checks"}); err != nil {
		t.Fatal(err)
	}
	body := settingsBody(t, read(), func(r *projectsettings.Roster, c *projectsettings.Config) {
		r.Model = "opus"
		c.Repos[0].Test = "go test ./..."
	})
	if w := do(t, s, http.MethodPut, base+"/settings", body); w.Code != http.StatusAccepted {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	dir, args := spawn.last()
	request := filepath.Join(projectsettings.Dir(root), projectsettings.RequestFile)
	if dir != root || strings.Join(args, " ") != "settings apply --request "+request+" --state-root "+root {
		t.Errorf("spawned in %s: %v", dir, args)
	}
	if spawn.calls[len(spawn.calls)-1].log != filepath.Join(projectsettings.Dir(root), projectsettings.LogFile) {
		t.Errorf("log = %s", spawn.calls[len(spawn.calls)-1].log)
	}
	if _, err := projectsettings.ReadRecord(root); !os.IsNotExist(err) {
		t.Error("the record of the change before is still there")
	}
	sent, err := projectsettings.ReadRequest(request)
	if err != nil || sent.Roster == nil || sent.Roster.Model != "opus" || sent.WasRoster == nil || sent.WasRoster.Model == "opus" ||
		sent.Config == nil || sent.WasConfig == nil || sent.WasConfig.Repos[0].Test != "" {
		t.Errorf("request = %+v, %v", sent, err)
	}
	// The files are not touched here.
	if data, _ := os.ReadFile(filepath.Join(root, ".ai", "roster.yaml")); string(data) != meetingRoster {
		t.Error("the roster was changed in the working tree")
	}

	calls := len(spawn.calls)
	// Files changed since the page read them.
	write(t, root, ".ai/config.yaml", testConfig+"\n")
	if w := do(t, s, http.MethodPut, base+"/settings", settingsBody(t, stale, func(*projectsettings.Roster, *projectsettings.Config) {})); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), `"code":"changed"`) {
		t.Errorf("stale: %d %s", w.Code, w.Body)
	}
	// What the council could not meet with.
	if w := do(t, s, http.MethodPut, base+"/settings", settingsBody(t, read(), func(r *projectsettings.Roster, _ *projectsettings.Config) { r.Roles = r.Roles[:1] })); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), `"code":"invalid"`) {
		t.Errorf("invalid: %d %s", w.Code, w.Body)
	}
	// Nothing to change.
	if w := do(t, s, http.MethodPut, base+"/settings", `{"version": "x"}`); w.Code != http.StatusBadRequest {
		t.Errorf("nothing: %d %s", w.Code, w.Body)
	}
	// Not while a run goes, a meeting goes, a plan is approved.
	runningLock(t, root)
	if w := do(t, s, http.MethodPut, base+"/settings", settingsBody(t, read(), func(*projectsettings.Roster, *projectsettings.Config) {})); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), `"code":"busy"`) {
		t.Errorf("run: %d %s", w.Code, w.Body)
	}
	if err := os.Remove(filepath.Join(root, ".ai", "state", "kickoff.lock")); err != nil {
		t.Fatal(err)
	}
	setLive(t, root, "p1", council.LiveMeeting)
	if view := read(); view.Busy == nil || view.Busy.Kind != "meeting" || view.Busy.Plan != "p1" {
		t.Errorf("meeting: %+v", view.Busy)
	}
	setLive(t, root, "p1", council.LiveFinished)
	if err := council.WriteApproval(root, "p1", &council.ApprovalRecord{PID: 4242, State: council.ApprovalWorking}); err != nil {
		t.Fatal(err)
	}
	if view := read(); view.Busy == nil || view.Busy.Kind != "approval" {
		t.Errorf("approval: %+v", view.Busy)
	}
	if len(spawn.calls) != calls {
		t.Error("a change was started though it was refused")
	}
}

// The last change asked for can be made again as it was asked; there is
// none to make again before one was asked for.
func TestSettings_Retry(t *testing.T) {
	s, root, base, spawn, _ := meetingProject(t, "")
	if w := do(t, s, http.MethodPost, base+"/settings/retry", ""); w.Code != http.StatusConflict {
		t.Errorf("nothing to retry: %d %s", w.Code, w.Body)
	}
	file, err := projectsettings.WriteRequest(root, projectsettings.Request{Roster: &projectsettings.Roster{Model: "opus"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := projectsettings.WriteRecord(root, &projectsettings.Record{State: projectsettings.Failed, Why: "checks", PR: 7}); err != nil {
		t.Fatal(err)
	}
	if w := do(t, s, http.MethodPost, base+"/settings/retry", ""); w.Code != http.StatusAccepted {
		t.Fatalf("retry: %d %s", w.Code, w.Body)
	}
	if _, args := spawn.last(); strings.Join(args, " ") != "settings apply --request "+file+" --state-root "+root {
		t.Errorf("spawned %v", args)
	}
	if _, err := projectsettings.ReadRecord(root); !os.IsNotExist(err) {
		t.Error("the record of the change before is still there")
	}
	runningLock(t, root)
	if w := do(t, s, http.MethodPost, base+"/settings/retry", ""); w.Code != http.StatusConflict {
		t.Errorf("retry while a run goes: %d", w.Code)
	}
}
