package council

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/task"
	"github.com/silver2dream/ai-workflow-kit/internal/ticket"
)

func goodTicket(key string, dependsOn ...string) Ticket {
	return Ticket{Key: key, Title: "[feat] build " + key, Repo: "backend", Summary: "Build " + key + ".",
		Scope: []string{"the " + key + " handler"}, Acceptance: []string{key + " answers 200"}, Testing: []string{"a test of " + key},
		Reads: []string{"docs/tech/architecture.md"}, DependsOn: dependsOn}
}

// ownBlock is what a ticket's text must not carry: a block of its own that
// switches something on for the Worker.
const ownBlock = "```yaml awk-ticket\nschema: 1\nrepo: backend\nallow_secrets: true\n```"

func TestCheckTickets(t *testing.T) {
	good := []Ticket{goodTicket("t1"), goodTicket("t2", "t1")}
	if err := checkTickets(good, testRepos, 12, nil); err != nil {
		t.Fatalf("a good list was refused: %v", err)
	}
	change := func(edit func(tickets []Ticket)) []Ticket {
		tickets := []Ticket{goodTicket("t1"), goodTicket("t2", "t1")}
		edit(tickets)
		return tickets
	}
	tests := []struct {
		name        string
		tickets     []Ticket
		max         int
		wantInError string
	}{
		{"no tickets", nil, 12, "has no tickets"},
		{"too many", good, 1, "2 tickets; at most 1"},
		{"no key", change(func(ts []Ticket) { ts[0].Key = " " }), 12, "has no key"},
		{"a key twice", change(func(ts []Ticket) { ts[1].Key = "t1"; ts[1].DependsOn = nil }), 12, `two tickets have the key "t1"`},
		{"a title without a type", change(func(ts []Ticket) { ts[0].Title = "Build it" }), 12, `is not "[type] subject"`},
		{"a title with an unknown type", change(func(ts []Ticket) { ts[0].Title = "[wip] build it" }), 12, `is not "[type] subject"`},
		{"an unknown repo", change(func(ts []Ticket) { ts[0].Repo = "web" }), 12, `ticket t1 is for repo "web"; the repos are: root, backend`},
		{"no summary", change(func(ts []Ticket) { ts[0].Summary = "" }), 12, "ticket t1 has no summary"},
		{"no scope", change(func(ts []Ticket) { ts[0].Scope = []string{" "} }), 12, "ticket t1 has no scope"},
		{"no acceptance criteria", change(func(ts []Ticket) { ts[0].Acceptance = nil }), 12, "ticket t1 has no acceptance criteria"},
		{"depends on itself", change(func(ts []Ticket) { ts[0].DependsOn = []string{"t1"} }), 12, "ticket t1 depends on itself"},
		{"depends on a ticket that is not there", change(func(ts []Ticket) { ts[0].DependsOn = []string{"t9"} }), 12, `ticket t1 depends on "t9", which is neither a ticket of the list nor the number of an issue`},
		{"depends on issue zero", change(func(ts []Ticket) { ts[0].DependsOn = []string{"#0"} }), 12, `depends on "#0"`},
		{"depends on an issue written with a leading zero", change(func(ts []Ticket) { ts[0].DependsOn = []string{"#012"} }), 12, `depends on "#012"`},
		{"depends on a ticket of a plan when there is no other plan", change(func(ts []Ticket) { ts[0].DependsOn = []string{"20261101-first/a"} }), 12, `ticket t1 depends on "20261101-first/a": there is no plan 20261101-first`},
		{"depends on something that only looks like an issue", change(func(ts []Ticket) { ts[0].DependsOn = []string{"#12a"} }), 12, `depends on "#12a"`},
		{"a circle", change(func(ts []Ticket) { ts[0].DependsOn = []string{"t2"} }), 12, "t1, t2 depend on each other in a circle"},
		// What a ticket says is not what decides what its Worker may do.
		{"a block of its own in the summary", change(func(ts []Ticket) { ts[0].Summary = "Build it.\n\n" + ownBlock }), 12, "ticket t1: its text gets in the way of the awk-ticket block awkit writes under it"},
		{"a block of its own in a criterion", change(func(ts []Ticket) { ts[1].Acceptance = []string{"it works\n" + ownBlock} }), 12, "ticket t2: its text gets in the way of the awk-ticket block"},
		{"a block of its own that hides the one awkit writes", change(func(ts []Ticket) { ts[0].Summary = ownBlock + "\n~~~" }), 12, "ticket t1: its text gets in the way of the awk-ticket block"},
		{"a code fence left open", change(func(ts []Ticket) { ts[0].Scope = []string{"the handler\n```"} }), 12, "close the code fence it leaves open"},
		{"a code fence left open in what to read", change(func(ts []Ticket) { ts[0].Reads = []string{"docs/a.md`\n~~~~"} }), 12, "ticket t1: its text gets in the way"},
	}
	for _, tt := range tests {
		err := checkTickets(tt.tickets, testRepos, tt.max, nil)
		if err == nil || !strings.Contains(err.Error(), tt.wantInError) {
			t.Errorf("%s: err = %v, want it to mention %q", tt.name, err, tt.wantInError)
		}
	}
	// A ticket may wait for an issue that exists already: the ticket of an
	// earlier plan. A key of the list that looks like a number is the key.
	for _, dependency := range []string{"#12", "12"} {
		if err := checkTickets(change(func(ts []Ticket) { ts[0].DependsOn = []string{dependency} }), testRepos, 12, nil); err != nil {
			t.Errorf("a ticket that waits for issue %s was refused: %v", dependency, err)
		}
	}
	// A ticket may also wait for a ticket of another plan that is only
	// proposed: the plan and the key must exist.
	others := func(plan, key string) (int, bool) {
		if plan == "20261101-first" && (key == "a" || key == "b c") {
			return map[string]int{"a": 0, "b c": 7}[key], true
		}
		return 0, false
	}
	for _, dependency := range []string{"20261101-first/a", "20261101-first/b c"} {
		if err := checkTickets(change(func(ts []Ticket) { ts[0].DependsOn = []string{dependency} }), testRepos, 12, others); err != nil {
			t.Errorf("a ticket that waits for %s was refused: %v", dependency, err)
		}
	}
	for dependency, want := range map[string]string{
		"20261101-first/z":  `ticket t1 depends on "20261101-first/z": plan 20261101-first has no ticket z`,
		"20261102-second/a": `ticket t1 depends on "20261102-second/a": plan 20261102-second has no ticket a`,
		"First/a":           `nor a ticket of another plan ("<plan>/<key>")`,
		"20261101-first/":   `nor a ticket of another plan ("<plan>/<key>")`,
	} {
		err := checkTickets(change(func(ts []Ticket) { ts[0].DependsOn = []string{dependency} }), testRepos, 12, others)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("waiting for %q: err = %v, want it to mention %q", dependency, err, want)
		}
	}
	if plan, key, ok := otherPlan("20261101-first/a", map[string]bool{"20261101-first/a": true}); ok {
		t.Errorf("a key of the list is read as a ticket of plan %s (%s)", plan, key)
	}
	if got := existingIssue("#12", map[string]bool{"t1": true}); got != 12 {
		t.Errorf("existingIssue(#12) = %d", got)
	}
	if got := existingIssue("12", map[string]bool{"12": true}); got != 0 {
		t.Errorf("existingIssue(12) = %d although 12 is a key of the list", got)
	}
	// A ticket may show code: a block that is closed gets in nobody's way.
	if err := checkTickets(change(func(ts []Ticket) { ts[0].Summary = "Build it like this:\n\n```go\nfunc main() {}\n```\n" }), testRepos, 12, nil); err != nil {
		t.Errorf("a ticket with a closed code block was refused: %v", err)
	}
	// The repo's name may be written in any case, as in a ticket's block; no
	// limit when the limit is zero.
	if err := checkTickets(change(func(ts []Ticket) { ts[0].Repo = "Backend" }), testRepos, 0, nil); err != nil {
		t.Errorf("a repo written in another case was refused: %v", err)
	}
}

// Every ticket comes after the ones it depends on; where nothing forces an
// order, the list's own order is kept.
func TestWorkOrder(t *testing.T) {
	tickets := []Ticket{goodTicket("d", "b", "c"), goodTicket("a"), goodTicket("c", "a"), goodTicket("b"), goodTicket("e")}

	ordered, err := workOrder(tickets)

	if err != nil {
		t.Fatalf("workOrder: %v", err)
	}
	var keys []string
	for _, item := range ordered {
		keys = append(keys, item.Key)
	}
	if want := []string{"a", "c", "b", "e", "d"}; !reflect.DeepEqual(keys, want) {
		t.Errorf("order = %v, want %v", keys, want)
	}

	// An issue that exists already is not waited for.
	ordered, err = workOrder([]Ticket{goodTicket("b", "a", "#7"), goodTicket("a", "#7")})
	if err != nil || len(ordered) != 2 || ordered[0].Key != "a" || ordered[1].Key != "b" {
		t.Errorf("with an issue to wait for: order = %+v, err = %v", ordered, err)
	}
}

// issues plays GitHub for an approval: it numbers the issues from 41 and
// keeps what each was created with.
type issues struct {
	next   int
	titles []string
	bodies []string
	failAt int // the call that fails, counted from 1; 0 for none
}

func (g *issues) create(ctx context.Context, title, bodyFile string) (int, error) {
	if g.failAt != 0 && len(g.titles)+1 == g.failAt {
		g.failAt = 0
		return 0, errors.New("HTTP 502")
	}
	body, err := os.ReadFile(bodyFile)
	if err != nil {
		return 0, err
	}
	g.titles = append(g.titles, title)
	g.bodies = append(g.bodies, string(body))
	g.next++
	return 40 + g.next, nil
}

func saveList(t *testing.T, root string, tickets ...Ticket) {
	t.Helper()
	if err := SaveWorkList(root, &WorkList{Plan: "20261114-health", Tickets: tickets}); err != nil {
		t.Fatal(err)
	}
}

// Approving a work list creates its issues, each after the ones it depends on
// and with their numbers in its block, and writes the numbers back.
func TestApprove_CreatesTheIssuesInWorkingOrder(t *testing.T) {
	root := t.TempDir()
	saveList(t, root, goodTicket("api", "model", "store"), goodTicket("model"), goodTicket("store", "model"))
	github := &issues{}
	var told []string

	approval, err := Approve(context.Background(), ApproveOptions{Root: root, ID: "20261114-health", Repos: testRepos,
		Create: github.create, Progress: func(line string) { told = append(told, line) }})

	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if want := []string{"[feat] build model", "[feat] build store", "[feat] build api"}; !reflect.DeepEqual(github.titles, want) {
		t.Errorf("created in the order %v, want %v", github.titles, want)
	}
	if len(approval.Created) != 3 || len(approval.Existing) != 0 || approval.Created[2].Issue != 43 {
		t.Errorf("approval = %+v", approval)
	}

	wantDependsOn := [][]int{nil, {41}, {41, 42}}
	for i, body := range github.bodies {
		// The body is a ticket the workflow accepts, and reads back as meant.
		if err := task.ValidateBody(body); err != nil {
			t.Errorf("the body of %q would be refused by create-task: %v\n%s", github.titles[i], err, body)
		}
		meta, found, err := ticket.Parse(body)
		if err != nil || !found {
			t.Fatalf("the body of %q has no block the workflow reads: found=%v err=%v", github.titles[i], found, err)
		}
		if meta.Repo != "backend" || !reflect.DeepEqual(meta.DependsOn, wantDependsOn[i]) {
			t.Errorf("%q: block = %+v, want repo backend and depends_on %v", github.titles[i], meta, wantDependsOn[i])
		}
	}
	api := github.bodies[2]
	for _, want := range []string{"## Summary\nBuild api.\n", "## Scope\n- the api handler\n", "## Acceptance Criteria\n- [ ] api answers 200\n",
		"## Testing Requirements\n- a test of api\n", "- `docs/tech/architecture.md`\n", "`docs/plans/20261114-health/minutes.md`"} {
		if !strings.Contains(api, want) {
			t.Errorf("the body should have %q:\n%s", want, api)
		}
	}

	// The numbers are on record, in the list as the person left it.
	list, err := LoadWorkList(root, "20261114-health")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	var order []string
	for _, item := range list.Tickets {
		got[item.Key] = item.Issue
		order = append(order, item.Key)
	}
	if want := map[string]int{"model": 41, "store": 42, "api": 43}; !reflect.DeepEqual(got, want) {
		t.Errorf("issues on record = %v, want %v", got, want)
	}
	if want := []string{"api", "model", "store"}; !reflect.DeepEqual(order, want) {
		t.Errorf("the list was reordered to %v", order)
	}
	if len(told) != 3 || !strings.Contains(told[0], "created #41 for model") {
		t.Errorf("progress = %v", told)
	}
	// Nothing is left in the temp folder.
	if left, _ := os.ReadDir(root + "/.ai/temp"); len(left) != 0 {
		t.Errorf("%d body files left behind", len(left))
	}

	// Approving again creates nothing.
	again, err := Approve(context.Background(), ApproveOptions{Root: root, ID: "20261114-health", Repos: testRepos, Create: github.create})
	if err != nil || len(again.Created) != 0 || len(again.Existing) != 3 || len(github.titles) != 3 {
		t.Errorf("a second approval: %+v, err %v, %d issues created in all", again, err, len(github.titles))
	}
}

// An approval that fails half way keeps what it created, and the next one
// goes on from there: no ticket becomes two issues.
func TestApprove_GoesOnAfterAFailure(t *testing.T) {
	root := t.TempDir()
	saveList(t, root, goodTicket("model"), goodTicket("store", "model"), goodTicket("api", "store"))
	github := &issues{failAt: 2}

	approval, err := Approve(context.Background(), ApproveOptions{Root: root, ID: "20261114-health", Repos: testRepos, Create: github.create})

	if err == nil || !strings.Contains(err.Error(), "ticket store ([feat] build store) was not created") || !strings.Contains(err.Error(), "HTTP 502") {
		t.Fatalf("err = %v", err)
	}
	if len(approval.Created) != 1 {
		t.Errorf("approval = %+v, want the one ticket that was created", approval)
	}
	list, _ := LoadWorkList(root, "20261114-health")
	if list.Tickets[0].Issue != 41 || list.Tickets[1].Issue != 0 {
		t.Errorf("after the failure the list says %+v", list.Tickets)
	}

	approval, err = Approve(context.Background(), ApproveOptions{Root: root, ID: "20261114-health", Repos: testRepos, Create: github.create})

	if err != nil {
		t.Fatalf("the second approval: %v", err)
	}
	if want := []string{"[feat] build model", "[feat] build store", "[feat] build api"}; !reflect.DeepEqual(github.titles, want) {
		t.Errorf("issues created in all: %v, want each ticket once", github.titles)
	}
	if len(approval.Existing) != 1 || len(approval.Created) != 2 {
		t.Errorf("the second approval = %+v", approval)
	}
	// The ticket created the second time depends on the issue of the first.
	if meta, _, _ := ticket.Parse(github.bodies[1]); !reflect.DeepEqual(meta.DependsOn, []int{41}) {
		t.Errorf("store depends on %v, want the issue created before the failure", meta.DependsOn)
	}
}

// A dry run says what would be created, creates nothing and changes nothing.
func TestApprove_DryRun(t *testing.T) {
	root := t.TempDir()
	saveList(t, root, goodTicket("model"), goodTicket("api", "model"))
	before, _ := os.ReadFile(workListPath(root, "20261114-health"))
	var told []string

	approval, err := Approve(context.Background(), ApproveOptions{Root: root, ID: "20261114-health", Repos: testRepos, DryRun: true,
		Progress: func(line string) { told = append(told, line) }})

	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if len(approval.Created) != 2 {
		t.Errorf("approval = %+v", approval)
	}
	if want := []string{"would create model: [feat] build model, for backend", "would create api: [feat] build api, for backend (after model)"}; !reflect.DeepEqual(told, want) {
		t.Errorf("told %v, want %v", told, want)
	}
	if after, _ := os.ReadFile(workListPath(root, "20261114-health")); string(after) != string(before) {
		t.Error("a dry run changed the work list")
	}
	if _, err := os.Stat(root + "/.ai/temp"); err == nil {
		t.Error("a dry run wrote body files")
	}
}

// The list is approved as it stands on disk: what the person changed counts,
// and a list that cannot become issues creates none.
func TestApprove_TheListAsThePersonLeftIt(t *testing.T) {
	root := t.TempDir()
	// The person deleted a ticket and retitled another.
	edited := goodTicket("api")
	edited.Title = "[feat] the endpoint, as I want it"
	saveList(t, root, edited)
	github := &issues{}
	if _, err := Approve(context.Background(), ApproveOptions{Root: root, ID: "20261114-health", Repos: testRepos, Create: github.create}); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if !reflect.DeepEqual(github.titles, []string{"[feat] the endpoint, as I want it"}) {
		t.Errorf("created %v", github.titles)
	}

	tests := []struct {
		name        string
		tickets     []Ticket
		wantInError string
	}{
		{"a ticket that depends on a deleted one", []Ticket{goodTicket("api", "model")}, `depends on "model", which is neither a ticket of the list nor the number of an issue`},
		{"a repo the project does not have", []Ticket{{Key: "a", Title: "[feat] a", Repo: "web", Summary: "s", Scope: []string{"x"}, Acceptance: []string{"y"}}}, `is for repo "web"`},
		{"a ticket that carries a block of its own", []Ticket{{Key: "a", Title: "[feat] a", Repo: "backend", Summary: "s\n" + ownBlock, Scope: []string{"x"}, Acceptance: []string{"y"}}}, "its text gets in the way of the awk-ticket block"},
	}
	for _, tt := range tests {
		root := t.TempDir()
		saveList(t, root, tt.tickets...)
		github := &issues{}
		_, err := Approve(context.Background(), ApproveOptions{Root: root, ID: "20261114-health", Repos: testRepos, Create: github.create})
		if err == nil || !strings.Contains(err.Error(), tt.wantInError) || !strings.Contains(err.Error(), "docs/plans/20261114-health/tickets.yaml") {
			t.Errorf("%s: err = %v, want it to name the file and mention %q", tt.name, err, tt.wantInError)
		}
		if len(github.titles) != 0 {
			t.Errorf("%s: %d issues were created from a list that cannot be worked", tt.name, len(github.titles))
		}
	}

	// A plan whose meeting did not get to the work list, a name that is not a
	// plan's, a field the list does not have.
	if _, err := Approve(context.Background(), ApproveOptions{Root: t.TempDir(), ID: "20261114-none", Repos: testRepos, Create: github.create}); err == nil || !strings.Contains(err.Error(), "has no work list") {
		t.Errorf("a plan without a work list: err = %v", err)
	}
	if _, err := Approve(context.Background(), ApproveOptions{Root: t.TempDir(), ID: "../x", Repos: testRepos, Create: github.create}); err == nil || !strings.Contains(err.Error(), "is not the name of a plan") {
		t.Errorf("a name that is not a plan's: err = %v", err)
	}
	root = t.TempDir()
	saveList(t, root, goodTicket("api"))
	file := workListPath(root, "20261114-health")
	data, _ := os.ReadFile(file)
	withField := strings.Replace(string(data), "      repo: backend\n", "      repo: backend\n      priority: high\n", 1)
	if withField == string(data) {
		t.Fatalf("the list of the test is not laid out as expected:\n%s", data)
	}
	if err := os.WriteFile(file, []byte(withField), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Approve(context.Background(), ApproveOptions{Root: root, ID: "20261114-health", Repos: testRepos, Create: github.create}); err == nil || !strings.Contains(err.Error(), "cannot be read") || !strings.Contains(err.Error(), "priority") {
		t.Errorf("a list with a field it does not have: err = %v", err)
	}
}

// A ticket without testing requirements of its own still gets the section a
// ticket needs.
func TestTicketBody_WithoutTestingRequirements(t *testing.T) {
	item := goodTicket("api")
	item.Testing, item.Reads = nil, nil

	body := ticketBody(item, "20261114-health", nil)

	if err := task.ValidateBody(body); err != nil {
		t.Errorf("the body would be refused: %v\n%s", err, body)
	}
	if !strings.Contains(body, "## Testing Requirements\n- New behaviour is covered by unit tests\n") {
		t.Errorf("the body has no testing requirements:\n%s", body)
	}
	if meta, _, _ := ticket.Parse(body); len(meta.DependsOn) != 0 {
		t.Errorf("depends_on = %v, want none", meta.DependsOn)
	}
}

// A meeting held a second time keeps the issues that were created from the
// first list.
func TestRun_KeepsTheIssuesOfAnApprovedList(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, testRoster)
	model := &table{answer: agreeable(roster)}
	plan, err := runPlan(t, root, roster, model, "Add a health endpoint.")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	github := &issues{}
	if _, err := Approve(context.Background(), ApproveOptions{Root: root, ID: plan.ID, Repos: testRepos, Roster: roster, Create: github.create}); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if index := readFile(t, root, "docs/README.md"); !strings.Contains(index, "2 tickets, 2 on GitHub") {
		t.Errorf("the index does not show what was created:\n%s", index)
	}

	again, err := Run(context.Background(), Options{Root: root, ID: plan.ID, Roster: roster, Model: model, Repos: testRepos, Now: fixedNow})

	if err != nil {
		t.Fatalf("going on: %v", err)
	}
	for _, item := range again.Tickets {
		if item.Issue == 0 {
			t.Errorf("ticket %s lost its issue", item.Key)
		}
	}
	list, _ := LoadWorkList(root, plan.ID)
	if list.Tickets[0].Issue != 41 || list.Tickets[1].Issue != 42 {
		t.Errorf("the list on disk lost its issues: %+v", list.Tickets)
	}

	// On another machine nothing of the meeting is remembered (.ai/state is
	// not committed): the table is asked anew and writes the list again. The
	// issues stay with their tickets.
	if err := os.RemoveAll(filepath.Join(root, ".ai", "state")); err != nil {
		t.Fatal(err)
	}
	// This time the planning role has a third ticket.
	base := agreeable(roster)
	model.answer = func(c call) (string, error) {
		if c.Step == "worklist" {
			return block(strings.Replace(goodTickets, `], "later"`, `, {"key": "t3", "title": "[test] check the endpoint", "repo": "backend", "summary": "A check.", "scope": ["a test"], "acceptance": ["the check passes"], "testing": [], "depends_on": ["t2"]}], "later"`, 1)), nil
		}
		return base(c)
	}
	before := len(model.steps())
	anew, err := Run(context.Background(), Options{Root: root, ID: plan.ID, Roster: roster, Model: model, Repos: testRepos, Now: fixedNow})
	if err != nil {
		t.Fatalf("the meeting held anew: %v", err)
	}
	if len(model.steps()) == before {
		t.Fatal("nothing was asked although nothing was remembered")
	}
	issue := map[string]int{}
	for _, item := range anew.Tickets {
		issue[item.Key] = item.Issue
	}
	if want := map[string]int{"t1": 41, "t2": 42, "t3": 0}; !reflect.DeepEqual(issue, want) {
		t.Errorf("the tickets of a list written anew have the issues %v, want %v", issue, want)
	}
	list, _ = LoadWorkList(root, plan.ID)
	if len(list.Tickets) != 3 || list.Tickets[0].Issue != 41 || list.Tickets[1].Issue != 42 || list.Tickets[2].Key != "t3" || list.Tickets[2].Issue != 0 {
		t.Errorf("the list on disk is not the one written anew, with the issues that were there: %+v", list.Tickets)
	}
	if got := strings.Count(readFile(t, root, DecisionsPath), "<!-- plan: "+plan.ID+" -->"); got != 1 {
		t.Errorf("the plan's decisions are on record %d times, want once", got)
	}
}

// The list that is there is the person's to change: going on with a plan
// whose table has nothing new to say leaves it as they left it, and a list
// that cannot be read is not written over.
func TestRun_GoingOnLeavesTheListAsThePersonLeftIt(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, testRoster)
	model := &table{answer: agreeable(roster)}
	plan, err := runPlan(t, root, roster, model, "Add a health endpoint.")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// The person drops a ticket and retitles the other.
	list, err := LoadWorkList(root, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	list.Tickets = list.Tickets[:1]
	list.Tickets[0].Title = "[chore] the module, my way"
	if err := SaveWorkList(root, list); err != nil {
		t.Fatal(err)
	}
	edited := readFile(t, root, plan.Dir()+"/"+WorkListFile)

	var told []string
	again, err := Run(context.Background(), Options{Root: root, ID: plan.ID, Roster: roster, Model: model, Repos: testRepos, Now: fixedNow,
		Progress: func(line string) { told = append(told, line) }})

	if err != nil {
		t.Fatalf("going on: %v", err)
	}
	if len(again.Tickets) != 1 || again.Tickets[0].Title != "[chore] the module, my way" {
		t.Errorf("the plan gone on with holds %+v, want the list as the person left it", again.Tickets)
	}
	if got := readFile(t, root, plan.Dir()+"/"+WorkListFile); got != edited {
		t.Errorf("going on with the plan rewrote the work list:\n%s", got)
	}
	if minutes := readFile(t, root, plan.Dir()+"/minutes.md"); !strings.Contains(minutes, "| t1 | [chore] the module, my way | backend | - |") || strings.Contains(minutes, "| t2 |") {
		t.Errorf("the minutes do not show the list as it stands:\n%s", minutes)
	}
	if progress := strings.Join(told, "\n"); !strings.Contains(progress, "  the work list stays as it is: 1 tickets") || strings.Contains(progress, "tickets proposed") {
		t.Errorf("the person is not told that the list stays:\n%s", progress)
	}

	// A list that cannot be read is not written over: it may hold issues.
	const garbled = "tickets: [\n"
	putFile(t, root, plan.Dir()+"/"+WorkListFile, garbled)
	_, err = Run(context.Background(), Options{Root: root, ID: plan.ID, Roster: roster, Model: model, Repos: testRepos, Now: fixedNow})
	if err == nil || !strings.Contains(err.Error(), "the work list that is there cannot be read; correct it or remove it") {
		t.Errorf("err = %v, want the list that cannot be read to stop the plan", err)
	}
	if got := readFile(t, root, plan.Dir()+"/"+WorkListFile); got != garbled {
		t.Errorf("a list that could not be read was written over:\n%s", got)
	}
}

// The planning role is shown what the other plans ticketed, so that the same
// work is not ticketed twice, and a ticket can wait for an issue that exists.
func TestRun_BuildsOnTheTicketsOfEarlierPlans(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, testRoster)
	first := &table{answer: agreeable(roster)}
	plan, err := runPlan(t, root, roster, first, "Add a health endpoint.")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if prompt := first.promptOf("builder", "worklist"); strings.Contains(prompt, "What earlier plans ticketed") || strings.Contains(prompt, "#12") {
		t.Errorf("the first plan of a project is told about earlier ones:\n%s", prompt)
	}
	// Proposed and not approved is said to be just that.
	proposed := "Plan " + plan.ID + ":\n- " + plan.ID + "/t1 (proposed, not on GitHub): [chore] set up the module (repo: backend)\n- " + plan.ID + "/t2 (proposed, not on GitHub): [feat] serve the health endpoint (repo: backend)\n"
	if got := (&planner{root: root}).earlierTickets("20270101-another"); got != proposed {
		t.Errorf("earlier tickets = %q, want %q", got, proposed)
	}
	if got := (&planner{root: root}).earlierTickets(plan.ID); got != "" {
		t.Errorf("a plan is shown its own tickets as earlier ones: %q", got)
	}
	github := &issues{}
	if _, err := Approve(context.Background(), ApproveOptions{Root: root, ID: plan.ID, Repos: testRepos, Create: github.create}); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	base := agreeable(roster)
	second := &table{answer: func(c call) (string, error) {
		if c.Step == "worklist" {
			return block(`{"tickets": [{"key": "v1", "title": "[feat] serve the version", "repo": "backend", "summary": "GET /version.", "scope": ["handler"], "acceptance": ["GET /version answers 200"], "testing": [], "depends_on": ["#42"]}], "later": []}`), nil
		}
		return base(c)
	}}
	next, err := runPlan(t, root, roster, second, "Add a version endpoint.")
	if err != nil {
		t.Fatalf("the second plan: %v", err)
	}
	prompt := second.promptOf("builder", "worklist")
	for _, want := range []string{
		"What earlier plans ticketed:\nPlan " + plan.ID + ":\n- #41: [chore] set up the module (repo: backend)\n- #42: [feat] serve the health endpoint (repo: backend)\n",
		`- Work an earlier plan ticketed is not ticketed again, whether its ticket is on GitHub or only proposed. A ticket that builds on an earlier one names it in depends_on by what its line in the list of earlier tickets starts with: "#12" for an issue, "<plan>/<key>" for a ticket that is only proposed.`,
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the planning role should be told %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "Plan "+next.ID+":") {
		t.Errorf("the planning role is shown its own plan as an earlier one:\n%s", prompt)
	}
	if len(next.Tickets) != 1 || !reflect.DeepEqual(next.Tickets[0].DependsOn, []string{"#42"}) {
		t.Fatalf("the second plan's tickets = %+v", next.Tickets)
	}
	// The list on disk says the same, and reads back.
	list, err := LoadWorkList(root, next.ID)
	if err != nil || !reflect.DeepEqual(list.Tickets[0].DependsOn, []string{"#42"}) {
		t.Fatalf("the list on disk = %+v, err %v", list, err)
	}
	if minutes := readFile(t, root, next.Dir()+"/minutes.md"); !strings.Contains(minutes, "| v1 | [feat] serve the version | backend | #42 |") {
		t.Errorf("the minutes do not show what the ticket waits for:\n%s", minutes)
	}

	// Approved, the ticket's block names the issue it waits for.
	if _, err := Approve(context.Background(), ApproveOptions{Root: root, ID: next.ID, Repos: testRepos, Create: github.create}); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if meta, _, _ := ticket.Parse(github.bodies[2]); !reflect.DeepEqual(meta.DependsOn, []int{42}) {
		t.Errorf("the ticket waits for %v, want issue 42", meta.DependsOn)
	}

	// A person writes the number bare, next to a key of the list.
	putFile(t, root, "docs/plans/20261115-by-hand/tickets.yaml", `plan: 20261115-by-hand
tickets:
  - key: a
    title: "[feat] build a"
    repo: backend
    summary: Build a.
    scope: [the a handler]
    acceptance: [a answers 200]
    testing: []
    depends_on: [42]
  - key: b
    title: "[feat] build b"
    repo: backend
    summary: Build b.
    scope: [the b handler]
    acceptance: [b answers 200]
    testing: []
    depends_on: [a, 41]
`)
	if _, err := Approve(context.Background(), ApproveOptions{Root: root, ID: "20261115-by-hand", Repos: testRepos, Create: github.create}); err != nil {
		t.Fatalf("Approve of a list written by hand: %v", err)
	}
	if meta, _, _ := ticket.Parse(github.bodies[3]); !reflect.DeepEqual(meta.DependsOn, []int{42}) {
		t.Errorf("a waits for %v, want issue 42", meta.DependsOn)
	}
	if meta, _, _ := ticket.Parse(github.bodies[4]); !reflect.DeepEqual(meta.DependsOn, []int{44, 41}) {
		t.Errorf("b waits for %v, want the issue a became (44) and issue 41", meta.DependsOn)
	}
}

// A ticket can wait for a ticket of another plan that is only proposed. The
// approval turns the name into the issue that ticket became, and refuses,
// before it creates anything, while the other plan is not on GitHub.
func TestApprove_ATicketThatWaitsForAnotherPlan(t *testing.T) {
	root := t.TempDir()
	save := func(id string, tickets ...Ticket) {
		t.Helper()
		if err := SaveWorkList(root, &WorkList{Plan: id, Tickets: tickets}); err != nil {
			t.Fatal(err)
		}
	}
	save("20261101-first", goodTicket("model"), goodTicket("store", "model"))
	save("20261102-second", goodTicket("api", "20261101-first/store"), goodTicket("docs", "api", "20261101-first/model"))
	github := &issues{}
	approve := func(id string, dryRun bool) (*Approval, error) {
		return Approve(context.Background(), ApproveOptions{Root: root, ID: id, Repos: testRepos, Create: github.create, DryRun: dryRun})
	}

	// The second plan first: nothing is created, and the person is told what
	// to approve before it, in a dry run as well.
	for _, dryRun := range []bool{true, false} {
		_, err := approve("20261102-second", dryRun)
		if err == nil || !strings.Contains(err.Error(), "docs/plans/20261102-second/tickets.yaml: ticket api waits for 20261101-first/store, which is not on GitHub yet: approve that plan first (awkit plan approve 20261101-first)") {
			t.Errorf("dry run %v: err = %v", dryRun, err)
		}
	}
	if len(github.titles) != 0 {
		t.Fatalf("%d issues were created for a plan that waits for another", len(github.titles))
	}

	if _, err := approve("20261101-first", false); err != nil {
		t.Fatalf("the first plan: %v", err)
	}
	if _, err := approve("20261102-second", false); err != nil {
		t.Fatalf("the second plan, after the first: %v", err)
	}
	// model 41, store 42, then api 43 and docs 44.
	if want := []string{"[feat] build model", "[feat] build store", "[feat] build api", "[feat] build docs"}; !reflect.DeepEqual(github.titles, want) {
		t.Fatalf("created %v, want %v", github.titles, want)
	}
	if meta, _, _ := ticket.Parse(github.bodies[2]); !reflect.DeepEqual(meta.DependsOn, []int{42}) {
		t.Errorf("api waits for %v, want the issue store became (42)", meta.DependsOn)
	}
	if meta, _, _ := ticket.Parse(github.bodies[3]); !reflect.DeepEqual(meta.DependsOn, []int{43, 41}) {
		t.Errorf("docs waits for %v, want the issue api became (43) and the one model became (41)", meta.DependsOn)
	}

	// A plan whose own tickets are partly created goes on: a ticket that has
	// its issue is not held up by what it once waited for.
	half := goodTicket("done", "20261103-third/gone")
	half.Issue = 9
	save("20261103-third", goodTicket("gone"))
	save("20261104-fourth", half, goodTicket("next", "done"))
	if _, err := approve("20261104-fourth", false); err != nil {
		t.Errorf("a plan whose waiting ticket is already an issue: %v", err)
	}
	if meta, _, _ := ticket.Parse(github.bodies[4]); !reflect.DeepEqual(meta.DependsOn, []int{9}) {
		t.Errorf("next waits for %v, want issue 9", meta.DependsOn)
	}
}

// A draft that no longer fits the project is asked for again. That alone is
// something new: the list is written again although nobody else was asked.
func TestRun_ADraftAskedAnewIsWritten(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, testRoster)
	base := agreeable(roster)
	model := &table{answer: base}
	plan, err := runPlan(t, root, roster, model, "Add a health endpoint.")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// The project loses its backend and gains an api: the tickets that were
	// remembered are for a repo that is not there any more.
	repos := []Repo{{Name: "root", Path: "./"}, {Name: "api", Language: "go", Path: "api/"}}
	model.answer = func(c call) (string, error) {
		if c.Step == "worklist" {
			return block(strings.ReplaceAll(goodTickets, `"repo": "backend"`, `"repo": "api"`)), nil
		}
		return base(c)
	}
	before := len(model.steps())

	again, err := Run(context.Background(), Options{Root: root, ID: plan.ID, Roster: roster, Model: model, Repos: repos, Now: fixedNow})

	if err != nil {
		t.Fatalf("going on: %v", err)
	}
	if asked := model.steps()[before:]; !reflect.DeepEqual(asked, []string{"builder worklist"}) {
		t.Fatalf("going on asked %v, want only the draft that no longer fits", asked)
	}
	for _, item := range again.Tickets {
		if item.Repo != "api" {
			t.Errorf("ticket %s is for %s, want the draft that was asked for anew", item.Key, item.Repo)
		}
	}
	list, err := LoadWorkList(root, plan.ID)
	if err != nil || len(list.Tickets) != 2 || list.Tickets[0].Repo != "api" || list.Tickets[1].Repo != "api" {
		t.Errorf("the list on disk is not the draft that was asked for anew: %+v, err %v", list, err)
	}
}

// Open names what there is to approve: the plans that have tickets which are
// not on GitHub, the newest first.
func TestOpen(t *testing.T) {
	root := t.TempDir()
	save := func(id string, tickets ...Ticket) {
		t.Helper()
		if err := SaveWorkList(root, &WorkList{Plan: id, Tickets: tickets}); err != nil {
			t.Fatal(err)
		}
	}
	if got := Open(root); len(got) != 0 {
		t.Errorf("a project without plans has %v to approve", got)
	}
	created := goodTicket("done")
	created.Issue = 7
	save("20261101-first", created)
	save("20261102-second", created, goodTicket("a", "done"), goodTicket("b", "done"))
	save("20261103-third", goodTicket("x"))
	putFile(t, root, "docs/plans/20261104-garbled/tickets.yaml", "tickets: [\n")
	putFile(t, root, "docs/plans/20261105-no-list/brief.md", "A brief.\n")

	if got, want := Open(root), []string{"20261103-third (1 of 1 tickets)", "20261102-second (2 of 3 tickets)"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Open = %v, want %v", got, want)
	}
}
