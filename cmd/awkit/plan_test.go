package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/council"
)

func TestSplitFlags(t *testing.T) {
	takesValue := map[string]bool{"--resume": true, "--brief-file": true}
	tests := []struct {
		args      []string
		wantFlags []string
		wantWords []string
	}{
		{[]string{"a brief", "--resume", "x"}, []string{"--resume", "x"}, []string{"a brief"}},
		{[]string{"--brief-file", "b.md", "--help"}, []string{"--brief-file", "b.md", "--help"}, nil},
		{[]string{"two", "words", "--dry-run", "more"}, []string{"--dry-run"}, []string{"two", "words", "more"}},
		{[]string{"--", "--not-a-flag", "x"}, nil, []string{"--not-a-flag", "x"}},
		{[]string{"--resume"}, []string{"--resume"}, nil},
		{nil, nil, nil},
	}
	for _, tt := range tests {
		flags, words := splitFlags(tt.args, takesValue)
		if !reflect.DeepEqual(flags, tt.wantFlags) || !reflect.DeepEqual(words, tt.wantWords) {
			t.Errorf("splitFlags(%q) = %q, %q; want %q, %q", tt.args, flags, words, tt.wantFlags, tt.wantWords)
		}
	}
}

// What a plan cannot start from is refused before anything is asked.
func TestCmdPlan_Arguments(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name string
		args []string
		want int
	}{
		{"help", []string{"--help"}, 0},
		{"no brief", []string{"--state-root", dir}, 2},
		{"a brief and a plan to go on with", []string{"more words", "--resume", "20261114-x", "--state-root", dir}, 2},
		{"a brief twice", []string{"words", "--brief-file", "brief.md", "--state-root", dir}, 2},
		{"a brief file that is not there", []string{"--brief-file", filepath.Join(dir, "absent.md"), "--state-root", dir}, 1},
		{"a project without a config", []string{"a brief", "--state-root", dir}, 1},
		{"approve: no plan named", []string{"approve", "--state-root", dir}, 2},
		{"approve: two plans named", []string{"approve", "a", "b", "--state-root", dir}, 2},
		{"approve: help", []string{"approve", "--help"}, 0},
		{"an option that does not exist", []string{"a brief", "--fast"}, 2},
	}
	for _, tt := range tests {
		if got := cmdPlan(tt.args); got != tt.want {
			t.Errorf("%s: cmdPlan(%q) = %d, want %d", tt.name, tt.args, got, tt.want)
		}
	}
}

func writePlanConfig(t *testing.T, dir, repos string) {
	t.Helper()
	path := filepath.Join(dir, ".ai", "config", "workflow.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("git:\n  integration_branch: develop\n"+repos), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A ticket can be for a repo of the config or for the repository root.
func TestPlanRepos(t *testing.T) {
	dir := t.TempDir()
	writePlanConfig(t, dir, "repos:\n  - {name: backend, path: backend/, language: go}\n  - {name: frontend, path: frontend/, language: godot}\n")
	got, err := planRepos(dir)
	want := []council.Repo{{Name: "root", Path: "./"}, {Name: "backend", Language: "go", Path: "backend/"}, {Name: "frontend", Language: "godot", Path: "frontend/"}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("planRepos = %+v, %v; want %+v", got, err, want)
	}

	// A config that lists the root itself: once, with what the config says.
	dir = t.TempDir()
	writePlanConfig(t, dir, "repos:\n  - {name: root, path: ./, language: go}\n")
	got, err = planRepos(dir)
	if want := []council.Repo{{Name: "root", Language: "go", Path: "./"}}; err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("planRepos = %+v, %v; want %+v", got, err, want)
	}

	// Written with a capital it is still the root: once, under the name
	// tickets use, with what the config says.
	dir = t.TempDir()
	writePlanConfig(t, dir, "repos:\n  - {name: Root, path: ./, language: go}\n  - {name: Backend, path: backend/, language: go}\n")
	got, err = planRepos(dir)
	if want := []council.Repo{{Name: "root", Language: "go", Path: "./"}, {Name: "Backend", Language: "go", Path: "backend/"}}; err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("planRepos = %+v, %v; want %+v", got, err, want)
	}

	if _, err := planRepos(t.TempDir()); err == nil || !strings.Contains(err.Error(), "awkit init") {
		t.Errorf("without a config: err = %v, want it to say what to do", err)
	}
}

func TestRosterPresetFor(t *testing.T) {
	tests := []struct {
		languages []string
		want      string
	}{
		{[]string{"go", "godot"}, "godot-go"},
		{[]string{"go", "csharp"}, "godot-go"},
		{[]string{"go"}, "generic"},
		{[]string{"godot"}, "generic"},
		{[]string{"", "python"}, "generic"},
		{nil, "generic"},
	}
	for _, tt := range tests {
		var repos []council.Repo
		for i, language := range tt.languages {
			repos = append(repos, council.Repo{Name: fmt.Sprintf("r%d", i), Language: language})
		}
		if got := rosterPresetFor(repos); got != tt.want {
			t.Errorf("rosterPresetFor(%v) = %q, want %q", tt.languages, got, tt.want)
		}
	}
}

// A project without a roster gets the one that fits its repos, once. A roster
// that is there is the project's own.
func TestPlanRoster(t *testing.T) {
	dir := t.TempDir()
	game := []council.Repo{{Name: "backend", Language: "go"}, {Name: "frontend", Language: "godot"}}

	roster, err := planRoster(dir, "", game)
	if err != nil {
		t.Fatalf("planRoster: %v", err)
	}
	if roster.Role("numerical-designer") == nil {
		t.Errorf("a Go and Godot project did not get the game studio's roster: %v", roster.IDs())
	}
	file := filepath.Join(dir, ".ai", "roster.yaml")
	written, err := os.ReadFile(file)
	if err != nil || string(written) != string(council.DefaultRoster("godot-go")) {
		t.Errorf("the roster was not written to %s: %v", file, err)
	}

	// Changed by the project: kept, and used.
	own := "roles:\n  - {id: lead, decides: true, plans: true, stance: leads}\n  - {id: critic, stance: criticises}\n"
	if err := os.WriteFile(file, []byte(own), 0o644); err != nil {
		t.Fatal(err)
	}
	roster, err = planRoster(dir, "", game)
	if err != nil || !reflect.DeepEqual(roster.IDs(), []string{"lead", "critic"}) {
		t.Errorf("the project's own roster was not used: %v, %v", roster, err)
	}
	if after, _ := os.ReadFile(file); string(after) != own {
		t.Error("the project's own roster was rewritten")
	}

	// A roster that cannot hold a meeting is an error that names the file.
	if err := os.WriteFile(file, []byte("roles:\n  - {id: lead, decides: true, plans: true, stance: leads}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := planRoster(dir, "", game); err == nil || !strings.Contains(err.Error(), "roster.yaml") || !strings.Contains(err.Error(), "at least two") {
		t.Errorf("err = %v", err)
	}
	// A roster named on the command line is not written for the caller.
	absent := filepath.Join(t.TempDir(), "mine.yaml")
	if _, err := planRoster(dir, absent, game); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("err = %v", err)
	}
	if fileExists(absent) {
		t.Error("a roster named on the command line was written")
	}
}

// scriptedCouncil answers every question of a plan in the simplest way that
// is accepted, for whatever roster it is asked as.
type scriptedCouncil struct {
	mu    sync.Mutex
	calls int
	// curious makes two roles ask the owner something, and the deciding role
	// assume an answer to one of the questions.
	curious bool
	// toldOwner counts the questions in which a role was told what the owner
	// answered.
	toldOwner int
	// leaves makes every role leave the documents that are written as they are.
	leaves bool
}

var councilRole = regexp.MustCompile(`^You are the .+? \(([a-z0-9-]+)\) at a planning meeting`)
var councilOwned = regexp.MustCompile(`(?m)^- (docs/\S+\.md)$`)

func (m *scriptedCouncil) Ask(ctx context.Context, model, prompt string) (string, error) {
	m.mu.Lock()
	m.calls++
	if strings.Contains(prompt, "  The owner: A readiness probe.\n") {
		m.toldOwner++
	}
	m.mu.Unlock()
	role := "moderator"
	if match := councilRole.FindStringSubmatch(prompt); match != nil {
		role = match[1]
	}
	block := func(json string) (string, error) { return "```json\n" + json + "\n```\n", nil }
	switch {
	case strings.Contains(prompt, "state your position on the brief"):
		question := ""
		if m.curious {
			question = map[string]string{"product-owner": `"Who calls the endpoint?"`, "skeptic": `"May it be reached from outside?"`}[role]
		}
		return block(`{"summary": "` + role + ` agrees.", "proposals": ["build it"], "concerns": [], "questions": [` + question + `]}`)
	case strings.Contains(prompt, "Your task now: object."), strings.Contains(prompt, "Your task now: object to the work list"):
		return block(`{"objections": []}`)
	case strings.Contains(prompt, "Say where the meeting stands"):
		return block(`{"consensus": ["build it"], "open": [], "questions": []}`)
	case strings.Contains(prompt, "Your task now: decide"):
		assumptions := ""
		if m.curious {
			assumptions = `{"question": "May it be reached   from outside?", "assumption": "No: it is internal only.", "if_wrong": "It needs a token."}, {"question": "", "assumption": "The database is PostgreSQL.", "if_wrong": ""}`
		}
		return block(`{"decisions": [{"title": "scope", "decision": "Build the endpoint.", "rationale": "It is small.", "dissent": ""}], "assumptions": [` + assumptions + `]}`)
	case strings.Contains(prompt, "write the documents you own"):
		// The documents asked for are listed between the task and the rules.
		task := prompt[strings.Index(prompt, "write the documents you own"):]
		var b strings.Builder
		for _, match := range councilOwned.FindAllStringSubmatch(task, -1) {
			if m.leaves {
				fmt.Fprintf(&b, "<<<UNCHANGED %s>>>\n", match[1])
				continue
			}
			fmt.Fprintf(&b, "<<<DOCUMENT %s>>>\n---\nstatus: draft\n---\n# %s\n<<<END>>>\n", match[1], match[1])
		}
		return b.String(), nil
	case strings.Contains(prompt, "Your task now: turn the plan into tickets"):
		return block(`{"tickets": [
			{"key": "t1", "title": "[feat] serve /healthz", "repo": "backend", "summary": "The endpoint.", "scope": ["handler"], "acceptance": ["answers 200"], "testing": ["a test"]},
			{"key": "t2", "title": "[docs] describe /healthz", "repo": "root", "summary": "The docs.", "scope": ["README"], "acceptance": ["the README names the endpoint"], "testing": [], "depends_on": ["t1"]}
		], "later": []}`)
	}
	return "", fmt.Errorf("the test does not know this prompt: %.200s", prompt)
}

// From a brief to issues: the plan writes its files and creates nothing; the
// approval creates the issues once.
func TestCmdPlan_FromABriefToIssues(t *testing.T) {
	dir := t.TempDir()
	writePlanConfig(t, dir, "repos:\n  - {name: backend, path: backend/, language: go}\n")
	model := &scriptedCouncil{}
	origModel, origCreate := planModel, planCreate
	t.Cleanup(func() { planModel, planCreate = origModel, origCreate })
	planModel = model
	var created []string
	planCreate = func(ctx context.Context, stateRoot, title, bodyFile string) (int, error) {
		body, err := os.ReadFile(bodyFile)
		if err != nil {
			return 0, err
		}
		if stateRoot != dir {
			return 0, fmt.Errorf("the issue is created for %s, want the project %s", stateRoot, dir)
		}
		created = append(created, title+"\n"+string(body))
		return 100 + len(created), nil
	}

	if code := cmdPlan([]string{"Add a health endpoint", "to the API server.", "--state-root", dir}); code != 0 {
		t.Fatalf("cmdPlan = %d", code)
	}

	plans, _ := filepath.Glob(filepath.Join(dir, "docs", "plans", "*"))
	if len(plans) != 1 {
		t.Fatalf("%d plans under docs/plans, want one", len(plans))
	}
	id := filepath.Base(plans[0])
	if !strings.HasSuffix(id, "-add-a-health-endpoint-to") {
		t.Errorf("plan id = %q", id)
	}
	if brief, _ := os.ReadFile(filepath.Join(plans[0], "brief.md")); string(brief) != "Add a health endpoint to the API server.\n" {
		t.Errorf("brief = %q", brief)
	}
	for _, rel := range []string{"minutes.md", "tickets.yaml"} {
		if !fileExists(filepath.Join(plans[0], rel)) {
			t.Errorf("the plan has no %s", rel)
		}
	}
	// The project had no roster: it got the generic one, and its documents.
	if !fileExists(filepath.Join(dir, ".ai", "roster.yaml")) {
		t.Error("no roster was written")
	}
	for _, doc := range []string{"docs/product/requirements.md", "docs/tech/architecture.md", "docs/qa/risks.md", "docs/decisions.md", "docs/README.md"} {
		if !fileExists(filepath.Join(dir, filepath.FromSlash(doc))) {
			t.Errorf("%s was not written", doc)
		}
	}
	if len(created) != 0 {
		t.Fatalf("the plan created %d issues; it only proposes", len(created))
	}

	// A dry run of the approval creates nothing either.
	if code := cmdPlan([]string{"approve", id, "--dry-run", "--state-root", dir}); code != 0 || len(created) != 0 {
		t.Fatalf("approve --dry-run = %d, %d issues created", code, len(created))
	}

	if code := cmdPlan([]string{"approve", id, "--state-root", dir}); code != 0 {
		t.Fatalf("approve = %d", code)
	}
	if len(created) != 2 || !strings.HasPrefix(created[0], "[feat] serve /healthz\n") || !strings.HasPrefix(created[1], "[docs] describe /healthz\n") {
		t.Fatalf("created %d issues: %q", len(created), created)
	}
	if !strings.Contains(created[1], "repo: root\ndepends_on: [101]\n") {
		t.Errorf("the second issue does not wait for the first:\n%s", created[1])
	}
	list, _ := os.ReadFile(filepath.Join(plans[0], "tickets.yaml"))
	if !strings.Contains(string(list), "issue: 101") || !strings.Contains(string(list), "issue: 102") {
		t.Errorf("the issues are not on record in the work list:\n%s", list)
	}

	// Approving again creates nothing; going on with the finished plan asks nothing.
	if code := cmdPlan([]string{"approve", id, "--state-root", dir}); code != 0 || len(created) != 2 {
		t.Errorf("a second approval = %d, %d issues in all", code, len(created))
	}
	before := model.calls
	if code := cmdPlan([]string{"--resume", id, "--state-root", dir}); code != 0 || model.calls != before {
		t.Errorf("going on with a finished plan = %d, %d more calls", code, model.calls-before)
	}

	// A plan that is not there.
	if code := cmdPlan([]string{"approve", "20260101-nothing", "--state-root", dir}); code != 1 {
		t.Errorf("approve of a plan that is not there = %d, want 1", code)
	}
	if code := cmdPlan([]string{"--resume", "20260101-nothing", "--state-root", dir}); code != 1 {
		t.Errorf("going on with a plan that is not there = %d, want 1", code)
	}
}

// `awkit validate` checks the roster too: one that cannot hold a meeting is
// an error, one that can is named, and a project without one is fine.
func TestCmdValidate_Roster(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, ".ai", "config", "workflow.yaml")
	if err := os.MkdirAll(filepath.Join(dir, ".ai", "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "backend"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte("version: \"1.2\"\nproject:\n  name: demo\n  type: monorepo\nrepos:\n  - {name: backend, path: backend/, type: directory, language: go, verify: {build: \"go build ./...\", test: \"go test ./...\"}}\ngit:\n  integration_branch: develop\n  release_branch: main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	validate := func() (int, string) {
		code := -1
		out := captureStdout(t, func() { code = cmdValidate([]string{"--config", config}) })
		return code, out
	}

	// No roster: nothing to say about one.
	if code, out := validate(); code != 0 || strings.Contains(out, "Roster:") {
		t.Fatalf("without a roster: validate = %d\n%s", code, out)
	}

	roster := filepath.Join(dir, ".ai", "roster.yaml")
	if err := os.WriteFile(roster, council.DefaultRoster("godot-go"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, out := validate(); code != 0 || !strings.Contains(out, "Roster: 5 roles (producer decides, tech-lead plans)") {
		t.Errorf("with the game studio's roster: validate = %d\n%s", code, out)
	}

	if err := os.WriteFile(roster, []byte("roles:\n  - {id: lead, decides: true, stance: leads}\n  - {id: critic, stance: criticises}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, out := validate(); code != 1 || !strings.Contains(out, ".ai/roster.yaml: 0 roles plan") || strings.Contains(out, "Configuration is valid") {
		t.Errorf("with a roster nobody plans in: validate = %d\n%s", code, out)
	}
}

// captureStdout runs fn and returns what it printed. The pipe is read while
// fn runs: a plan prints more than a pipe holds.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		done <- buf.String()
	}()
	defer func() { os.Stdout = old }()
	fn()
	_ = w.Close()
	return <-done
}

// With --ask the council's questions are put to the person at the terminal
// before the table argues; without it nobody is asked. Either way the end of
// the plan shows what is still assumed.
func TestCmdPlan_AskThePersonAtTheTerminal(t *testing.T) {
	origModel, origInput := planModel, planInput
	t.Cleanup(func() { planModel, planInput = origModel, origInput })

	dir := t.TempDir()
	writePlanConfig(t, dir, "repos:\n  - {name: backend, path: backend/, language: go}\n")
	model := &scriptedCouncil{curious: true}
	planModel = model
	// The first question is answered, the second is left to the council.
	planInput = strings.NewReader("A readiness probe.\n\n")

	out := captureStdout(t, func() {
		if code := cmdPlan([]string{"--ask", "Add a health endpoint.", "--state-root", dir}); code != 0 {
			t.Errorf("cmdPlan --ask = %d", code)
		}
	})

	for _, want := range []string{
		"Questions: 2 for the owner of the project\n",
		"The council has 2 question(s) only you can answer.\nAnswer in a line, or press Enter to let the council assume.\n",
		"\n1/2  Who calls the endpoint?\n> ",
		"\n2/2  May it be reached from outside?\n> ",
		"  1 answered, 1 left to the council to assume\n",
		"1 decisions, 5 documents written, 2 tickets proposed",
		"The plan assumes 2 thing(s) only you can answer (docs/decisions.md):\n" +
			"  A1   May it be reached from outside?\n       assumed: No: it is internal only.\n" +
			"  A2   The database is PostgreSQL.\n" +
			"Where one is wrong, correct the documents and the work list before you approve it.\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the output should have %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "the council asks you first") {
		t.Errorf("a person who was asked is told to use --ask:\n%s", out)
	}
	if model.toldOwner == 0 {
		t.Error("no role was told what the person answered")
	}
	minutes, _ := filepath.Glob(filepath.Join(dir, "docs", "plans", "*", "minutes.md"))
	if len(minutes) != 1 {
		t.Fatalf("%d minutes, want one", len(minutes))
	}
	if record, _ := os.ReadFile(minutes[0]); !strings.Contains(string(record), "- Who calls the endpoint?\n  - A readiness probe.\n- May it be reached from outside?\n  - (left open: the council assumes)\n") {
		t.Errorf("the minutes do not hold what the person was asked:\n%s", record)
	}

	// Without --ask nobody is asked and nothing is read; the plan says what it
	// assumes and how to be asked the next time.
	dir = t.TempDir()
	writePlanConfig(t, dir, "repos:\n  - {name: backend, path: backend/, language: go}\n")
	model = &scriptedCouncil{curious: true}
	planModel = model
	unread := strings.NewReader("never read\n")
	planInput = unread
	out = captureStdout(t, func() {
		if code := cmdPlan([]string{"Add a health endpoint.", "--state-root", dir}); code != 0 {
			t.Errorf("cmdPlan = %d", code)
		}
	})
	if strings.Contains(out, "The council has") || unread.Len() != len("never read\n") || model.toldOwner != 0 {
		t.Errorf("a person who did not ask to be asked was asked:\n%s", out)
	}
	for _, want := range []string{"The plan assumes 2 thing(s) only you can answer", "Where one is wrong, correct the documents and the work list before you approve it. With ", "the council asks you first.\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("the output should have %q:\n%s", want, out)
		}
	}
}

// A plan says how many documents it wrote: one a role left as it was is not
// among them. And an approval that names no plan says which there are.
func TestCmdPlan_WhatAPlanWroteAndWhatWaits(t *testing.T) {
	origModel, origCreate := planModel, planCreate
	t.Cleanup(func() { planModel, planCreate = origModel, origCreate })
	dir := t.TempDir()
	writePlanConfig(t, dir, "repos:\n  - {name: backend, path: backend/, language: go}\n")
	created := 0
	planCreate = func(ctx context.Context, stateRoot, title, bodyFile string) (int, error) {
		created++
		return 300 + created, nil
	}

	planModel = &scriptedCouncil{}
	first := captureStdout(t, func() {
		if code := cmdPlan([]string{"Add a health endpoint.", "--state-root", dir}); code != 0 {
			t.Errorf("cmdPlan = %d", code)
		}
	})
	if !strings.Contains(first, "1 decisions, 5 documents written, 2 tickets proposed") {
		t.Errorf("the first plan should say it wrote five documents:\n%s", first)
	}
	// The next plan leaves every document as it is.
	planModel = &scriptedCouncil{leaves: true}
	second := captureStdout(t, func() {
		if code := cmdPlan([]string{"Add a version endpoint.", "--state-root", dir}); code != 0 {
			t.Errorf("cmdPlan = %d", code)
		}
	})
	if !strings.Contains(second, "1 decisions, 0 documents written, 2 tickets proposed") {
		t.Errorf("the second plan should say it wrote no document:\n%s", second)
	}

	plans, _ := filepath.Glob(filepath.Join(dir, "docs", "plans", "*"))
	if len(plans) != 2 {
		t.Fatalf("%d plans, want two", len(plans))
	}
	health, version := filepath.Base(plans[0]), filepath.Base(plans[1])

	// No plan named: both are listed, the newest first.
	code := -1
	listed := captureStderr(func() { code = cmdPlan([]string{"approve", "--state-root", dir}) })
	want := "Plans with tickets that are not on GitHub yet:\n  " + version + " (2 of 2 tickets)\n  " + health + " (2 of 2 tickets)\n"
	if code != 2 || !strings.Contains(listed, want) {
		t.Errorf("approve without a plan = %d, should list %q:\n%s", code, want, listed)
	}
	// With one of them on GitHub, the other is what is left.
	_ = captureStdout(t, func() {
		if code := cmdPlan([]string{"approve", health, "--state-root", dir}); code != 0 {
			t.Errorf("approve = %d", code)
		}
	})
	listed = captureStderr(func() { code = cmdPlan([]string{"approve", "--state-root", dir}) })
	if code != 2 || !strings.Contains(listed, "  "+version+" (2 of 2 tickets)\n") || strings.Contains(listed, health) {
		t.Errorf("approve without a plan = %d, should list only %s:\n%s", code, version, listed)
	}
	// With nothing left there is nothing to list.
	_ = captureStdout(t, func() {
		if code := cmdPlan([]string{"approve", version, "--state-root", dir}); code != 0 {
			t.Errorf("approve = %d", code)
		}
	})
	listed = captureStderr(func() { code = cmdPlan([]string{"approve", "--state-root", dir}) })
	if code != 2 || strings.Contains(listed, "Plans with tickets") {
		t.Errorf("approve without a plan = %d, with nothing left to approve:\n%s", code, listed)
	}
}

// A Worker reads the project from the integration branch: an approval says so
// when what the council wrote under docs/ is not committed. A dry run creates
// nothing and has nothing to warn about.
func TestCmdPlanApprove_WarnsAboutDocsThatAreNotCommitted(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	origModel, origCreate := planModel, planCreate
	t.Cleanup(func() { planModel, planCreate = origModel, origCreate })
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	writePlanConfig(t, dir, "repos:\n  - {name: backend, path: backend/, language: go}\n")
	planModel = &scriptedCouncil{}
	planCreate = func(ctx context.Context, stateRoot, title, bodyFile string) (int, error) { return 1, nil }

	// Nothing under docs/: nothing to say.
	if got := uncommittedDocs(dir); got != "" {
		t.Errorf("a project without docs has uncommitted docs: %q", got)
	}
	// A folder that is not a repository: nothing to say either.
	if got := uncommittedDocs(t.TempDir()); got != "" {
		t.Errorf("outside a repository: %q", got)
	}

	_ = captureStdout(t, func() {
		if code := cmdPlan([]string{"Add a health endpoint.", "--state-root", dir}); code != 0 {
			t.Errorf("cmdPlan = %d", code)
		}
	})
	plans, _ := filepath.Glob(filepath.Join(dir, "docs", "plans", "*"))
	if len(plans) != 1 {
		t.Fatalf("%d plans, want one", len(plans))
	}
	id := filepath.Base(plans[0])

	approve := func(args ...string) (stderr string) {
		t.Helper()
		_ = captureStdout(t, func() {
			stderr = captureStderr(func() {
				if code := cmdPlan(append([]string{"approve", id, "--state-root", dir}, args...)); code != 0 {
					t.Errorf("approve %v = %d", args, code)
				}
			})
		})
		return stderr
	}
	if warned := approve("--dry-run"); strings.Contains(warned, "not committed") {
		t.Errorf("a dry run warns about docs that are not committed:\n%s", warned)
	}
	warned := approve()
	for _, want := range []string{"docs/ has changes that are not committed. A Worker reads the project from the integration branch (develop) and will not see them:", "?? docs/"} {
		if !strings.Contains(warned, want) {
			t.Errorf("the approval should warn %q:\n%s", want, warned)
		}
	}

	// Committed: nothing to warn about. Then many changed files: the first
	// eight are named, the rest counted.
	git("add", "docs")
	git("commit", "-q", "-m", "docs")
	if got := uncommittedDocs(dir); got != "" {
		t.Errorf("committed docs are said to be uncommitted: %q", got)
	}
	for i := 0; i < 11; i++ {
		file := filepath.Join(dir, "docs", fmt.Sprintf("note-%02d.md", i))
		if err := os.WriteFile(file, []byte("a note\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	lines := strings.Split(uncommittedDocs(dir), "\n")
	if len(lines) != 9 || !strings.Contains(lines[0], "docs/note-00.md") || lines[8] != "  ... and 3 more" {
		t.Errorf("many uncommitted docs are listed as %q, want eight and a count", lines)
	}
}

// The terminal: a line is an answer, an empty line leaves the question open,
// and when the input ends the questions that are left stay open.
func TestOwnerAtTerminal(t *testing.T) {
	questions := []string{"One?", "Two\n  lines?", "Three?"}
	tests := []struct {
		name, input string
		want        []string
	}{
		{"every question answered", "  first \r\nsecond\nthird\n", []string{"first", "second", "third"}},
		{"one left open", "first\n\nthird\n", []string{"first", "", "third"}},
		{"the input ends", "first\n", []string{"first", "", ""}},
		{"a last line without its end", "first\nsecond", []string{"first", "second", ""}},
		{"no input at all", "", []string{"", "", ""}},
	}
	for _, tt := range tests {
		var out bytes.Buffer
		got, err := ownerAtTerminal(strings.NewReader(tt.input), &out)(context.Background(), questions)
		if err != nil || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: answers = %q, err %v; want %q", tt.name, got, err, tt.want)
		}
		if !strings.Contains(out.String(), "The council has 3 question(s) only you can answer.") || !strings.Contains(out.String(), "\n1/3  One?\n> ") {
			t.Errorf("%s: the questions were not put:\n%s", tt.name, out.String())
		}
		// A question is put on one line, however it was written.
		if tt.name == "every question answered" && !strings.Contains(out.String(), "\n2/3  Two lines?\n> ") {
			t.Errorf("a question of two lines was not put on one:\n%s", out.String())
		}
	}
}

// A meeting that stops is reported as a failure, with how to go on.
func TestCmdPlan_AMeetingThatStops(t *testing.T) {
	dir := t.TempDir()
	writePlanConfig(t, dir, "repos:\n  - {name: backend, path: backend/, language: go}\n")
	origModel := planModel
	t.Cleanup(func() { planModel = origModel })
	planModel = failingCouncil{}

	if code := cmdPlan([]string{"Add a health endpoint.", "--state-root", dir}); code != 1 {
		t.Errorf("cmdPlan = %d, want 1", code)
	}
	minutes, _ := filepath.Glob(filepath.Join(dir, "docs", "plans", "*", "minutes.md"))
	if len(minutes) != 1 {
		t.Fatalf("the stopped meeting left %d minutes, want one", len(minutes))
	}
	if text, _ := os.ReadFile(minutes[0]); !strings.Contains(string(text), "not finished") {
		t.Errorf("the minutes do not say the meeting stopped:\n%s", text)
	}
}

type failingCouncil struct{}

func (failingCouncil) Ask(ctx context.Context, model, prompt string) (string, error) {
	return "", fmt.Errorf("the claude CLI is not on the PATH")
}
