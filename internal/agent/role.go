// Package agent runs one agent for the engine: a role's settings expanded
// into a CLI's arguments and settings (claude.go, codex.go), the process
// held to its time and spend, its answer checked against a schema, and what
// it was run with written to the event log (design.md §5.5, task 1.2).
//
// The limits are the runtime's, not the prompt's: the tools a role has are
// the tools the CLI is given; a role that is to write nothing gets a CLI that
// cannot write; the settings and environment are the ones the runtime makes,
// not the person's.
package agent

import (
	"fmt"
	"slices"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/claudecli"
	"github.com/silver2dream/ai-workflow-kit/internal/guard"
)

// Role is what an agent is run for.
type Role string

const (
	// Implementer writes the ticket's change in its worktree.
	Implementer Role = "implementer"
	// Reviewer reads the change and the engine's test results, and answers
	// with a review. It writes nothing and runs nothing.
	Reviewer Role = "reviewer"
	// Resolver resolves a rebase's conflicts in the lane's worktree.
	Resolver Role = "resolver"
	// The helpers turn text into text and have no tools: triage of an
	// error the engine has not seen, a person's words into an instruction,
	// a guardrail's second opinion, a review case into a lesson.
	Triage      Role = "triage"
	Interpreter Role = "interpreter"
	Judge       Role = "judge"
	Distiller   Role = "distiller"
)

// Roles lists the roles the runtime knows.
var Roles = []Role{Implementer, Reviewer, Resolver, Triage, Interpreter, Judge, Distiller}

// The backends: the coding agent CLIs the runtime drives.
const (
	ClaudeCode = "claude-code"
	Codex      = "codex"
)

// Tools is how much an agent may do, from nothing to running commands.
// Each level has what the one before it has.
type Tools string

const (
	// NoTools: it answers from what it is given.
	NoTools Tools = "none"
	// ReadTools: it reads, lists and searches files.
	ReadTools Tools = "read"
	// EditTools: it also writes and edits files.
	EditTools Tools = "edit"
	// ShellTools: it also runs commands, each held to the command policy.
	ShellTools Tools = "shell"
)

var toolLevels = []Tools{NoTools, ReadTools, EditTools, ShellTools}

func (t Tools) level() int { return slices.Index(toolLevels, t) }

// Profile is a role's settings for one run.
type Profile struct {
	Role    Role
	Backend string
	// Model and Effort are the backend's; "" is its own default.
	Model  string
	Effort string
	Tools  Tools
	// Minutes is how long one run may take.
	Minutes int
	// USD is how much one run may spend; 0 is no limit. The Claude Code CLI
	// stops itself there; for a backend that cannot, it is checked after
	// the run.
	USD float64
	// SchemaRetries is how many times an answer that does not match its
	// schema is asked for again.
	SchemaRetries int
}

// limit is how far a role's settings may go.
type limit struct {
	tools      Tools
	backends   []string
	maxMinutes int
}

var limits = map[Role]limit{
	Implementer: {ShellTools, []string{ClaudeCode, Codex}, 240},
	Reviewer:    {ReadTools, []string{ClaudeCode, Codex}, 120},
	Resolver:    {ShellTools, []string{ClaudeCode, Codex}, 120},
	Triage:      {NoTools, []string{ClaudeCode, Codex}, 15},
	Interpreter: {NoTools, []string{ClaudeCode, Codex}, 15},
	Judge:       {NoTools, []string{ClaudeCode, Codex}, 15},
	Distiller:   {NoTools, []string{ClaudeCode, Codex}, 15},
}

// Override is a role's settings as the project gives them: what is empty or
// zero keeps the default.
type Override struct {
	Backend string
	Model   string
	Effort  string
	Tools   Tools
	Minutes int
	USD     float64
}

// Defaults are a role's settings before the project's. implementer is the
// implementer's own (its backend and model are the project's worker's), for
// the roles that follow it: the resolver works as it does, and the reviewer
// is another model than it.
func Defaults(role Role, implementer Profile) Profile {
	switch role {
	case Implementer:
		p := implementer
		p.Role, p.Tools = Implementer, ShellTools
		if p.Backend == "" {
			p.Backend = Codex
		}
		if p.Minutes == 0 {
			p.Minutes = 30
		}
		return p
	case Reviewer:
		model := "opus"
		if implementer.Backend == ClaudeCode && implementer.Model == "opus" {
			model = "sonnet"
		}
		return Profile{Role: Reviewer, Backend: ClaudeCode, Model: model, Tools: ReadTools, Minutes: 20, SchemaRetries: 1}
	case Resolver:
		backend := implementer.Backend
		if backend == "" {
			backend = Codex
		}
		return Profile{Role: Resolver, Backend: backend, Model: implementer.Model, Effort: implementer.Effort, Tools: ShellTools, Minutes: 20, SchemaRetries: 1}
	case Distiller:
		return Profile{Role: Distiller, Backend: ClaudeCode, Model: "sonnet", Tools: NoTools, Minutes: 2, SchemaRetries: 1}
	default:
		return Profile{Role: role, Backend: ClaudeCode, Model: "haiku", Tools: NoTools, Minutes: 2, SchemaRetries: 1}
	}
}

// Resolve gives a role its settings: the defaults with the project's
// override on them. An override that goes past the role's limit is refused,
// not trimmed: a reviewer set up to write is a mistake to be told about.
func Resolve(role Role, implementer Profile, o Override) (Profile, error) {
	lim, ok := limits[role]
	if !ok {
		return Profile{}, fmt.Errorf("no agent role %q (the roles: %s)", role, roleList())
	}
	p := Defaults(role, implementer)
	if o.Backend != "" && o.Backend != p.Backend {
		// A model and an effort are a backend's own: another backend starts
		// from its own defaults, not from names it does not know.
		p.Backend, p.Model, p.Effort = o.Backend, "", ""
	}
	if o.Model != "" {
		p.Model = o.Model
	}
	if o.Effort != "" {
		p.Effort = o.Effort
	}
	if o.Tools != "" {
		p.Tools = o.Tools
	}
	if o.Minutes != 0 {
		p.Minutes = o.Minutes
	}
	if o.USD != 0 {
		p.USD = o.USD
	}
	return p, p.check(lim)
}

func (p Profile) check(lim limit) error {
	if !slices.Contains(lim.backends, p.Backend) {
		return fmt.Errorf("the %s cannot run on %q (it runs on %s)", p.Role, p.Backend, strings.Join(lim.backends, " or "))
	}
	if p.Tools.level() < 0 {
		return fmt.Errorf("the %s's tools %q are none of none, read, edit, shell", p.Role, p.Tools)
	}
	if p.Tools.level() > lim.tools.level() {
		return fmt.Errorf("the %s may have at most the %q tools, not %q", p.Role, lim.tools, p.Tools)
	}
	if p.Minutes <= 0 || p.Minutes > lim.maxMinutes {
		return fmt.Errorf("the %s's time limit is %d minutes: it must be 1 to %d", p.Role, p.Minutes, lim.maxMinutes)
	}
	if p.USD < 0 {
		return fmt.Errorf("the %s's spend limit is negative", p.Role)
	}
	switch p.Backend {
	case ClaudeCode:
		if !claudecli.ValidEffort(p.Effort) {
			return fmt.Errorf("the %s's effort %q is not one the claude CLI takes (%s)", p.Role, p.Effort, claudecli.EffortHint)
		}
	case Codex:
		if p.Effort != "" && !slices.Contains(codexEfforts, p.Effort) {
			return fmt.Errorf("the %s's effort %q is not one codex takes (%s)", p.Role, p.Effort, strings.Join(codexEfforts, ", "))
		}
	}
	return nil
}

// codexEfforts are the reasoning efforts codex takes (model_reasoning_effort).
var codexEfforts = []string{"minimal", "low", "medium", "high", "xhigh"}

func roleList() string {
	names := make([]string, len(Roles))
	for i, r := range Roles {
		names[i] = string(r)
	}
	return strings.Join(names, ", ")
}

// guardRole is the command policy a role's tool calls are held to. The
// roles that write and run commands are held as a Worker is; the others
// have no command to run and only read, which the same policy checks (no
// reading where credentials are kept).
func (p Profile) guardRole() string {
	return guard.RoleImplementer
}
