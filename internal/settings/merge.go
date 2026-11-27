package settings

import (
	"fmt"
	"reflect"
	"strings"
)

// A change is made to the settings as the page showed them (was), and lands
// on the settings as the integration branch has them (theirs), which may
// have changed meanwhile: on GitHub, or here and pushed. What the person
// changed is taken from the change; everything else from the branch. So a
// change of the test command does not put back a merge strategy someone
// else changed.

// Conflict says the person and the integration branch both changed the same
// thing, differently: nothing is changed.
type Conflict struct {
	What string
}

func (c *Conflict) Error() string {
	return fmt.Sprintf("%s was changed on the integration branch too, since the page was opened: open the settings again and make the change to them as they are now", c.What)
}

func pick[T comparable](was, mine, theirs T) T {
	if mine != was {
		return mine
	}
	return theirs
}

func pickInt(was, mine, theirs *int) *int {
	value := func(n *int) int {
		if n == nil || *n <= 0 {
			return 0
		}
		return *n
	}
	if value(mine) != value(was) {
		return mine
	}
	return theirs
}

func pickFloat(was, mine, theirs *float64) *float64 {
	value := func(n *float64) float64 {
		if n == nil {
			return 0
		}
		return *n
	}
	if value(mine) != value(was) {
		return mine
	}
	return theirs
}

// pickRole takes each setting of an agent the person changed, and the
// others as the branch has them.
func pickRole(was, mine, theirs AgentRole) AgentRole {
	return AgentRole{
		Backend: pick(was.Backend, mine.Backend, theirs.Backend),
		Model:   pick(was.Model, mine.Model, theirs.Model),
		Effort:  pick(was.Effort, mine.Effort, theirs.Effort),
		Minutes: pickInt(was.Minutes, mine.Minutes, theirs.Minutes),
		USD:     pickFloat(was.USD, mine.USD, theirs.USD),
	}
}

// mergeRoster returns the roster on the branch with what the person changed.
// The roles are one thing: changed on both sides, differently, they are a
// conflict.
func mergeRoster(was, mine, theirs *Roster) (*Roster, error) {
	merged := *theirs
	merged.Language = pick(was.Language, mine.Language, theirs.Language)
	merged.Model = pick(was.Model, mine.Model, theirs.Model)
	merged.Effort = pick(was.Effort, mine.Effort, theirs.Effort)
	merged.Limits = Limits{
		Topics:         pickInt(was.Limits.Topics, mine.Limits.Topics, theirs.Limits.Topics),
		Disputes:       pickInt(was.Limits.Disputes, mine.Limits.Disputes, theirs.Limits.Disputes),
		DisputeRounds:  pickInt(was.Limits.DisputeRounds, mine.Limits.DisputeRounds, theirs.Limits.DisputeRounds),
		Calls:          pickInt(was.Limits.Calls, mine.Limits.Calls, theirs.Limits.Calls),
		Parallel:       pickInt(was.Limits.Parallel, mine.Limits.Parallel, theirs.Limits.Parallel),
		TimeoutSeconds: pickInt(was.Limits.TimeoutSeconds, mine.Limits.TimeoutSeconds, theirs.Limits.TimeoutSeconds),
	}
	if !sameRoles(mine.Roles, was.Roles) {
		if !sameRoles(theirs.Roles, was.Roles) && !sameRoles(theirs.Roles, mine.Roles) {
			return nil, &Conflict{What: "the council's roles"}
		}
		merged.Roles = mine.Roles
	}
	return &merged, nil
}

// sameRoles compares roles as the file has them: what From says is how the
// page got there, and a stance by its text.
func sameRoles(a, b []Role) bool {
	norm := func(roles []Role) []Role {
		out := make([]Role, len(roles))
		for i, role := range roles {
			role.From = ""
			role.ID, role.Title, role.Model, role.Effort = strings.TrimSpace(role.ID), strings.TrimSpace(role.Title), strings.TrimSpace(role.Model), strings.TrimSpace(role.Effort)
			role.Stance = strings.TrimSpace(role.Stance)
			var owns []string
			for _, entry := range role.Owns {
				if entry = strings.TrimSpace(entry); entry != "" {
					owns = append(owns, entry)
				}
			}
			role.Owns = owns
			out[i] = role
		}
		return out
	}
	return reflect.DeepEqual(norm(a), norm(b))
}

// mergeConfig returns the part of the configuration on the branch with what
// the person changed. A repo the person changed that the branch no longer
// has is a conflict.
func mergeConfig(was, mine, theirs *Config) (*Config, error) {
	merged := *theirs
	merged.Name = pick(was.Name, mine.Name, theirs.Name)
	merged.Description = pick(was.Description, mine.Description, theirs.Description)
	merged.Worker = Worker{
		Backend: pick(was.Worker.Backend, mine.Worker.Backend, theirs.Worker.Backend),
		Model:   pick(was.Worker.Model, mine.Worker.Model, theirs.Worker.Model),
		Effort:  pick(was.Worker.Effort, mine.Worker.Effort, theirs.Worker.Effort),
	}
	merged.Agents = Agents{
		Reviewer: pickRole(was.Agents.Reviewer, mine.Agents.Reviewer, theirs.Agents.Reviewer),
		Resolver: pickRole(was.Agents.Resolver, mine.Agents.Resolver, theirs.Agents.Resolver),
	}
	merged.Review = Review{
		ScoreThreshold: pickInt(was.Review.ScoreThreshold, mine.Review.ScoreThreshold, theirs.Review.ScoreThreshold),
		MergeStrategy:  pick(was.Review.MergeStrategy, mine.Review.MergeStrategy, theirs.Review.MergeStrategy),
	}
	merged.Retries = Retries{
		ReviewAttempts: pickInt(was.Retries.ReviewAttempts, mine.Retries.ReviewAttempts, theirs.Retries.ReviewAttempts),
		Worker:         pickRetries(was.Retries.Worker, mine.Retries.Worker, theirs.Retries.Worker),
	}
	merged.VerifyMinutes = pickInt(was.VerifyMinutes, mine.VerifyMinutes, theirs.VerifyMinutes)

	byName := func(repos []RepoCommands, name string) *RepoCommands {
		for i := range repos {
			if repos[i].Name == name {
				return &repos[i]
			}
		}
		return nil
	}
	merged.Repos = make([]RepoCommands, len(theirs.Repos))
	copy(merged.Repos, theirs.Repos)
	for i := range merged.Repos {
		there := &merged.Repos[i]
		mineRepo, wasRepo := byName(mine.Repos, there.Name), byName(was.Repos, there.Name)
		if mineRepo == nil || wasRepo == nil {
			continue
		}
		there.Setup = pick(wasRepo.Setup, mineRepo.Setup, there.Setup)
		there.Build = pick(wasRepo.Build, mineRepo.Build, there.Build)
		there.Test = pick(wasRepo.Test, mineRepo.Test, there.Test)
	}
	for _, repo := range mine.Repos {
		if byName(theirs.Repos, repo.Name) != nil {
			continue
		}
		if wasRepo := byName(was.Repos, repo.Name); wasRepo == nil || *wasRepo != repo {
			return nil, &Conflict{What: fmt.Sprintf("the repo %q", repo.Name)}
		}
	}
	return &merged, nil
}

// pickRetries is pickInt for a count where 0 is a value: none.
func pickRetries(was, mine, theirs *int) *int {
	value := func(n *int) int {
		if n == nil {
			return -1
		}
		return *n
	}
	if value(mine) != value(was) {
		return mine
	}
	return theirs
}
