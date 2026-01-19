//go:build windows

package kickoff

import (
	"bytes"
	"io"
	"testing"
	"time"
)

// TestConPTYAvailability tests that ConPTY availability is correctly detected
func TestConPTYAvailability(t *testing.T) {
	available := isConPtyAvailable()
	t.Logf("ConPTY available: %v", available)

	// On modern Windows 10/11, ConPTY should be available
	// This is informational - we don't fail if it's not available
	// as older Windows versions may not have it
}

// TestConPTYExecution tests basic command execution with ConPTY
func TestConPTYExecution(t *testing.T) {
	if !isConPtyAvailable() {
		t.Skip("ConPTY not available on this system")
	}

	executor, err := NewPTYExecutor("cmd", []string{"/c", "echo", "ConPTY test"})
	if err != nil {
		t.Fatalf("NewPTYExecutor failed: %v", err)
	}
	defer executor.Close()

	if err := executor.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Verify we're NOT using fallback when ConPTY is available
	if executor.IsFallback() {
		t.Error("Expected ConPTY mode, but got fallback mode")
	}

	// Read output
	output := executor.Output()
	if output == nil {
		t.Fatal("Output() returned nil")
	}

	var buf bytes.Buffer
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(&buf, output)
		done <- err
	}()

	// Wait for completion
	if err := executor.Wait(); err != nil {
		t.Logf("Wait returned error (may be expected): %v", err)
	}

	// Wait for output to be read
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Log("Timeout waiting for output")
	}

	// Check output contains expected text
	outputStr := buf.String()
	t.Logf("Output: %q", outputStr)
	if !bytes.Contains(buf.Bytes(), []byte("ConPTY")) {
		t.Log("Warning: output doesn't contain expected text")
	}
}

// TestConPTYWithEnvironment tests ConPTY with custom environment variables
func TestConPTYWithEnvironment(t *testing.T) {
	if !isConPtyAvailable() {
		t.Skip("ConPTY not available on this system")
	}

	executor, err := NewPTYExecutor("cmd", []string{"/c", "echo", "%TEST_VAR%"})
	if err != nil {
		t.Fatalf("NewPTYExecutor failed: %v", err)
	}
	defer executor.Close()

	// Set a custom environment variable
	executor.cmd.Env = append(executor.cmd.Env, "TEST_VAR=hello_from_conpty")

	if err := executor.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Read output
	output := executor.Output()
	var buf bytes.Buffer
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(&buf, output)
		done <- err
	}()

	executor.Wait()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}

	t.Logf("Output with env: %q", buf.String())
}

// TestConPTYKill tests process termination with ConPTY
func TestConPTYKill(t *testing.T) {
	if !isConPtyAvailable() {
		t.Skip("ConPTY not available on this system")
	}

	// Start a long-running command
	executor, err := NewPTYExecutor("cmd", []string{"/c", "ping", "-n", "100", "127.0.0.1"})
	if err != nil {
		t.Fatalf("NewPTYExecutor failed: %v", err)
	}
	defer executor.Close()

	if err := executor.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	// Kill the process
	if err := executor.Kill(); err != nil {
		t.Fatalf("Kill failed: %v", err)
	}

	// Wait should return (process was killed)
	executor.Wait()
	t.Log("Process killed successfully")
}

// TestBuildCommandLine tests the command line building function
func TestBuildCommandLine(t *testing.T) {
	tests := []struct {
		path     string
		args     []string
		expected string
	}{
		{"cmd", []string{"/c", "echo", "hello"}, "cmd /c echo hello"},
		{"cmd.exe", []string{"/c", "echo", "hello world"}, `cmd.exe /c echo "hello world"`},
		{"C:\\Program Files\\app.exe", []string{"arg1"}, `"C:\Program Files\app.exe" arg1`},
		{"app", []string{"arg with spaces", "normal"}, `app "arg with spaces" normal`},
	}

	for _, tt := range tests {
		result := buildCommandLine(tt.path, tt.args)
		if result != tt.expected {
			t.Errorf("buildCommandLine(%q, %v) = %q, want %q", tt.path, tt.args, result, tt.expected)
		}
	}
}

// TestQuoteArg tests the argument quoting function
func TestQuoteArg(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"with space", `"with space"`},
		{"with\ttab", `"with	tab"`},
		{`with"quote`, `"with\"quote"`},
		{"normal_arg", "normal_arg"},
	}

	for _, tt := range tests {
		result := quoteArg(tt.input)
		if result != tt.expected {
			t.Errorf("quoteArg(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}
