//go:build windows

package util

import (
	"os/exec"
	"sync/atomic"

	"golang.org/x/sys/windows"
)

// RunWithChildren runs cmd to its end. When cmd's context ends first, the
// command is stopped together with whatever it started.
//
// A build or a test starts other processes (the test binaries, a test host).
// Killing the shell alone leaves them running and holding the command's
// output open, so the wait went on past the time limit.
//
// The command runs in a job object, and at the end of the context the job is
// terminated. What a process starts belongs to its job, also where the parent
// has gone by then, which is why following parent process ids (taskkill /T)
// does not find the children of a Git Bash shell.
//
// Nothing changes for a command that ends by itself: what it left running
// keeps running, in the job or not. And without a job (it cannot be created,
// or the command cannot join it) the command alone is stopped, as it was
// before.
//
// The command joins the job right after it is started. A process it starts
// before that is outside the job; a shell needs far longer to start one than
// the joining takes.
func RunWithChildren(cmd *exec.Cmd) error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return cmd.Run()
	}
	defer windows.CloseHandle(job)

	var joined atomic.Bool
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		if joined.Load() && windows.TerminateJobObject(job, 1) == nil {
			return nil
		}
		return cmd.Process.Kill()
	}

	if err := cmd.Start(); err != nil {
		return err
	}
	if process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid)); err == nil {
		joined.Store(windows.AssignProcessToJobObject(job, process) == nil)
		_ = windows.CloseHandle(process)
	}
	return cmd.Wait()
}
