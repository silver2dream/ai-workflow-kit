// Package council turns a brief into planning documents, decisions and a
// proposed list of tickets by letting several roles argue it out.
//
// The roles and what each stands for come from the project's roster. The
// order of play does not come from a prompt: this package asks each role in
// turn, counts the rounds and the calls, and writes the files. A role only
// ever answers one question with one text, and only this package writes to
// disk, so a role cannot write a document another role owns.
//
// How a document is written is the project's to say: a role is given the
// project's rules for the files it writes, and what it wrote is read against
// them (byrules.go).
//
// A plan ends with a proposal. Nothing is created on GitHub until a person
// approves the work list.
package council

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
	"text/template"

	"github.com/silver2dream/ai-workflow-kit/internal/claudecli"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
	"gopkg.in/yaml.v3"
)

// RosterPath is where a project keeps its roster, relative to its root.
const RosterPath = layout.RosterFile

// Roster says who sits at the table.
type Roster struct {
	// Language the council writes in. Empty means the language of the brief.
	Language string `yaml:"language"`
	// Model answers for every role that names none.
	Model string `yaml:"model"`
	// Effort is how hard every role that names none thinks: one of the levels
	// the claude CLI takes, or empty for its own default.
	Effort string `yaml:"effort"`
	// Limits bound one plan.
	Limits Limits `yaml:"limits"`
	// Roles are the ones who argue. The moderator is not one of them: it has
	// no stance, and is played by this package with the roster's model.
	Roles []Role `yaml:"roles"`
}

// Limits bound what one plan may cost.
type Limits struct {
	// Topics is how many topics an agenda may have.
	Topics int `yaml:"topics"`
	// Disputes is how many disputed points of one topic are argued out; the
	// rest go to the deciding role as they are.
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
	// Effort is how hard this role thinks; the roster's when empty.
	Effort string `yaml:"effort"`
	// Stance is what the role stands for, pushes for and pushes back on.
	Stance string `yaml:"stance"`
	// Owns lists what only this role writes, by paths from the project's
	// root: a document, or a folder, written with a slash at its end, in which
	// the role writes any document and opens a new one when a topic ends in
	// it. A document is a Markdown file, or a file of the game data: a table
	// (.csv) or its schema (.xml). A folder or a file inside another role's
	// folder is not that role's: what is named closest owns it
	// (gamedata/Datas/text/ inside gamedata/Datas/).
	Owns []string `yaml:"owns"`
	// Decides marks the one role that rules on what stays disputed.
	Decides bool `yaml:"decides"`
	// Plans marks the one role that drafts the work list.
	Plans bool `yaml:"plans"`

	// inner are what other roles own inside this role's folders. Validate
	// sets them.
	inner []string
}

// The rosters a project starts with are templates: the engineering documents
// a roster names are kept with the project's repos, wherever those are.
//
//go:embed rosters/*.yaml.tmpl
var rosters embed.FS

// rosterRepos is what a roster a project starts with is written from.
type rosterRepos []Repo

// Docs lists the directories of the repos' engineering documents, each once,
// in the order of the repos. A project that names no repo has the one that is
// the whole repository.
func (repos rosterRepos) Docs() []string {
	var dirs []string
	for _, entry := range repos {
		if dir := layout.RepoDocs(entry.Path); !contains(dirs, dir) {
			dirs = append(dirs, dir)
		}
	}
	if len(dirs) == 0 {
		dirs = []string{layout.EngineeringDocs}
	}
	return dirs
}

// plainScalar is a path YAML reads as the string it is, without quotes.
var plainScalar = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_./-]*$`)

// rosterFile writes a document's path as a value of the roster.
func rosterFile(dir, name string) string {
	file := dir + name
	if plainScalar.MatchString(file) {
		return file
	}
	return fmt.Sprintf("%q", file)
}

// DefaultRoster returns the roster a new project of a preset starts with,
// written for the project's repos: the engineering documents it names are the
// ones of those repos (layout.RepoDocs). A preset without a roster of its own
// gets the generic one. It fails when the roster could not hold a meeting for
// such repos: one that lies outside the project, say.
func DefaultRoster(preset string, repos []Repo) ([]byte, error) {
	text, err := rosters.ReadFile("rosters/" + preset + ".yaml.tmpl")
	if err != nil {
		if text, err = rosters.ReadFile("rosters/generic.yaml.tmpl"); err != nil {
			panic("council: the generic roster is missing from the build: " + err.Error())
		}
	}
	tmpl, err := template.New("roster").Option("missingkey=error").Funcs(template.FuncMap{"file": rosterFile}).Parse(string(text))
	if err != nil {
		panic("council: the roster of " + preset + " is not a template: " + err.Error())
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, rosterRepos(repos)); err != nil {
		return nil, fmt.Errorf("the roster of %s cannot be written: %w", preset, err)
	}
	if _, err := ParseRoster(out.Bytes()); err != nil {
		return nil, fmt.Errorf("the roster a project of these repos starts with cannot be used: %w", err)
	}
	return out.Bytes(), nil
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
	if r.Limits.Topics <= 0 {
		r.Limits.Topics = 8
	}
	if r.Limits.Disputes <= 0 {
		r.Limits.Disputes = 2
	}
	if r.Limits.DisputeRounds <= 0 {
		r.Limits.DisputeRounds = 2
	}
	if r.Limits.Calls <= 0 {
		// Eight topics of five roles, with their documents read against the
		// project's rules, and the work list.
		r.Limits.Calls = 200
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
		if r.Roles[i].Effort == "" {
			r.Roles[i].Effort = r.Effort
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
	if !claudecli.ValidEffort(r.Effort) {
		return fmt.Errorf("effort %q: use %s", r.Effort, claudecli.EffortHint)
	}
	seen := map[string]bool{}
	var deciders, planners []string
	type claim struct{ entry, role string }
	var claims []claim
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
		if !claudecli.ValidEffort(role.Effort) {
			return fmt.Errorf("role %q: effort %q: use %s", role.ID, role.Effort, claudecli.EffortHint)
		}
		if role.Decides {
			deciders = append(deciders, role.ID)
		}
		if role.Plans {
			planners = append(planners, role.ID)
		}
		for _, entry := range role.Owns {
			if err := checkOwnedEntry(entry); err != nil {
				return fmt.Errorf("role %q: %w", role.ID, err)
			}
			// Two paths that differ in case only are one file where the file
			// system does not tell them apart. What one role names inside
			// another's folder is the first role's; a role does not name what
			// it owns twice.
			for _, other := range claims {
				a, b := strings.ToLower(other.entry), strings.ToLower(entry)
				var inner, outer claim
				switch {
				case a == b && other.role == role.ID:
					return fmt.Errorf("role %q owns %s twice", role.ID, entry)
				case a == b:
					return fmt.Errorf("%s has two owners, %q and %q; a document has one", entry, other.role, role.ID)
				case strings.HasSuffix(a, "/") && strings.HasPrefix(b, a):
					inner, outer = claim{entry, role.ID}, other
				case strings.HasSuffix(b, "/") && strings.HasPrefix(a, b):
					inner, outer = other, claim{entry, role.ID}
				default:
					continue
				}
				if inner.role == outer.role {
					return fmt.Errorf("role %q owns %s twice: it is in %s", role.ID, inner.entry, outer.entry)
				}
			}
			claims = append(claims, claim{entry, role.ID})
		}
	}
	// What is named inside a role's folder is another role's: a role that
	// names something inside its own folder was refused above.
	for i := range r.Roles {
		role := &r.Roles[i]
		role.inner = nil
		for _, folder := range role.Folders() {
			for _, other := range claims {
				if inFolder(other.entry, folder) {
					role.inner = append(role.inner, other.entry)
				}
			}
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

// Voice is what the moderator answers with: the roster's model and effort.
func (r *Roster) Voice() Voice { return Voice{Model: r.Model, Effort: r.Effort} }

// Voice is what the role answers with: its model and effort, which are the
// roster's when it names none.
func (r *Role) Voice() Voice { return Voice{Model: r.Model, Effort: r.Effort} }

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

// Owner returns the role that owns a document, or nil.
func (r *Roster) Owner(doc string) *Role {
	for i := range r.Roles {
		if r.Roles[i].OwnsDocument(doc) {
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

// indexPath is the index of the project's documents. Only this package keeps
// the part of it between its two marker lines.
const indexPath = "docs/README.md"

// instructions are the files an agent is instructed by, in whichever
// directory they lie. `awkit generate` or a person writes them, never a role.
var instructions = map[string]bool{"claude.md": true, "claude.local.md": true, "agents.md": true}

// checkOwnedEntry checks an entry of a role's owns: a document, or a folder
// of documents.
func checkOwnedEntry(entry string) error {
	folder, isFolder := strings.CutSuffix(entry, "/")
	if !isFolder {
		return checkOwnedPath(entry)
	}
	plain := path.Clean(folder) == folder && folder != "." && !strings.ContainsAny(folder, `\:`) &&
		!strings.HasPrefix(folder, "/") && folder != ".." && !strings.HasPrefix(folder, "../")
	if !plain {
		return fmt.Errorf("folder %q: write the path plainly, from the project's root, with forward slashes and a slash at its end (e.g. docs/design/)", entry)
	}
	for _, name := range strings.Split(folder, "/") {
		if strings.HasPrefix(name, ".") {
			return fmt.Errorf("folder %q is in %s, which a tool keeps for itself; a role's documents are kept elsewhere", entry, name)
		}
	}
	lower := strings.ToLower(entry)
	plans := strings.ToLower(PlansDir) + "/"
	switch {
	case strings.HasPrefix(lower, plans):
		return fmt.Errorf("folder %q is under %s/, where the council keeps its plans", entry, PlansDir)
	case strings.HasPrefix(plans, lower):
		return fmt.Errorf("folder %q holds the council's own files (%s, %s, %s/): own a folder inside it", entry, DecisionsPath, indexPath, PlansDir)
	}
	return nil
}

// isDocument reports whether a file, named in lower case, is of a kind a role
// writes: a Markdown document, or a file of the game data, a table (.csv) or
// its schema (.xml).
func isDocument(lower string) bool {
	return strings.HasSuffix(lower, ".md") || isData(lower)
}

// isData reports whether a file, named in lower case, is of the kind the game
// data is kept in: a table or its schema. What it holds is checked only when
// the project keeps its game data there (gamedata.Layout).
func isData(lower string) bool {
	return strings.HasSuffix(lower, ".csv") || strings.HasSuffix(lower, ".xml")
}

// checkOwnedPath checks that a path can be a role's document: a Markdown
// file, or a table or a schema of the game data, of the project, named from
// the project's root the way it is stored. It is
// not one of the files this package keeps, not a file an agent is instructed
// by, and not in a directory a tool keeps for itself (.claude, .ai, .git,
// .github): what a role writes is there to be read, not to be obeyed.
//
// Where the file system does not tell upper case from lower, neither does
// this check: Docs/Decisions.md is the decision log there.
func checkOwnedPath(doc string) error {
	// An empty path is not plain either: Clean makes "." of it.
	plain := path.Clean(doc) == doc && !strings.ContainsAny(doc, `\:`) &&
		!strings.HasPrefix(doc, "/") && doc != ".." && !strings.HasPrefix(doc, "../")
	if !plain {
		return fmt.Errorf("document %q: write the path plainly, from the project's root and with forward slashes (e.g. docs/design/game-design.md)", doc)
	}
	names := strings.Split(doc, "/")
	for _, name := range names {
		if strings.HasPrefix(name, ".") {
			return fmt.Errorf("document %q is in %s, which a tool keeps for itself; a role's document is kept elsewhere", doc, name)
		}
	}
	lower := strings.ToLower(doc)
	switch {
	case !isDocument(lower):
		return fmt.Errorf("document %q is neither a Markdown file nor a table of the game data (.csv) or its schema (.xml)", doc)
	case lower == strings.ToLower(DecisionsPath) || lower == strings.ToLower(indexPath):
		return fmt.Errorf("document %q is kept by the council itself and has no owner", doc)
	case strings.HasPrefix(lower, strings.ToLower(PlansDir)+"/"):
		return fmt.Errorf("document %q is under %s/, where the council keeps its plans", doc, PlansDir)
	case instructions[strings.ToLower(names[len(names)-1])]:
		return fmt.Errorf("document %q is what an agent is instructed by, not a document a role writes", doc)
	}
	return nil
}

// OwnsDocument reports whether the role is the owner of a document: the
// roster names the file, or a folder it is in and no other role names it or a
// folder closer to it. A path that cannot be a document is nobody's.
func (role *Role) OwnsDocument(doc string) bool {
	if checkOwnedPath(doc) != nil {
		return false
	}
	for _, inner := range role.inner {
		if strings.EqualFold(inner, doc) || strings.HasSuffix(inner, "/") && inFolder(doc, inner) {
			return false
		}
	}
	for _, owned := range role.Owns {
		if owned == doc || strings.HasSuffix(owned, "/") && strings.HasPrefix(doc, owned) {
			return true
		}
	}
	return false
}

// inFolder reports whether a path is inside a folder, upper and lower case
// alike: where the file system does not tell them apart, they are one path.
func inFolder(file, folder string) bool {
	return len(file) > len(folder) && strings.EqualFold(file[:len(folder)], folder)
}

// Files lists the documents the roster names for the role by their files.
func (role *Role) Files() []string {
	var files []string
	for _, owned := range role.Owns {
		if !strings.HasSuffix(owned, "/") {
			files = append(files, owned)
		}
	}
	return files
}

// Folders lists the folders the role writes in.
func (role *Role) Folders() []string {
	var folders []string
	for _, owned := range role.Owns {
		if strings.HasSuffix(owned, "/") {
			folders = append(folders, owned)
		}
	}
	return folders
}
