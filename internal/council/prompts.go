package council

import (
	"fmt"
	"strings"
)

// The prompts of a plan. Each call is one question to one role; everything
// the role needs to answer it is in the prompt, because a role remembers
// nothing between calls. What a role has to have read is in the prompt too:
// the documents are the meeting's memory.

const jsonOnly = "Answer with one JSON object in a ```json block, and nothing else."

// maxReading is how much of the documents, in bytes, one question carries.
// Documents that do not fit are named and not shown.
const maxReading = 120000

// frame opens every prompt to a role: who it is, what it stands for, who else
// is at the table, the project, its documents, what is decided already, and
// the brief.
func (p *planner) frame(role *Role) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are the %s (%s) at a planning meeting for a software project. The meeting turns a brief into documents, decisions and a list of tickets, one topic at a time.\n\n", role.Title, role.ID)
	b.WriteString("What you stand for:\n")
	b.WriteString(indent(strings.TrimSpace(role.Stance)))
	b.WriteString("\n\nYou hold this for the whole meeting. Agreement you do not mean helps nobody: when something is weak from where you stand, say so and say why. When it is sound, say so in a line and move on.\n\n")
	b.WriteString(p.languageLine())
	b.WriteString("\n\nThe table:\n")
	for _, other := range p.roster.Roles {
		note := ""
		switch {
		case other.ID == role.ID:
			note = " - you"
		case other.Decides:
			note = " - decides what stays disputed"
		}
		fmt.Fprintf(&b, "- %s (%s)%s\n", other.ID, other.Title, note)
	}
	b.WriteString("\nThe project's repos:\n")
	for _, repo := range p.repos {
		fmt.Fprintf(&b, "- %s%s\n", repo.Name, repoNote(repo))
	}
	b.WriteString(p.documentMap(role))
	if log := p.decisionLog(); log != "" {
		b.WriteString("\nDecisions on record from earlier plans (" + DecisionsPath + "):\n")
		b.WriteString(log)
	}
	b.WriteString("\nThe brief:\n<brief>\n")
	b.WriteString(strings.TrimSpace(p.brief))
	b.WriteString("\n</brief>\n")
	b.WriteString(p.ownerSaid())
	return b.String()
}

// documentMap tells a role where the project's documents are and whose each
// is. A role that does not know proposes a document nobody writes, or one in
// a place the project's rules keep it out of, and a decision follows it there.
func (p *planner) documentMap(role *Role) string {
	var b strings.Builder
	for i := range p.roster.Roles {
		other := &p.roster.Roles[i]
		whose := other.ID
		if other.ID == role.ID {
			whose = "you"
		}
		for _, folder := range other.Folders() {
			fmt.Fprintf(&b, "- %s (%s: any document in this folder)\n", folder, whose)
		}
		for _, doc := range p.documentsOf(other) {
			state := ""
			if !p.exists(doc) {
				state = ", not written yet"
			}
			fmt.Fprintf(&b, "- %s (%s%s)\n", doc, whose, state)
		}
	}
	if b.Len() == 0 {
		return ""
	}
	yours := "You own none of them: you write no document."
	if len(role.Owns) > 0 {
		yours = "You write the ones that are yours, and no other."
	}
	return "\nThe project's documents. Each has one owner, the role that writes it, and is kept where the roster names it; nothing said or decided at this meeting moves one, merges two or gives one to another role. A folder inside another role's folder is not that role's:\n" + b.String() + yours + "\n" + p.gameDataNote()
}

// ownerSaid tells what the owner of the project answered when the questions
// of the agenda were put to them. It stands with the brief: it is the
// owner's word, and no role argues against it.
func (p *planner) ownerSaid() string {
	var b strings.Builder
	for _, answer := range p.answers {
		if answer.Answer != "" {
			fmt.Fprintf(&b, "- Asked: %s\n  The owner: %s\n", oneLine(answer.Question), oneLine(answer.Answer))
		}
	}
	if b.Len() == 0 {
		return ""
	}
	return "\nThe owner of the project was asked before the meeting, and answered. This holds like the brief:\n" + b.String()
}

func (p *planner) languageLine() string {
	if p.roster.Language != "" {
		return "Write everything in " + p.roster.Language + ". JSON keys, role ids, repo names and file paths stay as they are."
	}
	return "Write in the language the brief is written in. JSON keys, role ids, repo names and file paths stay as they are."
}

func repoNote(repo Repo) string {
	var parts []string
	if repo.Language != "" {
		parts = append(parts, repo.Language)
	}
	if repo.Path != "" && repo.Path != "./" {
		parts = append(parts, "in "+repo.Path)
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

func indent(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = "  " + line
	}
	return strings.Join(lines, "\n")
}

func bullets(items []string) string {
	if len(items) == 0 {
		return "  (none)\n"
	}
	var b strings.Builder
	for _, item := range items {
		fmt.Fprintf(&b, "  - %s\n", strings.TrimSpace(item))
	}
	return b.String()
}

func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

const moderatorFrame = "You moderate a planning meeting for a software project. You have no opinion on the subject, you never take a side and you never answer for a role. "

// topicHead says which topic the meeting is at, what it settles and who
// speaks to it.
func topicHead(plan *Plan, r *TopicRecord) string {
	speakers := strings.Join(r.Topic.Speakers, ", ")
	return fmt.Sprintf("\nThe topic now, %d of %d: %s\nIt settles: %s\n%s leads; speaking after, in this order: %s.\nIt ends in these documents: %s\n",
		r.Number, len(plan.Topics), oneLine(r.Topic.Title), oneLine(r.Topic.Question), r.Topic.Lead, speakers, strings.Join(r.Topic.Writes, ", "))
}

// earlierTopics names the topics of this plan that were taken up before this
// one: their documents are the ones the meeting goes on from.
func earlierTopics(plan *Plan, r *TopicRecord) string {
	var b strings.Builder
	for _, other := range plan.Topics[:r.Number-1] {
		fmt.Fprintf(&b, "- %d. %s: ended in %s\n", other.Number, oneLine(other.Topic.Title), strings.Join(other.Topic.Writes, ", "))
	}
	if b.Len() == 0 {
		return ""
	}
	return "\nTopics taken up before this one:\n" + b.String()
}

// decidedSoFar shows what this plan has decided before a topic.
func decidedSoFar(plan *Plan, r *TopicRecord) string {
	var decisions []Decision
	var assumptions []Assumption
	for _, other := range plan.Topics[:r.Number-1] {
		decisions = append(decisions, other.Decisions...)
		assumptions = append(assumptions, other.Assumptions...)
	}
	if len(decisions)+len(assumptions) == 0 {
		return ""
	}
	return "\nWhat this meeting has decided so far:\n" + decisionsText(decisions, assumptions)
}

// reading shows documents whole, as far as one question carries them: the
// documents a topic reads, and the ones it ends in as they stand.
func (p *planner) reading(docs []string) string {
	var b strings.Builder
	room := maxReading
	var left []string
	for _, doc := range docs {
		content := p.read(doc)
		if strings.TrimSpace(content) == "" {
			continue
		}
		shown, cut := headOf(content, maxDocument)
		if len(shown) > room {
			left = append(left, doc)
			continue
		}
		room -= len(shown)
		owner := "nobody's"
		if role := p.roster.Owner(doc); role != nil {
			owner = role.ID + "'s"
		}
		fmt.Fprintf(&b, "===== %s (%s)\n%s\n", doc, owner, strings.TrimRight(shown, "\n"))
		if cut {
			b.WriteString("(cut here; the file has the rest)\n")
		}
	}
	if len(left) > 0 {
		fmt.Fprintf(&b, "(Not shown, the documents together being longer than one question carries: %s.)\n", strings.Join(left, ", "))
	}
	return b.String()
}

// topicReading shows what a topic reads, and what it ends in as it stands.
func (p *planner) topicReading(r *TopicRecord) string {
	var docs []string
	for _, doc := range append(append([]string(nil), r.Topic.Reads...), r.Topic.Writes...) {
		if !contains(docs, doc) {
			docs = append(docs, doc)
		}
	}
	shown := p.reading(docs)
	if shown == "" {
		return "\nNone of the documents of this topic is written yet.\n"
	}
	return "\nThe documents of this topic, as they stand. Read them before you answer:\n" + shown
}

// speechText shows what one role said to a topic.
func speechText(speech *Speech) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", strings.TrimSpace(speech.Says))
	if items := nonEmpty(speech.Proposals); len(items) > 0 {
		b.WriteString("Proposes:\n" + bullets(items))
	}
	if items := nonEmpty(speech.Concerns); len(items) > 0 {
		b.WriteString("Worried about:\n" + bullets(items))
	}
	for _, objection := range speech.Objections {
		fmt.Fprintf(&b, "Objects to %s: %s\n  Why: %s\n  Wants: %s\n", objection.To, oneLine(objection.Point), oneLine(objection.Why), oneLine(objection.Wants))
	}
	return b.String()
}

// saidText shows what was said to a topic, in the order it was said.
func (p *planner) saidText(r *TopicRecord) string {
	var b strings.Builder
	if r.Proposal != nil {
		lead := p.roster.Role(r.Proposal.Role)
		fmt.Fprintf(&b, "## %s (%s), who leads, proposes:\n%s\n", lead.ID, lead.Title, speechText(r.Proposal))
	}
	for i := range r.Speeches {
		role := p.roster.Role(r.Speeches[i].Role)
		fmt.Fprintf(&b, "## %s (%s):\n%s\n", role.ID, role.Title, speechText(&r.Speeches[i]))
	}
	return b.String()
}

// proposalPrompt asks the lead of a topic how it is settled.
func (p *planner) proposalPrompt(role *Role, plan *Plan, r *TopicRecord) string {
	return p.frame(role) + earlierTopics(plan, r) + decidedSoFar(plan, r) + topicHead(plan, r) + p.topicReading(r) + `
Your task now: you lead this topic. Propose how it is settled, from where you stand. Be specific: numbers, names, limits. A proposal anyone could make is not one. The others speak after you, in turn.

` + jsonOnly + `
{
  "says": "your proposal, in a few sentences",
  "proposals": ["what you propose, one concrete item each"],
  "concerns": ["what worries you about the topic"]
}
At most 8 proposals and 4 concerns.
`
}

// speechPrompt asks a role that speaks to a topic what it says, once it has
// heard the ones before it.
func (p *planner) speechPrompt(role *Role, plan *Plan, r *TopicRecord) string {
	// Who has spoken: an objection is to what was said, not to what a role
	// that speaks later might say.
	spoken := []string{r.Topic.Lead}
	for _, speech := range r.Speeches {
		spoken = append(spoken, speech.Role)
	}
	return p.frame(role) + earlierTopics(plan, r) + decidedSoFar(plan, r) + topicHead(plan, r) + p.topicReading(r) +
		"\nWhat was said to the topic so far:\n\n" + p.saidText(r) + `
Your task now: speak to the topic, from where you stand. Say what you would add or change. Object to what you cannot accept in what was said, addressed to the role that said it: a proposal that is wrong, risky, too large, impossible to test, or in conflict with what you stand for. Only what was said can be objected to, so an objection is addressed to one of ` + strings.Join(spoken, ", ") + `; what a role that speaks after you might say, you say yourself. Do not repeat what was said, and do not invent objections: when you can accept it, say so in a line.

` + jsonOnly + `
{
  "says": "where you stand on the topic, in a few sentences",
  "proposals": ["what you would add or change, one concrete item each"],
  "concerns": ["what worries you"],
  "objections": [{"to": "the role id of who said it", "point": "what you object to", "why": "your reason, concretely", "wants": "what would settle it for you"}]
}
At most 5 proposals, 3 concerns and 3 objections, the strongest first.
`
}

// disputesPrompt asks the moderator which objections of a topic are argued out.
func (p *planner) disputesPrompt(r *TopicRecord) string {
	var b strings.Builder
	b.WriteString(moderatorFrame)
	fmt.Fprintf(&b, "From the objections raised on the topic below, pick the %d that most need to be argued out.\n\n", p.roster.Limits.Disputes)
	b.WriteString(p.languageLine())
	fmt.Fprintf(&b, "\n\nThe topic: %s\nIt settles: %s\n\nThe objections:\n", oneLine(r.Topic.Title), oneLine(r.Topic.Question))
	for i, objection := range r.Objections {
		fmt.Fprintf(&b, "%d. %s objects to %s\n   Point: %s\n   Why: %s\n   Wants: %s\n", i+1, objection.From, objection.To,
			oneLine(objection.Point), oneLine(objection.Why), oneLine(objection.Wants))
	}
	b.WriteString(`
Prefer the objections that would change what gets built if they stood. Objections about the same point become one dispute: keep the strongest wording and one challenger. Do not soften an objection, and do not turn one into agreement.

` + jsonOnly + `
{
  "disputes": [
    {"topic": "the point, in a few words", "challenger": "role id", "defender": "role id", "point": "the objection, in the challenger's terms", "wants": "what the challenger asks for"}
  ]
}
`)
	return b.String()
}

// exchangesText shows how a dispute has gone so far.
func (p *planner) exchangesText(dispute *Dispute) string {
	var b strings.Builder
	for i, exchange := range dispute.Exchanges {
		fmt.Fprintf(&b, "Round %d\n  %s (%s): %s\n  Their proposal now: %s\n", i+1, dispute.Defender, exchange.Stance,
			oneLine(exchange.Reply), oneLine(exchange.Position))
		if exchange.Answered {
			verdict := "does not accept"
			if exchange.Accepts {
				verdict = "accepts"
			}
			fmt.Fprintf(&b, "  %s %s: %s\n", dispute.Challenger, verdict, oneLine(exchange.Answer))
			if !exchange.Accepts && exchange.StillWants != "" {
				fmt.Fprintf(&b, "  Still wants: %s\n", oneLine(exchange.StillWants))
			}
		}
	}
	return b.String()
}

// ownSaying reminds a role of what it said to the topic itself.
func ownSaying(r *TopicRecord, role *Role) string {
	if r.Proposal != nil && r.Proposal.Role == role.ID {
		return "What you proposed:\n" + speechText(r.Proposal)
	}
	for i := range r.Speeches {
		if r.Speeches[i].Role == role.ID {
			return "What you said:\n" + speechText(&r.Speeches[i])
		}
	}
	return ""
}

// defencePrompt asks the challenged role to answer an objection.
func (p *planner) defencePrompt(role *Role, plan *Plan, r *TopicRecord, dispute *Dispute) string {
	var b strings.Builder
	b.WriteString(p.frame(role))
	b.WriteString(topicHead(plan, r))
	b.WriteString("\n" + ownSaying(r, role))
	fmt.Fprintf(&b, "\n%s objects to what you said.\n  Point: %s\n  Wants: %s\n", dispute.Challenger, oneLine(dispute.Point), oneLine(dispute.Wants))
	if len(dispute.Exchanges) > 0 {
		b.WriteString("\nHow it has gone so far:\n")
		b.WriteString(p.exchangesText(dispute))
	}
	b.WriteString(`
Your task now: answer the objection. There are three ways:
- "concede": the objection is right. Say what you change.
- "revise": part of it is right. Say what your proposal becomes.
- "hold": you keep your position. Say why the objection does not stand.
Do not concede to end the argument, and do not hold out of habit.

` + jsonOnly + `
{"stance": "concede | revise | hold", "reply": "your answer, in a few sentences", "position": "your proposal on this point as it now stands"}
`)
	return b.String()
}

// replyPrompt asks the challenger whether the answer settles the objection.
func (p *planner) replyPrompt(role *Role, plan *Plan, r *TopicRecord, dispute *Dispute) string {
	var b strings.Builder
	b.WriteString(p.frame(role))
	b.WriteString(topicHead(plan, r))
	b.WriteString("\n" + ownSaying(r, role))
	fmt.Fprintf(&b, "\nYou objected to %s.\n  Your point: %s\n  You wanted: %s\n\nHow it has gone:\n", dispute.Defender, oneLine(dispute.Point), oneLine(dispute.Wants))
	b.WriteString(p.exchangesText(dispute))
	b.WriteString(`
Your task now: say whether the last answer settles your objection. Accept an answer that deals with your reason, even if it is not what you asked for. Do not accept one that only sounds agreeable.

` + jsonOnly + `
{"accepts": true, "reply": "why, in a few sentences", "wants": "when you do not accept: what is still missing"}
`)
	return b.String()
}

// disputesText shows every dispute of a topic with how it ended.
func (p *planner) disputesText(r *TopicRecord, onlyOpen bool) string {
	var b strings.Builder
	for i := range r.Disputes {
		dispute := &r.Disputes[i]
		if onlyOpen && dispute.Settled {
			continue
		}
		state := "NOT SETTLED"
		if dispute.Settled {
			state = "settled"
		}
		fmt.Fprintf(&b, "### %s (%s against %s) - %s\nObjection: %s\n%s", dispute.Topic, dispute.Challenger, dispute.Defender, state,
			oneLine(dispute.Point), p.exchangesText(dispute))
		if dispute.Outcome != "" {
			fmt.Fprintf(&b, "Outcome: %s\n", dispute.Outcome)
		}
		b.WriteString("\n")
	}
	if b.Len() == 0 {
		return "(none)\n"
	}
	return b.String()
}

// summaryPrompt asks the moderator where a topic stands.
func (p *planner) summaryPrompt(plan *Plan, r *TopicRecord) string {
	var b strings.Builder
	b.WriteString(moderatorFrame)
	b.WriteString("Say where the topic below stands, faithfully: who holds what. Do not smooth a disagreement over, and add nothing of your own.\n\n")
	b.WriteString(p.languageLine())
	b.WriteString("\n\nThe brief:\n<brief>\n" + strings.TrimSpace(p.brief) + "\n</brief>\n")
	b.WriteString(topicHead(plan, r))
	b.WriteString("\nWhat was said:\n\n" + p.saidText(r))
	b.WriteString(p.ownerSaid())
	b.WriteString("\nThe disputes:\n\n" + p.disputesText(r, false))
	if dropped := r.undisputed(); len(dropped) > 0 {
		b.WriteString("Objections that were not argued out:\n")
		b.WriteString(bullets(dropped))
	}
	b.WriteString(`
` + jsonOnly + `
{
  "consensus": ["what the roles agree on, one point each"],
  "open": ["what is still disputed, naming who holds which side"],
  "questions": ["what only the owner of the project can answer, and has not answered"]
}
`)
	return b.String()
}

// decisionPrompt asks the deciding role to rule on a topic.
func (p *planner) decisionPrompt(role *Role, plan *Plan, r *TopicRecord) string {
	var b strings.Builder
	b.WriteString(p.frame(role))
	b.WriteString(decidedSoFar(plan, r))
	b.WriteString(topicHead(plan, r))
	b.WriteString("\nWhat was said:\n\n" + p.saidText(r))
	b.WriteString("\nWhere the topic stands, by the moderator:\nAgreed:\n")
	b.WriteString(bullets(r.Summary.Consensus))
	b.WriteString("Still disputed:\n")
	b.WriteString(bullets(r.Summary.Open))
	b.WriteString("For the owner to answer:\n")
	b.WriteString(bullets(r.Summary.Questions))
	b.WriteString("\nThe disputes that were not settled, in full:\n\n")
	b.WriteString(p.disputesText(r, true))
	if open := p.openQuestions(plan); len(open) > 0 {
		b.WriteString("Questions of the agenda the owner of the project has not answered:\n")
		b.WriteString(bullets(open))
	}
	b.WriteString(`
Your task now: decide this topic. Rule on every point that is still disputed: which way it goes, and why. Then state the other decisions the topic settles, the agreed ones included, briefly. A decision says what is built, or how. Which documents there are, where each is kept and who writes it is not one: the list above says, and the documents are written there whatever is decided. Write each so that it can be checked later. A decision that changes one on record names it (D12) and says what it becomes.

The plan is no larger than the brief. What the brief does not ask for stays out, however much the table said about it, unless what it asks for cannot work without it; where you take such a thing in, the decision says why.

What only the owner of the project can answer is not yours to decide, and the meeting does not wait for the answer. Where a ruling of this topic rests on such a question, say in assumptions what the plan takes to be true until the owner answers, and what has to change if it is not. An assumption this meeting made already (A2) is named in the decision, not made again.

` + jsonOnly + `
{
  "decisions": [
    {"title": "the point, in a few words", "decision": "what is decided", "rationale": "why", "dissent": "who held otherwise, and what; an empty string when nobody did"}
  ],
  "assumptions": [
    {"question": "what only the owner can answer", "assumption": "what the plan takes to be true until then", "if_wrong": "what has to change if it is not"}
  ]
}
`)
	return b.String()
}

// decisionsText shows decisions and what they assume, each under the name it
// has in the decision log.
func decisionsText(decisions []Decision, assumptions []Assumption) string {
	var b strings.Builder
	for _, decision := range decisions {
		fmt.Fprintf(&b, "%s. %s: %s\n   Why: %s\n", decision.ID, oneLine(decision.Title), oneLine(decision.Decision), oneLine(decision.Rationale))
		if strings.TrimSpace(decision.Dissent) != "" {
			fmt.Fprintf(&b, "   Held otherwise: %s\n", oneLine(decision.Dissent))
		}
	}
	if b.Len() == 0 {
		b.WriteString("(none)\n")
	}
	if len(assumptions) > 0 {
		b.WriteString("\nAssumed until the owner of the project says otherwise:\n")
		for _, assumption := range assumptions {
			fmt.Fprintf(&b, "%s. %s\n", assumption.ID, oneLine(firstNonEmpty(assumption.Question, assumption.Assumption)))
			if strings.TrimSpace(assumption.Question) != "" {
				fmt.Fprintf(&b, "   Assumed: %s\n", oneLine(assumption.Assumption))
			}
			if strings.TrimSpace(assumption.IfWrong) != "" {
				fmt.Fprintf(&b, "   If that is wrong: %s\n", oneLine(assumption.IfWrong))
			}
		}
	}
	return b.String()
}

// throughTopic shows what a meeting has decided up to and with a topic.
func throughTopic(plan *Plan, r *TopicRecord) string {
	var decisions []Decision
	var assumptions []Assumption
	for _, other := range plan.Topics[:r.Number] {
		decisions = append(decisions, other.Decisions...)
		assumptions = append(assumptions, other.Assumptions...)
	}
	return decisionsText(decisions, assumptions)
}

// asTheyStand shows a role its documents of a topic as they are.
func (p *planner) asTheyStand(docs []string) string {
	var b strings.Builder
	for _, doc := range docs {
		content := p.read(doc)
		if strings.TrimSpace(content) == "" {
			fmt.Fprintf(&b, "- %s: not written yet\n", doc)
			continue
		}
		fmt.Fprintf(&b, "===== %s\n%s\n", doc, strings.TrimRight(content, "\n"))
	}
	return b.String()
}

// documentsPrompt asks a role to write its documents of a topic: the ones the
// council can write (at.asked: the topic's, of this role, without what is too
// long), by the project's rules for those files (at.book).
func (p *planner) documentsPrompt(role *Role, plan *Plan, r *TopicRecord, at *seat) string {
	asked, book := at.asked, at.book
	var b strings.Builder
	b.WriteString(p.frame(role))
	b.WriteString(topicHead(plan, r))
	b.WriteString("\nThe topic is decided. What this meeting has decided, up to and with this topic:\n")
	b.WriteString(throughTopic(plan, r))
	b.WriteString("\nWhat the table agreed on in this topic:\n")
	b.WriteString(bullets(r.Summary.Consensus))
	b.WriteString("\nWhat was said:\n\n" + p.saidText(r))
	var others []string
	for _, doc := range r.Topic.Reads {
		if !contains(asked, doc) {
			others = append(others, doc)
		}
	}
	if shown := p.reading(others); shown != "" {
		b.WriteString("\nThe documents this topic read:\n" + shown)
	}
	b.WriteString("\nYour documents of this topic as they stand:\n" + p.asTheyStand(asked))
	if len(at.tables) > 0 {
		b.WriteString("\nThe tables of the game data this topic wrote before your turn:\n" + p.reading(at.tables))
		if len(at.keys) > 0 {
			b.WriteString("\nThey hold these text keys, and the text tables do not have them yet. Write the text of each in your text tables, in the language the text is written in and in each language it is translated into:\n" + keysText(at.keys))
		}
	}
	b.WriteString("\nYour task now: write your documents of this topic, as they read after it:\n")
	written, data := false, false
	for _, doc := range asked {
		fmt.Fprintf(&b, "- %s\n", doc)
		if p.exists(doc) {
			written = true
		}
		data = data || p.isGameData(doc)
	}
	for _, doc := range r.Topic.Writes {
		if role.OwnsDocument(doc) && !contains(asked, doc) {
			fmt.Fprintf(&b, "(%s is too long to be rewritten here. It stays as it is: do not hand it in.)\n", doc)
		}
	}
	b.WriteString(`
A document is the current truth about its subject, not the minutes of a meeting: no "we discussed", no "it was proposed", no list of the decisions and no account of who objected; docs/decisions.md holds those. Where a decision settles something, the document follows it, also where you held otherwise, and names the decision (D4) where a reader will ask why. What rests on an assumption says so and names it (A2). What another role's document holds, yours links to and does not repeat.
`)
	if data {
		b.WriteString(gameDataForm)
	}
	b.WriteString(p.byTheRules(role, plan, book))
	if written {
		b.WriteString(`
A document that is written already: change what this topic changes and keep the rest as it is, word for word. When the topic changes nothing in it, do not write it again; answer for it with this single line:
<<<UNCHANGED path/of/the/document.md>>>
`)
	}
	fmt.Fprintf(&b, `
One document is at most %d bytes: the council writes a document whole. Answer with the documents and nothing else, each one between these two lines:
<<<DOCUMENT path/of/the/document.md>>>
(the whole document)
<<<END>>>
`, maxDocument)
	return b.String()
}

// documentState says what a meeting did with each document it was asked for.
func documentState(plan *Plan) map[string]string {
	state := map[string]string{}
	for _, written := range plan.Documents {
		switch {
		case written.Refused != "":
		case written.Unchanged:
			if state[written.Path] == "" {
				state[written.Path] = "left unchanged by this meeting"
			}
		default:
			state[written.Path] = "written in this meeting"
		}
	}
	return state
}

// documentsText shows the project's documents as they stand after the
// meeting, whole as far as one question carries them, for the role that turns
// them into tickets.
func (p *planner) documentsText(plan *Plan) string {
	state := documentState(plan)
	var b strings.Builder
	room := maxReading
	var left []string
	for i := range p.roster.Roles {
		role := &p.roster.Roles[i]
		for _, doc := range p.documentsOf(role) {
			content := p.read(doc)
			if strings.TrimSpace(content) == "" {
				continue
			}
			shown, cut := headOf(content, maxDocument)
			if len(shown) > room {
				left = append(left, doc)
				continue
			}
			room -= len(shown)
			note := state[doc]
			if note == "" {
				note = "not written by this meeting"
			}
			fmt.Fprintf(&b, "===== %s (owner: %s; %s)\n%s\n", doc, role.ID, note, strings.TrimRight(shown, "\n"))
			if cut {
				b.WriteString("(cut here; the file has the rest)\n")
			}
		}
	}
	if len(left) > 0 {
		fmt.Fprintf(&b, "(Not shown, the documents together being longer than one question carries: %s.)\n", strings.Join(left, ", "))
	}
	if b.Len() == 0 {
		return "(none)\n"
	}
	return b.String()
}

// documentsList names the project's documents and what the meeting did with
// each, for the roles that look at the work list.
func (p *planner) documentsList(plan *Plan) string {
	state := documentState(plan)
	var b strings.Builder
	for i := range p.roster.Roles {
		role := &p.roster.Roles[i]
		for _, doc := range p.documentsOf(role) {
			if !p.exists(doc) {
				continue
			}
			note := state[doc]
			if note == "" {
				note = "not written by this meeting"
			}
			fmt.Fprintf(&b, "- %s (owner: %s; %s)\n", doc, role.ID, note)
		}
	}
	if b.Len() == 0 {
		return "(none)\n"
	}
	return b.String()
}

const workShape = `{
  "milestones": [
    {"key": "m1", "title": "what a person sees working once it is reached", "goal": "why it comes at this point, in a sentence or two", "done_when": ["what shows it is reached, one check each"]}
  ],
  "tickets": [
    {
      "key": "t1",
      "title": "[feat] what the ticket does",
      "repo": "a repo name",
      "summary": "what and why, in two or three sentences",
      "scope": ["what is built or changed"],
      "acceptance": ["behaviour a test can check"],
      "testing": ["what must be tested"],
      "reads": ["the path of a document the developer has to read"],
      "depends_on": ["key of a ticket that must be done first"]
    }
  ],
  "later": ["what this plan leaves out of its milestones, for a later plan"]`

// notTickets says what a work list does not hold, to the role that writes it
// and to the roles that look at it.
const notTickets = "A ticket is work on the project itself: code, tests, build, configuration. The documents are written: no ticket writes or rewrites one of them. What waits for an answer from the owner of the project is not a ticket either."

func (p *planner) ticketRules(earlier bool) string {
	names := make([]string, len(p.repos))
	for i, repo := range p.repos {
		names[i] = repo.Name
	}
	rules := fmt.Sprintf(`Rules for the milestones:
- Lay what the meeting decided out in milestones, at most %d, in the order they are reached. Each is something a person can see working, not a layer of the code: the first is the smallest that shows the core of the brief works, end to end.
- done_when says how a person checks the milestone is reached, one check each.
- Only the first milestone is ticketed. The others are named, not ticketed: they are planned when the first is reached, from what was learned on the way.

Rules for the tickets:
- %s
- The tickets build the first milestone, and no more.
- Each ticket is a piece one developer can build, test and merge alone. A ticket costs a pull request and a review: what one developer builds in a day is one ticket, not three.
- Cut the work by what somebody can see working once the ticket is merged, not by function or by file. A helper is built by the ticket that first needs it.
- Tests come with the ticket that builds what they test. A ticket of tests alone is for what only shows once several tickets are in, an end-to-end run for instance.
- Each ticket is for exactly one repo: %s.
- A ticket that needs another ticket's work first names it in depends_on, by key. No circles.
- The order of the list is the order the tickets should be worked on.
- Acceptance criteria describe behaviour a test can check: not test names, not "the code is clean".
- A ticket can be understood alone: it names, in reads, the documents the developer has to read.
- A title is "[type] subject" in lower case; type is one of feat, fix, refactor, test, docs, chore.
- At most %d tickets. When the first milestone needs more, it is too large: make it smaller.
`, maxMilestones, notTickets, strings.Join(names, ", "), p.maxTickets)
	if earlier {
		rules += "- Work an earlier plan ticketed is not ticketed again, whether its ticket is on GitHub or only proposed. A ticket that builds on an earlier one names it in depends_on by what its line in the list of earlier tickets starts with: \"#12\" for an issue, \"<plan>/<key>\" for a ticket that is only proposed.\n"
	}
	return rules
}

// worklistPrompt asks the planning role for the milestones and the tickets.
func (p *planner) worklistPrompt(role *Role, plan *Plan) string {
	var b strings.Builder
	b.WriteString(p.frame(role))
	b.WriteString("\nThe meeting is over. What it decided:\n" + decisionsText(plan.Decisions, plan.Assumptions))
	b.WriteString("\nThe project's documents, as they stand after the meeting:\n\n" + p.documentsText(plan))
	earlier := p.earlierTickets(plan.ID)
	if earlier != "" {
		b.WriteString("\nWhat earlier plans ticketed:\n" + earlier)
	}
	b.WriteString("\nYour task now: lay the plan out in milestones, and turn the first into tickets.\n\n" + p.ticketRules(earlier != "") + "\n" + jsonOnly + "\n" + workShape + "\n}\n")
	return b.String()
}

// milestonesText shows the milestones of a work list.
func milestonesText(milestones []Milestone) string {
	var b strings.Builder
	for i, milestone := range milestones {
		ticketed := ""
		if i == 0 {
			ticketed = " (ticketed below)"
		}
		fmt.Fprintf(&b, "%s  %s%s\n  %s\n  Reached when:\n%s", milestone.Key, oneLine(milestone.Title), ticketed, oneLine(milestone.Goal), indent(bullets(milestone.DoneWhen)))
		b.WriteString("\n")
	}
	return b.String()
}

// ticketsText shows a work list.
func ticketsText(tickets []Ticket) string {
	var b strings.Builder
	for _, ticket := range tickets {
		fmt.Fprintf(&b, "%s  %s  (repo: %s", ticket.Key, ticket.Title, ticket.Repo)
		if len(ticket.DependsOn) > 0 {
			fmt.Fprintf(&b, "; after %s", strings.Join(ticket.DependsOn, ", "))
		}
		fmt.Fprintf(&b, ")\n  %s\n  Scope:\n%s  Accepted when:\n%s", oneLine(ticket.Summary), indent(bullets(ticket.Scope)), indent(bullets(ticket.Acceptance)))
		b.WriteString("\n")
	}
	return b.String()
}

// worklistReviewPrompt asks a role what is wrong with the work list.
func (p *planner) worklistReviewPrompt(role *Role, plan *Plan, milestones []Milestone, tickets []Ticket) string {
	return p.frame(role) + "\nWhat the meeting decided:\n" + decisionsText(plan.Decisions, plan.Assumptions) + "\nThe documents, written by their owners:\n" + p.documentsList(plan) +
		"\nThe milestones " + p.roster.Planner().ID + " laid out:\n\n" + milestonesText(milestones) +
		"The tickets of the first milestone:\n\n" + ticketsText(tickets) + `
Your task now: object to the work list, from where you stand: a first milestone that does not show the core of the brief works, or is larger than it needs to be; work the first milestone needs that is missing; a ticket too large to merge alone; a wrong order; an acceptance criterion nobody can check; something the decisions ruled out. ` + notTickets + ` Do not ask for more than was decided. When the list is acceptable, the list of objections is empty.

` + jsonOnly + `
{"objections": [{"ticket": "the key of a ticket or a milestone, or empty for the list as a whole", "point": "what is wrong", "wants": "what would fix it"}]}
At most 4 objections, the strongest first.
`
}

// worklistRevisionPrompt asks the planning role to take the objections in.
func (p *planner) worklistRevisionPrompt(role *Role, plan *Plan, milestones []Milestone, tickets []Ticket, objections []ListObjection) string {
	var b strings.Builder
	b.WriteString(p.frame(role))
	b.WriteString("\nWhat the meeting decided:\n" + decisionsText(plan.Decisions, plan.Assumptions) + "\nThe documents, written by their owners:\n" + p.documentsList(plan) +
		"\nYour milestones:\n\n" + milestonesText(milestones) + "Your tickets of the first milestone:\n\n" + ticketsText(tickets) + "\nThe table's objections to them:\n")
	for _, objection := range objections {
		target := "the list"
		if objection.Ticket != "" {
			target = objection.Ticket
		}
		fmt.Fprintf(&b, "- %s on %s: %s Wants: %s\n", objection.From, target, oneLine(objection.Point), oneLine(objection.Wants))
	}
	earlier := p.earlierTickets(plan.ID) != ""
	b.WriteString(`
Your task now: revise the milestones and the tickets. Take in what is right. An objection that asks for more than was decided, or for something the rules below keep out of a list, is not taken in. Where you leave something as it is, say why in kept.

` + p.ticketRules(earlier) + "\n" + jsonOnly + "\n" + workShape + `,
  "kept": ["an objection you did not take, and why"]
}
`)
	return b.String()
}
