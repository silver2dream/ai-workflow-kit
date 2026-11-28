package gitlock

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// A helper process: with GITLOCK_HOLD set to a directory it takes that
// repository's lock, says so on its standard output, and holds it until its
// standard input closes (or it is killed).
func TestMain(m *testing.M) {
	if dir := os.Getenv("GITLOCK_HOLD"); dir != "" {
		err := Hold(context.Background(), dir, func() error {
			fmt.Println("held")
			_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
			return nil
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// repo is a repository with one commit, and its .git directory.
func repo(t *testing.T) (string, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", "a.txt")
	git(t, dir, "commit", "-q", "-m", "a")
	return dir, filepath.Join(dir, ".git")
}

func same(t *testing.T, got, want string) {
	t.Helper()
	g, err1 := filepath.EvalSymlinks(filepath.Dir(got))
	w, err2 := filepath.EvalSymlinks(filepath.Dir(want))
	if err1 != nil || err2 != nil || !strings.EqualFold(filepath.Join(g, filepath.Base(got)), filepath.Join(w, filepath.Base(want))) {
		t.Errorf("lock = %q, want %q", got, want)
	}
}

func TestPath(t *testing.T) {
	dir, dotgit := repo(t)
	want := filepath.Join(dotgit, FileName)

	t.Run("the working tree and a folder in it", func(t *testing.T) {
		same(t, Path(dir), want)
		sub := filepath.Join(dir, "x", "y")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		same(t, Path(sub), want)
	})
	t.Run("a worktree shares its main working tree's", func(t *testing.T) {
		wt := filepath.Join(dir, ".worktrees", "issue-5")
		git(t, dir, "worktree", "add", "-q", "-b", "feat/5", wt)
		same(t, Path(wt), want)
		same(t, Path(filepath.Join(wt, "deep")), want)
	})
	t.Run("a git directory named by a .git file without commondir is its own", func(t *testing.T) {
		sub := filepath.Join(dir, "sub")
		modules := filepath.Join(dotgit, "modules", "sub")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(modules, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sub, ".git"), []byte("gitdir: ../.git/modules/sub\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		same(t, Path(sub), filepath.Join(modules, FileName))
	})
	t.Run("a .git file that names nothing", func(t *testing.T) {
		odd := t.TempDir()
		if err := os.WriteFile(filepath.Join(odd, ".git"), []byte("not a pointer\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := Path(odd); got != "" {
			t.Errorf("lock = %q, want none", got)
		}
	})
}

func TestHold_OneAtATimeInAProcess(t *testing.T) {
	dir, _ := repo(t)
	var in, most atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			err := Hold(context.Background(), dir, func() error {
				n := in.Add(1)
				for {
					m := most.Load()
					if n <= m || most.CompareAndSwap(m, n) {
						break
					}
				}
				time.Sleep(5 * time.Millisecond)
				in.Add(-1)
				return nil
			})
			if err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if most.Load() != 1 {
		t.Errorf("%d held the lock at once", most.Load())
	}
}

func TestHold_FnsErrorIsReturned(t *testing.T) {
	dir, _ := repo(t)
	want := errors.New("no")
	if err := Hold(context.Background(), dir, func() error { return want }); !errors.Is(err, want) {
		t.Errorf("err = %v", err)
	}
	// Outside a repository fn runs as it is.
	ran := false
	if err := Hold(context.Background(), t.TempDir(), func() error { ran = true; return nil }); err != nil || !ran {
		t.Errorf("outside a repository: ran %v, err %v", ran, err)
	}
}

// holder starts a process that holds dir's lock, and returns once it does.
func holder(t *testing.T, dir string) (*exec.Cmd, func()) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "GITLOCK_HOLD="+dir)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "held" {
		_ = cmd.Process.Kill()
		t.Fatalf("the holder said %q, %v", line, err)
	}
	return cmd, func() { stdin.Close() }
}

func TestHold_AnotherProcessHoldsIt(t *testing.T) {
	dir, _ := repo(t)
	cmd, let := holder(t, dir)

	// While it holds the lock, a wait that ends first gets nothing.
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	err := Hold(ctx, dir, func() error { t.Error("held it while another process did"); return nil })
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want the wait's end", err)
	}

	// Once it lets go, the lock is taken.
	got := make(chan error, 1)
	go func() { got <- Hold(context.Background(), dir, func() error { return nil }) }()
	select {
	case err := <-got:
		t.Fatalf("held it before the other process let go: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	let()
	select {
	case err := <-got:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the lock was not taken after the other process let it go")
	}
	_ = cmd.Wait()
}

func TestHold_AProcessThatDiesLetsGo(t *testing.T) {
	dir, _ := repo(t)
	cmd, _ := holder(t, dir)
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := Hold(ctx, dir, func() error { return nil }); err != nil {
		t.Fatalf("the lock of a process that died: %v", err)
	}
}

func TestContended(t *testing.T) {
	for said, want := range map[string]bool{
		"fatal: Unable to create '/r/.git/index.lock': File exists.\n\nAnother git process seems to be running in this repository": true,
		"error: cannot lock ref 'refs/remotes/origin/main': is at 1 but expected 2":                                                true,
		"error: could not lock config file .git/config: File exists":                                                               true,
		"fatal: Unable to create '/r/.git/shallow.lock': File exists.":                                                             true,
		"error: unable to lock packed-refs":                                                                                        true,
		"fatal: couldn't find remote ref nope":                                                                                     false,
		"CONFLICT (content): Merge conflict in a.txt":                                                                              false,
		"": false,
	} {
		if got := Contended(&Error{Args: []string{"x"}, Err: errors.New("exit status 1"), Stderr: said}); got != want {
			t.Errorf("Contended(%q) = %v", said, got)
		}
	}
	if Contended(errors.New("index.lock': File exists")) {
		t.Error("an error that is not a git command's is contended")
	}
}

func TestCommand_Run(t *testing.T) {
	dir, dotgit := repo(t)

	t.Run("its standard output", func(t *testing.T) {
		out, err := Output(context.Background(), dir, 0, "rev-parse", "--abbrev-ref", "HEAD")
		if err != nil || strings.TrimSpace(out) != "main" {
			t.Errorf("out %q, err %v", out, err)
		}
	})
	t.Run("a failure says what git said, and is the process's", func(t *testing.T) {
		err := Run(context.Background(), dir, 0, "rev-parse", "--verify", "--quiet", "refs/heads/nope")
		var exit *exec.ExitError
		if err == nil || !errors.As(err, &exit) {
			t.Fatalf("err = %v, want an exit error", err)
		}
		_, err = Output(context.Background(), dir, 0, "checkout", "nope")
		if err == nil || !strings.Contains(err.Error(), "did not match") {
			t.Errorf("err = %v, want what git said", err)
		}
	})
	t.Run("the environment is added", func(t *testing.T) {
		out, err := Command{Dir: dir, Args: []string{"var", "GIT_AUTHOR_IDENT"}, Env: []string{"GIT_AUTHOR_NAME=Someone", "GIT_AUTHOR_EMAIL=s@example.com"}}.Run(context.Background())
		if err != nil || !strings.HasPrefix(out, "Someone <s@example.com>") {
			t.Errorf("out %q, err %v", out, err)
		}
	})
	t.Run("a timeout ends a try", func(t *testing.T) {
		start := time.Now()
		// git hash-object --stdin waits for its input, which never comes.
		_, err := Command{Dir: dir, Args: []string{"hash-object", "--stdin"}, Timeout: 200 * time.Millisecond}.Run(context.Background())
		if time.Since(start) > 5*time.Second {
			t.Errorf("the timeout did not end it: %v, %v", time.Since(start), err)
		}
	})
	t.Run("a lock of git's own, let go, is tried again", func(t *testing.T) {
		was := backoff
		backoff = 50 * time.Millisecond
		defer func() { backoff = was }()
		lock := filepath.Join(dotgit, "index.lock")
		if err := os.WriteFile(lock, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		go func() {
			time.Sleep(120 * time.Millisecond)
			_ = os.Remove(lock)
		}()
		if err := Run(context.Background(), dir, 0, "add", "b.txt"); err != nil {
			t.Fatalf("not tried again: %v", err)
		}
		if got := git(t, dir, "diff", "--cached", "--name-only"); got != "b.txt" {
			t.Errorf("staged %q", got)
		}
	})
	t.Run("one that is never let go fails as often as it may be tried", func(t *testing.T) {
		was, wasTries := backoff, Tries
		backoff, Tries = 10*time.Millisecond, 3
		defer func() { backoff, Tries = was, wasTries }()
		lock := filepath.Join(dotgit, "index.lock")
		if err := os.WriteFile(lock, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(lock)
		if err := os.WriteFile(filepath.Join(dir, "c.txt"), []byte("c\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		err := Run(context.Background(), dir, 0, "add", "c.txt")
		if !Contended(err) {
			t.Errorf("err = %v, want the lock's", err)
		}
	})
}
