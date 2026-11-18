package council

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

func init() {
	// The steps of the owner's part, as the test table tells them from their
	// prompts.
	promptStep = append(promptStep,
		struct{ marker, step string }{"Your task now: you decide for the project. Read every decision", "ownerreview"},
		struct{ marker, step string }{"Your task now: read each of these documents again, all of it", "reread"},
		struct{ marker, step string }{"Your task now: lay the milestones and the tickets out again so that they follow the plan", "relist"},
	)
}

const ownerPlan = "20261114-owner"

func holdPlan(t *testing.T, root string, roster *Roster, model Model, opts Options) (*Plan, error) {
	t.Helper()
	opts.Root, opts.Roster, opts.Model, opts.Repos, opts.Now = root, roster, model, testRepos, fixedNow
	if opts.ID == "" {
		opts.Brief, opts.Name = "A health endpoint.", ownerPlan
	}
	return Run(context.Background(), opts)
}

// owning answers like agreeable, and as the owner's part asks: a review that
// replaces the first decision, documents read again, a list laid out again.
func owning(roster *Roster) func(c call) (string, error) {
	base := agreeable(roster)
	return func(c call) (string, error) {
		switch c.Step {
		case "ownerreview":
			return block(`{"note": "Phones first.", "replace": [{"id": "D1", "title": "scope", "decision": "Build the health endpoint, and show it on a phone.", "rationale": "The owner said so."}], "withdraw": [], "add": [], "answered": [], "documents": [{"path": "docs/product/requirements.md", "why": "it says what is built"}]}`), nil
		case "reread":
			var b strings.Builder
			for _, doc := range rereadAsked(c.Prompt) {
				fmt.Fprintf(&b, "<<<DOCUMENT %s>>>\n# %s\n\nRead again by %s: on a phone too (D2).\n<<<END>>>\n", doc, doc, c.Role)
			}
			return b.String(), nil
		case "relist":
			return block(goodWork), nil
		}
		return base(c)
	}
}

// rereadAsked lists the documents a role is asked to read again: the ones
// its prompt shows as they stand.
func rereadAsked(prompt string) []string {
	var out []string
	for _, path := range []string{"docs/product/requirements.md", "docs/product/journeys.md", "docs/qa/risks.md", "docs/tech/architecture.md"} {
		if strings.Contains(prompt, "Your documents to read again, as they stand:") && strings.Contains(prompt[strings.Index(prompt, "Your documents to read again, as they stand:"):], path) {
			out = append(out, path)
		}
	}
	return out
}

// speaking returns a table that calls when once, the first time a role is
// asked for a step of a topic.
type hook struct {
	once sync.Once
}

func (h *hook) at(c call, topic int, role, step string, do func()) {
	if c.Topic == topic && c.Role == role && c.Step == step {
		h.once.Do(do)
	}
}

// What the owner says reaches the next role to speak, in the same topic: not
// the next topic, and not a meeting held again.
func TestOwner_AWordReachesTheNextSpeaker(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, testRoster)
	var said hook
	model := &table{}
	model.answer = func(c call) (string, error) {
		said.at(c, 1, "designer", "speak", func() {
			if _, err := Say(root, ownerPlan, "It has to work on a phone.", "", fixedNow()); err != nil {
				t.Error(err)
			}
		})
		return owning(roster)(c)
	}
	if _, err := holdPlan(t, root, roster, model, Options{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(model.promptIn(1, "designer", "speak"), "It has to work on a phone.") {
		t.Error("the role that was speaking when the owner spoke was told: the word came after its prompt")
	}
	if !strings.Contains(model.promptIn(1, "skeptic", "speak"), "It has to work on a phone.") {
		t.Error("the next role to speak, in the same topic, was not told what the owner said")
	}
	if !strings.Contains(model.promptOf("builder", "worklist"), "It has to work on a phone.") {
		t.Error("the work list was laid out without what the owner said")
	}
	// Nothing was decided when it was said: nothing is read again.
	if n := model.count("owner", "ownerreview"); n != 0 {
		t.Errorf("%d reviews; nothing was decided before the word", n)
	}
	minutes := readFile(t, root, PlansDir+"/"+ownerPlan+"/minutes.md")
	if !strings.Contains(minutes, "## What the owner said while the meeting went on") || !strings.Contains(minutes, "W1: It has to work on a phone.") {
		t.Errorf("the minutes do not say what the owner said:\n%s", minutes)
	}
	entries, err := ReadTranscript(root, ownerPlan, 0)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, entry := range entries {
		kinds = append(kinds, entry.Kind+":"+entry.Role)
	}
	joined := strings.Join(kinds, " ")
	for _, want := range []string{"agenda:moderator", "proposal:owner", "speech:designer", "word:", "speech:skeptic", "summary:moderator", "ruling:owner", "documents:owner", "worklist:builder", "stopped:"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the transcript lacks %s: %s", want, joined)
		}
	}
	if strings.Index(joined, "word:") > strings.Index(joined, "speech:skeptic") {
		t.Errorf("the word is told after the speech it came before: %s", joined)
	}
	live, err := ReadLive(root, ownerPlan)
	if err != nil || live.State != LiveFinished || live.Topics != 2 || live.PID != os.Getpid() {
		t.Errorf("live = %+v, %v", live, err)
	}
}

// What the owner says after something was decided has the deciding role read
// every decision of the plan again, and the owners read again in full every
// document that rests on what changed. The topics are not held again.
func TestOwner_AWordAfterARulingHasThePlanReadAgain(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, testRoster)
	var said hook
	model := &table{}
	model.answer = func(c call) (string, error) {
		said.at(c, 2, "builder", "propose", func() {
			if _, err := Say(root, ownerPlan, "It has to work on a phone.", "", fixedNow()); err != nil {
				t.Error(err)
			}
		})
		return owning(roster)(c)
	}
	plan, err := holdPlan(t, root, roster, model, Options{})
	if err != nil {
		t.Fatal(err)
	}

	review := model.promptOf("owner", "ownerreview")
	for _, want := range []string{"It has to work on a phone.", "D1. scope: Build the health endpoint only."} {
		if !strings.Contains(review, want) {
			t.Errorf("the review is not shown %q:\n%s", want, review)
		}
	}
	steps := strings.Join(model.steps(), "\n")
	// Before the next role speaks: the review, then the documents.
	if at, next := strings.Index(steps, "owner ownerreview"), strings.Index(steps, "designer speak\nskeptic speak\nmoderator summary\nowner decide"); at < 0 || next < 0 {
		t.Fatalf("calls:\n%s", steps)
	}
	// Every document a topic whose decision changed ended in, read again by
	// its owner, whole; none of the topics held again.
	for _, role := range []string{"owner", "designer", "skeptic"} {
		if model.count(role, "reread") != 1 {
			t.Errorf("%s read its document again %d times, want once", role, model.count(role, "reread"))
		}
	}
	if model.count("owner", "propose") != 1 || model.count("builder", "propose") != 1 {
		t.Errorf("a topic was held again:\n%s", steps)
	}
	if got := readFile(t, root, "docs/product/journeys.md"); !strings.Contains(got, "Read again by designer: on a phone too (D2).") {
		t.Errorf("journeys.md was not written again:\n%s", got)
	}

	// The decision log only grows: D1 stays, D2 replaces it, the topic after
	// counts on from there.
	log := readFile(t, root, DecisionsPath)
	for _, want := range []string{"- **D1 scope**: Build the health endpoint only.", "### What the owner said (W1)", "- **D2 scope** (replaces D1): Build the health endpoint, and show it on a phone.", "- **D3 stack**"} {
		if !strings.Contains(log, want) {
			t.Errorf("the decision log lacks %q:\n%s", want, log)
		}
	}
	// What the table is told from then on holds D2 and not D1.
	decide := model.promptIn(2, "owner", "decide")
	if !strings.Contains(decide, "D2. scope: Build the health endpoint, and show it on a phone.") || strings.Contains(decide, "D1. scope") {
		t.Errorf("the ruling of the next topic is not shown the plan as it holds:\n%s", decide)
	}
	if len(plan.Revisions) != 1 || plan.Revisions[0].At != "topic build designer speaks" {
		t.Errorf("revisions = %+v", plan.Revisions)
	}

	record, err := ReadRecord(root, ownerPlan)
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]DecisionState{}
	for _, d := range record.Decisions {
		states[d.ID] = d
	}
	if states["D1"].ReplacedBy != "D2" || states["D2"].Replaces != "D1" || states["D3"].Topic != "build" {
		t.Errorf("record decisions = %+v", record.Decisions)
	}
	if record.Status != LiveFinished || len(record.Words) != 1 || len(record.Revisions) != 1 {
		t.Errorf("record = %+v", record)
	}
}

// A meeting that goes on with a plan takes each word in where the earlier
// run did, and replays what was made of it: no review is asked for twice.
func TestOwner_GoingOnReplaysAWordWhereItWasTakenIn(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, testRoster)
	var said hook
	failing := true
	model := &table{}
	model.answer = func(c call) (string, error) {
		said.at(c, 2, "builder", "propose", func() {
			if _, err := Say(root, ownerPlan, "It has to work on a phone.", "", fixedNow()); err != nil {
				t.Error(err)
			}
		})
		if c.Step == "worklist" && failing {
			return "", errors.New("the model is down")
		}
		return owning(roster)(c)
	}
	if _, err := holdPlan(t, root, roster, model, Options{}); err == nil {
		t.Fatal("the meeting did not stop at the work list")
	}
	failing = false
	before := len(model.steps())
	plan, err := holdPlan(t, root, roster, model, Options{ID: ownerPlan})
	if err != nil {
		t.Fatal(err)
	}
	again := append([]string(nil), model.steps()[before:]...)
	sort.Strings(again[1:]) // the table looks at the list all at once
	if strings.Join(again, ",") != "builder worklist,designer review,owner review,skeptic review" {
		t.Errorf("going on asked: %v", again)
	}
	if model.count("owner", "ownerreview") != 1 || len(plan.Revisions) != 1 || len(plan.Words) != 1 {
		t.Errorf("the word was not replayed where it was taken in: %d reviews, %+v", model.count("owner", "ownerreview"), plan.Revisions)
	}
	if strings.Count(readFile(t, root, DecisionsPath), "### What the owner said (W1)") != 1 {
		t.Error("the review is in the decision log twice")
	}
}

// What the owner says once the meeting is over is taken in when it goes on:
// the plan is read again, and the work list laid out again from it. Nothing
// else is asked again.
func TestOwner_AWordAfterTheMeetingLaysTheListOutAgain(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, testRoster)
	model := &table{answer: owning(roster)}
	if _, err := holdPlan(t, root, roster, model, Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := Say(root, ownerPlan, "It has to work on a phone.", "", fixedNow()); err != nil {
		t.Fatal(err)
	}
	before := len(model.steps())
	if _, err := holdPlan(t, root, roster, model, Options{ID: ownerPlan}); err != nil {
		t.Fatal(err)
	}
	again := strings.Join(model.steps()[before:], ",")
	if !strings.HasPrefix(again, "owner ownerreview,") || !strings.HasSuffix(again, ",builder relist") || strings.Contains(again, "propose") {
		t.Errorf("going on asked: %s", again)
	}
	if !strings.Contains(model.promptOf("builder", "relist"), "D3 replaces D1") {
		t.Error("the list was laid out again without what the review changed")
	}
}

// The owner asks a meeting to pause: it stops after the step it is at, says
// that it waits, and goes on from there with nothing asked twice.
func TestOwner_APauseStopsAfterTheStep(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, testRoster)
	var paused hook
	model := &table{}
	model.answer = func(c call) (string, error) {
		paused.at(c, 1, "designer", "speak", func() {
			if err := Pause(root, ownerPlan); err != nil {
				t.Error(err)
			}
		})
		return owning(roster)(c)
	}
	_, err := holdPlan(t, root, roster, model, Options{})
	var waiting *Waiting
	if !errors.As(err, &waiting) || waiting.State != LivePaused {
		t.Fatalf("err = %v, want a pause", err)
	}
	if steps := model.steps(); steps[len(steps)-1] != "designer speak" {
		t.Errorf("the meeting went on past the step it was at: %v", steps)
	}
	if live, _ := ReadLive(root, ownerPlan); live == nil || live.State != LivePaused {
		t.Errorf("live = %+v", live)
	}
	if !strings.Contains(readFile(t, root, PlansDir+"/"+ownerPlan+"/minutes.md"), "**waits for the owner**") {
		t.Error("the minutes do not say the meeting waits")
	}
	before := len(model.steps())
	if _, err := holdPlan(t, root, roster, model, Options{ID: ownerPlan}); err != nil {
		t.Fatal(err)
	}
	if first := model.steps()[before]; first != "skeptic speak" {
		t.Errorf("going on started at %s, not where the pause was", first)
	}
	if model.count("designer", "speak") != 2 { // once a topic
		t.Errorf("designer spoke %d times", model.count("designer", "speak"))
	}
}

// An owner who asked to see the agenda first is shown it; the meeting goes
// on with the agenda as they changed it, once they let it.
func TestOwner_TheAgendaWaitsForTheOwner(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, testRoster)
	model := &table{answer: owning(roster)}
	_, err := holdPlan(t, root, roster, model, Options{ReviewAgenda: true})
	var waiting *Waiting
	if !errors.As(err, &waiting) || waiting.State != LiveAgenda {
		t.Fatalf("err = %v, want the agenda to wait", err)
	}
	if steps := model.steps(); len(steps) != 1 || steps[0] != "moderator agenda" {
		t.Fatalf("calls before the owner saw the agenda: %v", steps)
	}
	// Going on before the owner let it goes nowhere.
	if _, err := holdPlan(t, root, roster, model, Options{ID: ownerPlan}); !errors.As(err, &waiting) || waiting.State != LiveAgenda {
		t.Fatalf("went on without the owner: %v", err)
	}

	agenda, err := LoadAgenda(root, ownerPlan)
	if err != nil {
		t.Fatal(err)
	}
	broken := *agenda
	broken.Topics = append([]Topic(nil), agenda.Topics...)
	broken.Topics[0].Lead = "nobody"
	if err := SaveAgenda(root, &broken); err != nil {
		t.Fatal(err)
	}
	if err := ReleaseAgenda(root, ownerPlan, roster); err == nil {
		t.Error("an agenda the table cannot hold was let go")
	}
	agenda.Topics[0].Title = "What is built, for a phone"
	if err := SaveAgenda(root, agenda); err != nil {
		t.Fatal(err)
	}
	if err := ReleaseAgenda(root, ownerPlan, roster); err != nil {
		t.Fatal(err)
	}
	if _, err := holdPlan(t, root, roster, model, Options{ID: ownerPlan}); err != nil {
		t.Fatal(err)
	}
	if model.count("moderator", "agenda") != 1 || !strings.Contains(model.promptIn(1, "owner", "propose"), "What is built, for a phone") {
		t.Error("the meeting did not go on with the agenda as the owner left it")
	}
}

// Questions put to an owner who is not at the terminal: the meeting writes
// them down and waits; it goes on with the answers, and every role is told
// them.
func TestOwner_QuestionsWaitForAnswersInThePage(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, testRoster)
	model := &table{}
	model.answer = func(c call) (string, error) {
		if c.Step == "agenda" {
			agenda := strings.Replace(agendaFor(roster), `"questions": []`, `"questions": ["Who uses it?", "How often?"]`, 1)
			return block(agenda), nil
		}
		return owning(roster)(c)
	}
	_, err := holdPlan(t, root, roster, model, Options{AskInPage: true})
	var waiting *Waiting
	if !errors.As(err, &waiting) || waiting.State != LiveQuestions {
		t.Fatalf("err = %v, want the questions to wait", err)
	}
	questions, err := Questions(root, ownerPlan)
	if err != nil || strings.Join(questions, "|") != "Who uses it?|How often?" {
		t.Fatalf("questions = %v, %v", questions, err)
	}
	if err := AnswerQuestions(root, ownerPlan, []OwnerAnswer{{Question: "Who is it?", Answer: "x"}}); err == nil {
		t.Error("an answer to a question nobody asked was taken")
	}
	if err := AnswerQuestions(root, ownerPlan, []OwnerAnswer{{Question: "Who uses it?", Answer: "The on-call engineer."}, {Question: "How often?"}}); err != nil {
		t.Fatal(err)
	}
	if left, _ := Questions(root, ownerPlan); len(left) != 0 {
		t.Errorf("questions still wait: %v", left)
	}
	if _, err := holdPlan(t, root, roster, model, Options{ID: ownerPlan}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(model.promptIn(1, "owner", "propose"), "The on-call engineer.") {
		t.Error("the lead was not told the owner's answer")
	}
}

// Live says where the meeting is while it goes on: the topic, the step, the
// role that is asked.
func TestOwner_LiveSaysWhereTheMeetingIs(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, testRoster)
	var looked hook
	var seen *Live
	model := &table{}
	model.answer = func(c call) (string, error) {
		looked.at(c, 2, "skeptic", "speak", func() { seen, _ = ReadLive(root, ownerPlan) })
		return owning(roster)(c)
	}
	if _, err := holdPlan(t, root, roster, model, Options{}); err != nil {
		t.Fatal(err)
	}
	if seen == nil || seen.State != LiveMeeting || seen.Topic != 2 || seen.Topics != 2 || seen.Speaker != "skeptic" || seen.Step != "topic build skeptic speaks" || seen.Limit != 60 {
		t.Errorf("live while skeptic spoke to topic 2 = %+v", seen)
	}
}

// Say, Pause and the answers are refused for a plan that is not there.
func TestOwner_RefusesAPlanThatIsNotThere(t *testing.T) {
	root := t.TempDir()
	if _, err := Say(root, "nothing", "hello", "", fixedNow()); err == nil {
		t.Error("said to a plan that is not there")
	}
	if _, err := Say(root, "../x", "hello", "", fixedNow()); err == nil {
		t.Error("said to a path")
	}
	if err := Pause(root, "nothing"); err == nil {
		t.Error("paused a plan that is not there")
	}
	putFile(t, root, PlansDir+"/p1/brief.md", "x\n")
	if _, err := Say(root, "p1", "   ", "", fixedNow()); err == nil {
		t.Error("an empty word was taken")
	}
	w1, err := Say(root, "p1", "first", "", fixedNow())
	w2, err2 := Say(root, "p1", "second", "Who?", fixedNow())
	if err != nil || err2 != nil || w1.ID != "W1" || w2.ID != "W2" || w2.Question != "Who?" {
		t.Errorf("words = %+v %+v (%v %v)", w1, w2, err, err2)
	}
	if data, _ := os.ReadFile(filepath.Join(StateDir(root, "p1"), wordsFile)); strings.Count(string(data), "\n") != 2 {
		t.Errorf("owner.jsonl:\n%s", data)
	}
	var check []OwnerWord
	words, _ := Words(root, "p1")
	raw, _ := json.Marshal(words)
	_ = json.Unmarshal(raw, &check)
	if len(check) != 2 || check[1].Text != "second" {
		t.Errorf("words read back = %+v", check)
	}
}
