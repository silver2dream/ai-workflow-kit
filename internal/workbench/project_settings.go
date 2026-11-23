package workbench

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/landing"
	projectsettings "github.com/silver2dream/ai-workflow-kit/internal/settings"
)

// The settings page changes the council's roster and the part of the
// configuration a person changes. A change goes to the integration branch as
// every change of the project does, by `awkit settings apply` in a process of
// its own: through a pull request merged once its checks pass, after which
// the working tree is brought up to the branch. The page follows its record.

// SettingsView is a project's settings as the page shows them.
type SettingsView struct {
	// Version names the files as they were read: a change made to other
	// files than these is refused.
	Version string `json:"version"`
	// Roster is nil when the file cannot be read; RosterProblem says why, or
	// what keeps the council from meeting with it. RosterMissing says the
	// project has none yet: what is shown is the one its first plan writes.
	Roster        *projectsettings.Roster `json:"roster"`
	RosterMissing bool                    `json:"roster_missing,omitempty"`
	RosterProblem string                  `json:"roster_problem,omitempty"`
	Config        *projectsettings.Config `json:"config"`
	ConfigProblem string                  `json:"config_problem,omitempty"`
	Defaults      SettingsDefaults        `json:"defaults"`
	// Origin says the project has a remote named origin: a change goes
	// there.
	Origin bool `json:"origin"`
	// Busy is what keeps a change from starting now, nil when nothing does.
	Busy *Busy `json:"busy"`
	// Apply is where the last change made from the page is; nil when there
	// was none.
	Apply *projectsettings.Record `json:"apply"`
}

// SettingsDefaults are what a value left empty gets.
type SettingsDefaults struct {
	Roster projectsettings.RosterDefaults `json:"roster"`
	Config projectsettings.ConfigDefaults `json:"config"`
}

// Busy is what keeps the settings from being changed now: a run (which reads
// them), a meeting or an approval (which write and merge the files a change
// would merge too), or a change on its way.
type Busy struct {
	Kind string `json:"kind"`
	Plan string `json:"plan,omitempty"`
}

// The state of a change whose process is gone without saying it ended.
const applyStopped = "stopped"

// applyOf returns where the last change of the settings is; nil when there
// was none. One that said it was working, and whose process is gone, is said
// to have stopped.
func (s *Server) applyOf(root string) *projectsettings.Record {
	record, err := projectsettings.ReadRecord(root)
	if err != nil {
		return nil
	}
	if record.State == projectsettings.Working && !s.opts.Alive(record.PID) {
		record.State, record.Why = applyStopped, "stopped"
		record.Error = "the process that changed the settings is gone (it was stopped, or the computer was): save again to go on"
	}
	return record
}

// busy says what keeps the settings from being changed now.
func (s *Server) busy(root string) *Busy {
	if going, _ := s.runGoing(root); going {
		return &Busy{Kind: "run"}
	}
	if plan := s.meetingAnywhere(root); plan != "" {
		return &Busy{Kind: "meeting", Plan: plan}
	}
	if plan := s.approvalAnywhere(root); plan != "" {
		return &Busy{Kind: "approval", Plan: plan}
	}
	if record := s.applyOf(root); record != nil && record.State == projectsettings.Working {
		return &Busy{Kind: "settings"}
	}
	return nil
}

// settingsVersion names the settings files as they are.
func settingsVersion(files *projectsettings.Files) string {
	sum := sha256.New()
	if !files.RosterMissing {
		sum.Write(files.Roster)
	}
	sum.Write([]byte{0})
	sum.Write(files.Config)
	return hex.EncodeToString(sum.Sum(nil))[:16]
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request, root string) {
	files, err := projectsettings.ReadFiles(root)
	if err != nil {
		writeError(w, http.StatusConflict, "the settings cannot be read: "+err.Error())
		return
	}
	view := SettingsView{
		Version: settingsVersion(files), RosterMissing: files.RosterMissing,
		Defaults: SettingsDefaults{Roster: projectsettings.DefaultsOfRoster(), Config: projectsettings.DefaultsOfConfig()},
		Busy:     s.busy(root), Apply: s.applyOf(root),
	}
	if view.Roster, err = projectsettings.ReadRoster(files.Roster); err != nil {
		view.RosterProblem = err.Error()
	} else if err := projectsettings.CheckRoster(files.Roster); err != nil {
		view.RosterProblem = err.Error()
	}
	if view.Config, err = projectsettings.ReadConfig(files.Config); err != nil {
		view.ConfigProblem = err.Error()
	} else if err := projectsettings.CheckConfig(files.Config); err != nil {
		view.ConfigProblem = err.Error()
	}
	if out, err := landing.Exec(r.Context(), root, "git", "remote", "get-url", "origin"); err == nil && strings.TrimSpace(out) != "" {
		view.Origin = true
	}
	writeJSON(w, http.StatusOK, view)
}

// errorWithCode is an error the page tells apart by its code: busy, changed,
// invalid, conflict.
func errorWithCode(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"error": message, "code": code})
}

// handleSaveSettings starts a change of the settings: it is checked here, so
// that what a run could not use is said at once, and made by `awkit settings
// apply`.
func (s *Server) handleSaveSettings(w http.ResponseWriter, r *http.Request, root string) {
	var body struct {
		Version string                  `json:"version"`
		Roster  *projectsettings.Roster `json:"roster"`
		Config  *projectsettings.Config `json:"config"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Roster == nil && body.Config == nil {
		writeError(w, http.StatusBadRequest, "the request changes nothing")
		return
	}
	if busy := s.busy(root); busy != nil {
		message := map[string]string{
			"run":      "a run is going in this project: change the settings once it has stopped",
			"meeting":  "the meeting of plan " + busy.Plan + " is going on",
			"approval": "plan " + busy.Plan + " is being approved",
			"settings": "a change of the settings is on its way already",
		}[busy.Kind]
		errorWithCode(w, http.StatusConflict, "busy", message)
		return
	}
	files, err := projectsettings.ReadFiles(root)
	if err != nil {
		writeError(w, http.StatusConflict, "the settings cannot be read: "+err.Error())
		return
	}
	if body.Version != settingsVersion(files) {
		errorWithCode(w, http.StatusConflict, "changed", "the settings files were changed since the page read them: open them again")
		return
	}
	// The change is made to the settings as the page showed them: what it
	// leaves as it was is taken as the integration branch has it.
	request := projectsettings.Request{Roster: body.Roster, Config: body.Config}
	if body.Roster != nil {
		if request.WasRoster, err = projectsettings.ReadRoster(files.Roster); err != nil {
			errorWithCode(w, http.StatusUnprocessableEntity, "invalid", err.Error())
			return
		}
	}
	if body.Config != nil {
		if request.WasConfig, err = projectsettings.ReadConfig(files.Config); err != nil {
			errorWithCode(w, http.StatusUnprocessableEntity, "invalid", err.Error())
			return
		}
	}
	if _, _, err := request.Change(files); err != nil {
		errorWithCode(w, http.StatusUnprocessableEntity, "invalid", err.Error())
		return
	}

	file, err := projectsettings.WriteRequest(root, request)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.startApply(w, root, file)
}

// handleRetrySettings makes the last change asked for again, as it was
// asked: after its checks failed and were put right on GitHub, after its
// pull request was merged by hand, or after its process was stopped.
func (s *Server) handleRetrySettings(w http.ResponseWriter, r *http.Request, root string) {
	if busy := s.busy(root); busy != nil {
		errorWithCode(w, http.StatusConflict, "busy", "the settings cannot be changed now: "+busy.Kind)
		return
	}
	file := filepath.Join(projectsettings.Dir(root), projectsettings.RequestFile)
	if _, err := projectsettings.ReadRequest(file); err != nil {
		writeError(w, http.StatusConflict, "there is no change of the settings to make again: save them on the page")
		return
	}
	s.startApply(w, root, file)
}

// startApply starts `awkit settings apply` for a request file.
func (s *Server) startApply(w http.ResponseWriter, root, file string) {
	// A record from a change before is not this one's: the page waits for
	// the new process to write its own.
	_ = os.Remove(filepath.Join(projectsettings.Dir(root), projectsettings.RecordFile))
	log := filepath.Join(projectsettings.Dir(root), projectsettings.LogFile)
	if _, err := s.opts.Spawn(root, []string{"settings", "apply", "--request", file, "--state-root", root}, log); err != nil {
		writeError(w, http.StatusInternalServerError, "the change cannot be started: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
