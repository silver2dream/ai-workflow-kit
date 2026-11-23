package settings

import (
	"errors"
	"fmt"
	"strings"

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
	Review      Review         `json:"review"`
	Retries     Retries        `json:"retries"`
	// VerifyMinutes bounds one build, test or setup command; nil for the
	// default.
	VerifyMinutes *int `json:"verify_minutes"`
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
	Backends       []string `json:"backends"`
	Strategies     []string `json:"strategies"`
	Efforts        []string `json:"efforts"`
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
		Backends:       []string{"codex", "claude-code"},
		Strategies:     []string{"squash", "merge", "rebase"},
		Efforts:        claudecli.Efforts,
	}
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
		Review:        Review{ScoreThreshold: positive(file.Review.ScoreThreshold), MergeStrategy: file.Review.MergeStrategy},
		Retries:       Retries{ReviewAttempts: positive(file.Escalation.MaxReviewAttempts)},
		VerifyMinutes: positive(file.Timeouts.VerifyMinutes),
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

// CheckConfig says what keeps a run from using a configuration file, nil
// when nothing does.
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
	return nil
}

// The sections of a configuration, in the order a new project's has them: a
// section that is added goes where this order puts it.
var configKeys = []string{"version", "project", "repos", "git", "github", "rules", "agents", "worker", "review", "escalation", "timeouts", "feedback", "hooks", "lessons"}

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
