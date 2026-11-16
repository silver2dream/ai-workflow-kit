package install

import (
	"bufio"
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/config"
	"github.com/silver2dream/ai-workflow-kit/internal/council"
	"github.com/silver2dream/ai-workflow-kit/internal/generate"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
)

//go:embed deprecated.txt
var deprecatedFiles string

type Options struct {
	Preset      string
	ProjectName string
	Force       bool
	ForceConfig bool // only the configuration is written over
	SkipConfig  bool // the configuration is left alone (an upgrade)
	// Branches are the branches a new configuration names (DetectBranches).
	// Without them it names a single trunk, main.
	Branches   Branches
	NoGenerate bool // Skip the files derived from the config, CI workflows included
	// CI is what to do about the CI workflows: init writes the missing ones
	// (generate.CISync), upgrade only refreshes the ones awkit owns
	// (generate.CIRefresh). The zero value leaves them alone.
	CI       generate.CIPolicy
	Scaffold bool // Execute scaffold after install
	DryRun   bool // For scaffold dry-run
}

// InstallResult contains information about what was done
type InstallResult struct {
	ConfigSkipped  bool
	ConfigPath     string
	ScaffoldResult *ScaffoldResult
	ScaffoldError  error
	// GenerateError is why the files derived from the config (CLAUDE.md,
	// AGENTS.md, the CI workflows) were not written. The kit itself is
	// installed all the same.
	GenerateError error
	// CIWritten and CIRemoved are the CI workflows the install wrote and
	// removed.
	CIWritten []string
	CIRemoved []string
	// GenerateWarnings are what the generator found and went on from, for
	// the caller to show: the install generates without a word.
	GenerateWarnings []string
}

// ScaffoldOptions configures scaffold behavior
type ScaffoldOptions struct {
	Preset      string
	TargetDir   string
	ProjectName string
	Force       bool
	DryRun      bool
}

// ScaffoldResult contains scaffold operation results
type ScaffoldResult struct {
	Created []string
	Skipped []string
	Failed  []string
	Errors  []error
}

// Error types for scaffold operations
var (
	ErrUnknownPreset      = errors.New("unknown preset")
	ErrScaffoldConflict   = errors.New("file already exists")
	ErrScaffoldFailed     = errors.New("scaffold failed")
	ErrNoScaffold         = errors.New("preset has no project skeleton")
	ErrMissingPreset      = errors.New("--preset required for upgrade --scaffold")
	ErrInvalidProjectName = errors.New("invalid project name: contains special characters")
)

func Install(kit fs.FS, targetDir string, opts Options) (*InstallResult, error) {
	result := &InstallResult{}

	// Resolve "." or "./" to absolute path
	if targetDir == "." || targetDir == "./" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("cannot resolve current directory: %w", err)
		}
		targetDir = cwd
	}
	targetDir = filepath.Clean(targetDir)

	if st, err := os.Stat(targetDir); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("target directory does not exist or is not a directory: %s", targetDir)
	}

	if opts.ProjectName == "" {
		opts.ProjectName = filepath.Base(targetDir)
	}

	// The configuration of a new project comes from its preset. An upgrade
	// leaves the one that is there alone.
	configSkipped, err := applyPreset(targetDir, opts)
	if err != nil {
		return nil, err
	}
	result.ConfigSkipped = configSkipped
	result.ConfigPath = config.Path(targetDir)

	// A new project gets the roster of its preset: who sits at the table when
	// `awkit plan` turns a brief into documents and tickets. An upgrade writes
	// none: `awkit plan` does, from the project's repos, the first time it runs.
	if !opts.SkipConfig {
		if err := ensureRoster(targetDir, opts.Preset); err != nil {
			return nil, err
		}
	}

	// The rules for the preset's stack go with its configuration, which lists
	// them. A project that kept the configuration it had keeps its rules too.
	if !configSkipped {
		if err := ensureStarterRules(targetDir, opts.Preset); err != nil {
			return nil, err
		}
	}

	// The kit: rules, skills and subagents, at the paths they have in a
	// project. Only the kit's own files are written: what else the project
	// keeps under .claude is its own and is not touched.
	if err := copyDir(kit, KitRoot, filepath.Join(targetDir, filepath.FromSlash(KitRoot)), opts.Force); err != nil {
		return nil, err
	}

	// Files earlier versions of the kit installed and this one does not.
	CleanupDeprecatedFiles(targetDir)

	if err := ensureRuntimeDirs(targetDir); err != nil {
		return nil, err
	}

	if err := ensureGitIgnore(targetDir); err != nil {
		return nil, err
	}

	if err := ensureGitAttributes(targetDir); err != nil {
		return nil, err
	}

	if err := migrateExistingCIWorkflow(targetDir); err != nil {
		return nil, err
	}

	// Scaffold integration point (after all kit files are installed)
	if opts.Scaffold {
		scaffoldResult, err := Scaffold(targetDir, ScaffoldOptions{
			Preset:      opts.Preset,
			TargetDir:   targetDir,
			ProjectName: opts.ProjectName,
			Force:       opts.Force,
			DryRun:      false,
		})
		if err != nil {
			// Scaffold failure doesn't affect already installed AWK kit
			result.ScaffoldResult = scaffoldResult
			result.ScaffoldError = err
		} else {
			result.ScaffoldResult = scaffoldResult
		}
	}

	if !opts.NoGenerate {
		generated, err := generate.Generate(generate.Options{StateRoot: targetDir, CI: opts.CI, Quiet: true})
		result.GenerateError = err
		if generated != nil {
			result.GenerateWarnings = generated.Warnings
			workflows := filepath.Join(".github", "workflows") + string(filepath.Separator)
			for _, file := range generated.GeneratedFiles {
				if strings.Contains(file, workflows) {
					result.CIWritten = append(result.CIWritten, file)
				}
			}
			result.CIRemoved = generated.RemovedFiles
		}
	}

	return result, nil
}

// KitRoot is the directory of the kit, in the kit and in a project: where
// Claude Code looks for rules, skills and subagents.
const KitRoot = ".claude"

func copyDir(src fs.FS, srcDir, dstDir string, force bool) error {
	return fs.WalkDir(src, srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dstDir, rel)

		if d.IsDir() {
			return os.MkdirAll(dstPath, 0o755)
		}

		if !force {
			if _, err := os.Stat(dstPath); err == nil {
				return nil
			}
		}

		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return err
		}

		r, err := src.Open(path)
		if err != nil {
			return err
		}
		defer r.Close()

		data, err := io.ReadAll(r)
		if err != nil {
			return err
		}

		if strings.HasSuffix(path, ".sh") || strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".yaml") {
			data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
		}

		if err := os.WriteFile(dstPath, data, 0o644); err != nil {
			return err
		}

		if strings.HasSuffix(path, ".sh") {
			_ = os.Chmod(dstPath, 0o755)
		}

		return nil
	})
}

// ensureRoster writes the roster of a preset into a project that has none.
// A roster that is there is the project's own and is never replaced.
func ensureRoster(targetDir, preset string) error {
	file := layout.Roster(targetDir)
	if _, err := os.Stat(file); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	return os.WriteFile(file, council.DefaultRoster(preset), 0o644)
}

// ensureRuntimeDirs creates the directories a run writes to: the state
// directory and what is kept in it, and the one of the Workers' worktrees.
// git ignores them, so they hold no placeholder files: a clone gets them the
// first time awkit needs them.
func ensureRuntimeDirs(targetDir string) error {
	dirs := []string{
		layout.State(targetDir),
		layout.State(targetDir, "traces"),
		layout.Results(targetDir),
		layout.Runs(targetDir),
		layout.Logs(targetDir),
		layout.Temp(targetDir),
		filepath.Join(targetDir, ".worktrees"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func ensureGitIgnore(targetDir string) error {
	const markerStart = "# >>> AI Workflow Kit >>>"
	const markerEnd = "# <<< AI Workflow Kit <<<"

	snippet := strings.Join([]string{
		markerStart,
		"# What a run leaves behind: state, results, logs (do not commit)",
		layout.StateDir + "/",
		".worktrees/",
		"# Claude Code local settings (do not commit)",
		".claude/settings.local.json",
		"# Common cache files (prevent audit P1 findings)",
		"__pycache__/",
		"*.pyc",
		"*.pyo",
		".pytest_cache/",
		"node_modules/",
		".npm/",
		".yarn/",
		"*.log",
		markerEnd,
	}, "\n") + "\n"

	path := filepath.Join(targetDir, ".gitignore")
	existing, _ := os.ReadFile(path)

	// If AWK section exists, replace it (to support upgrades with new entries)
	if bytes.Contains(existing, []byte(markerStart)) {
		startIdx := bytes.Index(existing, []byte(markerStart))
		endIdx := bytes.Index(existing, []byte(markerEnd))
		if endIdx > startIdx {
			// Remove old section and replace with new
			endIdx += len(markerEnd)
			// Skip trailing newline if present
			if endIdx < len(existing) && existing[endIdx] == '\n' {
				endIdx++
			}
			var out []byte
			out = append(out, existing[:startIdx]...)
			out = append(out, []byte(snippet)...)
			out = append(out, existing[endIdx:]...)
			return os.WriteFile(path, out, 0o644)
		}
		// Malformed section (start without end), append new section
	}

	var out []byte
	if len(existing) > 0 && !bytes.HasSuffix(existing, []byte("\n")) {
		out = append(out, existing...)
		out = append(out, '\n')
	} else {
		out = append(out, existing...)
	}
	out = append(out, '\n')
	out = append(out, []byte(snippet)...)
	return os.WriteFile(path, out, 0o644)
}

func ensureGitAttributes(targetDir string) error {
	path := filepath.Join(targetDir, ".gitattributes")
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	content := strings.Join([]string{
		"# Normalize text files and prevent CRLF breakage in CI",
		"* text=auto",
		"",
		"*.sh text eol=lf",
		"*.yml text eol=lf",
		"*.yaml text eol=lf",
		"*.md text eol=lf",
		"*.json text eol=lf",
		"*.ts text eol=lf",
		"*.tsx text eol=lf",
		"",
	}, "\n")
	return os.WriteFile(path, []byte(content), 0o644)
}

func ensureWorkflowConfig(targetDir, projectName string, forceOverwrite bool, preset func(projectName string) []byte) (skipped bool, err error) {
	path := config.Path(targetDir)
	if _, err := os.Stat(path); err == nil {
		if !forceOverwrite {
			return true, nil // File exists, skip
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return false, os.WriteFile(path, preset(projectName), 0o644)
}

// migrateCIWorkflow checks for and removes deprecated AWK CI jobs
// Returns (migrated bool, newContent []byte)
func migrateCIWorkflow(content []byte) (bool, []byte) {
	lines := strings.Split(string(content), "\n")
	var result []string
	migrated := false
	inAwkJob := false
	awkJobIndent := 0

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// Detect start of awk job (old deprecated job)
		if strings.TrimSpace(line) == "awk:" && strings.HasSuffix(strings.TrimRight(line, " \t"), ":") {
			// Found awk: job definition
			awkJobIndent = len(line) - len(strings.TrimLeft(line, " \t"))
			inAwkJob = true
			migrated = true
			continue
		}

		// If we're in awk job, skip until we hit a line with same or less indentation
		if inAwkJob {
			currentIndent := len(line) - len(strings.TrimLeft(line, " \t"))
			// Empty lines are part of the job
			if strings.TrimSpace(line) == "" {
				continue
			}
			// If indentation is greater, still in awk job
			if currentIndent > awkJobIndent {
				continue
			}
			// We've exited the awk job
			inAwkJob = false
		}

		result = append(result, line)
	}

	if migrated {
		// Clean up any double blank lines that might result
		return true, []byte(strings.Join(result, "\n"))
	}
	return false, content
}

// migrateExistingCIWorkflow takes the long-removed awk job out of a ci.yml
// that an old version of the kit wrote. A project without that file, or
// without that job, is left alone.
func migrateExistingCIWorkflow(targetDir string) error {
	path := filepath.Join(targetDir, ".github", "workflows", "ci.yml")
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if migrated, migratedContent := migrateCIWorkflow(content); migrated {
		return os.WriteFile(path, migratedContent, 0o644)
	}
	return nil
}

// validateProjectName checks for invalid characters in project name
func validateProjectName(name string) error {
	if strings.ContainsAny(name, `/\$"';|&><`) || strings.Contains(name, "`") {
		return ErrInvalidProjectName
	}
	return nil
}

// writeFileIfNotExists writes a file, skipping if exists unless force is true
// Returns (created, skipped, err)
func writeFileIfNotExists(path string, content []byte, force bool) (created bool, skipped bool, err error) {
	path = filepath.Clean(path)
	if _, err := os.Stat(path); err == nil {
		if !force {
			return false, true, nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, false, err
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return false, false, err
	}
	return true, false, nil
}

// CleanupDeprecatedFiles removes the files earlier versions of the kit
// installed and this one does not (deprecated.txt).
func CleanupDeprecatedFiles(targetDir string) {
	scanner := bufio.NewScanner(strings.NewReader(deprecatedFiles))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		path := filepath.Join(targetDir, filepath.FromSlash(line))
		if _, err := os.Stat(path); err == nil {
			_ = os.Remove(path)
		}
	}
}

func scaffoldFiles(files []struct {
	path    string
	content []byte
}, force, dryRun bool, result *ScaffoldResult) (*ScaffoldResult, error) {
	for _, f := range files {
		if dryRun {
			// Check if file exists for dry-run reporting
			if _, err := os.Stat(f.path); err == nil {
				if !force {
					result.Skipped = append(result.Skipped, f.path)
				} else {
					result.Created = append(result.Created, f.path)
				}
			} else {
				result.Created = append(result.Created, f.path)
			}
			continue
		}

		created, skipped, err := writeFileIfNotExists(f.path, f.content, force)
		if err != nil {
			result.Failed = append(result.Failed, f.path)
			result.Errors = append(result.Errors, err)
			continue
		}
		if created {
			result.Created = append(result.Created, f.path)
		} else if skipped {
			result.Skipped = append(result.Skipped, f.path)
		}
	}

	if len(result.Failed) > 0 {
		return result, ErrScaffoldFailed
	}
	return result, nil
}
