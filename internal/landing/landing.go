// Package landing brings a change the workflow made for a person onto the
// project's integration branch: the documents a meeting of the council wrote,
// or the settings changed in the workbench. A Worker starts from the
// integration branch, and a run only from a clean working tree: a change that
// is only in the working tree is seen by neither. It gets there the way every
// change of the project does: on a branch of its own, through a pull request
// that is merged once its checks pass. The working tree is then brought up to
// the branch.
//
// Nothing of the person's working tree is touched until the change is on the
// integration branch, and then only a file that is there exactly as it is
// here: what cannot be brought up is left as it is, and said.
package landing

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/gitlock"
)

// Change is a file of the working tree that is not committed.
type Change struct {
	// Path is the file's path from the root of the repository, with slashes.
	Path string
	// Deleted says the file was removed; Untracked that git does not know it.
	Deleted   bool
	Untracked bool
}

// Runner runs a command in a directory and returns what it printed on its
// standard output; on failure, the error says what it printed on its
// standard error.
type Runner func(ctx context.Context, dir, name string, args ...string) (string, error)

// Exec runs a command on this computer. A git command holds the repository's
// lock while it runs (internal/gitlock): a run's lanes may be at work on the
// same repository.
func Exec(ctx context.Context, dir, name string, args ...string) (string, error) {
	if name != "git" {
		return execute(ctx, dir, name, args...)
	}
	var out string
	var err error
	if lockErr := gitlock.Hold(ctx, dir, func() error {
		out, err = execute(ctx, dir, name, args...)
		return nil
	}); lockErr != nil {
		return "", lockErr
	}
	return out, err
}

func execute(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		said := strings.TrimSpace(stderr.String())
		if said == "" {
			said = strings.TrimSpace(string(out))
		}
		return string(out), fmt.Errorf("%s %s: %w: %s", name, firstWords(args), err, said)
	}
	return string(out), nil
}

// firstWords names a command by its first arguments, for an error.
func firstWords(args []string) string {
	if len(args) > 2 {
		args = args[:2]
	}
	return strings.Join(args, " ")
}

// Pending lists what git says is not committed under the paths given, from
// the repository at root. A file that was renamed counts as one deleted and
// one added.
func Pending(ctx context.Context, run Runner, root string, paths []string) ([]Change, error) {
	if run == nil {
		run = Exec
	}
	args := append([]string{"status", "--porcelain=v1", "-z", "--untracked-files=all", "--"}, paths...)
	out, err := run(ctx, root, "git", args...)
	if err != nil {
		return nil, err
	}
	return parseStatus(out), nil
}

// parseStatus reads `git status --porcelain=v1 -z`: "XY path", NUL after
// each; a rename or copy is followed by the path it came from.
func parseStatus(out string) []Change {
	var changes []Change
	fields := strings.Split(out, "\x00")
	for i := 0; i < len(fields); i++ {
		entry := fields[i]
		if len(entry) < 4 {
			continue
		}
		x, y, path := entry[0], entry[1], entry[3:]
		switch {
		case x == '?' && y == '?':
			changes = append(changes, Change{Path: path, Untracked: true})
		case x == 'R' || x == 'C':
			changes = append(changes, Change{Path: path})
			if i+1 < len(fields) && x == 'R' {
				changes = append(changes, Change{Path: fields[i+1], Deleted: true})
			}
			i++
		case x == 'D' || y == 'D':
			changes = append(changes, Change{Path: path, Deleted: true})
		default:
			changes = append(changes, Change{Path: path})
		}
	}
	return changes
}

// PR is a pull request.
type PR struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
}

// Check is one check of a pull request, by how GitHub sorts it: pass, fail,
// pending, skipping or cancel.
type Check struct {
	Name   string `json:"name"`
	Bucket string `json:"bucket"`
}

// GitHub is what landing asks of GitHub.
type GitHub interface {
	// OpenPR returns the open pull request of a branch, nil when there is none.
	OpenPR(ctx context.Context, root, branch string) (*PR, error)
	CreatePR(ctx context.Context, root, base, branch, title, body string) (*PR, error)
	// Checks lists the checks of a pull request; none when it has none (yet).
	Checks(ctx context.Context, root string, pr int) ([]Check, error)
	// Merge merges a pull request by a strategy (squash, merge or rebase) and
	// deletes its branch.
	Merge(ctx context.Context, root string, pr int, strategy string) error
}

// Options say what to land, and where.
type Options struct {
	// Root is the root of the project's repository.
	Root string
	// Integration is the branch the change goes to.
	Integration string
	// Branch is the branch they go on, which is theirs: it is made again
	// from the integration branch every time.
	Branch string
	// Title and Body are the commit's and the pull request's.
	Title, Body string
	// Strategy is how the pull request is merged: squash (the default),
	// merge or rebase.
	Strategy string
	// Changes are the files to land, as the working tree has them.
	Changes []Change
	// Edit makes the change in the worktree itself, after the Changes are
	// copied in: what is edited is then the file as the integration branch
	// has it, not as the working tree does. It is given the worktree's root.
	Edit func(ctx context.Context, work string) error

	Run    Runner // git; Exec when nil
	GitHub GitHub // gh when nil
	// Progress is told what is being done, a line at a time; OnPR, the pull
	// request once there is one, before its checks are waited for.
	Progress func(string)
	OnPR     func(PR)
	// Poll is how often the checks are looked at (15s); Wait how long they
	// may take (40 minutes). Grace is how long a pull request may show no
	// check before the project is taken to have no CI (90s); Absent how long
	// it may show none when the project has CI for pull requests, before it
	// is said that GitHub did not run it (10 minutes).
	Poll, Wait, Grace, Absent time.Duration
	// Sleep waits and Now tells the time; replaced in tests.
	Sleep func(time.Duration)
	Now   func() time.Time
}

// Landed is what Land did.
type Landed struct {
	// PR is the pull request the change went through; nil when there was
	// nothing to land, the integration branch having them already.
	PR *PR
}

// ChecksFailed says the checks of the pull request did not pass: the change
// is not on the integration branch.
type ChecksFailed struct {
	PR     PR
	Failed []string
}

func (e *ChecksFailed) Error() string {
	return fmt.Sprintf("the checks of pull request #%d did not pass (%s): the change is not on the integration branch; see %s", e.PR.Number, strings.Join(e.Failed, ", "), e.PR.URL)
}

// ChecksAbsent says the project has CI for pull requests into the
// integration branch, and GitHub ran none for this one: it is not merged as
// if its checks had passed.
type ChecksAbsent struct {
	PR   PR
	Wait time.Duration
}

func (e *ChecksAbsent) Error() string {
	return fmt.Sprintf("the project has CI for pull requests into the integration branch, and GitHub started none for pull request #%d in %s: see whether GitHub Actions runs for the repository, or merge it on GitHub yourself, then try again (%s)", e.PR.Number, e.Wait, e.PR.URL)
}

// ChecksTooLong says the checks did not end in the time they were given.
type ChecksTooLong struct {
	PR   PR
	Wait time.Duration
}

func (e *ChecksTooLong) Error() string {
	return fmt.Sprintf("the checks of pull request #%d did not end within %s: try again once they have (%s)", e.PR.Number, e.Wait, e.PR.URL)
}

// MergeRefused says GitHub would not merge the pull request: a review the
// branch's protection asks for, for one. Nothing is forced past it.
type MergeRefused struct {
	PR  PR
	Err error
}

func (e *MergeRefused) Error() string {
	return fmt.Sprintf("GitHub did not merge pull request #%d (%v): merge it on GitHub, then try again (%s)", e.PR.Number, e.Err, e.PR.URL)
}

func (e *MergeRefused) Unwrap() error { return e.Err }

func (o *Options) defaults() {
	if o.Run == nil {
		o.Run = Exec
	}
	if o.GitHub == nil {
		o.GitHub = GH{Run: o.Run}
	}
	if o.Progress == nil {
		o.Progress = func(string) {}
	}
	if o.Poll <= 0 {
		o.Poll = 15 * time.Second
	}
	if o.Wait <= 0 {
		o.Wait = 40 * time.Minute
	}
	if o.Grace <= 0 {
		o.Grace = 90 * time.Second
	}
	if o.Absent <= 0 {
		o.Absent = 10 * time.Minute
	}
	if o.Sleep == nil {
		o.Sleep = time.Sleep
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Strategy == "" {
		o.Strategy = "squash"
	}
}

// Land puts the changes on a branch made from the integration branch as it is
// on origin, opens a pull request for it (or takes the one that is open),
// waits for its checks and merges it. The working tree is not touched: the
// branch is made in a worktree of its own, which is removed again.
func Land(ctx context.Context, opts Options) (*Landed, error) {
	opts.defaults()
	git := func(dir string, args ...string) (string, error) { return opts.Run(ctx, dir, "git", args...) }
	if len(opts.Changes) == 0 && opts.Edit == nil {
		return &Landed{}, nil
	}
	if _, err := git(opts.Root, "fetch", "--quiet", "origin", opts.Integration); err != nil {
		return nil, fmt.Errorf("the integration branch %s cannot be fetched from origin: %w", opts.Integration, err)
	}

	// In the state's folder for what lives for one step: git ignores it,
	// and the workbench does not take a checkout made in it for a change of
	// the run.
	temp := filepath.Join(opts.Root, ".ai", "state", "tmp")
	if err := os.MkdirAll(temp, 0o755); err != nil {
		return nil, err
	}
	work, err := os.MkdirTemp(temp, "land-")
	if err != nil {
		return nil, err
	}
	// The worktree is made in the folder MkdirTemp made, which must be empty
	// for git, and taken away whatever happens.
	if err := os.Remove(work); err != nil {
		return nil, err
	}
	if _, err := git(opts.Root, "worktree", "add", "--quiet", "--detach", work, "origin/"+opts.Integration); err != nil {
		return nil, fmt.Errorf("a worktree of %s cannot be made: %w", opts.Integration, err)
	}
	defer func() {
		_, _ = git(opts.Root, "worktree", "remove", "--force", work)
		_ = os.RemoveAll(work)
		_, _ = git(opts.Root, "worktree", "prune")
	}()

	for _, change := range opts.Changes {
		if err := copyChange(opts.Root, work, change, git); err != nil {
			return nil, err
		}
	}
	if opts.Edit != nil {
		if err := opts.Edit(ctx, work); err != nil {
			return nil, err
		}
	}
	if _, err := git(work, "add", "--all", "--", "."); err != nil {
		return nil, err
	}
	staged, err := git(work, "status", "--porcelain")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(staged) == "" {
		opts.Progress("The integration branch has the change already: nothing to land")
		return &Landed{}, nil
	}
	// The workflows a pull request runs are the integration branch's.
	expect := ExpectsChecks(work, opts.Integration)
	if _, err := git(work, "commit", "--quiet", "-m", opts.Title, "-m", opts.Body); err != nil {
		return nil, fmt.Errorf("the change cannot be committed: %w", err)
	}
	if _, err := git(work, "push", "--quiet", "--force", "origin", "HEAD:refs/heads/"+opts.Branch); err != nil {
		return nil, fmt.Errorf("branch %s cannot be pushed to origin: %w", opts.Branch, err)
	}
	opts.Progress(fmt.Sprintf("Pushed the change to branch %s", opts.Branch))

	pr, err := opts.GitHub.OpenPR(ctx, opts.Root, opts.Branch)
	if err != nil {
		return nil, err
	}
	if pr == nil {
		if pr, err = opts.GitHub.CreatePR(ctx, opts.Root, opts.Integration, opts.Branch, opts.Title, opts.Body); err != nil {
			return nil, fmt.Errorf("the pull request of the change cannot be opened: %w", err)
		}
		opts.Progress(fmt.Sprintf("Opened pull request #%d: %s", pr.Number, pr.URL))
	} else {
		opts.Progress(fmt.Sprintf("Pull request #%d is open already, and has the change as it is now: %s", pr.Number, pr.URL))
	}

	if opts.OnPR != nil {
		opts.OnPR(*pr)
	}
	if err := waitForChecks(ctx, opts, *pr, expect); err != nil {
		return &Landed{PR: pr}, err
	}
	if err := opts.GitHub.Merge(ctx, opts.Root, pr.Number, opts.Strategy); err != nil {
		return &Landed{PR: pr}, &MergeRefused{PR: *pr, Err: err}
	}
	opts.Progress(fmt.Sprintf("Merged pull request #%d into %s", pr.Number, opts.Integration))
	return &Landed{PR: pr}, nil
}

// copyChange makes one change of the working tree in the worktree.
func copyChange(root, work string, change Change, git func(string, ...string) (string, error)) error {
	if change.Deleted {
		_, err := git(work, "rm", "--quiet", "--ignore-unmatch", "--", change.Path)
		return err
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(change.Path)))
	if err != nil {
		return err
	}
	target := filepath.Join(work, filepath.FromSlash(change.Path))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, data, 0o644)
}

// waitForChecks waits until the checks of a pull request ended, and says
// whether they passed. A pull request that shows no check for a while is
// taken to have none, the project having no CI; unless the project has CI
// for pull requests (expect), which GitHub then did not run.
func waitForChecks(ctx context.Context, opts Options, pr PR, expect bool) error {
	start := opts.Now()
	said := ""
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		checks, err := opts.GitHub.Checks(ctx, opts.Root, pr.Number)
		if err != nil {
			return err
		}
		var failed, pending []string
		for _, check := range checks {
			switch check.Bucket {
			case "fail", "cancel":
				failed = append(failed, check.Name)
			case "pending":
				pending = append(pending, check.Name)
			}
		}
		waited := opts.Now().Sub(start)
		switch {
		case len(failed) > 0:
			return &ChecksFailed{PR: pr, Failed: failed}
		case len(checks) == 0 && !expect && waited >= opts.Grace:
			opts.Progress(fmt.Sprintf("Pull request #%d has no checks: the project has no CI for pull requests", pr.Number))
			return nil
		case len(checks) == 0 && expect && waited >= opts.Absent:
			return &ChecksAbsent{PR: pr, Wait: opts.Absent}
		case len(checks) > 0 && len(pending) == 0:
			opts.Progress(fmt.Sprintf("The checks of pull request #%d passed", pr.Number))
			return nil
		case waited >= opts.Wait:
			return &ChecksTooLong{PR: pr, Wait: opts.Wait}
		}
		now := fmt.Sprintf("Waiting for the checks of pull request #%d: %d of %d pending", pr.Number, len(pending), len(checks))
		if len(checks) == 0 && expect {
			now = fmt.Sprintf("Waiting for the CI of pull request #%d to start", pr.Number)
		}
		if now != said && (len(checks) > 0 || expect) {
			opts.Progress(now)
			said = now
		}
		opts.Sleep(opts.Poll)
	}
}

// Synced says whether the working tree was brought up to the integration
// branch, and why not when it was not.
type Synced struct {
	Updated bool
	Why     string
}

// Sync brings the working tree up to the integration branch once the
// changes are on it: each one is checked to be there as it is here, given
// back to git, and the branch moved forward. It does so only when the
// working tree is on the integration branch and has nothing of its own the
// branch lacks; else it leaves everything as it is, and says why.
func Sync(ctx context.Context, run Runner, root, integration string, changes []Change) (Synced, error) {
	if run == nil {
		run = Exec
	}
	git := func(args ...string) (string, error) { return run(ctx, root, "git", args...) }
	branch, err := git("branch", "--show-current")
	if err != nil {
		return Synced{}, err
	}
	if branch = strings.TrimSpace(branch); branch != integration {
		return Synced{Why: fmt.Sprintf("the working tree is on branch %q, not on %s", branch, integration)}, nil
	}
	if _, err := git("fetch", "--quiet", "origin", integration); err != nil {
		return Synced{}, err
	}
	ahead, err := git("rev-list", "--count", "origin/"+integration+"..HEAD")
	if err != nil {
		return Synced{}, err
	}
	if strings.TrimSpace(ahead) != "0" {
		return Synced{Why: fmt.Sprintf("%s here has commits that are not on origin", integration)}, nil
	}
	for _, change := range changes {
		there, err := git("show", "origin/"+integration+":"+change.Path)
		onBranch := err == nil
		if change.Deleted {
			if onBranch {
				return Synced{Why: fmt.Sprintf("%s was removed here and is still on %s", change.Path, integration)}, nil
			}
			continue
		}
		if !onBranch {
			return Synced{Why: fmt.Sprintf("%s is not on %s", change.Path, integration)}, nil
		}
		here, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(change.Path)))
		if err != nil {
			return Synced{}, err
		}
		if !sameText(here, []byte(there)) {
			return Synced{Why: fmt.Sprintf("%s here is not what was merged: it was changed since", change.Path)}, nil
		}
	}
	// Every change is on the branch as it is here: nothing is lost by giving
	// the files back to git and moving forward.
	for _, change := range changes {
		if change.Untracked {
			if err := os.Remove(filepath.Join(root, filepath.FromSlash(change.Path))); err != nil && !errors.Is(err, os.ErrNotExist) {
				return Synced{}, err
			}
			continue
		}
		if _, err := git("checkout", "HEAD", "--", change.Path); err != nil {
			return Synced{}, err
		}
	}
	if _, err := git("merge", "--quiet", "--ff-only", "origin/"+integration); err != nil {
		return Synced{Why: fmt.Sprintf("%s here cannot be moved forward to origin (%v): the change is on origin", integration, err)}, nil
	}
	return Synced{Updated: true}, nil
}

// sameText compares two files as text: a checkout may have turned the line
// ends to CRLF.
func sameText(a, b []byte) bool {
	norm := func(b []byte) []byte { return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")) }
	return bytes.Equal(norm(a), norm(b))
}
