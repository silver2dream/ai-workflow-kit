package settings

import (
	"errors"
	"fmt"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/agent"
	"github.com/silver2dream/ai-workflow-kit/internal/claudecli"
	"github.com/silver2dream/ai-workflow-kit/internal/config"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
	"gopkg.in/yaml.v3"
)

// Config is the part of a project's configuration a person changes in the
// workbench: what the project is called, how each repo is built and tested,
// who writes the code, and how the work is reviewed. The repos and branches
// are shown and not changed: a repo that moved or a branch that is not on
// GitHub stops every run, and is changed in the file, where it is checked.
type Config struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Integration and Release are the branches; shown, not changed.
	Integration string         `json:"integration_branch"`
	Release     string         `json:"release_branch"`
	Repos       []RepoCommands `json:"repos"`
	Worker      Worker         `json:"worker"`
	// Agents are the engine's reviewer and resolver.
	Agents  Agents  `json:"agents"`
	Review  Review  `json:"review"`
	Retries Retries `json:"retries"`
	// VerifyMinutes bounds one build, test or setup command; nil for the
	// default.
	VerifyMinutes *int `json:"verify_minutes"`
	// Engine is how much the engine does at once.
	Engine Engine `json:"engine"`
}

// Engine is how much the engine does at once (engine.max_parallel,
// engine.max_per_backend): nil is the default, one step at a time, and a
// backend the map leaves out has no limit of its own.
type Engine struct {
	MaxParallel   *int           `json:"max_parallel"`
	MaxPerBackend map[string]int `json:"max_per_backend"`
}

// RepoCommands are the commands that build and test a repo. The repo is
// named by Name; Path and Language are shown, not changed.
type RepoCommands struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Language string `json:"language"`
	Setup    string `json:"setup"`
	Build    string `json:"build"`
	Test     string `json:"test"`
}

// Worker is who writes the code. Model and Effort are Claude Code's.
type Worker struct {
	Backend string `json:"backend"`
	Model   string `json:"model"`
	Effort  string `json:"effort"`
}

// Agents are the settings of the engine's reviewer and resolver
// (agents.reviewer, agents.resolver): what is left empty keeps the role's
// default, which follows the Worker.
type Agents struct {
	Reviewer AgentRole `json:"reviewer"`
	Resolver AgentRole `json:"resolver"`
}

// AgentRole is one of them: the backend, its model and effort, and how long
// and how much one run may take; nil for the default.
type AgentRole struct {
	Backend string   `json:"backend"`
	Model   string   `json:"model"`
	Effort  string   `json:"effort"`
	Minutes *int     `json:"minutes"`
	USD     *float64 `json:"usd"`
}

// Review is how a pull request is reviewed and merged.
type Review struct {
	// ScoreThreshold is the score out of 10 a review has to give; nil for
	// the default.
	ScoreThreshold *int   `json:"score_threshold"`
	MergeStrategy  string `json:"merge_strategy"`
}

// Retries are how often work is tried again; nil for the default.
type Retries struct {
	// ReviewAttempts is how often a pull request may be sent back before
	// its ticket waits for a person.
	ReviewAttempts *int `json:"review_attempts"`
	// Worker is how often a Worker tries again after it failed, 0 for not
	// at all.
	Worker *int `json:"worker"`
}

// ConfigDefaults are what a configuration that leaves a value out gets, and
// the values a choice may take.
type ConfigDefaults struct {
	Backend        string   `json:"backend"`
	Model          string   `json:"model"`
	ScoreThreshold int      `json:"score_threshold"`
	MergeStrategy  string   `json:"merge_strategy"`
	ReviewAttempts int      `json:"review_attempts"`
	WorkerRetries  int      `json:"worker_retries"`
	VerifyMinutes  int      `json:"verify_minutes"`
	MaxParallel    int      `json:"max_parallel"`
	Backends       []string `json:"backends"`
	Strategies     []string `json:"strategies"`
	Efforts        []string `json:"efforts"`
	// Agents are what the reviewer's and the resolver's settings may be;
	// their defaults follow the Worker, and the page works them out by the
	// rules of agent.Defaults.
	Agents AgentLimits `json:"agents"`
}

// AgentLimits are what the settings of the engine's agents may be.
type AgentLimits struct {
	// Minutes is how long one run takes by default, and MaxMinutes how long
	// it may be set to take, by role.
	Minutes    map[string]int `json:"minutes"`
	MaxMinutes map[string]int `json:"max_minutes"`
	// CodexEfforts are the efforts codex takes (Efforts are Claude Code's).
	CodexEfforts []string `json:"codex_efforts"`
}

// DefaultsOfConfig returns what a configuration that leaves a value out gets.
func DefaultsOfConfig() ConfigDefaults {
	defaults := config.Default()
	return ConfigDefaults{
		Backend:        defaults.Worker.Backend,
		Model:          defaults.Worker.ClaudeCode.Model,
		ScoreThreshold: defaults.Review.ScoreThreshold,
		MergeStrategy:  defaults.Review.MergeStrategy,
		ReviewAttempts: defaults.Escalation.MaxReviewAttempts,
		WorkerRetries:  defaults.Escalation.RetryCount,
		VerifyMinutes:  defaults.Timeouts.VerifyMinutes,
		MaxParallel:    defaults.Engine.MaxParallel,
		Backends:       []string{"codex", "claude-code"},
		Strategies:     []string{"squash", "merge", "rebase"},
		Efforts:        claudecli.Efforts,
		Agents:         agentLimits(defaults),
	}
}

func agentLimits(defaults *config.Config) AgentLimits {
	limits := AgentLimits{Minutes: map[string]int{}, MaxMinutes: map[string]int{}, CodexEfforts: agent.CodexEfforts()}
	worker := agent.ImplementerOf(defaults)
	for _, role := range []agent.Role{agent.Reviewer, agent.Resolver} {
		limits.Minutes[string(role)] = agent.Defaults(role, worker).Minutes
		limits.MaxMinutes[string(role)] = agent.MaxMinutes(role)
	}
	return limits
}

// ReadConfig reads the part of a configuration file the workbench shows, as
// the file has it: a value it leaves out is empty, or nil.
func ReadConfig(data []byte) (*Config, error) {
	var file config.Config
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("%s cannot be read: %w", layout.ConfigFile, err)
	}
	positive := func(n int) *int {
		if n <= 0 {
			return nil
		}
		return &n
	}
	release := file.Git.ReleaseBranch
	if release == "" {
		release = config.Default().Git.ReleaseBranch
	}
	cfg := &Config{
		Name: file.Project.Name, Description: file.Project.Description,
		Integration: file.Git.IntegrationBranch, Release: release,
		Repos: []RepoCommands{},
		Worker: Worker{
			Backend: file.Worker.Backend, Model: file.Worker.ClaudeCode.Model, Effort: file.Worker.ClaudeCode.Effort,
		},
		Agents:        Agents{Reviewer: roleOf(file.Agents.Reviewer), Resolver: roleOf(file.Agents.Resolver)},
		Review:        Review{ScoreThreshold: positive(file.Review.ScoreThreshold), MergeStrategy: file.Review.MergeStrategy},
		Retries:       Retries{ReviewAttempts: positive(file.Escalation.MaxReviewAttempts)},
		VerifyMinutes: positive(file.Timeouts.VerifyMinutes),
		Engine:        Engine{MaxParallel: positive(file.Engine.MaxParallel), MaxPerBackend: map[string]int{}},
	}
	for backend, limit := range file.Engine.MaxPerBackend {
		cfg.Engine.MaxPerBackend[backend] = limit
	}
	// Zero retries are written -1: a file that leaves the key out reads 0.
	switch retries := file.Escalation.RetryCount; {
	case retries < 0:
		none := 0
		cfg.Retries.Worker = &none
	case retries > 0:
		cfg.Retries.Worker = &retries
	}
	for _, repo := range file.Repos {
		cfg.Repos = append(cfg.Repos, RepoCommands{
			Name: repo.Name, Path: repo.Path, Language: repo.Language,
			Setup: repo.Verify.Setup, Build: repo.Verify.Build, Test: repo.Verify.Test,
		})
	}
	return cfg, nil
}

// roleOf is an agent's settings as the file has them: a number it leaves
// out, or gives as zero, is the default.
func roleOf(r config.AgentRole) AgentRole {
	role := AgentRole{Backend: r.Backend, Model: r.Model, Effort: r.Effort}
	if r.Minutes != 0 {
		minutes := r.Minutes
		role.Minutes = &minutes
	}
	if r.USD != 0 {
		usd := r.USD
		role.USD = &usd
	}
	return role
}

// CheckConfig says what keeps a run from using a configuration file, nil
// when nothing does: what the configuration checks of itself, and the
// settings of the engine's agents, which the engine refuses to start with.
func CheckConfig(data []byte) error {
	cfg, err := config.Parse(layout.ConfigFile, data)
	if err != nil {
		return err
	}
	if problems := cfg.Validate(); len(problems) > 0 {
		lines := make([]string, len(problems))
		for i, problem := range problems {
			lines[i] = problem.Error()
		}
		return fmt.Errorf("%s is not valid: %s", layout.ConfigFile, strings.Join(lines, "; "))
	}
	if _, err := agent.FromConfig(cfg); err != nil {
		return fmt.Errorf("%s is not valid: %w", layout.ConfigFile, err)
	}
	return nil
}

// The sections of a configuration, in the order a new project's has them: a
// section that is added goes where this order puts it.
var configKeys = []string{"version", "project", "repos", "git", "github", "rules", "agents", "worker", "engine", "review", "escalation", "timeouts", "feedback", "hooks", "lessons"}

// ApplyConfig writes the part of a configuration the workbench changes over
// what the file holds, and returns the file as it then is: every other key,
// and the comments, stay as they are. It fails, writing nothing, when a run
// could not use the file, or when a repo it names is not in it.
func ApplyConfig(data []byte, cfg Config) ([]byte, error) {
	doc, err := parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s cannot be read: %w", layout.ConfigFile, err)
	}
	top := document(doc)
	if top.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s is not a mapping of keys", layout.ConfigFile)
	}
	had := keysOf(top)

	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		return nil, errors.New("the project needs a name")
	}
	section(top, "project", func(project *yaml.Node) {
		setString(project, "name", name)
		setOptional(project, "description", strings.TrimSpace(cfg.Description))
	})

	repos := get(top, "repos")
	for _, commands := range cfg.Repos {
		var item *yaml.Node
		if repos != nil && repos.Kind == yaml.SequenceNode {
			for _, candidate := range repos.Content {
				if text(candidate, "name") == commands.Name {
					item = candidate
					break
				}
			}
		}
		if item == nil {
			return nil, fmt.Errorf("the repo %q is not in %s: repos are added and removed in the file", commands.Name, layout.ConfigFile)
		}
		section(item, "verify", func(verify *yaml.Node) {
			hadVerify := keysOf(verify)
			setString(verify, "setup", strings.TrimSpace(commands.Setup))
			setString(verify, "build", strings.TrimSpace(commands.Build))
			setString(verify, "test", strings.TrimSpace(commands.Test))
			arrange(verify, []string{"setup", "build", "test"}, hadVerify)
		})
	}

	section(top, "worker", func(worker *yaml.Node) {
		setOptional(worker, "backend", strings.TrimSpace(cfg.Worker.Backend))
		section(worker, "claude_code", func(claude *yaml.Node) {
			setOptional(claude, "model", strings.TrimSpace(cfg.Worker.Model))
			setOptional(claude, "effort", strings.TrimSpace(cfg.Worker.Effort))
		})
	})
	section(top, "agents", func(agents *yaml.Node) {
		for _, r := range []struct {
			key  string
			role AgentRole
		}{{"reviewer", cfg.Agents.Reviewer}, {"resolver", cfg.Agents.Resolver}} {
			section(agents, r.key, func(role *yaml.Node) {
				had := keysOf(role)
				setOptional(role, "backend", strings.TrimSpace(r.role.Backend))
				setOptional(role, "model", strings.TrimSpace(r.role.Model))
				setOptional(role, "effort", strings.TrimSpace(r.role.Effort))
				// Zero is the default; what is past a role's limit is
				// written, and the check below refuses it.
				minutes := r.role.Minutes
				if minutes != nil && *minutes == 0 {
					minutes = nil
				}
				setOptionalInt(role, "minutes", minutes)
				usd := r.role.USD
				if usd != nil && *usd == 0 {
					usd = nil
				}
				setOptionalFloat(role, "usd", usd)
				arrange(role, []string{"backend", "model", "effort", "minutes", "usd"}, had)
			})
		}
	})
	section(top, "review", func(review *yaml.Node) {
		setOptionalInt(review, "score_threshold", positiveOrNil(cfg.Review.ScoreThreshold))
		setOptional(review, "merge_strategy", strings.TrimSpace(cfg.Review.MergeStrategy))
	})
	section(top, "escalation", func(escalation *yaml.Node) {
		setOptionalInt(escalation, "max_review_attempts", positiveOrNil(cfg.Retries.ReviewAttempts))
		retries := cfg.Retries.Worker
		if retries != nil && *retries <= 0 {
			// None is -1: 0 would read as the default.
			none := -1
			retries = &none
		}
		setOptionalInt(escalation, "retry_count", retries)
	})
	section(top, "timeouts", func(timeouts *yaml.Node) {
		setOptionalInt(timeouts, "verify_minutes", positiveOrNil(cfg.VerifyMinutes))
	})
	section(top, "engine", func(engine *yaml.Node) {
		had := keysOf(engine)
		setOptionalInt(engine, "max_parallel", positiveOrNil(cfg.Engine.MaxParallel))
		// The page sets the backends it knows; one it does not is left as
		// the file has it, and the check below says what is wrong with it.
		section(engine, "max_per_backend", func(limits *yaml.Node) {
			had := keysOf(limits)
			for _, backend := range config.AgentBackends {
				var limit *int
				if n, ok := cfg.Engine.MaxPerBackend[backend]; ok {
					limit = &n
				}
				setOptionalInt(limits, backend, limit)
			}
			arrange(limits, config.AgentBackends, had)
		})
		arrange(engine, []string{"max_parallel", "max_per_backend"}, had)
	})
	arrange(top, configKeys, had)

	out, err := encode(doc, data, topLevelKey)
	if err != nil {
		return nil, err
	}
	if err := CheckConfig(out); err != nil {
		return nil, err
	}
	return out, nil
}

// section edits the mapping under a key: one that was not there is added
// only when something is written to it, and one that ends up empty is taken
// out.
func section(mapping *yaml.Node, key string, edit func(*yaml.Node)) {
	node := child(mapping, key)
	edit(node)
	if len(node.Content) == 0 {
		remove(mapping, key)
	}
}
