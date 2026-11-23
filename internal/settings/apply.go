package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/config"
	"github.com/silver2dream/ai-workflow-kit/internal/council"
	"github.com/silver2dream/ai-workflow-kit/internal/landing"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
)

// A change of the settings reaches the project the way the documents of a
// meeting do (package landing): it is made to the files as the integration
// branch has them, in a worktree of their own, with what `awkit generate`
// writes from the configuration; it goes through a pull request merged once
// its checks pass; and the working tree is then brought up to the branch.
// Until then, the files in the working tree are not touched: a run started
// meanwhile works with the settings as they were.

// Request is a change of the settings asked for: the roster, the
// configuration, or both. What it leaves nil is not changed.
type Request struct {
	Roster *Roster `json:"roster,omitempty"`
	Config *Config `json:"config,omitempty"`
	// WasRoster and WasConfig are the settings the change was made to, as
	// the page showed them. Where there is one, only what the change made
	// different from it is changed; the rest is as the integration branch
	// has it. Without one, the change is taken whole.
	WasRoster *Roster `json:"was_roster,omitempty"`
	WasConfig *Config `json:"was_config,omitempty"`
}

// What names what a request changes: roster, config.
func (r Request) What() []string {
	var what []string
	if r.Roster != nil {
		what = append(what, "roster")
	}
	if r.Config != nil {
		what = append(what, "config")
	}
	return what
}

// Record is where a change of the settings is: `awkit settings apply`
// writes it as it goes, the workbench follows it.
type Record struct {
	PID int `json:"pid"`
	// State is working, done or failed.
	State string `json:"state"`
	// Step is where it is, or stopped: check, change, checks, sync, done.
	Step string `json:"step"`
	// What the change is of: roster, config.
	What []string `json:"what"`
	// Why names, for a page, how it stopped when it failed: invalid (a run or
	// a meeting could not use the settings), conflict (what was changed was
	// changed on the integration branch too), run (a run is going), branch
	// (the integration branch is not on origin), checks, noci, slow, refused
	// (as for an approval), or error.
	Why   string `json:"why,omitempty"`
	Error string `json:"error,omitempty"`
	// PR is the pull request the change went through.
	PR  int    `json:"pr,omitempty"`
	URL string `json:"url,omitempty"`
	// Unchanged says the integration branch had these settings already.
	Unchanged bool `json:"unchanged,omitempty"`
	// Left says why the working tree was not brought up to the integration
	// branch, when it was not.
	Left    string    `json:"left,omitempty"`
	Lines   []string  `json:"lines"`
	Started time.Time `json:"started"`
	Updated time.Time `json:"updated"`
}

// The states of a change of the settings.
const (
	Working = "working"
	Done    = "done"
	Failed  = "failed"
)

// Branch is the branch a change of the settings goes on. It is one: a change
// made while the pull request of one before is open goes into it.
const Branch = "feat/awk-settings"

const maxLines = 40

// Say keeps a line of what the change said.
func (r *Record) Say(line string) {
	r.Lines = append(r.Lines, line)
	if len(r.Lines) > maxLines {
		r.Lines = r.Lines[len(r.Lines)-maxLines:]
	}
}

// Dir is where a change of the settings keeps its request, its record and
// its log.
func Dir(root string) string { return layout.State(root, "settings") }

// RequestFile, RecordFile and LogFile are in Dir.
const (
	RequestFile = "request.json"
	RecordFile  = "apply.json"
	LogFile     = "apply.log"
)

// ReadRecord reads where the last change of the settings is.
func ReadRecord(root string) (*Record, error) {
	data, err := os.ReadFile(filepath.Join(Dir(root), RecordFile))
	if err != nil {
		return nil, err
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

// WriteRecord writes where a change of the settings is, whole.
func WriteRecord(root string, record *Record) error {
	record.Updated = time.Now()
	if record.Lines == nil {
		record.Lines = []string{}
	}
	if record.What == nil {
		record.What = []string{}
	}
	return writeJSON(filepath.Join(Dir(root), RecordFile), record)
}

// ReadRequest reads a request from its file.
func ReadRequest(file string) (*Request, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var request Request
	if err := json.Unmarshal(data, &request); err != nil {
		return nil, fmt.Errorf("%s is not a request: %w", file, err)
	}
	return &request, nil
}

// WriteRequest writes a request where `awkit settings apply` is told to read
// it.
func WriteRequest(root string, request Request) (string, error) {
	file := filepath.Join(Dir(root), RequestFile)
	return file, writeJSON(file, request)
}

func writeJSON(file string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	temp := file + ".tmp"
	if err := os.WriteFile(temp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(temp, file)
}

// Files are a project's settings as they are in a folder: the roster (the
// one the project would start with when it has none) and the configuration.
type Files struct {
	Roster        []byte
	RosterMissing bool
	Config        []byte
}

// ReadFiles reads the settings of the project at root.
func ReadFiles(root string) (*Files, error) {
	files := &Files{}
	var err error
	if files.Config, err = os.ReadFile(layout.Config(root)); err != nil {
		return nil, err
	}
	files.Roster, err = os.ReadFile(layout.Roster(root))
	if errors.Is(err, os.ErrNotExist) {
		files.RosterMissing = true
		files.Roster, err = council.StartingRoster(reposOf(files.Config))
	}
	if err != nil {
		return nil, err
	}
	return files, nil
}

// reposOf lists the repos of a configuration as the council knows them.
func reposOf(data []byte) []council.Repo {
	cfg, err := config.Parse(layout.ConfigFile, data)
	if err != nil {
		return nil
	}
	repos := make([]council.Repo, 0, len(cfg.Repos))
	for _, repo := range cfg.Repos {
		dir := repo.Path
		if repo.Type == "root" {
			dir = "./"
		}
		repos = append(repos, council.Repo{Name: repo.Name, Language: repo.Language, Path: dir})
	}
	return repos
}

// Change returns the settings files of a folder as the request has them,
// only the ones it changes. It fails when a run or a meeting could not use
// them, or with a *Conflict when the folder has a change of its own to what
// the request changes.
func (r Request) Change(files *Files) (roster, cfg []byte, err error) {
	if r.Roster != nil {
		mine := r.Roster
		if r.WasRoster != nil {
			theirs, err := ReadRoster(files.Roster)
			if err != nil {
				return nil, nil, err
			}
			if mine, err = mergeRoster(r.WasRoster, r.Roster, theirs); err != nil {
				return nil, nil, err
			}
		}
		if roster, err = ApplyRoster(files.Roster, *mine); err != nil {
			return nil, nil, fmt.Errorf("the roster: %w", err)
		}
	}
	if r.Config != nil {
		mine := r.Config
		if r.WasConfig != nil {
			theirs, err := ReadConfig(files.Config)
			if err != nil {
				return nil, nil, err
			}
			if mine, err = mergeConfig(r.WasConfig, r.Config, theirs); err != nil {
				return nil, nil, err
			}
		}
		if cfg, err = ApplyConfig(files.Config, *mine); err != nil {
			return nil, nil, fmt.Errorf("the configuration: %w", err)
		}
	}
	return roster, cfg, nil
}

// ApplyOptions say what to change, and with what.
type ApplyOptions struct {
	Root    string
	Request Request
	// Run runs git, and GitHub asks GitHub; landing's own when nil.
	Run    landing.Runner
	GitHub landing.GitHub
	// RunGoing says whether a run is going in the project.
	RunGoing func(root string) bool
	// Generate writes what follows from the configuration of the project at
	// a root, as `awkit generate` does: in the worktree the change is made
	// in. Local writes what of it git does not keep, in the working tree
	// once it is brought up: the rest came with the branch, and is not
	// written again (a file written again with other line ends than the
	// checkout gave it shows as changed, and a run does not start).
	Generate func(root string) error
	Local    func(root string) error
	// Landing sets how long checks are waited for; for tests.
	Landing func(*landing.Options)
	// Save is told the record whenever it changed; Say, every line.
	Save func(*Record)
	Say  func(string)
}

// Apply changes the settings of a project as the request says: on the
// integration branch through a pull request, then in the working tree. It
// returns the record of what it did, failed or not.
func Apply(ctx context.Context, opts ApplyOptions) *Record {
	if opts.Run == nil {
		opts.Run = landing.Exec
	}
	if opts.Save == nil {
		opts.Save = func(*Record) {}
	}
	record := &Record{PID: os.Getpid(), State: Working, Step: "check", What: opts.Request.What(), Started: time.Now()}
	save := func() { opts.Save(record) }
	say := func(line string) {
		if opts.Say != nil {
			opts.Say(line)
		}
		record.Say(line)
		save()
	}
	step := func(name string) {
		record.Step = name
		save()
	}
	fail := func(why string, err error) *Record {
		record.State, record.Why, record.Error = Failed, why, err.Error()
		save()
		return record
	}
	save()

	if len(record.What) == 0 {
		return fail("invalid", errors.New("the request changes nothing"))
	}
	// What is asked for is checked against the settings here first: what a
	// run could not use is refused before anything is pushed.
	here, err := ReadFiles(opts.Root)
	if err != nil {
		return fail("error", fmt.Errorf("the settings of the project cannot be read: %w", err))
	}
	if _, _, err := opts.Request.Change(here); err != nil {
		var conflict *Conflict
		if errors.As(err, &conflict) {
			return fail("conflict", err)
		}
		return fail("invalid", err)
	}
	cfg, err := config.Load(opts.Root)
	if err != nil {
		return fail("error", fmt.Errorf("the project's config cannot be read: %w", err))
	}
	integration := cfg.Git.IntegrationBranch
	if opts.RunGoing != nil && opts.RunGoing(opts.Root) {
		return fail("run", errors.New("a run is going in this project: change the settings once it has stopped (awkit stop), so that it does not take up a ticket half way with them"))
	}
	if _, err := opts.Run(ctx, opts.Root, "git", "ls-remote", "--exit-code", "--heads", "origin", integration); err != nil {
		return fail("branch", fmt.Errorf("the integration branch %s is not on origin, or origin cannot be reached: the settings cannot be merged into it (%v)", integration, err))
	}

	step("change")
	request := opts.Request
	edit := func(ctx context.Context, work string) error {
		there, err := ReadFiles(work)
		if err != nil {
			return fmt.Errorf("the settings on %s cannot be read: %w", integration, err)
		}
		roster, configured, err := request.Change(there)
		if err != nil {
			return fmt.Errorf("the settings as %s has them cannot be changed so: %w", integration, err)
		}
		if roster != nil {
			if err := writeFile(layout.Roster(work), roster); err != nil {
				return err
			}
		}
		if configured != nil {
			if err := writeFile(layout.Config(work), configured); err != nil {
				return err
			}
			if opts.Generate != nil {
				if err := opts.Generate(work); err != nil {
					return fmt.Errorf("awkit generate failed on the changed configuration: %w", err)
				}
			}
		}
		return nil
	}
	options := landing.Options{
		Root: opts.Root, Integration: integration, Branch: Branch,
		Title:    "[chore] settings: " + titleOf(record.What),
		Body:     bodyOf(record.What, integration),
		Strategy: cfg.Review.Strategy(), Edit: edit,
		Run: opts.Run, GitHub: opts.GitHub, Progress: say,
		OnPR: func(pr landing.PR) {
			record.PR, record.URL = pr.Number, pr.URL
			step("checks")
		},
	}
	if opts.Landing != nil {
		opts.Landing(&options)
	}
	landed, err := landing.Land(ctx, options)
	if err != nil {
		var failed *landing.ChecksFailed
		var absent *landing.ChecksAbsent
		var slow *landing.ChecksTooLong
		var refused *landing.MergeRefused
		var conflict *Conflict
		switch {
		case errors.As(err, &conflict):
			return fail("conflict", err)
		case errors.As(err, &failed):
			return fail("checks", err)
		case errors.As(err, &absent):
			return fail("noci", err)
		case errors.As(err, &slow):
			return fail("slow", err)
		case errors.As(err, &refused):
			return fail("refused", err)
		}
		return fail("error", err)
	}
	if landed.PR == nil {
		record.Unchanged = true
		say(integration + " has these settings already")
	}

	step("sync")
	synced, err := landing.Sync(ctx, opts.Run, opts.Root, integration, nil)
	switch {
	case err != nil:
		record.Left = err.Error()
	case !synced.Updated:
		record.Left = synced.Why
	default:
		say(fmt.Sprintf("The working tree is up to %s", integration))
		// What a run needs on this computer only, and git does not keep,
		// follows the configuration too.
		if opts.Request.Config != nil && opts.Local != nil {
			if err := opts.Local(opts.Root); err != nil {
				record.Left = "Claude Code's local settings cannot be written: " + err.Error()
			}
		}
	}
	record.State, record.Step = Done, "done"
	save()
	return record
}

func writeFile(file string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	return os.WriteFile(file, data, 0o644)
}

func titleOf(what []string) string {
	var parts []string
	for _, w := range what {
		switch w {
		case "roster":
			parts = append(parts, "the council's roster")
		case "config":
			parts = append(parts, "the project's configuration")
		}
	}
	return strings.Join(parts, " and ")
}

func bodyOf(what []string, integration string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Changed in the AWK workbench: %s.\n\n", titleOf(what))
	for _, w := range what {
		if w == "config" {
			fmt.Fprintf(&b, "`awkit generate` wrote again what follows from the configuration (CLAUDE.md, AGENTS.md, the kit's rules and the CI workflows it keeps).\n\n")
		}
	}
	fmt.Fprintf(&b, "It is merged into `%s` once its checks pass, and the working tree is then brought up to it.\n", integration)
	return b.String()
}
