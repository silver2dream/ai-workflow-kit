package council

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
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
	// ID names a plan to go on with. Empty starts a new one.
	ID string
	// MaxTickets bounds the work list (12 when zero).
	MaxTickets int
	// Now is the clock (time.Now when nil).
	Now func() time.Time
	// Progress is told what the meeting is doing, a line at a time, and never
	// by two goroutines at once.
	Progress func(string)
	// Ask puts the table's questions to the owner of the project and returns
	// an answer for each, in their order; an empty answer leaves the question
	// open. Without it nobody is asked: the meeting never waits for a person,
	// and the deciding role says what the plan assumes instead.
	Ask func(ctx context.Context, questions []string) ([]string, error)
}

// Position is where a role stands on the brief.
type Position struct {
	Role      string   `json:"-"`
	Summary   string   `json:"summary"`
	Proposals []string `json:"proposals"`
	Concerns  []string `json:"concerns"`
	Questions []string `json:"questions"`
}

// Objection is what one role cannot accept in another's position.
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
// table. An empty answer means the owner left it open.
type OwnerAnswer struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// Summary is where the meeting stands, by the moderator.
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

	Positions []Position
	// Answers are what the owner of the project said when the table's
	// questions were put to them; empty when nobody was asked.
	Answers     []OwnerAnswer
	Objections  []Objection
	Disputes    []Dispute
	Summary     Summary
	Decisions   []Decision
	Assumptions []Assumption
	Documents   []WrittenDocument

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

// position returns where a role stands, or nil before it has said.
func (plan *Plan) position(role string) *Position {
	for i := range plan.Positions {
		if plan.Positions[i].Role == role {
			return &plan.Positions[i]
		}
	}
	return nil
}

// questions collects what the roles asked the owner, each with who asked.
func (plan *Plan) questions() []string {
	var out []string
	for _, position := range plan.Positions {
		for _, question := range position.Questions {
			if strings.TrimSpace(question) != "" {
				out = append(out, position.Role+": "+oneLine(question))
			}
		}
	}
	return out
}

// undisputed lists the objections that did not become a dispute.
func (plan *Plan) undisputed() []string {
	var out []string
	for _, objection := range plan.Objections {
		argued := false
		for _, dispute := range plan.Disputes {
			if dispute.Challenger == objection.From && dispute.Defender == objection.To {
				argued = true
			}
		}
		if !argued {
			out = append(out, fmt.Sprintf("%s to %s: %s", objection.From, objection.To, oneLine(objection.Point)))
		}
	}
	return out
}

// planner runs one meeting.
type planner struct {
	root       string
	brief      string
	roster     *Roster
	repos      []Repo
	maxTickets int
	now        func() time.Time
	session    *session
	ask        func(ctx context.Context, questions []string) ([]string, error)
	// answers are what the owner said; every role is told from then on.
	answers []OwnerAnswer
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

// ownDocuments shows a role the documents it owns as they stand, whole.
func (p *planner) ownDocuments(role *Role) string {
	var b strings.Builder
	for _, doc := range role.Owns {
		content := p.read(doc)
		switch {
		case strings.TrimSpace(content) == "":
			fmt.Fprintf(&b, "- %s: not written yet\n", doc)
		case len(content) > maxDocument:
			head, _ := headOf(content, maxDocument/5)
			fmt.Fprintf(&b, "===== %s (only its beginning: it is too long to be shown whole)\n%s\n", doc, head)
		default:
			fmt.Fprintf(&b, "===== %s\n%s\n", doc, strings.TrimRight(content, "\n"))
		}
	}
	return b.String()
}

// decisionLog shows the end of the decision log: what is already decided.
func (p *planner) decisionLog() string {
	const shown = 8000
	log := strings.TrimSpace(p.read(DecisionsPath))
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

// Run holds a meeting: every role states its position, the roles object to
// each other, the objections that matter most are argued out, the moderator
// says where things stand, the deciding role rules, every role writes the
// documents it owns, and the planning role draws up the work list, which the
// table gets to object to once.
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
	p := &planner{root: opts.Root, brief: plan.Brief, roster: opts.Roster, repos: opts.Repos,
		maxTickets: opts.MaxTickets, now: opts.Now, session: session, ask: opts.Ask}

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

// meet runs the steps of the meeting in order.
func (p *planner) meet(ctx context.Context, plan *Plan) error {
	steps := []struct {
		name string
		run  func(context.Context, *Plan) error
	}{
		{"positions", p.positions},
		{"questions", p.questions},
		{"objections", p.objections},
		{"disputes", p.disputes},
		{"summary", p.summary},
		{"decisions", p.decide},
		{"documents", p.documents},
		{"work list", p.worklist},
	}
	for _, step := range steps {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := step.run(ctx, plan); err != nil {
			return fmt.Errorf("the meeting stopped at the %s: %w", step.name, err)
		}
	}
	return nil
}

// positions asks every role where it stands.
func (p *planner) positions(ctx context.Context, plan *Plan) error {
	p.session.say("Positions: %d roles state where they stand", len(p.roster.Roles))
	positions := make([]Position, len(p.roster.Roles))
	err := p.session.each(len(p.roster.Roles), func(i int) error {
		role := &p.roster.Roles[i]
		position, err := settleJSON(ctx, p.session, "position "+role.ID, role.Model, p.positionPrompt(role), func(got *Position) error {
			if strings.TrimSpace(got.Summary) == "" {
				return errors.New("the position has no summary")
			}
			return nil
		})
		if err != nil {
			return err
		}
		position.Role = role.ID
		positions[i] = *position
		return nil
	})
	if err != nil {
		return err
	}
	plan.Positions = positions
	return nil
}

// maxQuestions is how many questions are put to the owner at once.
const maxQuestions = 7

// questions puts what the roles want to know from the owner of the project to
// the owner, when there is somebody to ask. What the owner answers, the table
// knows from here on; what stays open, the deciding role makes an assumption
// for. Answers that were given are kept: going on with a plan does not ask
// again.
func (p *planner) questions(ctx context.Context, plan *Plan) error {
	const step = "the owner answers"
	if kept, ok := p.session.remembered(step); ok {
		var answers []OwnerAnswer
		if err := json.Unmarshal([]byte(kept), &answers); err == nil {
			plan.Answers, p.answers = answers, answers
			p.session.say("Questions: what the owner answered is kept from the earlier run")
			return nil
		}
	}
	asked := plan.questions()
	if p.ask == nil || len(asked) == 0 {
		return nil
	}

	// Several roles ask the same thing in their own words: the moderator
	// makes one list of it. A few questions are put as they were asked.
	var list []string
	if len(asked) <= 3 {
		for _, position := range plan.Positions {
			list = append(list, nonEmpty(position.Questions)...)
		}
	} else {
		type answer struct {
			Questions []string `json:"questions"`
		}
		got, err := settleJSON(ctx, p.session, "moderator puts the questions", p.roster.Model, p.questionsPrompt(asked), func(got *answer) error {
			if len(nonEmpty(got.Questions)) == 0 {
				return errors.New("the list has no question, and the roles asked some")
			}
			return nil
		})
		if err != nil {
			return err
		}
		list = nonEmpty(got.Questions)
	}
	if len(list) > maxQuestions {
		list = list[:maxQuestions]
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

// objections asks every role what it cannot accept in the others' positions.
func (p *planner) objections(ctx context.Context, plan *Plan) error {
	p.session.say("Objections: every role reads the others' positions")
	type answer struct {
		Objections []Objection `json:"objections"`
	}
	perRole := make([][]Objection, len(p.roster.Roles))
	err := p.session.each(len(p.roster.Roles), func(i int) error {
		role := &p.roster.Roles[i]
		got, err := settleJSON(ctx, p.session, "objections "+role.ID, role.Model, p.objectionPrompt(role, plan), func(got *answer) error {
			for _, objection := range got.Objections {
				if objection.To == role.ID {
					return errors.New("an objection is addressed to yourself")
				}
				if p.roster.Role(objection.To) == nil {
					return fmt.Errorf("an objection is addressed to %q, who is not at the table (the roles are: %s)", objection.To, strings.Join(p.roster.IDs(), ", "))
				}
				if strings.TrimSpace(objection.Point) == "" {
					return errors.New("an objection has no point")
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		for _, objection := range got.Objections {
			objection.From = role.ID
			perRole[i] = append(perRole[i], objection)
		}
		return nil
	})
	if err != nil {
		return err
	}
	plan.Objections = nil
	for _, objections := range perRole {
		plan.Objections = append(plan.Objections, objections...)
	}
	p.session.say("  %d objections raised", len(plan.Objections))
	return nil
}

// disputes picks the objections to argue out and has each argued.
func (p *planner) disputes(ctx context.Context, plan *Plan) error {
	plan.Disputes = nil
	limit := p.roster.Limits.Disputes
	switch {
	case len(plan.Objections) == 0:
		p.session.say("Disputes: nobody objected")
		return nil
	case len(plan.Objections) <= limit:
		// Few enough to argue every one: nothing for the moderator to pick.
		for _, objection := range plan.Objections {
			plan.Disputes = append(plan.Disputes, Dispute{Topic: topicOf(objection.Point), Challenger: objection.From,
				Defender: objection.To, Point: objection.Point, Wants: objection.Wants})
		}
	default:
		type answer struct {
			Disputes []Dispute `json:"disputes"`
		}
		got, err := settleJSON(ctx, p.session, "moderator picks the disputes", p.roster.Model, p.disputesPrompt(plan), func(got *answer) error {
			if len(got.Disputes) == 0 {
				return errors.New("no dispute was picked, and there are objections")
			}
			for _, dispute := range got.Disputes {
				if p.roster.Role(dispute.Challenger) == nil || p.roster.Role(dispute.Defender) == nil || dispute.Challenger == dispute.Defender {
					return fmt.Errorf("the dispute %q is not between two roles at the table (the roles are: %s)", dispute.Topic, strings.Join(p.roster.IDs(), ", "))
				}
				if strings.TrimSpace(dispute.Point) == "" {
					return fmt.Errorf("the dispute %q has no point", dispute.Topic)
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		plan.Disputes = got.Disputes
		if len(plan.Disputes) > limit {
			plan.Disputes = plan.Disputes[:limit]
		}
	}

	p.session.say("Disputes: %d points are argued out", len(plan.Disputes))
	// Each dispute is argued on its own: what a role concedes in one is not
	// known to it in another, and need not be.
	return p.session.each(len(plan.Disputes), func(i int) error {
		return p.argue(ctx, plan, i, &plan.Disputes[i])
	})
}

// topicOf names a dispute after the first words of its point.
func topicOf(point string) string {
	words := strings.Fields(point)
	if len(words) > 8 {
		return strings.Join(words[:8], " ") + "..."
	}
	return strings.Join(words, " ")
}

// argue lets the two roles of a dispute go back and forth: the defender
// answers, the challenger says whether that settles it. It ends when the
// defender concedes, the challenger accepts, or the rounds are used up.
func (p *planner) argue(ctx context.Context, plan *Plan, index int, dispute *Dispute) error {
	defender, challenger := p.roster.Role(dispute.Defender), p.roster.Role(dispute.Challenger)
	type defence struct {
		Stance   string `json:"stance"`
		Reply    string `json:"reply"`
		Position string `json:"position"`
	}
	type reply struct {
		Accepts bool   `json:"accepts"`
		Reply   string `json:"reply"`
		Wants   string `json:"wants"`
	}

	for round := 1; round <= p.roster.Limits.DisputeRounds; round++ {
		step := fmt.Sprintf("dispute %d round %d", index+1, round)
		answer, err := settleJSON(ctx, p.session, step+" "+defender.ID+" answers", defender.Model, p.defencePrompt(defender, plan, dispute), func(got *defence) error {
			got.Stance = strings.ToLower(strings.TrimSpace(got.Stance))
			switch got.Stance {
			case "concede", "revise", "hold":
			default:
				return fmt.Errorf("stance is %q; it is one of concede, revise, hold", got.Stance)
			}
			if strings.TrimSpace(got.Reply) == "" {
				return errors.New("the answer has no reply")
			}
			return nil
		})
		if err != nil {
			return err
		}
		exchange := Exchange{Stance: answer.Stance, Reply: answer.Reply, Position: answer.Position}
		if exchange.Stance == "concede" {
			dispute.Exchanges = append(dispute.Exchanges, exchange)
			dispute.Settled = true
			dispute.Outcome = fmt.Sprintf("%s conceded in round %d", defender.ID, round)
			break
		}

		dispute.Exchanges = append(dispute.Exchanges, exchange)
		verdict, err := settleJSON(ctx, p.session, step+" "+challenger.ID+" replies", challenger.Model, p.replyPrompt(challenger, plan, dispute), func(got *reply) error {
			if strings.TrimSpace(got.Reply) == "" {
				return errors.New("the answer has no reply")
			}
			return nil
		})
		if err != nil {
			return err
		}
		last := &dispute.Exchanges[len(dispute.Exchanges)-1]
		last.Answered, last.Accepts, last.Answer, last.StillWants = true, verdict.Accepts, verdict.Reply, verdict.Wants
		if verdict.Accepts {
			dispute.Settled = true
			dispute.Outcome = fmt.Sprintf("%s accepted %s's answer in round %d", challenger.ID, defender.ID, round)
			break
		}
	}
	if !dispute.Settled {
		dispute.Outcome = fmt.Sprintf("not settled after %d round(s); it goes to %s", len(dispute.Exchanges), p.roster.Decider().ID)
	}
	state := "not settled"
	if dispute.Settled {
		state = "settled"
	}
	p.session.say("  %s against %s: %s", dispute.Challenger, dispute.Defender, state)
	return nil
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

// summary asks the moderator where the meeting stands.
func (p *planner) summary(ctx context.Context, plan *Plan) error {
	p.session.say("Summary: the moderator says where the meeting stands")
	got, err := settleJSON[Summary](ctx, p.session, "moderator sums up", p.roster.Model, p.summaryPrompt(plan), nil)
	if err != nil {
		return err
	}
	plan.Summary = *got
	return nil
}

// decide asks the deciding role to rule, and puts the rulings on record.
func (p *planner) decide(ctx context.Context, plan *Plan) error {
	decider := p.roster.Decider()
	p.session.say("Decisions: %s rules", decider.ID)
	type answer struct {
		Decisions   []Decision   `json:"decisions"`
		Assumptions []Assumption `json:"assumptions"`
	}
	got, err := settleJSON(ctx, p.session, "decisions "+decider.ID, decider.Model, p.decisionPrompt(decider, plan), func(got *answer) error {
		if len(got.Decisions) == 0 {
			return errors.New("nothing was decided; a plan rests on at least one decision")
		}
		for _, decision := range got.Decisions {
			if strings.TrimSpace(decision.Decision) == "" {
				return errors.New("a decision does not say what is decided")
			}
		}
		for _, assumption := range got.Assumptions {
			if strings.TrimSpace(assumption.Assumption) == "" {
				return errors.New("an assumption does not say what is assumed")
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	plan.Decisions, plan.Assumptions = got.Decisions, got.Assumptions
	for i := range plan.Decisions {
		if saysNobody(plan.Decisions[i].Dissent) {
			plan.Decisions[i].Dissent = ""
		}
	}
	p.number(plan)
	p.session.say("  %d decided, %d assumed until the owner answers", len(plan.Decisions), len(plan.Assumptions))
	return p.recordDecisions(plan)
}

// documents asks every role to write the documents it owns. Only this
// function writes them, and only to the paths the role owns.
func (p *planner) documents(ctx context.Context, plan *Plan) error {
	// What each role is asked for: the documents it owns, without the ones
	// that are too long for the council to write.
	type seat struct {
		role  *Role
		asked []string
		left  []WrittenDocument
	}
	var seats []seat
	for i := range p.roster.Roles {
		role := &p.roster.Roles[i]
		if len(role.Owns) == 0 {
			continue
		}
		at := seat{role: role}
		for _, doc := range role.Owns {
			if reason := p.tooLong(doc); reason != "" {
				at.left = append(at.left, WrittenDocument{Path: doc, Owner: role.ID, Refused: reason})
				continue
			}
			at.asked = append(at.asked, doc)
		}
		seats = append(seats, at)
	}

	p.session.say("Documents: %d roles write what they own", len(seats))
	perRole := make([][]WrittenDocument, len(seats))
	err := p.session.each(len(seats), func(i int) error {
		role, asked := seats[i].role, seats[i].asked
		perRole[i] = seats[i].left
		if len(asked) == 0 {
			return nil
		}
		var written []WrittenDocument
		err := p.session.settle(ctx, "documents "+role.ID, role.Model, p.documentsPrompt(role, plan, asked), func(answer string, replay bool) error {
			docs, err := parseDocuments(answer)
			if err != nil {
				return err
			}
			var accepted []document
			written = nil
			handedIn := map[string]bool{}
			for _, doc := range docs {
				switch {
				case !role.OwnsDocument(doc.Path):
					// Not an error of form: the document is left out and the
					// minutes say so. A role does not get to write what
					// another role owns, however it asks.
					written = append(written, WrittenDocument{Path: doc.Path, Owner: role.ID,
						Refused: fmt.Sprintf("%s does not own this document", role.ID)})
					continue
				case !contains(asked, doc.Path):
					continue // too long to be written: it stays, and is on record as left
				case handedIn[doc.Path]:
					return fmt.Errorf("%s was handed in twice", doc.Path)
				}
				handedIn[doc.Path] = true
				switch {
				case doc.Unchanged && strings.TrimSpace(p.read(doc.Path)) == "":
					return fmt.Errorf("%s is not written yet, so it cannot stay unchanged: write it", doc.Path)
				case doc.Unchanged:
					written = append(written, WrittenDocument{Path: doc.Path, Owner: role.ID, Unchanged: true})
				case strings.TrimSpace(doc.Content) == "":
					return fmt.Errorf("%s is empty", doc.Path)
				case len(doc.Content) > maxDocument:
					return fmt.Errorf("%s is %d bytes long; a document stays under %d bytes, so that it can be rewritten whole: shorten it", doc.Path, len(doc.Content), maxDocument)
				default:
					accepted = append(accepted, doc)
					written = append(written, WrittenDocument{Path: doc.Path, Owner: role.ID})
				}
			}
			for _, owned := range asked {
				if !handedIn[owned] {
					return fmt.Errorf("%s was not handed in; you own it", owned)
				}
			}
			// An answer from an earlier run was written then. What is on disk
			// now may have been changed by hand since, and stays.
			if replay {
				return nil
			}
			for _, doc := range accepted {
				if err := p.writeFile(doc.Path, doc.Content); err != nil {
					return broken{err}
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		perRole[i] = append(perRole[i], written...)
		return nil
	})
	if err != nil {
		return err
	}
	plan.Documents = nil
	for _, written := range perRole {
		plan.Documents = append(plan.Documents, written...)
	}
	return nil
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
