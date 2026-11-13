// Package council turns a brief into planning documents, decisions and a
// proposed list of tickets by letting several roles argue it out.
//
// The roles and what each stands for come from the project's roster. The
// order of play does not come from a prompt: this package asks each role in
// turn, counts the rounds and the calls, and writes the files. A role only
// ever answers one question with one text, and only this package writes to
// disk, so a role cannot write a document another role owns.
//
// A plan ends with a proposal. Nothing is created on GitHub until a person
// approves the work list.
package council

import (
	"embed"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// RosterPath is where a project keeps its roster, relative to its root.
const RosterPath = ".ai/roster.yaml"

// Roster says who sits at the table.
type Roster struct {
	// Language the council writes in. Empty means the language of the brief.
	Language string `yaml:"language"`
	// Model answers for every role that names none.
	Model string `yaml:"model"`
	// Limits bound one plan.
	Limits Limits `yaml:"limits"`
	// Roles are the ones who argue. The moderator is not one of them: it has
	// no stance, and is played by this package with the roster's model.
	Roles []Role `yaml:"roles"`
}

// Limits bound what one plan may cost.
type Limits struct {
	// Disputes is how many disputed points are argued out; the rest go to the
	// deciding role as they are.
	Disputes int `yaml:"disputes"`
	// DisputeRounds is how often a disputed point goes back and forth.
	DisputeRounds int `yaml:"dispute_rounds"`
	// Calls is the upper bound on model calls.
	Calls int `yaml:"calls"`
	// Parallel is how many calls are made at the same time.
	Parallel int `yaml:"parallel"`
	// TimeoutSeconds bounds one call.
	TimeoutSeconds int `yaml:"timeout_seconds"`
}

// Role is one seat at the table.
type Role struct {
	// ID names the role in files and in what other roles say about it.
	ID string `yaml:"id"`
	// Title is how the role is introduced.
	Title string `yaml:"title"`
	// Model answers for this role; the roster's when empty.
	Model string `yaml:"model"`
	// Stance is what the role stands for, pushes for and pushes back on.
	Stance string `yaml:"stance"`
	// Owns lists the documents only this role writes: paths under docs/.
	Owns []string `yaml:"owns"`
	// Decides marks the one role that rules on what stays disputed.
	Decides bool `yaml:"decides"`
	// Plans marks the one role that drafts the work list.
	Plans bool `yaml:"plans"`
}

//go:embed rosters/*.yaml
var rosters embed.FS

// DefaultRoster returns the roster a new project of a preset starts with. A
// preset without one of its own gets the generic roster.
func DefaultRoster(preset string) []byte {
	if data, err := rosters.ReadFile("rosters/" + preset + ".yaml"); err == nil {
		return data
	}
	data, err := rosters.ReadFile("rosters/generic.yaml")
	if err != nil {
		panic("council: the generic roster is missing from the build: " + err.Error())
	}
	return data
}

// LoadRoster reads and checks a roster file.
func LoadRoster(file string) (*Roster, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return ParseRoster(data)
}

// ParseRoster reads a roster. A field it does not know is an error: a
// misspelt "decides" would otherwise leave the table without anyone to decide.
func ParseRoster(data []byte) (*Roster, error) {
	var roster Roster
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&roster); err != nil {
		return nil, fmt.Errorf("the roster cannot be read: %w", err)
	}
	roster.applyDefaults()
	if err := roster.Validate(); err != nil {
		return nil, err
	}
	return &roster, nil
}

func (r *Roster) applyDefaults() {
	if r.Model == "" {
		r.Model = "sonnet"
	}
	if r.Limits.Disputes <= 0 {
		r.Limits.Disputes = 5
	}
	if r.Limits.DisputeRounds <= 0 {
		r.Limits.DisputeRounds = 2
	}
	if r.Limits.Calls <= 0 {
		r.Limits.Calls = 80
	}
	if r.Limits.Parallel <= 0 {
		r.Limits.Parallel = 3
	}
	if r.Limits.TimeoutSeconds <= 0 {
		// A role that writes two documents answers at length.
		r.Limits.TimeoutSeconds = 600
	}
	for i := range r.Roles {
		if r.Roles[i].Model == "" {
			r.Roles[i].Model = r.Model
		}
		if r.Roles[i].Title == "" {
			r.Roles[i].Title = r.Roles[i].ID
		}
	}
}

var roleID = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// Validate checks that the roster can hold a discussion: at least two roles
// to disagree, one of them to decide, one to plan, and no document with two
// owners.
func (r *Roster) Validate() error {
	if len(r.Roles) < 2 {
		return fmt.Errorf("the roster has %d role(s); a discussion needs at least two", len(r.Roles))
	}
	seen := map[string]bool{}
	owner := map[string]string{}
	var deciders, planners []string
	for _, role := range r.Roles {
		if !roleID.MatchString(role.ID) {
			return fmt.Errorf("role id %q: use lower-case letters, digits and hyphens, starting with a letter", role.ID)
		}
		if seen[role.ID] {
			return fmt.Errorf("role %q is in the roster twice", role.ID)
		}
		seen[role.ID] = true
		if strings.TrimSpace(role.Stance) == "" {
			return fmt.Errorf("role %q has no stance: say what it stands for", role.ID)
		}
		if role.Decides {
			deciders = append(deciders, role.ID)
		}
		if role.Plans {
			planners = append(planners, role.ID)
		}
		for _, doc := range role.Owns {
			if err := checkOwnedPath(doc); err != nil {
				return fmt.Errorf("role %q: %w", role.ID, err)
			}
			if other, taken := owner[doc]; taken {
				return fmt.Errorf("%s has two owners, %q and %q; a document has one", doc, other, role.ID)
			}
			owner[doc] = role.ID
		}
	}
	if len(deciders) != 1 {
		return fmt.Errorf("%d roles decide (%s); exactly one role has \"decides: true\"", len(deciders), strings.Join(deciders, ", "))
	}
	if len(planners) != 1 {
		return fmt.Errorf("%d roles plan (%s); exactly one role has \"plans: true\"", len(planners), strings.Join(planners, ", "))
	}
	return nil
}

// Role returns the role with an id, or nil.
func (r *Roster) Role(id string) *Role {
	for i := range r.Roles {
		if r.Roles[i].ID == id {
			return &r.Roles[i]
		}
	}
	return nil
}

// Decider returns the role that rules on what stays disputed.
func (r *Roster) Decider() *Role {
	for i := range r.Roles {
		if r.Roles[i].Decides {
			return &r.Roles[i]
		}
	}
	return nil
}

// Planner returns the role that drafts the work list.
func (r *Roster) Planner() *Role {
	for i := range r.Roles {
		if r.Roles[i].Plans {
			return &r.Roles[i]
		}
	}
	return nil
}

// IDs lists the role ids in the order of the roster.
func (r *Roster) IDs() []string {
	ids := make([]string, len(r.Roles))
	for i, role := range r.Roles {
		ids[i] = role.ID
	}
	return ids
}

// DecisionsPath is the decision log. Only this package appends to it.
const DecisionsPath = "docs/decisions.md"

// PlansDir holds one folder per plan. Only this package writes there.
const PlansDir = "docs/plans"

// checkOwnedPath checks that a path can be a role's document: a markdown file
// under docs/, written the way it is stored, and not one of the files this
// package keeps.
func checkOwnedPath(doc string) error {
	clean := path.Clean(doc)
	switch {
	case doc == "" || clean != doc || strings.Contains(doc, "\\"):
		return fmt.Errorf("document %q: write the path plainly, with forward slashes (e.g. docs/design/game-design.md)", doc)
	case !strings.HasPrefix(doc, "docs/"):
		return fmt.Errorf("document %q is outside docs/", doc)
	case !strings.HasSuffix(doc, ".md"):
		return fmt.Errorf("document %q is not a markdown file", doc)
	case doc == DecisionsPath || doc == "docs/README.md":
		return fmt.Errorf("document %q is kept by the council itself and has no owner", doc)
	case strings.HasPrefix(doc, PlansDir+"/"):
		return fmt.Errorf("document %q is under %s/, where the council keeps its plans", doc, PlansDir)
	}
	return nil
}

// Owns reports whether the role is the owner of a document.
func (role *Role) OwnsDocument(doc string) bool {
	for _, owned := range role.Owns {
		if owned == doc {
			return true
		}
	}
	return false
}
