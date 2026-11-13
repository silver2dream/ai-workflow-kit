package task

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
)

const validBody = `## Summary
Add a health endpoint.

## Scope
- backend/health

## Acceptance Criteria
- [ ] GET /healthz answers 200

## Testing Requirements
- [ ] a test covers the handler

` + validBlock

const validBlock = "```yaml awk-ticket\nschema: 1\nrepo: backend\ndepends_on: [12]\n```\n"

func writeBody(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "ticket.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeConfig(t *testing.T, dir, content string) {
	t.Helper()
	path := filepath.Join(dir, ".ai", "config", "workflow.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeGH replaces the gh CLI for one test and records what it was asked.
func fakeGH(t *testing.T, output string, err error) *[][]string {
	t.Helper()
	var calls [][]string
	orig := runGH
	runGH = func(ctx context.Context, args ...string) ([]byte, error) {
		calls = append(calls, args)
		return []byte(output), err
	}
	t.Cleanup(func() { runGH = orig })
	return &calls
}

func TestValidateBody(t *testing.T) {
	if err := ValidateBody(validBody); err != nil {
		t.Errorf("a complete ticket was refused: %v", err)
	}

	tests := []struct {
		name, body, wantInError string
	}{
		{"empty", "", "Summary, Scope, Acceptance Criteria, Testing Requirements"},
		{"one section missing", strings.Replace(validBody, "## Scope\n- backend/health\n\n", "", 1), "missing required sections: Scope"},
		{"several missing", "## Summary\ntext\n\n## Scope\n- x\n", "Acceptance Criteria, Testing Requirements"},
		{"no checkbox", strings.NewReplacer("- [ ] ", "- ").Replace(validBody), "missing acceptance checkboxes"},
		{"only ticked checkboxes", strings.NewReplacer("- [ ] ", "- [x] ").Replace(validBody), "missing acceptance checkboxes"},
		{"a heading of the wrong level is not the section", strings.Replace(validBody, "## Scope", "### Scope", 1), "missing required sections: Scope"},
		{"a longer heading is not the section", strings.Replace(validBody, "## Scope", "## Scope of work", 1), "missing required sections: Scope"},
		// The awk-ticket block.
		{"no block", strings.Replace(validBody, validBlock, "", 1), "has no `yaml awk-ticket` block"},
		{"what the block says, as prose", strings.Replace(validBody, validBlock, "## Metadata\n- **Repo**: backend\n- Depends on: #12\n", 1), "has no `yaml awk-ticket` block"},
		{"the block only as an example", strings.Replace(validBody, validBlock, "````markdown\n"+validBlock+"````\n", 1), "has no `yaml awk-ticket` block"},
		{"a field the block does not have", strings.Replace(validBody, "repo: backend\n", "repo: backend\npriority: high\n", 1), `unknown field "priority"`},
		{"a block without a repo", strings.Replace(validBody, "repo: backend\n", "", 1), "repo is missing"},
		{"a release outside the root repo", strings.Replace(validBody, "repo: backend\n", "repo: backend\nrelease: true\n", 1), "only a ticket for the root repo can be a release"},
		{"two blocks", validBody + "\n" + validBlock, "2 awk-ticket blocks"},
	}
	for _, tt := range tests {
		err := ValidateBody(tt.body)
		if err == nil {
			t.Errorf("%s: the body was accepted", tt.name)
			continue
		}
		if !strings.Contains(err.Error(), tt.wantInError) {
			t.Errorf("%s: error = %q, want it to mention %q", tt.name, err, tt.wantInError)
		}
	}

	// Heading case does not matter.
	if err := ValidateBody(strings.Replace(validBody, "## Summary", "## SUMMARY", 1)); err != nil {
		t.Errorf("a heading in another case was refused: %v", err)
	}
}

func TestCreateTask(t *testing.T) {
	dir := t.TempDir()
	body := writeBody(t, dir, validBody)
	calls := fakeGH(t, "https://github.com/acme/game/issues/42\n", nil)

	result, err := CreateTask(context.Background(), CreateTaskOptions{
		Title: "  [feat] add a health endpoint  ", BodyFile: body, StateRoot: dir,
	})

	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if result.IssueNumber != 42 || result.IssueURL != "https://github.com/acme/game/issues/42" {
		t.Errorf("result = %+v, want issue 42 with its URL", result)
	}
	want := []string{"issue", "create", "--title", "[feat] add a health endpoint", "--body-file", body, "--label", "ai-task"}
	if len(*calls) != 1 || !reflect.DeepEqual((*calls)[0], want) {
		t.Errorf("gh was called with %v, want once with %v", *calls, want)
	}
	// The ticket is created from the body as written.
	if got, _ := os.ReadFile(body); string(got) != validBody {
		t.Error("the body file was changed")
	}
}

// The label and the repository come from the project's config; the repository
// can be overridden for one ticket.
func TestCreateTask_LabelAndRepoFromConfig(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "github:\n  repo: acme/game\n  labels:\n    task: work-item\nrepos:\n  - name: backend\n    path: backend/\n")
	body := writeBody(t, dir, validBody)

	calls := fakeGH(t, "https://github.com/acme/game/issues/7\n", nil)
	if _, err := CreateTask(context.Background(), CreateTaskOptions{Title: "t", BodyFile: body, StateRoot: dir}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	want := []string{"issue", "create", "--title", "t", "--body-file", body, "--label", "work-item", "--repo", "acme/game"}
	if !reflect.DeepEqual((*calls)[0], want) {
		t.Errorf("gh was called with %v, want %v", (*calls)[0], want)
	}

	calls = fakeGH(t, "https://github.com/acme/other/issues/8\n", nil)
	if _, err := CreateTask(context.Background(), CreateTaskOptions{Title: "t", BodyFile: body, StateRoot: dir, Repo: "acme/other"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if got := (*calls)[0]; got[len(got)-1] != "acme/other" {
		t.Errorf("gh was called with %v, want the overriding repository", got)
	}
}

// A ticket is for a repo of the project's config, or for the root of the
// repository. One the Worker would refuse is not created.
func TestCreateTask_RepoOfTheConfig(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "repos:\n  - name: backend\n    path: backend/\n  - name: frontend\n    path: frontend/\n")

	for _, repo := range []string{"backend", "frontend", "root", "Backend"} {
		body := writeBody(t, dir, strings.Replace(validBody, "repo: backend\n", "repo: "+repo+"\n", 1))
		calls := fakeGH(t, "https://github.com/acme/game/issues/7\n", nil)
		if _, err := CreateTask(context.Background(), CreateTaskOptions{Title: "t", BodyFile: body, StateRoot: dir}); err != nil {
			t.Errorf("repo %q: %v", repo, err)
		}
		if len(*calls) != 1 {
			t.Errorf("repo %q: gh was called %d times, want once", repo, len(*calls))
		}
	}

	body := writeBody(t, dir, strings.Replace(validBody, "repo: backend\n", "repo: web\n", 1))
	calls := fakeGH(t, "https://github.com/acme/game/issues/8\n", nil)
	_, err := CreateTask(context.Background(), CreateTaskOptions{Title: "t", BodyFile: body, StateRoot: dir, DryRun: true})
	if err == nil || !strings.Contains(err.Error(), `repo "web", which is not a repo of .ai/config/workflow.yaml (it has: root, backend, frontend)`) {
		t.Errorf("error = %v, want it to name the repo and list the config's", err)
	}
	if len(*calls) != 0 {
		t.Errorf("gh was called although the ticket was refused: %v", *calls)
	}

	// A project without a config has nothing to check the repo against.
	bare := t.TempDir()
	body = writeBody(t, bare, strings.Replace(validBody, "repo: backend\n", "repo: web\n", 1))
	fakeGH(t, "https://github.com/acme/game/issues/9\n", nil)
	if _, err := CreateTask(context.Background(), CreateTaskOptions{Title: "t", BodyFile: body, StateRoot: bare}); err != nil {
		t.Errorf("without a config: %v", err)
	}
}

func TestCreateTask_DryRunCreatesNothing(t *testing.T) {
	dir := t.TempDir()
	body := writeBody(t, dir, validBody)
	calls := fakeGH(t, "", nil)

	result, err := CreateTask(context.Background(), CreateTaskOptions{Title: "[feat] x", BodyFile: body, StateRoot: dir, DryRun: true})

	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if len(*calls) != 0 {
		t.Errorf("a dry run called gh: %v", *calls)
	}
	if !strings.HasPrefix(result.DryRunCmd, "gh issue create --title [feat] x --body-file ") || !strings.HasSuffix(result.DryRunCmd, "--label ai-task") {
		t.Errorf("DryRunCmd = %q", result.DryRunCmd)
	}
}

// Nothing is created from a ticket the Worker and the review could not act on.
func TestCreateTask_RefusesBeforeCallingGitHub(t *testing.T) {
	dir := t.TempDir()
	good := writeBody(t, dir, validBody)
	bad := filepath.Join(dir, "bad.md")
	if err := os.WriteFile(bad, []byte("## Summary\nonly this\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, dir, "github: [this is not a mapping\n")

	tests := []struct {
		name        string
		opts        CreateTaskOptions
		wantInError string
	}{
		{"no title", CreateTaskOptions{BodyFile: good, StateRoot: t.TempDir()}, "title is required"},
		{"blank title", CreateTaskOptions{Title: "   ", BodyFile: good, StateRoot: t.TempDir()}, "title is required"},
		{"no body file", CreateTaskOptions{Title: "t", StateRoot: t.TempDir()}, "body file is required"},
		{"body file missing", CreateTaskOptions{Title: "t", BodyFile: filepath.Join(dir, "absent.md"), StateRoot: t.TempDir()}, "body file not found"},
		{"body incomplete", CreateTaskOptions{Title: "t", BodyFile: bad, StateRoot: t.TempDir()}, "missing required sections"},
		{"config unreadable", CreateTaskOptions{Title: "t", BodyFile: good, StateRoot: dir}, "failed to load config"},
	}
	for _, tt := range tests {
		calls := fakeGH(t, "https://github.com/acme/game/issues/1\n", nil)
		_, err := CreateTask(context.Background(), tt.opts)
		if err == nil || !strings.Contains(err.Error(), tt.wantInError) {
			t.Errorf("%s: error = %v, want it to mention %q", tt.name, err, tt.wantInError)
		}
		if len(*calls) != 0 {
			t.Errorf("%s: gh was called although the ticket was refused", tt.name)
		}
	}
}

func TestCreateTask_GitHubFailure(t *testing.T) {
	dir := t.TempDir()
	body := writeBody(t, dir, validBody)

	fakeGH(t, "HTTP 403: resource not accessible", errors.New("exit status 1"))
	if _, err := CreateTask(context.Background(), CreateTaskOptions{Title: "t", BodyFile: body, StateRoot: dir}); err == nil || !strings.Contains(err.Error(), "HTTP 403") {
		t.Errorf("error = %v, want gh's own message in it", err)
	}

	// gh succeeded but printed nothing that names the issue.
	fakeGH(t, "created\n", nil)
	if _, err := CreateTask(context.Background(), CreateTaskOptions{Title: "t", BodyFile: body, StateRoot: dir}); err == nil || !strings.Contains(err.Error(), "failed to parse issue number") {
		t.Errorf("error = %v, want a complaint about the missing issue number", err)
	}
}

func TestParseIssueOutput(t *testing.T) {
	tests := []struct {
		output  string
		wantNum int
		wantURL string
		wantErr bool
	}{
		{"https://github.com/owner/repo/issues/123\n", 123, "https://github.com/owner/repo/issues/123", false},
		{"Creating issue in owner/repo\n\nhttps://github.com/owner/repo/issues/7\n", 7, "https://github.com/owner/repo/issues/7", false},
		{"/issues/9", 9, "", false},
		{"no issue here", 0, "", true},
		{"", 0, "", true},
		{"https://github.com/owner/repo/pull/5", 0, "", true},
	}
	for _, tt := range tests {
		num, url, err := parseIssueOutput(tt.output)
		if (err != nil) != tt.wantErr || num != tt.wantNum || url != tt.wantURL {
			t.Errorf("parseIssueOutput(%q) = %d, %q, %v; want %d, %q, error %v", tt.output, num, url, err, tt.wantNum, tt.wantURL, tt.wantErr)
		}
	}
}

func TestResolveGHParams(t *testing.T) {
	cfg := &analyzer.Config{}
	if label, repo := resolveGHParams(CreateTaskOptions{}, cfg); label != "ai-task" || repo != "" {
		t.Errorf("defaults = %q, %q; want the default label and no repository", label, repo)
	}
	cfg.GitHub.Labels.Task = "work-item"
	cfg.GitHub.Repo = "acme/game"
	if label, repo := resolveGHParams(CreateTaskOptions{}, cfg); label != "work-item" || repo != "acme/game" {
		t.Errorf("from config = %q, %q", label, repo)
	}
	if _, repo := resolveGHParams(CreateTaskOptions{Repo: "acme/other"}, cfg); repo != "acme/other" {
		t.Errorf("repo = %q, want the one given for this ticket", repo)
	}
}
