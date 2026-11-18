//go:build windows

package workbench

import "syscall"

// On Windows the process gets a console of its own, hidden, in a group of its
// own: Ctrl+C at the workbench's terminal, or closing it, does not reach it.
// The tools it runs (git, gh, claude) have that console too.
const createNoWindow = 0x08000000

func detached() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | createNoWindow, HideWindow: true}
}
