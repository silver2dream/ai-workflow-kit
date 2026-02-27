//go:build !windows

package hooks

import (
	"os/exec"
	"syscall"
)

// setProcGroup sets process group attributes so the entire child tree
// can be killed when the context expires.
func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			// Kill the entire process group (negative PID)
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}
}
