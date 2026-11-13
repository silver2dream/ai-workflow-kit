package council

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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

func mustRoster(t *testing.T, text string) *Roster {
	t.Helper()
	roster, err := ParseRoster([]byte(text))
	if err != nil {
		t.Fatalf("roster: %v", err)
	}
	return roster
}

var (
	promptRole = regexp.MustCompile(`^You are the .+? \(([a-z0-9-]+)\) at a planning meeting`)
	promptStep = []struct{ marker, step string }{
		{"state your position on the brief", "position"},
		{"Put them to the owner as one list", "questions"},
		{"Your task now: object. Read the others' positions", "objections"},
		{"pick the ", "pick"},
		{"Your task now: answer the objection", "defence"},
		{"say whether the last answer settles your objection", "reply"},
		{"Say where the meeting stands", "summary"},
		{"Your task now: decide", "decide"},
		{"write the documents you own", "documents"},
		{"Your task now: turn the plan into tickets", "worklist"},
		{"Your task now: object to the work list", "review"},
		{"Your task now: revise the list", "revise"},
	}
)

// call is one question the table was asked.
type call struct {
	Role, Step, Prompt string
}

// table plays every role from a script: a function from who is asked what to
// the answer. It records the calls in the order they were made.
type table struct {
	mu     sync.Mutex
	calls  []call
	answer func(c call) (string, error)
}

func (m *table) Ask(ctx context.Context, model, prompt string) (string, error) {
	c := call{Role: "moderator", Prompt: prompt}
	if match := promptRole.FindStringSubmatch(prompt); match != nil {
		c.Role = match[1]
	}
	for _, known := range promptStep {
		if strings.Contains(prompt, known.marker) {
			c.Step = known.step
			break
		}
	}
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

func block(json string) string { return "Here it is.\n```json\n" + json + "\n```\n" }

func docs(role string, paths ...string) string {
	var b strings.Builder
	for _, path := range paths {
		fmt.Fprintf(&b, "<<<DOCUMENT %s>>>\n---\nstatus: draft\nowner: %s\n---\n# %s\n\nWritten by %s.\n<<<END>>>\n", path, role, path, role)
	}
	return b.String()
}

const goodTickets = `{"tickets": [
  {"key": "t2", "title": "[feat] serve the health endpoint", "repo": "backend", "summary": "GET /healthz.", "scope": ["handler"], "acceptance": ["GET /healthz answers 200"], "testing": ["a handler test"], "reads": ["docs/tech/architecture.md"], "depends_on": ["t1"]},
  {"key": "t1", "title": "[chore] set up the module", "repo": "backend", "summary": "The Go module.", "scope": ["go.mod"], "acceptance": ["go build passes"], "testing": [], "depends_on": []}
], "later": ["metrics"]}`

// agreeable answers every question in the simplest acceptable way: positions,
// no objections, a decision, the owned documents, a work list nobody objects to.
func agreeable(roster *Roster) func(c call) (string, error) {
	return func(c call) (string, error) {
		switch c.Step {
		case "position":
			return block(fmt.Sprintf(`{"summary": "%s holds its line.", "proposals": ["%s proposes"], "concerns": ["%s worries"], "questions": []}`, c.Role, c.Role, c.Role)), nil
		case "objections", "review":
			return block(`{"objections": []}`), nil
		case "summary":
			return block(`{"consensus": ["build the small thing"], "open": [], "questions": []}`), nil
		case "decide":
			return block(`{"decisions": [{"title": "scope", "decision": "Build the health endpoint only.", "rationale": "It is the smallest thing worth shipping.", "dissent": ""}]}`), nil
		case "documents":
			return docs(c.Role, roster.Role(c.Role).Owns...), nil
		case "worklist", "revise":
			return block(goodTickets), nil
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

// A meeting where nobody objects: every role is asked for its position, for
// its objections, for its documents; the moderator sums up, the deciding role
// decides, the planning role draws up the list and the others look at it.
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
	// Who was asked what, and how often.
	for _, role := range roster.IDs() {
		for _, step := range []string{"position", "objections", "documents"} {
			if got := model.count(role, step); got != 1 {
				t.Errorf("%s was asked for its %s %d times, want once", role, step, got)
			}
		}
	}
	for step, want := range map[string]int{"moderator summary": 1, "moderator pick": 0, "owner decide": 1, "builder worklist": 1, "builder revise": 0,
		"owner review": 1, "designer review": 1, "skeptic review": 1, "builder review": 0} {
		parts := strings.SplitN(step, " ", 2)
		if got := model.count(parts[0], parts[1]); got != want {
			t.Errorf("%s: %d calls, want %d", step, got, want)
		}
	}
	if want := 4 + 4 + 1 + 1 + 4 + 1 + 3; plan.Calls != want || len(model.steps()) != want {
		t.Errorf("%d calls counted, %d made, want %d", plan.Calls, len(model.steps()), want)
	}
	// The order of play: nobody objects before every position is in, nobody
	// decides before the summary, no ticket before the documents.
	order := map[string]int{}
	for i, step := range model.steps() {
		kind := strings.SplitN(step, " ", 2)[1]
		if _, seen := order[kind]; !seen {
			order[kind] = i
		}
		order["last "+kind] = i
	}
	for _, pair := range [][2]string{{"last position", "objections"}, {"last objections", "summary"}, {"summary", "decide"},
		{"decide", "documents"}, {"last documents", "worklist"}, {"worklist", "review"}} {
		if order[pair[0]] >= order[pair[1]] {
			t.Errorf("%q (call %d) should come before %q (call %d): %v", pair[0], order[pair[0]], pair[1], order[pair[1]], model.steps())
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
	minutes := readFile(t, root, plan.Dir()+"/minutes.md")
	for _, want := range []string{"Status: finished. 2 tickets proposed", "### owner (Product Owner)", "skeptic holds its line.",
		"Build the health endpoint only.", "`docs/qa/risks.md` (skeptic)", "| t1 | [chore] set up the module | backend | - |",
		"| t2 | [feat] serve the health endpoint | backend | t1 |", "awkit plan approve " + plan.ID, "Left for a later plan:\n- metrics"} {
		if !strings.Contains(minutes, want) {
			t.Errorf("the minutes should say %q:\n%s", want, minutes)
		}
	}
	decisions := readFile(t, root, DecisionsPath)
	for _, want := range []string{"# Decisions", "## 2026-11-14, plan " + plan.ID, "<!-- plan: " + plan.ID + " -->",
		"Decided by owner. At the table: owner, designer, builder, skeptic.", "- **D1 scope**: Build the health endpoint only.\n  - Why: It is the smallest thing worth shipping.\n"} {
		if !strings.Contains(decisions, want) {
			t.Errorf("the decision log should say %q:\n%s", want, decisions)
		}
	}
	index := readFile(t, root, "docs/README.md")
	for _, want := range []string{"| [docs/qa/risks.md](qa/risks.md) | skeptic | draft |", "| [" + plan.ID + "](plans/" + plan.ID + "/minutes.md) | 2 tickets, 0 on GitHub |"} {
		if !strings.Contains(index, want) {
			t.Errorf("the index should say %q:\n%s", want, index)
		}
	}

	// The work list: in an order that can be worked, nothing on GitHub.
	list, err := LoadWorkList(root, plan.ID)
	if err != nil {
		t.Fatalf("LoadWorkList: %v", err)
	}
	if len(list.Tickets) != 2 || list.Tickets[0].Key != "t1" || list.Tickets[1].Key != "t2" || list.Tickets[0].Issue != 0 || list.Tickets[1].Issue != 0 {
		t.Errorf("work list = %+v, want t1 before t2 and no issue yet", list.Tickets)
	}
	if !strings.Contains(readFile(t, root, plan.Dir()+"/"+WorkListFile), "awkit plan approve "+plan.ID) {
		t.Error("the work list does not say how to approve it")
	}
	// Every prompt and answer is kept, outside the documents.
	recorded, _ := filepath.Glob(filepath.Join(root, ".ai", "state", "council", plan.ID, "calls", "*.md"))
	if len(recorded) != plan.Calls {
		t.Errorf("%d calls on record, want %d", len(recorded), plan.Calls)
	}
}

// disputing is the agreeable table with two objections: the skeptic's against
// the owner, which the owner concedes at once, and the designer's against the
// builder, who holds both rounds.
func disputing(roster *Roster) func(c call) (string, error) {
	base := agreeable(roster)
	return func(c call) (string, error) {
		switch {
		case c.Step == "objections" && c.Role == "skeptic":
			return block(`{"objections": [{"to": "owner", "point": "No failure is named.", "why": "Nobody would know it is broken.", "wants": "A failure case in the requirements."}]}`), nil
		case c.Step == "objections" && c.Role == "designer":
			return block(`{"objections": [{"to": "builder", "point": "The endpoint returns JSON only.", "why": "People read it in a browser.", "wants": "A plain text answer too."}]}`), nil
		case c.Step == "defence" && c.Role == "owner":
			return block(`{"stance": "concede", "reply": "Right. I add it.", "position": "The requirements name what happens when the database is down."}`), nil
		case c.Step == "defence" && c.Role == "builder":
			return block(`{"stance": "Hold", "reply": "Monitoring reads JSON.", "position": "JSON only."}`), nil
		case c.Step == "reply" && c.Role == "designer":
			return block(`{"accepts": false, "reply": "People still read it.", "wants": "Text for browsers."}`), nil
		case c.Step == "summary":
			return block(`{"consensus": ["a health endpoint"], "open": ["designer wants plain text, builder holds JSON only"], "questions": []}`), nil
		case c.Step == "decide":
			return block(`{"decisions": [{"title": "format", "decision": "JSON only.", "rationale": "Its reader is a monitor.", "dissent": "designer wanted plain text for browsers"}]}`), nil
		}
		return base(c)
	}
}

// An objection is argued out between the two roles: the defender answers, the
// challenger says whether that settles it, and what is not settled when the
// rounds are used up goes to the deciding role.
func TestRun_DisputesAreArguedOut(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, testRoster)
	model := &table{answer: disputing(roster)}

	plan, err := runPlan(t, root, roster, model, "Add a health endpoint.")

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(plan.Disputes) != 2 {
		t.Fatalf("%d disputes, want 2: %+v", len(plan.Disputes), plan.Disputes)
	}
	// conceded: the skeptic against the owner; held: the designer against the builder.
	var conceded, held Dispute
	for _, dispute := range plan.Disputes {
		switch dispute.Challenger {
		case "skeptic":
			conceded = dispute
		case "designer":
			held = dispute
		}
	}
	if conceded.Defender != "owner" || held.Defender != "builder" {
		t.Fatalf("the disputes are not between the roles that objected: %+v", plan.Disputes)
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
	var decide string
	for _, c := range model.calls {
		if c.Step == "decide" {
			decide = c.Prompt
		}
	}
	if !strings.Contains(decide, "designer against builder") || !strings.Contains(decide, "People still read it.") {
		t.Errorf("the deciding role was not shown the open dispute:\n%s", decide)
	}
	if strings.Contains(decide, "skeptic against owner") {
		t.Errorf("the deciding role was asked to rule on a settled dispute:\n%s", decide)
	}
	// A role remembers nothing: whoever defends is told what it held.
	if defence := model.promptOf("owner", "defence"); !strings.Contains(defence, "Your own position:\nowner holds its line.\nYou proposed:\n  - owner proposes\n") {
		t.Errorf("the defender is not reminded of its own position:\n%s", defence)
	}
	if reply := model.promptOf("designer", "reply"); !strings.Contains(reply, "Your own position:\ndesigner holds its line.") {
		t.Errorf("the challenger is not reminded of its own position:\n%s", reply)
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

	minutes := readFile(t, root, plan.Dir()+"/minutes.md")
	for _, want := range []string{"skeptic against owner: **settled**", "designer against builder: **not settled**", "Round 2, builder (hold)",
		"not settled after 2 round(s); it goes to owner", "Held otherwise: designer wanted plain text for browsers"} {
		if !strings.Contains(minutes, want) {
			t.Errorf("the minutes should say %q:\n%s", want, minutes)
		}
	}
	if !strings.Contains(readFile(t, root, DecisionsPath), "- **D1 format**: JSON only.\n  - Why: Its reader is a monitor.\n  - Held otherwise: designer wanted plain text for browsers\n") {
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
	for _, dispute := range plan.Disputes {
		if !dispute.Settled {
			t.Errorf("dispute %s against %s is not settled", dispute.Challenger, dispute.Defender)
		}
		if dispute.Challenger == "designer" && dispute.Outcome != "designer accepted builder's answer in round 1" {
			t.Errorf("outcome = %q", dispute.Outcome)
		}
	}
	if got := model.count("builder", "defence"); got != 1 {
		t.Errorf("the defender was asked %d times after the challenger accepted, want 1", got)
	}
}

// With more objections than the table argues out, the moderator picks; with
// fewer, every one is argued and the moderator is not asked.
func TestRun_TheModeratorPicksWhenThereAreTooManyObjections(t *testing.T) {
	roster := mustRoster(t, testRoster)
	base := agreeable(roster)
	model := &table{answer: func(c call) (string, error) {
		switch c.Step {
		case "objections":
			to := "owner"
			if c.Role == "owner" {
				to = "builder"
			}
			return block(fmt.Sprintf(`{"objections": [{"to": "%s", "point": "%s objects", "why": "w", "wants": "x"}]}`, to, c.Role)), nil
		case "pick":
			return block(`{"disputes": [{"topic": "one", "challenger": "skeptic", "defender": "owner", "point": "skeptic objects", "wants": "x"},
				{"topic": "two", "challenger": "owner", "defender": "builder", "point": "owner objects", "wants": "x"},
				{"topic": "three", "challenger": "designer", "defender": "owner", "point": "designer objects", "wants": "x"}]}`), nil
		case "defence":
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
	if len(plan.Objections) != 4 || len(plan.Disputes) != 2 {
		t.Errorf("%d objections, %d disputes; want 4 and 2", len(plan.Objections), len(plan.Disputes))
	}
	for _, dispute := range plan.Disputes {
		if dispute.Challenger == "designer" {
			t.Error("a third dispute was argued out; the roster allows two")
		}
	}
	// The moderator's prompt takes no side and carries every objection.
	for _, c := range model.calls {
		if c.Step == "pick" {
			for _, want := range []string{"You have no opinion on the subject", "skeptic objects to owner", "owner objects to builder", "pick the 2 "} {
				if !strings.Contains(c.Prompt, want) {
					t.Errorf("the moderator's prompt should say %q:\n%s", want, c.Prompt)
				}
			}
		}
	}
}

// A role gets to write the documents it owns and no other. What it hands in
// beyond that is left out, and the minutes say so.
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
	minutes := readFile(t, root, plan.Dir()+"/minutes.md")
	if !strings.Contains(minutes, "`docs/product/requirements.md` handed in by skeptic: **not written**, skeptic does not own this document") {
		t.Errorf("the minutes do not say that a document was refused:\n%s", minutes)
	}
}

// An answer that cannot be used is asked for once more, with what was wrong
// with it. The role is told; nothing is guessed.
func TestRun_AnAnswerThatCannotBeUsedIsAskedForAgain(t *testing.T) {
	roster := mustRoster(t, testRoster)
	tests := []struct {
		name, role, step, bad, wantTold string
	}{
		{"no JSON", "designer", "position", "I think we should build it.", "holds no JSON object"},
		{"broken JSON", "designer", "position", "```json\n{\"summary\": \n```", "cannot be read"},
		{"a position without a summary", "designer", "position", block(`{"summary": " ", "proposals": []}`), "the position has no summary"},
		{"an objection to oneself", "skeptic", "objections", block(`{"objections": [{"to": "skeptic", "point": "p"}]}`), "addressed to yourself"},
		{"an objection to nobody at the table", "skeptic", "objections", block(`{"objections": [{"to": "cfo", "point": "p"}]}`), `"cfo", who is not at the table (the roles are: owner, designer, builder, skeptic)`},
		{"a decision that decides nothing", "owner", "decide", block(`{"decisions": []}`), "nothing was decided"},
		{"an assumption that assumes nothing", "owner", "decide", block(`{"decisions": [{"title": "t", "decision": "d"}], "assumptions": [{"question": "Who calls it?", "assumption": " "}]}`), "an assumption does not say what is assumed"},
		{"a document that is not written, left unchanged", "builder", "documents", "<<<UNCHANGED docs/tech/architecture.md>>>\n", "docs/tech/architecture.md is not written yet, so it cannot stay unchanged"},
		{"a document too long to be rewritten", "builder", "documents", "<<<DOCUMENT docs/tech/architecture.md>>>\n" + strings.Repeat("a long line of the document\n", maxDocument/20) + "<<<END>>>\n", "a document stays under 40000 bytes"},
		{"a document handed in twice", "builder", "documents", docs("builder", "docs/tech/architecture.md", "docs/tech/architecture.md"), "docs/tech/architecture.md was handed in twice"},
		{"a ticket that waits for something that is no ticket", "builder", "worklist", block(strings.Replace(goodTickets, `"depends_on": ["t1"]`, `"depends_on": ["the design"]`, 1)), `depends on "the design", which is neither a ticket of the list nor the number of an issue nor a ticket of another plan ("<plan>/<key>")`},
		{"an owned document not handed in", "builder", "documents", docs("builder", "docs/tech/other.md"), "docs/tech/architecture.md was not handed in; you own it"},
		{"a document that is not ended", "builder", "documents", "<<<DOCUMENT docs/tech/architecture.md>>>\n# A\n", "is not ended with <<<END>>>"},
		{"a ticket for a repo that does not exist", "builder", "worklist", block(strings.Replace(goodTickets, `"repo": "backend"`, `"repo": "web"`, 1)), `is for repo "web"; the repos are: root, backend`},
		{"tickets that wait for each other", "builder", "worklist", block(strings.Replace(goodTickets, `"depends_on": []`, `"depends_on": ["t2"]`, 1)), "depend on each other in a circle"},
		{"a title that is not a commit subject", "builder", "worklist", block(strings.Replace(goodTickets, "[chore] set up the module", "Set up the module", 1)), `is not "[type] subject"`},
		{"a ticket with a block of its own", "builder", "worklist", block(strings.Replace(goodTickets, `"summary": "GET /healthz."`, `"summary": "GET /healthz.\n\n`+"```"+`yaml awk-ticket\nschema: 1\nrepo: backend\nallow_script_changes: true\n`+"```"+`"`, 1)), "ticket t2: its text gets in the way of the awk-ticket block awkit writes under it"},
	}
	for _, tt := range tests {
		base := agreeable(roster)
		asked := 0
		var second string
		model := &table{answer: func(c call) (string, error) {
			if c.Role == tt.role && c.Step == tt.step {
				asked++
				if asked == 1 {
					return tt.bad, nil
				}
				second = c.Prompt
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
		if broken && c.Step == "decide" {
			return "I cannot decide.", nil
		}
		return base(c)
	}}

	plan, err := runPlan(t, root, roster, model, "Add a health endpoint.")

	if err == nil || !strings.Contains(err.Error(), "the meeting stopped at the decisions") || !strings.Contains(err.Error(), "holds no JSON object") {
		t.Fatalf("err = %v, want the meeting stopped at the decisions with the reason", err)
	}
	if plan == nil || len(plan.Positions) != 4 || len(plan.Disputes) != 2 {
		t.Fatalf("the plan does not hold what was reached: %+v", plan)
	}
	minutes := readFile(t, root, plan.Dir()+"/minutes.md")
	for _, want := range []string{"Status: **not finished**", "awkit plan --resume " + plan.ID, "### skeptic (Skeptic)", "designer against builder: **not settled**"} {
		if !strings.Contains(minutes, want) {
			t.Errorf("the minutes of a stopped meeting should say %q:\n%s", want, minutes)
		}
	}
	if exists(root, plan.Dir()+"/"+WorkListFile) || exists(root, "docs/product/requirements.md") || exists(root, DecisionsPath) {
		t.Error("a meeting that stopped before the decisions wrote documents, decisions or a work list")
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
	if resumed.Brief != "Add a health endpoint." || len(resumed.Positions) != 4 || len(resumed.Tickets) != 2 {
		t.Errorf("the resumed plan = brief %q, %d positions, %d tickets", resumed.Brief, len(resumed.Positions), len(resumed.Tickets))
	}
	asked := model.steps()[before:]
	for _, step := range asked {
		for _, settled := range []string{"position", "objections", "defence", "reply", "summary"} {
			if strings.HasSuffix(step, " "+settled) {
				t.Errorf("%q was asked again although it was settled; asked after going on: %v", step, asked)
			}
		}
	}
	if want := 1 + 4 + 1 + 3; len(asked) != want || resumed.Calls != want {
		t.Errorf("%d calls after going on (%d counted), want %d: %v", len(asked), resumed.Calls, want, asked)
	}
	// The person is told what was kept and what was asked.
	progress := strings.Join(told, "\n")
	for _, want := range []string{"  position owner: kept from the earlier run", "  moderator sums up: kept from the earlier run", "  decisions owner (0s)", "  1 decided, 0 assumed until the owner answers"} {
		if !strings.Contains(progress, want) {
			t.Errorf("going on should say %q:\n%s", want, progress)
		}
	}
	if strings.Contains(progress, "decisions owner: kept") {
		t.Errorf("the step that failed is said to be kept:\n%s", progress)
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
	if last := fmt.Sprintf("%03d", before+len(asked)); !numbers[last] || !numbers["001"] {
		t.Errorf("the calls on record are not numbered 001 to %s: %v", last, records)
	}

	// Going on with a finished plan asks nothing and records its decisions once.
	before = len(model.steps())
	if _, err := Run(context.Background(), Options{Root: root, ID: plan.ID, Roster: roster, Model: model, Repos: testRepos, Now: fixedNow}); err != nil {
		t.Fatalf("going on with a finished plan: %v", err)
	}
	if again := model.steps()[before:]; len(again) != 0 {
		t.Errorf("a finished plan asked again: %v", again)
	}
	if got := strings.Count(readFile(t, root, DecisionsPath), "<!-- plan: "+plan.ID+" -->"); got != 1 {
		t.Errorf("the plan's decisions are on record %d times, want once", got)
	}
}

// A plan makes no more model calls than its roster allows.
func TestRun_StopsWhenTheCallsAreUsedUp(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, strings.Replace(testRoster, "calls: 60", "calls: 6", 1))
	model := &table{answer: agreeable(roster)}

	plan, err := runPlan(t, root, roster, model, "Add a health endpoint.")

	if !errors.Is(err, ErrBudget) {
		t.Fatalf("err = %v, want ErrBudget", err)
	}
	if len(model.steps()) != 6 || plan.Calls != 6 {
		t.Errorf("%d calls made, %d counted; the roster allows 6", len(model.steps()), plan.Calls)
	}
	if !strings.Contains(readFile(t, root, plan.Dir()+"/minutes.md"), "Status: **not finished**") {
		t.Error("the minutes do not say the meeting is not finished")
	}
}

// A model that fails stops the meeting with the step it failed at.
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

	if err == nil || !strings.Contains(err.Error(), "the meeting stopped at the summary") || !strings.Contains(err.Error(), "exit status 1") {
		t.Errorf("err = %v", err)
	}
}

// The table objects to the work list once, and the planning role takes the
// objections in. An objection has to be about a ticket of the list.
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
			// One objection about two tickets, written loosely.
			return block(`{"objections": [{"ticket": " t1 ,t2, ", "point": "Nobody is told when it fails.", "wants": "An error a person can read."}]}`), nil
		case c.Step == "revise":
			return block(`{"tickets": [
				{"key": "t1", "title": "[chore] set up the module", "repo": "backend", "summary": "The Go module.", "scope": ["go.mod"], "acceptance": ["go build passes"], "testing": []},
				{"key": "t2", "title": "[feat] serve the health endpoint", "repo": "backend", "summary": "GET /healthz.", "scope": ["handler"], "acceptance": ["GET /healthz answers 200", "answers 503 when the database is down"], "testing": ["a handler test"], "depends_on": ["t1"]}
			], "later": [], "kept": ["metrics stay out: the decision is the endpoint only"]}`), nil
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
	if len(plan.Tickets) != 2 || len(plan.Tickets[1].Acceptance) != 2 {
		t.Errorf("the revised list was not taken: %+v", plan.Tickets)
	}
	for _, c := range model.calls {
		if c.Step != "revise" {
			continue
		}
		for _, want := range []string{"- skeptic on t2: No failure test. Wants: A criterion for the database being down.", "- designer on t1, t2: Nobody is told when it fails. Wants: An error a person can read."} {
			if !strings.Contains(c.Prompt, want) {
				t.Errorf("the planning role was not shown the objection %q:\n%s", want, c.Prompt)
			}
		}
	}
	// What the role was told about the ticket that is not there: its name alone.
	if again := model.promptOf("skeptic", "review"); !strings.Contains(again, `an objection names the ticket "t9", which is not in the list`) {
		t.Errorf("the role is not told which ticket is not in the list:\n%s", again[max(0, len(again)-300):])
	}
	minutes := readFile(t, root, plan.Dir()+"/minutes.md")
	for _, want := range []string{"- skeptic on t2: No failure test.", "Not taken in, and why:\n- metrics stay out"} {
		if !strings.Contains(minutes, want) {
			t.Errorf("the minutes should say %q:\n%s", want, minutes)
		}
	}
}

// What a role is told: who it is, what it stands for, who else is there, the
// project, its own documents, what is already decided, and the brief.
func TestPrompts_WhatARoleIsTold(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs", "qa"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "qa", "risks.md"), []byte("# Risks\n\nThe database can be down.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "decisions.md"), []byte("# Decisions\n\n1. **stack**: Go.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	roster := mustRoster(t, testRoster)
	model := &table{answer: agreeable(roster)}

	if _, err := runPlan(t, root, roster, model, "加入健康檢查端點。"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var position string
	for _, c := range model.calls {
		if c.Step == "position" && c.Role == "skeptic" {
			position = c.Prompt
		}
	}
	for _, want := range []string{
		"You are the Skeptic (skeptic) at a planning meeting",
		"  You look for how it fails.",
		"- owner (Product Owner) - decides what stays disputed",
		"- skeptic (Skeptic) - you",
		"- backend (go, in backend/)",
		"- root\n",
		"The documents you own. Nobody else writes them, and you write no other:\n- docs/qa/risks.md\n",
		"Your documents as they stand:\n===== docs/qa/risks.md\n# Risks\n\nThe database can be down.\n",
		"Decisions already on record (docs/decisions.md):\n# Decisions\n\n1. **stack**: Go.",
		"<brief>\n加入健康檢查端點。\n</brief>",
		"Write in the language the brief is written in.",
	} {
		if !strings.Contains(position, want) {
			t.Errorf("the prompt should say %q:\n%s", want, position)
		}
	}
	// What the role owns is named in every question; what it says is shown
	// where the role has to have read it: for its position and for writing.
	for step, wantContent := range map[string]bool{"position": true, "objections": false, "documents": true, "review": false} {
		prompt := model.promptOf("skeptic", step)
		if !strings.Contains(prompt, "The documents you own.") {
			t.Errorf("%s: the role is not told what it owns", step)
		}
		if got := strings.Contains(prompt, "The database can be down."); got != wantContent {
			t.Errorf("%s: shows the document's content = %v, want %v", step, got, wantContent)
		}
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
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	earlier := "# Decisions\n\n## 2026-10-01, plan 20261001-first\n\n<!-- plan: 20261001-first -->\n\n1. **stack**: Go and Godot.\n"
	if err := os.WriteFile(filepath.Join(root, "docs", "decisions.md"), []byte(earlier), 0o644); err != nil {
		t.Fatal(err)
	}
	readme := "# Our documents\n\nWritten by hand.\n"
	if err := os.WriteFile(filepath.Join(root, "docs", "README.md"), []byte(readme), 0o644); err != nil {
		t.Fatal(err)
	}
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
	if len(model.steps()) != 0 {
		t.Errorf("a plan that could not start asked the table: %v", model.steps())
	}
}
