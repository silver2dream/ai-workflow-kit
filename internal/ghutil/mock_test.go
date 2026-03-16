package ghutil

import (
	"context"
	"errors"
	"testing"
)

// MockGitHubCLI is a test double for GitHubCLI.
// Set Output/Err on the mock before the call under test; use Calls to inspect
// what was passed after the call.
type MockGitHubCLI struct {
	// Output is returned from RunWithRetry. May be set per-call via Outputs slice.
	Output []byte
	// Err is returned from RunWithRetry. May be set per-call via Errors slice.
	Err error
	// Outputs allows specifying different outputs per sequential call.
	Outputs [][]byte
	// Errors allows specifying different errors per sequential call.
	Errors []error
	// Calls records every RunWithRetry invocation for inspection.
	Calls []MockCall
}

// MockCall records the arguments passed to a single RunWithRetry invocation.
type MockCall struct {
	Name string
	Args []string
}

// RunWithRetry implements GitHubCLI and records the call.
func (m *MockGitHubCLI) RunWithRetry(_ context.Context, _ RetryConfig, name string, args ...string) ([]byte, error) {
	idx := len(m.Calls)
	m.Calls = append(m.Calls, MockCall{Name: name, Args: args})

	// Per-call outputs/errors take precedence over the single Output/Err.
	var out []byte
	var err error

	if idx < len(m.Outputs) {
		out = m.Outputs[idx]
	} else {
		out = m.Output
	}

	if idx < len(m.Errors) {
		err = m.Errors[idx]
	} else {
		err = m.Err
	}

	return out, err
}

// --- tests for MockGitHubCLI ---

func TestMockGitHubCLI_RecordsCalls(t *testing.T) {
	m := &MockGitHubCLI{Output: []byte("ok")}

	out, err := m.RunWithRetry(context.Background(), DefaultRetryConfig(), "gh", "issue", "list")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != "ok" {
		t.Errorf("output = %q, want %q", out, "ok")
	}
	if len(m.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(m.Calls))
	}
	if m.Calls[0].Name != "gh" {
		t.Errorf("call name = %q, want %q", m.Calls[0].Name, "gh")
	}
	if len(m.Calls[0].Args) != 2 || m.Calls[0].Args[0] != "issue" || m.Calls[0].Args[1] != "list" {
		t.Errorf("call args = %v, want [issue list]", m.Calls[0].Args)
	}
}

func TestMockGitHubCLI_ReturnsError(t *testing.T) {
	want := errors.New("auth failed")
	m := &MockGitHubCLI{Err: want}

	_, err := m.RunWithRetry(context.Background(), DefaultRetryConfig(), "gh", "pr", "view", "1")
	if err != want {
		t.Errorf("error = %v, want %v", err, want)
	}
}

func TestMockGitHubCLI_PerCallOutputs(t *testing.T) {
	m := &MockGitHubCLI{
		Outputs: [][]byte{[]byte("first"), []byte("second")},
		Errors:  []error{nil, errors.New("second call failed")},
	}

	out1, err1 := m.RunWithRetry(context.Background(), DefaultRetryConfig(), "gh", "a")
	out2, err2 := m.RunWithRetry(context.Background(), DefaultRetryConfig(), "gh", "b")

	if string(out1) != "first" || err1 != nil {
		t.Errorf("call 1: out=%q err=%v, want first/nil", out1, err1)
	}
	if string(out2) != "second" || err2 == nil {
		t.Errorf("call 2: out=%q err=%v, want second/<error>", out2, err2)
	}
	if len(m.Calls) != 2 {
		t.Errorf("expected 2 calls, got %d", len(m.Calls))
	}
}

func TestMockGitHubCLI_ImplementsInterface(t *testing.T) {
	// Compile-time check: *MockGitHubCLI satisfies GitHubCLI.
	var _ GitHubCLI = (*MockGitHubCLI)(nil)
}

func TestRealGitHubCLI_ImplementsInterface(t *testing.T) {
	// Compile-time check: the real implementation satisfies GitHubCLI.
	var _ GitHubCLI = NewGitHubCLI()
}
