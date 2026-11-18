package workbench

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// spawnAwkit starts awkit with args in a project's folder, on its own: it is
// no child of the workbench that a closed terminal or a stopped workbench
// takes down with it. Work started from the page goes on when the page and
// the workbench are closed, and ends when it is done or someone stops it.
// What it prints goes to log.
func spawnAwkit(root string, args []string, log string) (int, error) {
	self, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("awkit cannot find itself: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(log), 0o755); err != nil {
		return 0, err
	}
	out, err := os.OpenFile(log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, err
	}
	defer out.Close()
	cmd := exec.Command(self, args...)
	cmd.Dir = root
	cmd.Stdout, cmd.Stderr = out, out
	cmd.SysProcAttr = detached()
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	// Waited for while the workbench runs, so that it leaves no zombie; when
	// the workbench goes first, the system takes it over.
	go func() { _ = cmd.Wait() }()
	return pid, nil
}
