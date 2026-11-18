package workbench

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/config"
	"github.com/silver2dream/ai-workflow-kit/internal/council"
	"github.com/silver2dream/ai-workflow-kit/internal/doctor"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
	"github.com/silver2dream/ai-workflow-kit/internal/status"
	"github.com/silver2dream/ai-workflow-kit/internal/trace"
)

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/session", s.handleSession)
	s.mux.HandleFunc("PUT /api/onboarding", s.handleOnboarding)
	s.mux.HandleFunc("POST /api/projects", s.handleAddProject)
	s.mux.HandleFunc("DELETE /api/projects/{id}", s.handleRemoveProject)
	s.mux.HandleFunc("GET /api/projects/{id}/status", s.project(s.handleStatus))
	s.mux.HandleFunc("GET /api/projects/{id}/tickets", s.project(s.handleTickets))
	s.mux.HandleFunc("GET /api/projects/{id}/events", s.project(s.handleEvents))
	s.mux.HandleFunc("GET /api/projects/{id}/health", s.project(s.handleHealth))
	s.mux.HandleFunc("GET /api/projects/{id}/plans", s.project(s.handlePlans))
	s.mux.HandleFunc("POST /api/projects/{id}/plans", s.project(s.handleStartPlan))
	s.mux.HandleFunc("GET /api/projects/{id}/roster", s.project(s.handleRoster))
	s.mux.HandleFunc("GET /api/projects/{id}/plans/{plan}", s.plan(s.handlePlan))
	s.mux.HandleFunc("GET /api/projects/{id}/plans/{plan}/transcript", s.plan(s.handleTranscript))
	s.mux.HandleFunc("POST /api/projects/{id}/plans/{plan}/say", s.plan(s.handleSay))
	s.mux.HandleFunc("POST /api/projects/{id}/plans/{plan}/pause", s.plan(s.handlePause))
	s.mux.HandleFunc("POST /api/projects/{id}/plans/{plan}/continue", s.plan(s.handleContinue))
	s.mux.HandleFunc("POST /api/projects/{id}/plans/{plan}/answers", s.plan(s.handleAnswers))
	s.mux.HandleFunc("PUT /api/projects/{id}/plans/{plan}/agenda", s.plan(s.handleAgenda))
	s.mux.HandleFunc("PUT /api/projects/{id}/plans/{plan}/tickets/{key}", s.plan(s.handleEditTicket))
	s.mux.HandleFunc("DELETE /api/projects/{id}/plans/{plan}/tickets/{key}", s.plan(s.handleDeleteTicket))
	s.mux.HandleFunc("POST /api/projects/{id}/plans/{plan}/approve", s.plan(s.handleApprove))
	s.mux.HandleFunc("POST /api/projects/{id}/run", s.project(s.handleStartRun))
	s.mux.HandleFunc("POST /api/projects/{id}/run/stop", s.project(s.handleStopRun))
	s.mux.HandleFunc("GET /api/projects/{id}/run/logs", s.project(s.handleRunLogs))
	s.mux.HandleFunc("GET /api/projects/{id}/run/output", s.project(s.handleRunOutput))
	s.mux.HandleFunc("GET /api/projects/{id}/stream", s.project(s.handleStream))
	s.mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "the workbench has no "+r.Method+" "+r.URL.Path)
	})
	s.mux.HandleFunc("/", s.serveAssets)
}

// Session is what the page needs to start: who it talks to and what the
// person has.
type Session struct {
	Version      string        `json:"version"`
	SettingsPath string        `json:"settings_path"`
	Projects     []ProjectView `json:"projects"`
	Onboarding   Onboarding    `json:"onboarding"`
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	settings, err := s.opts.Store.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	session := Session{Version: s.opts.Version, SettingsPath: s.opts.Store.Path(), Projects: []ProjectView{}, Onboarding: settings.Onboarding}
	for _, p := range settings.Projects {
		session.Projects = append(session.Projects, view(p))
	}
	if session.Onboarding.Done == nil {
		session.Onboarding.Done = []string{}
	}
	writeJSON(w, http.StatusOK, session)
}

func (s *Server) handleOnboarding(w http.ResponseWriter, r *http.Request) {
	var onboarding Onboarding
	if err := readJSON(r, &onboarding); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := onboarding.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	settings, err := s.opts.Store.Update(func(settings *Settings) error {
		settings.Onboarding = onboarding
		return nil
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, settings.Onboarding)
}

func (s *Server) handleAddProject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// The page asks for the folder's full path. Cleaned, a full path has no
	// ".." left in it; one that has is refused rather than guessed at.
	path := filepath.Clean(strings.TrimSpace(body.Path))
	if !filepath.IsAbs(path) || strings.Contains(path, "..") {
		writeError(w, http.StatusBadRequest, "give the folder's full path, as the file manager's address bar shows it")
		return
	}
	root, err := ResolveProject(path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := s.AddProject(root); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, view(Project{Path: root}))
}

// AddProject keeps a project in the person's settings: `awkit ui` adds the
// one it was started in.
func (s *Server) AddProject(root string) (ProjectView, error) {
	_, err := s.opts.Store.Update(func(settings *Settings) error {
		addProject(settings, root, s.opts.Now())
		return nil
	})
	return view(Project{Path: root}), err
}

func (s *Server) handleRemoveProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	removed := false
	_, err := s.opts.Store.Update(func(settings *Settings) error {
		removed = removeProject(settings, id)
		return nil
	})
	switch {
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error())
	case !removed:
		writeError(w, http.StatusNotFound, "the workbench has no such project")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// project hands a handler the root of the project the address names.
func (s *Server) project(handle func(w http.ResponseWriter, r *http.Request, root string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		settings, err := s.opts.Store.Load()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		root, ok := findProject(settings, r.PathValue("id"))
		if !ok {
			writeError(w, http.StatusNotFound, "the workbench has no such project: add it first")
			return
		}
		if _, err := os.Stat(layout.Config(root)); err != nil {
			writeError(w, http.StatusConflict, fmt.Sprintf("%s no longer has %s", root, layout.ConfigFile))
			return
		}
		handle(w, r, root)
	}
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request, root string) {
	report, err := status.Collect(root, status.Options{Now: s.opts.Now()})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// ticketCache keeps what GitHub said for a while: every page that is open
// asks, and GitHub is slow and counts the asking.
type ticketCache struct {
	at      time.Time
	tickets *status.Tickets
	err     string
}

const ticketsFresh = 20 * time.Second

// TicketsView is the work of a project, or why it cannot be shown.
type TicketsView struct {
	Tickets *status.Tickets `json:"tickets,omitempty"`
	Stages  []StageView     `json:"stages"`
	Error   string          `json:"error,omitempty"`
	AskedAt time.Time       `json:"asked_at"`
}

// StageView is a stage a ticket can be in, in the order they are shown.
type StageView struct {
	Stage string `json:"stage"`
	Title string `json:"title"`
}

func (s *Server) handleTickets(w http.ResponseWriter, r *http.Request, root string) {
	cached := s.ticketsOf(r.Context(), root, r.URL.Query().Get("fresh") == "1")
	stages := make([]StageView, 0, len(status.Stages))
	for _, st := range status.Stages {
		stages = append(stages, StageView{st.Stage, st.Title})
	}
	writeJSON(w, http.StatusOK, TicketsView{Tickets: cached.tickets, Stages: stages, Error: cached.err, AskedAt: cached.at})
}

func (s *Server) ticketsOf(ctx context.Context, root string, fresh bool) ticketCache {
	s.ticketsMu.Lock()
	cached, ok := s.tickets[root]
	s.ticketsMu.Unlock()
	if ok && !fresh && s.opts.Now().Sub(cached.at) < ticketsFresh {
		return cached
	}
	cached = ticketCache{at: s.opts.Now()}
	cfg, err := config.Load(root)
	if err != nil {
		cached.err = err.Error()
	} else {
		ask, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		tickets, err := status.CollectTickets(ask, cfg.GitHub.Labels, s.opts.IssuesIn(root))
		if err != nil {
			cached.err = "GitHub cannot be asked for the tickets: " + err.Error()
		} else {
			cached.tickets = tickets
		}
	}
	s.ticketsMu.Lock()
	s.tickets[root] = cached
	s.ticketsMu.Unlock()
	return cached
}

// issuesWithGhIn asks gh, in the project's folder, so that it asks the
// project's own repository.
func issuesWithGhIn(root string) status.IssueLister {
	return func(ctx context.Context, label, state, fields string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, "gh", "issue", "list", "--label", label, "--state", state, "--limit", "1000", "--json", fields)
		cmd.Dir = root
		out, err := cmd.Output()
		if err != nil {
			var exit *exec.ExitError
			if errors.As(err, &exit) && len(exit.Stderr) > 0 {
				return nil, errors.New(strings.TrimSpace(string(exit.Stderr)))
			}
			return nil, err
		}
		return out, nil
	}
}

// EventsView is what happened in the run that is going on, or went on last.
type EventsView struct {
	Session string        `json:"session,omitempty"`
	Events  []trace.Event `json:"events"`
}

const maxEvents = 500

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request, root string) {
	last := maxEvents
	if n, err := strconv.Atoi(r.URL.Query().Get("last")); err == nil && n > 0 && n < maxEvents {
		last = n
	}
	reader := trace.NewEventReader(root)
	session := latestSession(reader)
	if asked := r.URL.Query().Get("session"); asked != "" {
		// A session is named by the file a run left, never by a path the
		// page sends: "../" in it reads no other file.
		session = knownSession(reader, asked)
		if session == "" {
			writeError(w, http.StatusNotFound, "the project has no session "+strconv.Quote(asked))
			return
		}
	}
	out := EventsView{Session: session, Events: []trace.Event{}}
	if session != "" {
		events, err := reader.ReadSessionFiltered(session, trace.EventFilter{Last: last})
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if events != nil {
			out.Events = events
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// knownSession is the session of the project's events that is named so, or
// "" when it has none of that name.
func knownSession(reader *trace.EventReader, name string) string {
	sessions, err := reader.ListSessions()
	if err != nil {
		return ""
	}
	for _, session := range sessions {
		if session == name {
			return session
		}
	}
	return ""
}

// latestSession is the run going on, or else the newest one that left events.
func latestSession(reader *trace.EventReader) string {
	if id, err := reader.CurrentSessionID(); err == nil {
		if _, err := os.Stat(reader.GetSessionFilePath(id)); err == nil {
			return id
		}
	}
	sessions, err := reader.ListSessions()
	if err != nil || len(sessions) == 0 {
		return ""
	}
	return sessions[0] // the newest first
}

// CheckView is one check of `awkit doctor`.
type CheckView struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
	// CanClean says awkit reset clears what the check found.
	CanClean bool `json:"can_clean,omitempty"`
}

// handleHealth is awkit doctor, both halves: what a run needs (setup), and
// what earlier runs left behind (state).
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request, root string) {
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	view := func(results []doctor.CheckResult) []CheckView {
		checks := []CheckView{}
		for _, c := range results {
			checks = append(checks, CheckView{Name: c.Name, Status: c.Status, Message: c.Message, CanClean: c.CanClean})
		}
		return checks
	}
	setup := view(doctor.Setup(s.opts.SetupProbe(root)))
	state := view(doctor.New(root).RunAll(ctx))
	writeJSON(w, http.StatusOK, map[string]any{"setup": setup, "state": state})
}

// PlanView is a plan of the council: what it was asked, how far it went, and
// the work it laid out.
type PlanView struct {
	ID         string              `json:"id"`
	Brief      string              `json:"brief"`
	Milestones []council.Milestone `json:"milestones"`
	Tickets    []PlanTicket        `json:"tickets"`
	// Approved counts the tickets that are on GitHub.
	Approved int `json:"approved"`
	// Meeting is where its meeting is; nil for a plan held before meetings
	// said so.
	Meeting *MeetingState `json:"meeting,omitempty"`
	// Approval is where its approval from the page is, without what it said.
	Approval *ApprovalState `json:"approval,omitempty"`
}

// ApprovalState is where an approval is, in short.
type ApprovalState struct {
	State string `json:"state"`
	Step  string `json:"step"`
	Why   string `json:"why,omitempty"`
	Left  string `json:"left,omitempty"`
}

// PlanTicket is a ticket of a plan's work list.
type PlanTicket struct {
	Key       string   `json:"key"`
	Title     string   `json:"title"`
	Repo      string   `json:"repo"`
	Issue     int      `json:"issue,omitempty"`
	DependsOn []string `json:"depends_on,omitempty"`
}

// planIDs lists the plans of a project, newest first: the ones with a folder,
// and the ones the page started whose meeting has not written theirs yet (or
// could not start): the person finds them again in the list.
func planIDs(root string) []string {
	ids := council.Plans(root)
	known := map[string]bool{}
	for _, id := range ids {
		known[id] = true
	}
	entries, _ := os.ReadDir(filepath.Dir(council.StateDir(root, "plan")))
	for _, entry := range entries {
		id := entry.Name()
		if entry.IsDir() && !known[id] && council.IsPlanName(id) {
			if _, err := os.Stat(startedBrief(root, id)); err == nil {
				ids = append(ids, id)
			}
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(ids)))
	return ids
}

// briefOf reads the brief of a plan: the plan's own, or the one the page
// gave the meeting it started.
func briefOf(root, id string) string {
	if brief, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(council.PlansDir), id, "brief.md")); err == nil {
		return strings.TrimSpace(string(brief))
	}
	if brief, err := os.ReadFile(startedBrief(root, id)); err == nil {
		return strings.TrimSpace(string(brief))
	}
	return ""
}

func (s *Server) handlePlans(w http.ResponseWriter, r *http.Request, root string) {
	plans := []PlanView{}
	for _, id := range planIDs(root) {
		plan := PlanView{ID: id, Brief: briefOf(root, id), Milestones: []council.Milestone{}, Tickets: []PlanTicket{}, Meeting: s.meetingOf(root, id)}
		if approval := s.approvalOf(root, id); approval != nil {
			plan.Approval = &ApprovalState{State: approval.State, Step: approval.Step, Why: approval.Why, Left: approval.Left}
		}
		if list, err := council.LoadWorkList(root, id); err == nil {
			plan.Milestones = append(plan.Milestones, list.Milestones...)
			for _, t := range list.Tickets {
				plan.Tickets = append(plan.Tickets, PlanTicket{Key: t.Key, Title: t.Title, Repo: t.Repo, Issue: t.Issue, DependsOn: t.DependsOn})
				if t.Issue != 0 {
					plan.Approved++
				}
			}
		}
		plans = append(plans, plan)
	}
	writeJSON(w, http.StatusOK, map[string]any{"plans": plans})
}
