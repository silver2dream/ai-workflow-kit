package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/council"
)

// A table of three for the tests: one decides, one plans, each owns a file.
const meetingRoster = `
limits:
  calls: 60
roles:
  - id: owner
    title: Product Owner
    decides: true
    stance: You answer for what is built.
    owns: [docs/product/requirements.md]
  - id: builder
    title: Builder
    plans: true
    stance: You answer for how it is built.
    owns: [docs/tech/architecture.md]
  - id: skeptic
    title: Skeptic
    stance: You look for how it fails.
    owns: [docs/qa/risks.md]
`

// spawned is what the workbench started, in place of awkit.
type spawned struct {
	mu    sync.Mutex
	calls []struct {
		root string
		args []string
		log  string
	}
}

func (s *spawned) spawn(root string, args []string, log string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, struct {
		root string
		args []string
		log  string
	}{root, args, log})
	return 4242, nil
}

func (s *spawned) last() (string, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.calls) == 0 {
		return "", nil
	}
	c := s.calls[len(s.calls)-1]
	return c.root, c.args
}

// meetingProject is a workbench with a project that has a roster and a plan
// p1 of two topics, whose meeting is in the state given (none when empty).
func meetingProject(t *testing.T, state string) (*Server, string, string, *spawned, *bool) {
	t.Helper()
	s, root, base := added(t, nil)
	write(t, root, ".ai/roster.yaml", meetingRoster)
	write(t, root, "docs/plans/p1/brief.md", "A health endpoint.\n")
	agenda := &council.Agenda{Plan: "p1", Questions: []string{"Who calls it?"}, Topics: []council.Topic{
		{ID: "scope", Title: "What is built", Question: "What does it do?", Lead: "owner", Speakers: []string{"skeptic"}, Writes: []string{"docs/product/requirements.md"}},
		{ID: "build", Title: "How it is built", Question: "How?", Lead: "builder", Speakers: []string{"skeptic"}, Writes: []string{"docs/tech/architecture.md"}},
	}}
	if err := council.SaveAgenda(root, agenda); err != nil {
		t.Fatal(err)
	}
	alive := true
	spawn := &spawned{}
	s.opts.Spawn = spawn.spawn
	s.opts.Alive = func(int) bool { return alive }
	if state != "" {
		setLive(t, root, "p1", state)
	}
	return s, root, base, spawn, &alive
}

func setLive(t *testing.T, root, id, state string) {
	t.Helper()
	live := council.Live{PID: 4242, State: state, Topic: 1, Topics: 2, Step: "topic scope skeptic speaks", Speaker: "skeptic", Calls: 3, Limit: 60}
	data, _ := json.Marshal(live)
	write(t, root, ".ai/state/council/"+id+"/live.json", string(data))
}

// A meeting started from the page runs in a process of its own, as awkit
// plan would at the terminal, named before it starts so that the page can
// follow it.
func TestMeetings_Start(t *testing.T) {
	s, root, base, spawn, _ := meetingProject(t, "")
	if w := do(t, s, http.MethodPost, base+"/plans", `{"brief": "  "}`); w.Code != http.StatusBadRequest {
		t.Errorf("an empty brief: %d", w.Code)
	}
	w := do(t, s, http.MethodPost, base+"/plans", `{"brief": "A daily login reward.", "review_agenda": true, "ask": true}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("start: %d %s", w.Code, w.Body)
	}
	id := decode[map[string]string](t, w)["id"]
	if !strings.HasSuffix(id, "-a-daily-login-reward") {
		t.Errorf("id = %q", id)
	}
	dir, args := spawn.last()
	brief := filepath.Join(council.StateDir(root, id), "brief-from-workbench.md")
	want := []string{"plan", "--name", id, "--brief-file", brief, "--review-agenda", "--ask-in-workbench", "--state-root", root}
	if dir != root || strings.Join(args, "|") != strings.Join(want, "|") {
		t.Errorf("spawned in %s: %v\nwant %v", dir, args, want)
	}
	// The page follows the meeting at once, before it wrote its brief.
	if w := do(t, s, http.MethodGet, base+"/plans/"+id, ""); w.Code != http.StatusOK || decode[PlanDetail](t, w).Brief != "A daily login reward." {
		t.Errorf("a meeting just started: %d %s", w.Code, w.Body)
	}
	if data, _ := os.ReadFile(brief); string(data) != "A daily login reward.\n" {
		t.Errorf("brief = %q", data)
	}
	if spawn.calls[0].log != filepath.Join(council.StateDir(root, id), "meeting.log") {
		t.Errorf("log = %s", spawn.calls[0].log)
	}
	// It is in the list of plans at once too, with the brief it was given:
	// one that could not start is found again there.
	listed := false
	for _, plan := range decode[map[string][]PlanView](t, do(t, s, http.MethodGet, base+"/plans", ""))["plans"] {
		listed = listed || plan.ID == id && plan.Brief == "A daily login reward."
	}
	if !listed {
		t.Errorf("the plan just started is not listed")
	}

	// One meeting at a time in a project.
	setLive(t, root, "p1", council.LiveMeeting)
	if w := do(t, s, http.MethodPost, base+"/plans", `{"brief": "Another thing."}`); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "p1") {
		t.Errorf("a second meeting: %d %s", w.Code, w.Body)
	}
}

// The page sees where a meeting is; a meeting whose process is gone without
// a word is said to have stopped.
func TestMeetings_Detail(t *testing.T) {
	s, root, base, _, alive := meetingProject(t, council.LiveMeeting)
	if _, err := council.Say(root, "p1", "Phones first.", "", time.Now()); err != nil {
		t.Fatal(err)
	}
	write(t, root, ".ai/state/council/p1/transcript.jsonl",
		`{"seq": 1, "step": "moderator sets the agenda", "kind": "agenda", "role": "moderator"}`+"\n"+
			`{"seq": 2, "step": "topic scope owner proposes", "topic": "scope", "kind": "proposal", "role": "owner", "data": {"says": "Build it."}}`+"\n")
	write(t, root, ".ai/state/council/p1/meeting.log", "AWK Plan: 3 roles at the table\nline two\n")

	raw := do(t, s, http.MethodGet, base+"/plans/p1", "").Body.String()
	// The page reads what the council writes by the names JSON gives it.
	for _, want := range []string{`"agenda":{"plan":"p1","questions":["Who calls it?"],"topics":[{"id":"scope"`, `"meeting":{"state":"meeting"`} {
		if !strings.Contains(raw, want) {
			t.Errorf("the detail lacks %s:\n%s", want, raw)
		}
	}
	detail := decode[PlanDetail](t, do(t, s, http.MethodGet, base+"/plans/p1", ""))
	if detail.Brief != "A health endpoint." || detail.Meeting == nil || detail.Meeting.State != council.LiveMeeting || detail.Meeting.Speaker != "skeptic" ||
		detail.Agenda == nil || len(detail.Agenda.Topics) != 2 || len(detail.Words) != 1 || detail.Log != nil {
		t.Errorf("detail = %+v", detail)
	}
	*alive = false
	detail = decode[PlanDetail](t, do(t, s, http.MethodGet, base+"/plans/p1", ""))
	if detail.Meeting.State != meetingStopped || !strings.Contains(detail.Meeting.Note, "gone") || len(detail.Log) != 2 {
		t.Errorf("a meeting whose process is gone = %+v, log %v", detail.Meeting, detail.Log)
	}
	plans := decode[struct {
		Plans []PlanView `json:"plans"`
	}](t, do(t, s, http.MethodGet, base+"/plans", ""))
	if len(plans.Plans) != 1 || plans.Plans[0].Meeting == nil || plans.Plans[0].Meeting.State != meetingStopped {
		t.Errorf("plans = %+v", plans)
	}

	transcript := decode[struct {
		Entries []council.Entry `json:"entries"`
	}](t, do(t, s, http.MethodGet, base+"/plans/p1/transcript?since=1", ""))
	if len(transcript.Entries) != 1 || transcript.Entries[0].Kind != council.EntryProposal {
		t.Errorf("transcript since 1 = %+v", transcript.Entries)
	}
	if w := do(t, s, http.MethodGet, base+"/plans/nothing", ""); w.Code != http.StatusNotFound {
		t.Errorf("a plan that is not there: %d", w.Code)
	}
	if w := do(t, s, http.MethodGet, base+"/plans/..%2F..%2Fx", ""); w.Code != http.StatusNotFound {
		t.Errorf("a path for a plan: %d", w.Code)
	}
}

// The owner says something, and pauses the meeting, from the page.
func TestMeetings_SayAndPause(t *testing.T) {
	s, root, base, _, alive := meetingProject(t, council.LiveMeeting)
	w := do(t, s, http.MethodPost, base+"/plans/p1/say", `{"text": "Phones first.", "question": "Who calls it?"}`)
	if w.Code != http.StatusOK || decode[council.OwnerWord](t, w).ID != "W1" {
		t.Errorf("say: %d %s", w.Code, w.Body)
	}
	if w := do(t, s, http.MethodPost, base+"/plans/p1/say", `{"text": ""}`); w.Code != http.StatusBadRequest {
		t.Errorf("say nothing: %d", w.Code)
	}
	if words, _ := council.Words(root, "p1"); len(words) != 1 || words[0].Question != "Who calls it?" {
		t.Errorf("words = %+v", words)
	}
	if w := do(t, s, http.MethodPost, base+"/plans/p1/pause", ""); w.Code != http.StatusNoContent {
		t.Errorf("pause: %d %s", w.Code, w.Body)
	}
	if _, err := os.Stat(filepath.Join(council.StateDir(root, "p1"), "pause")); err != nil {
		t.Errorf("no pause was asked for: %v", err)
	}
	*alive = false
	if w := do(t, s, http.MethodPost, base+"/plans/p1/pause", ""); w.Code != http.StatusConflict {
		t.Errorf("pause a meeting that is not going on: %d", w.Code)
	}
}

// Going on: once the owner looked at the agenda, or answered the questions,
// the meeting is started again where it stopped.
func TestMeetings_GoOn(t *testing.T) {
	s, root, base, spawn, alive := meetingProject(t, council.LiveMeeting)
	if w := do(t, s, http.MethodPost, base+"/plans/p1/continue", ""); w.Code != http.StatusConflict {
		t.Errorf("go on with a meeting that is going on: %d", w.Code)
	}
	*alive = false
	setLive(t, root, "p1", council.LiveAgenda)
	if w := do(t, s, http.MethodPost, base+"/plans/p1/continue", ""); w.Code != http.StatusAccepted {
		t.Fatalf("go on after the agenda: %d %s", w.Code, w.Body)
	}
	if _, err := os.Stat(filepath.Join(council.StateDir(root, "p1"), "agenda-released")); err != nil {
		t.Error("the agenda was not let go")
	}
	if dir, args := spawn.last(); dir != root || strings.Join(args, " ") != "plan --resume p1 --state-root "+root {
		t.Errorf("spawned: %v", args)
	}

	// The questions wait for their answers.
	setLive(t, root, "p1", council.LiveQuestions)
	write(t, root, ".ai/state/council/p1/questions.json", `["Who calls it?", "How often?"]`)
	before := len(spawn.calls)
	if w := do(t, s, http.MethodPost, base+"/plans/p1/continue", ""); w.Code != http.StatusConflict {
		t.Errorf("go on without the answers: %d", w.Code)
	}
	if w := do(t, s, http.MethodPost, base+"/plans/p1/answers", `{"answers": [{"question": "Who is it?", "answer": "x"}]}`); w.Code != http.StatusBadRequest {
		t.Errorf("an answer to a question nobody asked: %d", w.Code)
	}
	if len(spawn.calls) != before {
		t.Error("the meeting went on without its answers")
	}
	if w := do(t, s, http.MethodPost, base+"/plans/p1/answers", `{"answers": [{"question": "Who calls it?", "answer": "The probe."}, {"question": "How often?", "answer": ""}]}`); w.Code != http.StatusAccepted {
		t.Fatalf("answers: %d %s", w.Code, w.Body)
	}
	if len(spawn.calls) != before+1 {
		t.Error("the meeting did not go on with the answers")
	}
}

// The owner changes the agenda before the meeting holds it: what was taken
// up stays as it was, and an agenda the table cannot hold is not kept.
func TestMeetings_Agenda(t *testing.T) {
	s, root, base, _, alive := meetingProject(t, council.LiveMeeting)
	changed := `{"questions": [], "topics": [
	  {"id": "scope", "title": "What is built, for a phone", "question": "What does it do?", "lead": "owner", "speakers": ["skeptic"], "reads": [], "writes": ["docs/product/requirements.md"]}
	]}`
	if w := do(t, s, http.MethodPut, base+"/plans/p1/agenda", changed); w.Code != http.StatusConflict {
		t.Errorf("change the agenda of a meeting that is going on: %d", w.Code)
	}
	*alive = false
	if w := do(t, s, http.MethodPut, base+"/plans/p1/agenda", strings.Replace(changed, `"lead": "owner"`, `"lead": "nobody"`, 1)); w.Code != http.StatusBadRequest {
		t.Errorf("an agenda the table cannot hold: %d %s", w.Code, w.Body)
	}
	if w := do(t, s, http.MethodPut, base+"/plans/p1/agenda", changed); w.Code != http.StatusOK {
		t.Fatalf("change the agenda: %d %s", w.Code, w.Body)
	}
	if agenda, _ := council.LoadAgenda(root, "p1"); len(agenda.Topics) != 1 || agenda.Topics[0].Title != "What is built, for a phone" {
		t.Errorf("agenda = %+v", agenda)
	}

	// Topic 1 was taken up: it stays.
	write(t, root, ".ai/state/council/p1/record.json", `{"id": "p1", "topics": [{"number": 1, "started": true, "topic":
	  {"id": "scope", "title": "What is built, for a phone", "question": "What does it do?", "lead": "owner", "speakers": ["skeptic"], "reads": [], "writes": ["docs/product/requirements.md"]}}]}`)
	if w := do(t, s, http.MethodPut, base+"/plans/p1/agenda", strings.Replace(changed, "for a phone", "for a watch", 1)); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "taken up already") {
		t.Errorf("change a topic that was taken up: %d %s", w.Code, w.Body)
	}
}

func putWorkList(t *testing.T, root string) {
	t.Helper()
	list := &council.WorkList{Plan: "p1", Tickets: []council.Ticket{
		{Key: "t1", Title: "[chore] set up the module", Repo: "root", Summary: "The module.", Scope: []string{"go.mod"}, Acceptance: []string{"go build passes"}},
		{Key: "t2", Title: "[feat] serve /healthz", Repo: "root", Summary: "The endpoint.", Scope: []string{"handler"}, Acceptance: []string{"answers 200"}, DependsOn: []string{"t1"}},
	}}
	if err := council.SaveWorkList(root, list); err != nil {
		t.Fatal(err)
	}
}

// The owner changes and drops tickets before approving them; each change is
// kept only when the list would still be approved.
func TestMeetings_Tickets(t *testing.T) {
	s, root, base, _, _ := meetingProject(t, council.LiveFinished)
	putWorkList(t, root)
	w := do(t, s, http.MethodPut, base+"/plans/p1/tickets/t2", `{"title": "[feat] serve /health", "summary": "The endpoint.", "scope": ["handler"], "acceptance": ["answers 200"], "testing": []}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "[feat] serve /health") {
		t.Errorf("change a ticket: %d %s", w.Code, w.Body)
	}
	if w := do(t, s, http.MethodPut, base+"/plans/p1/tickets/t2", `{"title": "serve it", "summary": "x", "scope": ["a"], "acceptance": ["b"], "testing": []}`); w.Code != http.StatusBadRequest {
		t.Errorf("a title out of form: %d", w.Code)
	}
	if w := do(t, s, http.MethodDelete, base+"/plans/p1/tickets/t1", ""); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "t2 waits for t1") {
		t.Errorf("drop a ticket another waits for: %d %s", w.Code, w.Body)
	}
	if w := do(t, s, http.MethodDelete, base+"/plans/p1/tickets/t2", ""); w.Code != http.StatusOK {
		t.Errorf("drop a ticket: %d %s", w.Code, w.Body)
	}
	if list, _ := council.LoadWorkList(root, "p1"); len(list.Tickets) != 1 {
		t.Errorf("tickets = %+v", list.Tickets)
	}
}

// Approving the work list creates the issues; one that failed half way says
// what was created, and approving again goes on.
func TestMeetings_Approve(t *testing.T) {
	s, root, base, _, _ := meetingProject(t, council.LiveFinished)
	putWorkList(t, root)
	next, fail := 100, true
	s.opts.CreateIssue = func(ctx context.Context, dir, title, bodyFile string) (int, error) {
		if dir != root {
			t.Errorf("an issue was created from %s", dir)
		}
		if title == "[feat] serve /healthz" && fail {
			return 0, errors.New("GitHub is down")
		}
		body, _ := os.ReadFile(bodyFile)
		if !strings.Contains(string(body), "awk-ticket") {
			t.Errorf("the body has no block:\n%s", body)
		}
		next++
		return next, nil
	}
	w := do(t, s, http.MethodPost, base+"/plans/p1/approve", "")
	got := decode[Approval](t, w)
	if w.Code != http.StatusBadGateway || len(got.Created) != 1 || got.Created[0].Issue != 101 || !strings.Contains(got.Error, "GitHub is down") {
		t.Errorf("a failed approval: %d %+v", w.Code, got)
	}
	fail = false
	w = do(t, s, http.MethodPost, base+"/plans/p1/approve", "")
	got = decode[Approval](t, w)
	if w.Code != http.StatusOK || len(got.Existing) != 1 || len(got.Created) != 1 || got.Created[0].Issue != 102 {
		t.Errorf("approving again: %d %+v", w.Code, got)
	}
	detail := decode[PlanDetail](t, do(t, s, http.MethodGet, base+"/plans/p1", ""))
	if len(detail.Tickets) != 2 || detail.Tickets[0].Issue != 101 || detail.Tickets[1].Issue != 102 {
		t.Errorf("tickets = %+v", detail.Tickets)
	}
	if w := do(t, s, http.MethodPut, base+"/plans/p1/tickets/t1", `{"title": "[chore] x", "summary": "x", "scope": ["a"], "acceptance": ["b"], "testing": []}`); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "#101") {
		t.Errorf("change a ticket that is an issue: %d %s", w.Code, w.Body)
	}
}

// The stream tells the page when a meeting moves, and not for every prompt
// and answer it keeps.
func TestMeetings_TheStreamFollowsAMeeting(t *testing.T) {
	root := t.TempDir()
	var meeting area
	for _, a := range areas {
		if a.name == "meeting" {
			meeting = a
		}
	}
	before := fingerprint(root, meeting)
	write(t, root, ".ai/state/council/p1/calls/001-x.md", "a prompt")
	write(t, root, ".ai/state/council/p1/answers.json", "{}")
	if fingerprint(root, meeting) != before {
		t.Error("a prompt the meeting keeps was told as a change")
	}
	setLive(t, root, "p1", council.LiveMeeting)
	if fingerprint(root, meeting) == before {
		t.Error("where the meeting is was not told as a change")
	}
}

// The page shows who sits at the table; a project without a roster gets one
// at its first plan.
func TestMeetings_Roster(t *testing.T) {
	s, root, base, _, _ := meetingProject(t, "")
	roster := decode[RosterView](t, do(t, s, http.MethodGet, base+"/roster", ""))
	if roster.Missing || len(roster.Roles) != 3 || roster.Roles[0].ID != "owner" || !roster.Roles[0].Decides || !roster.Roles[1].Plans ||
		roster.Roles[2].Stance != "You look for how it fails." || roster.Calls != 60 || roster.Roles[0].Owns[0] != "docs/product/requirements.md" {
		t.Errorf("roster = %+v", roster)
	}
	if err := os.Remove(filepath.Join(root, ".ai", "roster.yaml")); err != nil {
		t.Fatal(err)
	}
	if missing := decode[RosterView](t, do(t, s, http.MethodGet, base+"/roster", "")); !missing.Missing || missing.Roles == nil {
		t.Errorf("no roster = %+v", missing)
	}
	write(t, root, ".ai/roster.yaml", "roles: [")
	if w := do(t, s, http.MethodGet, base+"/roster", ""); w.Code != http.StatusConflict {
		t.Errorf("a roster that cannot be read: %d", w.Code)
	}
}
