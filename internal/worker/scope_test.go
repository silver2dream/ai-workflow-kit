package worker

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/lanenote"
)

// stagedRepo is a repository with a commit, and a change staged on it: files
// written, and files deleted.
func stagedRepo(t *testing.T, write map[string]string, remove ...string) string {
	t.Helper()
	dir := commitsRepo(t, map[string]string{"rally/old.go": "package rally\n", "docs/a.md": "a\n"})
	for name, text := range write {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range remove {
		if err := os.Remove(filepath.Join(dir, filepath.FromSlash(name))); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	return dir
}

// A staged change is held to its ticket's paths: each file it touches,
// deleted ones too, without case.
func TestCheckScope(t *testing.T) {
	ctx := context.Background()
	paths := []string{"rally/**", "docs/A.md"}
	inside := stagedRepo(t, map[string]string{"rally/score.go": "package rally\n", "Rally/sub/x.go": "package sub\n", "docs/a.md": "b\n"})
	if err := CheckScope(ctx, inside, paths); err != nil {
		t.Errorf("a change within its paths: %v", err)
	}

	outside := stagedRepo(t, map[string]string{"rally/score.go": "package rally\n", "go.mod": "module x\n"}, "docs/a.md")
	err := CheckScope(ctx, outside, []string{"rally/**"})
	var scope *ScopeError
	if !errors.As(err, &scope) || strings.Join(scope.Outside, ",") != "docs/a.md,go.mod" {
		t.Fatalf("outside: %+v (%v)", scope, err)
	}
	if !strings.Contains(err.Error(), "docs/a.md, go.mod") || !strings.Contains(err.Error(), "the ticket's paths: rally/**") {
		t.Errorf("said: %v", err)
	}

	// A directory that is no repository: git cannot show the change, and
	// that is no refusal.
	if err := CheckScope(ctx, t.TempDir(), paths); err == nil || errors.As(err, &scope) {
		t.Errorf("no repository: %v", err)
	}
}

// The prompt names the ticket's paths, the owner's notes as the owner's
// word, and why the last attempt was refused for its scope, fenced as data.
func TestBuildWorkerPrompt_ScopeAndTheOwnersNotes(t *testing.T) {
	prompt := buildWorkerPrompt(workerPromptInputs{
		Ticket: "# Task", IssueID: 7,
		Paths:        []string{"rally/**", "docs/rules.md"},
		OwnerNotes:   "Keep the old name as an alias.",
		ScopeFailure: "the change touches files outside the ticket's paths: go.mod",
	})
	for _, want := range []string{
		"WHAT YOU MAY CHANGE", "- rally/**\n- docs/rules.md\n",
		"THE OWNER'S NOTES ON THIS TICKET", "Keep the old name as an alias.",
		"YOUR LAST ATTEMPT WAS REFUSED", "outside the ticket's paths: go.mod",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the prompt lacks %q:\n%s", want, prompt)
		}
	}
	if strings.Index(prompt, "THE OWNER'S NOTES") < strings.Index(prompt, "WHAT YOU MAY CHANGE") {
		t.Error("the notes come before the paths")
	}
	// The notes are the owner's: they are not fenced as data; the scope
	// check's words name files the change wrote, and are.
	pieces := promptPieces(workerPromptInputs{Ticket: "# Task", IssueID: 7, OwnerNotes: "x", ScopeFailure: "y"})
	trust := map[string]string{}
	for _, p := range pieces {
		trust[p.Source] = string(p.Trust)
	}
	if trust["the owner's notes on issue #7"] != "owner" || trust["the scope check of the last attempt"] != "agent" {
		t.Errorf("trust: %v", trust)
	}

	plain := buildWorkerPrompt(workerPromptInputs{Ticket: "# Task", IssueID: 7})
	for _, absent := range []string{"WHAT YOU MAY CHANGE", "THE OWNER'S NOTES", "YOUR LAST ATTEMPT WAS REFUSED"} {
		if strings.Contains(plain, absent) {
			t.Errorf("a prompt with none has %q", absent)
		}
	}
}

// What the prompt is given is gathered: the block's paths, the notes kept
// for the ticket, and the scope failure of the last attempt only.
func TestWritePromptFile_GathersScopeAndNotes(t *testing.T) {
	root := t.TempDir()
	ticket := "# Task\n\n```yaml awk-ticket\nschema: 2\nrepo: root\npaths: [rally/**]\nrisk: low\n```\n"
	if got := ticketPaths(ticket); strings.Join(got, ",") != "rally/**" {
		t.Errorf("paths: %v", got)
	}
	if got := ticketPaths("# Task without a block"); got != nil {
		t.Errorf("no block: %v", got)
	}
	if err := lanenote.Add(root, 7, lanenote.Note{Kind: lanenote.Returned, Text: "Keep the old name."}); err != nil {
		t.Fatal(err)
	}
	if err := WriteResultAtomic(root, 7, &IssueResult{IssueID: "7", Status: "failed", FailureStage: "scope", ErrorMessage: "outside: go.mod"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "prompt.txt")
	if _, err := writePromptFile(path, "", ticket, root, 7, 0, 0, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	for _, want := range []string{"- rally/**", "Keep the old name.", "outside: go.mod"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("the prompt lacks %q", want)
		}
	}
	// A failure at another stage is not taken for one of scope.
	if err := WriteResultAtomic(root, 7, &IssueResult{IssueID: "7", Status: "failed", FailureStage: "execution", ErrorMessage: "tests failed"}); err != nil {
		t.Fatal(err)
	}
	if _, err := writePromptFile(path, "", ticket, root, 7, 0, 0, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); strings.Contains(string(data), "YOUR LAST ATTEMPT WAS REFUSED") {
		t.Error("a failure of another stage is said as one of scope")
	}
}

// The dispatch's failure carries what the Worker said of it.
func TestWithWorkersWord(t *testing.T) {
	out := &DispatchOutput{Status: "failed", Error: "Worker 失敗，將重試 (1/3)"}
	withWorkersWord(out, &IssueResult{ErrorMessage: "the change touches files outside the ticket's paths: go.mod"})
	if out.Error != "Worker 失敗，將重試 (1/3): the change touches files outside the ticket's paths: go.mod" {
		t.Errorf("error: %q", out.Error)
	}
	for _, c := range []struct {
		out    *DispatchOutput
		result *IssueResult
	}{
		{&DispatchOutput{Status: "failed", Error: "x"}, &IssueResult{}},
		{&DispatchOutput{Status: "success", Error: "x"}, &IssueResult{ErrorMessage: "y"}},
		{&DispatchOutput{Status: "failed", Error: "x"}, nil},
	} {
		withWorkersWord(c.out, c.result)
		if c.out.Error != "x" {
			t.Errorf("%+v: %q", c.result, c.out.Error)
		}
	}
	withWorkersWord(nil, &IssueResult{ErrorMessage: "y"})
}

// A ticket without paths, or of a submodule, is not held to them.
func TestHoldToScope(t *testing.T) {
	ctx := context.Background()
	outside := stagedRepo(t, map[string]string{"go.mod": "module x\n"})
	var scope *ScopeError
	if err := holdToScope(ctx, outside, "directory", []string{"rally/**"}); !errors.As(err, &scope) {
		t.Errorf("a directory repo: %v", err)
	}
	if err := holdToScope(ctx, outside, "root", []string{"rally/**"}); !errors.As(err, &scope) {
		t.Errorf("the root repo: %v", err)
	}
	if err := holdToScope(ctx, outside, "submodule", []string{"rally/**"}); err != nil {
		t.Errorf("a submodule: %v", err)
	}
	if err := holdToScope(ctx, outside, "root", nil); err != nil {
		t.Errorf("no paths: %v", err)
	}
}
