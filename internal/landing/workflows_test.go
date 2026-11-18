package landing

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestExpectsChecks(t *testing.T) {
	cases := []struct {
		name, workflow string
		want           bool
	}{
		{"one event", "on: pull_request\njobs: {}\n", true},
		{"a list", "on: [push, pull_request]\njobs: {}\n", true},
		{"no filter", "on:\n  pull_request:\njobs: {}\n", true},
		{"its branch", "on:\n  pull_request:\n    branches: [develop]\njobs: {}\n", true},
		{"a pattern of it", "on:\n  pull_request:\n    branches: ['dev*']\njobs: {}\n", true},
		{"another branch", "on:\n  pull_request:\n    branches: [main]\njobs: {}\n", false},
		{"its branch ignored", "on:\n  pull_request:\n    branches-ignore: [develop]\njobs: {}\n", false},
		{"only some paths", "on:\n  pull_request:\n    paths: ['src/**']\njobs: {}\n", false},
		{"paths ignored", "on:\n  pull_request:\n    paths-ignore: ['docs/**']\njobs: {}\n", false},
		{"only when closed", "on:\n  pull_request:\n    types: [closed]\njobs: {}\n", false},
		{"opened among the types", "on:\n  pull_request:\n    types: [opened, labeled]\njobs: {}\n", true},
		{"pull_request_target", "on:\n  pull_request_target:\njobs: {}\n", true},
		{"pushes only", "on:\n  push:\n    branches: [develop]\njobs: {}\n", false},
		{"not a workflow", "this is: [not\n", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, dir, ".github/workflows/ci.yml", c.workflow)
			if got := ExpectsChecks(dir, "develop"); got != c.want {
				t.Errorf("ExpectsChecks = %v, want %v", got, c.want)
			}
		})
	}
	if ExpectsChecks(t.TempDir(), "develop") {
		t.Error("a repository without workflows expects checks")
	}
	dir := t.TempDir()
	write(t, dir, ".github/workflows/ci.yaml", "on: pull_request\n")
	if !ExpectsChecks(dir, "develop") {
		t.Error("a .yaml workflow was not read")
	}
}

// commitWorkflow puts a workflow on develop, as the project has it.
func (p *project) commitWorkflow(text string) {
	write(p.t, p.root, ".github/workflows/ci.yml", text)
	git(p.t, p.root, "add", ".github")
	git(p.t, p.root, "commit", "--quiet", "-m", "ci")
	git(p.t, p.root, "push", "--quiet", "origin", "develop")
}

// A project with CI for pull requests whose pull request GitHub never ran
// it for is not merged as if its checks had passed.
func TestLand_CIThatNeverStartsIsNotTakenForNoCI(t *testing.T) {
	p := newProject(t)
	p.commitWorkflow("on:\n  pull_request:\n    branches: [develop]\njobs: {}\n")
	p.meeting()
	opts := p.options(p.pending())
	var said []string
	opts.Progress = func(line string) { said = append(said, line) }
	_, err := Land(context.Background(), opts)
	var absent *ChecksAbsent
	if !errors.As(err, &absent) || len(p.gh.merged) != 0 {
		t.Fatalf("err = %v, merged %v", err, p.gh.merged)
	}
	if _, ok := p.onOrigin("docs/plans/p1/tickets.yaml"); ok {
		t.Error("the documents are on develop")
	}
	found := false
	for _, line := range said {
		found = found || line == "Waiting for the CI of pull request #41 to start"
	}
	if !found {
		t.Errorf("progress said %v", said)
	}

	// Its checks show up at last: it is merged once they pass.
	p.gh.checks, p.gh.looks = [][]Check{{{Name: "ci", Bucket: "pending"}}, {{Name: "ci", Bucket: "pass"}}}, 0
	if _, err := Land(context.Background(), p.options(p.pending())); err != nil || len(p.gh.merged) != 1 {
		t.Errorf("err = %v, merged %v", err, p.gh.merged)
	}
}

// CI that runs only for some paths may well not start for documents: the
// pull request is then one of a project without CI for it.
func TestLand_CIForOtherPathsIsNoCIForDocuments(t *testing.T) {
	p := newProject(t)
	p.commitWorkflow("on:\n  pull_request:\n    paths: ['src/**']\njobs: {}\n")
	p.meeting()
	if _, err := Land(context.Background(), p.options(p.pending())); err != nil || len(p.gh.merged) != 1 {
		t.Errorf("err = %v, merged %v", err, p.gh.merged)
	}
	if _, err := os.Stat(filepath.Join(p.root, ".github", "workflows", "ci.yml")); err != nil {
		t.Fatal(err)
	}
}
