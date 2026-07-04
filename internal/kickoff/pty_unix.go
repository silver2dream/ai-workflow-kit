//go:build !windows

package kickoff

import (
	"io"
	"os"

	"github.com/creack/pty"
)

// startPlatform starts the command with PTY on Unix systems
func (p *PTYExecutor) startPlatform() error {
	// Headless children opt out of the PTY entirely (no TTY -> no interactive
	// permission prompts, no ANSI injection).
	if p.standard {
		return p.startStandard()
	}

	// Try to start with PTY
	ptmx, err := pty.Start(p.cmd)
	if err != nil {
		// Fallback to standard execution
		return p.startStandard()
	}

	p.pty = ptmx
	p.output = ptmx
	return nil
}

// waitPlatform waits for the command to complete on Unix
func (p *PTYExecutor) waitPlatform() error {
	return p.cmd.Wait()
}

// killPlatform terminates the command on Unix
func (p *PTYExecutor) killPlatform() error {
	if p.cmd.Process != nil {
		return p.cmd.Process.Kill()
	}
	return nil
}

// startStandard starts the command without PTY (fallback mode)
func (p *PTYExecutor) startStandard() error {
	// Only a PTY that failed unexpectedly counts as a fallback; a deliberate
	// standard-mode child is not a degraded state and must not warn.
	if !p.standard {
		p.fallback = true
	}

	// Create pipes for stdout and stderr
	stdout, err := p.cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := p.cmd.StderrPipe()
	if err != nil {
		return err
	}

	// Combine stdout and stderr
	p.output = io.MultiReader(stdout, stderr)

	// Set stdin to os.Stdin for interactive commands
	p.cmd.Stdin = os.Stdin

	return p.cmd.Start()
}
