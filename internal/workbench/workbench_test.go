package workbench

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/status"
)

const testConfig = `version: "1.0"
project:
  name: rally
  type: single-repo
repos:
  - name: root
    path: ./
    type: root
    language: go
git:
  integration_branch: develop
  release_branch: main
`

// newProject makes an AWK project in a temporary folder.
func newProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, ".ai/config.yaml", testConfig)
	return root
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newServer is a workbench on a port of its own, with settings in a
// temporary file and GitHub played by a function.
func newServer(t *testing.T, issues status.IssueLister) *Server {
	t.Helper()
	s, err := New(Options{
		Store:    NewStore(filepath.Join(t.TempDir(), "workbench.json")),
		Version:  "v9.9.9",
		IssuesIn: func(string) status.IssueLister { return issues },
		Poll:     10 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	s.port = 4747
	return s
}

// do sends a request as the workbench's own page does.
func do(t *testing.T, s *Server, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	r.Host = "127.0.0.1:4747"
	r.AddCookie(&http.Cookie{Name: cookieName, Value: s.Token()})
	if method != http.MethodGet {
		r.Header.Set(mutationHeader, "1")
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("the answer is not JSON (%d): %s", w.Code, w.Body)
	}
	return v
}

func TestStore(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "awkit", "workbench.json"))
	settings, err := store.Load()
	if err != nil || settings.Onboarding.Status != "new" || len(settings.Projects) != 0 {
		t.Fatalf("a person who never opened the workbench has %+v, %v; want a new onboarding and no projects", settings, err)
	}
	if _, err := store.Update(func(s *Settings) error {
		s.Onboarding = Onboarding{Status: "paused", Step: 3, Done: []string{"health"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	settings, err = store.Load()
	if err != nil || settings.Onboarding.Status != "paused" || settings.Onboarding.Step != 3 || settings.Onboarding.Done[0] != "health" {
		t.Errorf("kept %+v, %v", settings.Onboarding, err)
	}

	// Onboarding a later version wrote, or a hand: the teaching starts over.
	if err := os.WriteFile(store.Path(), []byte(`{"onboarding": {"status": "lost", "step": 2}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if settings, err := store.Load(); err != nil || settings.Onboarding.Status != "new" {
		t.Errorf("onboarding with an unknown status = %+v, %v; want a new one", settings.Onboarding, err)
	}
	// A file that is not JSON is said, not taken for empty.
	if err := os.WriteFile(store.Path(), []byte(`{`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil {
		t.Error("a broken settings file was read without a word")
	}
}

func TestDefaultSettingsPath(t *testing.T) {
	t.Setenv(SettingsEnv, "")
	path, err := DefaultSettingsPath()
	if err != nil {
		t.Skipf("no settings folder here: %v", err)
	}
	if filepath.Base(path) != "workbench.json" || filepath.Base(filepath.Dir(path)) != "awkit" {
		t.Errorf("settings at %s; want <settings folder>/awkit/workbench.json", path)
	}
	t.Setenv(SettingsEnv, "elsewhere.json")
	if path, _ := DefaultSettingsPath(); path != "elsewhere.json" {
		t.Errorf("with %s set, settings at %s", SettingsEnv, path)
	}
}

func TestOnboarding_Validate(t *testing.T) {
	for _, ok := range []Onboarding{{Status: "new"}, {Status: "touring", Step: 8}, {Status: "skipped", Done: []string{"tour", "plan"}}} {
		if err := ok.Validate(); err != nil {
			t.Errorf("%+v was refused: %v", ok, err)
		}
	}
	for _, bad := range []Onboarding{{Status: ""}, {Status: "lost"}, {Status: "touring", Step: -1}, {Status: "touring", Step: 51},
		{Status: "done", Done: []string{"Tour"}}, {Status: "done", Done: []string{"<script>"}}, {Status: "done", Done: make([]string, 33)}} {
		if err := bad.Validate(); err == nil {
			t.Errorf("%+v was taken", bad)
		}
	}
}

func TestProjects(t *testing.T) {
	root := newProject(t)
	if got, err := ResolveProject(root); err != nil || got != root {
		t.Errorf("ResolveProject(%s) = %s, %v", root, got, err)
	}
	if _, err := ResolveProject(t.TempDir()); err == nil || !strings.Contains(err.Error(), "is not an AWK project") {
		t.Errorf("a folder without a configuration: %v", err)
	}
	if _, err := ResolveProject(filepath.Join(root, ".ai", "config.yaml")); err == nil || !strings.Contains(err.Error(), "is a file") {
		t.Errorf("a file: %v", err)
	}
	if _, err := ResolveProject(" "); err == nil {
		t.Error("no folder was taken")
	}

	var settings Settings
	addProject(&settings, root, time.Now())
	addProject(&settings, root, time.Now())
	if len(settings.Projects) != 1 {
		t.Errorf("a project added twice is kept %d times", len(settings.Projects))
	}
	if runtime.GOOS == "windows" {
		addProject(&settings, strings.ToUpper(root), time.Now())
		if len(settings.Projects) != 1 {
			t.Error("on Windows, a folder named in other letters is another project")
		}
	}
	if got, ok := findProject(settings, projectID(root)); !ok || got != root {
		t.Errorf("findProject = %s, %v", got, ok)
	}
	if v := view(settings.Projects[0]); v.Name != "rally" || v.Problem != "" || v.ID != projectID(root) {
		t.Errorf("view = %+v; want the configuration's name", v)
	}
	if !removeProject(&settings, projectID(root)) || len(settings.Projects) != 0 || removeProject(&settings, projectID(root)) {
		t.Error("a project is not removed once")
	}
	if v := view(Project{Path: filepath.Join(t.TempDir(), "gone")}); v.Problem == "" {
		t.Error("a project whose folder is gone shows no problem")
	}
}

func TestGuard(t *testing.T) {
	s := newServer(t, nil)
	send := func(host, target string, cookie bool, header bool, method string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, target, strings.NewReader(`{}`))
		r.Host = host
		if cookie {
			r.AddCookie(&http.Cookie{Name: cookieName, Value: s.Token()})
		}
		if header {
			r.Header.Set(mutationHeader, "1")
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w
	}
	for _, host := range []string{"evil.example:4747", "127.0.0.1:4748", "127.0.0.1", "192.168.1.5:4747"} {
		if w := send(host, "/api/session", true, false, http.MethodGet); w.Code != http.StatusForbidden {
			t.Errorf("host %s: %d, want 403", host, w.Code)
		}
	}
	for _, host := range []string{"127.0.0.1:4747", "localhost:4747"} {
		if w := send(host, "/api/session", true, false, http.MethodGet); w.Code != http.StatusOK {
			t.Errorf("host %s: %d, want 200", host, w.Code)
		}
	}
	if w := send("127.0.0.1:4747", "/api/session", false, false, http.MethodGet); w.Code != http.StatusUnauthorized {
		t.Errorf("no cookie: %d, want 401", w.Code)
	}
	r := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	r.Host = "127.0.0.1:4747"
	r.AddCookie(&http.Cookie{Name: cookieName, Value: "an-earlier-token"})
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("an earlier token: %d, want 401", w.Code)
	}
	// A change without the workbench's header is refused: another site's
	// page cannot add it.
	if w := send("127.0.0.1:4747", "/api/onboarding", true, false, http.MethodPut); w.Code != http.StatusForbidden {
		t.Errorf("a change without the header: %d, want 403", w.Code)
	}

	// The address `awkit ui` prints: the token becomes a cookie and leaves
	// the address.
	w = send("127.0.0.1:4747", "/?token="+s.Token(), false, false, http.MethodGet)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/" {
		t.Errorf("open with the token: %d to %q, want 303 to /", w.Code, w.Header().Get("Location"))
	}
	if cookie := w.Result().Cookies(); len(cookie) != 1 || cookie[0].Value != s.Token() || !cookie[0].HttpOnly || cookie[0].SameSite != http.SameSiteStrictMode {
		t.Errorf("cookies = %+v", cookie)
	}
	if w := send("127.0.0.1:4747", "/?token=wrong", false, false, http.MethodGet); w.Code != http.StatusForbidden {
		t.Errorf("a wrong token: %d, want 403", w.Code)
	}
	if s.URL() != "http://127.0.0.1:4747/?token="+s.Token() {
		t.Errorf("URL = %s", s.URL())
	}
}

func TestAssets(t *testing.T) {
	s := newServer(t, nil)
	if w := do(t, s, http.MethodGet, "/", ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "沒有包含工作台的畫面") {
		t.Errorf("an awkit built without the page: %d %s", w.Code, w.Body)
	}
	s.opts.Assets = fstest.MapFS{
		"index.html":        {Data: []byte("<!doctype html><title>AWK</title>")},
		"assets/app-1a2.js": {Data: []byte("console.log(1)")},
	}
	if w := do(t, s, http.MethodGet, "/", ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "<title>AWK</title>") || w.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("/: %d %q %s", w.Code, w.Header().Get("Cache-Control"), w.Body)
	}
	if w := do(t, s, http.MethodGet, "/assets/app-1a2.js", ""); w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
		t.Errorf("an asset: %d %q", w.Code, w.Header().Get("Cache-Control"))
	}
	// A path the page routes itself gets the page.
	if w := do(t, s, http.MethodGet, "/projects/abc", ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "<title>AWK</title>") {
		t.Errorf("a route of the page: %d %s", w.Code, w.Body)
	}
	if w := do(t, s, http.MethodGet, "/api/nothing", ""); w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "has no GET /api/nothing") {
		t.Errorf("an API the workbench does not have: %d %s", w.Code, w.Body)
	}
}

func TestSessionProjectsAndOnboarding(t *testing.T) {
	s := newServer(t, nil)
	root := newProject(t)
	session := decode[Session](t, do(t, s, http.MethodGet, "/api/session", ""))
	if session.Version != "v9.9.9" || len(session.Projects) != 0 || session.Onboarding.Status != "new" || session.Onboarding.Done == nil {
		t.Errorf("session = %+v", session)
	}

	if w := do(t, s, http.MethodPost, "/api/projects", `{"path": "`+filepath.ToSlash(root)+`"}`); w.Code != http.StatusOK || decode[ProjectView](t, w).Name != "rally" {
		t.Errorf("add a project: %d %s", w.Code, w.Body)
	}
	if w := do(t, s, http.MethodPost, "/api/projects", `{"path": "`+filepath.ToSlash(t.TempDir())+`"}`); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "awkit init") {
		t.Errorf("add a folder that is no project: %d %s", w.Code, w.Body)
	}
	if w := do(t, s, http.MethodPost, "/api/projects", `{"folder": "x"}`); w.Code != http.StatusBadRequest {
		t.Errorf("a body of another shape: %d", w.Code)
	}
	session = decode[Session](t, do(t, s, http.MethodGet, "/api/session", ""))
	if len(session.Projects) != 1 || session.Projects[0].ID != projectID(root) {
		t.Fatalf("projects = %+v", session.Projects)
	}

	if w := do(t, s, http.MethodPut, "/api/onboarding", `{"status": "paused", "step": 4, "done": ["tour"], "list_hidden": false}`); w.Code != http.StatusOK {
		t.Errorf("keep onboarding: %d %s", w.Code, w.Body)
	}
	if w := do(t, s, http.MethodPut, "/api/onboarding", `{"status": "flying", "step": 4, "done": []}`); w.Code != http.StatusBadRequest {
		t.Errorf("onboarding that is none: %d", w.Code)
	}
	if got := decode[Session](t, do(t, s, http.MethodGet, "/api/session", "")).Onboarding; got.Status != "paused" || got.Step != 4 {
		t.Errorf("onboarding kept as %+v", got)
	}

	if w := do(t, s, http.MethodDelete, "/api/projects/"+projectID(root), ""); w.Code != http.StatusNoContent {
		t.Errorf("remove: %d", w.Code)
	}
	if w := do(t, s, http.MethodDelete, "/api/projects/"+projectID(root), ""); w.Code != http.StatusNotFound {
		t.Errorf("remove twice: %d", w.Code)
	}
	if _, err := os.Stat(filepath.Join(root, ".ai", "config.yaml")); err != nil {
		t.Errorf("removing a project from the workbench touched its folder: %v", err)
	}
}

// added is a workbench with one project in it.
func added(t *testing.T, issues status.IssueLister) (*Server, string, string) {
	t.Helper()
	s := newServer(t, issues)
	root := newProject(t)
	if _, err := s.AddProject(root); err != nil {
		t.Fatal(err)
	}
	return s, root, "/api/projects/" + projectID(root)
}

func TestProjectAPI(t *testing.T) {
	asked := 0
	issues := func(ctx context.Context, label, state, fields string) ([]byte, error) {
		asked++
		if label == "ai-task" && state == "open" {
			return []byte(`[{"number": 306, "title": "[feat] show the room code", "labels": [{"name": "ai-task"}, {"name": "needs-human-review"}], "body": ""},
				{"number": 303, "title": "[feat] start the match", "labels": [{"name": "ai-task"}, {"name": "in-progress"}], "body": ""}]`), nil
		}
		return []byte(`[]`), nil
	}
	s, root, base := added(t, issues)

	if w := do(t, s, http.MethodGet, "/api/projects/nothing/status", ""); w.Code != http.StatusNotFound {
		t.Errorf("a project the workbench does not have: %d", w.Code)
	}
	if report := decode[status.Report](t, do(t, s, http.MethodGet, base+"/status", "")); report.Root != root || report.Run.State == "" {
		t.Errorf("status = %+v", report)
	}

	tickets := decode[TicketsView](t, do(t, s, http.MethodGet, base+"/tickets", ""))
	if tickets.Error != "" || tickets.Tickets == nil || len(tickets.Tickets.Open) != 2 || len(tickets.Stages) == 0 {
		t.Fatalf("tickets = %+v", tickets)
	}
	stages := map[int]string{}
	for _, ticket := range tickets.Tickets.Open {
		stages[ticket.Number] = ticket.Stage
	}
	if stages[306] != status.StageNeedsYou || stages[303] != status.StageInProgress {
		t.Errorf("stages = %v", stages)
	}
	// GitHub is asked once for a while, and again when the page wants it fresh.
	before := asked
	do(t, s, http.MethodGet, base+"/tickets", "")
	if asked != before {
		t.Errorf("GitHub was asked again within %s", ticketsFresh)
	}
	do(t, s, http.MethodGet, base+"/tickets?fresh=1", "")
	if asked == before {
		t.Error("a fresh look did not ask GitHub")
	}

	// GitHub that cannot be asked is said, not taken for no tickets.
	failing, _, failingBase := added(t, func(context.Context, string, string, string) ([]byte, error) {
		return nil, os.ErrPermission
	})
	if got := decode[TicketsView](t, do(t, failing, http.MethodGet, failingBase+"/tickets", "")); got.Error == "" || got.Tickets != nil {
		t.Errorf("tickets when GitHub cannot be asked = %+v", got)
	}

	// No run yet: no events, and no error.
	if events := decode[EventsView](t, do(t, s, http.MethodGet, base+"/events", "")); events.Session != "" || len(events.Events) != 0 {
		t.Errorf("events before any run = %+v", events)
	}
	write(t, root, ".ai/state/principal/session.json", `{"session_id": "principal-20261118-090000-ab12"}`)
	write(t, root, ".ai/state/events/principal-20261118-090000-ab12.jsonl",
		`{"seq": 1, "ts": "2026-11-18T09:00:01Z", "component": "principal", "type": "loop_start", "level": "info"}`+"\n"+
			`{"seq": 2, "ts": "2026-11-18T09:00:02Z", "component": "worker", "type": "dispatch", "level": "info", "issue_id": 303}`+"\n")
	events := decode[EventsView](t, do(t, s, http.MethodGet, base+"/events", ""))
	if events.Session != "principal-20261118-090000-ab12" || len(events.Events) != 2 || events.Events[1].IssueID != 303 {
		t.Errorf("events = %+v", events)
	}
	if last := decode[EventsView](t, do(t, s, http.MethodGet, base+"/events?last=1", "")); len(last.Events) != 1 || last.Events[0].Seq != 2 {
		t.Errorf("the last event = %+v", last.Events)
	}

	write(t, root, "docs/plans/20261117-plan/brief.md", "A tennis game, played online.\n")
	write(t, root, "docs/plans/20261117-plan/tickets.yaml", `plan: 20261117-plan
milestones:
  - key: m1
    title: Two players meet in a room
    goal: two computers in one match
    done_when: [both see the match start]
tickets:
  - key: t1
    title: "[feat] accept a connection"
    repo: root
    issue: 301
  - key: t2
    title: "[feat] join by code"
    repo: root
    depends_on: [t1]
`)
	plans := decode[struct{ Plans []PlanView }](t, do(t, s, http.MethodGet, base+"/plans", "")).Plans
	if len(plans) != 1 || plans[0].Brief != "A tennis game, played online." || len(plans[0].Milestones) != 1 || len(plans[0].Tickets) != 2 || plans[0].Approved != 1 || plans[0].Tickets[1].DependsOn[0] != "t1" {
		t.Errorf("plans = %+v", plans)
	}

	// A project whose configuration went away is said to be so.
	if err := os.Remove(filepath.Join(root, ".ai", "config.yaml")); err != nil {
		t.Fatal(err)
	}
	if w := do(t, s, http.MethodGet, base+"/status", ""); w.Code != http.StatusConflict {
		t.Errorf("a project without its configuration: %d", w.Code)
	}
}

func TestStream(t *testing.T) {
	s, root, base := added(t, nil)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	s.port = srv.Listener.Addr().(*net.TCPAddr).Port

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+base+"/stream", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: s.Token()})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("stream: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	lines := bufio.NewScanner(resp.Body)
	next := func() string {
		for lines.Scan() {
			if line := lines.Text(); strings.HasPrefix(line, "data: ") {
				return line
			}
		}
		t.Fatalf("the stream ended: %v", lines.Err())
		return ""
	}

	write(t, root, ".ai/state/loop_count", "3")
	if got := next(); got != `data: {"areas":["status"]}` {
		t.Errorf("a run's state changed: %s", got)
	}
	write(t, root, ".ai/state/events/s.jsonl", "{}\n")
	if got := next(); got != `data: {"areas":["events"]}` {
		t.Errorf("an event was written: %s", got)
	}
	// A log grows all the time a run goes on: it is not a change of the run.
	write(t, root, ".ai/state/logs/principal.log", "line\n")
	write(t, root, "docs/plans/p/brief.md", "x")
	if got := next(); got != `data: {"areas":["plans"]}` {
		t.Errorf("a plan was written, after a log: %s", got)
	}
}
