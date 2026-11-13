package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/config"
	"github.com/silver2dream/ai-workflow-kit/internal/install"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
)

// initSummary is what `awkit init` did, in the terms a person thinks in: the
// configuration, what agents read, the workflows, the skeleton. Init says
// this once at its end, where it used to print a line with a full path for
// every file.
type initSummary struct {
	Target string // the project, as the person named it
	Preset string

	ConfigKept  bool     // the configuration was there and was left alone
	PresetNamed bool     // the person named a preset; a kept configuration is then worth a word
	Integration string   // where pull requests go
	Release     string   // where releases are cut from
	Repos       []string // the repos the configuration names
	NoCommands  []string // the repos that have no build or test command yet

	Skills, Subagents, Rules int      // what is under .claude now
	Workflows                []string // the CI workflows written, by file name
	SkeletonDirs             []string // the top directories the skeleton wrote into
	SkeletonFiles            int
	SkeletonKept             int // skeleton files that were there and were left alone

	Warnings []string // what went wrong and did not stop the init
	InRepo   bool     // the project is a git repository
}

// collectInitSummary reads what an install left in targetDir, the project as
// the person named it.
func collectInitSummary(targetDir, preset string, result *install.InstallResult) initSummary {
	s := initSummary{Target: targetDir, Preset: preset}
	if targetDir == "." || targetDir == "./" {
		s.Target = "this directory"
	}
	if result == nil {
		return s
	}
	// The installer names the files it wrote by their full paths.
	if abs, err := filepath.Abs(targetDir); err == nil {
		targetDir = abs
	}
	s.ConfigKept = result.ConfigSkipped

	if cfg, err := config.LoadFile(config.Path(targetDir)); err == nil {
		s.Integration, s.Release = cfg.Git.IntegrationBranch, cfg.Git.ReleaseBranch
		for _, repo := range cfg.Repos {
			s.Repos = append(s.Repos, repo.Name)
			if strings.TrimSpace(repo.Verify.Build) == "" && strings.TrimSpace(repo.Verify.Test) == "" {
				s.NoCommands = append(s.NoCommands, repo.Name)
			}
		}
	}

	s.Skills = countEntries(layout.Skills(targetDir), true)
	s.Subagents = countEntries(layout.Agents(targetDir), false)
	s.Rules = countFiles(layout.Rules(targetDir), ".md")

	for _, file := range result.CIWritten {
		s.Workflows = append(s.Workflows, filepath.Base(file))
	}
	sort.Strings(s.Workflows)

	if sr := result.ScaffoldResult; sr != nil {
		s.SkeletonFiles, s.SkeletonKept = len(sr.Created), len(sr.Skipped)
		dirs := map[string]bool{}
		for _, file := range sr.Created {
			if rel, err := filepath.Rel(targetDir, file); err == nil {
				dirs[strings.SplitN(filepath.ToSlash(rel), "/", 2)[0]+"/"] = true
			}
		}
		for dir := range dirs {
			s.SkeletonDirs = append(s.SkeletonDirs, dir)
		}
		sort.Strings(s.SkeletonDirs)
		for i, file := range sr.Failed {
			s.Warnings = append(s.Warnings, fmt.Sprintf("a file of the skeleton was not written: %s: %v", file, sr.Errors[i]))
		}
	}
	if result.ScaffoldError != nil && (result.ScaffoldResult == nil || len(result.ScaffoldResult.Failed) == 0) {
		s.Warnings = append(s.Warnings, fmt.Sprintf("the skeleton was not written: %v", result.ScaffoldError))
	}
	if result.GenerateError != nil {
		s.Warnings = append(s.Warnings, fmt.Sprintf("CLAUDE.md, AGENTS.md and the CI workflows were not written: %v. Fix %s, then run awkit init again", result.GenerateError, layout.ConfigFile))
	}
	s.Warnings = append(s.Warnings, result.GenerateWarnings...)

	_, err := os.Stat(filepath.Join(targetDir, ".git"))
	s.InRepo = err == nil
	return s
}

// countEntries counts the directories (or the files) directly in dir.
func countEntries(dir string, dirs bool) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, entry := range entries {
		if entry.IsDir() == dirs {
			n++
		}
	}
	return n
}

// countFiles counts the files below dir that end in suffix.
func countFiles(dir, suffix string) int {
	n := 0
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, suffix) {
			n++
		}
		return nil
	})
	return n
}

// counted writes a count with its noun: "1 skill", "3 skills".
func counted(n int, noun string) string {
	return fmt.Sprintf("%d %s%s", n, noun, plural(n))
}

// printInitSummary writes what init did and what the person does next.
func printInitSummary(w io.Writer, s initSummary) {
	if s.ConfigKept {
		fmt.Fprintf(w, "\n%s AWK is set up in %s\n\n", green("✓"), s.Target)
	} else {
		fmt.Fprintf(w, "\n%s AWK is set up in %s (preset %s)\n\n", green("✓"), s.Target, s.Preset)
	}

	row := func(what, about string) { fmt.Fprintf(w, "  %-22s %s\n", what, about) }

	about := "the configuration"
	if s.ConfigKept {
		about = "the configuration that was there, kept as it is"
	}
	if s.Integration != "" {
		if s.Integration == s.Release || s.Release == "" {
			about += fmt.Sprintf(": pull requests go to %s", s.Integration)
		} else {
			about += fmt.Sprintf(": pull requests go to %s, releases are cut from %s", s.Integration, s.Release)
		}
	}
	row(layout.ConfigFile, about)
	row(layout.RosterFile, "who sits on the planning council (awkit plan)")
	row(".claude/", fmt.Sprintf("what agents read: %s, %s, %s", counted(s.Skills, "skill"), counted(s.Subagents, "subagent"), counted(s.Rules, "rule file")))
	row("CLAUDE.md, AGENTS.md", "the guides, generated from the configuration")
	if len(s.Workflows) > 0 {
		row(".github/workflows/", strings.Join(s.Workflows, ", "))
	}
	if s.SkeletonFiles > 0 || s.SkeletonKept > 0 {
		about := fmt.Sprintf("the project skeleton, %s", counted(s.SkeletonFiles, "file"))
		if s.SkeletonKept > 0 {
			about += fmt.Sprintf(" (%d that were there are kept; --force writes over them)", s.SkeletonKept)
		}
		what := strings.Join(s.SkeletonDirs, ", ")
		if what == "" {
			what = "skeleton"
		}
		row(what, about)
	}

	if s.ConfigKept && s.PresetNamed {
		fmt.Fprintf(w, "\n  The preset was not applied: the project has a configuration. To write the preset's over it:\n")
		fmt.Fprintf(w, "    awkit init --preset %s --force-config\n", s.Preset)
	}
	for _, warning := range s.Warnings {
		fmt.Fprintf(w, "\n%s %s\n", yellow("⚠"), warning)
	}

	fmt.Fprintf(w, "\n%s\n", bold("Next"))
	step := 0
	next := func(command, why string) {
		step++
		if why == "" {
			fmt.Fprintf(w, "  %d. %s\n", step, cyan(command))
			return
		}
		fmt.Fprintf(w, "  %d. %s\n     %s\n", step, cyan(command), why)
	}
	if len(s.NoCommands) > 0 {
		next("Edit "+layout.ConfigFile, fmt.Sprintf("set the build and test commands (verify:) of %s: the Worker and the review run them, and they are empty", strings.Join(s.NoCommands, ", ")))
	} else {
		next("Look over "+layout.ConfigFile, "the build and test commands of each repo, and where pull requests go")
	}
	if s.InRepo {
		next(`git add -A && git commit -m "[chore] set up awkit"`, "a run starts from a clean working tree")
	} else {
		next(`git init && git add -A && git commit -m "[chore] set up awkit"`, "the project is not a git repository yet; it also needs a GitHub remote")
	}
	next("awkit doctor", "checks the tools, the remote and the configuration, and says how to fix what is missing")
	next(`awkit plan "<what you want built>"`, "the council writes the documents and proposes the tickets; you approve them")
	next("awkit run", "works through the tickets")
	fmt.Fprintln(w)
}
