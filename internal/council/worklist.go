package council

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/silver2dream/ai-workflow-kit/internal/ticket"
)

// Ticket is one piece of work the council proposes. It becomes a GitHub issue
// only when a person approves the work list.
type Ticket struct {
	// Key names the ticket within its list; depends_on refers to it.
	Key        string   `json:"key" yaml:"key"`
	Title      string   `json:"title" yaml:"title"`
	Repo       string   `json:"repo" yaml:"repo"`
	Summary    string   `json:"summary" yaml:"summary"`
	Scope      []string `json:"scope" yaml:"scope"`
	Acceptance []string `json:"acceptance" yaml:"acceptance"`
	Testing    []string `json:"testing" yaml:"testing"`
	Reads      []string `json:"reads" yaml:"reads,omitempty"`
	// DependsOn names what must be done first: a ticket of the list by its
	// key, an issue that exists already by its number ("#12"), or a ticket of
	// another plan that is only proposed ("<plan>/<key>").
	DependsOn []string `json:"depends_on" yaml:"depends_on,omitempty"`
	// Issue is the GitHub issue the ticket became; zero until it is created.
	Issue int `json:"-" yaml:"issue,omitempty"`
}

// Milestone is a step a plan is built in: something a person can see working
// once it is reached.
type Milestone struct {
	Key      string   `json:"key" yaml:"key"`
	Title    string   `json:"title" yaml:"title"`
	Goal     string   `json:"goal" yaml:"goal"`
	DoneWhen []string `json:"done_when" yaml:"done_when"`
}

// maxMilestones bounds the milestones of a plan.
const maxMilestones = 6

var milestoneKey = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// checkMilestones checks that a plan is laid out in milestones a person can
// check: at least one, each with a key of its own, a title, a goal and how it
// is seen to be reached.
func checkMilestones(milestones []Milestone) error {
	if len(milestones) == 0 {
		return errors.New("the plan has no milestone: lay it out in milestones, and ticket the first")
	}
	if len(milestones) > maxMilestones {
		return fmt.Errorf("the plan has %d milestones; at most %d (leave the rest for a later plan, in later)", len(milestones), maxMilestones)
	}
	keys := map[string]bool{}
	for i, milestone := range milestones {
		key := strings.TrimSpace(milestone.Key)
		switch {
		case !milestoneKey.MatchString(key):
			return fmt.Errorf("milestone %d: the key %q is not letters, digits, - and _", i+1, milestone.Key)
		case keys[key]:
			return fmt.Errorf("two milestones have the key %q", key)
		case strings.TrimSpace(milestone.Title) == "":
			return fmt.Errorf("milestone %s has no title", key)
		case strings.TrimSpace(milestone.Goal) == "":
			return fmt.Errorf("milestone %s has no goal", key)
		case len(nonEmpty(milestone.DoneWhen)) == 0:
			return fmt.Errorf("milestone %s does not say how it is seen to be reached: done_when is empty", key)
		}
		keys[key] = true
	}
	return nil
}

// WorkList is the file a plan ends with: docs/plans/<id>/tickets.yaml.
type WorkList struct {
	Plan string `yaml:"plan"`
	// Milestones are the steps the plan is built in; the tickets are those of
	// the first.
	Milestones []Milestone `yaml:"milestones,omitempty"`
	Tickets    []Ticket    `yaml:"tickets"`
	// Later is what the list leaves for another plan.
	Later []string `yaml:"later,omitempty"`
}

// WorkListFile is the name of the work list in a plan's folder.
const WorkListFile = "tickets.yaml"

var ticketTitle = regexp.MustCompile(`^\[(feat|fix|refactor|test|docs|chore|perf|style)\] \S`)

// blockIntact reports whether the issue body of a ticket reads back as awkit
// wrote it: one awk-ticket block, the one awkit put there, for the ticket's
// repo and with nothing switched on. What a ticket says is written by a role
// or by a person; the block decides what an unattended Worker may do, and
// only awkit writes it.
func blockIntact(item Ticket) bool {
	meta, found, err := ticket.Parse(ticketBody(item, "plan", nil))
	if err != nil || !found {
		return false
	}
	return strings.EqualFold(meta.Repo, item.Repo) && len(meta.DependsOn) == 0 && meta.Severity == "" && !meta.Release &&
		!meta.AllowParentChanges && !meta.AllowScriptChanges && !meta.AllowSecrets
}

// issueRef is how a ticket names an issue that exists already, the ticket of
// an earlier plan: "#12", or a bare 12 in a list a person edited.
var issueRef = regexp.MustCompile(`^#?([1-9][0-9]*)$`)

// existingIssue returns the issue a dependency names, or 0 when it names a
// ticket of the list (keys) or nothing that can be an issue.
func existingIssue(dependency string, keys map[string]bool) int {
	if keys[dependency] {
		return 0
	}
	match := issueRef.FindStringSubmatch(dependency)
	if match == nil {
		return 0
	}
	number, err := strconv.Atoi(match[1])
	if err != nil {
		return 0
	}
	return number
}

// planRef is how a ticket names a ticket of another plan that has no issue
// yet: "<plan>/<key>".
var planRef = regexp.MustCompile(`^([a-z0-9][a-z0-9-]*)/(.+)$`)

// otherPlan reads a dependency as a ticket of another plan. It is none when
// the dependency is a key of the list or the number of an issue.
func otherPlan(dependency string, keys map[string]bool) (plan, key string, ok bool) {
	if keys[dependency] || existingIssue(dependency, keys) != 0 {
		return "", "", false
	}
	ref := planRef.FindStringSubmatch(dependency)
	if ref == nil {
		return "", "", false
	}
	return ref[1], ref[2], true
}

// earlier tells what became of a ticket of another plan: its issue, zero
// while the ticket is only proposed, and whether there is such a ticket.
type earlier func(plan, key string) (issue int, found bool)

// earlierIn answers from the work lists of a project's plans as they stand on
// disk, reading each once.
func earlierIn(root string) earlier {
	lists := map[string]*WorkList{}
	return func(plan, key string) (int, bool) {
		list, read := lists[plan]
		if !read {
			list, _ = LoadWorkList(root, plan)
			lists[plan] = list
		}
		if list == nil {
			return 0, false
		}
		for _, item := range list.Tickets {
			if item.Key == key {
				return item.Issue, true
			}
		}
		return 0, false
	}
}

// checkTickets checks that a work list can become issues: every ticket is for
// a repo of the project, says what it is for and when it is done, and the
// order the list asks for can be kept. other tells which tickets the other
// plans of the project have; nil when there are none.
func checkTickets(tickets []Ticket, repos []Repo, max int, other earlier) error {
	if len(tickets) == 0 {
		return errors.New("the list has no tickets")
	}
	if max > 0 && len(tickets) > max {
		return fmt.Errorf("the list has %d tickets; at most %d (name the rest in later)", len(tickets), max)
	}
	known := map[string]bool{}
	for _, repo := range repos {
		known[strings.ToLower(repo.Name)] = true
	}
	names := make([]string, len(repos))
	for i, repo := range repos {
		names[i] = repo.Name
	}
	keys := map[string]bool{}
	for _, ticket := range tickets {
		key := strings.TrimSpace(ticket.Key)
		switch {
		case key == "":
			return fmt.Errorf("the ticket %q has no key", ticket.Title)
		case keys[key]:
			return fmt.Errorf("two tickets have the key %q", key)
		}
		keys[key] = true
		if !ticketTitle.MatchString(ticket.Title) {
			return fmt.Errorf("ticket %s: the title %q is not \"[type] subject\" with a type of feat, fix, refactor, test, docs or chore", key, ticket.Title)
		}
		if !known[strings.ToLower(ticket.Repo)] {
			return fmt.Errorf("ticket %s is for repo %q; the repos are: %s", key, ticket.Repo, strings.Join(names, ", "))
		}
		if strings.TrimSpace(ticket.Summary) == "" {
			return fmt.Errorf("ticket %s has no summary", key)
		}
		if len(nonEmpty(ticket.Scope)) == 0 {
			return fmt.Errorf("ticket %s has no scope", key)
		}
		if len(nonEmpty(ticket.Acceptance)) == 0 {
			return fmt.Errorf("ticket %s has no acceptance criteria", key)
		}
		if !blockIntact(ticket) {
			return fmt.Errorf("ticket %s: its text gets in the way of the awk-ticket block awkit writes under it; take out the block it has of its own, or close the code fence it leaves open", key)
		}
	}
	for _, ticket := range tickets {
		for _, dependency := range ticket.DependsOn {
			switch {
			case dependency == ticket.Key:
				return fmt.Errorf("ticket %s depends on itself", ticket.Key)
			case keys[dependency], existingIssue(dependency, keys) != 0:
				continue
			}
			plan, key, ok := otherPlan(dependency, keys)
			if !ok {
				return fmt.Errorf("ticket %s depends on %q, which is neither a ticket of the list nor the number of an issue nor a ticket of another plan (\"<plan>/<key>\")", ticket.Key, dependency)
			}
			if other == nil {
				return fmt.Errorf("ticket %s depends on %q: there is no plan %s", ticket.Key, dependency, plan)
			}
			if _, found := other(plan, key); !found {
				return fmt.Errorf("ticket %s depends on %q: plan %s has no ticket %s", ticket.Key, dependency, plan, key)
			}
		}
	}
	if _, err := workOrder(tickets); err != nil {
		return err
	}
	return nil
}

func nonEmpty(items []string) []string {
	var out []string
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			out = append(out, strings.TrimSpace(item))
		}
	}
	return out
}

// workOrder returns the tickets in an order in which every ticket comes after
// the ones of the list it depends on, keeping the order of the list wherever
// it can. It fails when the dependencies go round in a circle. An issue that
// exists already is not waited for: it has its place in the queue.
func workOrder(tickets []Ticket) ([]Ticket, error) {
	keys := map[string]bool{}
	for _, ticket := range tickets {
		keys[ticket.Key] = true
	}
	done := map[string]bool{}
	var ordered []Ticket
	for len(ordered) < len(tickets) {
		progressed := false
		for _, ticket := range tickets {
			if done[ticket.Key] {
				continue
			}
			ready := true
			for _, dependency := range ticket.DependsOn {
				if keys[dependency] && !done[dependency] {
					ready = false
				}
			}
			if ready {
				done[ticket.Key] = true
				ordered = append(ordered, ticket)
				progressed = true
			}
		}
		if !progressed {
			var stuck []string
			for _, ticket := range tickets {
				if !done[ticket.Key] {
					stuck = append(stuck, ticket.Key)
				}
			}
			return nil, fmt.Errorf("the tickets %s depend on each other in a circle", strings.Join(stuck, ", "))
		}
	}
	return ordered, nil
}

// worklist asks the planning role for the milestones and the tickets of the
// first, lets the table object to the list once, and has the planning role
// take the objections in.
func (p *planner) worklist(ctx context.Context, plan *Plan) error {
	planner := p.roster.Planner()
	p.session.say("Work list: %s lays out the milestones and tickets the first", planner.ID)
	type list struct {
		Milestones []Milestone `json:"milestones"`
		Tickets    []Ticket    `json:"tickets"`
		Later      []string    `json:"later"`
		Kept       []string    `json:"kept"`
	}
	other := earlierIn(p.root)
	check := func(got *list) error {
		if err := checkMilestones(got.Milestones); err != nil {
			return err
		}
		return checkTickets(got.Tickets, p.repos, p.maxTickets, other)
	}

	draftStep, revisedStep := "work list "+planner.ID, "work list revised "+planner.ID
	p.topic = 0
	if err := p.point(ctx, plan, "work list", draftStep, planner.ID); err != nil {
		return err
	}
	// What the owner says from here on was not known to the draft: the list
	// is laid out again from the plan as it then holds.
	revisionsBefore := len(plan.Revisions)
	draft, err := settleJSON(ctx, p.session, draftStep, planner.Voice(), p.worklistPrompt(planner, plan), check)
	if err != nil {
		return err
	}
	asked := p.session.wasAsked(draftStep)

	// The table objects to the list, once.
	var reviewers []*Role
	for i := range p.roster.Roles {
		if p.roster.Roles[i].ID != planner.ID {
			reviewers = append(reviewers, &p.roster.Roles[i])
		}
	}
	type review struct {
		Objections []ListObjection `json:"objections"`
	}
	keys := map[string]bool{"": true}
	for _, ticket := range draft.Tickets {
		keys[ticket.Key] = true
	}
	for _, milestone := range draft.Milestones {
		keys[strings.TrimSpace(milestone.Key)] = true
	}
	if len(reviewers) > 0 {
		if err := p.point(ctx, plan, "work list review", "work list review "+reviewers[0].ID, ""); err != nil {
			return err
		}
	}
	perRole := make([][]ListObjection, len(reviewers))
	err = p.session.each(len(reviewers), func(i int) error {
		role := reviewers[i]
		got, err := settleJSON(ctx, p.session, "work list review "+role.ID, role.Voice(), p.worklistReviewPrompt(role, plan, draft.Milestones, draft.Tickets), func(got *review) error {
			for _, objection := range got.Objections {
				// One objection may be about several tickets: "t5, t7".
				for _, named := range strings.Split(objection.Ticket, ",") {
					if !keys[strings.TrimSpace(named)] {
						return fmt.Errorf("an objection names %q, which is neither a ticket nor a milestone of the list", strings.TrimSpace(named))
					}
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
			var named []string
			for _, key := range strings.Split(objection.Ticket, ",") {
				if key = strings.TrimSpace(key); key != "" {
					named = append(named, key)
				}
			}
			objection.Ticket = strings.Join(named, ", ")
			perRole[i] = append(perRole[i], objection)
		}
		return nil
	})
	if err != nil {
		return err
	}
	plan.ListObjections = nil
	for _, objections := range perRole {
		plan.ListObjections = append(plan.ListObjections, objections...)
	}
	for _, role := range reviewers {
		asked = asked || p.session.wasAsked("work list review "+role.ID)
	}

	final := draft
	if len(plan.ListObjections) > 0 {
		if err := p.point(ctx, plan, "work list revision", revisedStep, planner.ID); err != nil {
			return err
		}
		p.session.say("  %d objections to the list; %s revises it", len(plan.ListObjections), planner.ID)
		final, err = settleJSON(ctx, p.session, revisedStep, planner.Voice(),
			p.worklistRevisionPrompt(planner, plan, draft.Milestones, draft.Tickets, plan.ListObjections), check)
		if err != nil {
			return err
		}
		asked = asked || p.session.wasAsked(revisedStep)
	}

	// The owner's word reaches the list too: what was said after the draft
	// was laid out is taken in now, also when the meeting had finished and
	// goes on only for it, and the list is laid out again from the plan as
	// it then holds.
	if err := p.takeWords(ctx, plan, "after the work list", true); err != nil {
		return err
	}
	if len(plan.Revisions) > revisionsBefore {
		since := plan.Revisions[revisionsBefore:]
		var words []OwnerWord
		for _, review := range since {
			words = append(words, review.Words...)
		}
		step := "work list after the owner " + wordIDs(words) + " " + planner.ID
		p.session.say("  the owner said more after the list was laid out; %s lays it out again", planner.ID)
		final, err = settleJSON(ctx, p.session, step, planner.Voice(), p.worklistOwnerPrompt(planner, plan, final.Milestones, final.Tickets, since), check)
		if err != nil {
			return err
		}
		asked = asked || p.session.wasAsked(step)
	}

	ordered, err := workOrder(final.Tickets)
	if err != nil {
		return err
	}
	for i := range ordered {
		ordered[i].Key = strings.TrimSpace(ordered[i].Key)
		ordered[i].Scope, ordered[i].Acceptance = nonEmpty(ordered[i].Scope), nonEmpty(ordered[i].Acceptance)
		ordered[i].Testing, ordered[i].Reads = nonEmpty(ordered[i].Testing), nonEmpty(ordered[i].Reads)
	}
	milestones := make([]Milestone, len(final.Milestones))
	for i, milestone := range final.Milestones {
		milestones[i] = Milestone{Key: strings.TrimSpace(milestone.Key), Title: strings.TrimSpace(milestone.Title),
			Goal: strings.TrimSpace(milestone.Goal), DoneWhen: nonEmpty(milestone.DoneWhen)}
	}
	plan.Milestones, plan.Tickets, plan.Later, plan.Kept = milestones, ordered, nonEmpty(final.Later), nonEmpty(final.Kept)

	existing, err := LoadWorkList(p.root, plan.ID)
	switch {
	case os.IsNotExist(err):
	case err != nil:
		// Not written over: it may hold the issues that were created from it.
		return fmt.Errorf("the work list that is there cannot be read; correct it or remove it: %w", err)
	case !asked:
		// The table said nothing new in this run, and the list that is there
		// is the person's to change: it stays as they left it.
		plan.Milestones, plan.Tickets, plan.Later = existing.Milestones, existing.Tickets, existing.Later
		p.session.say("  the work list stays as it is: %d tickets", len(plan.Tickets))
		return nil
	default:
		// The issues that were created from the list stay, whatever the
		// meeting says the second time.
		issues := map[string]int{}
		for _, ticket := range existing.Tickets {
			issues[ticket.Key] = ticket.Issue
		}
		for i := range plan.Tickets {
			plan.Tickets[i].Issue = issues[plan.Tickets[i].Key]
		}
	}
	p.session.say("  %d milestones; %d tickets proposed for the first", len(plan.Milestones), len(plan.Tickets))
	saved := &WorkList{Plan: plan.ID, Milestones: plan.Milestones, Tickets: plan.Tickets, Later: plan.Later}
	if asked {
		p.tell(Entry{Step: "work list", Kind: EntryWorkList, Role: planner.ID, Data: saved})
	}
	return SaveWorkList(p.root, saved)
}

// Open names the plans of a project whose work list still has tickets that
// are not on GitHub, the newest first, each with how many of how many: what
// there is to approve.
func Open(root string) []string {
	var out []string
	for _, id := range planIDs(root) {
		list, err := LoadWorkList(root, id)
		if err != nil {
			continue
		}
		proposed := 0
		for _, item := range list.Tickets {
			if item.Issue == 0 {
				proposed++
			}
		}
		if proposed > 0 {
			out = append(out, fmt.Sprintf("%s (%d of %d tickets)", id, proposed, len(list.Tickets)))
		}
	}
	return out
}

// earlierTickets lists what the other plans of the project ticketed, the
// newest plan first, so that the planning role does not ticket the same work
// twice and can build on what is there.
func (p *planner) earlierTickets(current string) string {
	const plans, tickets = 6, 60
	var b strings.Builder
	shownPlans, shownTickets := 0, 0
	for _, id := range planIDs(p.root) {
		if id == current {
			continue
		}
		list, err := LoadWorkList(p.root, id)
		if err != nil || len(list.Tickets) == 0 {
			continue
		}
		if shownPlans == plans || shownTickets >= tickets {
			b.WriteString("(older plans left out)\n")
			break
		}
		shownPlans++
		fmt.Fprintf(&b, "Plan %s:\n", id)
		for _, ticket := range list.Tickets {
			// Each line starts with how a later ticket names this one.
			where := fmt.Sprintf("%s/%s (proposed, not on GitHub)", id, ticket.Key)
			if ticket.Issue != 0 {
				where = fmt.Sprintf("#%d", ticket.Issue)
			}
			fmt.Fprintf(&b, "- %s: %s (repo: %s)\n", where, oneLine(ticket.Title), ticket.Repo)
			shownTickets++
		}
	}
	return b.String()
}

const workListHeader = `# The work list the council proposes. Nothing here is on GitHub yet.
#
# The plan is laid out in milestones; the tickets are those of the first.
# The other milestones are planned when the first is reached.
#
# Read it, change what you want changed (titles, scope, order, depends_on;
# delete a ticket you do not want), then create the issues:
#
#   awkit plan approve %s
#
# depends_on names a ticket of this list by its key, an issue that exists
# already by its number, or a ticket of another plan as <plan>/<key> (that
# plan is then approved first). "issue" is filled in as each ticket is
# created; a ticket that has one is not created again.
`

func workListPath(root, id string) string {
	return filepath.Join(root, filepath.FromSlash(PlansDir), id, WorkListFile)
}

// SaveWorkList writes a plan's work list.
func SaveWorkList(root string, list *WorkList) error {
	body, err := yaml.Marshal(list)
	if err != nil {
		return err
	}
	file := workListPath(root, list.Plan)
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	return os.WriteFile(file, []byte(fmt.Sprintf(workListHeader, list.Plan)+"\n"+string(body)), 0o644)
}

// LoadWorkList reads a plan's work list as it stands: the person approving it
// may have changed it.
func LoadWorkList(root, id string) (*WorkList, error) {
	if !planID.MatchString(id) {
		return nil, fmt.Errorf("%q is not the name of a plan", id)
	}
	data, err := os.ReadFile(workListPath(root, id))
	if err != nil {
		return nil, err
	}
	var list WorkList
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&list); err != nil {
		return nil, fmt.Errorf("%s/%s/%s cannot be read: %w", PlansDir, id, WorkListFile, err)
	}
	list.Plan = id
	return &list, nil
}
