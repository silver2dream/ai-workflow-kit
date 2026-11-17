package council

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// A table of four for the tests: one decides, one plans, each owns a document.
const testRoster = `
limits:
  disputes: 2
  dispute_rounds: 2
  calls: 60
  parallel: 2
roles:
  - id: owner
    title: Product Owner
    decides: true
    stance: You answer for what is built.
    owns: [docs/product/requirements.md]
  - id: designer
    title: Designer
    stance: You answer for the person using it.
    owns: [docs/product/journeys.md]
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

var testRepos = []Repo{{Name: "root", Path: "./"}, {Name: "backend", Language: "go", Path: "backend/"}}

// agendaFor is the agenda of the tests for a table of the roles owner,
// designer, builder and skeptic: what is built, then how. Every role writes
// the files it owns in one of the two topics.
func agendaFor(roster *Roster) string {
	owns := func(ids ...string) []string {
		var out []string
		for _, id := range ids {
			if role := roster.Role(id); role != nil {
				out = append(out, role.Files()...)
			}
		}
		return out
	}
	list := func(items []string) string {
		if len(items) == 0 {
			return "[]"
		}
		return `["` + strings.Join(items, `", "`) + `"]`
	}
	return fmt.Sprintf(`{"topics": [
  {"id": "scope", "title": "What is built", "question": "What does the first version do?", "lead": "owner", "speakers": ["designer", "skeptic"],
   "reads": [], "writes": %s},
  {"id": "build", "title": "How it is built", "question": "How is it built and tested?", "lead": "builder", "speakers": ["designer", "skeptic"],
   "reads": %s, "writes": %s}
], "questions": []}`, list(owns("owner", "designer", "skeptic")), list(owns("owner")), list(owns("builder")))
}

func mustRoster(t *testing.T, text string) *Roster {
	t.Helper()
	roster, err := ParseRoster([]byte(text))
	if err != nil {
		t.Fatalf("roster: %v", err)
	}
	return roster
}

var (
	promptRole  = regexp.MustCompile(`^You are the .+? \(([a-z0-9-]+)\) at a planning meeting`)
	promptTopic = regexp.MustCompile(`The topic now, ([0-9]+) of [0-9]+:`)
	promptStep  = []struct{ marker, step string }{
		// The ones that carry the project's rules come first: what a rule
		// says must not be taken for the task of another step.
		{"say which of the findings above still stand", "recheck"},
		{"You check documents against the rules of a software project", "rulecheck"},
		{"Your task now: put right what the check found", "redocuments"},
		{"Your task now: put right what the check of the game data found", "redata"},
		{"Before the meeting you set its agenda", "agenda"},
		{"Your task now: you lead this topic", "propose"},
		{"Your task now: speak to the topic", "speak"},
		{"pick the ", "pick"},
		{"Your task now: answer the objection", "defence"},
		{"say whether the last answer settles your objection", "reply"},
		{"Say where the topic below stands", "summary"},
		{"Your task now: decide this topic", "decide"},
		{"Your task now: write your documents of this topic", "documents"},
		{"Your task now: lay the plan out in milestones", "worklist"},
		{"Your task now: object to the work list", "review"},
		{"Your task now: revise the milestones and the tickets", "revise"},
	}
)

// call is one question the table was asked: by whom, at which step, and in
// which topic (0 outside the topics).
type call struct {
	Role, Step string
	Topic      int
	Prompt     string
}

// table plays every role from a script: a function from who is asked what to
// the answer. It records the calls in the order they were made.
type table struct {
	mu     sync.Mutex
	calls  []call
	answer func(c call) (string, error)
}

// readCall tells who is asked what from a prompt.
func readCall(prompt string) call {
	c := call{Role: "moderator", Prompt: prompt}
	if match := promptRole.FindStringSubmatch(prompt); match != nil {
		c.Role = match[1]
	}
	if match := promptTopic.FindStringSubmatch(prompt); match != nil {
		c.Topic, _ = strconv.Atoi(match[1])
	}
	for _, known := range promptStep {
		if strings.Contains(prompt, known.marker) {
			c.Step = known.step
			break
		}
	}
	return c
}

func (m *table) Ask(ctx context.Context, model, prompt string) (string, error) {
	c := readCall(prompt)
	m.mu.Lock()
	m.calls = append(m.calls, c)
	m.mu.Unlock()
	if c.Step == "" {
		return "", fmt.Errorf("the test does not know this prompt:\n%s", prompt)
	}
	return m.answer(c)
}

// steps lists the calls as "role step", in order.
func (m *table) steps() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.calls))
	for i, c := range m.calls {
		out[i] = c.Role + " " + c.Step
	}
	return out
}

func (m *table) count(role, step string) int {
	n := 0
	for _, s := range m.steps() {
		if s == role+" "+step {
			n++
		}
	}
	return n
}

// promptOf returns the last prompt a role was given for a step.
func (m *table) promptOf(role, step string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	prompt := ""
	for _, c := range m.calls {
		if c.Role == role && c.Step == step {
			prompt = c.Prompt
		}
	}
	return prompt
}

// promptIn returns the last prompt a role was given for a step of a topic.
func (m *table) promptIn(topic int, role, step string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	prompt := ""
	for _, c := range m.calls {
		if c.Topic == topic && c.Role == role && c.Step == step {
			prompt = c.Prompt
		}
	}
	return prompt
}

func block(json string) string { return "Here it is.\n```json\n" + json + "\n```\n" }

func docs(role string, paths ...string) string {
	var b strings.Builder
	for _, path := range paths {
		fmt.Fprintf(&b, "<<<DOCUMENT %s>>>\n# %s\n\nOwner: %s.\n\nWritten by %s.\n<<<END>>>\n", path, path, role, role)
	}
	return b.String()
}

var askedLine = regexp.MustCompile(`(?m)^- (\S+\.md)$`)

// asked lists the documents a role is asked to write in a prompt.
func asked(prompt string) []string {
	const task = "Your task now: write your documents of this topic, as they read after it:\n"
	at := strings.Index(prompt, task)
	if at < 0 {
		return nil
	}
	list := prompt[at+len(task):]
	if end := strings.Index(list, "\n\n"); end >= 0 {
		list = list[:end]
	}
	var out []string
	for _, match := range askedLine.FindAllStringSubmatch(list, -1) {
		out = append(out, match[1])
	}
	return out
}

const goodWork = `{"milestones": [
  {"key": "m1", "title": "The endpoint answers", "goal": "The smallest thing that shows it works.", "done_when": ["GET /healthz answers 200"]},
  {"key": "m2", "title": "It is watched", "goal": "Somebody hears when it fails.", "done_when": ["an alert fires when it fails"]}
], "tickets": [
  {"key": "t2", "title": "[feat] serve the health endpoint", "repo": "backend", "summary": "GET /healthz.", "scope": ["handler"], "acceptance": ["GET /healthz answers 200"], "testing": ["a handler test"], "reads": ["docs/tech/architecture.md"], "depends_on": ["t1"]},
  {"key": "t1", "title": "[chore] set up the module", "repo": "backend", "summary": "The Go module.", "scope": ["go.mod"], "acceptance": ["go build passes"], "testing": [], "depends_on": []}
], "later": ["metrics"]}`

// agreeable answers every question in the simplest acceptable way: the
// agenda of the tests, proposals nobody objects to, a decision a topic, the
// documents asked for, a work list nobody objects to.
func agreeable(roster *Roster) func(c call) (string, error) {
	return func(c call) (string, error) {
		switch c.Step {
		case "agenda":
			return block(agendaFor(roster)), nil
		case "propose":
			return block(fmt.Sprintf(`{"says": "%s proposes for topic %d.", "proposals": ["%s proposes"], "concerns": ["%s worries"]}`, c.Role, c.Topic, c.Role, c.Role)), nil
		case "speak":
			return block(fmt.Sprintf(`{"says": "%s agrees.", "proposals": [], "concerns": [], "objections": []}`, c.Role)), nil
		case "review":
			return block(`{"objections": []}`), nil
		case "summary":
			return block(`{"consensus": ["build the small thing"], "open": [], "questions": []}`), nil
		case "decide":
			if c.Topic == 2 {
				return block(`{"decisions": [{"title": "stack", "decision": "One Go handler.", "rationale": "It is all the endpoint needs.", "dissent": ""}]}`), nil
			}
			return block(`{"decisions": [{"title": "scope", "decision": "Build the health endpoint only.", "rationale": "It is the smallest thing worth shipping.", "dissent": ""}]}`), nil
		case "documents":
			return docs(c.Role, asked(c.Prompt)...), nil
		case "worklist", "revise":
			return block(goodWork), nil
		case "rulecheck":
			return block(`{"findings": []}`), nil
		case "recheck":
			return block(`{"stands": []}`), nil
		}
		return "", fmt.Errorf("the agreeable table has no answer for %s %s", c.Role, c.Step)
	}
}

func fixedNow() time.Time { return time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC) }

func runPlan(t *testing.T, root string, roster *Roster, model Model, brief string) (*Plan, error) {
	t.Helper()
	return Run(context.Background(), Options{Root: root, Brief: brief, Roster: roster, Model: model, Repos: testRepos, Now: fixedNow})
}

func readFile(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("%s: %v", rel, err)
	}
	return string(data)
}

func exists(root, rel string) bool {
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil
}

// putFile writes a project file as a person would have.
func putFile(t *testing.T, root, rel, content string) {
	t.Helper()
	file := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A meeting where nobody objects: the moderator sets the agenda; in each
// topic the lead proposes, the others speak in turn, the moderator sums up,
// the deciding role rules and the owners write; then the planning role lays
// out the milestones and the others look at the list.
func TestRun_AMeetingWithoutObjections(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, testRoster)
	model := &table{answer: agreeable(roster)}

	plan, err := runPlan(t, root, roster, model, "Add a health endpoint to the API server.")

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if plan.ID != "20261114-add-a-health-endpoint-to" {
		t.Errorf("plan id = %q", plan.ID)
	}
	want := []string{
		"moderator agenda",
		"owner propose", "designer speak", "skeptic speak", "moderator summary", "owner decide",
		"owner documents", "designer documents", "skeptic documents",
		"builder propose", "designer speak", "skeptic speak", "moderator summary", "owner decide", "builder documents",
		"builder worklist", "owner review", "designer review", "skeptic review",
	}
	got := model.steps()
	// The owners of a topic write at the same time, and the others look at
	// the list at the same time: in those two places the order is free.
	sorted := func(steps []string, from, to int) string {
		part := append([]string(nil), steps[from:to]...)
		for i := range part {
			for j := i + 1; j < len(part); j++ {
				if part[j] < part[i] {
					part[i], part[j] = part[j], part[i]
				}
			}
		}
		return strings.Join(part, ", ")
	}
	if len(got) != len(want) || strings.Join(got[:6], ",") != strings.Join(want[:6], ",") || sorted(got, 6, 9) != sorted(want, 6, 9) ||
		strings.Join(got[9:16], ",") != strings.Join(want[9:16], ",") || sorted(got, 16, 19) != sorted(want, 16, 19) {
		t.Fatalf("the order of play:\n%v\nwant\n%v", got, want)
	}
	if plan.Calls != len(want) {
		t.Errorf("%d calls counted, %d made", plan.Calls, len(want))
	}
	// Who speaks to which topic.
	for _, c := range model.calls {
		switch c.Step {
		case "propose", "speak", "summary", "decide", "documents":
			if c.Topic == 0 {
				t.Errorf("%s %s does not say which topic it is in:\n%s", c.Role, c.Step, c.Prompt)
			}
		}
	}

	// What is on disk.
	if got := readFile(t, root, plan.Dir()+"/brief.md"); got != "Add a health endpoint to the API server.\n" {
		t.Errorf("brief.md = %q", got)
	}
	for _, role := range roster.Roles {
		for _, doc := range role.Owns {
			if content := readFile(t, root, doc); !strings.Contains(content, "Written by "+role.ID+".") {
				t.Errorf("%s was not written by its owner %s:\n%s", doc, role.ID, content)
			}
		}
	}
	agenda, err := LoadAgenda(root, plan.ID)
	if err != nil || len(agenda.Topics) != 2 || agenda.Topics[0].ID != "scope" || agenda.Topics[1].Lead != "builder" {
		t.Fatalf("the agenda on disk = %+v, %v", agenda, err)
	}
	if file := readFile(t, root, plan.Dir()+"/"+AgendaFile); !strings.Contains(file, "awkit plan --resume "+plan.ID) {
		t.Errorf("the agenda does not say how to go on after changing it:\n%s", file)
	}
	minutes := readFile(t, root, plan.Dir()+"/minutes.md")
	for _, want := range []string{"Status: finished. 2 tickets proposed",
		"| 1 | [What is built](topics/01-scope.md) | owner | designer, skeptic | `docs/product/requirements.md`, `docs/product/journeys.md`, `docs/qa/risks.md` | held |",
		"| 2 | [How it is built](topics/02-build.md) | builder | designer, skeptic | `docs/tech/architecture.md` | held |",
		"### 1. What is built\n\n- **D1 scope**: Build the health endpoint only.", "### 2. How it is built\n\n- **D2 stack**: One Go handler.",
		"`docs/qa/risks.md` (skeptic)", "- **m1 The endpoint answers** (ticketed): The smallest thing that shows it works.\n  - [ ] GET /healthz answers 200\n",
		"- **m2 It is watched**: Somebody hears when it fails.", "| t1 | [chore] set up the module | backend | - |",
		"| t2 | [feat] serve the health endpoint | backend | t1 |", "awkit plan approve " + plan.ID, "Left for a later plan:\n- metrics"} {
		if !strings.Contains(minutes, want) {
			t.Errorf("the minutes should say %q:\n%s", want, minutes)
		}
	}
	topic := readFile(t, root, plan.Dir()+"/topics/01-scope.md")
	for _, want := range []string{"# Plan " + plan.ID + ", topic 1: What is built", "- It settles: What does the first version do?\n- Lead: owner\n- Speaking after, in order: designer, skeptic\n",
		"## owner (Product Owner) proposes\n\nowner proposes for topic 1.\n\nProposes:\n- owner proposes\n", "## designer (Designer)\n\ndesigner agrees.\n",
		"## Where it stood", "- **D1 scope**: Build the health endpoint only.", "- `docs/product/journeys.md` (designer)\n"} {
		if !strings.Contains(topic, want) {
			t.Errorf("the minutes of the first topic should say %q:\n%s", want, topic)
		}
	}
	decisions := readFile(t, root, DecisionsPath)
	for _, want := range []string{"# Decisions", "## 2026-11-14, plan " + plan.ID, "<!-- plan: " + plan.ID + " -->",
		"Decided by owner, a topic at a time. At the table: owner, designer, builder, skeptic.",
		"### What is built\n\n<!-- plan: " + plan.ID + " topic: scope -->\n\n- **D1 scope**: Build the health endpoint only.\n  - Why: It is the smallest thing worth shipping.\n",
		"### How it is built\n\n<!-- plan: " + plan.ID + " topic: build -->\n\n- **D2 stack**: One Go handler.\n"} {
		if !strings.Contains(decisions, want) {
			t.Errorf("the decision log should say %q:\n%s", want, decisions)
		}
	}
	if strings.Count(decisions, "## 2026-11-14, plan") != 1 {
		t.Errorf("the plan has more than one heading in the decision log:\n%s", decisions)
	}
	index := readFile(t, root, "docs/README.md")
	for _, want := range []string{"| Document | Owner |\n|---|---|\n", "| [docs/qa/risks.md](qa/risks.md) | skeptic |\n",
		"| [docs/decisions.md](decisions.md) | the deciding role, through `awkit plan`; it only grows |\n",
		"| [" + plan.ID + "](plans/" + plan.ID + "/minutes.md) | [tickets.yaml](plans/" + plan.ID + "/tickets.yaml) |\n"} {
		if !strings.Contains(index, want) {
			t.Errorf("the index should say %q:\n%s", want, index)
		}
	}
	// The index points to a document and copies nothing from it: how far a
	// document is, the document says itself.
	for _, copied := range []string{"Status", "status", "draft", "2 tickets", "on GitHub"} {
		if strings.Contains(index, copied) {
			t.Errorf("the index says %q, which is the document's own to say:\n%s", copied, index)
		}
	}
	// A project without rules: the documents are written without, nothing is
	// read against any, and the minutes say nothing about rules.
	if plan.Rules != nil || plan.RuleChecks != nil || strings.Contains(minutes, "rules") {
		t.Errorf("a project without rules: rules %v, checks %v, minutes:\n%s", plan.Rules, plan.RuleChecks, minutes)
	}

	// The work list: the milestones, the first one's tickets in an order that
	// can be worked, nothing on GitHub.
	list, err := LoadWorkList(root, plan.ID)
	if err != nil {
		t.Fatalf("LoadWorkList: %v", err)
	}
	if len(list.Milestones) != 2 || list.Milestones[0].Key != "m1" || list.Milestones[1].DoneWhen[0] != "an alert fires when it fails" {
		t.Errorf("milestones = %+v", list.Milestones)
	}
	if len(list.Tickets) != 2 || list.Tickets[0].Key != "t1" || list.Tickets[1].Key != "t2" || list.Tickets[0].Issue != 0 || list.Tickets[1].Issue != 0 {
		t.Errorf("work list = %+v, want t1 before t2 and no issue yet", list.Tickets)
	}
	file := readFile(t, root, plan.Dir()+"/"+WorkListFile)
	for _, want := range []string{"awkit plan approve " + plan.ID, "the tickets are those of the first"} {
		if !strings.Contains(file, want) {
			t.Errorf("the work list should say %q:\n%s", want, file)
		}
	}
	// Every prompt and answer is kept, outside the documents.
	recorded, _ := filepath.Glob(filepath.Join(root, ".ai", "state", "council", plan.ID, "calls", "*.md"))
	if len(recorded) != plan.Calls {
		t.Errorf("%d calls on record, want %d", len(recorded), plan.Calls)
	}
}

// A topic starts from what the topics before it settled: its roles read the
// documents those wrote, and are told what was decided.
func TestRun_ATopicStartsFromTheOnesBefore(t *testing.T) {
	roster := mustRoster(t, testRoster)
	model := &table{answer: agreeable(roster)}
	if _, err := runPlan(t, t.TempDir(), roster, model, "Add a health endpoint."); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for name, prompt := range map[string]string{"the lead": model.promptIn(2, "builder", "propose"), "a speaker": model.promptIn(2, "skeptic", "speak")} {
		for _, want := range []string{
			"Topics taken up before this one:\n- 1. What is built: ended in docs/product/requirements.md, docs/product/journeys.md, docs/qa/risks.md\n",
			"What this meeting has decided so far:\nD1. scope: Build the health endpoint only.\n",
			"The topic now, 2 of 2: How it is built\nIt settles: How is it built and tested?\nbuilder leads; speaking after, in this order: designer, skeptic.\nIt ends in these documents: docs/tech/architecture.md\n",
			"The documents of this topic, as they stand. Read them before you answer:\n===== docs/product/requirements.md (owner's)\n# docs/product/requirements.md\n\nOwner: owner.\n\nWritten by owner.\n",
		} {
			if !strings.Contains(prompt, want) {
				t.Errorf("%s of the second topic should be told %q:\n%s", name, want, prompt)
			}
		}
	}
	// The first topic has nothing before it.
	first := model.promptIn(1, "owner", "propose")
	for _, not := range []string{"Topics taken up before", "What this meeting has decided so far", "The documents of this topic, as they stand"} {
		if strings.Contains(first, not) {
			t.Errorf("the first topic is told %q", not)
		}
	}
	if !strings.Contains(first, "None of the documents of this topic is written yet.") {
		t.Errorf("the first topic is not told that nothing is written:\n%s", first)
	}
	// A speaker is shown what was said before it, and only that.
	skeptic := model.promptIn(1, "skeptic", "speak")
	if !strings.Contains(skeptic, "What was said to the topic so far:\n\n## owner (Product Owner), who leads, proposes:\nowner proposes for topic 1.\nProposes:\n  - owner proposes\nWorried about:\n  - owner worries\n\n## designer (Designer):\ndesigner agrees.\n") {
		t.Errorf("the second speaker is not shown what was said before it:\n%s", skeptic)
	}
	if designer := model.promptIn(1, "designer", "speak"); strings.Contains(designer, "## skeptic (Skeptic):") {
		t.Error("the first speaker is shown what the second said")
	}
	// A speaker is told whom it can object to: those who spoke before it.
	for role, want := range map[string]string{"designer": "an objection is addressed to one of owner;", "skeptic": "an objection is addressed to one of owner, designer;"} {
		if prompt := model.promptIn(1, role, "speak"); !strings.Contains(prompt, want) {
			t.Errorf("%s is not told whom it can object to (%q):\n%s", role, want, prompt)
		}
	}
	// The deciding role is shown the whole topic, and what was decided before it.
	decide := model.promptIn(2, "owner", "decide")
	for _, want := range []string{"What this meeting has decided so far:\nD1. scope", "## skeptic (Skeptic):\nskeptic agrees.", "Agreed:\n  - build the small thing\n"} {
		if !strings.Contains(decide, want) {
			t.Errorf("the deciding role should be shown %q:\n%s", want, decide)
		}
	}
	// The owner who writes is shown the decisions up to and with its topic.
	if documents := model.promptIn(2, "builder", "documents"); !strings.Contains(documents, "The topic is decided. What this meeting has decided, up to and with this topic:\nD1. scope: Build the health endpoint only.\n   Why: It is the smallest thing worth shipping.\nD2. stack: One Go handler.") {
		t.Errorf("the owner who writes is not shown the decisions:\n%s", documents)
	}
}

// disputing is the agreeable table with two objections: in the first topic
// the skeptic's against the owner, which the owner concedes at once; in the
// second the designer's against the builder, who holds both rounds.
func disputing(roster *Roster) func(c call) (string, error) {
	base := agreeable(roster)
	return func(c call) (string, error) {
		switch {
		case c.Step == "speak" && c.Role == "skeptic" && c.Topic == 1:
			return block(`{"says": "It can fail.", "objections": [{"to": "owner", "point": "No failure is named.", "why": "Nobody would know it is broken.", "wants": "A failure case in the requirements."}]}`), nil
		case c.Step == "speak" && c.Role == "designer" && c.Topic == 2:
			return block(`{"says": "People read it.", "objections": [{"to": "builder", "point": "The endpoint returns JSON only.", "why": "People read it in a browser.", "wants": "A plain text answer too."}]}`), nil
		case c.Step == "defence" && c.Role == "owner":
			return block(`{"stance": "concede", "reply": "Right. I add it.", "position": "The requirements name what happens when the database is down."}`), nil
		case c.Step == "defence" && c.Role == "builder":
			return block(`{"stance": "Hold", "reply": "Monitoring reads JSON.", "position": "JSON only."}`), nil
		case c.Step == "reply" && c.Role == "designer":
			return block(`{"accepts": false, "reply": "People still read it.", "wants": "Text for browsers."}`), nil
		case c.Step == "summary" && c.Topic == 2:
			return block(`{"consensus": ["a health endpoint"], "open": ["designer wants plain text, builder holds JSON only"], "questions": []}`), nil
		case c.Step == "decide" && c.Topic == 2:
			return block(`{"decisions": [{"title": "format", "decision": "JSON only.", "rationale": "Its reader is a monitor.", "dissent": "designer wanted plain text for browsers"}]}`), nil
		}
		return base(c)
	}
}

// An objection is argued out between the two roles, within its topic: the
// defender answers, the challenger says whether that settles it, and what is
// not settled when the rounds are used up goes to the deciding role.
func TestRun_DisputesAreArguedOut(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, testRoster)
	model := &table{answer: disputing(roster)}

	plan, err := runPlan(t, root, roster, model, "Add a health endpoint.")

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(plan.Topics[0].Disputes) != 1 || len(plan.Topics[1].Disputes) != 1 {
		t.Fatalf("disputes: %+v and %+v; want one in each topic", plan.Topics[0].Disputes, plan.Topics[1].Disputes)
	}
	conceded, held := plan.Topics[0].Disputes[0], plan.Topics[1].Disputes[0]
	if conceded.Challenger != "skeptic" || conceded.Defender != "owner" || held.Challenger != "designer" || held.Defender != "builder" {
		t.Fatalf("the disputes are not between the roles that objected: %+v, %+v", conceded, held)
	}
	if !conceded.Settled || len(conceded.Exchanges) != 1 || conceded.Exchanges[0].Answered {
		t.Errorf("the conceded dispute = %+v; want it settled in one round, the challenger not asked", conceded)
	}
	if held.Settled || len(held.Exchanges) != 2 || !held.Exchanges[1].Answered || held.Exchanges[1].Accepts {
		t.Errorf("the held dispute = %+v; want two rounds and no agreement", held)
	}
	if held.Exchanges[0].Stance != "hold" {
		t.Errorf("stance = %q, want it read whatever its case", held.Exchanges[0].Stance)
	}
	for step, want := range map[string]int{"owner defence": 1, "skeptic reply": 0, "builder defence": 2, "designer reply": 2, "moderator pick": 0} {
		parts := strings.SplitN(step, " ", 2)
		if got := model.count(parts[0], parts[1]); got != want {
			t.Errorf("%s: %d calls, want %d", step, got, want)
		}
	}

	// The deciding role is shown what was not settled, in full, and only that.
	decide := model.promptIn(2, "owner", "decide")
	if !strings.Contains(decide, "designer against builder") || !strings.Contains(decide, "People still read it.") {
		t.Errorf("the deciding role was not shown the open dispute:\n%s", decide)
	}
	if strings.Contains(decide, "skeptic against owner") {
		t.Errorf("the deciding role was asked to rule on a dispute of another topic:\n%s", decide)
	}
	// A role remembers nothing: whoever defends is told what it said.
	if defence := model.promptOf("owner", "defence"); !strings.Contains(defence, "What you proposed:\nowner proposes for topic 1.\nProposes:\n  - owner proposes\n") || !strings.Contains(defence, "The topic now, 1 of 2: What is built") {
		t.Errorf("the defender is not reminded of what it proposed:\n%s", defence)
	}
	if reply := model.promptOf("designer", "reply"); !strings.Contains(reply, "What you said:\nPeople read it.\n") {
		t.Errorf("the challenger is not reminded of what it said:\n%s", reply)
	}
	if conceded.Outcome != "owner conceded in round 1" {
		t.Errorf("outcome = %q", conceded.Outcome)
	}
	// The second round shows the first.
	var second string
	for _, c := range model.calls {
		if c.Step == "defence" && c.Role == "builder" && strings.Contains(c.Prompt, "How it has gone so far") {
			second = c.Prompt
		}
	}
	if !strings.Contains(second, "designer does not accept: People still read it.") {
		t.Errorf("the defender's second round does not show the first:\n%s", second)
	}

	first, last := readFile(t, root, plan.Dir()+"/topics/01-scope.md"), readFile(t, root, plan.Dir()+"/topics/02-build.md")
	if !strings.Contains(first, "skeptic against owner: **settled**") || !strings.Contains(first, "Objects:\n- **to owner**: No failure is named.") {
		t.Errorf("the minutes of the first topic do not have its dispute:\n%s", first)
	}
	for _, want := range []string{"designer against builder: **not settled**", "Round 2, builder (hold)", "not settled after 2 round(s); it goes to owner", "Held otherwise: designer wanted plain text for browsers"} {
		if !strings.Contains(last, want) {
			t.Errorf("the minutes of the second topic should say %q:\n%s", want, last)
		}
	}
	if !strings.Contains(readFile(t, root, DecisionsPath), "- **D2 format**: JSON only.\n  - Why: Its reader is a monitor.\n  - Held otherwise: designer wanted plain text for browsers\n") {
		t.Error("the dissent is not in the decision log")
	}
}

// A challenger who accepts the answer ends the dispute.
func TestRun_AnAcceptedAnswerSettlesADispute(t *testing.T) {
	roster := mustRoster(t, testRoster)
	base := disputing(roster)
	model := &table{answer: func(c call) (string, error) {
		if c.Step == "reply" {
			return block(`{"accepts": true, "reply": "That deals with it.", "wants": ""}`), nil
		}
		return base(c)
	}}

	plan, err := runPlan(t, t.TempDir(), roster, model, "Add a health endpoint.")

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	held := plan.Topics[1].Disputes[0]
	if !held.Settled || held.Outcome != "designer accepted builder's answer in round 1" {
		t.Errorf("the dispute = %+v", held)
	}
	if got := model.count("builder", "defence"); got != 1 {
		t.Errorf("the defender was asked %d times after the challenger accepted, want 1", got)
	}
}

// With more objections in a topic than the roster argues out, the moderator
// picks; with fewer, every one is argued and the moderator is not asked.
func TestRun_TheModeratorPicksWhenThereAreTooManyObjections(t *testing.T) {
	roster := mustRoster(t, testRoster)
	base := agreeable(roster)
	model := &table{answer: func(c call) (string, error) {
		switch {
		case c.Step == "speak" && c.Topic == 1 && c.Role == "designer":
			return block(`{"says": "No.", "objections": [{"to": "owner", "point": "designer objects", "why": "w", "wants": "x"}]}`), nil
		case c.Step == "speak" && c.Topic == 1 && c.Role == "skeptic":
			return block(`{"says": "No.", "objections": [{"to": "owner", "point": "skeptic objects", "why": "w", "wants": "x"}, {"to": "designer", "point": "skeptic objects to designer", "why": "w", "wants": "x"}]}`), nil
		case c.Step == "pick":
			return block(`{"disputes": [{"topic": "one", "challenger": "skeptic", "defender": "owner", "point": "skeptic objects", "wants": "x"},
				{"topic": "two", "challenger": "designer", "defender": "owner", "point": "designer objects", "wants": "x"},
				{"topic": "three", "challenger": "skeptic", "defender": "designer", "point": "skeptic objects to designer", "wants": "x"}]}`), nil
		case c.Step == "defence":
			return block(`{"stance": "concede", "reply": "Right.", "position": "Changed."}`), nil
		}
		return base(c)
	}}

	plan, err := runPlan(t, t.TempDir(), roster, model, "Add a health endpoint.")

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := model.count("moderator", "pick"); got != 1 {
		t.Errorf("the moderator picked %d times, want once", got)
	}
	// The roster argues out two; what the moderator picks beyond that is cut.
	if len(plan.Topics[0].Objections) != 3 || len(plan.Topics[0].Disputes) != 2 {
		t.Errorf("%d objections, %d disputes; want 3 and 2", len(plan.Topics[0].Objections), len(plan.Topics[0].Disputes))
	}
	pick := model.promptOf("moderator", "pick")
	for _, want := range []string{"You have no opinion on the subject", "The topic: What is built", "skeptic objects to owner", "designer objects to owner", "pick the 2 "} {
		if !strings.Contains(pick, want) {
			t.Errorf("the moderator's prompt should say %q:\n%s", want, pick)
		}
	}
	if strings.Contains(plan.Topics[1].Topic.ID, "scope") || len(plan.Topics[1].Disputes) != 0 {
		t.Errorf("the second topic has disputes: %+v", plan.Topics[1].Disputes)
	}
}

// A role gets to write the documents it owns that its topic ends in, and no
// other. What it hands in beyond that is left out, and the minutes say so.
func TestRun_ARoleCannotWriteAnotherRolesDocument(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, testRoster)
	base := agreeable(roster)
	model := &table{answer: func(c call) (string, error) {
		if c.Step == "documents" && c.Role == "skeptic" {
			return docs("skeptic", "docs/qa/risks.md", "docs/product/requirements.md", "docs/decisions.md", "../outside.md", "docs/qa/extra.md"), nil
		}
		return base(c)
	}}

	plan, err := runPlan(t, root, roster, model, "Add a health endpoint.")

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if content := readFile(t, root, "docs/product/requirements.md"); !strings.Contains(content, "Written by owner.") {
		t.Errorf("the owner's document was written by someone else:\n%s", content)
	}
	if !strings.Contains(readFile(t, root, "docs/qa/risks.md"), "Written by skeptic.") {
		t.Error("the skeptic's own document was not written")
	}
	if strings.Contains(readFile(t, root, DecisionsPath), "Written by skeptic.") {
		t.Error("a role wrote into the decision log")
	}
	for _, path := range []string{"docs/qa/extra.md", "outside.md", "../outside.md"} {
		if exists(root, path) {
			t.Errorf("%s was written", path)
		}
	}
	refused := 0
	for _, doc := range plan.Documents {
		if doc.Owner == "skeptic" && doc.Refused != "" {
			refused++
		}
	}
	if refused != 4 {
		t.Errorf("%d documents refused, want the four the skeptic does not own: %+v", refused, plan.Documents)
	}
	minutes := readFile(t, root, plan.Dir()+"/topics/01-scope.md")
	if !strings.Contains(minutes, "`docs/product/requirements.md` handed in by skeptic: **not written**, skeptic does not own this document") {
		t.Errorf("the minutes do not say that a document was refused:\n%s", minutes)
	}
}

// An answer that cannot be used is asked for once more, with what was wrong
// with it. The role is told; nothing is guessed.
func TestRun_AnAnswerThatCannotBeUsedIsAskedForAgain(t *testing.T) {
	roster := mustRoster(t, testRoster)
	withWork := func(old, new string) string { return block(strings.Replace(goodWork, old, new, 1)) }
	tests := []struct {
		name, role, step string
		topic            int
		bad, wantTold    string
	}{
		{"no JSON", "designer", "speak", 1, "I think we should build it.", "holds no JSON object"},
		{"broken JSON", "designer", "speak", 1, "```json\n{\"says\": \n```", "cannot be read"},
		{"a proposal that says nothing", "owner", "propose", 1, block(`{"says": " ", "proposals": []}`), "the proposal says nothing"},
		{"a speech that says nothing", "designer", "speak", 1, block(`{"says": ""}`), "says is empty"},
		{"an objection to oneself", "skeptic", "speak", 1, block(`{"says": "s", "objections": [{"to": "skeptic", "point": "p"}]}`), "addressed to yourself"},
		{"an objection to a role that has not spoken yet", "designer", "speak", 1, block(`{"says": "s", "objections": [{"to": "skeptic", "point": "p"}]}`), `"skeptic", who has not spoken to this topic (so far: owner)`},
		{"an objection to nobody at the table", "skeptic", "speak", 1, block(`{"says": "s", "objections": [{"to": "cfo", "point": "p"}]}`), `"cfo", who has not spoken to this topic (so far: owner, designer)`},
		{"an objection without a point", "skeptic", "speak", 1, block(`{"says": "s", "objections": [{"to": "owner", "point": " "}]}`), "an objection has no point"},
		{"a decision that decides nothing", "owner", "decide", 1, block(`{"decisions": []}`), "nothing was decided"},
		{"a decision without what is decided", "owner", "decide", 1, block(`{"decisions": [{"title": "t", "decision": " ", "rationale": "r"}]}`), "a decision does not say what is decided"},
		{"an assumption that assumes nothing", "owner", "decide", 1, block(`{"decisions": [{"title": "t", "decision": "d"}], "assumptions": [{"question": "Who calls it?", "assumption": " "}]}`), "an assumption does not say what is assumed"},
		{"a document that is not written, left unchanged", "builder", "documents", 2, "<<<UNCHANGED docs/tech/architecture.md>>>\n", "docs/tech/architecture.md is not written yet, so it cannot stay unchanged"},
		{"a document too long to be rewritten", "builder", "documents", 2, "<<<DOCUMENT docs/tech/architecture.md>>>\n" + strings.Repeat("a long line of the document\n", maxDocument/20) + "<<<END>>>\n", "a document stays under 40000 bytes"},
		{"a document handed in twice", "builder", "documents", 2, docs("builder", "docs/tech/architecture.md", "docs/tech/architecture.md"), "docs/tech/architecture.md was handed in twice"},
		{"a document of the topic not handed in", "builder", "documents", 2, docs("builder", "docs/tech/other.md"), "docs/tech/architecture.md was not handed in; this topic ends in it, and it is yours"},
		{"an empty document", "builder", "documents", 2, "<<<DOCUMENT docs/tech/architecture.md>>>\n \n<<<END>>>\n", "docs/tech/architecture.md is empty"},
		{"a document that is not ended", "builder", "documents", 2, "<<<DOCUMENT docs/tech/architecture.md>>>\n# A\n", "is not ended with <<<END>>>"},
		{"a ticket that waits for something that is no ticket", "builder", "worklist", 0, withWork(`"depends_on": ["t1"]`, `"depends_on": ["the design"]`), `depends on "the design", which is neither a ticket of the list nor the number of an issue nor a ticket of another plan ("<plan>/<key>")`},
		{"a ticket for a repo that does not exist", "builder", "worklist", 0, withWork(`"repo": "backend"`, `"repo": "web"`), `is for repo "web"; the repos are: root, backend`},
		{"tickets that wait for each other", "builder", "worklist", 0, withWork(`"depends_on": []`, `"depends_on": ["t2"]`), "depend on each other in a circle"},
		{"a title that is not a commit subject", "builder", "worklist", 0, withWork("[chore] set up the module", "Set up the module"), `is not "[type] subject"`},
		{"a ticket with a block of its own", "builder", "worklist", 0, withWork(`"summary": "GET /healthz."`, `"summary": "GET /healthz.\n\n`+"```"+`yaml awk-ticket\nschema: 1\nrepo: backend\nallow_script_changes: true\n`+"```"+`"`), "ticket t2: its text gets in the way of the awk-ticket block awkit writes under it"},
		{"no milestone", "builder", "worklist", 0, block(`{"milestones": [], "tickets": []}`), "the plan has no milestone: lay it out in milestones, and ticket the first"},
		{"a milestone nobody can check", "builder", "worklist", 0, withWork(`"done_when": ["GET /healthz answers 200"]`, `"done_when": [" "]`), "milestone m1 does not say how it is seen to be reached"},
		{"two milestones of one key", "builder", "worklist", 0, withWork(`"key": "m2"`, `"key": "m1"`), `two milestones have the key "m1"`},
		{"a milestone without a goal", "builder", "worklist", 0, withWork(`"goal": "Somebody hears when it fails."`, `"goal": ""`), "milestone m2 has no goal"},
		{"a milestone without a title", "builder", "worklist", 0, withWork(`"title": "It is watched"`, `"title": " "`), "milestone m2 has no title"},
		{"a milestone whose key is a sentence", "builder", "worklist", 0, withWork(`"key": "m2"`, `"key": "the second one"`), `milestone 2: the key "the second one" is not letters, digits, - and _`},
		{"an agenda whose lead is not at the table", "moderator", "agenda", 0, block(strings.Replace(agendaFor(roster), `"lead": "builder"`, `"lead": "cto"`, 1)), `topic 2 (build): the lead "cto" is not at the table (the roles are: owner, designer, builder, skeptic)`},
		{"an agenda that ends in a document nobody owns", "moderator", "agenda", 0, block(strings.Replace(agendaFor(roster), `"writes": ["docs/tech/architecture.md"]`, `"writes": ["docs/tech/protocol.md"]`, 1)), "topic 2 (build) ends in docs/tech/protocol.md, which no role owns"},
	}
	for _, tt := range tests {
		base := agreeable(roster)
		asked := 0
		var second string
		model := &table{answer: func(c call) (string, error) {
			if c.Role == tt.role && c.Step == tt.step && c.Topic == tt.topic {
				asked++
				if asked == 1 {
					return tt.bad, nil
				}
				if asked == 2 {
					second = c.Prompt
				}
			}
			return base(c)
		}}

		_, err := runPlan(t, t.TempDir(), roster, model, "Add a health endpoint.")

		if err != nil {
			t.Errorf("%s: Run: %v", tt.name, err)
			continue
		}
		if asked != 2 {
			t.Errorf("%s: %s was asked %d times, want twice", tt.name, tt.role, asked)
		}
		if !strings.Contains(second, "Your previous answer could not be used: ") || !strings.Contains(second, tt.wantTold) {
			t.Errorf("%s: the second question should say what was wrong (%q):\n%s", tt.name, tt.wantTold, second[max(0, len(second)-400):])
		}
	}
}

// A second answer that cannot be used stops the meeting. What was reached is
// on record, and going on with the plan asks only what is still missing.
func TestRun_AMeetingThatStopsCanBeGoneOnWith(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, testRoster)
	base := disputing(roster)
	broken := true
	model := &table{answer: func(c call) (string, error) {
		if broken && c.Step == "decide" && c.Topic == 2 {
			return "I cannot decide.", nil
		}
		return base(c)
	}}

	plan, err := runPlan(t, root, roster, model, "Add a health endpoint.")

	if err == nil || !strings.Contains(err.Error(), "the meeting stopped at topic 2 (build)") || !strings.Contains(err.Error(), "holds no JSON object") {
		t.Fatalf("err = %v, want the meeting stopped at the second topic with the reason", err)
	}
	if plan == nil || !plan.Topics[0].Held || plan.Topics[1].Held || len(plan.Topics[1].Speeches) != 2 || len(plan.Topics[1].Disputes) != 1 {
		t.Fatalf("the plan does not hold what was reached: %+v", plan)
	}
	minutes := readFile(t, root, plan.Dir()+"/minutes.md")
	for _, want := range []string{"Status: **not finished**", "awkit plan --resume " + plan.ID, "| [What is built](topics/01-scope.md) |", "| held |",
		"| 2 | [How it is built](topics/02-build.md) | builder | designer, skeptic | `docs/tech/architecture.md` | **stopped** |"} {
		if !strings.Contains(minutes, want) {
			t.Errorf("the minutes of a stopped meeting should say %q:\n%s", want, minutes)
		}
	}
	if !strings.Contains(readFile(t, root, plan.Dir()+"/topics/02-build.md"), "designer against builder: **not settled**") {
		t.Error("what the stopped topic reached is not in its minutes")
	}
	// The first topic is written down; the second is not.
	if !exists(root, "docs/product/requirements.md") || exists(root, "docs/tech/architecture.md") || exists(root, plan.Dir()+"/"+WorkListFile) {
		t.Error("what was written does not match what was reached")
	}
	if log := readFile(t, root, DecisionsPath); !strings.Contains(log, "topic: scope") || strings.Contains(log, "topic: build") {
		t.Errorf("the decision log does not hold the first topic's decisions only:\n%s", log)
	}

	// Going on: the deciding role answers this time.
	broken = false
	before := len(model.steps())
	var told []string
	resumed, err := Run(context.Background(), Options{Root: root, ID: plan.ID, Roster: roster, Model: model, Repos: testRepos, Now: fixedNow,
		Progress: func(line string) { told = append(told, line) }})

	if err != nil {
		t.Fatalf("going on: %v", err)
	}
	if resumed.Brief != "Add a health endpoint." || !resumed.Topics[1].Held || len(resumed.Tickets) != 2 || len(resumed.Decisions) != 2 {
		t.Errorf("the resumed plan = brief %q, %d decisions, %d tickets", resumed.Brief, len(resumed.Decisions), len(resumed.Tickets))
	}
	asked := model.steps()[before:]
	if want := []string{"owner decide", "builder documents", "builder worklist"}; len(asked) != 6 || strings.Join(asked[:3], ",") != strings.Join(want, ",") || resumed.Calls != 6 {
		t.Errorf("after going on: %v (%d counted); want the decision, the document and the work list, and the three looks at it", asked, resumed.Calls)
	}
	// The person is told what was kept and what was asked.
	progress := strings.Join(told, "\n")
	for _, want := range []string{"Agenda: 2 topics, from " + plan.Dir() + "/" + AgendaFile, "  topic scope owner proposes: kept from the earlier run",
		"  topic build moderator sums up: kept from the earlier run", "  topic build decision (0s)", "  owner rules: 1 decided, 0 assumed until the owner answers"} {
		if !strings.Contains(progress, want) {
			t.Errorf("going on should say %q:\n%s", want, progress)
		}
	}
	if strings.Contains(progress, "topic build decision: kept") || strings.Contains(progress, "the moderator sets it") {
		t.Errorf("going on says it kept what failed, or set the agenda again:\n%s", progress)
	}
	if !strings.Contains(readFile(t, root, plan.Dir()+"/minutes.md"), "Status: finished.") {
		t.Error("the minutes still say the meeting is not finished")
	}

	// The record of the calls only grows: the calls of the second run are
	// numbered after those of the first, and none is written over.
	records, _ := filepath.Glob(filepath.Join(root, ".ai", "state", "council", plan.ID, "calls", "*.md"))
	if want := before + len(asked); len(records) != want {
		t.Errorf("%d calls on record after going on, want %d", len(records), want)
	}
	numbers := map[string]bool{}
	for _, record := range records {
		number := strings.SplitN(filepath.Base(record), "-", 2)[0]
		if numbers[number] {
			t.Errorf("two calls on record have the number %s: %v", number, records)
		}
		numbers[number] = true
	}

	// Going on with a finished plan asks nothing and records its decisions once.
	before = len(model.steps())
	if _, err := Run(context.Background(), Options{Root: root, ID: plan.ID, Roster: roster, Model: model, Repos: testRepos, Now: fixedNow}); err != nil {
		t.Fatalf("going on with a finished plan: %v", err)
	}
	if again := model.steps()[before:]; len(again) != 0 {
		t.Errorf("a finished plan asked again: %v", again)
	}
	log := readFile(t, root, DecisionsPath)
	for _, marker := range []string{planMarker(plan.ID), topicMarker(plan.ID, "scope"), topicMarker(plan.ID, "build")} {
		if got := strings.Count(log, marker); got != 1 {
			t.Errorf("%s is in the decision log %d times, want once:\n%s", marker, got, log)
		}
	}
	if !strings.Contains(log, "- **D1 scope**") || !strings.Contains(log, "- **D2 format**") || strings.Contains(log, "- **D3") {
		t.Errorf("the decisions are not named D1 and D2 once each:\n%s", log)
	}
}

// A plan makes no more model calls than its roster allows. When the agenda
// cannot be held within them, the meeting says so before the first topic;
// when it runs out all the same, it stops where it ran out.
func TestRun_StopsWhenTheCallsAreUsedUp(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, strings.Replace(testRoster, "calls: 60", "calls: 10", 1))
	model := &table{answer: agreeable(roster)}

	plan, err := runPlan(t, root, roster, model, "Add a health endpoint.")

	// Topic 1: lead, two speakers, summary, decision, three owners; topic 2:
	// five and one owner; the work list and three looks at it: 18.
	if err == nil || !strings.Contains(err.Error(), "the agenda needs at least 18 more model calls and the roster allows 9 more: raise limits.calls in the roster, or shorten the agenda in docs/plans/"+plan.ID+"/agenda.yaml, then go on with `awkit plan --resume "+plan.ID+"`") {
		t.Fatalf("err = %v, want the agenda said to be too long for the calls", err)
	}
	if got := model.steps(); len(got) != 1 || got[0] != "moderator agenda" {
		t.Errorf("calls made before the meeting said so: %v", got)
	}
	if !exists(root, plan.Dir()+"/"+AgendaFile) {
		t.Error("the agenda is not there to be shortened")
	}

	// Enough for the agenda as it was set, not for the disputes it ran into.
	roster = mustRoster(t, strings.Replace(testRoster, "calls: 60", "calls: 20", 1))
	model = &table{answer: disputing(roster)}
	plan, err = runPlan(t, t.TempDir(), roster, model, "Add a health endpoint.")
	if !errors.Is(err, ErrBudget) {
		t.Fatalf("err = %v, want ErrBudget", err)
	}
	if len(model.steps()) != 20 || plan.Calls != 20 {
		t.Errorf("%d calls made, %d counted; the roster allows 20", len(model.steps()), plan.Calls)
	}
}

// A plan that goes on counts the calls the topics it has still to hold need,
// not the ones held in an earlier run: a roster with just enough calls left
// for the rest holds it.
func TestRun_GoingOnCountsOnlyWhatIsLeft(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, testRoster)
	base := agreeable(roster)
	broken := true
	model := &table{answer: func(c call) (string, error) {
		if broken && c.Step == "propose" && c.Topic == 2 {
			return "", errors.New("claude failed: exit status 1")
		}
		return base(c)
	}}
	plan, err := runPlan(t, root, roster, model, "Add a health endpoint.")
	if err == nil || !plan.Topics[0].Held || plan.Topics[1].Held {
		t.Fatalf("the first run: err %v; want it stopped at the second topic, the first one held", err)
	}

	// Topic 2: the lead, two speakers, the summary, the decision and one
	// owner; the work list and three looks at it: 10. Topic 1 is held.
	broken = false
	tight := mustRoster(t, strings.Replace(testRoster, "calls: 60", "calls: 10", 1))
	model = &table{answer: agreeable(tight)}
	resumed, err := Run(context.Background(), Options{Root: root, ID: plan.ID, Roster: tight, Model: model, Repos: testRepos, Now: fixedNow})
	if err != nil {
		t.Fatalf("going on with the calls the rest needs: %v", err)
	}
	if !resumed.Topics[1].Held || len(resumed.Tickets) == 0 || resumed.Calls != 10 {
		t.Errorf("going on: topic 2 held %v, %d tickets, %d calls; want it held in 10 calls", resumed.Topics[1].Held, len(resumed.Tickets), resumed.Calls)
	}
}

// A model that fails stops the meeting with the topic and the step it failed at.
func TestRun_AModelFailureStopsTheMeeting(t *testing.T) {
	roster := mustRoster(t, testRoster)
	base := agreeable(roster)
	model := &table{answer: func(c call) (string, error) {
		if c.Step == "summary" {
			return "", errors.New("claude failed: exit status 1")
		}
		return base(c)
	}}

	_, err := runPlan(t, t.TempDir(), roster, model, "Add a health endpoint.")

	if err == nil || !strings.Contains(err.Error(), "the meeting stopped at topic 1 (scope): topic scope moderator sums up: ") || !strings.Contains(err.Error(), "exit status 1") {
		t.Errorf("err = %v", err)
	}
}

// The table objects to the work list once, and the planning role takes the
// objections in. An objection is about a ticket or a milestone of the list.
func TestRun_TheTableObjectsToTheWorkList(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, testRoster)
	base := agreeable(roster)
	reviewed := 0
	model := &table{answer: func(c call) (string, error) {
		switch {
		case c.Step == "review" && c.Role == "skeptic":
			reviewed++
			if reviewed == 1 {
				return block(`{"objections": [{"ticket": "t1, t9", "point": "No failure test.", "wants": "A ticket for it."}]}`), nil
			}
			return block(`{"objections": [{"ticket": "t2", "point": "No failure test.", "wants": "A criterion for the database being down."}]}`), nil
		case c.Step == "review" && c.Role == "designer":
			// One objection about two tickets, written loosely, and one about a milestone.
			return block(`{"objections": [{"ticket": " t1 ,t2, ", "point": "Nobody is told when it fails.", "wants": "An error a person can read."}, {"ticket": "m2", "point": "Nobody watches it.", "wants": "Watching in m1."}]}`), nil
		case c.Step == "revise":
			return block(`{"milestones": [{"key": "m1", "title": "The endpoint answers", "goal": "It works.", "done_when": ["GET /healthz answers 200"]}],
			"tickets": [
				{"key": "t1", "title": "[chore] set up the module", "repo": "backend", "summary": "The Go module.", "scope": ["go.mod"], "acceptance": ["go build passes"], "testing": []},
				{"key": "t2", "title": "[feat] serve the health endpoint", "repo": "backend", "summary": "GET /healthz.", "scope": ["handler"], "acceptance": ["GET /healthz answers 200", "answers 503 when the database is down"], "testing": ["a handler test"], "depends_on": ["t1"]}
			], "later": [], "kept": ["watching stays in m2: the first milestone is the endpoint"]}`), nil
		}
		return base(c)
	}}

	plan, err := runPlan(t, root, roster, model, "Add a health endpoint.")

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if reviewed != 2 {
		t.Errorf("the skeptic reviewed %d times; an objection to a ticket that is not there is asked for again", reviewed)
	}
	if got := model.count("builder", "revise"); got != 1 {
		t.Errorf("the list was revised %d times, want once", got)
	}
	if len(plan.Tickets) != 2 || len(plan.Tickets[1].Acceptance) != 2 || len(plan.Milestones) != 1 {
		t.Errorf("the revised list was not taken: %+v, %+v", plan.Milestones, plan.Tickets)
	}
	revise := model.promptOf("builder", "revise")
	for _, want := range []string{"- skeptic on t2: No failure test. Wants: A criterion for the database being down.", "- designer on t1, t2: Nobody is told when it fails. Wants: An error a person can read.",
		"- designer on m2: Nobody watches it.", "Your milestones:\n\nm1  The endpoint answers (ticketed below)\n", "m2  It is watched\n"} {
		if !strings.Contains(revise, want) {
			t.Errorf("the planning role was not shown %q:\n%s", want, revise)
		}
	}
	// What the role was told about the ticket that is not there: its name alone.
	if again := model.promptOf("skeptic", "review"); !strings.Contains(again, `an objection names "t9", which is neither a ticket nor a milestone of the list`) {
		t.Errorf("the role is not told which ticket is not in the list:\n%s", again[max(0, len(again)-300):])
	}
	review := model.promptOf("owner", "review")
	for _, want := range []string{"The milestones builder laid out:\n\nm1  The endpoint answers (ticketed below)\n  The smallest thing that shows it works.\n  Reached when:\n    - GET /healthz answers 200\n", "The tickets of the first milestone:\n\nt2  [feat] serve the health endpoint"} {
		if !strings.Contains(review, want) {
			t.Errorf("those who look at the list should be shown %q:\n%s", want, review)
		}
	}
	minutes := readFile(t, root, plan.Dir()+"/minutes.md")
	for _, want := range []string{"- skeptic on t2: No failure test.", "Not taken in, and why:\n- watching stays in m2"} {
		if !strings.Contains(minutes, want) {
			t.Errorf("the minutes should say %q:\n%s", want, minutes)
		}
	}
}

// What a role is told: who it is, what it stands for, who else is there, the
// project, the documents and whose they are, what is already decided, the
// brief, and the topic it speaks to.
func TestPrompts_WhatARoleIsTold(t *testing.T) {
	root := t.TempDir()
	putFile(t, root, "docs/qa/risks.md", "# Risks\n\nThe database can be down.\n")
	putFile(t, root, "docs/decisions.md", "# Decisions\n\n1. **stack**: Go.\n")
	roster := mustRoster(t, testRoster)
	model := &table{answer: agreeable(roster)}

	if _, err := runPlan(t, root, roster, model, "加入健康檢查端點。"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	speak := model.promptIn(1, "skeptic", "speak")
	for _, want := range []string{
		"You are the Skeptic (skeptic) at a planning meeting for a software project. The meeting turns a brief into documents, decisions and a list of tickets, one topic at a time.",
		"  You look for how it fails.",
		"- owner (Product Owner) - decides what stays disputed",
		"- skeptic (Skeptic) - you",
		"- backend (go, in backend/)",
		"- root\n",
		"The project's documents. Each has one owner, the role that writes it, and is kept where the roster names it; nothing said or decided at this meeting moves one, merges two or gives one to another role. A folder inside another role's folder is not that role's:\n- docs/product/requirements.md (owner, not written yet)\n- docs/product/journeys.md (designer, not written yet)\n- docs/tech/architecture.md (builder, not written yet)\n- docs/qa/risks.md (you)\nYou write the ones that are yours, and no other.\n",
		"Decisions on record from earlier plans (docs/decisions.md):\n# Decisions\n\n1. **stack**: Go.",
		"<brief>\n加入健康檢查端點。\n</brief>",
		"Write in the language the brief is written in.",
		"The documents of this topic, as they stand. Read them before you answer:\n===== docs/qa/risks.md (skeptic's)\n# Risks\n\nThe database can be down.\n",
	} {
		if !strings.Contains(speak, want) {
			t.Errorf("the prompt should say %q:\n%s", want, speak)
		}
	}
	// What a role writes, it is shown as it stands.
	if documents := model.promptIn(1, "skeptic", "documents"); !strings.Contains(documents, "Your documents of this topic as they stand:\n===== docs/qa/risks.md\n# Risks\n\nThe database can be down.\n") ||
		!strings.Contains(documents, "Your task now: write your documents of this topic, as they read after it:\n- docs/qa/risks.md\n\n") ||
		!strings.Contains(documents, "<<<UNCHANGED path/of/the/document.md>>>") {
		t.Errorf("the role is not shown its document of the topic as it stands:\n%s", documents)
	}
	if documents := model.promptIn(1, "owner", "documents"); !strings.Contains(documents, "- docs/product/requirements.md: not written yet\n") || strings.Contains(documents, "<<<UNCHANGED") {
		t.Errorf("a role with nothing written is offered to leave it unchanged:\n%s", documents)
	}
	// Those who look at the work list are told that the documents are written
	// and what is no ticket; so is whoever draws it up.
	for _, c := range []struct{ role, step string }{{"skeptic", "review"}, {"builder", "worklist"}} {
		prompt := model.promptOf(c.role, c.step)
		for _, want := range []string{"The documents are written: no ticket writes or rewrites one of them.", "What waits for an answer from the owner of the project is not a ticket either."} {
			if !strings.Contains(prompt, want) {
				t.Errorf("%s %s should be told %q:\n%s", c.role, c.step, want, prompt)
			}
		}
	}
	if review := model.promptOf("skeptic", "review"); !strings.Contains(review, "The documents, written by their owners:\n- docs/product/requirements.md (owner: owner; written in this meeting)\n") {
		t.Errorf("the review does not list the documents:\n%s", review)
	}
	// The moderator is shown the table and what each role writes.
	agenda := model.promptOf("moderator", "agenda")
	for _, want := range []string{"Before the meeting you set its agenda", "- owner (Product Owner) - decides what stays disputed\n  Stands for: You answer for what is built.\n  Writes: docs/product/requirements.md\n",
		"- builder (Builder) - draws up the work list\n", "The project's documents as they stand:\n- docs/qa/risks.md (skeptic)\n", "At most 8 topics.", "at most 7, each answerable"} {
		if !strings.Contains(agenda, want) {
			t.Errorf("the moderator should be told %q:\n%s", want, agenda)
		}
	}

	// A brief with no Latin letters still gets a name.
	if id := newID(root, "加入健康檢查端點。", fixedNow()); !strings.HasPrefix(id, "20261114-plan") {
		t.Errorf("plan id = %q", id)
	}

	// The roster can fix the language.
	roster.Language = "繁體中文"
	model = &table{answer: agreeable(roster)}
	if _, err := runPlan(t, t.TempDir(), roster, model, "Add a health endpoint."); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, c := range model.calls {
		if !strings.Contains(c.Prompt, "Write everything in 繁體中文.") {
			t.Errorf("%s %s is not told the language:\n%s", c.Role, c.Step, c.Prompt[:min(300, len(c.Prompt))])
		}
	}
}

// The decision log only grows, and the index keeps what a person wrote around
// its own part.
func TestRun_KeepsWhatIsAlreadyThere(t *testing.T) {
	root := t.TempDir()
	earlier := "# Decisions\n\n## 2026-10-01, plan 20261001-first\n\n<!-- plan: 20261001-first -->\n\n1. **stack**: Go and Godot.\n"
	putFile(t, root, "docs/decisions.md", earlier)
	readme := "# Our documents\n\nWritten by hand.\n"
	putFile(t, root, "docs/README.md", readme)
	roster := mustRoster(t, testRoster)

	first, err := runPlan(t, root, roster, &table{answer: agreeable(roster)}, "Add a health endpoint.")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	second, err := runPlan(t, root, roster, &table{answer: agreeable(roster)}, "Add a health endpoint.")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if second.ID != first.ID+"-2" {
		t.Errorf("the second plan of the day with the same words is %q, want %q", second.ID, first.ID+"-2")
	}
	decisions := readFile(t, root, DecisionsPath)
	if !strings.HasPrefix(decisions, strings.TrimRight(earlier, "\n")) {
		t.Errorf("the earlier decisions were changed:\n%s", decisions)
	}
	if strings.Index(decisions, "plan "+first.ID+"\n") > strings.Index(decisions, "plan "+second.ID+"\n") {
		t.Errorf("the decisions are not in the order they were made:\n%s", decisions)
	}
	if !strings.Contains(decisions, "- **D3 scope**") {
		t.Errorf("the second plan does not count on from the first:\n%s", decisions)
	}
	index := readFile(t, root, "docs/README.md")
	if !strings.HasPrefix(index, "# Our documents\n\nWritten by hand.\n") {
		t.Errorf("what was written by hand in the index is gone:\n%s", index)
	}
	if strings.Count(index, indexOpen) != 1 || strings.Count(index, indexClose) != 1 {
		t.Errorf("the index has its part more than once:\n%s", index)
	}
	for _, id := range []string{first.ID, second.ID} {
		if !strings.Contains(index, "plans/"+id+"/minutes.md") {
			t.Errorf("the index does not list plan %s:\n%s", id, index)
		}
	}
	if strings.Index(index, "plans/"+second.ID+"/minutes.md") > strings.Index(index, "plans/"+first.ID+"/minutes.md") {
		t.Errorf("the index does not list the newest plan first:\n%s", index)
	}
	// What the second plan is told of earlier plans: the first one's
	// decisions, not its own.
	model := &table{answer: agreeable(roster)}
	third, err := runPlan(t, root, roster, model, "Add metrics.")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	topic2 := model.promptIn(2, "builder", "propose")
	if !strings.Contains(topic2, "plan "+second.ID) || strings.Contains(topic2, "plan "+third.ID) || !strings.Contains(topic2, "What this meeting has decided so far:\nD5. scope") {
		t.Errorf("a topic is not told the earlier plans' decisions apart from this one's:\n%s", topic2)
	}
}

func TestRun_RefusesWhatItCannotStartFrom(t *testing.T) {
	roster := mustRoster(t, testRoster)
	model := &table{answer: agreeable(roster)}
	tests := []struct {
		name        string
		opts        Options
		wantInError string
	}{
		{"no roster", Options{Brief: "x", Repos: testRepos}, "needs a roster"},
		{"no brief", Options{Roster: roster, Repos: testRepos}, "needs a brief"},
		{"no repos", Options{Roster: roster, Brief: "x"}, "at least one repo"},
		{"a plan that does not exist", Options{Roster: roster, Repos: testRepos, ID: "20260101-nothing"}, "cannot be gone on with"},
		{"a name that is not a plan's", Options{Roster: roster, Repos: testRepos, ID: "../../etc"}, "is not the name of a plan"},
	}
	for _, tt := range tests {
		tt.opts.Root, tt.opts.Model, tt.opts.Now = t.TempDir(), model, fixedNow
		_, err := Run(context.Background(), tt.opts)
		if err == nil || !strings.Contains(err.Error(), tt.wantInError) {
			t.Errorf("%s: err = %v, want it to mention %q", tt.name, err, tt.wantInError)
		}
	}
}
