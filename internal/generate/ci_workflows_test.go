package generate

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/layout"
	"gopkg.in/yaml.v3"
)

// workflowNames turns the paths a run reports into names relative to the
// project's .github/workflows, or to the project for a workflow outside it.
func workflowNames(t *testing.T, dir string, paths []string) []string {
	t.Helper()
	names := []string{}
	for _, path := range paths {
		rel, err := filepath.Rel(filepath.Join(dir, ".github", "workflows"), path)
		if err != nil || strings.HasPrefix(rel, "..") {
			rel, _ = filepath.Rel(dir, path)
		}
		names = append(names, filepath.ToSlash(rel))
	}
	sort.Strings(names)
	return names
}

// runCI carries out a policy and returns the names of the workflows it wrote
// and removed.
func runCI(t *testing.T, dir string, ctx *TemplateContext, policy CIPolicy) (written, removed []string) {
	t.Helper()
	writtenPaths, removedPaths, err := generateCIWorkflows(dir, ctx, policy, progress{quiet: true})
	if err != nil {
		t.Fatalf("generateCIWorkflows: %v", err)
	}
	for _, path := range writtenPaths {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("reported %s as written, but it is not there", path)
		}
	}
	for _, path := range removedPaths {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("reported %s as removed, but it is still there", path)
		}
	}
	return workflowNames(t, dir, writtenPaths), workflowNames(t, dir, removedPaths)
}

// projectWorkflows lists what is in the project's .github/workflows.
func projectWorkflows(t *testing.T, dir string) []string {
	t.Helper()
	names := []string{}
	entries, err := os.ReadDir(filepath.Join(dir, ".github", "workflows"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
}

func readWorkflowFile(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".github", "workflows", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// takeOver does what the header tells a project to do: delete the comment.
func takeOver(t *testing.T, dir, name string) string {
	t.Helper()
	content := readWorkflowFile(t, dir, name)
	if !strings.HasPrefix(content, ciGeneratedHeader) {
		t.Fatalf("%s does not start with the generated header:\n%s", name, content)
	}
	own := strings.TrimPrefix(content, ciGeneratedHeader)
	writeWorkflowFile(t, dir, name, own)
	return own
}

func TestCIWorkflows_GoBackendAndCSharpFrontend(t *testing.T) {
	dir := t.TempDir()

	written, removed := runCI(t, dir, ciContext(goBackend, csharpFrontend), CISync)
	if !reflect.DeepEqual(written, []string{"ci-backend.yml", "ci-frontend.yml"}) || len(removed) != 0 {
		t.Fatalf("wrote %v, removed %v; want one workflow per repo", written, removed)
	}
	workflows := filepath.Join(dir, ".github", "workflows")

	backend := parseWorkflow(t, filepath.Join(workflows, "ci-backend.yml"))
	if backend.Name != "backend CI" {
		t.Errorf("backend workflow name = %q", backend.Name)
	}
	if got := backend.Jobs["build"].Defaults.Run.WorkingDirectory; got != "backend" {
		t.Errorf("backend job runs in %q, want backend", got)
	}
	if got := backend.actions(); !reflect.DeepEqual(got, []string{"actions/checkout", "actions/setup-go"}) {
		t.Errorf("backend actions = %v", got)
	}
	if got := backend.commands(); !reflect.DeepEqual(got, []string{"go build ./...", "go test ./..."}) {
		t.Errorf("backend commands = %q, want the repo's build and test commands", got)
	}

	frontend := parseWorkflow(t, filepath.Join(workflows, "ci-frontend.yml"))
	if got := frontend.Jobs["build"].Defaults.Run.WorkingDirectory; got != "frontend" {
		t.Errorf("frontend job runs in %q, want frontend", got)
	}
	if got := frontend.actions(); !reflect.DeepEqual(got, []string{"actions/checkout", "actions/setup-dotnet"}) {
		t.Errorf("frontend actions = %v", got)
	}
	if got := frontend.commands(); !reflect.DeepEqual(got, []string{"dotnet build", "dotnet test"}) {
		t.Errorf("frontend commands = %q", got)
	}
	if got := frontend.setup(t, "actions/setup-dotnet").With["dotnet-version"]; got != ciDotnetSDK {
		t.Errorf("dotnet-version = %q, want %q", got, ciDotnetSDK)
	}

	for name, w := range map[string]ciWorkflow{"backend": backend, "frontend": frontend} {
		// The triggers come from the project's own branches.
		if !reflect.DeepEqual(w.On.Push.Branches, []string{"develop", "main"}) || !reflect.DeepEqual(w.On.PullRequest.Branches, []string{"develop"}) {
			t.Errorf("%s triggers: push %v, pull_request %v; want the configured branches", name, w.On.Push.Branches, w.On.PullRequest.Branches)
		}
		// A build needs to read the code and nothing else.
		if !reflect.DeepEqual(w.Permissions, map[string]string{"contents": "read"}) {
			t.Errorf("%s permissions = %v, want read-only contents", name, w.Permissions)
		}
	}
}

func TestCIWorkflows_LanguageDecidesTheToolchain(t *testing.T) {
	tests := map[string]string{
		"go":      "actions/setup-go",
		"Golang":  "actions/setup-go",
		" go ":    "actions/setup-go",
		"csharp":  "actions/setup-dotnet",
		"C#":      "actions/setup-dotnet",
		"dotnet":  "actions/setup-dotnet",
		"godot":   "actions/setup-dotnet", // a Godot .NET project needs only the .NET SDK to build and test
		"generic": "",
		"elixir":  "",
		"":        "",
	}
	for language, wantSetup := range tests {
		dir := t.TempDir()
		repo := RepoConfig{Name: "app", Path: "./", Type: "root", Language: language,
			Verify: VerifyConfig{Build: "make build", Test: "make test"}}

		if written, _ := runCI(t, dir, ciContext(repo), CISync); !reflect.DeepEqual(written, []string{"ci.yml"}) {
			t.Fatalf("language %q: wrote %v, want ci.yml for the repository root", language, written)
		}
		w := parseWorkflow(t, filepath.Join(dir, ".github", "workflows", "ci.yml"))

		want := []string{"actions/checkout"}
		if wantSetup != "" {
			want = append(want, wantSetup)
		}
		if got := w.actions(); !reflect.DeepEqual(got, want) {
			t.Errorf("language %q: actions = %v, want %v", language, got, want)
		}
		if got := w.commands(); !reflect.DeepEqual(got, []string{"make build", "make test"}) {
			t.Errorf("language %q: commands = %q, want the repo's own", language, got)
		}
		if got := w.Jobs["build"].Defaults.Run.WorkingDirectory; got != "" {
			t.Errorf("language %q: a root repo needs no working directory, got %q", language, got)
		}
	}
}

func TestCIWorkflows_RepoWithoutCommandsGetsNoWorkflow(t *testing.T) {
	dir := t.TempDir()
	unset := RepoConfig{Name: "root", Path: "./", Type: "root", Language: "generic"}
	blank := RepoConfig{Name: "docs", Path: "docs/", Type: "directory", Language: "generic", Verify: VerifyConfig{Build: "  ", Test: ""}}
	setupOnly := RepoConfig{Name: "tools", Path: "tools/", Type: "directory", Language: "generic", Verify: VerifyConfig{Setup: "make deps"}}

	written, _ := runCI(t, dir, ciContext(unset, blank, setupOnly, goBackend), CISync)

	if !reflect.DeepEqual(written, []string{"ci-backend.yml"}) {
		t.Errorf("wrote %v, want only the repo that has something to build or test", written)
	}
	if got := projectWorkflows(t, dir); !reflect.DeepEqual(got, []string{"ci-backend.yml"}) {
		t.Errorf("workflows = %v: a workflow with nothing to run must not be written", got)
	}
}

// Every generated workflow says at its top that awkit owns it and how to take
// it over; that first line is what awkit recognizes its own files by.
func TestCIWorkflows_StartWithTheOwnershipMark(t *testing.T) {
	contents := map[string]string{
		"a repo's workflow":   generateCIContent(goBackend, ciGit),
		"the submodule check": generateValidateSubmodulesWorkflow(ciGit),
	}
	for name, content := range contents {
		if !strings.HasPrefix(content, ciGeneratedMark) {
			t.Errorf("%s does not start with the ownership mark:\n%s", name, content)
		}
		header := strings.SplitN(content, "\n\n", 2)[0]
		if !strings.Contains(header, "delete this comment") {
			t.Errorf("%s: the header should say how to take the file over, got:\n%s", name, header)
		}
		var parsed map[string]any
		if err := yaml.Unmarshal([]byte(content), &parsed); err != nil {
			t.Errorf("%s is not valid YAML: %v", name, err)
		}
	}
}

// While awkit owns a workflow, the workflow follows the config.
func TestCIWorkflows_FollowTheConfigWhileAwkitOwnsThem(t *testing.T) {
	changed := goBackend
	changed.Verify.Test = "go test -race ./..."

	for name, policy := range map[string]CIPolicy{"refresh": CIRefresh, "sync": CISync, "rewrite": CIRewrite} {
		dir := t.TempDir()
		runCI(t, dir, ciContext(goBackend), CISync)

		written, removed := runCI(t, dir, ciContext(changed), policy)

		if !reflect.DeepEqual(written, []string{"ci-backend.yml"}) || len(removed) != 0 {
			t.Errorf("%s: wrote %v, removed %v; want the workflow rewritten", name, written, removed)
		}
		w := parseWorkflow(t, filepath.Join(dir, ".github", "workflows", "ci-backend.yml"))
		if got := w.commands(); !reflect.DeepEqual(got, []string{"go build ./...", "go test -race ./..."}) {
			t.Errorf("%s: commands = %q, want the config's new test command", name, got)
		}
	}
}

// A project takes a workflow over by deleting the comment at its top. From
// then on the file is the project's, and only a rewrite touches it.
func TestCIWorkflows_TakenOverIsLeftAlone(t *testing.T) {
	changed := goBackend
	changed.Verify.Test = "go test -race ./..."

	for name, policy := range map[string]CIPolicy{"refresh": CIRefresh, "sync": CISync} {
		dir := t.TempDir()
		runCI(t, dir, ciContext(goBackend), CISync)
		own := takeOver(t, dir, "ci-backend.yml")

		written, removed := runCI(t, dir, ciContext(changed), policy)

		if len(written) != 0 || len(removed) != 0 {
			t.Errorf("%s: wrote %v, removed %v; the project's own workflow must be left alone", name, written, removed)
		}
		if got := readWorkflowFile(t, dir, "ci-backend.yml"); got != own {
			t.Errorf("%s: the project's workflow was changed:\n%s", name, got)
		}
	}

	dir := t.TempDir()
	runCI(t, dir, ciContext(goBackend), CISync)
	takeOver(t, dir, "ci-backend.yml")

	written, _ := runCI(t, dir, ciContext(changed), CIRewrite)

	if !reflect.DeepEqual(written, []string{"ci-backend.yml"}) {
		t.Errorf("rewrite: wrote %v, want the workflow written from the config", written)
	}
	if got := readWorkflowFile(t, dir, "ci-backend.yml"); !strings.HasPrefix(got, ciGeneratedMark) || !strings.Contains(got, "go test -race ./...") {
		t.Errorf("rewrite: the workflow should be awkit's again and follow the config:\n%s", got)
	}
}

// The mark counts only as the very first thing in the file. Anything else,
// including a workflow that merely quotes it, is the project's own.
func TestCIWorkflows_OnlyTheFirstLineMarksOwnership(t *testing.T) {
	for name, own := range map[string]string{
		"written by the project":   "name: mine\n",
		"mark after a blank line":  "\n" + generateCIContent(goBackend, ciGit),
		"mark after another line":  "# our pipeline\n" + generateCIContent(goBackend, ciGit),
		"mark indented":            " " + generateCIContent(goBackend, ciGit),
		"an empty file":            "",
		"mark quoted further down": "name: mine\n" + ciGeneratedMark + "\n",
	} {
		dir := t.TempDir()
		writeWorkflowFile(t, dir, "ci-backend.yml", own)

		written, removed := runCI(t, dir, ciContext(goBackend), CISync)

		if len(written) != 0 || len(removed) != 0 {
			t.Errorf("%s: wrote %v, removed %v; want the file left alone", name, written, removed)
		}
		if got := readWorkflowFile(t, dir, "ci-backend.yml"); got != own {
			t.Errorf("%s: the file was changed:\n%s", name, got)
		}
	}
}

func TestCIWorkflows_SyncWritesWhatIsMissingAndKeepsTheProjectsOwn(t *testing.T) {
	dir := t.TempDir()
	const own = "name: mine\n"
	writeWorkflowFile(t, dir, "ci-backend.yml", own)

	written, _ := runCI(t, dir, ciContext(goBackend, csharpFrontend), CISync)
	if !reflect.DeepEqual(written, []string{"ci-frontend.yml"}) {
		t.Errorf("wrote %v, want only the missing workflow", written)
	}
	if got := readWorkflowFile(t, dir, "ci-backend.yml"); got != own {
		t.Errorf("the project's own workflow was overwritten:\n%s", got)
	}

	written, _ = runCI(t, dir, ciContext(goBackend, csharpFrontend), CIRewrite)
	if !reflect.DeepEqual(written, []string{"ci-backend.yml", "ci-frontend.yml"}) {
		t.Errorf("rewrite wrote %v, want both", written)
	}
	if got := readWorkflowFile(t, dir, "ci-backend.yml"); got == own {
		t.Error("a rewrite should replace the project's own workflow")
	}
}

// What an upgrade asks for: awkit's own workflows follow the new kit, and no
// workflow appears that the project did not have.
func TestCIWorkflows_RefreshAddsNothing(t *testing.T) {
	dir := t.TempDir()

	written, removed := runCI(t, dir, ciContext(goBackend, csharpFrontend), CIRefresh)

	if len(written) != 0 || len(removed) != 0 {
		t.Errorf("wrote %v, removed %v; a refresh adds nothing", written, removed)
	}
	if got := projectWorkflows(t, dir); len(got) != 0 {
		t.Errorf("workflows = %v, want none", got)
	}
}

func TestCIWorkflows_NoneLeavesEverythingAlone(t *testing.T) {
	dir := t.TempDir()
	runCI(t, dir, ciContext(goBackend, csharpFrontend), CISync)
	before := readWorkflowFile(t, dir, "ci-backend.yml")
	changed := goBackend
	changed.Verify.Test = "go test -race ./..."

	// The backend changed and the frontend is gone from the config.
	written, removed := runCI(t, dir, ciContext(changed), CINone)

	if len(written) != 0 || len(removed) != 0 {
		t.Errorf("wrote %v, removed %v under CINone", written, removed)
	}
	if got := readWorkflowFile(t, dir, "ci-backend.yml"); got != before {
		t.Error("CINone rewrote a workflow")
	}
	if got := projectWorkflows(t, dir); !reflect.DeepEqual(got, []string{"ci-backend.yml", "ci-frontend.yml"}) {
		t.Errorf("workflows = %v, want both still there", got)
	}
}

// Earlier versions of the kit wrote one ci.yml for every repo. A project that
// has it keeps it as its CI: per-repo workflows next to it would build the
// repos twice.
func TestCIWorkflows_ProjectsOwnCIYmlCoversEveryRepo(t *testing.T) {
	const own = "name: ci\non: push\njobs:\n  backend:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make\n"

	for name, policy := range map[string]CIPolicy{"refresh": CIRefresh, "sync": CISync} {
		dir := t.TempDir()
		writeWorkflowFile(t, dir, "ci.yml", own)

		written, removed := runCI(t, dir, ciContext(goBackend, csharpFrontend), policy)

		if len(written) != 0 || len(removed) != 0 {
			t.Errorf("%s: wrote %v, removed %v next to the project's own ci.yml", name, written, removed)
		}
		if got := readWorkflowFile(t, dir, "ci.yml"); got != own {
			t.Errorf("%s: the project's ci.yml was changed:\n%s", name, got)
		}
	}

	// Asked to rewrite, the per-repo workflows are written. ci.yml belongs to
	// no repo of this config and is not touched.
	dir := t.TempDir()
	writeWorkflowFile(t, dir, "ci.yml", own)
	written, removed := runCI(t, dir, ciContext(goBackend, csharpFrontend), CIRewrite)
	if !reflect.DeepEqual(written, []string{"ci-backend.yml", "ci-frontend.yml"}) || len(removed) != 0 {
		t.Errorf("rewrite: wrote %v, removed %v; want the per-repo workflows", written, removed)
	}
	if got := readWorkflowFile(t, dir, "ci.yml"); got != own {
		t.Errorf("rewrite: ci.yml must stay as it was:\n%s", got)
	}
}

// What the generator says about a workflow it keeps names the file from the
// project's root, as its other lines do: not with the full path of the
// project on the machine it runs on.
func TestCIWorkflows_NotesNameFilesFromTheProjectsRoot(t *testing.T) {
	const own = "name: ci\non: push\njobs:\n  backend:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make\n"

	// The project's own ci.yml covers the repos.
	dir := t.TempDir()
	writeWorkflowFile(t, dir, "ci.yml", own)
	covered := planCIWorkflows(dir, ciContext(goBackend, csharpFrontend), CISync).notes
	if want := []string{"Kept: .github/workflows/ci.yml is the project's own and is taken to cover every repo, so no per-repo workflow is added (awkit generate --generate-ci writes them)"}; !reflect.DeepEqual(covered, want) {
		t.Errorf("notes = %q, want %q", covered, want)
	}
	// Asked to rewrite: the per-repo workflows are written, and ci.yml is
	// pointed out.
	rewrite := planCIWorkflows(dir, ciContext(goBackend, csharpFrontend), CIRewrite).notes
	if want := []string{"NOTE: .github/workflows/ci.yml does not come from this config; remove it if it repeats the per-repo workflows"}; !reflect.DeepEqual(rewrite, want) {
		t.Errorf("notes = %q, want %q", rewrite, want)
	}

	// A per-repo workflow the project took over.
	dir = t.TempDir()
	runCI(t, dir, ciContext(goBackend), CISync)
	takeOver(t, dir, "ci-backend.yml")
	kept := planCIWorkflows(dir, ciContext(goBackend), CISync).notes
	if want := []string{"Kept: .github/workflows/ci-backend.yml is the project's own"}; !reflect.DeepEqual(kept, want) {
		t.Errorf("notes = %q, want %q", kept, want)
	}
}

// A per-repo workflow awkit already owns keeps following the config even
// where the project owns ci.yml; only adding one next to it is held back.
func TestCIWorkflows_ProjectsOwnCIYmlDoesNotFreezeAwkitsWorkflows(t *testing.T) {
	dir := t.TempDir()
	runCI(t, dir, ciContext(goBackend), CISync)
	writeWorkflowFile(t, dir, "ci.yml", "name: ci\n")
	changed := goBackend
	changed.Verify.Test = "go test -race ./..."

	written, removed := runCI(t, dir, ciContext(changed, csharpFrontend), CISync)

	if !reflect.DeepEqual(written, []string{"ci-backend.yml"}) || len(removed) != 0 {
		t.Errorf("wrote %v, removed %v; want the backend workflow refreshed and no frontend workflow added", written, removed)
	}
	if got := readWorkflowFile(t, dir, "ci-backend.yml"); !strings.Contains(got, "go test -race ./...") {
		t.Errorf("the backend workflow did not follow the config:\n%s", got)
	}
}

// A ci.yml that awkit wrote for the root repo is not the project's own: the
// directory repos next to it still get their workflows, on the first run and
// when one is added later.
func TestCIWorkflows_RootAndDirectoryRepos(t *testing.T) {
	dir := t.TempDir()
	root := RepoConfig{Name: "root", Path: "./", Type: "root", Language: "generic", Verify: VerifyConfig{Test: "make test"}}

	written, _ := runCI(t, dir, ciContext(root, goBackend), CISync)
	if !reflect.DeepEqual(written, []string{"ci-backend.yml", "ci.yml"}) {
		t.Fatalf("wrote %v, want a workflow for the root repo and one for the directory repo", written)
	}

	written, _ = runCI(t, dir, ciContext(root, goBackend, csharpFrontend), CISync)
	if !reflect.DeepEqual(written, []string{"ci-backend.yml", "ci-frontend.yml", "ci.yml"}) {
		t.Errorf("wrote %v, want the new repo's workflow added and the others refreshed", written)
	}
}

// A workflow awkit owns goes when its repo leaves the config or loses its
// commands. Whatever the project owns stays.
func TestCIWorkflows_RemovesWhatNoRepoRunsAnyMore(t *testing.T) {
	withoutCommands := csharpFrontend
	withoutCommands.Verify = VerifyConfig{}

	tests := []struct {
		name   string
		policy CIPolicy
		after  *TemplateContext
	}{
		{"refresh, repo gone", CIRefresh, ciContext(goBackend)},
		{"sync, repo gone", CISync, ciContext(goBackend)},
		{"rewrite, repo gone", CIRewrite, ciContext(goBackend)},
		{"sync, repo lost its commands", CISync, ciContext(goBackend, withoutCommands)},
	}
	for _, tt := range tests {
		dir := t.TempDir()
		runCI(t, dir, ciContext(goBackend, csharpFrontend), CISync)
		const release = "name: release\n"
		writeWorkflowFile(t, dir, "release.yml", release)
		writeWorkflowFile(t, dir, "ci-legacy.yml", "name: legacy\n")

		_, removed := runCI(t, dir, tt.after, tt.policy)

		if !reflect.DeepEqual(removed, []string{"ci-frontend.yml"}) {
			t.Errorf("%s: removed %v, want only the workflow of the repo that is gone", tt.name, removed)
		}
		if got := projectWorkflows(t, dir); !reflect.DeepEqual(got, []string{"ci-backend.yml", "ci-legacy.yml", "release.yml"}) {
			t.Errorf("%s: workflows = %v", tt.name, got)
		}
		if got := readWorkflowFile(t, dir, "release.yml"); got != release {
			t.Errorf("%s: the project's release workflow was changed", tt.name)
		}
	}

	// A workflow the project took over is not removed with its repo.
	dir := t.TempDir()
	runCI(t, dir, ciContext(goBackend, csharpFrontend), CISync)
	takeOver(t, dir, "ci-frontend.yml")
	if _, removed := runCI(t, dir, ciContext(goBackend), CISync); len(removed) != 0 {
		t.Errorf("removed %v, which the project had taken over", removed)
	}
}

func TestCIWorkflows_SubmoduleCarriesItsOwnWorkflow(t *testing.T) {
	dir := t.TempDir()
	submodule := RepoConfig{Name: "engine", Path: "engine/", Type: "submodule", Language: "go", Verify: VerifyConfig{Test: "go test ./..."}}
	// The project's own ci.yml says nothing about another repository.
	writeWorkflowFile(t, dir, "ci.yml", "name: ci\n")
	ctx := ciContext(submodule)
	ctx.HasSubmodules = true

	written, _ := runCI(t, dir, ctx, CISync)

	if !reflect.DeepEqual(written, []string{"engine/.github/workflows/ci.yml", "validate-submodules.yml"}) {
		t.Fatalf("wrote %v, want the submodule's workflow inside the submodule and the project's submodule check", written)
	}
	w := parseWorkflow(t, filepath.Join(dir, "engine", ".github", "workflows", "ci.yml"))
	if got := w.Jobs["build"].Defaults.Run.WorkingDirectory; got != "" {
		t.Errorf("the submodule's workflow runs at its own root, got working directory %q", got)
	}
}

func TestCIWorkflows_SubmoduleCheck(t *testing.T) {
	dir := t.TempDir()
	ctx := ciContext(goBackend)
	ctx.HasSubmodules = true

	written, _ := runCI(t, dir, ctx, CISync)
	if !reflect.DeepEqual(written, []string{"ci-backend.yml", "validate-submodules.yml"}) {
		t.Errorf("wrote %v, want the repo workflow and validate-submodules.yml", written)
	}
	if got := readWorkflowFile(t, dir, "validate-submodules.yml"); !strings.Contains(got, "uses: "+actionCheckout+"\n") {
		t.Errorf("validate-submodules.yml should check out with %s:\n%s", actionCheckout, got)
	}

	// Taken over, it is the project's like any other workflow.
	own := takeOver(t, dir, "validate-submodules.yml")
	if written, _ := runCI(t, dir, ctx, CISync); !reflect.DeepEqual(written, []string{"ci-backend.yml"}) {
		t.Errorf("wrote %v; the submodule check now belongs to the project", written)
	}
	if got := readWorkflowFile(t, dir, "validate-submodules.yml"); got != own {
		t.Errorf("the project's validate-submodules.yml was changed:\n%s", got)
	}

	// While awkit owns it, it goes when the project has no submodules left.
	runCI(t, dir, ctx, CIRewrite)
	if _, removed := runCI(t, dir, ciContext(goBackend), CISync); !reflect.DeepEqual(removed, []string{"validate-submodules.yml"}) {
		t.Errorf("removed %v, want the submodule check of a project without submodules", removed)
	}
}

func TestCIWorkflows_NoRepos(t *testing.T) {
	dir := t.TempDir()
	if written, removed := runCI(t, dir, ciContext(), CISync); len(written) != 0 || len(removed) != 0 {
		t.Errorf("wrote %v, removed %v for a project without repos", written, removed)
	}
	if _, err := os.Stat(filepath.Join(dir, ".github")); !os.IsNotExist(err) {
		t.Error("nothing to write, yet .github was created")
	}
}

// A workflow that could not be written is an error, not a shorter list.
func TestCIWorkflows_WriteFailureIsReported(t *testing.T) {
	dir := t.TempDir()
	// .github is a file, so the workflows directory cannot be created.
	if err := os.WriteFile(filepath.Join(dir, ".github"), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}

	written, _, err := generateCIWorkflows(dir, ciContext(goBackend), CISync, progress{quiet: true})

	if err == nil {
		t.Fatalf("no error although nothing could be written (reported %v)", written)
	}
	if !strings.Contains(err.Error(), "backend") {
		t.Errorf("the error should name the repo, got: %v", err)
	}
	if len(written) != 0 {
		t.Errorf("reported %v as written", written)
	}
}

const ciTestConfig = `version: "1.2"
project:
  name: demo
  type: monorepo
repos:
  - name: backend
    path: backend/
    type: directory
    language: go
    verify:
      build: "go build ./..."
      test: "TEST_COMMAND"
git:
  integration_branch: develop
  release_branch: main
`

func writeCITestConfig(t *testing.T, dir, testCommand string) {
	t.Helper()
	path := layout.Config(dir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	config := strings.Replace(ciTestConfig, "TEST_COMMAND", testCommand, 1)
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGenerate_CIFollowsThePolicy(t *testing.T) {
	workflow := func(dir string) string { return filepath.Join(dir, ".github", "workflows", "ci-backend.yml") }
	lists := func(result *Result, path string) bool {
		for _, file := range result.GeneratedFiles {
			if file == path {
				return true
			}
		}
		return false
	}

	// The zero value leaves CI alone.
	without := t.TempDir()
	writeCITestConfig(t, without, "go test ./...")
	if _, err := Generate(Options{StateRoot: without}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if _, err := os.Stat(workflow(without)); !os.IsNotExist(err) {
		t.Error("no CI workflow should be written unless a policy asks for it")
	}

	dir := t.TempDir()
	writeCITestConfig(t, dir, "go test ./...")
	result, err := Generate(Options{StateRoot: dir, CI: CISync})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if _, err := os.Stat(workflow(dir)); err != nil {
		t.Fatalf("CISync should write the workflow: %v", err)
	}
	if !lists(result, workflow(dir)) {
		t.Errorf("the result should list the workflow it wrote, got %v", result.GeneratedFiles)
	}

	// The config changes; the workflow follows.
	writeCITestConfig(t, dir, "go test -race ./...")
	if _, err := Generate(Options{StateRoot: dir, CI: CISync}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got, _ := os.ReadFile(workflow(dir)); !strings.Contains(string(got), "go test -race ./...") {
		t.Errorf("the workflow did not follow the config:\n%s", got)
	}

	// Taken over, it is left alone until a rewrite is asked for.
	const own = "name: mine\n"
	if err := os.WriteFile(workflow(dir), []byte(own), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err = Generate(Options{StateRoot: dir, CI: CISync})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got, _ := os.ReadFile(workflow(dir)); string(got) != own {
		t.Errorf("CISync overwrote the project's workflow:\n%s", got)
	}
	if lists(result, workflow(dir)) {
		t.Error("the result lists a workflow that was left alone")
	}
	if _, err := Generate(Options{StateRoot: dir, CI: CIRewrite}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got, _ := os.ReadFile(workflow(dir)); string(got) == own {
		t.Error("CIRewrite should write the workflow from the config")
	}
}

func TestGenerate_DryRunListsTheWorkflowsWithoutWritingThem(t *testing.T) {
	dir := t.TempDir()
	writeCITestConfig(t, dir, "go test ./...")
	workflow := filepath.Join(dir, ".github", "workflows", "ci-backend.yml")

	result, err := Generate(Options{StateRoot: dir, CI: CISync, DryRun: true})

	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	listed := false
	for _, file := range result.GeneratedFiles {
		listed = listed || file == workflow
	}
	if !listed {
		t.Errorf("a dry run should list the workflow it would write, got %v", result.GeneratedFiles)
	}
	if _, err := os.Stat(filepath.Join(dir, ".github")); !os.IsNotExist(err) {
		t.Error("a dry run wrote to the project")
	}
}

// A workflow that cannot be written fails the run, after everything else is in
// place.
func TestGenerate_CIFailureIsAnError(t *testing.T) {
	dir := t.TempDir()
	writeCITestConfig(t, dir, "go test ./...")
	if err := os.WriteFile(filepath.Join(dir, ".github"), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Generate(Options{StateRoot: dir, CI: CISync})

	if err == nil {
		t.Fatal("Generate reported success although the workflow could not be written")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "CLAUDE.md")); statErr != nil {
		t.Errorf("the other generated files should be in place all the same: %v", statErr)
	}
}
