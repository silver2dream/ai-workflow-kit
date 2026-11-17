package council

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

// An agenda can be held when every topic settles a question, has a lead and
// roles to speak after it, all at the table, ends in documents of roles that
// speak to it, and reads only what is written by then.
func TestCheckAgenda(t *testing.T) {
	roster := mustRoster(t, testRoster)
	written := map[string]bool{"docs/qa/risks.md": true}
	exists := func(doc string) bool { return written[doc] }
	good := func() *Agenda {
		return &Agenda{Topics: []Topic{
			{ID: "scope", Title: "What is built", Question: "What?", Lead: "owner", Speakers: []string{"designer", "skeptic"},
				Reads: []string{"docs/qa/risks.md"}, Writes: []string{"docs/product/requirements.md", "docs/qa/risks.md"}},
			{ID: "build", Title: "How", Question: "How?", Lead: "builder", Speakers: []string{"skeptic"},
				Reads: []string{"docs/product/requirements.md"}, Writes: []string{"docs/tech/architecture.md"}},
		}}
	}
	if err := checkAgenda(good(), roster, 8, exists); err != nil {
		t.Fatalf("the agenda the cases are made from: %v", err)
	}
	tests := []struct {
		name   string
		change func(a *Agenda)
		want   string
	}{
		{"no topic", func(a *Agenda) { a.Topics = nil }, "the agenda has no topic"},
		{"more topics than the roster allows", func(a *Agenda) { a.Topics = append(a.Topics, a.Topics...) }, "the agenda has 4 topics; at most 2 (limits.topics in the roster)"},
		{"more questions than are put at once", func(a *Agenda) { a.Questions = []string{"1", "2", "3", "4", "5", "6", "7", "8"} }, "the agenda has 8 questions for the owner; at most 7"},
		{"an id that is a sentence", func(a *Agenda) { a.Topics[1].ID = "How it is built" }, `topic 2: the id "How it is built" is not lower-case letters, digits and hyphens`},
		{"two topics of one id", func(a *Agenda) { a.Topics[1].ID = "scope" }, "topic 2 (scope): two topics have this id"},
		{"no title", func(a *Agenda) { a.Topics[0].Title = " " }, "topic 1 (scope) has no title"},
		{"no question", func(a *Agenda) { a.Topics[0].Question = "" }, "topic 1 (scope) does not say what it settles: it has no question"},
		{"a lead nobody plays", func(a *Agenda) { a.Topics[0].Lead = "cto" }, `topic 1 (scope): the lead "cto" is not at the table (the roles are: owner, designer, builder, skeptic)`},
		{"nobody after the lead", func(a *Agenda) { a.Topics[1].Speakers = nil }, "topic 2 (build): nobody speaks after builder; a topic one role holds alone is not argued"},
		{"a speaker nobody plays", func(a *Agenda) { a.Topics[1].Speakers = []string{"cfo"} }, `topic 2 (build): the speaker "cfo" is not at the table`},
		{"a speaker twice", func(a *Agenda) { a.Topics[0].Speakers = []string{"designer", "designer"} }, "topic 1 (scope): designer speaks twice"},
		{"the lead among the speakers", func(a *Agenda) { a.Topics[0].Speakers = []string{"owner", "skeptic"} }, "topic 1 (scope): owner speaks twice; a role speaks once to a topic, the lead first"},
		{"no document", func(a *Agenda) { a.Topics[1].Writes = nil }, "topic 2 (build) ends in no document"},
		{"a document nobody owns", func(a *Agenda) { a.Topics[1].Writes = []string{"docs/tech/protocol.md"} }, "topic 2 (build) ends in docs/tech/protocol.md, which no role owns"},
		{"a file that is no document", func(a *Agenda) { a.Topics[1].Writes = []string{"backend/main.go"} }, `topic 2 (build): document "backend/main.go" is neither a Markdown file nor a table of the game data`},
		{"the decision log", func(a *Agenda) { a.Topics[1].Writes = []string{DecisionsPath} }, "kept by the council itself"},
		{"a document of a role that does not speak to it", func(a *Agenda) { a.Topics[1].Writes = []string{"docs/product/journeys.md"} },
			"topic 2 (build) ends in docs/product/journeys.md, which designer writes, and designer does not speak to the topic: add designer to the speakers"},
		{"a document twice", func(a *Agenda) {
			a.Topics[1].Writes = []string{"docs/tech/architecture.md", "docs/tech/architecture.md"}
		}, "topic 2 (build) ends in docs/tech/architecture.md twice"},
		{"reading what nobody wrote yet", func(a *Agenda) { a.Topics[0].Reads = []string{"docs/product/journeys.md"} },
			"topic 1 (scope) reads docs/product/journeys.md, which is not written yet and no earlier topic writes"},
		{"reading what a later topic writes", func(a *Agenda) { a.Topics[0].Reads = []string{"docs/tech/architecture.md"} },
			"topic 1 (scope) reads docs/tech/architecture.md, which is not written yet and no earlier topic writes"},
		{"reading what the topic itself writes first", func(a *Agenda) { a.Topics[0].Reads = []string{"docs/product/requirements.md"} },
			"topic 1 (scope) reads docs/product/requirements.md, which is not written yet"},
		{"reading what is no document of the table", func(a *Agenda) { a.Topics[1].Reads = []string{"README.md"} },
			"topic 2 (build) reads README.md, which is no document of the table"},
		{"reading a document twice", func(a *Agenda) { a.Topics[1].Reads = []string{"docs/qa/risks.md", "docs/qa/risks.md"} }, "topic 2 (build) reads docs/qa/risks.md twice"},
	}
	for _, tt := range tests {
		agenda := good()
		tt.change(agenda)
		max := 8
		if strings.HasPrefix(tt.name, "more topics") {
			max = 2
		}
		err := checkAgenda(agenda, roster, max, exists)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: err = %v, want it to say %q", tt.name, err, tt.want)
		}
	}
	// As many topics as the roster allows, and as many questions as are put
	// at once, are not too many.
	agenda := good()
	agenda.Questions = []string{"1", "2", "3", "4", "5", "6", "7", " "}
	if err := checkAgenda(agenda, roster, 2, exists); err != nil {
		t.Errorf("an agenda at the bounds: %v", err)
	}
}

// The agenda is the person's to change before the meeting goes on: the topics
// that were held stay as they were, the ones that were not are held as the
// agenda now says, and the moderator is not asked again.
func TestRun_AnAgendaAPersonChanged(t *testing.T) {
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
	if err == nil || !strings.Contains(err.Error(), "the meeting stopped at topic 2 (build)") {
		t.Fatalf("err = %v, want the meeting stopped at the second topic", err)
	}

	// The person turns the second topic around: another question, the owner
	// speaks too, and a third topic.
	agenda, err := LoadAgenda(root, plan.ID)
	if err != nil {
		t.Fatalf("LoadAgenda: %v", err)
	}
	agenda.Topics[1].Question = "How is it built, and how is it watched?"
	agenda.Topics[1].Speakers = []string{"skeptic", "owner"}
	agenda.Topics = append(agenda.Topics, Topic{ID: "risks", Title: "What can go wrong", Question: "What fails first?", Lead: "skeptic",
		Speakers: []string{"builder"}, Reads: []string{"docs/tech/architecture.md"}, Writes: []string{"docs/qa/risks.md"}})
	if err := SaveAgenda(root, agenda); err != nil {
		t.Fatal(err)
	}

	broken = false
	before := len(model.steps())
	resumed, err := Run(context.Background(), Options{Root: root, ID: plan.ID, Roster: roster, Model: model, Repos: testRepos, Now: fixedNow})
	if err != nil {
		t.Fatalf("going on: %v", err)
	}
	asked := model.steps()[before:]
	if asked[0] != "builder propose" || asked[1] != "skeptic speak" || asked[2] != "owner speak" {
		t.Errorf("the second topic was not held as the agenda now says: %v", asked)
	}
	for _, step := range asked {
		if step == "moderator agenda" || step == "owner propose" {
			t.Errorf("going on set the agenda again or held the first topic again: %v", asked)
		}
	}
	if len(resumed.Topics) != 3 || !resumed.Topics[2].Held || resumed.Topics[2].Topic.Lead != "skeptic" {
		t.Errorf("the topic the person added was not held: %+v", resumed.Topics)
	}
	if prompt := model.promptIn(2, "builder", "propose"); !strings.Contains(prompt, "The topic now, 2 of 3: How it is built\nIt settles: How is it built, and how is it watched?\nbuilder leads; speaking after, in this order: skeptic, owner.") {
		t.Errorf("the lead is not told the topic as the person changed it:\n%s", prompt)
	}
	if minutes := readFile(t, root, plan.Dir()+"/minutes.md"); !strings.Contains(minutes, "| 3 | [What can go wrong](topics/03-risks.md) | skeptic | builder | `docs/qa/risks.md` | held |") {
		t.Errorf("the minutes do not have the topic the person added:\n%s", minutes)
	}

	// An agenda changed so that it cannot be held is refused before anything
	// is asked, with what is wrong and where.
	agenda.Topics[2].Lead = "cfo"
	if err := SaveAgenda(root, agenda); err != nil {
		t.Fatal(err)
	}
	before = len(model.steps())
	_, err = Run(context.Background(), Options{Root: root, ID: plan.ID, Roster: roster, Model: model, Repos: testRepos, Now: fixedNow})
	if err == nil || !strings.Contains(err.Error(), "the meeting stopped at the agenda: docs/plans/"+plan.ID+"/agenda.yaml: topic 3 (risks): the lead \"cfo\" is not at the table") {
		t.Errorf("err = %v, want the agenda refused with what is wrong", err)
	}
	if asked := model.steps()[before:]; len(asked) != 0 {
		t.Errorf("an agenda that cannot be held was held: %v", asked)
	}
	// One that cannot be read is refused the same way.
	if err := os.WriteFile(agendaPath(root, plan.ID), []byte("topics:\n  - id: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(context.Background(), Options{Root: root, ID: plan.ID, Roster: roster, Model: model, Repos: testRepos, Now: fixedNow}); err == nil || !strings.Contains(err.Error(), "agenda.yaml cannot be read") {
		t.Errorf("err = %v, want the agenda that cannot be read refused", err)
	}
	if err := os.WriteFile(agendaPath(root, plan.ID), []byte("topics: []\nspeakers: [owner]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(context.Background(), Options{Root: root, ID: plan.ID, Roster: roster, Model: model, Repos: testRepos, Now: fixedNow}); err == nil || !strings.Contains(err.Error(), "field speakers not found") {
		t.Errorf("err = %v, want a key the agenda does not have refused", err)
	}
}

// folderRoster is the table of the tests whose roles own folders.
const folderRoster = `
limits:
  calls: 60
  parallel: 1
roles:
  - id: owner
    title: Product Owner
    decides: true
    stance: You answer for what is built.
    owns: [docs/product/]
  - id: designer
    title: Designer
    stance: You answer for the person using it.
    owns: [docs/design/]
  - id: builder
    title: Builder
    plans: true
    stance: You answer for how it is built.
    owns: [backend/docs/]
  - id: skeptic
    title: Skeptic
    stance: You look for how it fails.
    owns: [docs/qa/]
`

// A role that owns a folder writes the documents a topic ends in there, new
// ones too, at any depth. What it hands in beyond them is left as it is.
func TestRun_DocumentsInFolders(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, folderRoster)
	putFile(t, root, "docs/design/old.md", "# Old\n\nWritten before.\n")
	agenda := `{"topics": [
  {"id": "game", "title": "The game", "question": "What is played?", "lead": "designer", "speakers": ["owner", "skeptic"], "reads": ["docs/design/old.md"],
   "writes": ["docs/design/core-loop.md", "docs/design/systems/match.md", "docs/product/vision.md"]},
  {"id": "build", "title": "How", "question": "How?", "lead": "builder", "speakers": ["skeptic"], "reads": ["docs/design/core-loop.md"],
   "writes": ["backend/docs/architecture.md", "backend/docs/protocol.md", "docs/qa/risks.md"]}
], "questions": []}`
	base := agreeable(roster)
	model := &table{answer: func(c call) (string, error) {
		switch {
		case c.Step == "agenda":
			return block(agenda), nil
		case c.Step == "documents" && c.Role == "designer":
			// Beyond what the topic ends in: a document of its own it was not
			// asked for, and one of another role's folder.
			return docs("designer", append(asked(c.Prompt), "docs/design/extra.md", "docs/product/notes.md")...), nil
		}
		return base(c)
	}}

	plan, err := runPlan(t, root, roster, model, "A tennis game.")

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for doc, owner := range map[string]string{"docs/design/core-loop.md": "designer", "docs/design/systems/match.md": "designer", "docs/product/vision.md": "owner",
		"backend/docs/architecture.md": "builder", "backend/docs/protocol.md": "builder", "docs/qa/risks.md": "skeptic"} {
		if content := readFile(t, root, doc); !strings.Contains(content, "Written by "+owner+".") {
			t.Errorf("%s was not written by %s:\n%s", doc, owner, content)
		}
	}
	for _, doc := range []string{"docs/design/extra.md", "docs/product/notes.md"} {
		if exists(root, doc) {
			t.Errorf("%s was written, and the topic does not end in it", doc)
		}
	}
	reasons := map[string]string{}
	for _, doc := range plan.Documents {
		if doc.Refused != "" {
			reasons[doc.Path] = doc.Refused
		}
	}
	if want := map[string]string{"docs/design/extra.md": "this topic does not end in it", "docs/product/notes.md": "designer does not own this document"}; !reflect.DeepEqual(reasons, want) {
		t.Errorf("turned away: %v, want %v", reasons, want)
	}
	if got := readFile(t, root, "docs/design/old.md"); got != "# Old\n\nWritten before.\n" {
		t.Errorf("a document the topic only reads was changed:\n%s", got)
	}

	// Every role is shown the folders and what is in them, whose they are.
	speak := model.promptIn(2, "skeptic", "speak")
	for _, want := range []string{"- docs/product/ (owner: any document in this folder)\n- docs/product/vision.md (owner)\n",
		"- docs/design/ (designer: any document in this folder)\n- docs/design/core-loop.md (designer)\n- docs/design/old.md (designer)\n- docs/design/systems/match.md (designer)\n",
		"- docs/qa/ (you: any document in this folder)\n"} {
		if !strings.Contains(speak, want) {
			t.Errorf("the map of documents should say %q:\n%s", want, speak)
		}
	}
	// The moderator is shown what each role writes, and what is written.
	if prompt := model.promptOf("moderator", "agenda"); !strings.Contains(prompt, "  Writes: docs/design/\n") || !strings.Contains(prompt, "- docs/design/old.md (designer)\n") {
		t.Errorf("the moderator is not shown the folders and what is in them:\n%s", prompt)
	}
	// The index lists what is written in each folder, and the folders
	// nothing is written in yet.
	index := readFile(t, root, "docs/README.md")
	for _, want := range []string{"| [docs/design/core-loop.md](design/core-loop.md) | designer |\n", "| [docs/design/systems/match.md](design/systems/match.md) | designer |\n",
		"| [backend/docs/protocol.md](../backend/docs/protocol.md) | builder |\n"} {
		if !strings.Contains(index, want) {
			t.Errorf("the index should say %q:\n%s", want, index)
		}
	}
	empty := t.TempDir()
	if err := writeIndex(empty, roster); err != nil {
		t.Fatal(err)
	}
	if index := readFile(t, empty, "docs/README.md"); !strings.Contains(index, "| docs/design/ (nothing written yet) | designer |\n") {
		t.Errorf("the index does not name a folder nothing is written in:\n%s", index)
	}
	// What the planning role reads: every document, the ones in folders too.
	if worklist := model.promptOf("builder", "worklist"); !strings.Contains(worklist, "===== docs/design/systems/match.md (owner: designer; written in this meeting)\n") {
		t.Errorf("the planning role is not shown the documents in folders:\n%s", worklist)
	}
}

// A document two topics end in is written by the first and taken up again by
// the second: its owner is shown it as it stands, and may leave it.
func TestRun_ADocumentTwoTopicsEndIn(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, testRoster)
	agenda := `{"topics": [
  {"id": "scope", "title": "What is built", "question": "What?", "lead": "owner", "speakers": ["designer"], "reads": [], "writes": ["docs/product/requirements.md"]},
  {"id": "limits", "title": "What is left out", "question": "What not?", "lead": "skeptic", "speakers": ["owner"], "reads": ["docs/product/requirements.md"], "writes": ["docs/product/requirements.md", "docs/qa/risks.md"]},
  {"id": "build", "title": "How", "question": "How?", "lead": "builder", "speakers": ["owner"], "reads": [], "writes": ["docs/tech/architecture.md", "docs/product/requirements.md"]}
], "questions": []}`
	base := agreeable(roster)
	model := &table{answer: func(c call) (string, error) {
		switch {
		case c.Step == "agenda":
			return block(agenda), nil
		case c.Step == "documents" && c.Role == "owner" && c.Topic == 2:
			return "<<<DOCUMENT docs/product/requirements.md>>>\n# Requirements\n\nWhat is left out: metrics.\n<<<END>>>\n", nil
		case c.Step == "documents" && c.Role == "owner" && c.Topic == 3:
			return "<<<UNCHANGED docs/product/requirements.md>>>\n", nil
		}
		return base(c)
	}}

	plan, err := runPlan(t, root, roster, model, "Add a health endpoint.")

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := model.count("owner", "documents"); got != 3 {
		t.Errorf("the owner was asked for its document %d times, want once in each topic", got)
	}
	second := model.promptIn(2, "owner", "documents")
	if !strings.Contains(second, "Your documents of this topic as they stand:\n===== docs/product/requirements.md\n# docs/product/requirements.md\n\nOwner: owner.\n\nWritten by owner.\n") ||
		!strings.Contains(second, "<<<UNCHANGED path/of/the/document.md>>>") {
		t.Errorf("the second topic does not show the owner its document as the first left it:\n%s", second)
	}
	if got := readFile(t, root, "docs/product/requirements.md"); got != "# Requirements\n\nWhat is left out: metrics.\n" {
		t.Errorf("the document is not as the second topic left it:\n%s", got)
	}
	var states []string
	for _, doc := range plan.Documents {
		if doc.Path == "docs/product/requirements.md" {
			state := "written"
			if doc.Unchanged {
				state = "unchanged"
			}
			states = append(states, state)
		}
	}
	if want := []string{"written", "written", "unchanged"}; !reflect.DeepEqual(states, want) {
		t.Errorf("the document is on record as %v, want %v", states, want)
	}
	// Written in this meeting, whatever the last topic did with it.
	if state := documentState(plan)["docs/product/requirements.md"]; state != "written in this meeting" {
		t.Errorf("the document's state for the work list = %q", state)
	}
}
