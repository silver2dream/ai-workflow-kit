package workbench

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/council"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
	"github.com/silver2dream/ai-workflow-kit/internal/task"
)

// A plan's meeting is held by `awkit plan`, started from the page in a
// process of its own (spawnAwkit): closing the page or the workbench does not
// end it. The page follows it in the files it keeps (council.ReadLive,
// ReadTranscript, ReadRecord), and takes part in it the way a person at the
// terminal does (council.Say, Pause, ReleaseAgenda, AnswerQuestions).

// MeetingState is where a plan's meeting is.
type MeetingState struct {
	// State is one of the council's (meeting, agenda, questions, paused,
	// finished, failed), or stopped: it said it was meeting, and the process
	// that held it is gone.
	State   string    `json:"state"`
	Note    string    `json:"note,omitempty"`
	Topic   int       `json:"topic"`
	Topics  int       `json:"topics"`
	Step    string    `json:"step,omitempty"`
	Speaker string    `json:"speaker,omitempty"`
	Calls   int       `json:"calls"`
	Limit   int       `json:"limit"`
	Updated time.Time `json:"updated"`
	// Questions counts the questions that wait for the owner's answers.
	Questions int `json:"questions"`
}

// The state of a meeting whose process is gone without saying it stopped.
const meetingStopped = "stopped"

// meetingOf returns where a plan's meeting is; nil for a plan whose meeting
// left no word of it (one held before meetings said so).
func (s *Server) meetingOf(root, id string) *MeetingState {
	live, err := council.ReadLive(root, id)
	if err != nil {
		return nil
	}
	state := &MeetingState{State: live.State, Note: live.Note, Topic: live.Topic, Topics: live.Topics, Step: live.Step,
		Speaker: live.Speaker, Calls: live.Calls, Limit: live.Limit, Updated: live.Updated}
	if live.State == council.LiveMeeting && !s.opts.Alive(live.PID) {
		state.State = meetingStopped
		state.Note = "the process that held the meeting is gone (it was stopped, or the computer was): go on with it"
	}
	if questions, err := council.Questions(root, id); err == nil {
		state.Questions = len(questions)
	}
	return state
}

// goingOn reports whether a plan's meeting is being held now.
func (s *Server) goingOn(root, id string) bool {
	meeting := s.meetingOf(root, id)
	return meeting != nil && meeting.State == council.LiveMeeting
}

// meetingAnywhere names the plan of the project whose meeting is being held,
// if one is: a project holds one meeting at a time, since every meeting
// writes the decision log and the documents.
func (s *Server) meetingAnywhere(root string) string {
	for _, id := range planIDs(root) {
		if s.goingOn(root, id) {
			return id
		}
	}
	return ""
}

// TicketView is a ticket of a work list, with the issue it became.
type TicketView struct {
	council.Ticket
	Issue int `json:"issue,omitempty"`
}

// PlanDetail is all there is to show of a plan.
type PlanDetail struct {
	ID      string        `json:"id"`
	Brief   string        `json:"brief"`
	Meeting *MeetingState `json:"meeting,omitempty"`
	// Record is what the meeting came to; nil for a plan held before
	// meetings wrote one.
	Record    *council.Record     `json:"record,omitempty"`
	Agenda    *council.Agenda     `json:"agenda,omitempty"`
	Questions []string            `json:"questions"`
	Words     []council.OwnerWord `json:"words"`
	// Milestones and Tickets are the work list as it stands, which the
	// person may have changed; ListProblem says why it would not be approved.
	Milestones  []council.Milestone `json:"milestones"`
	Tickets     []TicketView        `json:"tickets"`
	ListProblem string              `json:"list_problem,omitempty"`
	// Log is the end of what the meeting's process printed, when it is not
	// meeting: why it stopped, when it says so nowhere else.
	Log []string `json:"log,omitempty"`
}

// planIn reports whether a project has a plan: its brief is written, or the
// page just started its meeting, which writes the brief when it starts.
func planIn(root, id string) bool {
	if !council.IsPlanName(id) {
		return false
	}
	if _, err := os.Stat(startedBrief(root, id)); err == nil {
		return true
	}
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(council.PlansDir), id, "brief.md"))
	return err == nil
}

// plan wraps a handler of one plan of a project: the plan must be there.
func (s *Server) plan(handle func(w http.ResponseWriter, r *http.Request, root, id string)) http.HandlerFunc {
	return s.project(func(w http.ResponseWriter, r *http.Request, root string) {
		id := r.PathValue("plan")
		if !planIn(root, id) {
			writeError(w, http.StatusNotFound, "the project has no plan "+strconv.Quote(id))
			return
		}
		handle(w, r, root, id)
	})
}

// startedBrief is the brief a meeting started from the page is given.
func startedBrief(root, id string) string {
	return filepath.Join(council.StateDir(root, id), "brief-from-workbench.md")
}

func meetingLog(root, id string) string {
	return filepath.Join(council.StateDir(root, id), "meeting.log")
}

// tail returns the last lines of a file, at most n.
func tail(file string, n int) []string {
	f, err := os.Open(file)
	if err != nil {
		return nil
	}
	defer f.Close()
	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > n {
			lines = lines[1:]
		}
	}
	return lines
}

func (s *Server) handlePlan(w http.ResponseWriter, r *http.Request, root, id string) {
	detail := PlanDetail{ID: id, Brief: briefOf(root, id), Questions: []string{}, Words: []council.OwnerWord{}, Milestones: []council.Milestone{}, Tickets: []TicketView{}}
	detail.Meeting = s.meetingOf(root, id)
	if record, err := council.ReadRecord(root, id); err == nil {
		detail.Record = record
	}
	if agenda, err := council.LoadAgenda(root, id); err == nil {
		detail.Agenda = agenda
	}
	if questions, err := council.Questions(root, id); err == nil {
		detail.Questions = questions
	}
	if words, err := council.Words(root, id); err == nil {
		detail.Words = words
	}
	if list, err := council.LoadWorkList(root, id); err == nil {
		detail.Milestones = append(detail.Milestones, list.Milestones...)
		for _, ticket := range list.Tickets {
			detail.Tickets = append(detail.Tickets, TicketView{Ticket: ticket, Issue: ticket.Issue})
		}
		if repos, err := council.ProjectRepos(root); err == nil && len(list.Tickets) > 0 {
			if err := council.CheckWorkList(root, id, repos); err != nil {
				detail.ListProblem = err.Error()
			}
		}
	}
	if detail.Meeting == nil || detail.Meeting.State != council.LiveMeeting {
		detail.Log = tail(meetingLog(root, id), 30)
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) handleTranscript(w http.ResponseWriter, r *http.Request, root, id string) {
	since, _ := strconv.Atoi(r.URL.Query().Get("since"))
	entries, err := council.ReadTranscript(root, id, since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

// maxBrief is how long a brief from the page may be.
const maxBrief = 20000

// startMeeting starts `awkit plan` in a process of its own.
func (s *Server) startMeeting(root, id string, args []string) error {
	_, err := s.opts.Spawn(root, append(append([]string{"plan"}, args...), "--state-root", root), meetingLog(root, id))
	return err
}

func (s *Server) handleStartPlan(w http.ResponseWriter, r *http.Request, root string) {
	var body struct {
		Brief        string `json:"brief"`
		ReviewAgenda bool   `json:"review_agenda"`
		Ask          bool   `json:"ask"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	brief := strings.TrimSpace(body.Brief)
	switch {
	case brief == "":
		writeError(w, http.StatusBadRequest, "say what you want built: the brief is empty")
		return
	case len(brief) > maxBrief:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("the brief is %d characters long; at most %d", len(brief), maxBrief))
		return
	}
	if other := s.meetingAnywhere(root); other != "" {
		writeError(w, http.StatusConflict, "the meeting of plan "+other+" is going on: a project holds one meeting at a time")
		return
	}
	id := council.NewName(root, brief, s.opts.Now())
	file := startedBrief(root, id)
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := os.WriteFile(file, []byte(brief+"\n"), 0o644); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	args := []string{"--name", id, "--brief-file", file}
	if body.ReviewAgenda {
		args = append(args, "--review-agenda")
	}
	if body.Ask {
		args = append(args, "--ask-in-workbench")
	}
	if err := s.startMeeting(root, id, args); err != nil {
		writeError(w, http.StatusInternalServerError, "the meeting cannot be started: "+err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"id": id})
}

func (s *Server) handleSay(w http.ResponseWriter, r *http.Request, root, id string) {
	var body struct {
		Text     string `json:"text"`
		Question string `json:"question"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	word, err := council.Say(root, id, body.Text, body.Question, s.opts.Now())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, word)
}

func (s *Server) handlePause(w http.ResponseWriter, r *http.Request, root, id string) {
	if !s.goingOn(root, id) {
		writeError(w, http.StatusConflict, "the meeting of plan "+id+" is not going on")
		return
	}
	if err := council.Pause(root, id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// goOn starts the meeting of a plan again, where it stopped.
func (s *Server) goOn(w http.ResponseWriter, root, id string) {
	if s.goingOn(root, id) {
		writeError(w, http.StatusConflict, "the meeting of plan "+id+" is going on")
		return
	}
	if other := s.meetingAnywhere(root); other != "" {
		writeError(w, http.StatusConflict, "the meeting of plan "+other+" is going on: a project holds one meeting at a time")
		return
	}
	if meeting := s.meetingOf(root, id); meeting != nil && meeting.State == council.LiveAgenda {
		roster, err := council.LoadRoster(layout.Roster(root))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := council.ReleaseAgenda(root, id, roster); err != nil {
			writeError(w, http.StatusBadRequest, "the agenda cannot be held: "+err.Error())
			return
		}
	}
	if err := s.startMeeting(root, id, []string{"--resume", id}); err != nil {
		writeError(w, http.StatusInternalServerError, "the meeting cannot be gone on with: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleContinue(w http.ResponseWriter, r *http.Request, root, id string) {
	if questions, err := council.Questions(root, id); err == nil && len(questions) > 0 {
		writeError(w, http.StatusConflict, "the meeting waits for the answers to its questions")
		return
	}
	s.goOn(w, root, id)
}

func (s *Server) handleAnswers(w http.ResponseWriter, r *http.Request, root, id string) {
	var body struct {
		Answers []council.OwnerAnswer `json:"answers"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := council.AnswerQuestions(root, id, body.Answers); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.goOn(w, root, id)
}

func (s *Server) handleAgenda(w http.ResponseWriter, r *http.Request, root, id string) {
	var body struct {
		Questions []string        `json:"questions"`
		Topics    []council.Topic `json:"topics"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.goingOn(root, id) {
		writeError(w, http.StatusConflict, "the meeting of plan "+id+" is going on: pause it before changing its agenda")
		return
	}
	roster, err := council.LoadRoster(layout.Roster(root))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	agenda := &council.Agenda{Plan: id, Questions: body.Questions, Topics: body.Topics}
	// A topic that was taken up stays as it was: going on replays what was
	// said to it, and its documents are written.
	if record, err := council.ReadRecord(root, id); err == nil {
		for _, topic := range record.Topics {
			if !topic.Started {
				continue
			}
			at := topic.Number - 1
			if at >= len(agenda.Topics) || !reflect.DeepEqual(normalTopic(agenda.Topics[at]), normalTopic(topic.Topic)) {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("topic %d (%s) was taken up already: it stays as it was, in its place", topic.Number, topic.Topic.Title))
				return
			}
		}
	}
	if err := council.CheckAgenda(root, agenda, roster); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := council.SaveAgenda(root, agenda); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, agenda)
}

// normalTopic is a topic as it compares: no list is nil.
func normalTopic(topic council.Topic) council.Topic {
	for _, list := range []*[]string{&topic.Speakers, &topic.Reads, &topic.Writes} {
		if *list == nil {
			*list = []string{}
		}
	}
	return topic
}

func (s *Server) handleEditTicket(w http.ResponseWriter, r *http.Request, root, id string) {
	var text council.TicketText
	if err := readJSON(r, &text); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.changeList(w, root, id, func(repos []council.Repo) (*council.WorkList, error) {
		return council.EditTicket(root, id, r.PathValue("key"), text, repos)
	})
}

func (s *Server) handleDeleteTicket(w http.ResponseWriter, r *http.Request, root, id string) {
	s.changeList(w, root, id, func(repos []council.Repo) (*council.WorkList, error) {
		return council.DeleteTicket(root, id, r.PathValue("key"), repos)
	})
}

// changeList changes a plan's work list the way the person asked, while no
// meeting writes it.
func (s *Server) changeList(w http.ResponseWriter, root, id string, change func([]council.Repo) (*council.WorkList, error)) {
	if s.goingOn(root, id) {
		writeError(w, http.StatusConflict, "the meeting of plan "+id+" is going on: its work list is not laid out yet")
		return
	}
	repos, err := council.ProjectRepos(root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	list, err := change(repos)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	tickets := []TicketView{}
	for _, ticket := range list.Tickets {
		tickets = append(tickets, TicketView{Ticket: ticket, Issue: ticket.Issue})
	}
	writeJSON(w, http.StatusOK, map[string]any{"tickets": tickets})
}

// Approval is what approving a work list did.
type Approval struct {
	Created  []TicketView `json:"created"`
	Existing []TicketView `json:"existing"`
	// Error says why it stopped, when it stopped before every ticket was on
	// GitHub; what was created before stays, and approving again goes on.
	Error string `json:"error,omitempty"`
}

func (s *Server) handleApprove(w http.ResponseWriter, r *http.Request, root, id string) {
	if s.goingOn(root, id) {
		writeError(w, http.StatusConflict, "the meeting of plan "+id+" is going on: its work list is not laid out yet")
		return
	}
	repos, err := council.ProjectRepos(root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	approval, err := council.Approve(ctx, council.ApproveOptions{Root: root, ID: id, Repos: repos,
		Create: func(ctx context.Context, title, bodyFile string) (int, error) {
			return s.opts.CreateIssue(ctx, root, title, bodyFile)
		}})
	out := Approval{Created: []TicketView{}, Existing: []TicketView{}}
	if approval != nil {
		for _, ticket := range approval.Created {
			out.Created = append(out.Created, TicketView{Ticket: ticket, Issue: ticket.Issue})
		}
		for _, ticket := range approval.Existing {
			out.Existing = append(out.Existing, TicketView{Ticket: ticket, Issue: ticket.Issue})
		}
	}
	if err != nil {
		out.Error = err.Error()
		code := http.StatusBadGateway
		if len(out.Created) == 0 && !errors.Is(err, context.DeadlineExceeded) {
			code = http.StatusBadRequest
		}
		writeJSON(w, code, out)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// createIssue makes the issue of a ticket as awkit create-task does.
func createIssue(ctx context.Context, root, title, bodyFile string) (int, error) {
	result, err := task.CreateTask(ctx, task.CreateTaskOptions{Title: title, BodyFile: bodyFile, StateRoot: root})
	if err != nil {
		return 0, err
	}
	return result.IssueNumber, nil
}

// RoleView is a seat at the council's table, as the page shows it.
type RoleView struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Stance  string   `json:"stance"`
	Owns    []string `json:"owns"`
	Decides bool     `json:"decides,omitempty"`
	Plans   bool     `json:"plans,omitempty"`
	Model   string   `json:"model,omitempty"`
	Effort  string   `json:"effort,omitempty"`
}

// RosterView is who sits at the table, and how far a plan may go.
type RosterView struct {
	// Missing says the project has no roster yet: its first plan writes the
	// one that fits its repos.
	Missing bool       `json:"missing,omitempty"`
	Roles   []RoleView `json:"roles"`
	Topics  int        `json:"topics"`
	Calls   int        `json:"calls"`
}

func (s *Server) handleRoster(w http.ResponseWriter, r *http.Request, root string) {
	file := layout.Roster(root)
	if _, err := os.Stat(file); errors.Is(err, os.ErrNotExist) {
		writeJSON(w, http.StatusOK, RosterView{Missing: true, Roles: []RoleView{}})
		return
	}
	roster, err := council.LoadRoster(file)
	if err != nil {
		writeError(w, http.StatusConflict, fmt.Sprintf("%s cannot be read: %v", council.RosterPath, err))
		return
	}
	view := RosterView{Roles: []RoleView{}, Topics: roster.Limits.Topics, Calls: roster.Limits.Calls}
	for _, role := range roster.Roles {
		owns := role.Owns
		if owns == nil {
			owns = []string{}
		}
		view.Roles = append(view.Roles, RoleView{ID: role.ID, Title: role.Title, Stance: strings.TrimSpace(role.Stance), Owns: owns,
			Decides: role.Decides, Plans: role.Plans, Model: role.Voice().Model, Effort: role.Voice().Effort})
	}
	writeJSON(w, http.StatusOK, view)
}
