//go:build !windows

package workbench

import "syscall"

// Elsewhere the process starts a session of its own: the workbench's
// terminal, its hangup and its Ctrl+C do not reach it.
func detached() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
