package council

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// What a meeting leaves on disk besides the roles' own documents: its minutes,
// its decisions in the decision log, and the index of the documents.

func mdBullets(b *strings.Builder, items []string) {
	if len(items) == 0 {
		b.WriteString("- (none)\n")
		return
	}
	for _, item := range items {
		fmt.Fprintf(b, "- %s\n", oneLine(item))
	}
}

// writeMinutes writes the record of the meeting: who said what. It is written
// from the roles' answers as they came, also when the meeting stopped early.
func (p *planner) writeMinutes(plan *Plan, stopped error) error {
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
	if stopped != nil {
		fmt.Fprintf(&b, "- Status: **not finished**. %s. Go on with `awkit plan --resume %s`.\n", oneLine(stopped.Error()), plan.ID)
	} else {
		fmt.Fprintf(&b, "- Status: finished. %d tickets proposed in `%s/%s`; none is on GitHub until `awkit plan approve %s`.\n",
			len(plan.Tickets), plan.Dir(), WorkListFile, plan.ID)
	}

	b.WriteString("\n## Brief\n\n" + strings.TrimSpace(plan.Brief) + "\n")

	if len(plan.Positions) > 0 {
		b.WriteString("\n## Positions\n")
		for _, position := range plan.Positions {
			role := p.roster.Role(position.Role)
			fmt.Fprintf(&b, "\n### %s (%s)\n\n%s\n\nProposes:\n", role.ID, role.Title, strings.TrimSpace(position.Summary))
			mdBullets(&b, position.Proposals)
			b.WriteString("\nWorried about:\n")
			mdBullets(&b, position.Concerns)
			if len(nonEmpty(position.Questions)) > 0 {
				b.WriteString("\nAsks the owner:\n")
				mdBullets(&b, position.Questions)
			}
		}
	}

	if len(plan.Answers) > 0 {
		b.WriteString("\n## What the owner was asked\n\nThe table's questions, put to the owner of the project before the roles objected to each other.\n\n")
		for _, answer := range plan.Answers {
			said := "(left open: the council assumes)"
			if answer.Answer != "" {
				said = oneLine(answer.Answer)
			}
			fmt.Fprintf(&b, "- %s\n  - %s\n", oneLine(answer.Question), said)
		}
	}

	if len(plan.Objections) > 0 {
		b.WriteString("\n## Objections\n\n")
		for _, objection := range plan.Objections {
			fmt.Fprintf(&b, "- **%s to %s**: %s\n  - Why: %s\n  - Wants: %s\n", objection.From, objection.To,
				oneLine(objection.Point), oneLine(objection.Why), oneLine(objection.Wants))
		}
	}

	if len(plan.Disputes) > 0 {
		b.WriteString("\n## Disputes\n")
		for i := range plan.Disputes {
			dispute := &plan.Disputes[i]
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
		if dropped := plan.undisputed(); len(dropped) > 0 {
			b.WriteString("\nObjections that were not argued out:\n")
			mdBullets(&b, dropped)
		}
	}

	if len(plan.Summary.Consensus)+len(plan.Summary.Open)+len(plan.Summary.Questions) > 0 {
		b.WriteString("\n## Where the meeting stood\n\nBy the moderator, who takes no side.\n\nAgreed:\n")
		mdBullets(&b, plan.Summary.Consensus)
		b.WriteString("\nStill disputed:\n")
		mdBullets(&b, plan.Summary.Open)
		b.WriteString("\nFor the owner to answer:\n")
		mdBullets(&b, plan.Summary.Questions)
	}

	if len(plan.Decisions) > 0 {
		fmt.Fprintf(&b, "\n## Decisions\n\nBy %s. They are on record in `%s`.\n\n", p.roster.Decider().ID, DecisionsPath)
		writeDecisions(&b, plan.Decisions, plan.Assumptions)
	}

	if len(plan.Documents) > 0 {
		b.WriteString("\n## Documents\n\n")
		for _, doc := range plan.Documents {
			switch {
			case doc.Refused != "" && p.roster.Role(doc.Owner).OwnsDocument(doc.Path):
				fmt.Fprintf(&b, "- `%s` (%s): **not written**, %s\n", doc.Path, doc.Owner, doc.Refused)
			case doc.Refused != "":
				fmt.Fprintf(&b, "- `%s` handed in by %s: **not written**, %s\n", doc.Path, doc.Owner, doc.Refused)
			case doc.Unchanged:
				fmt.Fprintf(&b, "- `%s` (%s): left as it was\n", doc.Path, doc.Owner)
			default:
				fmt.Fprintf(&b, "- `%s` (%s)\n", doc.Path, doc.Owner)
			}
		}
	}

	if len(plan.Tickets) > 0 {
		fmt.Fprintf(&b, "\n## Work list\n\nProposed by %s, in `%s/%s`.\n\n| Key | Ticket | Repo | After |\n|---|---|---|---|\n",
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

// number names the plan's decisions and assumptions: each counts on from the
// last one on record before the plan, so that going on with a plan gives them
// the names they have in the log.
func (p *planner) number(plan *Plan) {
	before := p.read(DecisionsPath)
	if at := strings.Index(before, planMarker(plan.ID)); at >= 0 {
		before = before[:at]
	}
	decision, assumption := lastNumber(decisionName, before), lastNumber(assumptionName, before)
	for i := range plan.Decisions {
		plan.Decisions[i].ID = fmt.Sprintf("D%d", decision+i+1)
	}
	for i := range plan.Assumptions {
		plan.Assumptions[i].ID = fmt.Sprintf("A%d", assumption+i+1)
	}
}

// recordDecisions appends the plan's decisions to the decision log. A plan's
// decisions go in once: going on with a plan does not record them again.
func (p *planner) recordDecisions(plan *Plan) error {
	file := filepath.Join(p.root, filepath.FromSlash(DecisionsPath))
	existing, err := os.ReadFile(file)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	marker := planMarker(plan.ID)
	if strings.Contains(string(existing), marker) {
		return nil
	}

	var b strings.Builder
	if len(existing) == 0 {
		b.WriteString(decisionsHeader)
	} else {
		b.WriteString(strings.TrimRight(string(existing), "\n") + "\n")
	}
	fmt.Fprintf(&b, "\n## %s, plan %s\n\n%s\n\n", p.today(), plan.ID, marker)
	fmt.Fprintf(&b, "Decided by %s. At the table: %s. Minutes: `%s/minutes.md`.\n\n", p.roster.Decider().ID, strings.Join(p.roster.IDs(), ", "), plan.Dir())
	writeDecisions(&b, plan.Decisions, plan.Assumptions)
	return p.writeFile(DecisionsPath, b.String())
}

const (
	indexOpen  = "<!-- awk:documents: kept by awkit plan; edit outside these two lines -->"
	indexClose = "<!-- /awk:documents -->"
	indexPath  = "docs/README.md"
)

var statusLine = regexp.MustCompile(`(?m)^status:[ \t]*(\S+)`)

// planIDs lists the plans of a project, the newest first.
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
// which document there is, who owns it, how far it is, and the plans. Only the
// part between its two marker lines is touched.
func writeIndex(root string, roster *Roster) error {
	p := &planner{root: root, roster: roster}
	var b strings.Builder
	b.WriteString(indexOpen + "\n\n## Documents\n\nEach document has one owner: the role that writes it when the council plans (`.ai/roster.yaml`).\n\n| Document | Owner | Status |\n|---|---|---|\n")
	for _, role := range p.roster.Roles {
		for _, doc := range role.Owns {
			status := "not written"
			if content := p.read(doc); strings.TrimSpace(content) != "" {
				status = "written"
				if match := statusLine.FindStringSubmatch(content); match != nil {
					status = match[1]
				}
			}
			fmt.Fprintf(&b, "| [%s](%s) | %s | %s |\n", doc, strings.TrimPrefix(doc, "docs/"), role.ID, status)
		}
	}
	fmt.Fprintf(&b, "| [%s](%s) | the deciding role, through `awkit plan` | append-only |\n", DecisionsPath, strings.TrimPrefix(DecisionsPath, "docs/"))

	b.WriteString("\n## Plans\n\n| Plan | Work list |\n|---|---|\n")
	for _, id := range planIDs(p.root) {
		state := "no work list yet"
		if list, err := LoadWorkList(p.root, id); err == nil {
			created := 0
			for _, ticket := range list.Tickets {
				if ticket.Issue != 0 {
					created++
				}
			}
			state = fmt.Sprintf("%d tickets, %d on GitHub", len(list.Tickets), created)
		}
		fmt.Fprintf(&b, "| [%s](plans/%s/minutes.md) | %s |\n", id, id, state)
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
