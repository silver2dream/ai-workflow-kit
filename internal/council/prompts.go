package council

import (
	"fmt"
	"strings"
)

// The prompts of a plan. Each call is one question to one role; everything
// the role needs to answer it is in the prompt, because a role remembers
// nothing between calls.

const jsonOnly = "Answer with one JSON object in a ```json block, and nothing else."

// frame opens every prompt to a role: who it is, what it stands for, who else
// is at the table, the project, what it owns, what is decided already, and
// the brief.
func (p *planner) frame(role *Role) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are the %s (%s) at a planning meeting for a software project. The meeting turns a brief into documents, decisions and a list of tickets.\n\n", role.Title, role.ID)
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
	if len(role.Owns) > 0 {
		b.WriteString("\nThe documents you own. Nobody else writes them, and you write no other:\n")
		for _, doc := range role.Owns {
			fmt.Fprintf(&b, "- %s\n", doc)
		}
	}
	if log := p.decisionLog(); log != "" {
		b.WriteString("\nDecisions already on record (docs/decisions.md):\n")
		b.WriteString(log)
	}
	b.WriteString("\nThe brief:\n<brief>\n")
	b.WriteString(strings.TrimSpace(p.brief))
	b.WriteString("\n</brief>\n")
	b.WriteString(p.ownerSaid())
	return b.String()
}

// ownerSaid tells what the owner of the project answered when the table's
// questions were put to them. It stands with the brief: it is the owner's
// word, and no role argues against it.
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
	return "\nThe owner of the project was asked by this table, and answered. This holds like the brief:\n" + b.String()
}

// questionsPrompt asks the moderator to make one list of what the roles want
// to know from the owner.
func (p *planner) questionsPrompt(asked []string) string {
	var b strings.Builder
	b.WriteString(moderatorFrame)
	b.WriteString("The roles have questions only the owner of the project can answer. Put them to the owner as one list: one question where several roles ask the same thing, each one short enough to be answered in a sentence or two, the ones whose answer changes most what gets built first. Add none of your own, and answer none.\n\n")
	b.WriteString(p.languageLine())
	b.WriteString("\n\nThe brief:\n<brief>\n" + strings.TrimSpace(p.brief) + "\n</brief>\n\nWhat the roles ask:\n")
	b.WriteString(bullets(asked))
	fmt.Fprintf(&b, "\n%s\n{\"questions\": [\"a question for the owner\"]}\nAt most %d questions.\n", jsonOnly, maxQuestions)
	return b.String()
}

// standing shows a role its own documents as they are, where it needs to
// have read them: before it takes a position, and before it writes.
func (p *planner) standing(role *Role) string {
	if len(role.Owns) == 0 {
		return ""
	}
	return "\nYour documents as they stand:\n" + p.ownDocuments(role)
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

// positionPrompt asks a role where it stands on the brief.
func (p *planner) positionPrompt(role *Role) string {
	return p.frame(role) + p.standing(role) + `
Your task now: state your position on the brief, from where you stand. Be specific: numbers, names, limits. A position anyone could hold is not one.

` + jsonOnly + `
{
  "summary": "your position, in two or three sentences",
  "proposals": ["what you propose, one concrete item each"],
  "concerns": ["what worries you about the brief as it stands"],
  "questions": ["what only the owner of the project can answer; empty if nothing blocks you"]
}
At most 6 proposals, 5 concerns and 3 questions.
`
}

// positionsText shows the positions to a role: everyone's but its own.
func (p *planner) positionsText(plan *Plan, except string) string {
	var b strings.Builder
	for _, position := range plan.Positions {
		if position.Role == except {
			continue
		}
		role := p.roster.Role(position.Role)
		fmt.Fprintf(&b, "## %s (%s)\n%s\nProposes:\n%sWorried about:\n%s\n", role.ID, role.Title,
			strings.TrimSpace(position.Summary), bullets(position.Proposals), bullets(position.Concerns))
	}
	return b.String()
}

// ownPosition reminds a role of what it said itself.
func ownPosition(plan *Plan, role *Role) string {
	position := plan.position(role.ID)
	if position == nil {
		return ""
	}
	return fmt.Sprintf("Your own position:\n%s\nYou proposed:\n%s", strings.TrimSpace(position.Summary), bullets(position.Proposals))
}

// objectionPrompt asks a role what it cannot accept in the others' positions.
func (p *planner) objectionPrompt(role *Role, plan *Plan) string {
	return p.frame(role) + "\n" + ownPosition(plan, role) + "\nWhat the others hold:\n\n" + p.positionsText(plan, role.ID) + `
Your task now: object. Read the others' positions against what you stand for, and name what you cannot accept as it stands: a proposal that is wrong, risky, too large, impossible to test, or in conflict with yours. One objection per point, addressed to the role that made it. Do not invent objections: when you can accept everything, the list is empty.

` + jsonOnly + `
{
  "objections": [
    {"to": "the role id", "point": "what you object to; name the proposal", "why": "your reason, concretely", "wants": "what would settle it for you"}
  ]
}
At most 4 objections, the strongest first.
`
}

const moderatorFrame = "You moderate a planning meeting for a software project. You have no opinion on the subject, you never take a side and you never answer for a role. "

// disputesPrompt asks the moderator which objections are argued out.
func (p *planner) disputesPrompt(plan *Plan) string {
	var b strings.Builder
	b.WriteString(moderatorFrame)
	fmt.Fprintf(&b, "From the objections below, pick the %d that most need to be argued out.\n\n", p.roster.Limits.Disputes)
	b.WriteString(p.languageLine())
	b.WriteString("\n\nThe objections:\n")
	for i, objection := range plan.Objections {
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

func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
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

// defencePrompt asks the challenged role to answer an objection.
func (p *planner) defencePrompt(role *Role, plan *Plan, dispute *Dispute) string {
	var b strings.Builder
	b.WriteString(p.frame(role))
	b.WriteString("\n" + ownPosition(plan, role))
	fmt.Fprintf(&b, "\n%s objects to your position.\n  Point: %s\n  Wants: %s\n", dispute.Challenger, oneLine(dispute.Point), oneLine(dispute.Wants))
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
func (p *planner) replyPrompt(role *Role, plan *Plan, dispute *Dispute) string {
	var b strings.Builder
	b.WriteString(p.frame(role))
	b.WriteString("\n" + ownPosition(plan, role))
	fmt.Fprintf(&b, "\nYou objected to %s.\n  Your point: %s\n  You wanted: %s\n\nHow it has gone:\n", dispute.Defender, oneLine(dispute.Point), oneLine(dispute.Wants))
	b.WriteString(p.exchangesText(dispute))
	b.WriteString(`
Your task now: say whether the last answer settles your objection. Accept an answer that deals with your reason, even if it is not what you asked for. Do not accept one that only sounds agreeable.

` + jsonOnly + `
{"accepts": true, "reply": "why, in a few sentences", "wants": "when you do not accept: what is still missing"}
`)
	return b.String()
}

// disputesText shows every dispute with how it ended.
func (p *planner) disputesText(plan *Plan, onlyOpen bool) string {
	var b strings.Builder
	for i := range plan.Disputes {
		dispute := &plan.Disputes[i]
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

// summaryPrompt asks the moderator where the meeting stands.
func (p *planner) summaryPrompt(plan *Plan) string {
	var b strings.Builder
	b.WriteString(moderatorFrame)
	b.WriteString("Say where the meeting stands, faithfully: who holds what. Do not smooth a disagreement over, and add nothing of your own.\n\n")
	b.WriteString(p.languageLine())
	b.WriteString("\n\nThe brief:\n<brief>\n" + strings.TrimSpace(p.brief) + "\n</brief>\n\nThe positions:\n\n")
	b.WriteString(p.positionsText(plan, ""))
	b.WriteString("What the roles asked the owner:\n")
	b.WriteString(bullets(plan.questions()))
	b.WriteString(p.ownerSaid())
	b.WriteString("\nThe disputes:\n\n")
	b.WriteString(p.disputesText(plan, false))
	if dropped := plan.undisputed(); len(dropped) > 0 {
		b.WriteString("Objections that were not argued out:\n")
		b.WriteString(bullets(dropped))
	}
	b.WriteString(`
` + jsonOnly + `
{
  "consensus": ["what the table agrees on, one point each"],
  "open": ["what is still disputed, naming who holds which side"],
  "questions": ["what only the owner of the project can answer, and has not answered"]
}
`)
	return b.String()
}

// decisionPrompt asks the deciding role to rule.
func (p *planner) decisionPrompt(role *Role, plan *Plan) string {
	var b strings.Builder
	b.WriteString(p.frame(role))
	b.WriteString("\nWhere the meeting stands, by the moderator:\nAgreed:\n")
	b.WriteString(bullets(plan.Summary.Consensus))
	b.WriteString("Still disputed:\n")
	b.WriteString(bullets(plan.Summary.Open))
	b.WriteString("For the owner to answer:\n")
	b.WriteString(bullets(plan.Summary.Questions))
	b.WriteString("\nThe disputes that were not settled, in full:\n\n")
	b.WriteString(p.disputesText(plan, true))
	b.WriteString(`
Your task now: decide. Rule on every point that is still disputed: which way it goes, and why. Then state the other decisions the plan rests on, the agreed ones included, briefly. A decision says what is built, or how; who writes which document is not one. Write each so that it can be checked later. A decision that changes one already on record names it (D12) and says what it becomes.

The plan is no larger than the brief. What the brief does not ask for stays out, however much the table said about it, unless what it asks for cannot work without it; where you take such a thing in, the decision says why.

What only the owner of the project can answer is not yours to decide, and the meeting does not wait for the answer. For each such question, say in assumptions what the plan takes to be true until the owner answers, and what has to change if it is not.

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

// decisionsText shows the decisions of the plan and what it assumes, each
// under the name it has in the decision log.
func decisionsText(plan *Plan) string {
	var b strings.Builder
	for _, decision := range plan.Decisions {
		fmt.Fprintf(&b, "%s. %s: %s\n   Why: %s\n", decision.ID, oneLine(decision.Title), oneLine(decision.Decision), oneLine(decision.Rationale))
		if strings.TrimSpace(decision.Dissent) != "" {
			fmt.Fprintf(&b, "   Held otherwise: %s\n", oneLine(decision.Dissent))
		}
	}
	if b.Len() == 0 {
		b.WriteString("(none)\n")
	}
	if len(plan.Assumptions) > 0 {
		b.WriteString("\nAssumed until the owner of the project says otherwise:\n")
		for _, assumption := range plan.Assumptions {
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

// documentsPrompt asks a role to write the documents it owns. asked are the
// ones the council can write: what the role owns, without what is too long.
func (p *planner) documentsPrompt(role *Role, plan *Plan, asked []string) string {
	var b strings.Builder
	b.WriteString(p.frame(role))
	b.WriteString(p.standing(role))
	b.WriteString("\nThe meeting is over. What was decided:\n")
	b.WriteString(decisionsText(plan))
	b.WriteString("\nWhat the table agreed on:\n")
	b.WriteString(bullets(plan.Summary.Consensus))
	b.WriteString("\nYour task now: write the documents you own, as they read after this meeting:\n")
	written := false
	for _, doc := range asked {
		fmt.Fprintf(&b, "- %s\n", doc)
		if strings.TrimSpace(p.read(doc)) != "" {
			written = true
		}
	}
	for _, doc := range role.Owns {
		if !contains(asked, doc) {
			fmt.Fprintf(&b, "(%s is too long to be rewritten here. It stays as it is: do not hand it in.)\n", doc)
		}
	}
	b.WriteString(`
A document is the current truth about its subject, not the minutes of a meeting: no "we discussed", no "it was proposed", no list of the decisions and no account of who objected; docs/decisions.md holds those. Where a decision settles something, the document follows it, also where you held otherwise, and names the decision (D4) where a reader will ask why. What rests on an assumption says so and names it (A2).
`)
	if written {
		b.WriteString(`
A document that is written already: change what this meeting changes and keep the rest as it is, word for word. When the meeting changes nothing in it, do not write it again; answer for it with this single line:
<<<UNCHANGED docs/the/path.md>>>
`)
	}
	fmt.Fprintf(&b, `
Start every document you write with this header, then a title:
---
status: draft
owner: %s
updated: %s
plan: %s
---

Keep each document under 300 lines. Answer with the documents and nothing else, each one between these two lines:
<<<DOCUMENT docs/the/path.md>>>
(the whole document)
<<<END>>>
`, role.ID, p.today(), plan.ID)
	return b.String()
}

// documentState says what a meeting did with each document it was asked for.
func documentState(plan *Plan) map[string]string {
	state := map[string]string{}
	for _, written := range plan.Documents {
		switch {
		case written.Refused != "":
		case written.Unchanged:
			state[written.Path] = "left unchanged by this meeting"
		default:
			state[written.Path] = "written in this meeting"
		}
	}
	return state
}

// documentsText shows the project's documents as they stand after the
// meeting, whole, for the role that turns them into tickets.
func (p *planner) documentsText(plan *Plan) string {
	state := documentState(plan)
	var b strings.Builder
	for _, role := range p.roster.Roles {
		for _, doc := range role.Owns {
			content := p.read(doc)
			if strings.TrimSpace(content) == "" {
				continue
			}
			note := state[doc]
			if note == "" {
				note = "not written by this meeting"
			}
			shown, cut := headOf(content, maxDocument)
			fmt.Fprintf(&b, "===== %s (owner: %s; %s)\n%s\n", doc, role.ID, note, strings.TrimRight(shown, "\n"))
			if cut {
				b.WriteString("(cut here; the file has the rest)\n")
			}
		}
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
	for _, role := range p.roster.Roles {
		for _, doc := range role.Owns {
			if strings.TrimSpace(p.read(doc)) == "" {
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

const ticketShape = `{
  "tickets": [
    {
      "key": "t1",
      "title": "[feat] what the ticket does",
      "repo": "a repo name",
      "summary": "what and why, in two or three sentences",
      "scope": ["what is built or changed"],
      "acceptance": ["behaviour a test can check"],
      "testing": ["what must be tested"],
      "reads": ["docs/... the developer has to read"],
      "depends_on": ["key of a ticket that must be done first"]
    }
  ],
  "later": ["what this list leaves for a later plan"]`

// notTickets says what a work list does not hold, to the role that writes it
// and to the roles that look at it.
const notTickets = "A ticket is work on the project itself: code, tests, build, configuration. The documents are written: no ticket writes or rewrites one of them. What waits for an answer from the owner of the project is not a ticket either."

func (p *planner) ticketRules(earlier bool) string {
	names := make([]string, len(p.repos))
	for i, repo := range p.repos {
		names[i] = repo.Name
	}
	rules := fmt.Sprintf(`Rules for the list:
- %s
- The list covers what this meeting decided, and no more.
- Each ticket is a piece one developer can build, test and merge alone.
- Each ticket is for exactly one repo: %s.
- A ticket that needs another ticket's work first names it in depends_on, by key. No circles.
- The order of the list is the order the tickets should be worked on.
- Acceptance criteria describe behaviour a test can check: not test names, not "the code is clean".
- A ticket can be understood alone: it names, in reads, the documents the developer has to read.
- A title is "[type] subject" in lower case; type is one of feat, fix, refactor, test, docs, chore.
- At most %d tickets. When the plan needs more, cover what comes first and name the rest in later.
`, notTickets, strings.Join(names, ", "), p.maxTickets)
	if earlier {
		rules += "- Work an earlier plan ticketed is not ticketed again, whether its ticket is on GitHub or only proposed. A ticket that builds on an earlier one names it in depends_on by what its line in the list of earlier tickets starts with: \"#12\" for an issue, \"<plan>/<key>\" for a ticket that is only proposed.\n"
	}
	return rules
}

// worklistPrompt asks the planning role for the tickets.
func (p *planner) worklistPrompt(role *Role, plan *Plan) string {
	var b strings.Builder
	b.WriteString(p.frame(role))
	b.WriteString("\nThe meeting is over. What was decided:\n" + decisionsText(plan))
	b.WriteString("\nThe project's documents, as they stand after the meeting:\n\n" + p.documentsText(plan))
	earlier := p.earlierTickets(plan.ID)
	if earlier != "" {
		b.WriteString("\nWhat earlier plans ticketed:\n" + earlier)
	}
	b.WriteString("\nYour task now: turn the plan into tickets.\n\n" + p.ticketRules(earlier != "") + "\n" + jsonOnly + "\n" + ticketShape + "\n}\n")
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
func (p *planner) worklistReviewPrompt(role *Role, plan *Plan, tickets []Ticket) string {
	return p.frame(role) + "\nWhat was decided:\n" + decisionsText(plan) + "\nThe documents, written by their owners:\n" + p.documentsList(plan) +
		"\nThe work list " + p.roster.Planner().ID + " drew up from them:\n\n" + ticketsText(tickets) + `
Your task now: object to the work list, from where you stand: work the decisions call for that is missing, a ticket too large to merge alone, a wrong order, an acceptance criterion nobody can check, something the decisions ruled out. ` + notTickets + ` Do not ask for more than was decided. When the list is acceptable, the list of objections is empty.

` + jsonOnly + `
{"objections": [{"ticket": "the key, or empty for the list as a whole", "point": "what is wrong", "wants": "what would fix it"}]}
At most 4 objections, the strongest first.
`
}

// worklistRevisionPrompt asks the planning role to take the objections in.
func (p *planner) worklistRevisionPrompt(role *Role, plan *Plan, tickets []Ticket, objections []ListObjection) string {
	var b strings.Builder
	b.WriteString(p.frame(role))
	b.WriteString("\nWhat was decided:\n" + decisionsText(plan) + "\nThe documents, written by their owners:\n" + p.documentsList(plan) +
		"\nYour work list:\n\n" + ticketsText(tickets) + "\nThe table's objections to it:\n")
	for _, objection := range objections {
		target := "the list"
		if objection.Ticket != "" {
			target = objection.Ticket
		}
		fmt.Fprintf(&b, "- %s on %s: %s Wants: %s\n", objection.From, target, oneLine(objection.Point), oneLine(objection.Wants))
	}
	earlier := p.earlierTickets(plan.ID) != ""
	b.WriteString(`
Your task now: revise the list. Take in what is right. An objection that asks for more than was decided, or for something the rules below keep out of a list, is not taken in. Where you leave something as it is, say why in kept.

` + p.ticketRules(earlier) + "\n" + jsonOnly + "\n" + ticketShape + `,
  "kept": ["an objection you did not take, and why"]
}
`)
	return b.String()
}
