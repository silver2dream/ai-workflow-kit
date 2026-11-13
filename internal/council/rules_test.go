package council

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// A code block that is not closed is not a block: what follows the opening
// line is the rest of the answer.
func TestFenced(t *testing.T) {
	type block = struct{ info, body string }
	got := fenced("words\n```JSON\n{\"a\": 1}\n```\nmore\r\n```\nplain\n```\n")
	if want := []block{{"json", `{"a": 1}`}, {"", "plain"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("blocks = %#v, want %#v", got, want)
	}
	if got := fenced("```json\n{\"a\": 1}\n"); len(got) != 0 {
		t.Errorf("a block that is not closed was read: %#v", got)
	}
	if got := fenced("```json\n{}\n```\n```\nnever closed\n"); len(got) != 1 || got[0].body != "{}" {
		t.Errorf("blocks = %#v, want the one that is closed", got)
	}
}

// crowded is the agreeable table where every role objects: more objections
// than the roster argues out, so that the moderator has to pick.
func crowded(roster *Roster) func(c call) (string, error) {
	base := agreeable(roster)
	return func(c call) (string, error) {
		switch c.Step {
		case "objections":
			to := "owner"
			if c.Role == "owner" {
				to = "builder"
			}
			return block(fmt.Sprintf(`{"objections": [{"to": "%s", "point": "%s objects", "why": "w", "wants": "x"}]}`, to, c.Role)), nil
		case "pick":
			return block(`{"disputes": [{"topic": "one", "challenger": "skeptic", "defender": "owner", "point": "skeptic objects", "wants": "x"},
				{"topic": "two", "challenger": "owner", "defender": "builder", "point": "owner objects", "wants": "x"}]}`), nil
		case "defence":
			return block(`{"stance": "concede", "reply": "Right.", "position": "Changed."}`), nil
		}
		return base(c)
	}
}

// More answers that cannot be used, in the steps a plain meeting does not
// reach: each is asked for once more, with what was wrong with it.
func TestRun_MoreAnswersThatCannotBeUsed(t *testing.T) {
	roster := mustRoster(t, testRoster)
	tests := []struct {
		name            string
		base            func(c call) (string, error)
		role, step, bad string
		wantTold        string
	}{
		{"an objection without a point", agreeable(roster), "skeptic", "objections", block(`{"objections": [{"to": "owner", "point": " "}]}`), "an objection has no point"},
		{"a decision that decides nothing", agreeable(roster), "owner", "decide", block(`{"decisions": [{"title": "t", "decision": " ", "rationale": "r"}]}`), "a decision does not say what is decided"},
		{"an empty document", agreeable(roster), "builder", "documents", "<<<DOCUMENT docs/tech/architecture.md>>>\n \n<<<END>>>\n", "docs/tech/architecture.md is empty"},
		{"a stance that is none of the three", disputing(roster), "builder", "defence", block(`{"stance": "maybe", "reply": "Perhaps.", "position": "p"}`), `stance is "maybe"; it is one of concede, revise, hold`},
		{"a defence without a reply", disputing(roster), "builder", "defence", block(`{"stance": "hold", "reply": " "}`), "the answer has no reply"},
		{"a reply without a reply", disputing(roster), "designer", "reply", block(`{"accepts": false, "reply": ""}`), "the answer has no reply"},
		{"no dispute picked", crowded(roster), "moderator", "pick", block(`{"disputes": []}`), "no dispute was picked, and there are objections"},
		{"a dispute with a role that is not at the table", crowded(roster), "moderator", "pick", block(`{"disputes": [{"topic": "x", "challenger": "cfo", "defender": "owner", "point": "p"}]}`), `the dispute "x" is not between two roles at the table (the roles are: owner, designer, builder, skeptic)`},
		{"a dispute against a role that is not at the table", crowded(roster), "moderator", "pick", block(`{"disputes": [{"topic": "x", "challenger": "owner", "defender": "cfo", "point": "p"}]}`), `the dispute "x" is not between two roles at the table`},
		{"a dispute of a role with itself", crowded(roster), "moderator", "pick", block(`{"disputes": [{"topic": "x", "challenger": "owner", "defender": "owner", "point": "p"}]}`), `the dispute "x" is not between two roles at the table`},
		{"a dispute without a point", crowded(roster), "moderator", "pick", block(`{"disputes": [{"topic": "x", "challenger": "skeptic", "defender": "owner", "point": " "}]}`), `the dispute "x" has no point`},
	}
	for _, tt := range tests {
		asked := 0
		var second string
		model := &table{answer: func(c call) (string, error) {
			if c.Role == tt.role && c.Step == tt.step {
				asked++
				if asked == 1 {
					return tt.bad, nil
				}
				if asked == 2 {
					second = c.Prompt
				}
			}
			return tt.base(c)
		}}

		if _, err := runPlan(t, t.TempDir(), roster, model, "Add a health endpoint."); err != nil {
			t.Errorf("%s: Run: %v", tt.name, err)
			continue
		}
		if asked < 2 {
			t.Errorf("%s: %s was asked %d times; an answer that cannot be used is asked for again", tt.name, tt.role, asked)
			continue
		}
		if !strings.Contains(second, "Your previous answer could not be used: ") || !strings.Contains(second, tt.wantTold) {
			t.Errorf("%s: the second question should say what was wrong (%q):\n%s", tt.name, tt.wantTold, second[max(0, len(second)-400):])
		}
	}
}

// Every piece of a step runs, also after one of them failed: what can be
// settled is settled and remembered, and going on asks only for what failed.
func TestRun_OneFailureDoesNotStopTheOthers(t *testing.T) {
	root := t.TempDir()
	// One call at a time, so that the roles are asked in the order of the roster.
	roster := mustRoster(t, strings.Replace(testRoster, "parallel: 2", "parallel: 1", 1))
	base := agreeable(roster)
	broken := true
	model := &table{answer: func(c call) (string, error) {
		if broken && c.Step == "position" && c.Role == "owner" {
			return "I have no position.", nil
		}
		return base(c)
	}}

	plan, err := runPlan(t, root, roster, model, "Add a health endpoint.")

	if err == nil || !strings.Contains(err.Error(), "the meeting stopped at the positions") {
		t.Fatalf("err = %v, want the meeting stopped at the positions", err)
	}
	if got := model.count("owner", "position"); got != 2 {
		t.Errorf("the role whose answer could not be used was asked %d times, want twice", got)
	}
	for _, role := range []string{"designer", "builder", "skeptic"} {
		if got := model.count(role, "position"); got != 1 {
			t.Errorf("%s was asked for its position %d times after the first role failed, want once", role, got)
		}
	}

	broken = false
	before := len(model.steps())
	if _, err := Run(context.Background(), Options{Root: root, ID: plan.ID, Roster: roster, Model: model, Repos: testRepos, Now: fixedNow}); err != nil {
		t.Fatalf("going on: %v", err)
	}
	var positions []string
	for _, step := range model.steps()[before:] {
		if strings.HasSuffix(step, " position") {
			positions = append(positions, step)
		}
	}
	if !reflect.DeepEqual(positions, []string{"owner position"}) {
		t.Errorf("going on asked for the positions %v, want only the one that failed", positions)
	}
}

// A call that failed is on record too, with what went wrong.
func TestRun_AFailedCallIsOnRecord(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, testRoster)
	base := agreeable(roster)
	model := &table{answer: func(c call) (string, error) {
		if c.Step == "summary" {
			return "", errors.New("claude failed: exit status 1")
		}
		return base(c)
	}}

	plan, err := runPlan(t, root, roster, model, "Add a health endpoint.")

	if err == nil {
		t.Fatal("the meeting went on after a call failed")
	}
	records, _ := filepath.Glob(filepath.Join(root, ".ai", "state", "council", plan.ID, "calls", "*-moderator-sums-up.md"))
	if len(records) != 1 {
		t.Fatalf("%d records of the call that failed, want one", len(records))
	}
	record := readFile(t, filepath.Dir(records[0]), filepath.Base(records[0]))
	for _, want := range []string{"# moderator sums up (sonnet)", "## prompt\n\nYou moderate a planning meeting", "## error\n\nclaude failed: exit status 1\n"} {
		if !strings.Contains(record, want) {
			t.Errorf("the record should have %q:\n%s", want, record)
		}
	}
	if all, _ := filepath.Glob(filepath.Join(root, ".ai", "state", "council", plan.ID, "calls", "*.md")); len(all) != plan.Calls {
		t.Errorf("%d calls on record, %d made", len(all), plan.Calls)
	}
}

// A role is shown the others' positions without its own among them, which it
// is reminded of apart; the moderator, who has none, is shown them all.
func TestPrompts_TheOthersPositions(t *testing.T) {
	roster := mustRoster(t, testRoster)
	model := &table{answer: agreeable(roster)}
	if _, err := runPlan(t, t.TempDir(), roster, model, "Add a health endpoint."); err != nil {
		t.Fatalf("Run: %v", err)
	}

	objections := model.promptOf("skeptic", "objections")
	at := strings.Index(objections, "What the others hold:")
	if at < 0 {
		t.Fatalf("the question does not show what the others hold:\n%s", objections)
	}
	own, others := objections[:at], objections[at:]
	if !strings.Contains(own, "Your own position:\nskeptic holds its line.\nYou proposed:\n  - skeptic proposes\n") {
		t.Errorf("the role is not reminded of its own position:\n%s", own)
	}
	for _, want := range []string{"## owner (Product Owner)\nowner holds its line.\nProposes:\n  - owner proposes\nWorried about:\n  - owner worries\n", "## designer (Designer)", "## builder (Builder)"} {
		if !strings.Contains(others, want) {
			t.Errorf("the others' positions should have %q:\n%s", want, others)
		}
	}
	if strings.Contains(others, "## skeptic (Skeptic)") {
		t.Errorf("the role's own position is among the others':\n%s", others)
	}
	if summary := model.promptOf("moderator", "summary"); !strings.Contains(summary, "## skeptic (Skeptic)") || !strings.Contains(summary, "## owner (Product Owner)") {
		t.Errorf("the moderator is not shown every position:\n%s", summary)
	}
}

// What the roster's model is: the moderator is played by it, a role by its
// own when it names one.
func TestRun_WhoPlaysWhom(t *testing.T) {
	roster := mustRoster(t, strings.Replace(testRoster, "    title: Skeptic\n", "    title: Skeptic\n    model: opus\n", 1))
	var mu = make(chan struct{}, 1)
	played := map[string]string{}
	base := agreeable(roster)
	model := modelFunc(func(ctx context.Context, name, prompt string) (string, error) {
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
		mu <- struct{}{}
		played[c.Role] = name
		<-mu
		return base(c)
	})

	if _, err := runPlan(t, t.TempDir(), roster, model, "Add a health endpoint."); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if want := map[string]string{"moderator": "sonnet", "owner": "sonnet", "designer": "sonnet", "builder": "sonnet", "skeptic": "opus"}; !reflect.DeepEqual(played, want) {
		t.Errorf("played by %v, want %v", played, want)
	}
}

// modelFunc is a Model made of a function.
type modelFunc func(ctx context.Context, model, prompt string) (string, error)

func (f modelFunc) Ask(ctx context.Context, model, prompt string) (string, error) {
	return f(ctx, model, prompt)
}
