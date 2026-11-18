package council

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/layout"
)

// What a meeting leaves on disk besides the roles' own documents: its minutes,
// one file for the meeting and one for each topic, its decisions in the
// decision log, and the index of the documents.

func mdBullets(b *strings.Builder, items []string) {
	if len(items) == 0 {
		b.WriteString("- (none)\n")
		return
	}
	for _, item := range items {
		fmt.Fprintf(b, "- %s\n", oneLine(item))
	}
}

// topicFile names the minutes of a topic, relative to the plan's folder.
func topicFile(r *TopicRecord) string {
	return fmt.Sprintf("topics/%02d-%s.md", r.Number, r.Topic.ID)
}

// started reports whether anything of a topic was said.
func (r *TopicRecord) started() bool {
	return r.Proposal != nil
}

// writeMinutes writes the record of the meeting: the minutes of the meeting,
// and of every topic that was taken up. It is written from the roles' answers
// as they came, also when the meeting stopped early.
func (p *planner) writeMinutes(plan *Plan, stopped error) error {
	if err := p.reviewGameData(plan); err != nil {
		return err
	}
	for i := range plan.Topics {
		if plan.Topics[i].started() {
			if err := p.writeTopicMinutes(plan, &plan.Topics[i]); err != nil {
				return err
			}
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Plan %s: minutes\n\n", plan.ID)
	b.WriteString("> The record of the council's meeting, written by awkit from what the roles answered. Nobody edits it; what holds now is in the documents and in `docs/decisions.md`.\n\n")
	fmt.Fprintf(&b, "- Date: %s\n", p.today())
	var seats []string
	for _, role := range p.roster.Roles {
		seat := role.ID
		switch {
		case role.Decides && role.Plans:
			seat += " (decides, plans)"
		case role.Decides:
			seat += " (decides)"
		case role.Plans:
			seat += " (plans)"
		}
		seats = append(seats, seat)
	}
	fmt.Fprintf(&b, "- At the table: %s\n", strings.Join(seats, ", "))
	var waiting *Waiting
	switch {
	case errors.As(stopped, &waiting):
		fmt.Fprintf(&b, "- Status: **waits for the owner**: %s. Go on with `awkit plan --resume %s`.\n", waiting.Error(), plan.ID)
	case stopped != nil:
		fmt.Fprintf(&b, "- Status: **not finished**. %s. Go on with `awkit plan --resume %s`.\n", oneLine(stopped.Error()), plan.ID)
	case len(plan.Tickets) > 0:
		fmt.Fprintf(&b, "- Status: finished. %d tickets proposed in `%s/%s`; none is on GitHub until `awkit plan approve %s`.\n",
			len(plan.Tickets), plan.Dir(), WorkListFile, plan.ID)
	default:
		b.WriteString("- Status: under way.\n")
	}

	b.WriteString("\n## Brief\n\n" + strings.TrimSpace(plan.Brief) + "\n")

	if plan.Agenda != nil {
		fmt.Fprintf(&b, "\n## Agenda\n\nSet by the moderator, in `%s/%s`.\n\n| # | Topic | Lead | Speakers | Ends in | |\n|---|---|---|---|---|---|\n", plan.Dir(), AgendaFile)
		for i := range plan.Topics {
			r := &plan.Topics[i]
			title := strings.ReplaceAll(oneLine(r.Topic.Title), "|", "/")
			if r.started() {
				title = "[" + title + "](" + topicFile(r) + ")"
			}
			state := "not taken up"
			switch {
			case r.Held:
				state = "held"
			case r.started():
				state = "**stopped**"
			}
			fmt.Fprintf(&b, "| %d | %s | %s | %s | %s | %s |\n", r.Number, title, r.Topic.Lead, strings.Join(r.Topic.Speakers, ", "),
				strings.Join(quoted(r.Topic.Writes), ", "), state)
		}
	}

	if len(plan.Answers) > 0 {
		b.WriteString("\n## What the owner was asked\n\nThe questions of the agenda, put to the owner of the project before the first topic.\n\n")
		for _, answer := range plan.Answers {
			said := "(left open: the council assumes)"
			if answer.Answer != "" {
				said = oneLine(answer.Answer)
			}
			fmt.Fprintf(&b, "- %s\n  - %s\n", oneLine(answer.Question), said)
		}
	}

	if len(plan.Words) > 0 {
		b.WriteString("\n## What the owner said while the meeting went on\n\nEach holds like the brief from the step it was taken in at. Before it, " + p.roster.Decider().ID +
			" read every decision of the plan again against it, and the owners read again in full the documents that rest on what changed.\n")
		for i := range plan.Revisions {
			review := &plan.Revisions[i]
			fmt.Fprintf(&b, "\n### %s (taken in at: %s)\n\n", wordIDs(review.Words), review.At)
			for _, word := range review.Words {
				if word.Question != "" {
					fmt.Fprintf(&b, "> **%s**, to \"%s\": %s\n>\n", word.ID, oneLine(word.Question), oneLine(word.Text))
				} else {
					fmt.Fprintf(&b, "> **%s**: %s\n>\n", word.ID, oneLine(word.Text))
				}
			}
			b.WriteString("\n" + reviewText(review))
			if len(review.Documents) > 0 {
				b.WriteString("\nRead again:\n")
				writeDocuments(&b, p.roster, review.Documents)
			}
		}
		// A word taken in before anything was decided needed no reading again.
		var plain []OwnerWord
		for _, word := range plan.Words {
			reviewed := false
			for _, review := range plan.Revisions {
				for _, w := range review.Words {
					reviewed = reviewed || w.ID == word.ID
				}
			}
			if !reviewed {
				plain = append(plain, word)
			}
		}
		if len(plain) > 0 {
			b.WriteString("\nSaid before anything was decided, and held from then on:\n\n" + wordsText(plain))
		}
	}

	if len(plan.Decisions) > 0 {
		fmt.Fprintf(&b, "\n## Decisions\n\nBy %s, a topic at a time. They are on record in `%s`.\n", p.roster.Decider().ID, DecisionsPath)
		for i := range plan.Topics {
			r := &plan.Topics[i]
			if len(r.Decisions)+len(r.Assumptions) == 0 {
				continue
			}
			fmt.Fprintf(&b, "\n### %d. %s\n\n", r.Number, oneLine(r.Topic.Title))
			writeDecisions(&b, r.Decisions, r.Assumptions)
		}
	}

	if len(plan.Documents) > 0 {
		b.WriteString("\n## Documents\n\n")
		writeDocuments(&b, p.roster, plan.Documents)
		writeRuleChecks(&b, plan.Rules, plan.RulesLeft, plan.RuleChecks)
	}
	writeGameData(&b, plan)

	if len(plan.Milestones) > 0 {
		fmt.Fprintf(&b, "\n## Milestones\n\nLaid out by %s. Only the first is ticketed; the others are planned when it is reached.\n\n", p.roster.Planner().ID)
		for i, milestone := range plan.Milestones {
			ticketed := ""
			if i == 0 {
				ticketed = " (ticketed)"
			}
			fmt.Fprintf(&b, "- **%s %s**%s: %s\n", milestone.Key, oneLine(milestone.Title), ticketed, oneLine(milestone.Goal))
			for _, check := range milestone.DoneWhen {
				fmt.Fprintf(&b, "  - [ ] %s\n", oneLine(check))
			}
		}
	}

	if len(plan.Tickets) > 0 {
		fmt.Fprintf(&b, "\n## Work list\n\nThe tickets of the first milestone, proposed by %s, in `%s/%s`.\n\n| Key | Ticket | Repo | After |\n|---|---|---|---|\n",
			p.roster.Planner().ID, plan.Dir(), WorkListFile)
		for _, ticket := range plan.Tickets {
			after := "-"
			if len(ticket.DependsOn) > 0 {
				after = strings.Join(ticket.DependsOn, ", ")
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", ticket.Key, strings.ReplaceAll(oneLine(ticket.Title), "|", "/"), ticket.Repo, after)
		}
		if len(plan.ListObjections) > 0 {
			b.WriteString("\nThe table's objections to the first draft:\n")
			for _, objection := range plan.ListObjections {
				target := "the list"
				if objection.Ticket != "" {
					target = objection.Ticket
				}
				fmt.Fprintf(&b, "- %s on %s: %s Wants: %s\n", objection.From, target, oneLine(objection.Point), oneLine(objection.Wants))
			}
		}
		if len(plan.Kept) > 0 {
			b.WriteString("\nNot taken in, and why:\n")
			mdBullets(&b, plan.Kept)
		}
		if len(plan.Later) > 0 {
			b.WriteString("\nLeft for a later plan:\n")
			mdBullets(&b, plan.Later)
		}
	}
	return p.writeFile(plan.Dir()+"/minutes.md", b.String())
}

func quoted(files []string) []string {
	out := make([]string, len(files))
	for i, file := range files {
		out[i] = "`" + file + "`"
	}
	return out
}

// writeSpeech writes what one role said to a topic.
func writeSpeech(b *strings.Builder, speech *Speech) {
	b.WriteString(strings.TrimSpace(speech.Says) + "\n")
	if items := nonEmpty(speech.Proposals); len(items) > 0 {
		b.WriteString("\nProposes:\n")
		mdBullets(b, items)
	}
	if items := nonEmpty(speech.Concerns); len(items) > 0 {
		b.WriteString("\nWorried about:\n")
		mdBullets(b, items)
	}
	if len(speech.Objections) > 0 {
		b.WriteString("\nObjects:\n")
		for _, objection := range speech.Objections {
			fmt.Fprintf(b, "- **to %s**: %s\n  - Why: %s\n  - Wants: %s\n", objection.To, oneLine(objection.Point), oneLine(objection.Why), oneLine(objection.Wants))
		}
	}
}

// writeTopicMinutes writes the record of one topic: who said what, how the
// disputes went, where it stood, what was decided, what was written.
func (p *planner) writeTopicMinutes(plan *Plan, r *TopicRecord) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# Plan %s, topic %d: %s\n\n", plan.ID, r.Number, oneLine(r.Topic.Title))
	b.WriteString("> Part of the [minutes](../minutes.md) of the meeting, written by awkit from what the roles answered.\n\n")
	fmt.Fprintf(&b, "- It settles: %s\n- Lead: %s\n- Speaking after, in order: %s\n", oneLine(r.Topic.Question), r.Topic.Lead, strings.Join(r.Topic.Speakers, ", "))
	if len(r.Topic.Reads) > 0 {
		fmt.Fprintf(&b, "- Read first: %s\n", strings.Join(quoted(r.Topic.Reads), ", "))
	}
	fmt.Fprintf(&b, "- Ends in: %s\n", strings.Join(quoted(r.Topic.Writes), ", "))

	if r.Proposal != nil {
		role := p.roster.Role(r.Proposal.Role)
		fmt.Fprintf(&b, "\n## %s (%s) proposes\n\n", role.ID, role.Title)
		writeSpeech(&b, r.Proposal)
	}
	for i := range r.Speeches {
		role := p.roster.Role(r.Speeches[i].Role)
		fmt.Fprintf(&b, "\n## %s (%s)\n\n", role.ID, role.Title)
		writeSpeech(&b, &r.Speeches[i])
	}

	if len(r.Disputes) > 0 {
		b.WriteString("\n## Disputes\n")
		for i := range r.Disputes {
			dispute := &r.Disputes[i]
			state := "not settled"
			if dispute.Settled {
				state = "settled"
			}
			fmt.Fprintf(&b, "\n### %d. %s\n\n%s against %s: **%s**\n\nObjection: %s\n\n", i+1, oneLine(dispute.Topic),
				dispute.Challenger, dispute.Defender, state, oneLine(dispute.Point))
			for round, exchange := range dispute.Exchanges {
				fmt.Fprintf(&b, "- Round %d, %s (%s): %s\n", round+1, dispute.Defender, exchange.Stance, oneLine(exchange.Reply))
				if strings.TrimSpace(exchange.Position) != "" {
					fmt.Fprintf(&b, "  - Proposal now: %s\n", oneLine(exchange.Position))
				}
				if exchange.Answered {
					verdict := "does not accept"
					if exchange.Accepts {
						verdict = "accepts"
					}
					fmt.Fprintf(&b, "  - %s %s: %s\n", dispute.Challenger, verdict, oneLine(exchange.Answer))
					if !exchange.Accepts && strings.TrimSpace(exchange.StillWants) != "" {
						fmt.Fprintf(&b, "  - Still wants: %s\n", oneLine(exchange.StillWants))
					}
				}
			}
			if dispute.Outcome != "" {
				fmt.Fprintf(&b, "\nOutcome: %s\n", dispute.Outcome)
			}
		}
		if dropped := r.undisputed(); len(dropped) > 0 {
			b.WriteString("\nObjections that were not argued out:\n")
			mdBullets(&b, dropped)
		}
	}

	if len(r.Summary.Consensus)+len(r.Summary.Open)+len(r.Summary.Questions) > 0 {
		b.WriteString("\n## Where it stood\n\nBy the moderator, who takes no side.\n\nAgreed:\n")
		mdBullets(&b, r.Summary.Consensus)
		b.WriteString("\nStill disputed:\n")
		mdBullets(&b, r.Summary.Open)
		b.WriteString("\nFor the owner to answer:\n")
		mdBullets(&b, r.Summary.Questions)
	}

	if len(r.Decisions) > 0 {
		fmt.Fprintf(&b, "\n## Decisions\n\nBy %s. They are on record in `%s`.\n\n", p.roster.Decider().ID, DecisionsPath)
		writeDecisions(&b, r.Decisions, r.Assumptions)
	}

	if len(r.Documents) > 0 {
		b.WriteString("\n## Documents\n\n")
		writeDocuments(&b, p.roster, r.Documents)
		writeRuleChecks(&b, r.Rules, r.RulesLeft, r.RuleChecks)
		writeDataCheck(&b, r.DataCheck)
	}
	return p.writeFile(plan.Dir()+"/"+topicFile(r), b.String())
}

// writeDocuments says what became of documents.
func writeDocuments(b *strings.Builder, roster *Roster, documents []WrittenDocument) {
	for _, doc := range documents {
		owner := roster.Role(doc.Owner)
		switch {
		case doc.Refused != "" && owner != nil && owner.OwnsDocument(doc.Path):
			fmt.Fprintf(b, "- `%s` (%s): **not written**, %s\n", doc.Path, doc.Owner, doc.Refused)
		case doc.Refused != "":
			fmt.Fprintf(b, "- `%s` handed in by %s: **not written**, %s\n", doc.Path, doc.Owner, doc.Refused)
		case doc.Unchanged:
			fmt.Fprintf(b, "- `%s` (%s): left as it was\n", doc.Path, doc.Owner)
		default:
			fmt.Fprintf(b, "- `%s` (%s)\n", doc.Path, doc.Owner)
		}
	}
}

// writeRuleChecks puts on record which rules of the project the documents were
// written by, and what reading them against those rules came to: what was
// found, and what still stands for the person who approves the plan.
func writeRuleChecks(b *strings.Builder, ruleFiles, left []string, checks []RuleCheck) {
	if len(ruleFiles)+len(left) == 0 {
		return
	}
	b.WriteString("\n### By the project's rules\n")
	if len(ruleFiles) > 0 {
		fmt.Fprintf(b, "\nEach role was given the rules that hold for the files it writes: %s. What it wrote in this meeting was read against them by a reader who takes no side, and the owner put right what was found, once.\n", strings.Join(quoted(ruleFiles), ", "))
	}
	if len(left) > 0 {
		fmt.Fprintf(b, "\nNot passed on, because the rules together are longer than one question carries: %s. Nothing was read against them.\n", strings.Join(quoted(left), ", "))
	}
	if len(checks) == 0 {
		return
	}
	b.WriteString("\n")
	finding := func(label string, found RuleFinding) {
		fmt.Fprintf(b, "  - %s: `%s` against `%s`: %s\n", label, found.Document, found.Rule, oneLine(found.Point))
		if strings.TrimSpace(found.Fix) != "" {
			fmt.Fprintf(b, "    - To put right: %s\n", oneLine(found.Fix))
		}
	}
	for _, check := range checks {
		switch {
		case len(check.Found) == 0 && check.Settled:
			fmt.Fprintf(b, "- %s: nothing at odds with the rules\n", check.Owner)
		case len(check.Found) == 0:
			fmt.Fprintf(b, "- %s: **not read**; the meeting stopped first\n", check.Owner)
		case !check.Settled:
			fmt.Fprintf(b, "- %s: %d point(s) found; **the meeting stopped before they were put right and read again**\n", check.Owner, len(check.Found))
		case len(check.Open) == 0:
			fmt.Fprintf(b, "- %s: %d point(s) found and put right\n", check.Owner, len(check.Found))
		default:
			fmt.Fprintf(b, "- %s: %d point(s) found; **%d still stand(s)** after the owner's turn\n", check.Owner, len(check.Found), len(check.Open))
		}
		for _, found := range check.Found {
			finding("Found", found)
		}
		for _, open := range check.Open {
			finding("**Still stands**", open)
		}
	}
}

// writeDecisions writes decisions and assumptions the way the log holds them:
// each under its name, D7 or A3.
func writeDecisions(b *strings.Builder, decisions []Decision, assumptions []Assumption) {
	for _, decision := range decisions {
		fmt.Fprintf(b, "- **%s %s**: %s\n  - Why: %s\n", decision.ID, oneLine(firstNonEmpty(decision.Title, "decision")), oneLine(decision.Decision), oneLine(decision.Rationale))
		if strings.TrimSpace(decision.Dissent) != "" {
			fmt.Fprintf(b, "  - Held otherwise: %s\n", oneLine(decision.Dissent))
		}
	}
	if len(assumptions) == 0 {
		return
	}
	b.WriteString("\nAssumed until the owner of the project says otherwise:\n\n")
	for _, assumption := range assumptions {
		if strings.TrimSpace(assumption.Question) == "" {
			fmt.Fprintf(b, "- **%s** %s\n", assumption.ID, oneLine(assumption.Assumption))
		} else {
			fmt.Fprintf(b, "- **%s** %s\n  - Assumed: %s\n", assumption.ID, oneLine(assumption.Question), oneLine(assumption.Assumption))
		}
		if strings.TrimSpace(assumption.IfWrong) != "" {
			fmt.Fprintf(b, "  - If that is wrong: %s\n", oneLine(assumption.IfWrong))
		}
	}
}

const decisionsHeader = `# Decisions

What the project has decided, in the order it was decided. This file only
grows: a decision is changed by a later one that names it, never by editing
the earlier one. D7 names one decision and A3 one assumption for good.
`

func planMarker(id string) string {
	return "<!-- plan: " + id + " -->"
}

func topicMarker(plan, topic string) string {
	return "<!-- plan: " + plan + " topic: " + topic + " -->"
}

var (
	decisionName   = regexp.MustCompile(`(?m)^- \*\*D([0-9]+)\b`)
	assumptionName = regexp.MustCompile(`(?m)^- \*\*A([0-9]+)\b`)
)

// lastNumber returns the highest number a pattern names in a text.
func lastNumber(pattern *regexp.Regexp, text string) int {
	last := 0
	for _, match := range pattern.FindAllStringSubmatch(text, -1) {
		if n, err := strconv.Atoi(match[1]); err == nil && n > last {
			last = n
		}
	}
	return last
}

// number names the decisions and assumptions of a topic: each counts on from
// the last one on record before the topic, so that going on with a plan gives
// them the names they have in the log.
func (p *planner) number(plan *Plan, r *TopicRecord) {
	before := p.read(DecisionsPath)
	if at := strings.Index(before, topicMarker(plan.ID, r.Topic.ID)); at >= 0 {
		before = before[:at]
	}
	decision, assumption := lastNumber(decisionName, before), lastNumber(assumptionName, before)
	for i := range r.Decisions {
		r.Decisions[i].ID = fmt.Sprintf("D%d", decision+i+1)
	}
	for i := range r.Assumptions {
		r.Assumptions[i].ID = fmt.Sprintf("A%d", assumption+i+1)
	}
}

// recordDecisions appends the decisions of a topic to the decision log, under
// the plan's heading. A topic's decisions go in once: going on with a plan
// does not record them again.
func (p *planner) recordDecisions(plan *Plan, r *TopicRecord) error {
	file := filepath.Join(p.root, filepath.FromSlash(DecisionsPath))
	existing, err := os.ReadFile(file)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	log := string(existing)
	if strings.Contains(log, topicMarker(plan.ID, r.Topic.ID)) {
		return nil
	}

	var b strings.Builder
	if strings.TrimSpace(log) == "" {
		b.WriteString(decisionsHeader)
	} else {
		b.WriteString(strings.TrimRight(log, "\n") + "\n")
	}
	if !strings.Contains(log, planMarker(plan.ID)) {
		fmt.Fprintf(&b, "\n## %s, plan %s\n\n%s\n\n", p.today(), plan.ID, planMarker(plan.ID))
		fmt.Fprintf(&b, "Decided by %s, a topic at a time. At the table: %s. Minutes: `%s/minutes.md`.\n", p.roster.Decider().ID, strings.Join(p.roster.IDs(), ", "), plan.Dir())
	}
	fmt.Fprintf(&b, "\n### %s\n\n%s\n\n", oneLine(r.Topic.Title), topicMarker(plan.ID, r.Topic.ID))
	writeDecisions(&b, r.Decisions, r.Assumptions)
	return p.writeFile(DecisionsPath, b.String())
}

const (
	indexOpen  = "<!-- awk:documents: kept by awkit plan; edit outside these two lines -->"
	indexClose = "<!-- /awk:documents -->"
)

// fromIndex returns the way from the index to a file of the project, the way
// a link in the index writes it.
func fromIndex(file string) string {
	rel, err := filepath.Rel(filepath.FromSlash(path.Dir(indexPath)), filepath.FromSlash(file))
	if err != nil {
		return file
	}
	return filepath.ToSlash(rel)
}

// indexLink writes a link of the index to a file of the project, for a cell
// of a table.
func indexLink(label, file string) string {
	target := fromIndex(file)
	if strings.ContainsAny(target, " ()") {
		target = "<" + target + ">"
	}
	return strings.ReplaceAll("["+label+"]("+target+")", "|", "\\|")
}

// planIDs lists the plans of a project, the newest first.
// Plans names the plans of a project, the newest first.
func Plans(root string) []string { return planIDs(root) }

func planIDs(root string) []string {
	entries, _ := os.ReadDir(filepath.Join(root, filepath.FromSlash(PlansDir)))
	var ids []string
	for _, entry := range entries {
		if entry.IsDir() && planID.MatchString(entry.Name()) {
			ids = append(ids, entry.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(ids)))
	return ids
}

// writeIndex keeps the index of the project's documents in docs/README.md:
// which document there is, where, who owns it, and the plans. It points to the
// documents and copies nothing from them: how far a document is, the document
// says itself. Only the part between its two marker lines is touched.
func writeIndex(root string, roster *Roster) error {
	p := &planner{root: root, roster: roster}
	var b strings.Builder
	b.WriteString(indexOpen + "\n\n## Documents\n\nEach document has one owner: the role that writes it when the council plans (`" + layout.RosterFile + "`). How far a document is, and when it was last checked, it says itself: this index only points to it.\n\n| Document | Owner |\n|---|---|\n")
	escape := func(text string) string { return strings.ReplaceAll(text, "|", "\\|") }
	for i := range p.roster.Roles {
		role := &p.roster.Roles[i]
		docs := p.documentsOf(role)
		for _, doc := range docs {
			if !p.exists(doc) {
				fmt.Fprintf(&b, "| %s (not written yet) | %s |\n", escape(doc), role.ID)
				continue
			}
			fmt.Fprintf(&b, "| %s | %s |\n", indexLink(doc, doc), role.ID)
		}
		// A folder nothing is written in yet is named, so that it is known
		// whose it is.
		for _, folder := range role.Folders() {
			empty := true
			for _, doc := range docs {
				if strings.HasPrefix(doc, folder) && p.exists(doc) {
					empty = false
				}
			}
			if empty {
				fmt.Fprintf(&b, "| %s (nothing written yet) | %s |\n", escape(folder), role.ID)
			}
		}
	}
	fmt.Fprintf(&b, "| %s | the deciding role, through `awkit plan`; it only grows |\n", indexLink(DecisionsPath, DecisionsPath))

	b.WriteString("\n## Plans\n\n| Plan | Work list |\n|---|---|\n")
	for _, id := range planIDs(p.root) {
		list := "none yet"
		if _, err := os.Stat(workListPath(p.root, id)); err == nil {
			list = indexLink(WorkListFile, PlansDir+"/"+id+"/"+WorkListFile)
		}
		fmt.Fprintf(&b, "| %s | %s |\n", indexLink(id, PlansDir+"/"+id+"/minutes.md"), list)
	}
	b.WriteString("\n" + indexClose + "\n")
	section := b.String()

	existing := p.read(indexPath)
	var content string
	start, end := strings.Index(existing, indexOpen), strings.Index(existing, indexClose)
	switch {
	case strings.TrimSpace(existing) == "":
		content = "# Documents\n\nWhere the project's documents are and who answers for each.\n\n" + section
	case start >= 0 && end > start:
		content = existing[:start] + strings.TrimSuffix(section, "\n") + existing[end+len(indexClose):]
	default:
		content = strings.TrimRight(existing, "\n") + "\n\n" + section
	}
	return p.writeFile(indexPath, content)
}
