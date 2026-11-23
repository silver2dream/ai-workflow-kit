package settings

import (
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/council"
)

// rosterFile is a roster as a person keeps it: comments, blank lines, a
// stance written as a block.
const rosterFile = `# Who sits at the table.
model: sonnet # the default for every role

limits:
  topics: 2
  calls: 60

roles:
  # Decides what stays disputed.
  - id: product-owner
    title: Product Owner
    decides: true
    stance: |
      You answer for the outcome.
    owns:
      - docs/product/

  - id: architect
    title: Architect
    plans: true
    stance: |
      You answer for how it is built.
    owns:
      - docs/tech/

  - id: skeptic
    title: Skeptic
    stance: |
      You look for how this fails.
`

func intOf(n int) *int { return &n }

func readRoster(t *testing.T, data string) *Roster {
	t.Helper()
	roster, err := ReadRoster([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	return roster
}

func TestReadRoster_ReadsWhatTheFileSaysAndNothingMore(t *testing.T) {
	roster := readRoster(t, rosterFile)
	if roster.Model != "sonnet" || roster.Effort != "" || roster.Language != "" {
		t.Errorf("roster = %+v", roster)
	}
	if roster.Limits.Topics == nil || *roster.Limits.Topics != 2 || roster.Limits.Disputes != nil || roster.Limits.Calls == nil || *roster.Limits.Calls != 60 {
		t.Errorf("limits = %+v", roster.Limits)
	}
	if len(roster.Roles) != 3 {
		t.Fatalf("roles = %+v", roster.Roles)
	}
	owner := roster.Roles[0]
	if owner.ID != "product-owner" || !owner.Decides || owner.Plans || owner.Stance != "You answer for the outcome.\n" || len(owner.Owns) != 1 {
		t.Errorf("product owner = %+v", owner)
	}
	// A role that names nothing it owns has an empty list, not none: the
	// page reads a list.
	if roster.Roles[2].Owns == nil || roster.Roles[2].Model != "" {
		t.Errorf("skeptic = %+v", roster.Roles[2])
	}
}

// Writing back what was read changes nothing: not a comment, not a blank
// line, not a quote.
func TestApplyRoster_WhatWasReadWritesTheSameFile(t *testing.T) {
	out, err := ApplyRoster([]byte(rosterFile), *readRoster(t, rosterFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != rosterFile {
		t.Errorf("the file changed:\n%s", out)
	}
	// And so with Windows line ends.
	crlf := strings.ReplaceAll(rosterFile, "\n", "\r\n")
	if out, err := ApplyRoster([]byte(crlf), *readRoster(t, crlf)); err != nil || string(out) != crlf {
		t.Errorf("CRLF file changed (%v):\n%q", err, out)
	}
}

func TestApplyRoster_ChangesInPlaceAndKeepsTheRest(t *testing.T) {
	roster := readRoster(t, rosterFile)
	roster.Effort = "high"
	roster.Limits.Topics = nil        // back to the default
	roster.Limits.Parallel = intOf(5) // newly set
	roster.Roles[1].Model = "opus"
	roster.Roles[1].Stance = "You answer for how it is built,\nand in what order."
	// The skeptic is renamed and moved first; a new role joins at the end.
	skeptic := roster.Roles[2]
	skeptic.From, skeptic.ID, skeptic.Title = "skeptic", "qa-lead", "QA Lead"
	roster.Roles = []Role{skeptic, roster.Roles[0], roster.Roles[1], {
		ID: "designer", Title: "Designer", Stance: "You speak for the player.", Owns: []string{" docs/design/ ", ""},
	}}

	out, err := ApplyRoster([]byte(rosterFile), *roster)
	if err != nil {
		t.Fatal(err)
	}
	want := `# Who sits at the table.
model: sonnet # the default for every role

effort: high

limits:
  calls: 60
  parallel: 5

roles:
  - id: qa-lead
    title: QA Lead
    stance: |
      You look for how this fails.

  # Decides what stays disputed.
  - id: product-owner
    title: Product Owner
    decides: true
    stance: |
      You answer for the outcome.
    owns:
      - docs/product/

  - id: architect
    title: Architect
    model: opus
    plans: true
    stance: |
      You answer for how it is built,
      and in what order.
    owns:
      - docs/tech/

  - id: designer
    title: Designer
    stance: |
      You speak for the player.
    owns:
      - docs/design/
`
	if string(out) != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
	if _, err := council.ParseRoster(out); err != nil {
		t.Errorf("the council cannot use it: %v", err)
	}
}

// A roster the council could not meet with is refused, and the reason is
// the council's own.
func TestApplyRoster_RefusesWhatTheCouncilCannotUse(t *testing.T) {
	for name, change := range map[string]func(r *Roster){
		"no decider":    func(r *Roster) { r.Roles[0].Decides = false },
		"two planners":  func(r *Roster) { r.Roles[2].Plans = true },
		"one role":      func(r *Roster) { r.Roles = r.Roles[:1] },
		"no stance":     func(r *Roster) { r.Roles[1].Stance = "  " },
		"bad id":        func(r *Roster) { r.Roles[1].ID = "Architect" },
		"same id twice": func(r *Roster) { r.Roles[2].ID = "architect" },
		"bad effort":    func(r *Roster) { r.Roles[2].Effort = "huge" },
		"two owners":    func(r *Roster) { r.Roles[2].Owns = []string{"docs/tech/"} },
	} {
		t.Run(name, func(t *testing.T) {
			roster := readRoster(t, rosterFile)
			change(roster)
			if out, err := ApplyRoster([]byte(rosterFile), *roster); err == nil {
				t.Errorf("accepted:\n%s", out)
			}
		})
	}
}

// A project that has no roster yet starts from the one its first plan would
// write; an empty file is a roster too.
func TestApplyRoster_FromTheStartingRosterOrNothing(t *testing.T) {
	start, err := council.StartingRoster([]council.Repo{{Name: "root", Language: "go", Path: "./"}})
	if err != nil {
		t.Fatal(err)
	}
	roster := readRoster(t, string(start))
	roster.Model = "opus"
	out, err := ApplyRoster(start, *roster)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "model: opus") {
		t.Errorf("model not set:\n%s", out)
	}

	out, err = ApplyRoster(nil, Roster{Roles: []Role{
		{ID: "a", Stance: "one", Decides: true},
		{ID: "b", Stance: "two", Plans: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(out); !strings.HasPrefix(got, "roles:\n  - id: a\n") {
		t.Errorf("got:\n%s", got)
	}
}

func TestDefaultsOfRoster(t *testing.T) {
	defaults := DefaultsOfRoster()
	if defaults.Model != council.DefaultModel || defaults.Limits.Topics == nil || *defaults.Limits.Topics != council.DefaultLimits().Topics || len(defaults.Efforts) == 0 {
		t.Errorf("defaults = %+v", defaults)
	}
}
