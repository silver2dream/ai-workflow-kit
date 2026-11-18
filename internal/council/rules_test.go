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

// crowded is the agreeable table where both speakers of the first topic
// object: more objections than the roster argues out, so that the moderator
// has to pick.
func crowded(roster *Roster) func(c call) (string, error) {
	base := agreeable(roster)
	return func(c call) (string, error) {
		switch {
		case c.Step == "speak" && c.Topic == 1:
			return block(fmt.Sprintf(`{"says": "No.", "objections": [{"to": "owner", "point": "%s objects", "why": "w", "wants": "x"}, {"to": "owner", "point": "%s objects again", "why": "w", "wants": "x"}]}`, c.Role, c.Role)), nil
		case c.Step == "pick":
			return block(`{"disputes": [{"topic": "one", "challenger": "skeptic", "defender": "owner", "point": "skeptic objects", "wants": "x"},
				{"topic": "two", "challenger": "designer", "defender": "owner", "point": "designer objects", "wants": "x"}]}`), nil
		case c.Step == "defence":
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
		{"a stance that is none of the three", disputing(roster), "builder", "defence", block(`{"stance": "maybe", "reply": "Perhaps.", "position": "p"}`), `stance is "maybe"; it is one of concede, revise, hold`},
		{"a defence without a reply", disputing(roster), "builder", "defence", block(`{"stance": "hold", "reply": " "}`), "the answer has no reply"},
		{"a reply without a reply", disputing(roster), "designer", "reply", block(`{"accepts": false, "reply": ""}`), "the answer has no reply"},
		{"no dispute picked", crowded(roster), "moderator", "pick", block(`{"disputes": []}`), "no dispute was picked, and there are objections"},
		{"a dispute with a role that is not at the topic", crowded(roster), "moderator", "pick", block(`{"disputes": [{"topic": "x", "challenger": "builder", "defender": "owner", "point": "p"}]}`), `the dispute "x" is not between two roles of this topic (they are: owner, designer, skeptic)`},
		{"a dispute against a role that is not at the table", crowded(roster), "moderator", "pick", block(`{"disputes": [{"topic": "x", "challenger": "owner", "defender": "cfo", "point": "p"}]}`), `the dispute "x" is not between two roles of this topic`},
		{"a dispute of a role with itself", crowded(roster), "moderator", "pick", block(`{"disputes": [{"topic": "x", "challenger": "owner", "defender": "owner", "point": "p"}]}`), `the dispute "x" is not between two roles of this topic`},
		{"a dispute without a point", crowded(roster), "moderator", "pick", block(`{"disputes": [{"topic": "x", "challenger": "skeptic", "defender": "owner", "point": " "}]}`), `the dispute "x" has no point`},
		{"a review naming nothing of the list", agreeable(roster), "skeptic", "review", block(`{"objections": [{"ticket": "m9", "point": "p"}]}`), `an objection names "m9", which is neither a ticket nor a milestone of the list`},
		{"a review without a point", agreeable(roster), "skeptic", "review", block(`{"objections": [{"ticket": "t1", "point": " "}]}`), "an objection has no point"},
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

// Every piece of a step that runs side by side runs, also after one of them
// failed: what can be settled is settled and remembered, and going on asks
// only for what failed.
func TestRun_OneFailureDoesNotStopTheOthers(t *testing.T) {
	root := t.TempDir()
	// One call at a time, so that the owners are asked in the order of the roster.
	roster := mustRoster(t, strings.Replace(testRoster, "parallel: 2", "parallel: 1", 1))
	base := agreeable(roster)
	broken := true
	model := &table{answer: func(c call) (string, error) {
		if broken && c.Step == "documents" && c.Role == "owner" {
			return "I have nothing to write.", nil
		}
		return base(c)
	}}

	plan, err := runPlan(t, root, roster, model, "Add a health endpoint.")

	if err == nil || !strings.Contains(err.Error(), "the meeting stopped at topic 1 (scope): topic scope documents owner: ") {
		t.Fatalf("err = %v, want the meeting stopped at the owner's documents", err)
	}
	if got := model.count("owner", "documents"); got != 2 {
		t.Errorf("the role whose answer could not be used was asked %d times, want twice", got)
	}
	for _, role := range []string{"designer", "skeptic"} {
		if got := model.count(role, "documents"); got != 1 {
			t.Errorf("%s was asked for its documents %d times after the first role failed, want once", role, got)
		}
	}
	if !exists(root, "docs/qa/risks.md") || exists(root, "docs/product/requirements.md") {
		t.Error("what the others wrote is not there, or what failed is")
	}

	broken = false
	before := len(model.steps())
	if _, err := Run(context.Background(), Options{Root: root, ID: plan.ID, Roster: roster, Model: model, Repos: testRepos, Now: fixedNow}); err != nil {
		t.Fatalf("going on: %v", err)
	}
	var documents []string
	for _, step := range model.steps()[before:] {
		if strings.HasSuffix(step, " documents") {
			documents = append(documents, step)
		}
	}
	if !reflect.DeepEqual(documents, []string{"owner documents", "builder documents"}) {
		t.Errorf("going on asked for the documents %v, want the one that failed and the second topic's", documents)
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
	records, _ := filepath.Glob(filepath.Join(root, ".ai", "state", "council", plan.ID, "calls", "*-topic-scope-moderator-sums-up.md"))
	if len(records) != 1 {
		t.Fatalf("%d records of the call that failed, want one", len(records))
	}
	record := readFile(t, filepath.Dir(records[0]), filepath.Base(records[0]))
	for _, want := range []string{"# topic scope moderator sums up (sonnet)", "## prompt\n\nYou moderate a planning meeting", "## error\n\nclaude failed: exit status 1\n"} {
		if !strings.Contains(record, want) {
			t.Errorf("the record should have %q:\n%s", want, record)
		}
	}
	if all, _ := filepath.Glob(filepath.Join(root, ".ai", "state", "council", plan.ID, "calls", "*.md")); len(all) != plan.Calls {
		t.Errorf("%d calls on record, %d made", len(all), plan.Calls)
	}
}

// What the roster's model is: the moderator is played by it, a role by its
// own when it names one.
func TestRun_WhoPlaysWhom(t *testing.T) {
	roster := mustRoster(t, strings.Replace(testRoster, "    title: Skeptic\n", "    title: Skeptic\n    model: opus\n", 1))
	var mu = make(chan struct{}, 1)
	played := map[string]string{}
	base := agreeable(roster)
	model := modelFunc(func(ctx context.Context, voice Voice, prompt string) (string, error) {
		c := readCall(prompt)
		mu <- struct{}{}
		played[c.Role] = voice.Model
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

// How hard each one thinks: the moderator as the roster says; a role as it
// says, or else as the roster does. The record of each call says which.
func TestRun_WhoThinksHowHard(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, "effort: high\n"+strings.Replace(testRoster, "    title: Skeptic\n", "    title: Skeptic\n    model: opus\n    effort: max\n", 1))
	var mu = make(chan struct{}, 1)
	played := map[string][]Voice{}
	base := agreeable(roster)
	model := modelFunc(func(ctx context.Context, voice Voice, prompt string) (string, error) {
		c := readCall(prompt)
		mu <- struct{}{}
		played[c.Role] = append(played[c.Role], voice)
		first := c.Role == "skeptic" && len(played[c.Role]) == 1
		<-mu
		// The skeptic's first answer cannot be used: it is asked again, and
		// thinks as hard the second time.
		if first {
			return "I would rather not say.", nil
		}
		return base(c)
	})

	plan, err := runPlan(t, root, roster, model, "Add a health endpoint.")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	high, strong := Voice{Model: "sonnet", Effort: "high"}, Voice{Model: "opus", Effort: "max"}
	want := map[string]Voice{"moderator": high, "owner": high, "designer": high, "builder": high, "skeptic": strong}
	for role, voices := range played {
		for _, voice := range voices {
			if voice != want[role] {
				t.Errorf("%s was played by %v, want %v", role, voice, want[role])
			}
		}
	}
	if len(played) != len(want) || len(played["skeptic"]) < 2 {
		t.Errorf("played: %v; want every role, and the skeptic asked again", played)
	}
	calls := filepath.Join(root, ".ai", "state", "council", plan.ID, "calls")
	for pattern, want := range map[string]string{"*-moderator-sums-up.md": "(sonnet, effort high)", "*-skeptic-*.md": "(opus, effort max)", "*-skeptic-*again*.md": "(opus, effort max)"} {
		records, _ := filepath.Glob(filepath.Join(calls, pattern))
		if len(records) == 0 {
			t.Fatalf("no record of %s", pattern)
		}
		if record := readFile(t, calls, filepath.Base(records[0])); !strings.Contains(strings.SplitN(record, "\n", 2)[0], want) {
			t.Errorf("the record of %s should say %s:\n%s", filepath.Base(records[0]), want, strings.SplitN(record, "\n", 2)[0])
		}
	}
}

// modelFunc is a Model made of a function.
type modelFunc func(ctx context.Context, voice Voice, prompt string) (string, error)

func (f modelFunc) Ask(ctx context.Context, voice Voice, prompt string) (string, error) {
	return f(ctx, voice, prompt)
}
