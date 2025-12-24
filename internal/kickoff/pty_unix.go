//go:build !windows

package kickoff

import (
	"io"
	"os"

	"github.com/creack/pty"
)

// startPlatform starts the command with PTY on Unix systems
func (p *PTYExecutor) startPlatform() error {
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

// startStandard starts the command without PTY (fallback mode)
func (p *PTYExecutor) startStandard() error {
	p.fallback = true

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
