package council

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/layout"
	"github.com/silver2dream/ai-workflow-kit/internal/ticket"
)

// ApproveOptions is what turning a work list into issues needs.
type ApproveOptions struct {
	// Root is the project's root directory.
	Root string
	// ID names the plan whose work list is approved.
	ID string
	// Repos are the repos a ticket can be for.
	Repos []Repo
	// Roster, when given, lets the document index show what was created.
	Roster *Roster
	// DryRun reports what would be created and creates nothing.
	DryRun bool
	// Create makes one issue from a title and a body file, and returns its
	// number.
	Create func(ctx context.Context, title, bodyFile string) (int, error)
	// Progress is told about each ticket, a line at a time.
	Progress func(string)
}

// Approval is what an approval did.
type Approval struct {
	// Created are the tickets that became issues in this run, in the order
	// they were created. In a dry run: the ones that would.
	Created []Ticket
	// Existing are the tickets that already had an issue.
	Existing []Ticket
}

// Approve creates the issues of a plan's work list, as the list stands on
// disk: the person approving it may have changed it. Every ticket is created
// after the ones it depends on, so that its block can name their issues. The
// issue of each ticket is written back to the list as soon as it exists; a
// ticket that has one is not created again, so an approval that failed half
// way is simply run again.
func Approve(ctx context.Context, opts ApproveOptions) (*Approval, error) {
	if opts.Create == nil && !opts.DryRun {
		return nil, errors.New("approve: nothing to create issues with")
	}
	progress := opts.Progress
	if progress == nil {
		progress = func(string) {}
	}
	list, err := LoadWorkList(opts.Root, opts.ID)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("plan %s has no work list (%s/%s/%s): the meeting did not get that far", opts.ID, PlansDir, opts.ID, WorkListFile)
		}
		return nil, err
	}
	other := earlierIn(opts.Root)
	if err := checkTickets(list.Tickets, opts.Repos, 0, other); err != nil {
		return nil, fmt.Errorf("%s/%s/%s: %w", PlansDir, opts.ID, WorkListFile, err)
	}
	ordered, err := workOrder(list.Tickets)
	if err != nil {
		return nil, err
	}

	keys := map[string]bool{}
	issueOf := map[string]int{}
	for _, item := range list.Tickets {
		keys[item.Key] = true
		issueOf[item.Key] = item.Issue
	}
	setIssue := func(key string, issue int) {
		issueOf[key] = issue
		for i := range list.Tickets {
			if list.Tickets[i].Key == key {
				list.Tickets[i].Issue = issue
			}
		}
	}

	// A ticket that waits for a ticket of another plan needs that ticket's
	// issue in its block: the other plan is approved first. Found out before
	// anything is created.
	for _, item := range list.Tickets {
		if item.Issue != 0 {
			continue
		}
		for _, dependency := range item.DependsOn {
			plan, key, ok := otherPlan(dependency, keys)
			if !ok {
				continue
			}
			if issue, _ := other(plan, key); issue == 0 {
				return nil, fmt.Errorf("%s/%s/%s: ticket %s waits for %s, which is not on GitHub yet: approve that plan first (awkit plan approve %s)",
					PlansDir, opts.ID, WorkListFile, item.Key, dependency, plan)
			}
		}
	}

	approval := &Approval{}
	for _, item := range ordered {
		if item.Issue != 0 {
			approval.Existing = append(approval.Existing, item)
			progress(fmt.Sprintf("%s is issue #%d already: %s", item.Key, item.Issue, item.Title))
			continue
		}
		// What the ticket waits for: the issues the tickets of the list became
		// (they were created before it), and issues that existed already.
		var dependsOn []int
		for _, dependency := range item.DependsOn {
			if keys[dependency] {
				dependsOn = append(dependsOn, issueOf[dependency])
			} else if plan, key, ok := otherPlan(dependency, keys); ok {
				issue, _ := other(plan, key)
				dependsOn = append(dependsOn, issue)
			} else {
				dependsOn = append(dependsOn, existingIssue(dependency, keys))
			}
		}
		if opts.DryRun {
			approval.Created = append(approval.Created, item)
			after := ""
			if len(item.DependsOn) > 0 {
				after = " (after " + strings.Join(item.DependsOn, ", ") + ")"
			}
			progress(fmt.Sprintf("would create %s: %s, for %s%s", item.Key, item.Title, item.Repo, after))
			continue
		}

		body := ticketBody(item, opts.ID, dependsOn)
		bodyFile := filepath.Join(layout.Temp(opts.Root), fmt.Sprintf("plan-%s-%s.md", opts.ID, unsafeInName.ReplaceAllString(item.Key, "-")))
		if err := os.MkdirAll(filepath.Dir(bodyFile), 0o755); err != nil {
			return approval, err
		}
		if err := os.WriteFile(bodyFile, []byte(body), 0o644); err != nil {
			return approval, err
		}
		issue, err := opts.Create(ctx, item.Title, bodyFile)
		if err != nil {
			return approval, fmt.Errorf("ticket %s (%s) was not created: %w", item.Key, item.Title, err)
		}
		setIssue(item.Key, issue)
		// On record at once: whatever happens next, this ticket is not
		// created a second time.
		if err := SaveWorkList(opts.Root, list); err != nil {
			return approval, fmt.Errorf("issue #%d was created for ticket %s, but the work list could not be updated: %w", issue, item.Key, err)
		}
		_ = os.Remove(bodyFile)
		item.Issue = issue
		approval.Created = append(approval.Created, item)
		progress(fmt.Sprintf("created #%d for %s: %s", issue, item.Key, item.Title))
	}

	if opts.Roster != nil && !opts.DryRun {
		if err := writeIndex(opts.Root, opts.Roster); err != nil {
			return approval, err
		}
	}
	return approval, nil
}

// ticketBody writes the issue body of a ticket: the sections a ticket needs,
// what to read, and the block the workflow reads.
func ticketBody(item Ticket, planID string, dependsOn []int) string {
	var b strings.Builder
	b.WriteString("## Summary\n" + strings.TrimSpace(item.Summary) + "\n\n## Scope\n")
	for _, entry := range nonEmpty(item.Scope) {
		fmt.Fprintf(&b, "- %s\n", entry)
	}
	b.WriteString("\n## Acceptance Criteria\n")
	for _, criterion := range nonEmpty(item.Acceptance) {
		fmt.Fprintf(&b, "- [ ] %s\n", criterion)
	}
	b.WriteString("\n## Testing Requirements\n")
	testing := nonEmpty(item.Testing)
	if len(testing) == 0 {
		testing = []string{"New behaviour is covered by unit tests", "The existing tests still pass"}
	}
	for _, requirement := range testing {
		fmt.Fprintf(&b, "- %s\n", requirement)
	}
	b.WriteString("\n## References\n")
	for _, doc := range nonEmpty(item.Reads) {
		fmt.Fprintf(&b, "- `%s`\n", doc)
	}
	fmt.Fprintf(&b, "- `%s/%s/minutes.md`: the plan this ticket comes from, and `%s`\n\n", PlansDir, planID, DecisionsPath)
	b.WriteString(ticket.Render(ticket.Meta{Schema: ticket.Schema, Repo: item.Repo, DependsOn: dependsOn}))
	return b.String()
}
