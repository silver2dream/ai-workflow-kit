package council

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// curious is the agreeable table whose roles each have a question for the
// owner of the project, two of them the same one in other words.
func curious(roster *Roster) func(c call) (string, error) {
	base := agreeable(roster)
	asks := map[string]string{
		"owner":    "Who calls the endpoint?",
		"designer": "Does a person ever read the answer?",
		"builder":  "What calls this endpoint, and how often?",
		"skeptic":  "May the endpoint be reached from outside?",
	}
	return func(c call) (string, error) {
		switch c.Step {
		case "position":
			return block(`{"summary": "` + c.Role + ` holds its line.", "proposals": ["` + c.Role + ` proposes"], "concerns": [], "questions": ["` + asks[c.Role] + `"]}`), nil
		case "questions":
			return block(`{"questions": ["Who calls the endpoint, and how often?", "Does a person ever read the answer?", " ", "May the endpoint be reached from outside?"]}`), nil
		}
		return base(c)
	}
}

// The table's questions are put to the owner before the roles object to each
// other. What the owner answers is known to every role from there on; what
// the owner leaves open is left to the council to assume.
func TestRun_TheOwnerIsAskedBeforeTheTableArgues(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, testRoster)
	model := &table{answer: curious(roster)}
	var put [][]string
	ask := func(ctx context.Context, questions []string) ([]string, error) {
		put = append(put, questions)
		return []string{"  A readiness probe,\nevery ten seconds. ", "", "No."}, nil
	}
	var told []string

	plan, err := Run(context.Background(), Options{Root: root, Brief: "Add a health endpoint.", Roster: roster, Model: model, Repos: testRepos,
		Now: fixedNow, Ask: ask, Progress: func(line string) { told = append(told, line) }})

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// One list, made by the moderator from what four roles asked.
	wantQuestions := []string{"Who calls the endpoint, and how often?", "Does a person ever read the answer?", "May the endpoint be reached from outside?"}
	if len(put) != 1 || !reflect.DeepEqual(put[0], wantQuestions) {
		t.Fatalf("the owner was asked %v, want once: %v", put, wantQuestions)
	}
	moderator := model.promptOf("moderator", "questions")
	for _, want := range []string{"You have no opinion on the subject", "  - owner: Who calls the endpoint?\n", "  - builder: What calls this endpoint, and how often?\n", "At most 7 questions."} {
		if !strings.Contains(moderator, want) {
			t.Errorf("the moderator's question should say %q:\n%s", want, moderator)
		}
	}
	want := []OwnerAnswer{
		{Question: "Who calls the endpoint, and how often?", Answer: "A readiness probe,\nevery ten seconds."},
		{Question: "Does a person ever read the answer?"},
		{Question: "May the endpoint be reached from outside?", Answer: "No."},
	}
	if !reflect.DeepEqual(plan.Answers, want) {
		t.Errorf("answers = %+v\nwant %+v", plan.Answers, want)
	}

	// The owner was asked after the positions and before anybody objected.
	steps := model.steps()
	at := func(step string) int {
		for i, s := range steps {
			if s == step {
				return i
			}
		}
		return -1
	}
	if q := at("moderator questions"); q < 4 || q > at("owner objections") || q > at("skeptic objections") {
		t.Errorf("the questions were put at call %d: %v", q, steps)
	}

	// Every role is told what the owner said, from the objections on, and so
	// is the moderator who sums up. What was left open is not passed off as
	// an answer.
	said := "The owner of the project was asked by this table, and answered. This holds like the brief:\n" +
		"- Asked: Who calls the endpoint, and how often?\n  The owner: A readiness probe, every ten seconds.\n" +
		"- Asked: May the endpoint be reached from outside?\n  The owner: No.\n"
	for _, c := range []struct{ role, step string }{{"skeptic", "objections"}, {"moderator", "summary"}, {"owner", "decide"}, {"designer", "documents"}, {"builder", "worklist"}, {"skeptic", "review"}} {
		prompt := model.promptOf(c.role, c.step)
		if !strings.Contains(prompt, said) {
			t.Errorf("%s %s is not told what the owner said:\n%s", c.role, c.step, prompt)
		}
		if strings.Contains(prompt, "- Asked: Does a person ever read the answer?") {
			t.Errorf("%s %s is told of an answer the owner did not give", c.role, c.step)
		}
	}
	if position := model.promptOf("skeptic", "position"); strings.Contains(position, "The owner of the project was asked") {
		t.Error("a role was told the owner's answers before the owner was asked")
	}

	minutes := readFile(t, root, plan.Dir()+"/minutes.md")
	for _, want := range []string{"## What the owner was asked\n", "- Who calls the endpoint, and how often?\n  - A readiness probe, every ten seconds.\n",
		"- Does a person ever read the answer?\n  - (left open: the council assumes)\n"} {
		if !strings.Contains(minutes, want) {
			t.Errorf("the minutes should say %q:\n%s", want, minutes)
		}
	}
	if strings.Index(minutes, "## What the owner was asked") > strings.Index(minutes, "## Documents") || strings.Index(minutes, "## What the owner was asked") < strings.Index(minutes, "## Positions") {
		t.Error("the owner's answers are not between the positions and what followed")
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

// With nobody to ask, the meeting does not wait and spends no call on a list
// of questions: the deciding role says what the plan assumes.
func TestRun_WithNobodyToAskTheMeetingDoesNotWait(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, testRoster)
	model := &table{answer: curious(roster)}

	plan, err := runPlan(t, root, roster, model, "Add a health endpoint.")

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := model.count("moderator", "questions"); got != 0 || len(plan.Answers) != 0 {
		t.Errorf("without anybody to ask: %d calls to make the list, answers %+v", got, plan.Answers)
	}
	if minutes := readFile(t, root, plan.Dir()+"/minutes.md"); strings.Contains(minutes, "## What the owner was asked") {
		t.Errorf("the minutes say the owner was asked:\n%s", minutes)
	}
	// What the roles asked still reaches the moderator and the deciding role.
	if summary := model.promptOf("moderator", "summary"); !strings.Contains(summary, "  - skeptic: May the endpoint be reached from outside?\n") {
		t.Errorf("the moderator is not shown what the roles asked:\n%s", summary)
	}
}

// When the owner is asked: not at all without a question, a few questions as
// they were asked, never more than seven at once; an owner who cannot be
// asked stops the meeting there.
func TestRun_PuttingTheQuestions(t *testing.T) {
	roster := mustRoster(t, testRoster)
	asked := 0
	var put []string
	ask := func(ctx context.Context, questions []string) ([]string, error) {
		asked++
		put = questions
		return nil, nil
	}

	// No role has a question: the owner is not disturbed.
	model := &table{answer: agreeable(roster)}
	if _, err := Run(context.Background(), Options{Root: t.TempDir(), Brief: "Add a health endpoint.", Roster: roster, Model: model, Repos: testRepos, Now: fixedNow, Ask: ask}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if asked != 0 || model.count("moderator", "questions") != 0 {
		t.Errorf("with no question the owner was asked %d times", asked)
	}

	// Two roles ask: their questions are put as they were asked, and an owner
	// who answers none leaves all of them to the council.
	base := agreeable(roster)
	model = &table{answer: func(c call) (string, error) {
		if c.Step == "position" && (c.Role == "designer" || c.Role == "skeptic") {
			return block(`{"summary": "` + c.Role + ` holds its line.", "proposals": [], "concerns": [], "questions": ["What does ` + c.Role + ` need to know?", ""]}`), nil
		}
		return base(c)
	}}
	root := t.TempDir()
	plan, err := Run(context.Background(), Options{Root: root, Brief: "Add a health endpoint.", Roster: roster, Model: model, Repos: testRepos, Now: fixedNow, Ask: ask})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if want := []string{"What does designer need to know?", "What does skeptic need to know?"}; asked != 1 || !reflect.DeepEqual(put, want) {
		t.Errorf("the owner was asked %d times: %v, want once: %v", asked, put, want)
	}
	if got := model.count("moderator", "questions"); got != 0 {
		t.Errorf("the moderator was asked %d times to make a list of two questions", got)
	}
	if len(plan.Answers) != 2 || plan.Answers[0].Answer != "" || plan.Answers[1].Answer != "" {
		t.Errorf("answers = %+v, want two questions left open", plan.Answers)
	}
	if prompt := model.promptOf("owner", "decide"); strings.Contains(prompt, "The owner of the project was asked") {
		t.Errorf("the deciding role is told the owner answered, and the owner answered nothing:\n%s", prompt)
	}

	// Three roles ask: still few enough to be put as they were asked.
	model = &table{answer: func(c call) (string, error) {
		if c.Step == "position" && c.Role != "skeptic" {
			return block(`{"summary": "` + c.Role + ` holds its line.", "proposals": [], "concerns": [], "questions": ["What does ` + c.Role + ` need to know?"]}`), nil
		}
		return base(c)
	}}
	asked = 0
	if _, err := Run(context.Background(), Options{Root: t.TempDir(), Brief: "Add a health endpoint.", Roster: roster, Model: model, Repos: testRepos, Now: fixedNow, Ask: ask}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if want := []string{"What does owner need to know?", "What does designer need to know?", "What does builder need to know?"}; asked != 1 || !reflect.DeepEqual(put, want) {
		t.Errorf("the owner was asked %d times: %v, want once: %v", asked, put, want)
	}
	if got := model.count("moderator", "questions"); got != 0 {
		t.Errorf("the moderator was asked %d times to make a list of three questions", got)
	}

	// More questions than are put at once: the first seven.
	many := `"q1", "q2", "q3", "q4", "q5", "q6", "q7", "q8", "q9"`
	curiousTable := curious(roster)
	model = &table{answer: func(c call) (string, error) {
		if c.Step == "questions" {
			return block(`{"questions": [` + many + `]}`), nil
		}
		return curiousTable(c)
	}}
	if _, err := Run(context.Background(), Options{Root: t.TempDir(), Brief: "Add a health endpoint.", Roster: roster, Model: model, Repos: testRepos, Now: fixedNow, Ask: ask}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(put) != 7 || put[6] != "q7" {
		t.Errorf("the owner was put %d questions: %v, want the first seven", len(put), put)
	}

	// An owner who cannot be asked stops the meeting there, and going on asks.
	model = &table{answer: curious(roster)}
	root = t.TempDir()
	failing := func(ctx context.Context, questions []string) ([]string, error) {
		return nil, errors.New("the terminal is gone")
	}
	stopped, err := Run(context.Background(), Options{Root: root, Brief: "Add a health endpoint.", Roster: roster, Model: model, Repos: testRepos, Now: fixedNow, Ask: failing})
	if err == nil || !strings.Contains(err.Error(), "the meeting stopped at the questions: the terminal is gone") {
		t.Fatalf("err = %v", err)
	}
	if got := model.count("owner", "objections"); got != 0 {
		t.Errorf("the table went on to object although the owner could not be asked")
	}
	asked = 0
	if _, err := Run(context.Background(), Options{Root: root, ID: stopped.ID, Roster: roster, Model: model, Repos: testRepos, Now: fixedNow, Ask: ask}); err != nil {
		t.Fatalf("going on: %v", err)
	}
	if asked != 1 {
		t.Errorf("going on asked the owner %d times, want once", asked)
	}
	if got := model.count("moderator", "questions"); got != 1 {
		t.Errorf("the moderator made the list %d times, want once: it is kept", got)
	}
	// A list with nothing in it is asked for again.
	empties := 0
	model = &table{answer: func(c call) (string, error) {
		if c.Step == "questions" {
			empties++
			if empties == 1 {
				return block(`{"questions": [" "]}`), nil
			}
		}
		return curiousTable(c)
	}}
	if _, err := Run(context.Background(), Options{Root: t.TempDir(), Brief: "Add a health endpoint.", Roster: roster, Model: model, Repos: testRepos, Now: fixedNow, Ask: ask}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if empties != 2 {
		t.Errorf("a list without a question was asked for %d times, want twice", empties)
	}
}

// What the owner answered is kept with the plan: a meeting that stops after
// the questions and is gone on with tells the roles it still has to ask.
func TestRun_TheOwnersAnswersOutlastAStop(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, testRoster)
	base := curious(roster)
	broken := true
	model := &table{answer: func(c call) (string, error) {
		if broken && c.Step == "decide" {
			return "I cannot decide.", nil
		}
		return base(c)
	}}
	ask := func(ctx context.Context, questions []string) ([]string, error) {
		return []string{"A readiness probe.", "", ""}, nil
	}

	plan, err := Run(context.Background(), Options{Root: root, Brief: "Add a health endpoint.", Roster: roster, Model: model, Repos: testRepos, Now: fixedNow, Ask: ask})
	if err == nil || !strings.Contains(err.Error(), "the meeting stopped at the decisions") {
		t.Fatalf("err = %v, want the meeting stopped at the decisions", err)
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
	if got := model.count("moderator", "questions"); got != 1 {
		t.Errorf("the list of questions was made %d times, want once", got)
	}
	const said = "- Asked: Who calls the endpoint, and how often?\n  The owner: A readiness probe.\n"
	told := 0
	for _, c := range model.calls[before:] {
		if !strings.Contains(c.Prompt, said) {
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
