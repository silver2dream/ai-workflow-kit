//go:build !windows

package util

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// RunWithChildren runs cmd to its end. When cmd's context ends first, the
// command is stopped together with whatever it started.
//
// A build or a test starts other processes (the test binaries, a test host).
// Killing the shell alone leaves them running and holding the command's
// output open, so the wait went on past the time limit.
//
// The processes under the command are looked up and killed one by one. The
// command stays in the process group it was started in: a signal that reaches
// that group from outside (an interrupt at the terminal, the tool that
// started awkit ending it) reaches the command and its children as it always
// did. A group of its own would have cut them off from that.
//
// Nothing changes for a command that ends by itself: what it left running
// keeps running.
func RunWithChildren(cmd *exec.Cmd) error {
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		killTree(cmd.Process.Pid)
		return nil
	}
	return cmd.Run()
}

// killTree ends a process and every process under it. Each one is stopped
// before its children are looked up, so that it cannot start another in the
// meantime; once the whole tree stands still, all of it is killed.
func killTree(pid int) {
	var tree []int
	var freeze func(pid int)
	freeze = func(pid int) {
		if syscall.Kill(pid, syscall.SIGSTOP) != nil {
			return // gone already
		}
		tree = append(tree, pid)
		for _, child := range childrenOf(pid) {
			freeze(child)
		}
	}
	freeze(pid)
	for _, member := range tree {
		_ = syscall.Kill(member, syscall.SIGKILL)
	}
}

// childrenOf lists the running processes a process started. Where pgrep is
// missing there is no telling, and the process is ended alone, as it was
// before.
func childrenOf(pid int) []int {
	// pgrep ends with 1 when the process has no children; what it printed
	// is the answer either way.
	out, _ := exec.Command("pgrep", "-P", strconv.Itoa(pid)).Output()
	var children []int
	for _, field := range strings.Fields(string(out)) {
		if child, err := strconv.Atoi(field); err == nil {
			children = append(children, child)
		}
	}
	return children
}
