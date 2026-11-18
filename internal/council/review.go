package council

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/layout"
)

// When the owner says something while a meeting goes on, the next role to
// speak is told. What was decided before it was said is not left as it was:
// the deciding role reads every decision and assumption of the plan again,
// whole, against what the owner said and against each other, and says which
// stand, which are replaced, which are taken back, and what the owner
// answered. The owners of every document that rests on what changed read
// that document again in full and write it so that it holds, by the project's
// rules. The topics themselves are not held again.

// Review is what the deciding role made of what the owner said: the plan's
// decisions read again against it, and the documents read again after.
type Review struct {
	// Words are what the owner said that it was read against.
	Words []OwnerWord `json:"words"`
	// At is the point of the meeting where it was made.
	At string `json:"at"`
	// Note says, in a sentence or two, what the owner's word changes.
	Note string `json:"note"`
	// Decisions are the decisions it made: in place of an earlier one
	// (Replaces), or added.
	Decisions []Decision `json:"decisions"`
	// Withdrawn are the decisions it took back.
	Withdrawn []Withdrawal `json:"withdrawn"`
	// Answered are the assumptions the owner's word answers.
	Answered []AnsweredAssumption `json:"answered"`
	// Documents is what became of the documents that were read again, and
	// RuleChecks what reading them against the rules came to.
	Documents  []WrittenDocument `json:"documents"`
	RuleChecks []RuleCheck       `json:"rule_checks"`
}

// Withdrawal is a decision taken back, and why.
type Withdrawal struct {
	ID  string `json:"id"`
	Why string `json:"why"`
}

// AnsweredAssumption is an assumption the owner answered, and the answer.
type AnsweredAssumption struct {
	ID     string `json:"id"`
	Answer string `json:"answer"`
}

// replaced returns the decision that took the place of one, by its ID, or
// the withdrawal that took it back; both empty when it stands.
func (plan *Plan) replaced(id string) (by string, withdrawn *Withdrawal) {
	for _, review := range plan.Revisions {
		for _, decision := range review.Decisions {
			if decision.Replaces == id {
				by = decision.ID
			}
		}
		for i := range review.Withdrawn {
			if review.Withdrawn[i].ID == id {
				withdrawn = &review.Withdrawn[i]
			}
		}
	}
	return by, withdrawn
}

// answered returns what the owner answered to an assumption, by its ID.
func (plan *Plan) answered(id string) (string, bool) {
	for _, review := range plan.Revisions {
		for _, answer := range review.Answered {
			if answer.ID == id {
				return answer.Answer, true
			}
		}
	}
	return "", false
}

// standing returns what the plan holds now of what it decided in its first
// topics (through, from 1) and in every review so far: the decisions that
// were not replaced or taken back, with the ones that replaced them, and the
// assumptions the owner did not answer.
func (plan *Plan) standing(through int) ([]Decision, []Assumption) {
	if through > len(plan.Topics) {
		through = len(plan.Topics)
	}
	var decisions []Decision
	var assumptions []Assumption
	for _, topic := range plan.Topics[:through] {
		decisions = append(decisions, topic.Decisions...)
		assumptions = append(assumptions, topic.Assumptions...)
	}
	for _, review := range plan.Revisions {
		decisions = append(decisions, review.Decisions...)
	}
	var standingDecisions []Decision
	for _, decision := range decisions {
		if by, withdrawn := plan.replaced(decision.ID); by == "" && withdrawn == nil {
			standingDecisions = append(standingDecisions, decision)
		}
	}
	var open []Assumption
	for _, assumption := range assumptions {
		if _, ok := plan.answered(assumption.ID); !ok {
			open = append(open, assumption)
		}
	}
	return standingDecisions, open
}

// writtenSoFar lists the documents this plan wrote, each once.
func (plan *Plan) writtenSoFar() []string {
	var docs []string
	add := func(written []WrittenDocument) {
		for _, doc := range written {
			if doc.Refused == "" && !contains(docs, doc.Path) {
				docs = append(docs, doc.Path)
			}
		}
	}
	for _, topic := range plan.Topics {
		add(topic.Documents)
	}
	for _, review := range plan.Revisions {
		add(review.Documents)
	}
	return docs
}

// topicOfDecision returns the topic that made a decision; nil for one a
// review made.
func (plan *Plan) topicOfDecision(id string) *TopicRecord {
	for i := range plan.Topics {
		for _, decision := range plan.Topics[i].Decisions {
			if decision.ID == id {
				return &plan.Topics[i]
			}
		}
	}
	return nil
}

func wordIDs(words []OwnerWord) string {
	ids := make([]string, len(words))
	for i, word := range words {
		ids[i] = word.ID
	}
	return strings.Join(ids, ", ")
}

// review has the deciding role read the plan's decisions again against what
// the owner said, and the owners read again the documents that rest on what
// changed. Nothing is read again before anything is decided: the owner's word
// then holds from the next step on, like the brief.
func (p *planner) review(ctx context.Context, plan *Plan, words []OwnerWord, at string) error {
	decisions, assumptions := plan.standing(len(plan.Topics))
	if len(decisions)+len(assumptions) == 0 {
		return nil
	}
	decider := p.roster.Decider()
	step := "owner " + wordIDs(words) + " review"
	type replacement struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		Decision  string `json:"decision"`
		Rationale string `json:"rationale"`
	}
	type reread struct {
		Path string `json:"path"`
		Why  string `json:"why"`
	}
	type answer struct {
		Note      string               `json:"note"`
		Replace   []replacement        `json:"replace"`
		Withdraw  []Withdrawal         `json:"withdraw"`
		Add       []Decision           `json:"add"`
		Answered  []AnsweredAssumption `json:"answered"`
		Documents []reread             `json:"documents"`
	}
	decisionIDs := map[string]bool{}
	for _, decision := range decisions {
		decisionIDs[decision.ID] = true
	}
	assumptionIDs := map[string]bool{}
	for _, assumption := range assumptions {
		assumptionIDs[assumption.ID] = true
	}
	p.session.say("  %s reads every decision of the plan again against what the owner said", decider.ID)
	got, err := settleJSON(ctx, p.session, step, decider.Voice(), p.reviewPrompt(decider, plan, words, decisions, assumptions), func(got *answer) error {
		seen := map[string]bool{}
		for _, item := range got.Replace {
			id := strings.TrimSpace(item.ID)
			switch {
			case !decisionIDs[id]:
				return fmt.Errorf("replace names %q, which is not a decision of this plan that stands (they are: %s)", item.ID, strings.Join(sortedKeys(decisionIDs), ", "))
			case seen[id]:
				return fmt.Errorf("%s is replaced or withdrawn twice", id)
			case strings.TrimSpace(item.Decision) == "":
				return fmt.Errorf("the decision that replaces %s does not say what is decided", id)
			}
			seen[id] = true
		}
		for _, item := range got.Withdraw {
			id := strings.TrimSpace(item.ID)
			switch {
			case !decisionIDs[id]:
				return fmt.Errorf("withdraw names %q, which is not a decision of this plan that stands", item.ID)
			case seen[id]:
				return fmt.Errorf("%s is replaced or withdrawn twice", id)
			case strings.TrimSpace(item.Why) == "":
				return fmt.Errorf("%s is withdrawn without saying why", id)
			}
			seen[id] = true
		}
		for _, item := range got.Add {
			if strings.TrimSpace(item.Decision) == "" {
				return errors.New("an added decision does not say what is decided")
			}
		}
		if len(got.Replace)+len(got.Add) > maxDecisions {
			return fmt.Errorf("%d decisions made; at most %d: keep the ones the owner's word calls for", len(got.Replace)+len(got.Add), maxDecisions)
		}
		for _, item := range got.Answered {
			if !assumptionIDs[strings.TrimSpace(item.ID)] {
				return fmt.Errorf("answered names %q, which is not an assumption of this plan that is open", item.ID)
			}
			if strings.TrimSpace(item.Answer) == "" {
				return fmt.Errorf("%s is answered with nothing", item.ID)
			}
		}
		for _, item := range got.Documents {
			doc := strings.TrimSpace(item.Path)
			owner := p.ownerOf(doc)
			switch {
			case owner == nil:
				return fmt.Errorf("documents names %q, which is no role's document", item.Path)
			case !p.exists(doc):
				return fmt.Errorf("documents names %q, which is not written", item.Path)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	review := Review{Words: words, At: at, Note: strings.TrimSpace(got.Note)}
	for _, item := range got.Replace {
		review.Decisions = append(review.Decisions, Decision{Title: item.Title, Decision: item.Decision, Rationale: item.Rationale,
			Replaces: strings.TrimSpace(item.ID)})
	}
	for _, item := range got.Add {
		item.ID, item.Replaces = "", ""
		review.Decisions = append(review.Decisions, item)
	}
	for _, item := range got.Withdraw {
		review.Withdrawn = append(review.Withdrawn, Withdrawal{ID: strings.TrimSpace(item.ID), Why: item.Why})
	}
	for _, item := range got.Answered {
		review.Answered = append(review.Answered, AnsweredAssumption{ID: strings.TrimSpace(item.ID), Answer: item.Answer})
	}
	p.numberReview(plan, &review)

	// What is read again: the documents the deciding role names, and every
	// document a topic whose decision changed ended in.
	var docs []string
	for _, item := range got.Documents {
		if doc := strings.TrimSpace(item.Path); !contains(docs, doc) {
			docs = append(docs, doc)
		}
	}
	changed := map[string]bool{}
	for _, decision := range review.Decisions {
		if decision.Replaces != "" {
			changed[decision.Replaces] = true
		}
	}
	for _, item := range review.Withdrawn {
		changed[item.ID] = true
	}
	for id := range changed {
		if topic := plan.topicOfDecision(id); topic != nil {
			for _, doc := range topic.Topic.Writes {
				if p.exists(doc) && p.ownerOf(doc) != nil && !contains(docs, doc) {
					docs = append(docs, doc)
				}
			}
		}
	}
	sort.Strings(docs)

	plan.Revisions = append(plan.Revisions, review)
	index := len(plan.Revisions) - 1
	if err := p.recordReview(plan, &plan.Revisions[index]); err != nil {
		return err
	}
	p.session.say("  %d replaced, %d withdrawn, %d added, %d assumption(s) answered; %d document(s) to read again",
		len(got.Replace), len(review.Withdrawn), len(got.Add), len(review.Answered), len(docs))

	err = p.reread(ctx, plan, index, docs)
	p.tell(Entry{Step: step, Kind: EntryReview, Role: decider.ID, Data: plan.Revisions[index]})
	return err
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// ownerOf returns the role that owns a document; nil when none does.
func (p *planner) ownerOf(doc string) *Role {
	for i := range p.roster.Roles {
		if p.roster.Roles[i].OwnsDocument(doc) {
			return &p.roster.Roles[i]
		}
	}
	return nil
}

// reread has the owners of documents read them again in full after a
// review, and write them so that they hold; what was written is read against
// the project's rules, and the owner puts right what was found, once.
func (p *planner) reread(ctx context.Context, plan *Plan, index int, docs []string) error {
	review := &plan.Revisions[index]
	var seats []*seat
	for i := range p.roster.Roles {
		role := &p.roster.Roles[i]
		at := &seat{role: role}
		for _, doc := range docs {
			if !role.OwnsDocument(doc) {
				continue
			}
			if reason := p.tooLong(doc); reason != "" {
				at.written = append(at.written, WrittenDocument{Path: doc, Owner: role.ID, Refused: reason})
				continue
			}
			at.asked = append(at.asked, doc)
		}
		if len(at.asked)+len(at.written) == 0 {
			continue
		}
		at.book = p.rulesFor(at.asked)
		seats = append(seats, at)
	}
	prefix := "owner " + wordIDs(review.Words)
	err := p.session.each(len(seats), func(i int) error {
		at := seats[i]
		role := at.role
		if len(at.asked) == 0 {
			return nil
		}
		written, err := p.handIn(ctx, prefix+" documents "+role.ID, role, p.rereadPrompt(role, plan, review, at), at.asked, at.asked,
			func(doc string) error {
				return fmt.Errorf("%s was not answered for: hand it in rewritten, or leave it as it is with <<<UNCHANGED %s>>>", doc, doc)
			})
		if err != nil {
			return err
		}
		at.written = append(at.written, written...)
		read := writtenBy(written)
		if len(at.book.shown) == 0 || len(read) == 0 {
			return nil
		}
		at.check = &RuleCheck{Owner: role.ID}
		found, err := p.checkRules(ctx, prefix+" rules check "+role.ID, role, plan, read, at.book)
		if err != nil {
			return err
		}
		at.check.Found = found
		if len(found) == 0 {
			at.check.Settled = true
			return nil
		}
		revised, err := p.handIn(ctx, prefix+" documents revised "+role.ID, role, p.rereadFixPrompt(role, plan, at.asked, at.book, found), at.asked,
			documentsOf(found), func(doc string) error {
				return fmt.Errorf("%s was not answered for, and a finding names it: hand it in, or leave it as it is with <<<UNCHANGED %s>>>", doc, doc)
			})
		if err != nil {
			return err
		}
		changed := false
		for _, doc := range revised {
			if doc.Refused == "" && !doc.Unchanged {
				changed = true
			}
		}
		if !changed {
			at.check.Open, at.check.Settled = found, true
			return nil
		}
		open, err := p.recheckRules(ctx, prefix+" rules recheck "+role.ID, role, plan, read, at.book, found)
		if err != nil {
			return err
		}
		at.check.Open, at.check.Settled = open, true
		return nil
	})
	review.Documents, review.RuleChecks = nil, nil
	for _, at := range seats {
		review.Documents = append(review.Documents, at.written...)
		if at.check != nil {
			review.RuleChecks = append(review.RuleChecks, *at.check)
		}
	}
	return err
}

// numberReview names the decisions a review made: each counts on from the
// last one on record before the review, as a topic's do.
func (p *planner) numberReview(plan *Plan, review *Review) {
	before := p.read(DecisionsPath)
	if at := strings.Index(before, reviewMarker(plan.ID, review.Words)); at >= 0 {
		before = before[:at]
	}
	last := lastNumber(decisionName, before)
	for i := range review.Decisions {
		review.Decisions[i].ID = fmt.Sprintf("D%d", last+i+1)
	}
}

func reviewMarker(plan string, words []OwnerWord) string {
	return "<!-- plan: " + plan + " owner: " + wordIDs(words) + " -->"
}

// recordReview puts a review in the decision log, after what is there. The
// log only grows: the decisions a review replaces or takes back stay as they
// were written, and the review names them.
func (p *planner) recordReview(plan *Plan, review *Review) error {
	log := p.read(DecisionsPath)
	marker := reviewMarker(plan.ID, review.Words)
	if strings.Contains(log, marker) {
		return nil
	}
	var b strings.Builder
	b.WriteString(strings.TrimRight(log, "\n") + "\n")
	fmt.Fprintf(&b, "\n### What the owner said (%s)\n\n%s\n\n", wordIDs(review.Words), marker)
	for _, word := range review.Words {
		if word.Question != "" {
			fmt.Fprintf(&b, "> **%s**, to \"%s\": %s\n>\n", word.ID, oneLine(word.Question), oneLine(word.Text))
		} else {
			fmt.Fprintf(&b, "> **%s**: %s\n>\n", word.ID, oneLine(word.Text))
		}
	}
	b.WriteString(fmt.Sprintf("\n%s read every decision of the plan again against it.", p.roster.Decider().ID))
	if review.Note != "" {
		b.WriteString(" " + oneLine(review.Note))
	}
	b.WriteString("\n\n")
	for _, decision := range review.Decisions {
		replaces := ""
		if decision.Replaces != "" {
			replaces = " (replaces " + decision.Replaces + ")"
		}
		fmt.Fprintf(&b, "- **%s %s**%s: %s\n  - Why: %s\n", decision.ID, oneLine(firstNonEmpty(decision.Title, "decision")), replaces, oneLine(decision.Decision), oneLine(decision.Rationale))
	}
	for _, item := range review.Withdrawn {
		fmt.Fprintf(&b, "- **%s** withdrawn: %s\n", item.ID, oneLine(item.Why))
	}
	for _, item := range review.Answered {
		fmt.Fprintf(&b, "- **%s** answered by the owner: %s\n", item.ID, oneLine(item.Answer))
	}
	if len(review.Decisions)+len(review.Withdrawn)+len(review.Answered) == 0 {
		b.WriteString("- Every decision stands as it was.\n")
	}
	return p.writeFile(DecisionsPath, b.String())
}

// wordsText shows what the owner said.
func wordsText(words []OwnerWord) string {
	var b strings.Builder
	for _, word := range words {
		if word.Question != "" {
			fmt.Fprintf(&b, "- %s, answering \"%s\": %s\n", word.ID, oneLine(word.Question), oneLine(word.Text))
		} else {
			fmt.Fprintf(&b, "- %s: %s\n", word.ID, oneLine(word.Text))
		}
	}
	return b.String()
}

// reviewPrompt asks the deciding role to read every decision and assumption
// of the plan again against what the owner said.
func (p *planner) reviewPrompt(role *Role, plan *Plan, words []OwnerWord, decisions []Decision, assumptions []Assumption) string {
	var b strings.Builder
	b.WriteString(p.frame(role))
	b.WriteString("\nWhile the meeting went on, the owner of the project said:\n" + wordsText(words))
	b.WriteString("It holds like the brief: no role argues against it.\n")
	b.WriteString("\nWhat this meeting has decided so far, all of it:\n" + decisionsText(decisions, assumptions))
	var held []string
	for _, topic := range plan.Topics {
		if topic.Held {
			held = append(held, fmt.Sprintf("- %d. %s: ended in %s", topic.Number, oneLine(topic.Topic.Title), strings.Join(topic.Topic.Writes, ", ")))
		}
	}
	if len(held) > 0 {
		b.WriteString("\nThe topics held so far, and the documents each ended in:\n" + strings.Join(held, "\n") + "\n")
	}
	if docs := plan.writtenSoFar(); len(docs) > 0 {
		b.WriteString("\nThe documents this meeting wrote: " + strings.Join(docs, ", ") + "\n")
	}
	b.WriteString(`
Your task now: you decide for the project. Read every decision and every assumption above again, one by one, against what the owner said, and against each other: a decision that rests on one that changes changes with it. The topics are not held again; you rule here.
- A decision that still holds, you leave out of your answer: it stands.
- A decision the owner's word changes, or that rests on one that changes, you replace: say what it becomes, and why.
- A decision the owner's word makes pointless, you withdraw: say why.
- Where the owner's word calls for a decision none of the above makes, add it.
- An assumption the owner's word answers: say so, with the answer.
Then name every document that has to be read again in full: the ones that rest on a decision you replace or withdraw, the ones that write down an assumption that is answered, and the ones the owner's word touches directly. Its owner will read it all again, not only where a decision is named.
If nothing changes, say so in the note and leave every list empty.
Answer with one JSON object and nothing else:
{"note": "what the owner's word changes, in a sentence or two",
 "replace": [{"id": "D3", "title": "...", "decision": "...", "rationale": "..."}],
 "withdraw": [{"id": "D5", "why": "..."}],
 "add": [{"title": "...", "decision": "...", "rationale": "...", "dissent": ""}],
 "answered": [{"id": "A2", "answer": "..."}],
 "documents": [{"path": "docs/...", "why": "..."}]}
`)
	b.WriteString(p.languageLine() + "\n")
	return b.String()
}

// reviewText shows what a review changed.
func reviewText(review *Review) string {
	var b strings.Builder
	if review.Note != "" {
		b.WriteString(oneLine(review.Note) + "\n")
	}
	for _, decision := range review.Decisions {
		if decision.Replaces != "" {
			fmt.Fprintf(&b, "- %s replaces %s: %s\n", decision.ID, decision.Replaces, oneLine(decision.Decision))
		} else {
			fmt.Fprintf(&b, "- %s is added: %s\n", decision.ID, oneLine(decision.Decision))
		}
	}
	for _, item := range review.Withdrawn {
		fmt.Fprintf(&b, "- %s is withdrawn: %s\n", item.ID, oneLine(item.Why))
	}
	for _, item := range review.Answered {
		fmt.Fprintf(&b, "- %s is answered: %s\n", item.ID, oneLine(item.Answer))
	}
	if b.Len() == 0 {
		b.WriteString("Every decision stands as it was.\n")
	}
	return b.String()
}

// rereadPrompt asks a role to read its documents again in full after a
// review, and write them so that they hold.
func (p *planner) rereadPrompt(role *Role, plan *Plan, review *Review, at *seat) string {
	var b strings.Builder
	b.WriteString(p.frame(role))
	b.WriteString("\nWhile the meeting went on, the owner of the project said:\n" + wordsText(review.Words))
	b.WriteString("\n" + p.roster.Decider().ID + " read every decision of the plan again against it:\n" + reviewText(review))
	decisions, assumptions := plan.standing(len(plan.Topics))
	b.WriteString("\nWhat this meeting holds now, all of it:\n" + decisionsText(decisions, assumptions))
	b.WriteString("\nYour documents to read again, as they stand:\n" + p.asTheyStand(at.asked))
	b.WriteString(`
Your task now: read each of these documents again, all of it, not only where a changed decision is named. What follows from a decision that changed, what rests on an assumption that is answered, and what the owner's word touches has to change with it; what still holds stays as it is, word for word. A document is the current truth about its subject, not the minutes of a meeting: it names the decision (D4) where a reader will ask why, and does not tell the story of the change.
`)
	if slicesHaveGameData(at.asked, p) {
		b.WriteString(gameDataForm)
	}
	b.WriteString(p.byTheRules(role, plan, at.book))
	fmt.Fprintf(&b, `
When the whole of a document still holds, answer for it with this single line:
<<<UNCHANGED path/of/the/document.md>>>
Otherwise hand it in, whole, at most %d bytes, between these two lines:
<<<DOCUMENT path/of/the/document.md>>>
(the whole document)
<<<END>>>
Answer for every document above, and with nothing else.
`, maxDocument)
	return b.String()
}

func slicesHaveGameData(docs []string, p *planner) bool {
	for _, doc := range docs {
		if p.isGameData(doc) {
			return true
		}
	}
	return false
}

// rereadFixPrompt asks a role to put right what reading its documents
// against the rules found, after it wrote them again.
func (p *planner) rereadFixPrompt(role *Role, plan *Plan, asked []string, book rulebook, findings []RuleFinding) string {
	var b strings.Builder
	b.WriteString(p.frame(role))
	b.WriteString("\nYour documents as they stand:\n" + p.asTheyStand(asked))
	decisions, assumptions := plan.standing(len(plan.Topics))
	b.WriteString("\nWhat this meeting holds now:\n" + decisionsText(decisions, assumptions))
	b.WriteString("\nThe project's rules that hold for the files you write (" + layout.RulesDir + "/):\n\n" + book.text() + p.facts(role, plan))
	b.WriteString("\nYour documents were read against these rules. What the check found:\n" + findingsText(findings))
	b.WriteString(`
Your task now: put right what the check found. A finding that reads the rule the way it is written, you take in. Where it wants something moved to a document you do not own, take it out of yours and name that document in its place: its owner writes it there. A finding that misreads the rule, you leave: change nothing for it. Change nothing else in a document: what the meeting holds stands, and so does every word the findings are not about.

Hand in each document you change, whole, between these two lines:
<<<DOCUMENT path/of/the/document.md>>>
(the whole document)
<<<END>>>
For a document a finding names and you leave as it is, answer with this single line instead:
<<<UNCHANGED path/of/the/document.md>>>
Answer with nothing else.
`)
	return b.String()
}
