//go:build !windows

package workbench

import (
	"errors"
	"syscall"
	"time"
)

// killTree ends a process and every process it started: a run, its
// Principal, the Worker and the review it runs. A run the workbench started
// leads a process group of its own, which is ended whole: asked first, then
// made to. A run started elsewhere is in the group of what started it, which
// is left: the run alone is ended.
func killTree(pid int) error {
	target := pid
	if group, err := syscall.Getpgid(pid); err == nil && group == pid && group != syscall.Getpgrp() {
		target = -group
	}
	if err := syscall.Kill(target, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	for i := 0; i < 50; i++ {
		if syscall.Kill(pid, 0) != nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := syscall.Kill(target, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}
