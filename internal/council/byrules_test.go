package council

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/rules"
)

// The table of the tests, with the builder's document kept with its repo.
var repoRoster = strings.Replace(testRoster, "owns: [docs/tech/architecture.md]", "owns: [backend/docs/architecture.md]", 1)

const (
	docsRule     = ".claude/rules/docs.md"
	backendRule  = ".claude/rules/backend.md"
	frontendRule = ".claude/rules/frontend.md"
	gitRule      = ".claude/rules/_kit/git-workflow.md"
	agentsRule   = ".claude/rules/agents.md"
)

// projectRules are the rules of the project of the tests: one about documents,
// one about each repo, one about everything, one that says nothing, and the
// one AWK ships.
func projectRules() []rules.Rule {
	return []rules.Rule{
		rules.Parse(gitRule, []byte("# Git\n\nA commit is `[type] subject`.\n")),
		rules.Parse(agentsRule, []byte("# Agents\n\nSay what you verified.\n")),
		rules.Parse(backendRule, []byte("---\npaths:\n  - \"backend/**\"\n---\n\n# Backend\n\nOne database is the source of truth.\n")),
		rules.Parse(docsRule, []byte("---\npaths: \"docs/**, **/docs/**\"\n---\n\n# Docs\n\nUnder its title a document has the line **Status**, which is `Plan` until it is built.\n")),
		rules.Parse(".claude/rules/empty.md", []byte("---\npaths: [\"docs/**\"]\n---\n\n")),
		rules.Parse(frontendRule, []byte("---\npaths:\n  - \"frontend/**\"\n---\n\n# Frontend\n\nC# only.\n")),
	}
}

func runWithRules(t *testing.T, root string, roster *Roster, model Model, all []rules.Rule) (*Plan, error) {
	t.Helper()
	return Run(context.Background(), Options{Root: root, Brief: "Add a health endpoint.", Roster: roster, Model: model, Repos: testRepos, Rules: all, Now: fixedNow})
}

// prompts returns every prompt a role was given for a step, in order.
func (m *table) prompts(role, step string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for _, c := range m.calls {
		if c.Role == role && c.Step == step {
			out = append(out, c.Prompt)
		}
	}
	return out
}

// checksOf returns the questions the reader was asked about one role's
// documents, in order.
func (m *table) checksOf(role *Role) []string {
	var out []string
	for _, prompt := range m.prompts("moderator", "rulecheck") {
		if strings.Contains(prompt, fmt.Sprintf("by their owner, the %s (%s).", role.Title, role.ID)) {
			out = append(out, prompt)
		}
	}
	return out
}

// following is the agreeable table whose documents the reader finds nothing
// wrong with.
func following(roster *Roster) func(c call) (string, error) {
	base := agreeable(roster)
	return func(c call) (string, error) {
		if c.Step == "rulecheck" {
			return block(`{"findings": []}`), nil
		}
		return base(c)
	}
}

// finding is the reader's answer with one finding about the builder's
// document.
func finding(point string) string {
	return block(fmt.Sprintf(`{"findings": [{"document": "backend/docs/architecture.md", "rule": ".claude/rules/docs.md", "point": %q, "fix": "Add the line under the title."}]}`, point))
}

const revised = "<<<DOCUMENT backend/docs/architecture.md>>>\n# Architecture\n\n**Status**: Plan\n\nRevised by builder.\n<<<END>>>\n"

// checked is the following table where the reader finds the builder's
// document at odds with the rule about documents, as often as found says, and
// the builder answers the finding with its revision.
func checked(roster *Roster, found int, revision string) func(c call) (string, error) {
	base := following(roster)
	builder := roster.Role("builder")
	asked := 0
	return func(c call) (string, error) {
		switch {
		case c.Step == "rulecheck" && strings.Contains(c.Prompt, fmt.Sprintf("the %s (%s).", builder.Title, builder.ID)):
			asked++
			if asked <= found {
				return finding(fmt.Sprintf("Reading %d: it has no **Status** line.", asked)), nil
			}
		case c.Step == "redocuments":
			return revision, nil
		}
		return base(c)
	}
}

// A role is given the rules that hold for the files it writes, the ones
// Claude Code would load for somebody working on them: a rule that names
// those files, and a rule that names none. It is told what such a rule may
// ask a document to state. What it wrote is then read against the same rules.
func TestRun_DocumentsAreWrittenByTheProjectsRules(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, repoRoster)
	model := &table{answer: following(roster)}

	plan, err := runWithRules(t, root, roster, model, projectRules())

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	const (
		docs     = "<rule file=\".claude/rules/docs.md\" about=\"the files that match docs/**, **/docs/**\">\n# Docs\n\nUnder its title a document has the line **Status**, which is `Plan` until it is built.\n</rule>\n"
		backend  = "<rule file=\".claude/rules/backend.md\" about=\"the files that match backend/**\">\n# Backend\n\nOne database is the source of truth.\n</rule>\n"
		agents   = "<rule file=\".claude/rules/agents.md\" about=\"every file of the project\">\n# Agents\n\nSay what you verified.\n</rule>\n"
		frontend = "<rule file=\".claude/rules/frontend.md\""
		empty    = ".claude/rules/empty.md"
		// The rule AWK ships is about git, which no role works with.
		git = "git-workflow.md"
	)
	for _, tt := range []struct {
		role      string
		has, lack []string
	}{
		{"owner", []string{agents, docs}, []string{backend, frontend, empty, git}},
		{"skeptic", []string{agents, docs}, []string{backend, frontend, empty, git}},
		{"builder", []string{agents + "\n" + backend + "\n" + docs}, []string{frontend, empty, git}},
	} {
		role := roster.Role(tt.role)
		facts := fmt.Sprintf("What holds for these documents, wherever a rule asks a document to say it:\n- their owner: %s (%s)\n- today's date: 2026-11-14\n- the plan they are written in: %s\n- how far things are: this is a planning meeting. Nothing this plan decides is built yet, and nobody at the table read the code or ran anything", role.ID, role.Title, plan.ID)
		writing := model.promptOf(tt.role, "documents")
		reading := model.checksOf(role)
		if len(reading) != 1 {
			t.Fatalf("the documents of %s were read against the rules %d times, want once", tt.role, len(reading))
		}
		for name, prompt := range map[string]string{"is asked to write": writing, "has its documents read": reading[0]} {
			for _, want := range append(tt.has, facts) {
				if !strings.Contains(prompt, want) {
					t.Errorf("when %s %s, the question should hold %q:\n%s", tt.role, name, want, prompt)
				}
			}
			for _, not := range tt.lack {
				if strings.Contains(prompt, not) {
					t.Errorf("when %s %s, the question holds %q, which is not about its files", tt.role, name, not)
				}
			}
		}
		for _, want := range []string{"The project has rules (.claude/rules/). These hold for the files you write, and your documents follow them:",
			fmt.Sprintf("Where no rule says how a document starts, start it with its title, and under it this line:\nOwner: %s. Written: 2026-11-14, in plan %s.\n", role.ID, plan.ID),
			"One document is at most 40000 bytes: the council writes a document whole.", "<<<DOCUMENT path/of/the/document.md>>>"} {
			if !strings.Contains(writing, want) {
				t.Errorf("the question to %s should say %q:\n%s", tt.role, want, writing)
			}
		}
		// What awkit prescribed before the project's rules did is gone.
		for _, gone := range []string{"status: draft", "under 300 lines", "updated: "} {
			if strings.Contains(writing, gone) {
				t.Errorf("the question to %s still says %q", tt.role, gone)
			}
		}
		// The reader is shown what the role wrote, and what the role may write.
		for _, want := range []string{"You check documents against the rules of a software project. You have no opinion on their subject",
			fmt.Sprintf("The owner writes these files and no other:\n- %s\n", role.Owns[0]),
			fmt.Sprintf("The documents, as they stand:\n\n===== %s\n# %s\n\nOwner: %s.\n\nWritten by %s.\n", role.Owns[0], role.Owns[0], role.ID, role.ID),
			"At most 8 findings"} {
			if !strings.Contains(reading[0], want) {
				t.Errorf("the reader of %s's documents should be told %q:\n%s", tt.role, want, reading[0])
			}
		}
	}

	// Nothing was found: nobody revises, and the record says so.
	if got := len(model.prompts("moderator", "rulecheck")); got != 4 {
		t.Errorf("%d readings against the rules, want one for each of the four roles", got)
	}
	for _, role := range roster.IDs() {
		if got := model.count(role, "redocuments"); got != 0 {
			t.Errorf("%s was asked to put something right %d times, and nothing was found", role, got)
		}
	}
	if want := []string{agentsRule, backendRule, docsRule}; !reflect.DeepEqual(plan.Rules, want) || plan.RulesLeft != nil {
		t.Errorf("the plan was written by %v (left: %v), want %v", plan.Rules, plan.RulesLeft, want)
	}
	if len(plan.RuleChecks) != 4 {
		t.Fatalf("%d checks on record, want four", len(plan.RuleChecks))
	}
	for i, check := range plan.RuleChecks {
		if check.Owner != roster.IDs()[i] || !check.Settled || check.Found != nil || check.Open != nil {
			t.Errorf("check %d = %+v", i, check)
		}
	}
	if open := plan.OpenFindings(); open != nil {
		t.Errorf("open findings = %v", open)
	}
	minutes := readFile(t, root, plan.Dir()+"/minutes.md")
	for _, want := range []string{"- `backend/docs/architecture.md` (builder)\n",
		"### By the project's rules\n\nEach role was given the rules that hold for the files it writes: `.claude/rules/agents.md`, `.claude/rules/backend.md`, `.claude/rules/docs.md`. What it wrote in this meeting was read against them by a reader who takes no side, and the owner put right what was found, once.\n\n- owner: nothing at odds with the rules\n- designer: nothing at odds with the rules\n- builder: nothing at odds with the rules\n- skeptic: nothing at odds with the rules\n"} {
		if !strings.Contains(minutes, want) {
			t.Errorf("the minutes should say %q:\n%s", want, minutes)
		}
	}
	// A document kept with a repo is in the index, by the way from there.
	if index := readFile(t, root, "docs/README.md"); !strings.Contains(index, "| [backend/docs/architecture.md](../backend/docs/architecture.md) | builder |\n") {
		t.Errorf("the index does not lead to the document kept with the repo:\n%s", index)
	}
	// A role writes first and is read after.
	steps := strings.Join(model.steps(), "\n")
	if strings.Index(steps, "moderator rulecheck") < strings.Index(steps, "owner documents") {
		t.Errorf("a document was read before the first was written:\n%s", steps)
	}
}

// What the reader finds, the owner puts right, once, and what it changed is
// read again. The record says what was found and that nothing still stands.
func TestRun_WhatTheCheckFindsIsPutRightOnce(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, repoRoster)
	model := &table{answer: checked(roster, 1, revised)}
	var told []string

	plan, err := Run(context.Background(), Options{Root: root, Brief: "Add a health endpoint.", Roster: roster, Model: model, Repos: testRepos,
		Rules: projectRules(), Now: fixedNow, Progress: func(line string) { told = append(told, line) }})

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := model.count("builder", "redocuments"); got != 1 {
		t.Fatalf("the builder was asked to put it right %d times, want once", got)
	}
	for _, role := range []string{"owner", "designer", "skeptic"} {
		if got := model.count(role, "redocuments"); got != 0 {
			t.Errorf("%s was asked to put something right, and nothing was found in its documents", role)
		}
	}
	asked := model.promptOf("builder", "redocuments")
	for _, want := range []string{
		"Your documents as they stand:\n===== backend/docs/architecture.md\n# backend/docs/architecture.md\n\nOwner: builder.\n\nWritten by builder.\n",
		"The meeting is over. What was decided:\nD1. scope: Build the health endpoint only.",
		"The project's rules that hold for the files you write (.claude/rules/):\n\n<rule file=\".claude/rules/agents.md\"",
		"<rule file=\".claude/rules/docs.md\" about=\"the files that match docs/**, **/docs/**\">",
		"- their owner: builder (Builder)\n- today's date: 2026-11-14\n",
		"Your documents were read against these rules. What the check found:\n1. backend/docs/architecture.md, against .claude/rules/docs.md: Reading 1: it has no **Status** line.\n   To put right: Add the line under the title.\n",
		"Your task now: put right what the check found.", "Change nothing else in a document",
		"<<<UNCHANGED path/of/the/document.md>>>"} {
		if !strings.Contains(asked, want) {
			t.Errorf("the builder should be told %q:\n%s", want, asked)
		}
	}
	if got := readFile(t, root, "backend/docs/architecture.md"); got != "# Architecture\n\n**Status**: Plan\n\nRevised by builder.\n" {
		t.Errorf("the document is not the one the builder handed in the second time:\n%s", got)
	}
	// What the builder changed is read again, as it now stands, and no third time.
	readings := model.checksOf(roster.Role("builder"))
	if len(readings) != 2 {
		t.Fatalf("the builder's document was read %d times, want twice", len(readings))
	}
	if !strings.Contains(readings[1], "===== backend/docs/architecture.md\n# Architecture\n\n**Status**: Plan\n\nRevised by builder.\n") || strings.Contains(readings[1], "Written by builder.") {
		t.Errorf("the second reading is not of the document as it now stands:\n%s", readings[1])
	}

	var check RuleCheck
	for _, c := range plan.RuleChecks {
		if c.Owner == "builder" {
			check = c
		}
	}
	want := RuleFinding{Owner: "builder", Document: "backend/docs/architecture.md", Rule: docsRule, Point: "Reading 1: it has no **Status** line.", Fix: "Add the line under the title."}
	if !check.Settled || !reflect.DeepEqual(check.Found, []RuleFinding{want}) || len(check.Open) != 0 || plan.OpenFindings() != nil {
		t.Errorf("the builder's check = %+v", check)
	}
	for _, doc := range plan.Documents {
		if doc.Path == "backend/docs/architecture.md" && (doc.Unchanged || doc.Refused != "") {
			t.Errorf("the revised document is on record as %+v", doc)
		}
	}
	minutes := readFile(t, root, plan.Dir()+"/minutes.md")
	if want := "- builder: 1 point(s) found and put right\n  - Found: `backend/docs/architecture.md` against `.claude/rules/docs.md`: Reading 1: it has no **Status** line.\n    - To put right: Add the line under the title.\n- skeptic: nothing at odds with the rules\n"; !strings.Contains(minutes, want) {
		t.Errorf("the minutes should say %q:\n%s", want, minutes)
	}
	if strings.Contains(minutes, "Still stands") {
		t.Errorf("the minutes say something still stands:\n%s", minutes)
	}
	progress := strings.Join(told, "\n")
	for _, want := range []string{"Documents: 4 roles write what they own, by 3 rule(s) of the project", "  builder: 1 point(s) at odds with the rules; builder puts them right",
		"  rules check builder (", "  documents revised builder (", "  rules recheck builder ("} {
		if !strings.Contains(progress, want) {
			t.Errorf("whoever watches should be told %q:\n%s", want, progress)
		}
	}
	if strings.Contains(progress, "still at odds") {
		t.Errorf("whoever watches is told something is still at odds:\n%s", progress)
	}

	// The work list is drawn up from the document as it was put right.
	if worklist := model.promptOf("builder", "worklist"); !strings.Contains(worklist, "===== backend/docs/architecture.md (owner: builder; written in this meeting)\n# Architecture\n\n**Status**: Plan\n") {
		t.Errorf("the planning role is not shown the document as it was put right:\n%s", worklist)
	}

	// Going on with the finished plan asks nothing and writes nothing.
	const byHand = "# Architecture\n\nCorrected by hand.\n"
	putFile(t, root, "backend/docs/architecture.md", byHand)
	before := len(model.steps())
	again, err := Run(context.Background(), Options{Root: root, ID: plan.ID, Roster: roster, Model: model, Repos: testRepos, Rules: projectRules(), Now: fixedNow})
	if err != nil {
		t.Fatalf("going on: %v", err)
	}
	if asked := model.steps()[before:]; len(asked) != 0 {
		t.Errorf("going on with a finished plan asked: %v", asked)
	}
	if got := readFile(t, root, "backend/docs/architecture.md"); got != byHand {
		t.Errorf("going on wrote over what a person corrected:\n%s", got)
	}
	if !reflect.DeepEqual(again.RuleChecks, plan.RuleChecks) || !reflect.DeepEqual(again.Rules, plan.Rules) {
		t.Errorf("going on tells of other checks: %+v, was %+v", again.RuleChecks, plan.RuleChecks)
	}
}

// What still stands after the owner's turn is not argued a third time: it is
// on record for the person who approves the plan.
func TestRun_WhatStillStandsIsOnRecord(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, repoRoster)
	model := &table{answer: checked(roster, 2, revised)}
	var told []string

	plan, err := Run(context.Background(), Options{Root: root, Brief: "Add a health endpoint.", Roster: roster, Model: model, Repos: testRepos,
		Rules: projectRules(), Now: fixedNow, Progress: func(line string) { told = append(told, line) }})

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, readings := model.count("builder", "redocuments"), len(model.checksOf(roster.Role("builder"))); got != 1 || readings != 2 {
		t.Errorf("the builder revised %d times and was read %d times; want one revision and two readings", got, readings)
	}
	open := plan.OpenFindings()
	if len(open) != 1 || open[0].Point != "Reading 2: it has no **Status** line." || open[0].Owner != "builder" || open[0].Rule != docsRule {
		t.Fatalf("open findings = %+v", open)
	}
	minutes := readFile(t, root, plan.Dir()+"/minutes.md")
	if want := "- builder: 1 point(s) found; **1 still stand(s)** after the owner's turn\n  - Found: `backend/docs/architecture.md` against `.claude/rules/docs.md`: Reading 1: it has no **Status** line.\n    - To put right: Add the line under the title.\n  - **Still stands**: `backend/docs/architecture.md` against `.claude/rules/docs.md`: Reading 2: it has no **Status** line.\n    - To put right: Add the line under the title.\n"; !strings.Contains(minutes, want) {
		t.Errorf("the minutes should say %q:\n%s", want, minutes)
	}
	if !strings.Contains(strings.Join(told, "\n"), "  builder: 1 point(s) still at odds with the rules") {
		t.Errorf("whoever watches is not told that a point still stands:\n%s", strings.Join(told, "\n"))
	}
	// The meeting goes on all the same: a plan does not wait.
	if len(plan.Tickets) != 2 || !strings.Contains(minutes, "Status: finished.") {
		t.Errorf("the meeting did not finish: %d tickets", len(plan.Tickets))
	}
}

// An owner who leaves its documents as they are leaves the findings standing:
// nothing changed, so nothing is read again.
func TestRun_AnOwnerWhoChangesNothingLeavesTheFindingsStanding(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, repoRoster)
	model := &table{answer: checked(roster, 9, "The finding misreads the rule.\n<<<UNCHANGED backend/docs/architecture.md>>>\n")}

	plan, err := runWithRules(t, root, roster, model, projectRules())

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if readings := len(model.checksOf(roster.Role("builder"))); readings != 1 {
		t.Errorf("a document nobody changed was read %d times, want once", readings)
	}
	if got := readFile(t, root, "backend/docs/architecture.md"); !strings.Contains(got, "Written by builder.") {
		t.Errorf("the document was changed:\n%s", got)
	}
	var check RuleCheck
	for _, c := range plan.RuleChecks {
		if c.Owner == "builder" {
			check = c
		}
	}
	if !check.Settled || len(check.Found) != 1 || !reflect.DeepEqual(check.Open, check.Found) {
		t.Errorf("the builder's check = %+v; want what was found to stand", check)
	}
	for _, doc := range plan.Documents {
		if doc.Path == "backend/docs/architecture.md" && doc.Unchanged {
			t.Error("the document the meeting wrote is on record as left unchanged")
		}
	}
}

// The revision is handed in like the documents were: a role writes what it
// owns, and answers for every document a finding names.
func TestRun_TheRevisionIsHandedInLikeTheDocuments(t *testing.T) {
	two := strings.Replace(repoRoster, "owns: [backend/docs/architecture.md]", "owns: [backend/docs/architecture.md, backend/docs/protocol.md]", 1)
	const protocol = "# Protocol\n\nJSON over HTTP.\n"

	// The builder leaves the protocol as it was when it writes, and changes it
	// to put a finding about the architecture right. Both are then read.
	t.Run("a document left at first and changed for a finding", func(t *testing.T) {
		root := t.TempDir()
		roster := mustRoster(t, two)
		putFile(t, root, "backend/docs/protocol.md", protocol)
		base := checked(roster, 1, "<<<UNCHANGED backend/docs/architecture.md>>>\n<<<DOCUMENT backend/docs/protocol.md>>>\n# Protocol\n\n**Status**: Plan\n<<<END>>>\n<<<DOCUMENT docs/qa/risks.md>>>\n# Not mine\n<<<END>>>\n")
		model := &table{answer: func(c call) (string, error) {
			if c.Step == "documents" && c.Role == "builder" {
				return docs("builder", "backend/docs/architecture.md") + "<<<UNCHANGED backend/docs/protocol.md>>>\n", nil
			}
			return base(c)
		}}

		plan, err := runWithRules(t, root, roster, model, projectRules())

		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		readings := model.checksOf(roster.Role("builder"))
		if len(readings) != 2 {
			t.Fatalf("%d readings, want two", len(readings))
		}
		// First only what the meeting wrote; then what the revision wrote too.
		if strings.Contains(readings[0], "===== backend/docs/protocol.md") || !strings.Contains(readings[0], "===== backend/docs/architecture.md") {
			t.Errorf("the first reading is not of the one document the meeting wrote:\n%s", readings[0])
		}
		if !strings.Contains(readings[1], "===== backend/docs/architecture.md\n") || !strings.Contains(readings[1], "===== backend/docs/protocol.md\n# Protocol\n\n**Status**: Plan\n") {
			t.Errorf("the second reading is not of both documents as they stand:\n%s", readings[1])
		}
		// What was turned away was not written: there is nothing of it to read.
		if strings.Contains(readings[1], "===== docs/qa/risks.md") {
			t.Errorf("the second reading holds a document that was turned away:\n%s", readings[1])
		}
		state := map[string]WrittenDocument{}
		refused := 0
		for _, doc := range plan.Documents {
			if doc.Owner == "builder" && doc.Refused != "" {
				refused++
				if doc.Path != "docs/qa/risks.md" || doc.Refused != "builder does not own this document" {
					t.Errorf("turned away: %+v", doc)
				}
				continue
			}
			state[doc.Path] = doc
		}
		if refused != 1 {
			t.Errorf("%d documents of the builder were turned away, want the one it does not own", refused)
		}
		if state["backend/docs/protocol.md"].Unchanged || state["backend/docs/architecture.md"].Unchanged {
			t.Errorf("documents = %+v; want both on record as written", plan.Documents)
		}
		// The skeptic's document is the skeptic's.
		if got := readFile(t, root, "docs/qa/risks.md"); !strings.Contains(got, "Written by skeptic.") {
			t.Errorf("a revision wrote over another role's document:\n%s", got)
		}
		if minutes := readFile(t, root, plan.Dir()+"/minutes.md"); !strings.Contains(minutes, "- `docs/qa/risks.md` handed in by builder: **not written**, builder does not own this document\n") {
			t.Errorf("the minutes do not say that a document was turned away:\n%s", minutes)
		}
	})

	// A revision that does not answer for a document a finding names is asked
	// for again; so is one that cannot be read.
	for name, tt := range map[string]struct{ bad, told string }{
		"a document a finding names is not answered for": {"<<<DOCUMENT backend/docs/protocol.md>>>\n# Protocol\n<<<END>>>\n",
			"backend/docs/architecture.md was not answered for, and a finding names it: hand it in, or leave it as it is with <<<UNCHANGED backend/docs/architecture.md>>>"},
		"no document at all":         {"I see no problem.", "the answer holds no document"},
		"an empty document":          {"<<<DOCUMENT backend/docs/architecture.md>>>\n\n<<<END>>>\n", "backend/docs/architecture.md is empty"},
		"a document handed in twice": {revised + revised, "backend/docs/architecture.md was handed in twice"},
	} {
		root := t.TempDir()
		roster := mustRoster(t, two)
		base := checked(roster, 1, revised)
		asked := 0
		var second string
		model := &table{answer: func(c call) (string, error) {
			if c.Step == "redocuments" {
				if asked++; asked == 1 {
					return tt.bad, nil
				}
				second = c.Prompt
			}
			return base(c)
		}}
		if _, err := runWithRules(t, root, roster, model, projectRules()); err != nil {
			t.Errorf("%s: Run: %v", name, err)
			continue
		}
		if asked != 2 || !strings.Contains(second, "Your previous answer could not be used: "+tt.told) {
			t.Errorf("%s: asked %d times; the second question should say %q:\n%s", name, asked, tt.told, second[max(0, len(second)-500):])
		}
	}
}

// What the reader answers is checked like any answer: a finding is about a
// document that was read and names a rule that was shown.
func TestRun_FindingsThatCannotBeUsed(t *testing.T) {
	roster := mustRoster(t, repoRoster)
	for name, tt := range map[string]struct{ bad, told string }{
		"about a document that was not read": {`{"findings": [{"document": "docs/qa/risks.md", "rule": ".claude/rules/docs.md", "point": "p"}]}`,
			`a finding is about "docs/qa/risks.md", which is not one of the documents (backend/docs/architecture.md)`},
		"a rule that was not shown": {`{"findings": [{"document": "backend/docs/architecture.md", "rule": ".claude/rules/frontend.md", "point": "p"}]}`,
			`a finding names the rule ".claude/rules/frontend.md", which is not one of the rules shown (.claude/rules/agents.md, .claude/rules/backend.md, .claude/rules/docs.md)`},
		"the rule AWK ships, which is not passed on": {`{"findings": [{"document": "backend/docs/architecture.md", "rule": ".claude/rules/_kit/git-workflow.md", "point": "p"}]}`,
			`a finding names the rule ".claude/rules/_kit/git-workflow.md", which is not one of the rules shown`},
		"no rule":  {`{"findings": [{"document": "backend/docs/architecture.md", "rule": "", "point": "p"}]}`, `a finding names the rule ""`},
		"no point": {`{"findings": [{"document": "backend/docs/architecture.md", "rule": ".claude/rules/docs.md", "point": "  "}]}`, "a finding has no point"},
		"no JSON":  {"The documents look fine to me.", "the JSON of the answer cannot be read"},
	} {
		base := following(roster)
		asked := 0
		var second string
		model := &table{answer: func(c call) (string, error) {
			if c.Step == "rulecheck" && strings.Contains(c.Prompt, "the Builder (builder).") {
				if asked++; asked == 1 {
					return block(tt.bad), nil
				}
				second = c.Prompt
			}
			return base(c)
		}}
		plan, err := runWithRules(t, t.TempDir(), roster, model, projectRules())
		if err != nil {
			t.Errorf("%s: Run: %v", name, err)
			continue
		}
		if asked != 2 || !strings.Contains(second, "Your previous answer could not be used: "+tt.told) {
			t.Errorf("%s: asked %d times; the second question should say %q:\n%s", name, asked, tt.told, second[max(0, len(second)-600):])
		}
		if open := plan.OpenFindings(); open != nil {
			t.Errorf("%s: an answer that could not be used left findings: %+v", name, open)
		}
	}

	// A rule is named the way a reader names it: by its file, by the name of
	// the file, or by its title. Whatever was written, the record has the file.
	for _, named := range []string{".claude/rules/docs.md", "`.claude/rules/docs.md`", "docs.md", "docs", "Docs", "rules/docs.md", " DOCS.md "} {
		base := following(roster)
		model := &table{answer: func(c call) (string, error) {
			if c.Step == "rulecheck" && strings.Contains(c.Prompt, "the Builder (builder).") {
				return block(fmt.Sprintf(`{"findings": [{"document": "`+"`backend/docs/architecture.md`"+`", "rule": %q, "point": "p", "fix": ""}]}`, named)), nil
			}
			if c.Step == "redocuments" {
				return "<<<UNCHANGED backend/docs/architecture.md>>>\n", nil
			}
			return base(c)
		}}
		plan, err := runWithRules(t, t.TempDir(), roster, model, projectRules())
		if err != nil {
			t.Errorf("a rule named %q: Run: %v", named, err)
			continue
		}
		if open := plan.OpenFindings(); len(open) != 1 || open[0].Rule != docsRule || open[0].Document != "backend/docs/architecture.md" {
			t.Errorf("a rule named %q is on record as %+v", named, open)
		}
		if got := model.count("moderator", "rulecheck"); got != 4 {
			t.Errorf("a rule named %q: the reader was asked %d times, want once for each role", named, got)
		}
	}

	// More findings than are kept: the first eight, which matter most.
	var many []string
	for i := 1; i <= 11; i++ {
		many = append(many, fmt.Sprintf(`{"document": "backend/docs/architecture.md", "rule": "docs", "point": "point %d"}`, i))
	}
	base := following(roster)
	model := &table{answer: func(c call) (string, error) {
		if c.Step == "rulecheck" && strings.Contains(c.Prompt, "the Builder (builder).") {
			return block(`{"findings": [` + strings.Join(many, ", ") + `]}`), nil
		}
		if c.Step == "redocuments" {
			return "<<<UNCHANGED backend/docs/architecture.md>>>\n", nil
		}
		return base(c)
	}}
	plan, err := runWithRules(t, t.TempDir(), roster, model, projectRules())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if open := plan.OpenFindings(); len(open) != 8 || open[0].Point != "point 1" || open[7].Point != "point 8" {
		t.Errorf("%d findings kept, want the first eight: %+v", len(open), open)
	}
}

// Only what the meeting wrote is read against the rules. A document its owner
// left as it was is not this meeting's, and a role that wrote nothing has
// nothing to be read.
func TestRun_OnlyWhatTheMeetingWroteIsRead(t *testing.T) {
	root := t.TempDir()
	two := strings.Replace(repoRoster, "owns: [docs/qa/risks.md]", "owns: [docs/qa/risks.md, docs/qa/tests.md]", 1)
	roster := mustRoster(t, two)
	putFile(t, root, "docs/qa/risks.md", "# Risks\n\nAs they were.\n")
	putFile(t, root, "docs/product/journeys.md", "# Journeys\n\nAs they were.\n")
	base := following(roster)
	model := &table{answer: func(c call) (string, error) {
		switch {
		case c.Step == "documents" && c.Role == "skeptic":
			return "<<<UNCHANGED docs/qa/risks.md>>>\n" + docs("skeptic", "docs/qa/tests.md"), nil
		case c.Step == "documents" && c.Role == "designer":
			return "<<<UNCHANGED docs/product/journeys.md>>>\n", nil
		}
		return base(c)
	}}

	plan, err := runWithRules(t, root, roster, model, projectRules())

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	skeptic := model.checksOf(roster.Role("skeptic"))
	if len(skeptic) != 1 || !strings.Contains(skeptic[0], "===== docs/qa/tests.md\n") || strings.Contains(skeptic[0], "===== docs/qa/risks.md") {
		t.Errorf("the skeptic's documents were read %d times; want once, the one it wrote:\n%s", len(skeptic), strings.Join(skeptic, "\n---\n"))
	}
	// The reader is told everything the role may write, read or not.
	if len(skeptic) == 1 && !strings.Contains(skeptic[0], "The owner writes these files and no other:\n- docs/qa/risks.md\n- docs/qa/tests.md\n") {
		t.Errorf("the reader is not told what the skeptic owns:\n%s", skeptic[0])
	}
	if got := len(model.checksOf(roster.Role("designer"))); got != 0 {
		t.Errorf("a role that wrote nothing was read %d times", got)
	}
	var owners []string
	for _, check := range plan.RuleChecks {
		owners = append(owners, check.Owner)
	}
	if want := []string{"owner", "builder", "skeptic"}; !reflect.DeepEqual(owners, want) {
		t.Errorf("checks on record for %v, want %v", owners, want)
	}
	// The rules are passed on to a role all the same: it decides what to leave.
	if designer := model.promptOf("designer", "documents"); !strings.Contains(designer, "<rule file=\".claude/rules/docs.md\"") {
		t.Errorf("a role with a document to leave or change is not given the rules:\n%s", designer)
	}
}

// A project without rules for a role's files gives that role none: it is told
// how a document starts, and nothing is read. A rule whose frontmatter cannot
// be read names no file, so it holds for every role, as it does for Claude
// Code.
func TestRun_RulesThatHoldForNobodyAndForEverybody(t *testing.T) {
	roster := mustRoster(t, repoRoster)
	root := t.TempDir()
	model := &table{answer: following(roster)}

	plan, err := runWithRules(t, root, roster, model, []rules.Rule{
		rules.Parse(frontendRule, []byte("---\npaths: [\"frontend/**\"]\n---\n\n# Frontend\n\nC# only.\n")),
		rules.Parse(backendRule, []byte("---\npaths: [\"backend/**\"]\n---\n\n# Backend\n\nOne database.\n")),
	})

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	owner := model.promptOf("owner", "documents")
	if want := "\nStart every document you write with its title, and under it this line:\nOwner: owner. Written: 2026-11-14, in plan " + plan.ID + ".\n"; !strings.Contains(owner, want) || strings.Contains(owner, "<rule") || strings.Contains(owner, "The project has rules") {
		t.Errorf("a role no rule holds for should be told %q and given no rule:\n%s", want, owner)
	}
	if builder := model.promptOf("builder", "documents"); !strings.Contains(builder, "<rule file=\".claude/rules/backend.md\"") || strings.Contains(builder, "frontend.md") {
		t.Errorf("the builder is not given the one rule about its files:\n%s", builder)
	}
	if got := len(model.prompts("moderator", "rulecheck")); got != 1 {
		t.Errorf("%d readings, want the one of the builder's document", got)
	}
	if !reflect.DeepEqual(plan.Rules, []string{backendRule}) || len(plan.RuleChecks) != 1 || plan.RuleChecks[0].Owner != "builder" {
		t.Errorf("rules %v, checks %+v", plan.Rules, plan.RuleChecks)
	}

	broken := rules.Parse(".claude/rules/broken.md", []byte("---\npaths: [frontend/**\n---\n\n# Broken\n\nMeant for the frontend.\n"))
	if broken.Problem == "" || !broken.Everywhere() {
		t.Fatalf("the rule of the test can be read: %+v", broken)
	}
	model = &table{answer: following(roster)}
	if _, err := runWithRules(t, t.TempDir(), roster, model, []rules.Rule{broken}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, role := range roster.IDs() {
		if prompt := model.promptOf(role, "documents"); !strings.Contains(prompt, "<rule file=\".claude/rules/broken.md\" about=\"every file of the project\">\n# Broken\n\nMeant for the frontend.\n</rule>") {
			t.Errorf("%s is not given the rule that cannot be read:\n%s", role, prompt)
		}
	}
}

// The rules a question carries are bounded. A rule that names the files comes
// before one about everything; what does not fit is named, to the role, to
// the reader, to whoever watches and in the minutes, and nothing is read
// against it.
func TestRun_RulesTooLongToPassOn(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, repoRoster)
	model := &table{answer: following(roster)}
	long := func(file, frontmatter, title string, size int) rules.Rule {
		return rules.Parse(file, []byte(frontmatter+"# "+title+"\n\n"+strings.Repeat("A line of the rule that is here to take room.\n", size/46)))
	}
	all := []rules.Rule{
		long(".claude/rules/a-everything.md", "", "Everything", 45000),
		long(".claude/rules/b-docs.md", "---\npaths: \"docs/**, **/docs/**\"\n---\n\n", "Documents", 30000),
		long(".claude/rules/c-small.md", "", "Small", 1000),
		long(".claude/rules/d-backend.md", "---\npaths: backend/**\n---\n\n", "Backend", 25000),
	}
	var told []string

	plan, err := Run(context.Background(), Options{Root: root, Brief: "Add a health endpoint.", Roster: roster, Model: model, Repos: testRepos,
		Rules: all, Now: fixedNow, Progress: func(line string) { told = append(told, line) }})

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// For a document under docs/: the rule about documents (30 KB), then, of
	// the two about everything, the one that still fits.
	owner := model.promptOf("owner", "documents")
	for _, want := range []string{"<rule file=\".claude/rules/b-docs.md\"", "<rule file=\".claude/rules/c-small.md\"",
		"These rules hold too. They are not shown, because the rules together are longer than one question carries:\n- .claude/rules/a-everything.md (Everything)\n"} {
		if !strings.Contains(owner, want) {
			t.Errorf("the owner's question should hold %q", want)
		}
	}
	if strings.Contains(owner, "<rule file=\".claude/rules/a-everything.md\"") || strings.Contains(owner, "d-backend.md") {
		t.Error("the owner's question shows a rule that does not fit, or one that is not about its files")
	}
	// For the builder's: the two that name its files (55 KB) come first, and
	// only the small one fits beside them.
	builder := model.promptOf("builder", "documents")
	for _, want := range []string{"<rule file=\".claude/rules/b-docs.md\"", "<rule file=\".claude/rules/d-backend.md\"", "<rule file=\".claude/rules/c-small.md\"", "- .claude/rules/a-everything.md (Everything)\n"} {
		if !strings.Contains(builder, want) {
			t.Errorf("the builder's question should hold %q", want)
		}
	}
	if len(builder) > 120000 {
		t.Errorf("the builder's question is %d bytes long", len(builder))
	}
	if reading := model.checksOf(roster.Role("builder")); len(reading) != 1 || !strings.Contains(reading[0], "- .claude/rules/a-everything.md (Everything)\n") || strings.Contains(reading[0], "<rule file=\".claude/rules/a-everything.md\"") {
		t.Error("the reader is not told which rule it is not shown")
	}
	if want := []string{".claude/rules/b-docs.md", ".claude/rules/c-small.md", ".claude/rules/d-backend.md"}; !reflect.DeepEqual(plan.Rules, want) {
		t.Errorf("written by %v, want %v", plan.Rules, want)
	}
	if want := []string{".claude/rules/a-everything.md"}; !reflect.DeepEqual(plan.RulesLeft, want) {
		t.Errorf("left out %v, want %v", plan.RulesLeft, want)
	}
	if progress := strings.Join(told, "\n"); !strings.Contains(progress, "  not passed on, the rules together being longer than one question carries: .claude/rules/a-everything.md") {
		t.Errorf("whoever watches is not told a rule was left out:\n%s", progress)
	}
	if minutes := readFile(t, root, plan.Dir()+"/minutes.md"); !strings.Contains(minutes, "\nNot passed on, because the rules together are longer than one question carries: `.claude/rules/a-everything.md`. Nothing was read against them.\n") {
		t.Errorf("the minutes do not say a rule was left out:\n%s", minutes)
	}

	// A project whose one rule is too long to pass on: the documents are
	// written without, nothing is read, and the minutes say why.
	root = t.TempDir()
	model = &table{answer: following(roster)}
	plan, err = runWithRules(t, root, roster, model, []rules.Rule{long(".claude/rules/huge.md", "", "Huge", maxRules+100)})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if plan.Rules != nil || !reflect.DeepEqual(plan.RulesLeft, []string{".claude/rules/huge.md"}) || plan.RuleChecks != nil {
		t.Errorf("written by %v, left %v, checks %v", plan.Rules, plan.RulesLeft, plan.RuleChecks)
	}
	if got := model.count("moderator", "rulecheck"); got != 0 {
		t.Errorf("documents written without a rule were read %d times", got)
	}
	if owner := model.promptOf("owner", "documents"); strings.Contains(owner, "<rule") || !strings.Contains(owner, "\nStart every document you write with its title") {
		t.Errorf("a role that is passed no rule:\n%s", owner)
	}
	if minutes := readFile(t, root, plan.Dir()+"/minutes.md"); !strings.Contains(minutes, "### By the project's rules\n\nNot passed on, because the rules together are longer than one question carries: `.claude/rules/huge.md`. Nothing was read against them.\n") || strings.Contains(minutes, "Each role was given") {
		t.Errorf("the minutes of a plan no rule was passed on to:\n%s", minutes)
	}

	// The bound itself is 60000 bytes, the number the documentation gives: a
	// rule of exactly the room that is left fits, one byte more does not.
	p := &planner{rules: []rules.Rule{{File: "a.md", Text: strings.Repeat("x", 59990)}, {File: "b.md", Text: strings.Repeat("y", 10)}, {File: "c.md", Text: "z"}}}
	if book := p.rulesFor([]string{"docs/a.md"}); !reflect.DeepEqual(book.files(), []string{"a.md", "b.md"}) || len(book.left) != 1 || book.left[0].File != "c.md" {
		t.Errorf("shown %v, left %v; want the rules that fill the room, and the next one left", book.files(), book.left)
	}
	p = &planner{rules: []rules.Rule{{File: "a.md", Text: strings.Repeat("x", 60001)}, {File: "b.md", Text: strings.Repeat("y", 60000)}}}
	if book := p.rulesFor([]string{"docs/a.md"}); !reflect.DeepEqual(book.files(), []string{"b.md"}) || len(book.left) != 1 {
		t.Errorf("shown %v, left %v; want the rule that is as long as the room", book.files(), book.left)
	}
	if book := (&planner{}).rulesFor([]string{"docs/a.md"}); book.shown != nil || book.left != nil || book.text() != "" {
		t.Errorf("a project without rules: %+v", book)
	}
	if book := p.rulesFor(nil); book.shown != nil || book.left != nil {
		t.Errorf("no file: %+v", book)
	}
}

// A meeting that stops while the documents are read goes on from there: what
// was written is not asked for again, and the record says how far it got.
func TestRun_StoppedWhileTheDocumentsWereRead(t *testing.T) {
	roster := mustRoster(t, strings.Replace(repoRoster, "parallel: 2", "parallel: 1", 1))

	t.Run("before a reading", func(t *testing.T) {
		root := t.TempDir()
		base := checked(roster, 0, revised)
		down := true
		model := &table{answer: func(c call) (string, error) {
			if down && c.Step == "rulecheck" && strings.Contains(c.Prompt, "the Builder (builder).") {
				return "", errors.New("claude failed: exit status 1")
			}
			return base(c)
		}}

		plan, err := runWithRules(t, root, roster, model, projectRules())

		if err == nil || !strings.Contains(err.Error(), "the meeting stopped at the documents: rules check builder: claude failed") {
			t.Fatalf("err = %v, want the meeting stopped at the reading of the builder's documents", err)
		}
		// What was written is on record, with how far the reading got.
		if len(plan.Documents) != 4 {
			t.Errorf("%d documents on record, want the four that were written: %+v", len(plan.Documents), plan.Documents)
		}
		minutes := readFile(t, root, plan.Dir()+"/minutes.md")
		for _, want := range []string{"Status: **not finished**", "- `backend/docs/architecture.md` (builder)\n", "- designer: nothing at odds with the rules\n",
			"- builder: **not read**; the meeting stopped first\n", "- skeptic: nothing at odds with the rules\n"} {
			if !strings.Contains(minutes, want) {
				t.Errorf("the minutes should say %q:\n%s", want, minutes)
			}
		}
		if open := plan.OpenFindings(); open != nil {
			t.Errorf("open findings = %+v", open)
		}

		down = false
		before := len(model.steps())
		again, err := Run(context.Background(), Options{Root: root, ID: plan.ID, Roster: roster, Model: model, Repos: testRepos, Rules: projectRules(), Now: fixedNow})
		if err != nil {
			t.Fatalf("going on: %v", err)
		}
		asked := model.steps()[before:]
		if len(asked) == 0 || asked[0] != "moderator rulecheck" {
			t.Errorf("going on asked %v, want the reading that failed first", asked)
		}
		for _, step := range asked {
			if strings.HasSuffix(step, " documents") {
				t.Errorf("going on asked for documents that were written: %v", asked)
			}
		}
		if got := len(model.checksOf(roster.Role("owner"))); got != 1 {
			t.Errorf("the owner's documents were read %d times over both runs, want once", got)
		}
		if len(again.RuleChecks) != 4 || !again.RuleChecks[2].Settled {
			t.Errorf("checks after going on: %+v", again.RuleChecks)
		}
	})

	t.Run("between a finding and its revision", func(t *testing.T) {
		root := t.TempDir()
		base := checked(roster, 1, revised)
		down := true
		model := &table{answer: func(c call) (string, error) {
			if down && c.Step == "redocuments" {
				return "", errors.New("claude failed: exit status 1")
			}
			return base(c)
		}}

		plan, err := runWithRules(t, root, roster, model, projectRules())

		if err == nil || !strings.Contains(err.Error(), "the meeting stopped at the documents: documents revised builder: claude failed") {
			t.Fatalf("err = %v, want the meeting stopped at the builder's revision", err)
		}
		minutes := readFile(t, root, plan.Dir()+"/minutes.md")
		if want := "- builder: 1 point(s) found; **the meeting stopped before they were put right and read again**\n  - Found: `backend/docs/architecture.md` against `.claude/rules/docs.md`: Reading 1: it has no **Status** line.\n"; !strings.Contains(minutes, want) {
			t.Errorf("the minutes should say %q:\n%s", want, minutes)
		}
		// What was found is not said to stand, and not said to be put right.
		if open := plan.OpenFindings(); open != nil {
			t.Errorf("open findings = %+v", open)
		}
		if strings.Contains(minutes, "found and put right") {
			t.Errorf("the minutes say a finding was put right:\n%s", minutes)
		}

		down = false
		before := len(model.steps())
		again, err := Run(context.Background(), Options{Root: root, ID: plan.ID, Roster: roster, Model: model, Repos: testRepos, Rules: projectRules(), Now: fixedNow})
		if err != nil {
			t.Fatalf("going on: %v", err)
		}
		if asked := model.steps()[before:]; len(asked) < 2 || asked[0] != "builder redocuments" || asked[1] != "moderator rulecheck" {
			t.Errorf("going on asked %v, want the revision and then its reading", asked)
		}
		if got := readFile(t, root, "backend/docs/architecture.md"); !strings.Contains(got, "Revised by builder.") {
			t.Errorf("the revision was not written when the plan was gone on with:\n%s", got)
		}
		if check := again.RuleChecks[2]; !check.Settled || len(check.Found) != 1 || len(check.Open) != 0 {
			t.Errorf("the builder's check after going on: %+v", check)
		}
	})
}

// The questions that carry rules are counted against the plan's calls like
// any other.
func TestRun_ReadingsCountAgainstTheCalls(t *testing.T) {
	roster := mustRoster(t, repoRoster)
	plain := &table{answer: agreeable(roster)}
	without, err := runPlan(t, t.TempDir(), roster, plain, "Add a health endpoint.")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	model := &table{answer: checked(roster, 1, revised)}
	with, err := runWithRules(t, t.TempDir(), roster, model, projectRules())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Four readings, one revision and its reading.
	if with.Calls != without.Calls+6 {
		t.Errorf("%d calls with rules, %d without; want six more", with.Calls, without.Calls)
	}

	// A plan out of calls stops like anywhere else, and says where.
	tight := mustRoster(t, strings.Replace(repoRoster, "calls: 60", fmt.Sprintf("calls: %d", without.Calls-4), 1))
	model = &table{answer: following(tight)}
	_, err = runWithRules(t, t.TempDir(), tight, model, projectRules())
	if !errors.Is(err, ErrBudget) || !strings.Contains(err.Error(), "the meeting stopped at the documents") {
		t.Errorf("err = %v, want the plan out of calls at the documents", err)
	}
}

// The index leads to a document wherever it is kept, and does not link to one
// that is not written.
func TestWriteIndex_DocumentsKeptElsewhere(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, "roles:\n  - {id: a, decides: true, stance: s, owns: [backend/docs/architecture.md, 'my api/docs/a b.md', README.md, docs/design/game.md]}\n  - {id: b, plans: true, stance: s, owns: [docs/qa/risks.md]}\n")
	for _, doc := range []string{"backend/docs/architecture.md", "my api/docs/a b.md", "README.md", "docs/design/game.md"} {
		putFile(t, root, doc, "# A document\n\nstatus: draft\n")
	}
	putFile(t, root, "docs/plans/20261101-first/minutes.md", "# Minutes\n")

	if err := writeIndex(root, roster); err != nil {
		t.Fatalf("writeIndex: %v", err)
	}

	index := readFile(t, root, "docs/README.md")
	for _, want := range []string{
		"| [backend/docs/architecture.md](../backend/docs/architecture.md) | a |\n",
		"| [my api/docs/a b.md](<../my api/docs/a b.md>) | a |\n",
		"| [README.md](../README.md) | a |\n",
		"| [docs/design/game.md](design/game.md) | a |\n",
		"| docs/qa/risks.md (not written yet) | b |\n",
		"| [20261101-first](plans/20261101-first/minutes.md) | none yet |\n",
	} {
		if !strings.Contains(index, want) {
			t.Errorf("the index should say %q:\n%s", want, index)
		}
	}
	if strings.Contains(index, "draft") {
		t.Errorf("the index copies what a document says about itself:\n%s", index)
	}
	for file, want := range map[string]string{"docs/a.md": "a.md", "docs/x/y.md": "x/y.md", "a.md": "../a.md", "x/y/z.md": "../x/y/z.md"} {
		if got := fromIndex(file); got != want {
			t.Errorf("fromIndex(%q) = %q, want %q", file, got, want)
		}
	}
	if got := indexLink("a|b", "docs/a|b.md"); got != `[a\|b](a\|b.md)` {
		t.Errorf("a link with a bar in it = %q", got)
	}
}

// What the reader is asked: the words that keep it from judging the design
// and from making findings up.
func TestRuleCheckPrompt(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, repoRoster)
	model := &table{answer: following(roster)}
	if _, err := runWithRules(t, root, roster, model, projectRules()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	reading := model.checksOf(roster.Role("builder"))[0]
	for _, want := range []string{
		"whether the design is good is not yours to say: a table of roles argued that out and decided.",
		"Write in the language the brief is written in.",
		"Go by the words of the rule and quote them.",
		"A rule asks nothing of a document it is not about",
		"What cannot be put right in these documents (the rule asks for a file the owner does not write, say) is a point too; say so in fix.",
		"Do not look for points: when the documents do what the rules ask, the list is empty.",
		`{"findings": [{"document": "the document's path", "rule": "the rule's file", "point": "what is at odds, with the words of the rule", "fix": "what to change in the document"}]}`,
	} {
		if !strings.Contains(reading, want) {
			t.Errorf("the reader should be told %q:\n%s", want, reading)
		}
	}
	// The reader has no seat: it is not told a stance, the brief or the table.
	for _, not := range []string{"What you stand for", "<brief>", "The table:"} {
		if strings.Contains(reading, not) {
			t.Errorf("the reader is told %q", not)
		}
	}
	// It is played with the roster's model, like the moderator.
	strong := mustRoster(t, strings.Replace(repoRoster, "    title: Builder\n", "    title: Builder\n    model: opus\n", 1))
	played := map[string]string{}
	base := checked(strong, 1, revised)
	asking := modelFunc(func(ctx context.Context, name, prompt string) (string, error) {
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
		// The builder is played by a model of its own: who reads its documents,
		// and who puts them right.
		if c.Step == "redocuments" || c.Step == "rulecheck" && strings.Contains(prompt, "the Builder (builder).") {
			played[c.Step] = name
		}
		return base(c)
	})
	single := mustRoster(t, strings.Replace(strings.Replace(repoRoster, "    title: Builder\n", "    title: Builder\n    model: opus\n", 1), "parallel: 2", "parallel: 1", 1))
	if _, err := runWithRules(t, t.TempDir(), single, asking, projectRules()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if want := map[string]string{"rulecheck": "sonnet", "redocuments": "opus"}; !reflect.DeepEqual(played, want) {
		t.Errorf("played by %v, want %v", played, want)
	}
}

// The rules AWK ships are told from the project's own by where they are.
func TestOfTheKit(t *testing.T) {
	for file, want := range map[string]bool{
		".claude/rules/_kit/git-workflow.md": true,
		".claude/rules/_kit/more/x.md":       true,
		".claude/rules/_kitchen.md":          false,
		".claude/rules/_kit.md":              false,
		".claude/rules/docs.md":              false,
		".claude/rules/backend/_kit/x.md":    false,
	} {
		if got := OfTheKit(rules.Rule{File: file}); got != want {
			t.Errorf("OfTheKit(%s) = %v, want %v", file, got, want)
		}
	}
}

// A rule is found by what a reader calls it, when that is one rule.
func TestRulebookNamed(t *testing.T) {
	book := rulebook{shown: []rules.Rule{
		{File: ".claude/rules/docs.md", Title: "Docs Source of Truth"},
		{File: ".claude/rules/backend/api.md", Title: "API"},
		{File: ".claude/rules/frontend/api.md", Title: "Client API"},
		{File: ".claude/rules/testing.md", Title: "API"},
	}}
	for name, want := range map[string]string{
		".claude/rules/docs.md":         ".claude/rules/docs.md",
		"docs.md":                       ".claude/rules/docs.md",
		"docs":                          ".claude/rules/docs.md",
		"Docs Source of Truth":          ".claude/rules/docs.md",
		"docs source of truth":          ".claude/rules/docs.md",
		"rules/docs.md":                 ".claude/rules/docs.md",
		"./.claude/rules/docs.md":       ".claude/rules/docs.md",
		"\"docs.md\"":                   ".claude/rules/docs.md",
		".claude/rules/backend/api.md":  ".claude/rules/backend/api.md",
		"backend/api.md":                ".claude/rules/backend/api.md",
		"rules/frontend/api.md":         ".claude/rules/frontend/api.md",
		"Client API":                    ".claude/rules/frontend/api.md",
		"testing":                       ".claude/rules/testing.md",
		"api.md":                        "", // two files of that name
		"api":                           "", // two files, and a title
		"API":                           "",
		"":                              "",
		"security.md":                   "",
		".claude/rules/security.md":     "",
		"other/docs.md":                 "",
		"ules/docs.md":                  "", // not the end of the path at a slash
		"s.md":                          "",
		"md":                            "",
		".claude/rules/backend/api.mdx": "",
	} {
		got, ok := book.named(name)
		if got.File != want || ok != (want != "") {
			t.Errorf("named(%q) = %q, %v; want %q", name, got.File, ok, want)
		}
	}
}
