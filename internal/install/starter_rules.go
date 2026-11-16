package install

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/silver2dream/ai-workflow-kit/internal/config"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
	"github.com/silver2dream/ai-workflow-kit/internal/repo"
)

// starterRulesFS holds the rules a new project starts with:
//
//	starter-rules/common/<name>.md.tmpl    every project
//	starter-rules/<preset>/<name>.md.tmpl  the rules of that preset: of its
//	                                       stack, or of its one repo
//
// They are templates: what differs from project to project (its repos, where
// its contract is, what its owner has decided) is filled in when the project
// is set up. Unlike the kit's rules they are the project's own from the moment
// they are written: an upgrade does not touch them, and the project is
// expected to tighten them as its architecture settles.
//
//go:embed starter-rules
var starterRulesFS embed.FS

const (
	starterRulesRoot   = "starter-rules"
	commonRulesDir     = "common"
	ruleTemplateSuffix = ".md.tmpl"
)

// Answers are what the owner of a project said when it was set up. They go
// into the project's rules, in the owner's words.
type Answers struct {
	// Contract is where the contract between the project's repos is defined
	// (a directory of .proto files, for instance). Empty when that is not
	// decided yet.
	Contract string
	// Decided is, for a repo by its name, what the owner says is decided about
	// how it is built. A repo that is left out has nothing decided yet.
	Decided map[string]string
}

// RuleRepo is a repo of the project as its rules name it.
type RuleRepo struct {
	Name string
	// Path is the repo's directory with a trailing slash ("backend/"), or
	// "./" for a repo that is the whole repository.
	Path     string
	Language string
	// Decided is what the owner said is decided about how this repo is built.
	Decided string
}

// IsRoot reports whether the repo is the whole repository.
func (r RuleRepo) IsRoot() bool {
	return r.Path == "./"
}

// Docs is the directory of the repo's engineering documents. A repo in a
// directory of its own keeps them there; one that is the whole repository
// keeps them apart from the product documents under docs/.
func (r RuleRepo) Docs() string {
	if r.IsRoot() {
		return "docs/engineering/"
	}
	return r.Path + "docs/"
}

// RuleContext is what a starter rule is written from.
type RuleContext struct {
	Repos []RuleRepo
	// Contract is where the contract between the repos is defined; empty when
	// that is not decided.
	Contract string
	// Date is the day the owner answered, as 2006-01-02.
	Date string
}

// NewRuleContext builds the context of a project's rules from its
// configuration and what its owner answered.
func NewRuleContext(cfg *config.Config, answers Answers, date string) RuleContext {
	ctx := RuleContext{Contract: cleanContract(answers.Contract), Date: date}
	for _, r := range cfg.Repos {
		ctx.Repos = append(ctx.Repos, RuleRepo{
			Name:     r.Name,
			Path:     rulePath(r.Type, r.Path),
			Language: r.Language,
			Decided:  oneLine(answers.Decided[r.Name]),
		})
	}
	return ctx
}

// rulePath writes a repo's directory the way a rule names it.
func rulePath(repoType, dir string) string {
	dir = strings.Trim(strings.ReplaceAll(strings.TrimSpace(dir), "\\", "/"), "/")
	dir = strings.TrimPrefix(dir, "./")
	if repoType == "root" || dir == "" || dir == "." {
		return "./"
	}
	return dir + "/"
}

// cleanContract writes a path the owner typed the way a rule names it: with
// forward slashes and, for a directory, the slash the owner gave it.
func cleanContract(contract string) string {
	contract = strings.ReplaceAll(strings.TrimSpace(contract), "\\", "/")
	return strings.TrimPrefix(contract, "./")
}

// oneLine keeps what the owner typed on one line: it goes into a list item.
func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// CodeFrontmatter is the frontmatter of a rule about code: the directories of
// the project's repos, so that the rule is loaded when a file in one of them
// is worked on. A project whose repo is the whole repository has no such
// directory, and the rule holds everywhere: it has no frontmatter.
func (c RuleContext) CodeFrontmatter() string {
	var b strings.Builder
	for _, r := range c.Repos {
		if r.IsRoot() {
			return ""
		}
		fmt.Fprintf(&b, "  - %q\n", r.Path+"**")
	}
	if b.Len() == 0 {
		return ""
	}
	return "---\npaths:\n" + b.String() + "---\n\n"
}

// Has reports whether a repo of the project is built with a toolchain
// (repo.ToolchainGo, repo.ToolchainDotnet, ...).
func (c RuleContext) Has(toolchain string) bool {
	return len(c.of(toolchain)) > 0
}

func (c RuleContext) of(toolchain string) []RuleRepo {
	var repos []RuleRepo
	for _, r := range c.Repos {
		if repo.ToolchainOf(r.Language) == toolchain {
			repos = append(repos, r)
		}
	}
	return repos
}

// PathsOf names, for a sentence, where the code of a toolchain is.
func (c RuleContext) PathsOf(toolchain string) string {
	var paths []string
	for _, r := range c.of(toolchain) {
		if r.IsRoot() {
			return "this repository"
		}
		paths = append(paths, "`"+r.Path+"`")
	}
	return strings.Join(paths, ", ")
}

// Repo returns the repo of a name, or one with nothing in it when the project
// has none of that name.
func (c RuleContext) Repo(name string) RuleRepo {
	for _, r := range c.Repos {
		if r.Name == name {
			return r
		}
	}
	return RuleRepo{}
}

// RootRepo returns the repo that is the whole repository, or nil.
func (c RuleContext) RootRepo() *RuleRepo {
	for i := range c.Repos {
		if c.Repos[i].IsRoot() {
			return &c.Repos[i]
		}
	}
	return nil
}

// Multi reports whether the project has more than one repo.
func (c RuleContext) Multi() bool {
	return len(c.Repos) > 1
}

// ArchitectureDocs names the architecture document of every repo.
func (c RuleContext) ArchitectureDocs() string {
	docs := make([]string, len(c.Repos))
	for i, r := range c.Repos {
		docs[i] = "`" + r.Docs() + "architecture.md`"
	}
	return strings.Join(docs, ", ")
}

// StarterRules lists the names of the rules the projects of a preset start
// with, sorted: the ones every project gets and the ones of the preset's
// stack.
func StarterRules(preset string) []string {
	seen := map[string]bool{}
	var names []string
	for _, dir := range []string{commonRulesDir, presetName(preset)} {
		entries, err := fs.ReadDir(starterRulesFS, path.Join(starterRulesRoot, dir))
		if err != nil {
			continue
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ruleTemplateSuffix) {
				continue
			}
			name = strings.TrimSuffix(name, ruleTemplateSuffix)
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	sort.Strings(names)
	return names
}

// starterRuleTemplate returns the template of one starter rule. A preset's
// own rule of a name stands in for the common one.
func starterRuleTemplate(preset, name string) ([]byte, error) {
	for _, dir := range []string{presetName(preset), commonRulesDir} {
		data, err := starterRulesFS.ReadFile(path.Join(starterRulesRoot, dir, name+ruleTemplateSuffix))
		if err == nil {
			return data, nil
		}
	}
	return nil, fmt.Errorf("the preset %s has no starter rule %q", presetName(preset), name)
}

// RenderStarterRule writes one starter rule of a preset for a project.
func RenderStarterRule(preset, name string, ctx RuleContext) ([]byte, error) {
	text, err := starterRuleTemplate(preset, name)
	if err != nil {
		return nil, err
	}
	tmpl, err := template.New(name).Option("missingkey=error").Parse(string(text))
	if err != nil {
		return nil, fmt.Errorf("the starter rule %s: %w", name, err)
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, ctx); err != nil {
		return nil, fmt.Errorf("the starter rule %s: %w", name, err)
	}
	return out.Bytes(), nil
}

// ensureStarterRules writes the starter rules of a preset into a project. A
// rule file that is there is the project's own and is kept.
func ensureStarterRules(targetDir, preset string, ctx RuleContext) error {
	for _, name := range StarterRules(preset) {
		data, err := RenderStarterRule(preset, name, ctx)
		if err != nil {
			return err
		}
		file := filepath.Join(layout.Rules(targetDir), name+".md")
		if _, _, err := writeFileIfNotExists(file, data, false); err != nil {
			return err
		}
	}
	return nil
}

// HasOwnRules reports whether a project has rule files of its own: a rule
// anywhere under its rules directory, the kit's folder apart. Claude Code
// reads the rule files in subdirectories too, so a rule in a folder counts.
// Such a project is not written a rule over, so what its owner would answer
// about it has nowhere to go but those files.
func HasOwnRules(targetDir string) bool {
	found := false
	kit := layout.KitRules(targetDir)
	_ = filepath.WalkDir(layout.Rules(targetDir), func(file string, entry fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return nil // a directory that is not there, or cannot be read, holds no rule
		case entry.IsDir() && file == kit:
			return filepath.SkipDir
		case !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md"):
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}
