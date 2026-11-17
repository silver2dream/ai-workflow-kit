package council

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// curious is the agreeable table whose agenda has three questions for the
// owner of the project.
func curious(roster *Roster) func(c call) (string, error) {
	base := agreeable(roster)
	return func(c call) (string, error) {
		if c.Step == "agenda" {
			return block(strings.Replace(agendaFor(roster), `"questions": []`,
				`"questions": ["Who calls the endpoint, and how often?", "Does a person ever read the answer?", " ", "May the endpoint be reached from outside?"]`, 1)), nil
		}
		return base(c)
	}
}

// The questions of the agenda are put to the owner before the first topic.
// What the owner answers is known to every role from there on; what the
// owner leaves open is left to the deciding role to assume.
func TestRun_TheOwnerIsAskedBeforeTheFirstTopic(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, testRoster)
	model := &table{answer: curious(roster)}
	var put [][]string
	var askedAfter int
	ask := func(ctx context.Context, questions []string) ([]string, error) {
		put = append(put, questions)
		askedAfter = len(model.steps())
		return []string{"  A readiness probe,\nevery ten seconds. ", "", "No."}, nil
	}
	var told []string

	plan, err := Run(context.Background(), Options{Root: root, Brief: "Add a health endpoint.", Roster: roster, Model: model, Repos: testRepos,
		Now: fixedNow, Ask: ask, Progress: func(line string) { told = append(told, line) }})

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	wantQuestions := []string{"Who calls the endpoint, and how often?", "Does a person ever read the answer?", "May the endpoint be reached from outside?"}
	if len(put) != 1 || !reflect.DeepEqual(put[0], wantQuestions) {
		t.Fatalf("the owner was asked %v, want once: %v", put, wantQuestions)
	}
	// Asked after the agenda was set, before anybody spoke to a topic.
	if askedAfter != 1 {
		t.Errorf("the owner was asked after %d calls, want right after the agenda", askedAfter)
	}
	want := []OwnerAnswer{
		{Question: "Who calls the endpoint, and how often?", Answer: "A readiness probe,\nevery ten seconds."},
		{Question: "Does a person ever read the answer?"},
		{Question: "May the endpoint be reached from outside?", Answer: "No."},
	}
	if !reflect.DeepEqual(plan.Answers, want) {
		t.Errorf("answers = %+v\nwant %+v", plan.Answers, want)
	}
	// The agenda keeps the questions, for whoever goes on with the plan.
	if agenda, err := LoadAgenda(root, plan.ID); err != nil || !reflect.DeepEqual(agenda.Questions, wantQuestions) {
		t.Errorf("the agenda on disk has the questions %v, %v", agenda, err)
	}

	// Every role is told what the owner said, and so is the moderator. What
	// was left open is not passed off as an answer: the deciding role is told
	// it is open.
	said := "The owner of the project was asked before the meeting, and answered. This holds like the brief:\n" +
		"- Asked: Who calls the endpoint, and how often?\n  The owner: A readiness probe, every ten seconds.\n" +
		"- Asked: May the endpoint be reached from outside?\n  The owner: No.\n"
	for _, c := range []struct{ role, step string }{{"owner", "propose"}, {"skeptic", "speak"}, {"moderator", "summary"}, {"owner", "decide"}, {"designer", "documents"}, {"builder", "worklist"}, {"skeptic", "review"}} {
		prompt := model.promptOf(c.role, c.step)
		if !strings.Contains(prompt, said) {
			t.Errorf("%s %s is not told what the owner said:\n%s", c.role, c.step, prompt)
		}
		if strings.Contains(prompt, "- Asked: Does a person ever read the answer?") {
			t.Errorf("%s %s is told of an answer the owner did not give", c.role, c.step)
		}
	}
	if decide := model.promptOf("owner", "decide"); !strings.Contains(decide, "Questions of the agenda the owner of the project has not answered:\n  - Does a person ever read the answer?\n") ||
		strings.Contains(decide, "  - Who calls the endpoint, and how often?\n") {
		t.Errorf("the deciding role is not told which question is open:\n%s", decide)
	}

	minutes := readFile(t, root, plan.Dir()+"/minutes.md")
	for _, want := range []string{"## What the owner was asked\n", "- Who calls the endpoint, and how often?\n  - A readiness probe, every ten seconds.\n",
		"- Does a person ever read the answer?\n  - (left open: the council assumes)\n"} {
		if !strings.Contains(minutes, want) {
			t.Errorf("the minutes should say %q:\n%s", want, minutes)
		}
	}
	progress := strings.Join(told, "\n")
	for _, want := range []string{"Questions: 3 for the owner of the project", "  2 answered, 1 left to the council to assume"} {
		if !strings.Contains(progress, want) {
			t.Errorf("the person should be told %q:\n%s", want, progress)
		}
	}

	// Going on with the plan does not ask again, with or without anybody to ask.
	for _, again := range []func(context.Context, []string) ([]string, error){ask, nil} {
		told = nil
		resumed, err := Run(context.Background(), Options{Root: root, ID: plan.ID, Roster: roster, Model: model, Repos: testRepos, Now: fixedNow,
			Ask: again, Progress: func(line string) { told = append(told, line) }})
		if err != nil {
			t.Fatalf("going on: %v", err)
		}
		if len(put) != 1 {
			t.Fatalf("the owner was asked again: %v", put)
		}
		if !reflect.DeepEqual(resumed.Answers, want) {
			t.Errorf("gone on with, the answers are %+v", resumed.Answers)
		}
		if !strings.Contains(strings.Join(told, "\n"), "Questions: what the owner answered is kept from the earlier run") {
			t.Errorf("the person is not told that the answers were kept: %v", told)
		}
	}
}

// With nobody to ask, the meeting does not wait: the questions stay on the
// agenda, and the deciding role is told they are open.
func TestRun_WithNobodyToAskTheMeetingDoesNotWait(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, testRoster)
	model := &table{answer: curious(roster)}

	plan, err := runPlan(t, root, roster, model, "Add a health endpoint.")

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(plan.Answers) != 0 {
		t.Errorf("without anybody to ask: answers %+v", plan.Answers)
	}
	if minutes := readFile(t, root, plan.Dir()+"/minutes.md"); strings.Contains(minutes, "## What the owner was asked") {
		t.Errorf("the minutes say the owner was asked:\n%s", minutes)
	}
	if decide := model.promptOf("owner", "decide"); !strings.Contains(decide, "Questions of the agenda the owner of the project has not answered:\n  - Who calls the endpoint, and how often?\n  - Does a person ever read the answer?\n  - May the endpoint be reached from outside?\n") {
		t.Errorf("the deciding role is not told the open questions:\n%s", decide)
	}
	for _, c := range model.calls {
		if strings.Contains(c.Prompt, "The owner of the project was asked before the meeting") {
			t.Errorf("%s %s is told the owner answered, and nobody was asked", c.Role, c.Step)
		}
	}
}

// When the owner is asked: not at all without a question; an agenda with too
// many questions is asked for again; an owner who cannot be asked stops the
// meeting there, and going on asks.
func TestRun_PuttingTheQuestions(t *testing.T) {
	roster := mustRoster(t, testRoster)
	asked := 0
	var put []string
	ask := func(ctx context.Context, questions []string) ([]string, error) {
		asked++
		put = questions
		return nil, nil
	}

	// The agenda has no question: the owner is not disturbed.
	model := &table{answer: agreeable(roster)}
	if _, err := Run(context.Background(), Options{Root: t.TempDir(), Brief: "Add a health endpoint.", Roster: roster, Model: model, Repos: testRepos, Now: fixedNow, Ask: ask}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if asked != 0 {
		t.Errorf("with no question the owner was asked %d times", asked)
	}

	// An owner who answers none leaves all of them to the council.
	model = &table{answer: curious(roster)}
	plan, err := Run(context.Background(), Options{Root: t.TempDir(), Brief: "Add a health endpoint.", Roster: roster, Model: model, Repos: testRepos, Now: fixedNow, Ask: ask})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if asked != 1 || len(put) != 3 || len(plan.Answers) != 3 || plan.Answers[0].Answer != "" {
		t.Errorf("asked %d times %v; answers %+v", asked, put, plan.Answers)
	}
	if prompt := model.promptOf("owner", "decide"); strings.Contains(prompt, "The owner of the project was asked") {
		t.Errorf("the deciding role is told the owner answered, and the owner answered nothing:\n%s", prompt)
	}

	// More questions than are put at once: the agenda is asked for again.
	many := `"questions": ["q1", "q2", "q3", "q4", "q5", "q6", "q7", "q8"]`
	base := agreeable(roster)
	agendas := 0
	model = &table{answer: func(c call) (string, error) {
		if c.Step == "agenda" {
			agendas++
			if agendas == 1 {
				return block(strings.Replace(agendaFor(roster), `"questions": []`, many, 1)), nil
			}
		}
		return base(c)
	}}
	if _, err := Run(context.Background(), Options{Root: t.TempDir(), Brief: "Add a health endpoint.", Roster: roster, Model: model, Repos: testRepos, Now: fixedNow, Ask: ask}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if again := model.promptOf("moderator", "agenda"); agendas != 2 || !strings.Contains(again, "the agenda has 8 questions for the owner; at most 7, the ones the plan turns on") {
		t.Errorf("an agenda of eight questions was asked for %d times:\n%s", agendas, again[max(0, len(again)-300):])
	}

	// An owner who cannot be asked stops the meeting there, and going on asks.
	model = &table{answer: curious(roster)}
	root := t.TempDir()
	failing := func(ctx context.Context, questions []string) ([]string, error) {
		return nil, errors.New("the terminal is gone")
	}
	stopped, err := Run(context.Background(), Options{Root: root, Brief: "Add a health endpoint.", Roster: roster, Model: model, Repos: testRepos, Now: fixedNow, Ask: failing})
	if err == nil || !strings.Contains(err.Error(), "the meeting stopped at the owner's questions: the terminal is gone") {
		t.Fatalf("err = %v", err)
	}
	if got := model.steps(); len(got) != 1 {
		t.Errorf("the meeting went on although the owner could not be asked: %v", got)
	}
	asked = 0
	if _, err := Run(context.Background(), Options{Root: root, ID: stopped.ID, Roster: roster, Model: model, Repos: testRepos, Now: fixedNow, Ask: ask}); err != nil {
		t.Fatalf("going on: %v", err)
	}
	if asked != 1 {
		t.Errorf("going on asked the owner %d times, want once", asked)
	}
	if got := model.count("moderator", "agenda"); got != 1 {
		t.Errorf("the agenda was set %d times, want once: it is kept", got)
	}
}

// What the owner answered is kept with the plan: a meeting that stops in a
// topic and is gone on with tells the roles it still has to ask.
func TestRun_TheOwnersAnswersOutlastAStop(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, testRoster)
	base := curious(roster)
	broken := true
	model := &table{answer: func(c call) (string, error) {
		if broken && c.Step == "decide" && c.Topic == 2 {
			return "I cannot decide.", nil
		}
		return base(c)
	}}
	ask := func(ctx context.Context, questions []string) ([]string, error) {
		return []string{"A readiness probe.", "", ""}, nil
	}

	plan, err := Run(context.Background(), Options{Root: root, Brief: "Add a health endpoint.", Roster: roster, Model: model, Repos: testRepos, Now: fixedNow, Ask: ask})
	if err == nil || !strings.Contains(err.Error(), "the meeting stopped at topic 2 (build)") {
		t.Fatalf("err = %v, want the meeting stopped at the second topic", err)
	}

	// Gone on with, and nobody to ask this time.
	broken = false
	before := len(model.steps())
	resumed, err := Run(context.Background(), Options{Root: root, ID: plan.ID, Roster: roster, Model: model, Repos: testRepos, Now: fixedNow})
	if err != nil {
		t.Fatalf("going on: %v", err)
	}
	if len(resumed.Answers) != 3 || resumed.Answers[0].Answer != "A readiness probe." {
		t.Errorf("gone on with, the answers are %+v", resumed.Answers)
	}
	const said = "- Asked: Who calls the endpoint, and how often?\n  The owner: A readiness probe.\n"
	told := 0
	for _, c := range model.calls[before:] {
		if !strings.Contains(c.Prompt, said) && c.Role != "moderator" {
			t.Errorf("%s %s, asked after going on, is not told what the owner said", c.Role, c.Step)
		}
		told++
	}
	if told == 0 {
		t.Error("going on asked nothing")
	}
	if minutes := readFile(t, root, plan.Dir()+"/minutes.md"); !strings.Contains(minutes, "- Who calls the endpoint, and how often?\n  - A readiness probe.\n") {
		t.Errorf("the minutes of the plan gone on with do not hold the answer:\n%s", minutes)
	}
}
