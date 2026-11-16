package generate

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	awkit "github.com/silver2dream/ai-workflow-kit"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
)

// generated runs one of the generators for a configuration and returns what
// it wrote. The rule files the configuration lists are in the project: a rule
// named after a repo is about that repo's directory, any other about
// everything.
func generated(t *testing.T, generate func(string, *TemplateContext) error, cfg Config) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range cfg.Rules.Custom {
		writeRuleFile(t, root, layout.RulesDir, name, cfg)
	}
	for _, name := range cfg.Rules.Kit {
		if name != gitWorkflowRule {
			writeRuleFile(t, root, layout.KitRulesDir, name, cfg)
		}
	}
	return generatedIn(t, root, generate, cfg)
}

// generatedIn runs one of the generators for a configuration in a project
// directory as it is.
func generatedIn(t *testing.T, root string, generate func(string, *TemplateContext) error, cfg Config) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "out.md")
	if err := generate(out, buildContext(root, &cfg)); err != nil {
		t.Fatalf("generate: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func writeRuleFile(t *testing.T, root, dir, name string, cfg Config) {
	t.Helper()
	text := "# " + name + "\n\nA rule.\n"
	for _, repo := range cfg.Repos {
		if repo.Name == name {
			text = "---\npaths:\n  - \"" + repo.Path + "**\"\n---\n\n" + text
		}
	}
	file := filepath.Join(root, filepath.FromSlash(dir), name+".md")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func godotGoProject() Config {
	return Config{
		Project: ProjectConfig{Name: "demo", Type: "monorepo"},
		Repos: []RepoConfig{
			{Name: "backend", Path: "backend/", Type: "directory", Language: "go"},
			{Name: "frontend", Path: "frontend/", Type: "directory", Language: "csharp"},
		},
		Git:   GitConfig{IntegrationBranch: "develop", ReleaseBranch: "main"},
		Rules: RulesConfig{Kit: []string{"git-workflow"}, Custom: []string{"backend", "frontend"}},
	}
}

// CLAUDE.md is loaded into every session of the project, the Principal's for
// each of its loop iterations. It says who the reader is and where each thing
// is defined, and leaves the things themselves where they are defined.
func TestGenerateClaudeMd_IsAMap(t *testing.T) {
	got := generated(t, generateClaudeMd, godotGoProject())

	for _, want := range []string{
		"you are the **Principal**",
		"you are a **Worker**. Read `AGENTS.md` instead of this file.",
		"you are working with a person, and none of these roles is yours",
		"| Repos, branches, verify commands, labels, review and retry settings | `.ai/config.yaml` |",
		"`.claude/skills/principal-workflow/SKILL.md`",
		"| What a ticket must contain | `.claude/skills/principal-workflow/references/ticket.md` |",
		"| Branches, commit format, pull requests | `.claude/rules/_kit/git-workflow.md` |",
		"| How code and documents are written here | The rule files under `.claude/rules/`. Each names the files it is about; `AGENTS.md` lists them |",
		"`docs/README.md` (the index) and `docs/decisions.md` (append-only)",
		"| Who sits on the planning council | `.ai/roster.yaml` |",
		"`.ai/state/`; read it with `awkit status` and `awkit events`",
		"- Name: demo (monorepo)",
		"- Repos: `backend` (`backend/`, go), `frontend` (`frontend/`, csharp)",
		"merged into `develop`; `main` is the release branch",
		"do not read `.ai/state/logs/` to follow the Worker",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("CLAUDE.md lacks %q", want)
		}
	}

	// What it used to carry and its definition places carry now.
	for _, gone := range []string{"phases/", "tasks/create-task", "Quick Reference", "REPO TYPE SUPPORT", "File Locations", "MUST FOLLOW", "awkit reset"} {
		if strings.Contains(got, gone) {
			t.Errorf("CLAUDE.md still carries %q", gone)
		}
	}
	if lines := strings.Count(got, "\n"); lines > 45 {
		t.Errorf("CLAUDE.md is %d lines: it is a map, and every line of it is read on every iteration of a run", lines)
	}

	// Every file of the kit it names is one the kit ships.
	named := regexp.MustCompile("`([.]claude/(?:skills|rules/_kit)/[A-Za-z0-9_./-]+[.]md)`").FindAllStringSubmatch(got, -1)
	if len(named) < 3 {
		t.Fatalf("CLAUDE.md names %d files of the kit, want the skill, the ticket reference and the git rule", len(named))
	}
	for _, match := range named {
		if _, err := fs.Stat(awkit.KitFS, match[1]); err != nil {
			t.Errorf("CLAUDE.md names %s, which the kit does not ship", match[1])
		}
	}
}

// A project with no rule file of its own is told where one goes. One that
// has rules gets one row for all of them, however many they are: the file is
// read on every iteration of a run, and which rule is about which files is in
// AGENTS.md. A rule of the kit other than the git rule counts like the
// project's own.
func TestGenerateClaudeMd_TheRulesRow(t *testing.T) {
	const where = "| How code is written here | No rule file yet. One goes in `.claude/rules/` and is listed under `rules.custom` in the configuration |"
	const row = "| How code and documents are written here | The rule files under `.claude/rules/`. Each names the files it is about; `AGENTS.md` lists them |"

	none := godotGoProject()
	none.Rules.Custom = nil
	got := generated(t, generateClaudeMd, none)
	if !strings.Contains(got, where) || strings.Contains(got, row) {
		t.Errorf("a project without rules of its own:\n%s", got)
	}

	two := generated(t, generateClaudeMd, godotGoProject())
	many := godotGoProject()
	many.Rules.Custom = []string{"backend", "frontend", "agents", "security", "testing", "docs-source-of-truth", "coding-style", "audit-checklist"}
	got = generated(t, generateClaudeMd, many)
	if !strings.Contains(got, row) || strings.Contains(got, where) {
		t.Errorf("a project with rules of its own:\n%s", got)
	}
	if got != two {
		t.Errorf("CLAUDE.md grew with the number of rules:\n%s", got)
	}
	for _, name := range many.Rules.Custom {
		if strings.Contains(got, name+".md") {
			t.Errorf("CLAUDE.md names the rule file %s.md: the list of rules is in AGENTS.md", name)
		}
	}

	more := godotGoProject()
	more.Rules.Kit = []string{"git-workflow", "security"}
	more.Rules.Custom = nil
	got = generated(t, generateClaudeMd, more)
	if !strings.Contains(got, row) || strings.Contains(got, where) {
		t.Errorf("a second rule of the kit is a rule like the project's own:\n%s", got)
	}
	if strings.Count(got, "git-workflow.md") != 1 {
		t.Errorf("the git rule is named %d times, want once", strings.Count(got, "git-workflow.md"))
	}

	noGit := godotGoProject()
	noGit.Rules.Kit = nil
	if got := generated(t, generateClaudeMd, noGit); strings.Contains(got, "git-workflow") {
		t.Errorf("a project that does not use the kit's git rule is pointed at it:\n%s", got)
	}
}

// A Worker is not a Claude Code session in the project's root: nothing loads
// a rule for it by the files the rule names. AGENTS.md tells it which rule is
// about which files, so that it reads the ones that apply to its ticket and
// not every rule of the project.
func TestGenerateAgentsMd_RulesWithWhatTheyAreAbout(t *testing.T) {
	cfg := godotGoProject()
	cfg.Rules.Custom = []string{"backend", "frontend", "agents"}
	got := generated(t, generateAgentsMd, cfg)

	for _, want := range []string{
		"## MUST-READ (before any work)\n\nThese are the project's rules. A rule applies to the files it names: read the ones that apply to the files your ticket changes, and obey them. A document you write or move is a file too.\n\n",
		"| Rule | Applies to |\n|---|---|\n",
		"| `.claude/rules/_kit/git-workflow.md` | everything (branch and PR conventions; the runner makes the commits, you do not) |\n",
		"| `.claude/rules/backend.md` | `backend/**` |\n",
		"| `.claude/rules/frontend.md` | `frontend/**` |\n",
		"| `.claude/rules/agents.md` | everything |\n",
		"Do not proceed if a rule that applies is missing—stop and report what you cannot find.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("AGENTS.md lacks %q:\n%s", want, section(got, "## MUST-READ"))
		}
	}
	if strings.Contains(got, "Read and obey:") {
		t.Error("AGENTS.md still tells a Worker to read every rule")
	}

	// The kit's rules come first, then the project's, in the order listed.
	rows := section(got, "## MUST-READ")
	order := []string{"_kit/git-workflow.md", "rules/backend.md", "rules/frontend.md", "rules/agents.md"}
	at := -1
	for _, name := range order {
		next := strings.Index(rows, name)
		if next < at {
			t.Errorf("%s is out of the order the configuration lists the rules in:\n%s", name, rows)
		}
		at = next
	}

	// A rule that is listed and not there: the Worker is told so where the
	// rule is named, not left to find out.
	root := t.TempDir()
	writeRuleFile(t, root, layout.RulesDir, "backend", cfg)
	got = generatedIn(t, root, generateAgentsMd, cfg)
	for _, want := range []string{
		"| `.claude/rules/backend.md` | `backend/**` |\n",
		"| `.claude/rules/frontend.md` | the file is missing |\n",
		"| `.claude/rules/agents.md` | the file is missing |\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("AGENTS.md of a project with missing rule files lacks %q:\n%s", want, section(got, "## MUST-READ"))
		}
	}

	// The git rule is written by the generator: what it is about is known
	// without the file, which a first run has yet to write.
	if !strings.Contains(got, "| `.claude/rules/_kit/git-workflow.md` | everything (") {
		t.Errorf("the git rule, before its file is written:\n%s", section(got, "## MUST-READ"))
	}

	// A rule of the kit other than the git rule is read like the project's.
	kit := godotGoProject()
	kit.Rules.Kit = []string{"git-workflow", "backend"}
	kit.Rules.Custom = nil
	if got := generated(t, generateAgentsMd, kit); !strings.Contains(got, "| `.claude/rules/_kit/backend.md` | `backend/**` |\n") {
		t.Errorf("a second rule of the kit:\n%s", section(got, "## MUST-READ"))
	}

	// No rule at all: the table is there and empty, and nothing is named.
	bare := godotGoProject()
	bare.Rules = RulesConfig{}
	if got := generated(t, generateAgentsMd, bare); strings.Contains(section(got, "## MUST-READ"), ".md`") {
		t.Errorf("a project that lists no rule:\n%s", section(got, "## MUST-READ"))
	}
}

// section returns a guide from a heading to the next heading of its level.
func section(text, heading string) string {
	_, rest, found := strings.Cut(text, heading)
	if !found {
		return "(no section " + heading + ")"
	}
	body, _, _ := strings.Cut(rest, "\n## ")
	return heading + body
}

// The git rule file says its rules without shouting, and says what the
// project's branches are.
func TestGenerateGitWorkflowMd_Rules(t *testing.T) {
	got := generated(t, generateGitWorkflowMd, godotGoProject())

	for _, want := range []string{
		"# Git & Workflow Rules\n",
		"Scope: every branch, commit and pull request of this project.",
		"change the configuration, not this file",
		"## 1. Branches",
		"This project keeps its repos as directories of one repository:\n- `backend/`: backend (go)\n- `frontend/`: frontend (csharp)\n",
		"- Integration branch: `develop`. Every pull request targets it",
		"- Release branch: `main`. Nothing targets it except a release: a pull request from `develop`, or the pull request of a ticket whose `awk-ticket` block sets `release: true`.",
		"`feat/ai-issue-<id>`",
		"## 2. Commit messages",
		"- Format: `[type] subject`",
		"`feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `chore`",
		"square brackets with no colon after them, and the subject is lowercase",
		"- Example: `[feat] add login endpoint`",
		"## 3. Pull requests",
		"- A change reaches `develop` or `main` through a pull request, never by a direct push.",
		"`Closes #<issue>`",
		"## 4. Issues",
		"- A ticket is a GitHub issue, and an issue cannot be deleted: one opened by mistake stays for good.",
		"- Issues are created by `awkit plan approve` (the tickets of a plan a person approved) and by `awkit create-task` (one ticket a person asked for). Nothing else creates one: no `gh issue create`, and none on an agent's own initiative.",
		"is not an issue. Keep it in the session, or write it into the pull request.",
		"- When it is not clear that an issue was asked for, it was not: ask the person, and without one to ask, open none.",
		"## 5. Rule files",
		"the code and the log are right, and the rule file is corrected",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the git rule file lacks %q", want)
		}
	}
	for _, shout := range []string{"STRICT", "MANDATORY", "MUST", "CUSTOM", "Submodule"} {
		if strings.Contains(got, shout) {
			t.Errorf("the git rule file says %q", shout)
		}
	}
}

// The commit format is not a setting. The Worker's runner writes its commits
// and the titles of its pull requests as "[type] subject" whatever a project
// would like, so the rule agents are given says that, with the example.
func TestGenerateGitWorkflowMd_TheCommitFormatIsTheWorkflows(t *testing.T) {
	got := generated(t, generateGitWorkflowMd, godotGoProject())
	for _, want := range []string{
		"- Format: `[type] subject`\n",
		"- The type is in square brackets with no colon after them, and the subject is lowercase.",
		"- Example: `[feat] add login endpoint`\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the git rule file lacks %q:\n%s", want, got)
		}
	}
}

// Sections are numbered as they come: the one on submodules is there for a
// project that has them, and the one on rule files follows it.
func TestGenerateGitWorkflowMd_Submodules(t *testing.T) {
	cfg := godotGoProject()
	cfg.Repos = []RepoConfig{{Name: "engine", Path: "engine/", Type: "submodule", Language: "go"}}
	got := generated(t, generateGitWorkflowMd, cfg)
	for _, want := range []string{
		"This project uses submodules:\n- `engine` (go)\n",
		"## 4. Submodules\n\n- The commit a submodule is pinned to is reachable from that submodule's `develop`.\n- A pinned commit changes only when the ticket asks for it.\n",
		"## 5. Issues",
		"## 6. Rule files",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the git rule file of a project with submodules lacks %q:\n%s", want, got)
		}
	}
	for _, twice := range []string{"## 4. Issues", "## 4. Rule files", "## 5. Rule files"} {
		if strings.Contains(got, twice) {
			t.Errorf("the sections are not numbered as they come: %q", twice)
		}
	}

	single := godotGoProject()
	single.Project.Type = "single-repo"
	single.Repos = []RepoConfig{{Name: "root", Path: "./", Type: "root", Language: "go"}}
	if got := generated(t, generateGitWorkflowMd, single); strings.Contains(got, "This project") {
		t.Errorf("a single repository is described as a monorepo:\n%s", got)
	}
}

// A project with one trunk has the same branch to merge into and to release
// from. The guides name it once, and do not describe a release branch that is
// not there.
func TestGeneratedGuides_OneTrunk(t *testing.T) {
	cfg := godotGoProject()
	cfg.Git.IntegrationBranch, cfg.Git.ReleaseBranch = "main", "main"

	rule := generated(t, generateGitWorkflowMd, cfg)
	for _, want := range []string{
		"- Branch: `main`. Every pull request targets it, whichever repo the change is in. The project has no separate release branch.\n",
		"- A change reaches `main` through a pull request, never by a direct push.\n",
	} {
		if !strings.Contains(rule, want) {
			t.Errorf("the git rule file of a project with one trunk lacks %q:\n%s", want, rule)
		}
	}
	for _, wrong := range []string{"Integration branch", "Release branch", "`main` or `main`", "release: true"} {
		if strings.Contains(rule, wrong) {
			t.Errorf("the git rule file of a project with one trunk says %q", wrong)
		}
	}

	guide := generated(t, generateClaudeMd, cfg)
	if !strings.Contains(guide, "- Branch: pull requests are merged into `main`; there is no separate release branch\n") {
		t.Errorf("CLAUDE.md of a project with one trunk:\n%s", guide)
	}
	if strings.Contains(guide, "is the release branch") {
		t.Error("CLAUDE.md of a project with one trunk names a release branch")
	}
}

// captureStdout returns what fn prints.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string)
	go func() {
		data, _ := io.ReadAll(r)
		done <- string(data)
	}()
	fn()
	w.Close()
	os.Stdout = old
	return <-done
}

// projectWith writes a configuration into a new directory and returns it.
func projectWith(t *testing.T, config string) string {
	t.Helper()
	dir := t.TempDir()
	path := layout.Config(dir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

const projectWithAMissingRule = `project: {name: demo, type: single-repo}
repos:
  - {name: root, path: ./, type: root, language: go, verify: {build: "go build ./...", test: "go test ./..."}}
git: {integration_branch: main, release_branch: main}
rules: {kit: [git-workflow], custom: [backend]}
`

// The generator says what it writes, a line a file, with the path as it is
// from the project's root: a person reads these, and a full path of a
// temporary directory three times the width of the terminal is no help.
func TestGenerate_SaysWhatItWrites(t *testing.T) {
	dir := projectWith(t, projectWithAMissingRule)
	var result *Result
	out := captureStdout(t, func() {
		var err error
		if result, err = Generate(Options{StateRoot: dir, CI: CISync}); err != nil {
			t.Errorf("Generate: %v", err)
		}
	})
	for _, want := range []string{
		"[generate] wrote CLAUDE.md\n",
		"[generate] wrote AGENTS.md\n",
		"[generate] wrote .claude/rules/_kit/git-workflow.md\n",
		"[generate] wrote .claude/settings.local.json\n",
		"[generate] wrote .claude/agents/pr-reviewer.md\n",
		"[generate] wrote .github/workflows/ci.yml\n",
		"[generate] WARNING: custom rule 'backend' not found",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the progress lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, dir) || strings.Contains(out, filepath.ToSlash(dir)) {
		t.Errorf("the progress shows the full path of the project:\n%s", out)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "custom rule 'backend' not found") {
		t.Errorf("Warnings = %q, want the one about the rule file that is not there", result.Warnings)
	}
}

// Asked for silence, the generator prints nothing and still returns what it
// wrote and what it found: init and upgrade say it in their own words.
func TestGenerate_Quiet(t *testing.T) {
	dir := projectWith(t, projectWithAMissingRule)
	var result *Result
	out := captureStdout(t, func() {
		var err error
		if result, err = Generate(Options{StateRoot: dir, CI: CISync, Quiet: true}); err != nil {
			t.Errorf("Generate: %v", err)
		}
	})
	if out != "" {
		t.Errorf("a quiet generate printed:\n%s", out)
	}
	if len(result.Warnings) != 1 {
		t.Errorf("Warnings = %q, want the one about the rule file that is not there", result.Warnings)
	}
	wrote := map[string]bool{}
	for _, file := range result.GeneratedFiles {
		rel, err := filepath.Rel(dir, file)
		if err != nil {
			t.Fatal(err)
		}
		wrote[filepath.ToSlash(rel)] = true
	}
	for _, want := range []string{"CLAUDE.md", "AGENTS.md", ".claude/rules/_kit/git-workflow.md", ".claude/settings.local.json", ".github/workflows/ci.yml"} {
		if !wrote[want] {
			t.Errorf("GeneratedFiles lacks %s: %v", want, result.GeneratedFiles)
		}
	}
}
