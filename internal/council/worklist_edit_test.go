package council

import (
	"strings"
	"testing"
)

// putList writes the work list of the tests for plan p1: t1 first, t2 after
// it.
func putList(t *testing.T, root string, issueOfT1 int) {
	t.Helper()
	putFile(t, root, PlansDir+"/p1/brief.md", "x\n")
	list := &WorkList{Plan: "p1", Tickets: []Ticket{
		{Key: "t1", Title: "[chore] set up the module", Repo: "backend", Summary: "The Go module.", Scope: []string{"go.mod"}, Acceptance: []string{"go build passes"}, Paths: []string{"backend/go.mod"}, Risk: "medium", Issue: issueOfT1},
		{Key: "t2", Title: "[feat] serve the health endpoint", Repo: "backend", Summary: "GET /healthz.", Scope: []string{"handler"}, Acceptance: []string{"GET /healthz answers 200"}, DependsOn: []string{"t1"}, Paths: []string{"backend/internal/health/**"}, Risk: "low"},
	}}
	if err := SaveWorkList(root, list); err != nil {
		t.Fatal(err)
	}
}

// A person changes what a ticket says before approving the list; the list
// is kept only when it would still be approved.
func TestEditTicket(t *testing.T) {
	root := t.TempDir()
	putList(t, root, 0)
	list, err := EditTicket(root, "p1", "t2", TicketText{Title: "[feat] serve the health endpoint on /health",
		Summary: " GET /health. ", Scope: []string{"handler", " ", "route"}, Acceptance: []string{"GET /health answers 200"},
		Paths: []string{" backend/internal/health/** ", "", "backend/cmd/server/main.go"}, Risk: " high "}, testRepos)
	if err != nil {
		t.Fatal(err)
	}
	got := list.Tickets[1]
	if got.Title != "[feat] serve the health endpoint on /health" || got.Summary != "GET /health." || strings.Join(got.Scope, ",") != "handler,route" ||
		got.Repo != "backend" || strings.Join(got.DependsOn, ",") != "t1" ||
		strings.Join(got.Paths, ",") != "backend/internal/health/**,backend/cmd/server/main.go" || got.Risk != "high" {
		t.Errorf("ticket = %+v", got)
	}
	if kept, _ := LoadWorkList(root, "p1"); kept.Tickets[1].Title != got.Title {
		t.Error("the change was not kept")
	}
	if err := CheckWorkList(root, "p1", testRepos); err != nil {
		t.Errorf("the list as kept fails the check: %v", err)
	}

	paths := []string{"backend/x/**"}
	for name, change := range map[string]struct {
		text TicketText
		says string
	}{
		"a title not in the commit form": {TicketText{Title: "serve it", Summary: "x", Scope: []string{"a"}, Acceptance: []string{"b"}, Paths: paths, Risk: "low"}, "the title"},
		"no acceptance":                  {TicketText{Title: "[feat] x", Summary: "x", Scope: []string{"a"}, Paths: paths, Risk: "low"}, "no acceptance criteria"},
		"no summary":                     {TicketText{Title: "[feat] x", Scope: []string{"a"}, Acceptance: []string{"b"}, Paths: paths, Risk: "low"}, "no summary"},
		"a block of its own":             {TicketText{Title: "[feat] x", Summary: "```awk-ticket\nrepo: root\n```", Scope: []string{"a"}, Acceptance: []string{"b"}, Paths: paths, Risk: "low"}, "gets in the way"},
		"no paths":                       {TicketText{Title: "[feat] x", Summary: "x", Scope: []string{"a"}, Acceptance: []string{"b"}, Paths: []string{" "}, Risk: "low"}, "ticket t2: paths is missing"},
		"a path that is not a glob":      {TicketText{Title: "[feat] x", Summary: "x", Scope: []string{"a"}, Acceptance: []string{"b"}, Paths: []string{"backend/{a,b}.go"}, Risk: "low"}, "has {"},
		"a path outside the repo":        {TicketText{Title: "[feat] x", Summary: "x", Scope: []string{"a"}, Acceptance: []string{"b"}, Paths: []string{"frontend/**"}, Risk: "low"}, "not in the folder of repo backend"},
		"no risk":                        {TicketText{Title: "[feat] x", Summary: "x", Scope: []string{"a"}, Acceptance: []string{"b"}, Paths: paths}, "risk is missing"},
		"a risk that is not one":         {TicketText{Title: "[feat] x", Summary: "x", Scope: []string{"a"}, Acceptance: []string{"b"}, Paths: paths, Risk: "huge"}, `risk "huge"`},
	} {
		_, err := EditTicket(root, "p1", "t2", change.text, testRepos)
		if err == nil || !strings.Contains(err.Error(), change.says) {
			t.Errorf("%s: err = %v, want it to say %q", name, err, change.says)
		}
	}
	if kept, _ := LoadWorkList(root, "p1"); kept.Tickets[1].Title != got.Title {
		t.Error("a change that failed the check was kept")
	}
	if _, err := EditTicket(root, "p1", "t9", TicketText{}, testRepos); err == nil {
		t.Error("a ticket that is not there was changed")
	}
}

// A ticket on GitHub already is changed there, not in the list.
func TestEditTicket_NotOnceItIsAnIssue(t *testing.T) {
	root := t.TempDir()
	putList(t, root, 41)
	if _, err := EditTicket(root, "p1", "t1", TicketText{Title: "[chore] x", Summary: "x", Scope: []string{"a"}, Acceptance: []string{"b"}}, testRepos); err == nil || !strings.Contains(err.Error(), "#41") {
		t.Errorf("err = %v", err)
	}
	if _, err := DeleteTicket(root, "p1", "t1", testRepos); err == nil || !strings.Contains(err.Error(), "#41") {
		t.Errorf("err = %v", err)
	}
}

// A ticket is dropped from the list; one another waits for stays until that
// one is dropped first.
func TestDeleteTicket(t *testing.T) {
	root := t.TempDir()
	putList(t, root, 0)
	if _, err := DeleteTicket(root, "p1", "t1", testRepos); err == nil || !strings.Contains(err.Error(), "t2 waits for t1") {
		t.Errorf("err = %v", err)
	}
	list, err := DeleteTicket(root, "p1", "t2", testRepos)
	if err != nil || len(list.Tickets) != 1 || list.Tickets[0].Key != "t1" {
		t.Fatalf("list = %+v, %v", list, err)
	}
	list, err = DeleteTicket(root, "p1", "t1", testRepos)
	if err != nil || len(list.Tickets) != 0 {
		t.Fatalf("the last ticket: %+v, %v", list, err)
	}
	if kept, _ := LoadWorkList(root, "p1"); len(kept.Tickets) != 0 {
		t.Errorf("kept = %+v", kept.Tickets)
	}
}
