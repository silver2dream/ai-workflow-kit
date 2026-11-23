// Package settings reads and changes a project's settings the way the
// workbench shows them: the roster of its council (.ai/roster.yaml) and the
// part of its configuration (.ai/config.yaml) a person changes. A change is
// made to the YAML of the file, so that its comments and the keys the
// workbench does not show stay as they are, and is checked the way the
// command that reads the file checks it before it is written.
package settings

import (
	"errors"
	"fmt"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/claudecli"
	"github.com/silver2dream/ai-workflow-kit/internal/council"
	"gopkg.in/yaml.v3"
)

// Roster is the council's roster as the file has it: nothing is filled in,
// so that a value left empty stays the default and is not written down as it.
type Roster struct {
	// Language the council writes in; empty for the language of the brief.
	Language string `json:"language"`
	// Model and Effort answer for every role that names none.
	Model  string `json:"model"`
	Effort string `json:"effort"`
	Limits Limits `json:"limits"`
	Roles  []Role `json:"roles"`
}

// Limits bound one plan; nil is the default.
type Limits struct {
	Topics         *int `json:"topics"`
	Disputes       *int `json:"disputes"`
	DisputeRounds  *int `json:"dispute_rounds"`
	Calls          *int `json:"calls"`
	Parallel       *int `json:"parallel"`
	TimeoutSeconds *int `json:"timeout_seconds"`
}

// Role is one seat at the table.
type Role struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Model   string   `json:"model"`
	Effort  string   `json:"effort"`
	Stance  string   `json:"stance"`
	Owns    []string `json:"owns"`
	Decides bool     `json:"decides"`
	Plans   bool     `json:"plans"`
	// From is the id the role has in the file, for a role whose id was
	// changed: it keeps its place and its comments.
	From string `json:"from,omitempty"`
}

// RosterDefaults are what a roster that leaves a value out gets.
type RosterDefaults struct {
	Model   string   `json:"model"`
	Limits  Limits   `json:"limits"`
	Efforts []string `json:"efforts"`
}

// DefaultsOfRoster returns what a roster that leaves a value out gets.
func DefaultsOfRoster() RosterDefaults {
	limits := council.DefaultLimits()
	return RosterDefaults{
		Model: council.DefaultModel,
		Limits: Limits{
			Topics: &limits.Topics, Disputes: &limits.Disputes, DisputeRounds: &limits.DisputeRounds,
			Calls: &limits.Calls, Parallel: &limits.Parallel, TimeoutSeconds: &limits.TimeoutSeconds,
		},
		Efforts: claudecli.Efforts,
	}
}

// ReadRoster reads what a roster file holds. A roster the council could not
// use is read all the same, so that it can be put right; Check says what is
// wrong with it.
func ReadRoster(data []byte) (*Roster, error) {
	var file struct {
		Language string `yaml:"language"`
		Model    string `yaml:"model"`
		Effort   string `yaml:"effort"`
		Limits   struct {
			Topics         int `yaml:"topics"`
			Disputes       int `yaml:"disputes"`
			DisputeRounds  int `yaml:"dispute_rounds"`
			Calls          int `yaml:"calls"`
			Parallel       int `yaml:"parallel"`
			TimeoutSeconds int `yaml:"timeout_seconds"`
		} `yaml:"limits"`
		Roles []struct {
			ID      string   `yaml:"id"`
			Title   string   `yaml:"title"`
			Model   string   `yaml:"model"`
			Effort  string   `yaml:"effort"`
			Stance  string   `yaml:"stance"`
			Owns    []string `yaml:"owns"`
			Decides bool     `yaml:"decides"`
			Plans   bool     `yaml:"plans"`
		} `yaml:"roles"`
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("the roster cannot be read: %w", err)
	}
	positive := func(n int) *int {
		if n <= 0 {
			return nil
		}
		return &n
	}
	roster := &Roster{
		Language: file.Language, Model: file.Model, Effort: file.Effort,
		Limits: Limits{
			Topics: positive(file.Limits.Topics), Disputes: positive(file.Limits.Disputes),
			DisputeRounds: positive(file.Limits.DisputeRounds), Calls: positive(file.Limits.Calls),
			Parallel: positive(file.Limits.Parallel), TimeoutSeconds: positive(file.Limits.TimeoutSeconds),
		},
		Roles: []Role{},
	}
	for _, role := range file.Roles {
		owns := role.Owns
		if owns == nil {
			owns = []string{}
		}
		roster.Roles = append(roster.Roles, Role{
			ID: role.ID, Title: role.Title, Model: role.Model, Effort: role.Effort,
			Stance: role.Stance, Owns: owns, Decides: role.Decides, Plans: role.Plans,
		})
	}
	return roster, nil
}

// CheckRoster says what keeps the council from meeting with a roster file,
// nil when nothing does.
func CheckRoster(data []byte) error {
	_, err := council.ParseRoster(data)
	return err
}

// roleItem is a line that starts a role of the roster.
func roleItem(line string) bool { return strings.HasPrefix(line, "  - ") }

// ApplyRoster writes a roster over what a roster file holds, and returns the
// file as it then is. Roles are matched by id (or by From, for one whose id
// was changed): a role keeps its comments and the keys this does not know,
// and the roles are in the order given. It fails, writing nothing, when the
// council could not meet with the roster.
func ApplyRoster(data []byte, roster Roster) ([]byte, error) {
	doc, err := parse(data)
	if err != nil {
		return nil, fmt.Errorf("the roster cannot be read: %w", err)
	}
	top := document(doc)
	if top.Kind != yaml.MappingNode {
		return nil, errors.New("the roster is not a mapping of keys")
	}
	had := keysOf(top)
	setOptional(top, "language", strings.TrimSpace(roster.Language))
	setOptional(top, "model", strings.TrimSpace(roster.Model))
	setOptional(top, "effort", strings.TrimSpace(roster.Effort))

	limits := roster.Limits
	if limits == (Limits{}) && get(top, "limits") == nil {
		// Nothing to say, and nothing said.
	} else {
		node := child(top, "limits")
		hadLimits := keysOf(node)
		setOptionalInt(node, "topics", positiveOrNil(limits.Topics))
		setOptionalInt(node, "disputes", positiveOrNil(limits.Disputes))
		setOptionalInt(node, "dispute_rounds", positiveOrNil(limits.DisputeRounds))
		setOptionalInt(node, "calls", positiveOrNil(limits.Calls))
		setOptionalInt(node, "parallel", positiveOrNil(limits.Parallel))
		setOptionalInt(node, "timeout_seconds", positiveOrNil(limits.TimeoutSeconds))
		arrange(node, limitKeys, hadLimits)
		if len(node.Content) == 0 {
			remove(top, "limits")
		}
	}

	roles := get(top, "roles")
	if roles == nil || roles.Kind != yaml.SequenceNode {
		roles = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		set(top, "roles", roles)
		roles = get(top, "roles")
	}
	was := map[string]*yaml.Node{}
	for _, item := range roles.Content {
		if id := text(item, "id"); id != "" && item.Kind == yaml.MappingNode {
			was[id] = item
		}
	}
	var items []*yaml.Node
	for _, role := range roster.Roles {
		id := strings.TrimSpace(role.ID)
		item := was[id]
		if role.From != "" && was[role.From] != nil {
			item = was[role.From]
		}
		if item == nil {
			item = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		}
		// A role is taken once: two roles of the same id are refused below.
		for key, taken := range was {
			if taken == item {
				delete(was, key)
			}
		}
		setRole(item, role, id)
		items = append(items, item)
	}
	roles.Content = items
	arrange(top, rosterKeys, had)

	out, err := encode(doc, data, topLevelKey, roleItem)
	if err != nil {
		return nil, err
	}
	if err := CheckRoster(out); err != nil {
		return nil, err
	}
	return out, nil
}

// The keys of a roster, of its limits and of a role, in the order the
// rosters a project starts with have them: a key that is added goes where
// this order puts it.
var (
	rosterKeys = []string{"language", "model", "effort", "limits", "roles"}
	limitKeys  = []string{"topics", "disputes", "dispute_rounds", "calls", "parallel", "timeout_seconds"}
	roleKeys   = []string{"id", "title", "model", "effort", "decides", "plans", "stance", "owns"}
)

// setRole writes a role into its mapping.
func setRole(item *yaml.Node, role Role, id string) {
	had := keysOf(item)
	defer arrange(item, roleKeys, had)
	set(item, "id", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: id})
	setOptional(item, "title", strings.TrimSpace(role.Title))
	setOptional(item, "model", strings.TrimSpace(role.Model))
	setOptional(item, "effort", strings.TrimSpace(role.Effort))
	setFlag(item, "decides", role.Decides)
	setFlag(item, "plans", role.Plans)

	stance := strings.TrimSpace(role.Stance)
	if old := get(item, "stance"); old == nil || strings.TrimSpace(old.Value) != stance {
		// A stance is a paragraph, written as a block as the rosters a
		// project starts with have it.
		node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: stance + "\n", Style: yaml.LiteralStyle}
		if stance == "" {
			node = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "", Style: yaml.DoubleQuotedStyle}
		}
		set(item, "stance", node)
	}

	var owns []string
	for _, entry := range role.Owns {
		if entry = strings.TrimSpace(entry); entry != "" {
			owns = append(owns, entry)
		}
	}
	setList(item, "owns", owns)
}

// positiveOrNil is a limit as it is written: one of zero or less is the
// default, and not written.
func positiveOrNil(n *int) *int {
	if n == nil || *n <= 0 {
		return nil
	}
	return n
}
