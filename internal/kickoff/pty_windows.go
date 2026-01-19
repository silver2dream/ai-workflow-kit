//go:build windows

package kickoff

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/UserExistsError/conpty"
)

// conptyWrapper wraps conpty.ConPty to implement io.ReadWriteCloser
type conptyWrapper struct {
	cpty *conpty.ConPty
}

func (w *conptyWrapper) Read(p []byte) (int, error) {
	return w.cpty.Read(p)
}

func (w *conptyWrapper) Write(p []byte) (int, error) {
	return w.cpty.Write(p)
}

func (w *conptyWrapper) Close() error {
	return w.cpty.Close()
}

// isConPtyAvailable checks if ConPTY is available on this Windows version
// ConPTY requires Windows 10 version 1809 (build 17763) or later
func isConPtyAvailable() bool {
	return conpty.IsConPtyAvailable()
}

// startPlatform starts the command on Windows
// Uses ConPTY if available (Windows 10 1809+), otherwise falls back to standard execution
func (p *PTYExecutor) startPlatform() error {
	// Check if ConPTY is available
	if !isConPtyAvailable() {
		return p.startStandard()
	}

	// Build command line string for ConPTY
	// ConPTY expects a single command line string, not separate args
	cmdLine := buildCommandLine(p.cmd.Path, p.cmd.Args[1:])

	// Prepare ConPTY options
	var opts []conpty.ConPtyOption

	// Set working directory if specified
	if p.cmd.Dir != "" {
		opts = append(opts, conpty.ConPtyWorkDir(p.cmd.Dir))
	}

	// Set environment if specified
	if len(p.cmd.Env) > 0 {
		opts = append(opts, conpty.ConPtyEnv(p.cmd.Env))
	}

	// Set default terminal size (80x25 is standard)
	opts = append(opts, conpty.ConPtyDimensions(80, 25))

	// Start the ConPTY process
	cpty, err := conpty.Start(cmdLine, opts...)
	if err != nil {
		// If ConPTY fails to start, fall back to standard execution
		return p.startStandard()
	}

	// Wrap ConPTY to implement io.ReadWriteCloser
	wrapper := &conptyWrapper{cpty: cpty}
	p.pty = wrapper
	p.output = wrapper

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

// buildCommandLine constructs a properly escaped command line string for Windows
func buildCommandLine(path string, args []string) string {
	// Start with the executable path
	var parts []string
	parts = append(parts, quoteArg(path))

	// Add each argument, properly quoted
	for _, arg := range args {
		parts = append(parts, quoteArg(arg))
	}

	return strings.Join(parts, " ")
}

// quoteArg quotes an argument if it contains spaces or special characters
func quoteArg(arg string) string {
	// If the argument contains spaces, tabs, or quotes, it needs to be quoted
	if strings.ContainsAny(arg, " \t\"") {
		// Escape any existing quotes and wrap in quotes
		escaped := strings.ReplaceAll(arg, `"`, `\"`)
		return `"` + escaped + `"`
	}
	return arg
}

// waitPlatform waits for the command to complete on Windows
func (p *PTYExecutor) waitPlatform() error {
	// If using ConPTY, wait on the conpty instance
	if wrapper, ok := p.pty.(*conptyWrapper); ok && wrapper != nil {
		exitCode, err := wrapper.cpty.Wait(context.Background())
		if err != nil {
			return err
		}
		if exitCode != 0 {
			return fmt.Errorf("process exited with code %d", exitCode)
		}
		return nil
	}

	// Fallback mode: wait on the exec.Cmd
	return p.cmd.Wait()
}

// killPlatform terminates the command on Windows
func (p *PTYExecutor) killPlatform() error {
	// If using ConPTY, close the conpty (which terminates the process)
	if wrapper, ok := p.pty.(*conptyWrapper); ok && wrapper != nil {
		return wrapper.cpty.Close()
	}

	// Fallback mode: kill the exec.Cmd process
	if p.cmd.Process != nil {
		return p.cmd.Process.Kill()
	}
	return nil
}
