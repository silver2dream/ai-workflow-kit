package engine

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/agent"
	"github.com/silver2dream/ai-workflow-kit/internal/config"
	"github.com/silver2dream/ai-workflow-kit/internal/ghutil"
	"github.com/silver2dream/ai-workflow-kit/internal/gitlock"
	"github.com/silver2dream/ai-workflow-kit/internal/guard"
	"github.com/silver2dream/ai-workflow-kit/internal/rules"
)

// The engine's resolution of a pull request that conflicts with its base,
// or is behind it (design.md §5.5, task 1.5). The engine rebases the
// branch itself. Where the rebase stops on a conflict, a resolver edits the
// conflicted files and answers whether it resolved them; the engine checks
// that no marker is left and that nothing but the conflicted files changed,
// stages them, continues the rebase, and pushes. The resolver neither
// stages, commits nor pushes. The step says where the lane goes; the
// engine's loop records it.

// ResolveStep resolves the pull request of a lane.
type ResolveStep struct {
	// Root is the project.
	Root string
	// Agents runs the resolver, with Profile as its settings; a Profile
	// without a role is the resolver's defaults.
	Agents  AgentRunner
	Profile agent.Profile
	// GHTimeout bounds each call to GitHub, GitTimeout each git command; 0
	// is a minute and five minutes.
	GHTimeout  time.Duration
	GitTimeout time.Duration

	// Replaced in tests.
	now    func() time.Time
	base   func(ctx context.Context, pr int) (string, error)
	ticket func(ctx context.Context, issue int) (string, error)
}

// maxResolverRounds bounds the commits of one rebase the resolver is asked
// about: a branch that conflicts on more is for a person.
const maxResolverRounds = 8

// maxConflictChars is how much of a conflicted file the resolver is given;
// it reads the file itself for the rest.
const maxConflictChars = 30000

//go:embed resolve_instructions.md
var resolveInstructions string

// What a resolver answers.
const (
	ResolverResolved   = "RESOLVED"
	ResolverTooComplex = "TOO_COMPLEX"
	ResolverFailed     = "FAILED"
)

// resolveSchema is the resolver's answer. Every field is required, as for
// the review (reviewSchema).
const resolveSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["result", "files", "reason"],
  "properties": {
    "result": {"type": "string", "enum": ["RESOLVED", "TOO_COMPLEX", "FAILED"]},
    "files": {"type": "array", "items": {"type": "string"}, "description": "the files you resolved, as git names them"},
    "reason": {"type": "string", "description": "how you resolved them, or why you did not"}
  }
}`

// resolverAnswer is a resolver's answer, as resolveSchema has it.
type resolverAnswer struct {
	Result string   `json:"result"`
	Files  []string `json:"files"`
	Reason string   `json:"reason"`
}

func (s *ResolveStep) defaults() {
	if s.GHTimeout == 0 {
		s.GHTimeout = time.Minute
	}
	if s.GitTimeout == 0 {
		s.GitTimeout = 5 * time.Minute
	}
	if s.Profile.Role == "" {
		s.Profile = agent.Defaults(agent.Resolver, agent.Profile{})
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.base == nil {
		s.base = s.prBase
	}
	if s.ticket == nil {
		s.ticket = s.issueText
	}
}

// Resolve rebases the pull request of a lane an implementer was dispatched
// to resolve (Implementing, with Resolve set) onto its base, has the
// conflicts it stops on resolved, and pushes it. It says where the lane goes:
// back to review when the branch is pushed, back to Resolving when the
// resolution failed (to be tried again, as often as the loop allows), to a
// person when it needs one. The worktree is never left in a rebase. An
// error is the caller's mistake, or the engine stopping (ctx); the lane
// then stays where it is.
//
// The resolver's files are kept under the lane's run folder for this
// dispatch (agent.RunDir(root, issue, lane.Seq)), a folder for each commit
// the rebase stopped on.
func (s *ResolveStep) Resolve(ctx context.Context, lane Lane) (Transition, error) {
	if lane.State != Implementing || lane.Resolve == "" {
		return Transition{}, fmt.Errorf("lane #%d is %s (resolve %q): only a lane dispatched to resolve its pull request is resolved", lane.Issue, show(lane.State), lane.Resolve)
	}
	if lane.PR == 0 {
		return Transition{}, fmt.Errorf("lane #%d has no pull request to resolve", lane.Issue)
	}
	s.defaults()
	r := &resolution{step: s, lane: lane, wt: filepath.Join(s.Root, ".worktrees", "issue-"+strconv.Itoa(lane.Issue)), branch: fmt.Sprintf("feat/ai-issue-%d", lane.Issue)}
	t, err := r.run(ctx)
	// Whatever happened, the worktree is not left in a rebase.
	if r.rebasing() {
		_, _ = r.git(context.WithoutCancel(ctx), "rebase", "--abort")
	}
	if err == nil && ctx.Err() != nil {
		err = ctx.Err()
	}
	return t, err
}

// resolution is one run of the step for a lane.
type resolution struct {
	step     *ResolveStep
	lane     Lane
	wt       string
	branch   string
	base     string
	resolved []string
}

func (r *resolution) move(to State, cause Cause, note string) Transition {
	return Transition{Issue: r.lane.Issue, From: Implementing, To: to, Cause: cause, Note: note, At: r.step.now().UTC()}
}

func (r *resolution) failed(note string) Transition {
	return r.move(Resolving, CauseResolveFailed, note)
}

func (r *resolution) needsYou(why, note string) Transition {
	t := r.move(NeedsYou, CauseEscalated, note)
	t.Why = why
	return t
}

func (r *resolution) run(ctx context.Context) (Transition, error) {
	if err := r.worktree(ctx); err != nil {
		return r.failed("the worktree could not be made ready: " + err.Error()), nil
	}
	base, err := r.step.base(ctx, r.lane.PR)
	if err != nil || base == "" {
		base = r.integrationBranch()
	}
	r.base = base

	// What the remote has of the branch is taken in before anything is
	// rewritten: a person may have pushed to the pull request.
	if _, err := r.git(ctx, "fetch", "origin", "--prune"); err != nil {
		return r.failed("git fetch: " + err.Error()), nil
	}
	lease, _ := r.git(ctx, "rev-parse", "--verify", "--quiet", "refs/remotes/origin/"+r.branch)
	if lease != "" {
		if _, err := r.git(ctx, "merge", "--ff-only", "refs/remotes/origin/"+r.branch); err != nil {
			return r.needsYou(WhyNeedsHumanReview, fmt.Sprintf("the branch %s on GitHub and in the worktree have gone different ways: a person decides which to keep", r.branch)), nil
		}
	}

	if _, err := r.git(ctx, "rebase", "origin/"+r.base); err != nil {
		if ctx.Err() != nil {
			return Transition{}, ctx.Err()
		}
		if len(r.unmerged(ctx)) == 0 {
			return r.failed(fmt.Sprintf("the rebase onto origin/%s failed: %v", r.base, err)), nil
		}
		if t, done := r.resolveAll(ctx); !done {
			return t, ctx.Err()
		}
	}

	// The push replaces only the branch the worktree was based on: what was
	// pushed since is not overwritten.
	args := []string{"push", "--force-with-lease=" + r.branch + ":" + lease, "origin", "HEAD:refs/heads/" + r.branch}
	if lease == "" {
		args = []string{"push", "origin", "HEAD:refs/heads/" + r.branch}
	}
	if _, err := r.git(ctx, args...); err != nil {
		if ctx.Err() != nil {
			return Transition{}, ctx.Err()
		}
		return r.failed("git push: " + err.Error()), nil
	}
	note := "rebased onto origin/" + r.base
	if len(r.resolved) > 0 {
		note += "; the resolver resolved " + strings.Join(r.resolved, ", ")
	}
	return r.move(Reviewing, CauseResolved, note), nil
}

// worktree makes the lane's worktree ready: there, on the lane's branch,
// out of any rebase, with nothing in it but the branch.
func (r *resolution) worktree(ctx context.Context) error {
	if _, err := os.Stat(r.wt); err != nil {
		if _, err := r.gitIn(ctx, r.step.Root, "fetch", "origin", "--prune"); err != nil {
			return err
		}
		if _, err := r.gitIn(ctx, r.step.Root, "worktree", "add", r.wt, r.branch); err != nil {
			if _, err2 := r.gitIn(ctx, r.step.Root, "worktree", "add", "-b", r.branch, r.wt, "origin/"+r.branch); err2 != nil {
				return fmt.Errorf("%v; %v", err, err2)
			}
		}
	}
	if r.rebasing() {
		if _, err := r.git(ctx, "rebase", "--abort"); err != nil {
			return err
		}
	}
	on, err := r.git(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return err
	}
	if on != r.branch {
		return fmt.Errorf("it is on %s, not on the lane's branch %s", on, r.branch)
	}
	if _, err := r.git(ctx, "reset", "--hard", "HEAD"); err != nil {
		return err
	}
	_, err = r.git(ctx, "clean", "-fd")
	return err
}

// resolveAll has the conflicts of each commit the rebase stops on resolved,
// and continues it, to its end. done is false when it did not get there,
// and t is then where the lane goes.
func (r *resolution) resolveAll(ctx context.Context) (t Transition, done bool) {
	ticket := ""
	if text, err := r.step.ticket(ctx, r.lane.Issue); err == nil {
		ticket = text
	}
	for round := 1; r.rebasing(); round++ {
		if ctx.Err() != nil {
			return Transition{}, false
		}
		conflicted := r.unmerged(ctx)
		if len(conflicted) == 0 {
			// A commit whose changes are all in the base already: there is
			// nothing of it to keep.
			if _, err := r.git(ctx, "rebase", "--skip"); err != nil {
				return r.failed("git rebase --skip: " + err.Error()), false
			}
			continue
		}
		if round > maxResolverRounds {
			return r.needsYou(WhyConflictRetries, fmt.Sprintf("the rebase onto origin/%s stopped on more than %d commits of the pull request", r.base, maxResolverRounds)), false
		}
		if t, ok := r.round(ctx, round, conflicted, ticket); !ok {
			return t, false
		}
		for _, f := range conflicted {
			if !slices.Contains(r.resolved, f) {
				r.resolved = append(r.resolved, f)
			}
		}
		if _, err := r.git(ctx, "-c", "core.editor=true", "rebase", "--continue"); err != nil {
			if ctx.Err() != nil {
				return Transition{}, false
			}
			if len(r.unmerged(ctx)) > 0 {
				continue // the next commit conflicts too
			}
			if staged, _ := r.git(ctx, "diff", "--cached", "--name-only"); staged == "" {
				// The resolution left the commit with nothing to add.
				if _, err := r.git(ctx, "rebase", "--skip"); err == nil {
					continue
				}
			}
			return r.failed("git rebase --continue: " + err.Error()), false
		}
	}
	return Transition{}, true
}

// round has the resolver resolve the conflicts of one commit, checks what
// it did, and stages it. ok is false when the lane goes elsewhere: t.
func (r *resolution) round(ctx context.Context, round int, conflicted []string, ticket string) (t Transition, ok bool) {
	c, err := agent.Build(agent.Resolver, r.inputs(ctx, conflicted, ticket), true)
	if err != nil {
		return r.failed("the resolver's context could not be built: " + err.Error()), false
	}
	schema := json.RawMessage(resolveSchema)
	if c.Untrusted {
		if schema, err = agent.SuspiciousSchema(schema); err != nil {
			return r.failed(err.Error()), false
		}
	}
	dir := filepath.Join(agent.RunDir(r.step.Root, r.lane.Issue, r.lane.Seq), "round-"+strconv.Itoa(round))
	cfg := r.config()
	env := agent.Envelope{
		Profile: r.step.Profile, Dir: r.wt, Prompt: c.Prompt, Schema: schema,
		StateRoot: r.step.Root, Issue: r.lane.Issue, RunDir: filepath.Join(dir, "resolver"),
		Protected: []string{cfg.BaseBranch(), cfg.Git.ReleaseBranch},
	}
	if err := agent.SaveEnvelope(dir, env, c, r.step.now().UTC()); err != nil {
		return r.failed("what the resolver is given could not be kept: " + err.Error()), false
	}
	guard.Record(r.step.Root, c.Verdicts, r.lane.Issue)
	if c.Blocked() {
		return r.needsYou(WhyGuardBlocked, "the guard stopped what the resolver was to read: "+guard.Summary(guard.Blocking(c.Verdicts))), false
	}

	before, err := r.snapshot(ctx, conflicted)
	if err != nil {
		return r.failed("the worktree could not be read: " + err.Error()), false
	}
	res := r.step.Agents.Run(ctx, env)
	if ctx.Err() != nil {
		return Transition{}, false
	}
	switch res.Status {
	case agent.StatusOK:
	case agent.StatusRefused:
		return r.needsYou(WhyAgentRefused, "the resolver was not run: "+res.Error), false
	default:
		return r.failed(fmt.Sprintf("the resolver ended %s: %s", res.Status, res.Error)), false
	}
	guard.Record(r.step.Root, c.SuspiciousVerdicts(c.Suspicious(res.Output, res.Text)), r.lane.Issue)

	var answer resolverAnswer
	if err := json.Unmarshal(res.Output, &answer); err != nil {
		return r.failed("the resolver's answer could not be read: " + err.Error()), false
	}
	files := strings.Join(conflicted, ", ")
	switch answer.Result {
	case ResolverResolved:
	case ResolverTooComplex:
		return r.needsYou(WhyNeedsHumanReview, fmt.Sprintf("the conflict with origin/%s in %s needs a person: %s", r.base, files, answer.Reason)), false
	default:
		return r.failed(fmt.Sprintf("the resolver did not resolve %s: %s", files, answer.Reason)), false
	}

	// What it did is checked, not taken from its word.
	after, err := r.snapshot(ctx, conflicted)
	if err != nil {
		return r.failed("the worktree could not be read: " + err.Error()), false
	}
	if after.head != before.head {
		return r.failed("the resolver moved the branch (it committed, or went on with the rebase itself): a resolution is only edits"), false
	}
	outside, companions := r.outside(before, after, conflicted)
	if len(outside) > 0 {
		v := guard.Verdict{
			Result: guard.Block, Class: guard.Security, Stage: guard.StageOutput, Rule: "resolver-scope",
			Source: fmt.Sprintf("PR #%d resolution", r.lane.PR),
			Reason: "the resolver changed files that were not in conflict: " + strings.Join(outside, ", "),
		}
		guard.Record(r.step.Root, []guard.Verdict{v}, r.lane.Issue)
		return r.needsYou(WhyGuardOutput, v.Reason+"; nothing was pushed"), false
	}
	if marked := r.markersLeft(conflicted); len(marked) > 0 {
		return r.failed("conflict markers are left in " + strings.Join(marked, ", ")), false
	}
	if _, err := r.git(ctx, append([]string{"add", "-A", "--"}, append(conflicted, companions...)...)...); err != nil {
		return r.failed("git add: " + err.Error()), false
	}
	if left := r.unmerged(ctx); len(left) > 0 {
		return r.failed("still in conflict after the resolution: " + strings.Join(left, ", ")), false
	}
	// What its checks left behind (a binary, a cache) goes: it is not part
	// of the change, and the review's run of the tests is not to see it.
	if _, err := r.git(ctx, "clean", "-fd"); err != nil {
		return r.failed("git clean: " + err.Error()), false
	}
	return Transition{}, true
}

// inputs are what the resolver is given (design.md §5.6): its instructions,
// each conflicted file with its markers and the base's recent commits on
// it, the ticket in short, and the project's rules for those files.
func (r *resolution) inputs(ctx context.Context, conflicted []string, ticket string) []agent.Input {
	in := []agent.Input{{Kind: agent.KindInstructions, Source: "awkit", Title: "Your task", Text: strings.NewReplacer(
		"{{pr}}", strconv.Itoa(r.lane.PR),
		"{{issue}}", strconv.Itoa(r.lane.Issue),
		"{{base}}", r.base,
	).Replace(strings.TrimSpace(resolveInstructions))}}
	if ticket != "" {
		if len(ticket) > 3000 {
			ticket = ticket[:3000] + "\n(the rest of the ticket is left out)"
		}
		in = append(in, agent.Input{Kind: agent.KindTicketSummary, Source: fmt.Sprintf("issue #%d", r.lane.Issue), Title: "The ticket, in short", Text: ticket})
	}
	if all, err := rules.LoadAll(r.step.Root); err == nil {
		for _, rule := range rules.For(all, conflicted...) {
			in = append(in, agent.Input{Kind: agent.KindRules, Source: rule.File, Title: rule.Title, Text: rule.Text})
		}
	}
	replaying, _ := r.git(ctx, "log", "-1", "--format=%h %s", "REBASE_HEAD")
	for _, file := range conflicted {
		text := "(the file is not in the worktree: one side deleted it)"
		if data, err := os.ReadFile(filepath.Join(r.wt, filepath.FromSlash(file))); err == nil {
			text = string(data)
			if len(text) > maxConflictChars {
				text = text[:maxConflictChars] + "\n(the rest of the file is left out: read it)"
			}
		}
		history, _ := r.git(ctx, "log", "-5", "--format=%h %s", "origin/"+r.base, "--", file)
		if history != "" {
			text = "The base's latest commits on this file:\n" + history + "\n\nThe file as the rebase left it:\n" + text
		}
		if replaying != "" {
			text = "The pull request's commit being replayed: " + replaying + "\n\n" + text
		}
		in = append(in, agent.Input{Kind: agent.KindConflict, Source: file, Title: "Conflicted: " + file, Text: text, MaxChars: maxConflictChars + 2000})
	}
	return in
}

// snapshot is what the worktree holds outside the conflicted files: the
// commit, the index of every other file, what differs from the index, and
// the untracked files.
type snapshot struct {
	head      string
	index     map[string]string
	changed   []string
	untracked []string
}

func (r *resolution) snapshot(ctx context.Context, conflicted []string) (snapshot, error) {
	var s snapshot
	var err error
	if s.head, err = r.git(ctx, "rev-parse", "HEAD"); err != nil {
		return s, err
	}
	staged, err := r.git(ctx, "ls-files", "-s", "-z")
	if err != nil {
		return s, err
	}
	s.index = map[string]string{}
	for _, entry := range strings.Split(staged, "\x00") {
		meta, file, ok := strings.Cut(entry, "\t")
		if ok && !slices.Contains(conflicted, file) {
			s.index[file] += meta + ";"
		}
	}
	changed, err := r.git(ctx, "diff", "--name-only", "-z")
	if err != nil {
		return s, err
	}
	s.changed = nulSplit(changed)
	untracked, err := r.git(ctx, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return s, err
	}
	s.untracked = nulSplit(untracked)
	return s, nil
}

func nulSplit(z string) []string {
	var out []string
	for _, n := range strings.Split(z, "\x00") {
		if n != "" {
			out = append(out, n)
		}
	}
	return out
}

// companions are the files a manifest's tool writes with it: when the
// manifest is in conflict, they may change with its resolution.
var companions = map[string][]string{
	"go.mod":         {"go.sum"},
	"package.json":   {"package-lock.json", "npm-shrinkwrap.json", "yarn.lock", "pnpm-lock.yaml"},
	"Cargo.toml":     {"Cargo.lock"},
	"pyproject.toml": {"poetry.lock", "uv.lock"},
	"Gemfile":        {"Gemfile.lock"},
	"composer.json":  {"composer.lock"},
}

// outside are the tracked files the resolver changed, in the worktree or
// the index, that it may not have: any but the conflicted ones and their
// manifests' companions. allowed are the companions it did change or make,
// to be staged with the resolution.
func (r *resolution) outside(before, after snapshot, conflicted []string) (outside, allowed []string) {
	may := func(file string) bool {
		if slices.Contains(conflicted, file) {
			return true
		}
		for _, c := range conflicted {
			if slices.Contains(companions[path.Base(c)], path.Base(file)) && path.Dir(c) == path.Dir(file) {
				return true
			}
		}
		return false
	}
	add := func(file string) {
		switch {
		case slices.Contains(conflicted, file):
		case may(file):
			if !slices.Contains(allowed, file) {
				allowed = append(allowed, file)
			}
		case !slices.Contains(outside, file):
			outside = append(outside, file)
		}
	}
	for file, entry := range after.index {
		if before.index[file] != entry {
			add(file)
		}
	}
	for file := range before.index {
		if _, ok := after.index[file]; !ok {
			add(file)
		}
	}
	for _, file := range after.changed {
		if !slices.Contains(before.changed, file) {
			add(file)
		}
	}
	// A file it made is not committed (only the resolution is staged) and is
	// cleaned away after it; a companion it made, a lockfile, is staged.
	for _, file := range after.untracked {
		if !slices.Contains(before.untracked, file) && may(file) {
			add(file)
		}
	}
	slices.Sort(outside)
	slices.Sort(allowed)
	return outside, allowed
}

// markersLeft are the conflicted files that still hold a conflict marker.
func (r *resolution) markersLeft(conflicted []string) []string {
	var marked []string
	for _, file := range conflicted {
		data, err := os.ReadFile(filepath.Join(r.wt, filepath.FromSlash(file)))
		if err != nil {
			continue // resolved by deleting it
		}
		if hasMarker(data) {
			marked = append(marked, file)
		}
	}
	return marked
}

// hasMarker says whether text holds a line git writes to mark a conflict.
// A lone ======= is not one: it underlines a Markdown heading.
func hasMarker(text []byte) bool {
	for _, line := range bytes.Split(text, []byte("\n")) {
		line = bytes.TrimRight(line, "\r")
		for _, m := range [][]byte{[]byte("<<<<<<<"), []byte(">>>>>>>"), []byte("|||||||")} {
			if bytes.HasPrefix(line, m) && (len(line) == len(m) || line[len(m)] == ' ') {
				return true
			}
		}
	}
	return false
}

// unmerged are the files in conflict.
func (r *resolution) unmerged(ctx context.Context) []string {
	out, err := r.git(ctx, "diff", "--name-only", "--diff-filter=U", "-z")
	if err != nil {
		return nil
	}
	return nulSplit(out)
}

// rebasing says whether the worktree is in the middle of a rebase.
func (r *resolution) rebasing() bool {
	for _, name := range []string{"rebase-merge", "rebase-apply"} {
		dir, err := r.git(context.Background(), "rev-parse", "--git-path", name)
		if err != nil {
			continue
		}
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(r.wt, dir)
		}
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

func (r *resolution) git(ctx context.Context, args ...string) (string, error) {
	return r.gitIn(ctx, r.wt, args...)
}

// gitIn runs git in dir, with no editor and no prompt, and returns what it
// printed, trimmed; the error carries what it said.
func (r *resolution) gitIn(ctx context.Context, dir string, args ...string) (string, error) {
	// The repository's lock is held while it runs: lanes run side by side,
	// on worktrees of one repository (internal/gitlock).
	out, err := gitlock.Command{
		Dir: dir, Args: args, Timeout: r.step.GitTimeout,
		Env: []string{"GIT_EDITOR=true", "GIT_TERMINAL_PROMPT=0"},
	}.Run(ctx)
	if err != nil {
		var failed *gitlock.Error
		if !errors.As(err, &failed) {
			// The lock was not taken.
			return strings.TrimSpace(out), err
		}
		said := failed.Stderr
		if said == "" {
			said = strings.TrimSpace(out)
		}
		if said != "" {
			return strings.TrimSpace(out), fmt.Errorf("%w: %s", failed.Err, said)
		}
		return strings.TrimSpace(out), failed.Err
	}
	return strings.TrimRight(out, "\n\r "), nil
}

func (r *resolution) config() *config.Config {
	cfg, err := config.Load(r.step.Root)
	if err != nil {
		cfg = config.Default()
	}
	return cfg
}

func (r *resolution) integrationBranch() string { return r.config().BaseBranch() }

// prBase is the base branch of a pull request, as GitHub has it.
func (s *ResolveStep) prBase(ctx context.Context, pr int) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, s.GHTimeout)
	defer cancel()
	out, err := ghutil.RunWithRetry(ctx, ghutil.DefaultRetryConfig(), "gh", "pr", "view", strconv.Itoa(pr), "--json", "baseRefName", "--jq", ".baseRefName")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// issueText is a ticket's title and body, as GitHub has them.
func (s *ResolveStep) issueText(ctx context.Context, issue int) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, s.GHTimeout)
	defer cancel()
	out, err := ghutil.RunWithRetry(ctx, ghutil.DefaultRetryConfig(), "gh", "issue", "view", strconv.Itoa(issue), "--json", "title,body")
	if err != nil {
		return "", err
	}
	var it struct{ Title, Body string }
	if err := json.Unmarshal(out, &it); err != nil {
		return "", err
	}
	if it.Title == "" && it.Body == "" {
		return "", errors.New("the issue has no title or body")
	}
	return "# " + it.Title + "\n\n" + it.Body, nil
}
