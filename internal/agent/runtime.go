package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/guard"
	"github.com/silver2dream/ai-workflow-kit/internal/trace"
	"github.com/silver2dream/ai-workflow-kit/internal/util"
)

// Envelope is one run's task: the role's settings, where it works, what it
// is told, and the shape its answer must have.
type Envelope struct {
	Profile Profile
	// Dir is where the agent works: a lane's worktree. A role without tools
	// works in an empty folder of its own when it is "".
	Dir    string
	Prompt string
	// Schema is the JSON Schema of the answer; nil for an answer in words.
	Schema json.RawMessage
	// StateRoot is the project: where the run is recorded, and the guard's
	// verdicts with it.
	StateRoot string
	Issue     int
	// RunDir keeps the run's files (its settings, schema, prompt and
	// answer); a temporary folder, removed after, when "".
	RunDir string
	// Protected are the branches nothing is pushed to, for the command
	// policy; AllowControl lets the agent write .claude and .ai, as a
	// ticket's allow_script_changes does.
	Protected    []string
	AllowControl bool
	// Awkit is the awkit whose `guard tool` is the command-policy hook; ""
	// is this one. It is another only where this program is not awkit: a
	// test, run against a build of awkit.
	Awkit string
}

// Status is how a run ended.
type Status string

const (
	StatusOK      Status = "ok"
	StatusFailed  Status = "failed"  // the CLI failed, or said it did
	StatusTimeout Status = "timeout" // the time limit ended it
	StatusBudget  Status = "budget"  // it spent what it may
	StatusSchema  Status = "schema"  // no answer matched the schema, as often as it was asked
	StatusRefused Status = "refused" // the backend cannot be held to the role's settings: it was not run
)

// Usage is what a run took, over every time it was asked.
type Usage struct {
	DurationMS   int64   `json:"duration_ms"`
	Turns        int     `json:"turns,omitempty"`
	InputTokens  int     `json:"input_tokens,omitempty"`
	CachedTokens int     `json:"cached_tokens,omitempty"`
	OutputTokens int     `json:"output_tokens,omitempty"`
	CostUSD      float64 `json:"cost_usd,omitempty"`
}

func (u *Usage) add(o Usage) {
	u.DurationMS += o.DurationMS
	u.Turns += o.Turns
	u.InputTokens += o.InputTokens
	u.CachedTokens += o.CachedTokens
	u.OutputTokens += o.OutputTokens
	u.CostUSD += o.CostUSD
}

// Expanded is what a role's settings became for the CLI: what was run, so
// that what limits a run had can be looked up after it.
type Expanded struct {
	Program string   `json:"program"`
	Args    []string `json:"args"`
	// Settings are the CLI settings the run was given, as written.
	Settings json.RawMessage `json:"settings,omitempty"`
	// Env are the names of the environment variables it had.
	Env []string `json:"env"`
	// Capabilities are what the CLI was found to support, and which of the
	// role's limits it could not be held to (enforced by the runtime
	// instead, or only recorded).
	Unenforced []string `json:"unenforced,omitempty"`
}

// Result is how a run ended and what it answered.
type Result struct {
	Status Status `json:"status"`
	// Output is the structured answer, checked against the schema.
	Output json.RawMessage `json:"output,omitempty"`
	// Text is the answer as the agent gave it.
	Text  string `json:"text,omitempty"`
	Error string `json:"error,omitempty"`
	Usage Usage  `json:"usage"`
	// Tries counts the times it was asked: more than one when an answer did
	// not match the schema.
	Tries int `json:"tries"`
	// Denials counts the tool calls the CLI or the command policy refused.
	Denials  int      `json:"denials,omitempty"`
	Expanded Expanded `json:"expanded"`
}

// invocation is a run of a CLI.
type invocation struct {
	program  string
	args     []string
	stdin    string
	env      []string
	settings json.RawMessage
	// unenforced are the role's limits this CLI cannot be held to.
	unenforced []string
}

// answer is what a CLI's output says.
type answer struct {
	text    string
	output  json.RawMessage // the backend's structured answer, when it has one
	usage   Usage
	denials int
	// failed is the CLI's own word that the run failed; budget, that it
	// stopped at the spend limit.
	failed string
	budget bool
}

// adapter turns a role's settings into a CLI's invocation, and its output
// into an answer.
type adapter interface {
	prepare(env Envelope, files runFiles) (invocation, error)
	parse(stdout, stderr []byte, files runFiles) (answer, error)
}

// runFiles are where one try's files go.
type runFiles struct {
	dir string
}

func (f runFiles) path(name string) string { return filepath.Join(f.dir, name) }

// Runner runs agents.
type Runner struct {
	// Writer is the event stream runs are written to; nil writes to the
	// process's stream, or, when it has none, to the project's agent file.
	Writer *trace.EventWriter
	// adapters, now, environ and minute are replaced in tests.
	adapters map[string]adapter
	now      func() time.Time
	environ  func() []string
	minute   time.Duration
}

// NewRunner is a runner for the backends this machine may have.
func NewRunner() *Runner {
	return &Runner{
		adapters: map[string]adapter{ClaudeCode: &claudeAdapter{}, Codex: &codexAdapter{}},
		now:      time.Now,
		environ:  os.Environ,
		minute:   time.Minute,
	}
}

// Run runs an agent for an envelope. It never returns an error: how the run
// ended is the result's Status, and what went wrong its Error.
func (r *Runner) Run(ctx context.Context, env Envelope) Result {
	res := r.run(ctx, env)
	r.record(env, res)
	return res
}

func (r *Runner) run(ctx context.Context, env Envelope) Result {
	p := env.Profile
	if lim, ok := limits[p.Role]; !ok {
		return Result{Status: StatusRefused, Error: fmt.Sprintf("no agent role %q", p.Role)}
	} else if err := p.check(lim); err != nil {
		return Result{Status: StatusRefused, Error: err.Error()}
	}
	ad, ok := r.adapters[p.Backend]
	if !ok {
		return Result{Status: StatusRefused, Error: fmt.Sprintf("no backend %q", p.Backend)}
	}
	var sch *schema
	if env.Schema != nil {
		s, err := parseSchema(env.Schema)
		if err != nil {
			return Result{Status: StatusRefused, Error: err.Error()}
		}
		sch = s
	}

	runDir := env.RunDir
	if runDir == "" {
		dir, err := os.MkdirTemp("", "awkit-agent-")
		if err != nil {
			return Result{Status: StatusFailed, Error: err.Error()}
		}
		defer os.RemoveAll(dir)
		runDir = dir
	}
	if env.Dir == "" {
		if p.Tools != NoTools {
			return Result{Status: StatusRefused, Error: fmt.Sprintf("the %s works in a folder, and none was given", p.Role)}
		}
		// A role without tools still starts in a folder: an empty one.
		env.Dir = filepath.Join(runDir, "work")
		if err := os.MkdirAll(env.Dir, 0o755); err != nil {
			return Result{Status: StatusFailed, Error: err.Error()}
		}
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(p.Minutes)*r.minute)
	defer cancel()

	var res Result
	prompt := env.Prompt
	for try := 0; try <= p.SchemaRetries; try++ {
		res.Tries = try + 1
		files := runFiles{dir: filepath.Join(runDir, fmt.Sprintf("try-%d", try+1))}
		if err := os.MkdirAll(files.dir, 0o755); err != nil {
			res.Status, res.Error = StatusFailed, err.Error()
			return res
		}
		asked := env
		asked.Prompt = prompt
		inv, err := ad.prepare(asked, files)
		if err != nil {
			res.Status, res.Error = StatusRefused, err.Error()
			return res
		}
		inv.env = guard.AgentEnv(r.environ(), credentialsOf(p.Backend)...)
		res.Expanded = Expanded{Program: inv.program, Args: inv.args, Settings: inv.settings, Env: names(inv.env), Unenforced: inv.unenforced}

		started := r.now()
		stdout, stderr, runErr := r.exec(ctx, env.Dir, inv)
		ans, parseErr := ad.parse(stdout, stderr, files)
		ans.usage.DurationMS = r.now().Sub(started).Milliseconds()
		res.Usage.add(ans.usage)
		res.Denials += ans.denials
		res.Text = ans.text

		switch {
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			res.Status, res.Error = StatusTimeout, fmt.Sprintf("the %s ran past its %d minutes", p.Role, p.Minutes)
			return res
		case ctx.Err() != nil:
			res.Status, res.Error = StatusFailed, "the run was stopped"
			return res
		case ans.budget:
			res.Status, res.Error = StatusBudget, fmt.Sprintf("the %s spent its $%.2f", p.Role, p.USD)
			return res
		case ans.failed != "":
			res.Status, res.Error = StatusFailed, ans.failed
			return res
		case runErr != nil:
			res.Status, res.Error = StatusFailed, failure(runErr, stdout, stderr)
			return res
		case parseErr != nil:
			res.Status, res.Error = StatusFailed, parseErr.Error()
			return res
		}
		if p.USD > 0 && res.Usage.CostUSD > p.USD {
			res.Status, res.Error = StatusBudget, fmt.Sprintf("the %s spent $%.2f of its $%.2f", p.Role, res.Usage.CostUSD, p.USD)
			return res
		}
		if sch == nil {
			res.Status = StatusOK
			return res
		}

		out := ans.output
		if out == nil {
			out = json.RawMessage(strings.TrimSpace(stripFence(ans.text)))
		}
		if err := validate(env.Schema, out); err != nil {
			res.Status, res.Error = StatusSchema, err.Error()
			// Asked again, in a new process, with what was wrong.
			prompt = env.Prompt + "\n\nYour last answer did not match the required JSON schema: " + err.Error() + ". Answer again with only a JSON value that matches it."
			continue
		}
		res.Status, res.Error, res.Output = StatusOK, "", compact(out)
		return res
	}
	return res
}

// exec runs an invocation to its end, and the processes it started with it
// when the time is up.
func (r *Runner) exec(ctx context.Context, dir string, inv invocation) (stdout, stderr []byte, err error) {
	cmd := exec.CommandContext(ctx, inv.program, inv.args...)
	cmd.Dir = dir
	cmd.Env = inv.env
	cmd.Stdin = strings.NewReader(inv.stdin)
	var out, errOut limitedBuffer
	out.max, errOut.max = 32<<20, 1<<20
	cmd.Stdout, cmd.Stderr = &out, &errOut
	cmd.WaitDelay = 10 * time.Second
	err = util.RunWithChildren(cmd)
	return out.Bytes(), errOut.Bytes(), err
}

// limitedBuffer keeps the first max bytes of what is written to it.
type limitedBuffer struct {
	bytes.Buffer
	max int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := b.max - b.Len(); room > 0 {
		if len(p) > room {
			b.Buffer.Write(p[:room])
		} else {
			b.Buffer.Write(p)
		}
	}
	return len(p), nil
}

// failure says why a CLI that exited with an error did: the end of what it
// wrote to standard error, or else of what it wrote to standard output.
func failure(err error, stdout, stderr []byte) string {
	said := strings.TrimSpace(string(stderr))
	if said == "" {
		said = strings.TrimSpace(string(stdout))
	}
	if lines := strings.Split(said, "\n"); len(lines) > 6 {
		said = strings.Join(lines[len(lines)-6:], "\n")
	}
	if r := []rune(said); len(r) > 600 {
		said = "…" + string(r[len(r)-600:])
	}
	if said == "" {
		return err.Error()
	}
	return err.Error() + ": " + said
}

// stripFence takes an answer out of a markdown code fence, the way a model
// that was asked for JSON in words sometimes wraps it.
func stripFence(text string) string {
	t := strings.TrimSpace(text)
	if !strings.HasPrefix(t, "```") {
		return t
	}
	t = strings.TrimPrefix(t, "```")
	if nl := strings.IndexByte(t, '\n'); nl >= 0 {
		t = t[nl+1:]
	}
	return strings.TrimSuffix(strings.TrimSpace(t), "```")
}

func compact(raw json.RawMessage) json.RawMessage {
	var b bytes.Buffer
	if json.Compact(&b, raw) != nil {
		return raw
	}
	return b.Bytes()
}

func credentialsOf(backend string) []string {
	if backend == Codex {
		return guard.CodexCredentialEnv
	}
	return guard.ClaudeCredentialEnv
}

func names(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if eq := strings.IndexByte(kv, '='); eq > 0 {
			out = append(out, kv[:eq])
		}
	}
	return out
}

// Session is the event file of the runs, for a process that has no event
// stream of its own.
const Session = trace.SessionAgent

// The event of a run.
const (
	Component = "agent"
	TypeRun   = "agent_run"
)

// runEvent is what the event log keeps of a run: never the prompt or the
// answer, which may carry what is not to be kept.
type runEvent struct {
	Role     Role     `json:"role"`
	Backend  string   `json:"backend"`
	Model    string   `json:"model,omitempty"`
	Effort   string   `json:"effort,omitempty"`
	Tools    Tools    `json:"tools"`
	Minutes  int      `json:"minutes"`
	USD      float64  `json:"usd,omitempty"`
	Status   Status   `json:"status"`
	Error    string   `json:"error,omitempty"`
	Tries    int      `json:"tries"`
	Denials  int      `json:"denials,omitempty"`
	Usage    Usage    `json:"usage"`
	Expanded Expanded `json:"expanded"`
}

func (r *Runner) record(env Envelope, res Result) {
	w := r.Writer
	if w == nil {
		w = trace.GetGlobalWriter()
	}
	if w == nil {
		if env.StateRoot == "" {
			return
		}
		own, err := trace.NewEventWriter(env.StateRoot, Session)
		if err != nil {
			return
		}
		defer own.Close()
		w = own
	}
	p := env.Profile
	level := trace.LevelInfo
	if res.Status != StatusOK {
		level = trace.LevelWarn
	}
	opts := []trace.EventOption{trace.WithData(runEvent{
		Role: p.Role, Backend: p.Backend, Model: p.Model, Effort: p.Effort, Tools: p.Tools, Minutes: p.Minutes, USD: p.USD,
		Status: res.Status, Error: res.Error, Tries: res.Tries, Denials: res.Denials, Usage: res.Usage, Expanded: res.Expanded,
	})}
	if env.Issue > 0 {
		opts = append(opts, trace.WithIssue(env.Issue))
	}
	_ = w.Write(Component, TypeRun, level, opts...)
}
