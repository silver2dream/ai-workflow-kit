package council

import (
	"fmt"
	"strings"
	"testing"
)

// decisions answers a decision of n decisions and m assumptions.
func decisions(n, m int) string {
	var ds, as []string
	for i := 1; i <= n; i++ {
		ds = append(ds, fmt.Sprintf(`{"title": "point %d", "decision": "Do %d.", "rationale": "Because.", "dissent": ""}`, i, i))
	}
	for i := 1; i <= m; i++ {
		as = append(as, fmt.Sprintf(`{"question": "Who %d?", "assumption": "Them.", "if_wrong": "Ask."}`, i))
	}
	return block(`{"decisions": [` + strings.Join(ds, ", ") + `], "assumptions": [` + strings.Join(as, ", ") + `]}`)
}

// A topic puts on record the choices the documents follow, not every detail
// the table agreed on: at most maxDecisions decisions and maxAssumptions
// assumptions. A ruling of more is asked again.
func TestRun_ATopicDecidesWhatMatters(t *testing.T) {
	roster := mustRoster(t, testRoster)
	base := agreeable(roster)
	asked := map[int]int{}
	model := &table{answer: func(c call) (string, error) {
		if c.Step == "decide" && c.Topic == 1 {
			asked[1]++
			if asked[1] == 1 {
				return decisions(maxDecisions+1, 1), nil
			}
			return decisions(maxDecisions, maxAssumptions), nil
		}
		if c.Step == "decide" && c.Topic == 2 {
			return decisions(1, maxAssumptions+1), nil
		}
		return base(c)
	}}
	_, err := runPlan(t, t.TempDir(), roster, model, "Add a health endpoint.")
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("%d assumptions; at most %d: keep the ones a ruling of this topic rests on", maxAssumptions+1, maxAssumptions)) {
		t.Errorf("err = %v, want the second topic's assumptions refused", err)
	}
	again := ""
	for _, c := range model.calls {
		if c.Step == "decide" && c.Topic == 1 && strings.Contains(c.Prompt, "Your previous answer could not be used") {
			again = c.Prompt
		}
	}
	if !strings.Contains(again, fmt.Sprintf("%d decisions; at most %d: merge the ones that go together", maxDecisions+1, maxDecisions)) {
		t.Errorf("a ruling of too many decisions is not asked again:\n%.300s", again)
	}
	prompt := model.promptIn(1, "owner", "decide")
	for _, want := range []string{
		"What the table agreed in passing, and what follows plainly from a ruling, is not a decision of its own",
		"How the work is cut into tickets, and what a ticket holds, is not one either",
		fmt.Sprintf("At most %d decisions and %d assumptions, the ones that matter most first.", maxDecisions, maxAssumptions),
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the deciding role is not told %q:\n%s", want, prompt)
		}
	}
}

// A ticket holds the fields of its form: the roles that draw up the list and
// look at it are told so, and a document names where a fact is defined rather
// than writing it again.
func TestPrompts_TheFormOfATicketAndOneCopyOfAFact(t *testing.T) {
	roster := mustRoster(t, testRoster)
	model := &table{answer: agreeable(roster)}
	if _, err := runPlan(t, t.TempDir(), roster, model, "Add a health endpoint."); err != nil {
		t.Fatalf("Run: %v", err)
	}
	const form = "A ticket holds the fields of the form above and no other: no estimate, no owner, no date"
	for _, step := range []string{"worklist", "review"} {
		role := "builder"
		if step == "review" {
			role = "owner"
		}
		if prompt := model.promptOf(role, step); !strings.Contains(prompt, form) {
			t.Errorf("%s %s is not told what a ticket holds:\n%s", role, step, prompt)
		}
	}
	documents := model.promptOf("owner", "documents")
	if !strings.Contains(documents, "What another document defines (a number, a name, a timing, a code, a rule), yours names where it is defined, the document and its section, and does not write it again") {
		t.Errorf("a role is not told to name where a fact is defined:\n%s", documents)
	}
	if strings.Contains(documents, "A number the game reads is in a table") {
		t.Errorf("a project without game data is told of its tables:\n%s", documents)
	}
}

// A topic reads a text file of the project no role owns, shown as one.
func TestRun_ATopicReadsAFileOfTheProject(t *testing.T) {
	root := t.TempDir()
	putFile(t, root, "api/health.proto", "syntax = \"proto3\";\nservice Health {}\n")
	roster := mustRoster(t, testRoster)
	base := agreeable(roster)
	agenda := `{"topics": [
  {"id": "scope", "title": "What is built", "question": "What?", "lead": "owner", "speakers": ["designer", "skeptic"], "reads": ["api/health.proto"],
   "writes": ["docs/product/requirements.md", "docs/product/journeys.md", "docs/qa/risks.md"]},
  {"id": "build", "title": "How", "question": "How?", "lead": "builder", "speakers": ["skeptic"], "reads": [], "writes": ["docs/tech/architecture.md"]}
], "questions": []}`
	model := &table{answer: func(c call) (string, error) {
		if c.Step == "agenda" {
			return block(agenda), nil
		}
		return base(c)
	}}
	if _, err := runPlan(t, root, roster, model, "Add a health endpoint."); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if prompt := model.promptIn(1, "owner", "propose"); !strings.Contains(prompt, "===== api/health.proto (a file of the project)\nsyntax = \"proto3\";") {
		t.Errorf("the lead is not shown the file of the project:\n%s", prompt)
	}
	if agendaPrompt := model.promptOf("moderator", "agenda"); !strings.Contains(agendaPrompt, "or a text file of the project that is there, such as a configuration or a contract") {
		t.Errorf("the moderator is not told a topic reads files of the project:\n%s", agendaPrompt)
	}
}
