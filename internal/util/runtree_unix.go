//go:build !windows

package util

import (
	"os/exec"
	"syscall"
)

// RunWithChildren runs cmd to its end. When cmd's context ends first, the
// command is stopped together with whatever it started.
//
// A build or a test starts other processes (the test binaries, a test host).
// Killing the shell alone leaves them running and holding the command's
// output open, so the wait went on past the time limit. Here the command gets
// a process group of its own, and the group is killed.
func RunWithChildren(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	return cmd.Run()
}
