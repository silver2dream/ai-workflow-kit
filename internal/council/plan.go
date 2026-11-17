package council

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/gamedata"
	"github.com/silver2dream/ai-workflow-kit/internal/rules"
)

// Repo is a repo of the project: what a ticket can be for.
type Repo struct {
	Name     string
	Language string
	Path     string
}

// Options is what a plan starts from.
type Options struct {
	// Root is the project's root directory.
	Root string
	// Brief says what is wanted. It may be empty when ID names a plan to go on with.
	Brief string
	// Roster is who sits at the table.
	Roster *Roster
	// Model answers for the roles.
	Model Model
	// Repos are the repos a ticket can be for.
	Repos []Repo
	// Rules are the project's rule files (rules.LoadAll). The roles write
	// their documents by the ones that hold for those files, and what they
	// wrote is read against them. The rules AWK ships are not among them
	// (OfTheKit). Without rules the documents are written without, and
	// nothing is checked.
	Rules []rules.Rule
	// ID names a plan to go on with. Empty starts a new one.
	ID string
	// MaxTickets bounds the work list (12 when zero).
	MaxTickets int
	// Now is the clock (time.Now when nil).
	Now func() time.Time
	// Progress is told what the meeting is doing, a line at a time, and never
	// by two goroutines at once.
	Progress func(string)
	// Ask puts the questions of the agenda to the owner of the project and
	// returns an answer for each, in their order; an empty answer leaves the
	// question open. Without it nobody is asked: the meeting never waits for a
	// person, and the deciding role says what the plan assumes instead.
	Ask func(ctx context.Context, questions []string) ([]string, error)
}

// Objection is what one role cannot accept in what another said.
type Objection struct {
	From  string `json:"-"`
	To    string `json:"to"`
	Point string `json:"point"`
	Why   string `json:"why"`
	Wants string `json:"wants"`
}

// Dispute is an objection that is argued out between two roles.
type Dispute struct {
	Topic      string `json:"topic"`
	Challenger string `json:"challenger"`
	Defender   string `json:"defender"`
	Point      string `json:"point"`
	Wants      string `json:"wants"`

	Exchanges []Exchange `json:"-"`
	// Settled says the two ended in agreement. A dispute that is not settled
	// goes to the deciding role.
	Settled bool   `json:"-"`
	Outcome string `json:"-"`
}

// Exchange is one round of a dispute: the defender's answer and, unless the
// defender conceded, what the challenger made of it.
type Exchange struct {
	Stance   string // concede, revise or hold
	Reply    string
	Position string

	Answered   bool // the challenger was asked
	Accepts    bool
	Answer     string
	StillWants string
}

// OwnerAnswer is what the owner of the project said to a question of the
// agenda. An empty answer means the owner left it open.
type OwnerAnswer struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// Summary is where a topic stands, by the moderator.
type Summary struct {
	Consensus []string `json:"consensus"`
	Open      []string `json:"open"`
	Questions []string `json:"questions"`
}

// Decision is a ruling of the deciding role.
type Decision struct {
	// ID names the decision in the log and wherever it is referred to: D7. It
	// is given here, not by the role, and counts on from the decisions that
	// are on record, so that it names one decision for as long as the project
	// lasts.
	ID        string `json:"-"`
	Title     string `json:"title"`
	Decision  string `json:"decision"`
	Rationale string `json:"rationale"`
	Dissent   string `json:"dissent"`
}

// Assumption is what a plan takes to be true where only the owner of the
// project can say. The meeting does not wait for the answer; the owner is
// shown what the plan rests on.
type Assumption struct {
	// ID names the assumption the way a decision is named: A3.
	ID         string `json:"-"`
	Question   string `json:"question"`
	Assumption string `json:"assumption"`
	IfWrong    string `json:"if_wrong"`
}

// WrittenDocument is what became of a document in a plan.
type WrittenDocument struct {
	Path  string
	Owner string
	// Unchanged says the owner left the document as it stood.
	Unchanged bool
	// Refused says why the document was not written; empty when it was.
	Refused string
}

// RuleCheck is what reading one role's documents against the project's rules
// came to.
type RuleCheck struct {
	// Owner is the role whose documents were read.
	Owner string
	// Found is what the reading found, before the owner put anything right.
	Found []RuleFinding
	// Settled says how it ended is known: nothing was found, or the owner had
	// its turn at putting it right and the reader said what still stands. It
	// is false when the meeting stopped in between.
	Settled bool
	// Open is what still stands after the owner's turn.
	Open []RuleFinding
}

// ListObjection is what a role finds wrong with the work list.
type ListObjection struct {
	From   string `json:"-"`
	Ticket string `json:"ticket"`
	Point  string `json:"point"`
	Wants  string `json:"wants"`
}

// Plan is what a meeting produced.
type Plan struct {
	// ID names the plan; its folder is docs/plans/<ID>.
	ID    string
	Brief string

	// Agenda is what the meeting takes up, in order.
	Agenda *Agenda
	// Answers are what the owner of the project said when the questions of
	// the agenda were put to them; empty when nobody was asked.
	Answers []OwnerAnswer
	// Topics are the topics of the agenda as far as they were held, in order.
	Topics []TopicRecord

	// Decisions and Assumptions are those of every topic, in order.
	Decisions   []Decision
	Assumptions []Assumption
	// Documents is what became of the documents of every topic, in order.
	Documents []WrittenDocument
	// Rules are the rule files of the project the documents were written by,
	// and RulesLeft the ones that hold for them too and were too long,
	// together with the others, to be passed on.
	Rules, RulesLeft []string
	// RuleChecks are what reading the documents against those rules came to,
	// a role and a topic at a time; only what was read is in it.
	RuleChecks []RuleCheck
	// GameData are the files of the game data the plan wrote; DataOpen is
	// what the check of the game data finds in them now; Reviews are the
	// pages that show them for a person to review.
	GameData []string
	DataOpen []gamedata.Finding
	Reviews  []string

	// Milestones are the steps the plan is built in; only the first one is
	// ticketed.
	Milestones     []Milestone
	Tickets        []Ticket
	ListObjections []ListObjection
	Later          []string
	Kept           []string

	// Calls is how many model calls this run made.
	Calls int
}

// Dir returns the plan's folder, relative to the project root.
func (plan *Plan) Dir() string {
	return PlansDir + "/" + plan.ID
}

// OpenFindings lists the points where a document is still at odds with a
// rule of the project after its owner had a turn at putting it right.
func (plan *Plan) OpenFindings() []RuleFinding {
	var open []RuleFinding
	for _, check := range plan.RuleChecks {
		open = append(open, check.Open...)
	}
	return open
}

// gather puts together what the topics came to, in their order.
func (plan *Plan) gather() {
	plan.Decisions, plan.Assumptions, plan.Documents, plan.RuleChecks = nil, nil, nil, nil
	plan.Rules, plan.RulesLeft = nil, nil
	for _, topic := range plan.Topics {
		plan.Decisions = append(plan.Decisions, topic.Decisions...)
		plan.Assumptions = append(plan.Assumptions, topic.Assumptions...)
		plan.Documents = append(plan.Documents, topic.Documents...)
		plan.RuleChecks = append(plan.RuleChecks, topic.RuleChecks...)
		for _, file := range topic.Rules {
			if !contains(plan.Rules, file) {
				plan.Rules = append(plan.Rules, file)
			}
		}
		for _, file := range topic.RulesLeft {
			if !contains(plan.RulesLeft, file) {
				plan.RulesLeft = append(plan.RulesLeft, file)
			}
		}
	}
	sort.Strings(plan.Rules)
	sort.Strings(plan.RulesLeft)
}

// planner runs one meeting.
type planner struct {
	root       string
	planID     string
	brief      string
	roster     *Roster
	repos      []Repo
	rules      []rules.Rule
	maxTickets int
	now        func() time.Time
	session    *session
	ask        func(ctx context.Context, questions []string) ([]string, error)
	// answers are what the owner said; every role is told from then on.
	answers []OwnerAnswer
	// data is where the project keeps its game data; nil when it keeps none.
	data *gamedata.Layout
}

func (p *planner) today() string {
	return p.now().Format("2006-01-02")
}

// read returns a project file's content, or "" when it cannot be read.
func (p *planner) read(rel string) string {
	data, err := os.ReadFile(filepath.Join(p.root, filepath.FromSlash(rel)))
	if err != nil {
		return ""
	}
	return string(data)
}

// exists reports whether a document is written: there, and not empty.
func (p *planner) exists(doc string) bool {
	return strings.TrimSpace(p.read(doc)) != ""
}

// maxDocument is how long, in bytes, a document may be for the council to
// write it. A role writes a document whole, from the whole of what is there:
// of a longer one it would be shown a part, and what it is not shown it would
// drop.
const maxDocument = 40000

// headOf returns the beginning of a text, at most max bytes and ending with a
// whole line, and whether anything was left out.
func headOf(text string, max int) (string, bool) {
	if len(text) <= max {
		return text, false
	}
	head := text[:max]
	if newline := strings.LastIndex(head, "\n"); newline >= 0 {
		return head[:newline+1], true
	}
	return strings.ToValidUTF8(head, ""), true
}

// tailOf returns the end of a text, at most max bytes and starting with a
// whole line, and whether anything was left out.
func tailOf(text string, max int) (string, bool) {
	if len(text) <= max {
		return text, false
	}
	tail := text[len(text)-max:]
	if newline := strings.Index(tail, "\n"); newline >= 0 {
		return tail[newline+1:], true
	}
	return strings.ToValidUTF8(tail, ""), true
}

// tooLong says why the council cannot write a document that is there: it is
// longer than a role is shown. Empty when it can.
func (p *planner) tooLong(doc string) string {
	if size := len(p.read(doc)); size > maxDocument {
		return fmt.Sprintf("it is %d bytes long, and the council writes a document whole, up to %d bytes: split it", size, maxDocument)
	}
	return ""
}

// documentsOf lists a role's documents: the files the roster names, written
// or not, and the Markdown files that are in its folders, in that order.
func (p *planner) documentsOf(role *Role) []string {
	docs := append([]string(nil), role.Files()...)
	for _, folder := range role.Folders() {
		var found []string
		_ = filepath.WalkDir(filepath.Join(p.root, filepath.FromSlash(folder)), func(file string, entry os.DirEntry, err error) error {
			switch {
			case err != nil:
				return nil // a folder that is not there holds nothing
			case entry.IsDir() && strings.HasPrefix(entry.Name(), "."):
				return filepath.SkipDir
			case entry.IsDir():
				return nil
			}
			rel, err := filepath.Rel(p.root, file)
			if err != nil {
				return nil
			}
			if doc := filepath.ToSlash(rel); role.OwnsDocument(doc) {
				found = append(found, doc)
			}
			return nil
		})
		sort.Strings(found)
		for _, doc := range found {
			if !contains(docs, doc) {
				docs = append(docs, doc)
			}
		}
	}
	return docs
}

// decisionLog shows the end of the decision log as it was before this plan:
// what was decided already. What this plan decides is shown apart.
func (p *planner) decisionLog() string {
	const shown = 8000
	log := p.read(DecisionsPath)
	if at := strings.Index(log, planMarker(p.planID)); at >= 0 {
		log = log[:at]
		// The heading of this plan's section stands right before its marker.
		if heading := strings.LastIndex(log, "\n## "); heading >= 0 {
			log = log[:heading]
		}
	}
	log = strings.TrimSpace(log)
	if log == "" {
		return ""
	}
	if tail, cut := tailOf(log, shown); cut {
		log = "(earlier entries left out)\n" + tail
	}
	return log + "\n"
}

var slugWord = regexp.MustCompile(`[a-z0-9]+`)

// newID names a new plan after the day and the first words of the brief, and
// makes sure no plan of that name exists.
func newID(root, brief string, now time.Time) string {
	words := slugWord.FindAllString(strings.ToLower(brief), -1)
	if len(words) > 5 {
		words = words[:5]
	}
	slug := strings.Join(words, "-")
	if len(slug) > 40 {
		slug = strings.Trim(slug[:40], "-")
	}
	if slug == "" {
		slug = "plan"
	}
	base := now.Format("20060102") + "-" + slug
	id := base
	for n := 2; ; n++ {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(PlansDir), id)); os.IsNotExist(err) {
			return id
		}
		id = fmt.Sprintf("%s-%d", base, n)
	}
}

var planID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Run holds a meeting, the way a studio holds one: the moderator sets the
// agenda; each topic is taken up in turn, the role it is about proposing, the
// others speaking after it in the order the agenda gives, two roles that
// disagree arguing it out, the moderator saying where it stands, the deciding
// role ruling, and the owners writing what was settled into their documents,
// which the next topic starts from. Then the planning role lays the plan out
// in milestones and tickets the first, and the table objects to the list once.
//
// The plan is returned also when the meeting stopped early: what was reached
// is on disk, and Run with the plan's ID goes on from there.
func Run(ctx context.Context, opts Options) (*Plan, error) {
	if opts.Roster == nil {
		return nil, errors.New("a plan needs a roster")
	}
	if err := opts.Roster.Validate(); err != nil {
		return nil, err
	}
	if opts.Model == nil {
		opts.Model = ClaudeCLI{}
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.MaxTickets <= 0 {
		opts.MaxTickets = 12
	}
	if len(opts.Repos) == 0 {
		return nil, errors.New("a plan needs at least one repo to write tickets for")
	}

	plan := &Plan{ID: opts.ID, Brief: strings.TrimSpace(opts.Brief)}
	if plan.ID == "" {
		if plan.Brief == "" {
			return nil, errors.New("a plan needs a brief: say what is wanted")
		}
		plan.ID = newID(opts.Root, plan.Brief, opts.Now())
	} else {
		if !planID.MatchString(plan.ID) {
			return nil, fmt.Errorf("%q is not the name of a plan", plan.ID)
		}
		kept, err := os.ReadFile(filepath.Join(opts.Root, filepath.FromSlash(plan.Dir()), "brief.md"))
		if err != nil {
			return nil, fmt.Errorf("plan %s cannot be gone on with: %w", plan.ID, err)
		}
		plan.Brief = strings.TrimSpace(string(kept))
	}

	stateDir := filepath.Join(opts.Root, ".ai", "state", "council", plan.ID)
	session, err := newSession(opts.Model, opts.Roster.Limits, stateDir, opts.Progress)
	if err != nil {
		return nil, err
	}
	p := &planner{root: opts.Root, planID: plan.ID, brief: plan.Brief, roster: opts.Roster, repos: opts.Repos, rules: opts.Rules,
		maxTickets: opts.MaxTickets, now: opts.Now, session: session, ask: opts.Ask}
	p.findGameData()

	if err := p.writeFile(plan.Dir()+"/brief.md", plan.Brief+"\n"); err != nil {
		return plan, err
	}

	meetingErr := p.meet(ctx, plan)
	plan.Calls = session.callsMade()

	// Whatever was reached goes on record, also when the meeting stopped early.
	if err := p.writeMinutes(plan, meetingErr); err != nil && meetingErr == nil {
		meetingErr = err
	}
	if err := writeIndex(p.root, p.roster); err != nil && meetingErr == nil {
		meetingErr = err
	}
	return plan, meetingErr
}

// meet runs the meeting: the agenda, the owner's questions, the topics in
// order, the work list.
func (p *planner) meet(ctx context.Context, plan *Plan) error {
	if err := p.setAgenda(ctx, plan); err != nil {
		return fmt.Errorf("the meeting stopped at the agenda: %w", err)
	}
	if err := p.askOwner(ctx, plan); err != nil {
		return fmt.Errorf("the meeting stopped at the owner's questions: %w", err)
	}
	if err := p.checkBudget(plan); err != nil {
		return err
	}
	for i := range plan.Topics {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := p.hold(ctx, plan, i)
		plan.gather()
		if err != nil {
			return fmt.Errorf("the meeting stopped at topic %d (%s): %w", i+1, plan.Topics[i].Topic.ID, err)
		}
		// The record of a topic is there as soon as it is held: a long
		// meeting can be followed in its minutes.
		if err := p.writeMinutes(plan, nil); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := p.worklist(ctx, plan); err != nil {
		return fmt.Errorf("the meeting stopped at the work list: %w", err)
	}
	return nil
}

// maxQuestions is how many questions are put to the owner at once.
const maxQuestions = 7

// askOwner puts the questions of the agenda to the owner of the project, when
// there is somebody to ask. What the owner answers, the table knows from here
// on; what stays open, the deciding role makes an assumption for where a
// ruling rests on it. Answers that were given are kept: going on with a plan
// does not ask again.
func (p *planner) askOwner(ctx context.Context, plan *Plan) error {
	const step = "the owner answers"
	if kept, ok := p.session.remembered(step); ok {
		var answers []OwnerAnswer
		if err := json.Unmarshal([]byte(kept), &answers); err == nil {
			plan.Answers, p.answers = answers, answers
			p.session.say("Questions: what the owner answered is kept from the earlier run")
			return nil
		}
	}
	list := nonEmpty(plan.Agenda.Questions)
	if p.ask == nil || len(list) == 0 {
		return nil
	}
	p.session.say("Questions: %d for the owner of the project", len(list))
	said, err := p.ask(ctx, list)
	if err != nil {
		return err
	}
	answers := make([]OwnerAnswer, len(list))
	answered := 0
	for i, question := range list {
		answers[i].Question = question
		if i < len(said) {
			answers[i].Answer = strings.TrimSpace(said[i])
		}
		if answers[i].Answer != "" {
			answered++
		}
	}
	kept, err := json.Marshal(answers)
	if err != nil {
		return err
	}
	if err := p.session.remember(step, string(kept)); err != nil {
		return err
	}
	plan.Answers, p.answers = answers, answers
	p.session.say("  %d answered, %d left to the council to assume", answered, len(list)-answered)
	return nil
}

// openQuestions lists the questions of the agenda the owner did not answer.
func (p *planner) openQuestions(plan *Plan) []string {
	var open []string
	for _, question := range nonEmpty(plan.Agenda.Questions) {
		answered := false
		for _, answer := range p.answers {
			if answer.Question == question && answer.Answer != "" {
				answered = true
			}
		}
		if !answered {
			open = append(open, question)
		}
	}
	return open
}

// checkBudget says before the topics are taken up when the roster's calls
// cannot carry the agenda: a meeting that runs out half way leaves documents
// that were written from half a meeting.
func (p *planner) checkBudget(plan *Plan) error {
	needed := 0
	for _, record := range plan.Topics {
		if p.session.isRemembered(record.step("decision")) {
			continue // held in an earlier run
		}
		needed += p.minimumCalls(record.Topic)
	}
	if !p.session.isRemembered("work list " + p.roster.Planner().ID) {
		needed += len(p.roster.Roles) // the draft, and every other role's look at it
	}
	if left := p.roster.Limits.Calls - p.session.callsMade(); needed > left {
		return fmt.Errorf("the agenda needs at least %d more model calls and the roster allows %d more: raise limits.calls in the roster, or shorten the agenda in %s/%s, then go on with `awkit plan --resume %s`",
			needed, left, plan.Dir(), AgendaFile, plan.ID)
	}
	return nil
}

// minimumCalls is the fewest model calls a topic is held with: the lead, each
// speaker, the moderator, the deciding role, and every owner who writes, once
// to write and, where the project has rules for its documents, once to have
// them read.
func (p *planner) minimumCalls(topic Topic) int {
	calls := 1 + len(topic.Speakers) + 2
	for i := range p.roster.Roles {
		role := &p.roster.Roles[i]
		var mine []string
		for _, doc := range topic.Writes {
			if role.OwnsDocument(doc) {
				mine = append(mine, doc)
			}
		}
		if len(mine) == 0 {
			continue
		}
		calls++
		if len(p.rulesFor(mine).shown) > 0 {
			calls++
		}
	}
	return calls
}

// nobody is how a role says, where it was asked for an empty string, that
// there is nothing to say.
var nobody = map[string]bool{
	"": true, "-": true, "none": true, "no": true, "no one": true, "nobody": true, "n/a": true, "na": true, "nil": true,
	"無": true, "无": true, "沒有": true, "没有": true, "無人": true, "无人": true, "なし": true, "없음": true,
}

// saysNobody reports whether a dissent only says that nobody dissented.
func saysNobody(dissent string) bool {
	return nobody[strings.ToLower(strings.Trim(strings.TrimSpace(dissent), ".。;；!！ "))]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func contains(items []string, item string) bool {
	for _, candidate := range items {
		if candidate == item {
			return true
		}
	}
	return false
}

// writeFile writes a project file, creating its directory.
func (p *planner) writeFile(rel, content string) error {
	file := filepath.Join(p.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	return os.WriteFile(file, []byte(content), 0o644)
}
