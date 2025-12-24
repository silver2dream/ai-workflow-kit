package kickoff

import (
	"io"
	"os/exec"
)

// PTYExecutor handles pseudo-terminal execution of commands
type PTYExecutor struct {
	cmd      *exec.Cmd
	pty      io.ReadWriteCloser // Platform-specific PTY
	output   io.Reader
	fallback bool // true if using standard execution
}

// NewPTYExecutor creates a new PTY executor for the given command
func NewPTYExecutor(command string, args []string) (*PTYExecutor, error) {
	cmd := exec.Command(command, args...)
	return &PTYExecutor{
		cmd: cmd,
	}, nil
}

// Start begins execution of the command
// Platform-specific implementation in pty_unix.go and pty_windows.go
func (p *PTYExecutor) Start() error {
	return p.startPlatform()
}

// Wait waits for the command to complete
func (p *PTYExecutor) Wait() error {
	return p.cmd.Wait()
}

// Output returns a reader for the command output
func (p *PTYExecutor) Output() io.Reader {
	return p.output
}

// Kill terminates the command
func (p *PTYExecutor) Kill() error {
	if p.cmd.Process != nil {
		return p.cmd.Process.Kill()
	}
	return nil
}

// IsFallback returns true if using standard execution instead of PTY
func (p *PTYExecutor) IsFallback() bool {
	return p.fallback
}

// Close closes the PTY
func (p *PTYExecutor) Close() error {
	if p.pty != nil {
		return p.pty.Close()
	}
	return nil
}
