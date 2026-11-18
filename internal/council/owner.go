package council

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// The owner of the project takes part in a meeting from outside the process
// that holds it: they say something while it goes on, ask it to stop, look at
// its agenda before the first topic, answer its questions. Each is a file in
// the plan's state folder, written by whoever the owner uses (the workbench,
// or awkit plan in another terminal) and read by the meeting between steps.

// OwnerWord is something the owner said to a meeting while it went on: a
// direction, a correction, or the answer to a question it raised. It holds
// like the brief from the step it is taken in on.
type OwnerWord struct {
	// ID names it in the minutes and the decision log: W1, W2, ...
	ID   string    `json:"id"`
	At   time.Time `json:"at"`
	Text string    `json:"text"`
	// Question is what it answers, when it answers a question the meeting
	// asked: one of the agenda, or what an assumption asked.
	Question string `json:"question,omitempty"`
}

// The files of the owner's part, in a plan's state folder.
const (
	wordsFile      = "owner.jsonl"
	pauseFile      = "pause"
	agendaReview   = "agenda-review"
	agendaReleased = "agenda-released"
	questionsAsked = "questions-in-page"
	questionsFile  = "questions.json"
	ownerAnswers   = "owner-answers.json"
)

// Waiting is how a meeting stops for its owner: the owner asked it to pause,
// or it waits for the owner to look at its agenda or answer its questions.
// It is no failure: the meeting goes on (awkit plan --resume) once the owner
// did what it waits for.
type Waiting struct {
	// State is LivePaused, LiveAgenda or LiveQuestions.
	State string
}

func (w *Waiting) Error() string {
	switch w.State {
	case LiveAgenda:
		return "the agenda is set, and waits for the owner to look at it"
	case LiveQuestions:
		return "the questions of the agenda wait for the owner's answers"
	default:
		return "the owner asked the meeting to pause"
	}
}

// planDir is a plan's folder, when the plan exists: it has its brief.
func planDir(root, id string) (string, error) {
	if !planID.MatchString(id) {
		return "", fmt.Errorf("%q is not the name of a plan", id)
	}
	dir := filepath.Join(root, filepath.FromSlash(PlansDir), id)
	if _, err := os.Stat(filepath.Join(dir, "brief.md")); err != nil {
		return "", fmt.Errorf("there is no plan %s", id)
	}
	return dir, nil
}

// saying keeps two words written at once from taking the same name.
var saying sync.Mutex

// Say puts what the owner says to a plan's meeting. A meeting that goes on
// takes it in before its next step; one that stopped, when it goes on.
func Say(root, id, text, question string, now time.Time) (OwnerWord, error) {
	text, question = strings.TrimSpace(text), strings.TrimSpace(question)
	if text == "" {
		return OwnerWord{}, errors.New("say something: the word is empty")
	}
	if _, err := planDir(root, id); err != nil {
		return OwnerWord{}, err
	}
	saying.Lock()
	defer saying.Unlock()
	words, err := Words(root, id)
	if err != nil {
		return OwnerWord{}, err
	}
	word := OwnerWord{ID: fmt.Sprintf("W%d", len(words)+1), At: now, Text: text, Question: question}
	line, err := json.Marshal(word)
	if err != nil {
		return OwnerWord{}, err
	}
	dir := StateDir(root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return OwnerWord{}, err
	}
	f, err := os.OpenFile(filepath.Join(dir, wordsFile), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return OwnerWord{}, err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		_ = f.Close()
		return OwnerWord{}, err
	}
	if err := f.Close(); err != nil {
		return OwnerWord{}, err
	}
	return word, nil
}

// Words returns what the owner said to a plan's meeting, in order.
func Words(root, id string) ([]OwnerWord, error) {
	if !planID.MatchString(id) {
		return nil, fmt.Errorf("%q is not the name of a plan", id)
	}
	f, err := os.Open(filepath.Join(StateDir(root, id), wordsFile))
	if errors.Is(err, os.ErrNotExist) {
		return []OwnerWord{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	words := []OwnerWord{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var word OwnerWord
		if err := json.Unmarshal([]byte(line), &word); err != nil || word.ID == "" {
			continue
		}
		words = append(words, word)
	}
	return words, scanner.Err()
}

// Pause asks a plan's meeting to stop after the step it is at.
func Pause(root, id string) error {
	if _, err := planDir(root, id); err != nil {
		return err
	}
	dir := StateDir(root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, pauseFile), []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644)
}

// ReleaseAgenda lets a meeting that waits for its owner to look at its agenda
// go on, with the agenda as it stands now. It is checked first.
func ReleaseAgenda(root, id string, roster *Roster) error {
	if _, err := planDir(root, id); err != nil {
		return err
	}
	if err := CheckAgendaFile(root, id, roster); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(StateDir(root, id), agendaReleased), []byte("released\n"), 0o644)
}

// CheckAgendaFile checks a plan's agenda as it stands, as the meeting would
// before it goes on with it.
func CheckAgendaFile(root, id string, roster *Roster) error {
	agenda, err := LoadAgenda(root, id)
	if err != nil {
		return err
	}
	return CheckAgenda(root, agenda, roster)
}

// Questions returns the questions of a plan's agenda that wait for the
// owner's answers; none when it waits for none.
func Questions(root, id string) ([]string, error) {
	if !planID.MatchString(id) {
		return nil, fmt.Errorf("%q is not the name of a plan", id)
	}
	data, err := os.ReadFile(filepath.Join(StateDir(root, id), questionsFile))
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var questions []string
	if err := json.Unmarshal(data, &questions); err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(StateDir(root, id), ownerAnswers)); err == nil {
		return []string{}, nil // answered already
	}
	return questions, nil
}

// AnswerQuestions gives a meeting that waits for them the owner's answers to
// the questions of its agenda; an empty answer leaves a question to the
// council to assume.
func AnswerQuestions(root, id string, answers []OwnerAnswer) error {
	questions, err := Questions(root, id)
	if err != nil {
		return err
	}
	if len(questions) == 0 {
		return fmt.Errorf("the meeting of plan %s waits for no answers", id)
	}
	for _, answer := range answers {
		if !contains(questions, answer.Question) {
			return fmt.Errorf("%q is not a question the meeting asked", oneLine(answer.Question))
		}
	}
	data, err := json.MarshalIndent(answers, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(StateDir(root, id), ownerAnswers), data, 0o644)
}

func (p *planner) stateFile(name string) string {
	return filepath.Join(p.session.stateDir, name)
}

func (p *planner) marked(name string) bool {
	_, err := os.Stat(p.stateFile(name))
	return err == nil
}

// askInPage is how a meeting asks its owner when the owner is not at its
// terminal: it writes the questions down and stops; the answers are there
// when it goes on.
func (p *planner) askInPage(ctx context.Context, questions []string) ([]string, error) {
	data, err := os.ReadFile(p.stateFile(ownerAnswers))
	if errors.Is(err, os.ErrNotExist) {
		list, err := json.MarshalIndent(questions, "", "  ")
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(p.stateFile(questionsFile), list, 0o644); err != nil {
			return nil, broken{err}
		}
		p.tell(Entry{Step: "the owner is asked", Kind: EntryQuestions, Data: questions})
		return nil, &Waiting{State: LiveQuestions}
	}
	if err != nil {
		return nil, err
	}
	var given []OwnerAnswer
	if err := json.Unmarshal(data, &given); err != nil {
		return nil, fmt.Errorf("%s cannot be read: %w", ownerAnswers, err)
	}
	answers := make([]string, len(questions))
	for i, question := range questions {
		for _, answer := range given {
			if answer.Question == question {
				answers[i] = answer.Answer
			}
		}
	}
	return answers, nil
}

// point is a place between two steps of the meeting where it may stop for
// its owner and takes in what the owner said: before a role speaks, before
// the moderator sums up, before the ruling, the documents and the work list.
// label names the place, once in a plan; next is the step that follows, when
// it is known, and speaker the role that is asked next.
//
// What the owner says is taken in at the first point the meeting reaches
// after it was said, so that the next role to speak is told. A meeting that
// goes on with a plan takes each word in where the earlier run did, and what
// it made of it is replayed there, so that it comes to the same place.
func (p *planner) point(ctx context.Context, plan *Plan, label, next, speaker string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	live := p.session.isLive(next)
	if live && p.marked(pauseFile) {
		return &Waiting{State: LivePaused}
	}
	if err := p.takeWords(ctx, plan, label, live); err != nil {
		return err
	}
	p.live.set(func(l *Live) {
		l.State, l.Step, l.Speaker, l.Topic = LiveMeeting, label, speaker, p.topic
		l.Calls = p.session.callsMade()
	})
	return nil
}

// takeWords takes in what the owner said: first the words an earlier run
// took in at this point, then, where the meeting is past what an earlier run
// did, the ones nobody has taken in yet.
func (p *planner) takeWords(ctx context.Context, plan *Plan, label string, live bool) error {
	words, err := Words(p.root, plan.ID)
	if err != nil || len(words) == 0 {
		return nil // a word that cannot be read is taken in when it can
	}
	var replayed, fresh []OwnerWord
	for _, word := range words {
		if p.taken[word.ID] {
			continue
		}
		if at, ok := p.session.remembered(wordStep(word)); ok {
			if at == label {
				replayed = append(replayed, word)
			}
			continue
		}
		if live {
			fresh = append(fresh, word)
		}
	}
	for _, batch := range [][]OwnerWord{replayed, fresh} {
		if len(batch) == 0 {
			continue
		}
		for _, word := range batch {
			if !p.session.isRemembered(wordStep(word)) {
				if err := p.session.remember(wordStep(word), label); err != nil {
					return broken{err}
				}
				p.tell(Entry{Step: wordStep(word), Kind: EntryWord, Data: word})
				p.session.say("The owner said (%s): %s", word.ID, oneLine(word.Text))
			}
			p.taken[word.ID] = true
			p.words = append(p.words, word)
		}
		plan.Words = p.words
		if err := p.review(ctx, plan, batch, label); err != nil {
			return err
		}
	}
	return nil
}

func wordStep(word OwnerWord) string {
	return "owner word " + word.ID
}

// tell puts what was said on the plan's transcript, when this run said it.
func (p *planner) tell(entry Entry) {
	switch entry.Kind {
	case EntryAgenda, EntryProposal, EntrySpeech, EntryExchange, EntrySummary, EntryRuling, EntryDocuments:
		// What a role said is told by the run that asked it; an earlier run
		// told what it was told.
		if !p.session.wasAsked(entry.Step) {
			return
		}
	}
	if entry.Topic == "" && p.topic > 0 && p.topic <= len(p.plan.Topics) {
		entry.Topic = p.plan.Topics[p.topic-1].Topic.ID
	}
	p.transcript.add(entry)
}

// NewName names a new plan as a meeting would: the day and the first words
// of the brief, and no plan of that name exists.
func NewName(root, brief string, now time.Time) string {
	return newID(root, brief, now)
}

// CheckAgenda checks an agenda as the meeting would before it holds it.
func CheckAgenda(root string, agenda *Agenda, roster *Roster) error {
	exists := func(doc string) bool {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(doc)))
		return err == nil && strings.TrimSpace(string(data)) != ""
	}
	return checkAgenda(agenda, roster, roster.Limits.Topics, exists)
}

// IsPlanName reports whether a name can name a plan: lowercase letters,
// digits and -, and nothing that climbs out of the plans folder.
func IsPlanName(id string) bool {
	return planID.MatchString(id)
}
