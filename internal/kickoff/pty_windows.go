//go:build windows

package kickoff

import (
	"io"
	"os"
)

// startPlatform starts the command on Windows
// TODO: Implement ConPTY support for Windows 11
// For now, falls back to standard execution
func (p *PTYExecutor) startPlatform() error {
	return p.startStandard()
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
