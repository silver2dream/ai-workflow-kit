// Package gitlock keeps the git commands AWK runs on one repository from
// running into each other (design.md §5.3, task 2.2b).
//
// Lanes run side by side, and each Worker is a process of its own (`awkit
// run-issue`) on a worktree of the same repository: they share its refs, its
// configuration and, for the commands that run in the main working tree, its
// index. Two fetches at once fight over refs/remotes, two `push -u` over
// .git/config, and a Worker that removes an index.lock it takes for stale
// breaks the index of a command that holds it. So every git command AWK runs
// holds the repository's lock while it runs: an operating system lock on a
// file in the repository's common git directory, which a worktree shares with
// its main working tree. The system lets the lock go when its process ends,
// so a lock is never left behind. It is held for one command at a time, never
// from one to the next.
//
// What an agent runs in its worktree does not take the lock; a command that
// meets one of git's own locks taken by it is tried again a little later.
package gitlock

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// FileName is the lock's file, in the repository's common git directory.
const FileName = "awkit-git.lock"

// Path is the lock of the repository a git command run in dir works on: the
// file in its common git directory. A worktree's is its main working tree's;
// a submodule is a repository of its own. "" when dir is in no repository.
func Path(dir string) string {
	common := commonDir(dir)
	if common == "" {
		return ""
	}
	return filepath.Join(common, FileName)
}

// commonDir is the common git directory of the repository dir is in, found
// as git finds it: the nearest .git above dir, a directory or a file that
// names one ("gitdir: …"), whose commondir file names the common one.
func commonDir(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	for d := abs; ; {
		dotgit := filepath.Join(d, ".git")
		info, err := os.Stat(dotgit)
		if err == nil {
			if info.IsDir() {
				return dotgit
			}
			return linked(d, dotgit)
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
}

// linked is the common git directory a .git file names.
func linked(worktree, dotgit string) string {
	data, err := os.ReadFile(dotgit)
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(strings.SplitN(string(data), "\n", 2)[0])
	gitdir, ok := strings.CutPrefix(line, "gitdir:")
	if !ok {
		return ""
	}
	gitdir = strings.TrimSpace(gitdir)
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(worktree, gitdir)
	}
	data, err = os.ReadFile(filepath.Join(gitdir, "commondir"))
	if err != nil {
		// A submodule's git directory, or a worktree git made without one:
		// the directory is its own.
		return filepath.Clean(gitdir)
	}
	common := strings.TrimSpace(string(data))
	if !filepath.IsAbs(common) {
		common = filepath.Join(gitdir, common)
	}
	return filepath.Clean(common)
}

// Within this process the lock is a turn: whoever holds it, of the
// goroutines that want it, before the file is locked for the others.
var (
	turnsMu sync.Mutex
	turns   = map[string]chan struct{}{}
)

func turn(path string) chan struct{} {
	turnsMu.Lock()
	defer turnsMu.Unlock()
	t, ok := turns[path]
	if !ok {
		t = make(chan struct{}, 1)
		turns[path] = t
	}
	return t
}

// pollFrom and pollTo bound how often a lock another process holds is tried.
const (
	pollFrom = 10 * time.Millisecond
	pollTo   = 200 * time.Millisecond
)

// Hold runs fn while it holds the lock of the repository dir is in, and
// waits for it as long as ctx lets it. fn must not take the lock again: a
// git command in it is run with Plain. Outside a repository fn runs as it is.
func Hold(ctx context.Context, dir string, fn func() error) error {
	path := Path(dir)
	if path == "" {
		return fn()
	}
	release, err := acquire(ctx, path)
	if err != nil {
		return err
	}
	defer release()
	return fn()
}

func acquire(ctx context.Context, path string) (func(), error) {
	t := turn(path)
	select {
	case t <- struct{}{}:
	case <-ctx.Done():
		return nil, fmt.Errorf("waiting for the git lock %s: %w", path, ctx.Err())
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		<-t
		return nil, fmt.Errorf("the git lock %s cannot be opened: %w", path, err)
	}
	wait := pollFrom
	for {
		ok, err := tryLock(f)
		if err != nil {
			f.Close()
			<-t
			return nil, fmt.Errorf("the git lock %s cannot be taken: %w", path, err)
		}
		if ok {
			break
		}
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			f.Close()
			<-t
			return nil, fmt.Errorf("waiting for the git lock %s, held by another process: %w", path, ctx.Err())
		}
		wait = min(2*wait, pollTo)
	}
	return func() {
		unlock(f)
		f.Close()
		<-t
	}, nil
}

// Command is a git command AWK runs.
type Command struct {
	// Dir is where it runs (git -C).
	Dir  string
	Args []string
	// Env is added to the process's environment.
	Env []string
	// Timeout bounds one try; 0 is none.
	Timeout time.Duration
}

// Error is a git command that failed: what it printed on its standard error
// is in its message. It unwraps to the process's error (*exec.ExitError).
type Error struct {
	Args   []string
	Err    error
	Stderr string
}

func (e *Error) Error() string {
	msg := fmt.Sprintf("git %s: %v", firstWords(e.Args), e.Err)
	if e.Stderr != "" {
		msg += ": " + e.Stderr
	}
	return msg
}

func (e *Error) Unwrap() error { return e.Err }

func firstWords(args []string) string {
	if len(args) > 2 {
		args = args[:2]
	}
	return strings.Join(args, " ")
}

// Tries bounds how often a command that met one of git's own locks is run;
// backoff is the wait before the second try, doubled for each one after.
var (
	Tries   = 5
	backoff = 250 * time.Millisecond
)

// Run runs the command while it holds the repository's lock, and returns
// what it printed on its standard output. A command that meets one of git's
// own locks, taken by a process that does not take this one (an agent in its
// worktree, a person), is run again a little later, with the lock let go in
// between.
func (c Command) Run(ctx context.Context) (string, error) {
	wait := backoff
	for try := 1; ; try++ {
		var out string
		var err error
		if lockErr := Hold(ctx, c.Dir, func() error {
			out, err = c.plain(ctx)
			return nil
		}); lockErr != nil {
			return "", lockErr
		}
		if err == nil || try >= Tries || !Contended(err) || ctx.Err() != nil {
			return out, err
		}
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return out, err
		}
		wait *= 2
	}
}

// Plain runs the command without the lock: inside Hold.
func (c Command) Plain(ctx context.Context) (string, error) {
	return c.plain(ctx)
}

func (c Command) plain(ctx context.Context) (string, error) {
	if c.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
	}
	args := c.Args
	if c.Dir != "" {
		args = append([]string{"-C", c.Dir}, args...)
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	if len(c.Env) > 0 {
		cmd.Env = append(os.Environ(), c.Env...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), &Error{Args: c.Args, Err: err, Stderr: strings.TrimSpace(stderr.String())}
	}
	return stdout.String(), nil
}

// contention is what git says when one of its own locks is taken.
var contention = []string{
	".lock': file exists",
	"another git process seems to be running",
	"cannot lock ref",
	"could not lock config file",
	"unable to lock",
}

// Contended says whether a git command failed because one of git's own
// locks was taken: running it again later may work.
func Contended(err error) bool {
	var e *Error
	if !errors.As(err, &e) {
		return false
	}
	said := strings.ToLower(e.Stderr)
	for _, c := range contention {
		if strings.Contains(said, c) {
			return true
		}
	}
	return false
}

// Run runs git args in dir with the repository's lock, as Command.Run.
func Run(ctx context.Context, dir string, timeout time.Duration, args ...string) error {
	_, err := Command{Dir: dir, Args: args, Timeout: timeout}.Run(ctx)
	return err
}

// Output runs git args in dir with the repository's lock and returns what it
// printed on its standard output, as Command.Run.
func Output(ctx context.Context, dir string, timeout time.Duration, args ...string) (string, error) {
	return Command{Dir: dir, Args: args, Timeout: timeout}.Run(ctx)
}
