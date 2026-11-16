package util

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func needsShell(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not on the PATH")
	}
}

// A command that is out of time is stopped together with what it started,
// however deep. The sleep here is the child of a shell that is the child of
// the command, and holds the command's output open for as long as it lives:
// with the command alone killed, the wait went on until the sleep was done.
//
// The limit is seconds, not a fraction of one: the shell has to start and
// print its first line within it, and on a busy machine that takes a while.
func TestRunWithChildren_StopsWhatTheCommandStarted(t *testing.T) {
	needsShell(t)
	const limit, asleep = 4 * time.Second, 60 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()

	var output bytes.Buffer
	cmd := exec.CommandContext(ctx, "sh", "-c", `echo started; sh -c "sleep 60; echo deep"; echo finished`)
	cmd.Stdout, cmd.Stderr = &output, &output
	cmd.WaitDelay = 5 * time.Second

	started := time.Now()
	err := RunWithChildren(cmd)
	took := time.Since(started)

	if err == nil || ctx.Err() != context.DeadlineExceeded {
		t.Errorf("a command out of time: err = %v, context %v; want an error at the deadline", err, ctx.Err())
	}
	if took > asleep/2 {
		t.Errorf("the command was waited for %v after a limit of %v: what it started was not stopped with it", took, limit)
	}
	if got := output.String(); !strings.Contains(got, "started") || strings.Contains(got, "deep") || strings.Contains(got, "finished") {
		t.Errorf("output = %q, want what was printed before the limit and nothing after", got)
	}
}

// A command that ends by itself is run as any other: its output, its failure
// when it fails, and what it left running keeps running. A setup command may
// start something the build and the tests after it need.
func TestRunWithChildren_ACommandThatEndsByItself(t *testing.T) {
	needsShell(t)
	run := func(dir, script string) (string, error) {
		var output bytes.Buffer
		cmd := exec.CommandContext(context.Background(), "sh", "-c", script)
		cmd.Dir = dir
		cmd.Stdout, cmd.Stderr = &output, &output
		err := RunWithChildren(cmd)
		return output.String(), err
	}
	dir := t.TempDir()

	if out, err := run(dir, "echo one; echo two >&2"); err != nil || !strings.Contains(out, "one") || !strings.Contains(out, "two") {
		t.Errorf("a command that passes: %q, %v", out, err)
	}

	out, err := run(dir, "echo before; exit 7")
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 7 || !strings.Contains(out, "before") {
		t.Errorf("a command that fails: %q, %v; want its output and exit code 7", out, err)
	}

	// What the command left in the background writes its mark a few seconds
	// after it was started. The command has ended before that: the mark is
	// not there yet when it returns, and appears afterwards.
	mark := filepath.Join(dir, "mark")
	if out, err := run(dir, "(sleep 4; echo alive > mark) >/dev/null 2>&1 & echo left"); err != nil || !strings.Contains(out, "left") {
		t.Fatalf("a command that leaves something running: %q, %v", out, err)
	}
	if _, err := os.Stat(mark); err == nil {
		t.Fatal("the command was waited for until what it left running had finished")
	}
	for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		if _, err := os.Stat(mark); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("what the command left running was ended with it")
		}
	}

	// A command that cannot be started says so.
	if err := RunWithChildren(exec.Command("no-such-command-anywhere")); err == nil {
		t.Error("a command that does not exist was run")
	}
}
