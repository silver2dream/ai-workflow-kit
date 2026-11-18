package workbench

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Work started from the page goes on when the page and the workbench are
// closed, and ends when it is done or someone stops it. So it is not the
// workbench's child: the workbench starts `awkit detach`, which starts the
// work in a process of its own and ends at once. The work then has no parent
// that is still there, and neither a closed terminal nor a tool that ends a
// process with everything it started (taskkill /T, an editor closing its
// terminal) reaches it.

// spawnAwkit starts awkit with args in a project's folder, on its own, and
// returns the pid of the process that does the work. What it prints goes to
// log.
func spawnAwkit(root string, args []string, log string) (int, error) {
	self, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("awkit cannot find itself: %w", err)
	}
	cmd := exec.Command(self, append([]string{"detach", "--log", log, "--"}, args...)...)
	cmd.Dir = root
	cmd.SysProcAttr = detached()
	out, err := cmd.Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok && len(exit.Stderr) > 0 {
			return 0, fmt.Errorf("%s", strings.TrimSpace(string(exit.Stderr)))
		}
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, fmt.Errorf("awkit detach said %q, not a pid", strings.TrimSpace(string(out)))
	}
	return pid, nil
}

// StartAlone starts awkit with args in dir, in a process of its own (a
// session of its own; on Windows a hidden console and a process group of its
// own), with what it prints going to log, and returns its pid without
// waiting for it. awkit detach is the one that calls it.
func StartAlone(dir string, args []string, log string) (int, error) {
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
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = out, out
	cmd.SysProcAttr = detached()
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	return pid, nil
}
