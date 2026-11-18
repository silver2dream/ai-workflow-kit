package council

import (
	"fmt"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/config"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
	"github.com/silver2dream/ai-workflow-kit/internal/ticket"
)

// A person reads the work list before approving it, and may change what a
// ticket says or drop a ticket. Each change is checked as approving the list
// checks it, and kept only when the list still passes: what is on disk can
// always be approved. A ticket that is on GitHub already is changed there.

// TicketText is what a person may change in a ticket: what it says. Its key,
// its repo and what it waits for stay as the council laid them out.
type TicketText struct {
	Title      string   `json:"title"`
	Summary    string   `json:"summary"`
	Scope      []string `json:"scope"`
	Acceptance []string `json:"acceptance"`
	Testing    []string `json:"testing"`
}

// CheckWorkList checks a plan's work list as approving it would.
func CheckWorkList(root, id string, repos []Repo) error {
	list, err := LoadWorkList(root, id)
	if err != nil {
		return err
	}
	return checkTickets(list.Tickets, repos, 0, earlierIn(root))
}

// findTicket returns the index of a ticket of the list by its key, and why
// it cannot be changed: it is not there, or it is on GitHub already.
func findTicket(list *WorkList, key string) (int, error) {
	for i := range list.Tickets {
		if list.Tickets[i].Key == key {
			if issue := list.Tickets[i].Issue; issue != 0 {
				return i, fmt.Errorf("ticket %s is issue #%d on GitHub already: change it there", key, issue)
			}
			return i, nil
		}
	}
	return -1, fmt.Errorf("the work list of plan %s has no ticket %s", list.Plan, key)
}

// EditTicket changes what a ticket of a plan's work list says, and keeps the
// list when it still passes the check of an approval.
func EditTicket(root, id, key string, text TicketText, repos []Repo) (*WorkList, error) {
	list, err := LoadWorkList(root, id)
	if err != nil {
		return nil, err
	}
	at, err := findTicket(list, key)
	if err != nil {
		return nil, err
	}
	item := &list.Tickets[at]
	item.Title, item.Summary = strings.TrimSpace(text.Title), strings.TrimSpace(text.Summary)
	item.Scope, item.Acceptance, item.Testing = nonEmpty(text.Scope), nonEmpty(text.Acceptance), nonEmpty(text.Testing)
	if err := checkTickets(list.Tickets, repos, 0, earlierIn(root)); err != nil {
		return nil, err
	}
	if err := SaveWorkList(root, list); err != nil {
		return nil, err
	}
	return list, nil
}

// DeleteTicket drops a ticket from a plan's work list. A ticket another one
// waits for stays until that one no longer does.
func DeleteTicket(root, id, key string, repos []Repo) (*WorkList, error) {
	list, err := LoadWorkList(root, id)
	if err != nil {
		return nil, err
	}
	at, err := findTicket(list, key)
	if err != nil {
		return nil, err
	}
	var waiting []string
	for _, item := range list.Tickets {
		for _, dependency := range item.DependsOn {
			if dependency == key {
				waiting = append(waiting, item.Key)
			}
		}
	}
	if len(waiting) > 0 {
		return nil, fmt.Errorf("ticket %s waits for %s: drop %s first", strings.Join(waiting, ", "), key, strings.Join(waiting, ", "))
	}
	list.Tickets = append(list.Tickets[:at], list.Tickets[at+1:]...)
	if len(list.Tickets) > 0 {
		if err := checkTickets(list.Tickets, repos, 0, earlierIn(root)); err != nil {
			return nil, err
		}
	}
	if err := SaveWorkList(root, list); err != nil {
		return nil, err
	}
	return list, nil
}

// ProjectRepos lists the repos a ticket of the project in root can be for:
// the ones of its configuration, and the repository root.
func ProjectRepos(root string) ([]Repo, error) {
	cfg, err := config.LoadFile(config.Path(root))
	if err != nil {
		return nil, fmt.Errorf("the project's config cannot be read (%s): %w\nRun `awkit init` first", layout.ConfigFile, err)
	}
	var repos []Repo
	for _, name := range cfg.TicketRepos() {
		entry := Repo{Name: name}
		if configured := cfg.FindRepo(name); configured != nil {
			entry.Language, entry.Path = configured.Language, configured.Path
		} else if name == ticket.RootRepo {
			entry.Path = "./"
		}
		repos = append(repos, entry)
	}
	return repos, nil
}
