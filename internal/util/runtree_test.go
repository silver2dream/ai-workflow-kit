package util

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// A command that is out of time is stopped together with what it started.
// The sleep here is a child of the shell and holds the shell's output open
// for as long as it lives: with the shell alone killed, the wait went on
// until the sleep was done.
func TestRunWithChildren_StopsWhatTheCommandStarted(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not on the PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	var output bytes.Buffer
	cmd := exec.CommandContext(ctx, "sh", "-c", "echo started; sleep 5; echo finished")
	cmd.Stdout, cmd.Stderr = &output, &output
	cmd.WaitDelay = 5 * time.Second

	started := time.Now()
	err := RunWithChildren(cmd)
	took := time.Since(started)

	if err == nil || ctx.Err() != context.DeadlineExceeded {
		t.Errorf("a command out of time: err = %v, context %v; want an error at the deadline", err, ctx.Err())
	}
	if took > 3*time.Second {
		t.Errorf("the command was waited for %v after a limit of 300ms: what it started was not stopped with it", took)
	}
	if got := output.String(); !strings.Contains(got, "started") || strings.Contains(got, "finished") {
		t.Errorf("output = %q, want what was printed before the limit and nothing after", got)
	}
}

// A command that ends by itself is run as any other: its output, and its
// failure when it fails.
func TestRunWithChildren_ACommandThatEndsByItself(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not on the PATH")
	}
	run := func(script string) (string, error) {
		var output bytes.Buffer
		cmd := exec.CommandContext(context.Background(), "sh", "-c", script)
		cmd.Stdout, cmd.Stderr = &output, &output
		err := RunWithChildren(cmd)
		return output.String(), err
	}

	if out, err := run("echo one; echo two >&2"); err != nil || !strings.Contains(out, "one") || !strings.Contains(out, "two") {
		t.Errorf("a command that passes: %q, %v", out, err)
	}
	out, err := run("echo before; exit 7")
	var exit *exec.ExitError
	if !asExitError(err, &exit) || exit.ExitCode() != 7 || !strings.Contains(out, "before") {
		t.Errorf("a command that fails: %q, %v; want its output and exit code 7", out, err)
	}

	// A command that cannot be started says so.
	if err := RunWithChildren(exec.Command("no-such-command-anywhere")); err == nil {
		t.Error("a command that does not exist was run")
	}
}

func asExitError(err error, target **exec.ExitError) bool {
	exit, ok := err.(*exec.ExitError)
	if ok {
		*target = exit
	}
	return ok
}
