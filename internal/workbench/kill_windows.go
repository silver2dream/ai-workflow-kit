//go:build windows

package workbench

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// killTree ends a process and every process it started: a run, its
// Principal, the Worker and the review it runs. taskkill does what Windows
// has for it.
func killTree(pid int) error {
	cmd := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid))
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow, HideWindow: true}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("taskkill: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
