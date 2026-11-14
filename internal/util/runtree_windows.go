//go:build windows

package util

import (
	"os/exec"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"
)

// RunWithChildren runs cmd to its end. When cmd's context ends first, the
// command is stopped together with whatever it started.
//
// A build or a test starts other processes (the test binaries, a test host).
// Killing the shell alone leaves them running and holding the command's
// output open, so the wait went on past the time limit. Here the command runs
// in a job object, and the job is terminated: what a process starts belongs
// to its job, also where the parent has gone by then, which is why following
// parent process ids (taskkill /T) does not find the children of a Git Bash
// shell.
//
// Without a job (it cannot be created, or the command cannot join it) the
// command alone is stopped, as it was before.
func RunWithChildren(cmd *exec.Cmd) error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return cmd.Run()
	}
	defer windows.CloseHandle(job)

	// What is still in the job when its last handle is closed ends with it.
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, _ = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)))

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
