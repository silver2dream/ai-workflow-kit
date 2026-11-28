package worker

import (
	"context"
	"fmt"
	"regexp"
	"runtime"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/gitlock"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
	"github.com/silver2dream/ai-workflow-kit/internal/ticket"
)

// protectedPaths and protectedFiles are the workflow's own control files
// (layout.ProtectedDirs, layout.ProtectedFiles). A Worker that edits them
// changes the rules it is checked against, so the runner rejects such a
// change unless the ticket allows it (`allow_script_changes: true` in its
// awk-ticket block).
var (
	protectedPaths = layout.ProtectedDirs
	protectedFiles = layout.ProtectedFiles
)

// protectedPathList renders the protected set for the Worker prompt, so the
// prompt names exactly what the runner enforces.
func protectedPathList() string {
	all := append(append([]string{}, protectedPaths...), protectedFiles...)
	return strings.Join(all, ", ")
}

// isProtectedPath reports whether a repository-relative path is one of the
// workflow's control files. Case does not count: on a file system that
// ignores it, .Claude/settings.json is .claude/settings.json.
func isProtectedPath(normalized string) bool {
	name := strings.ToLower(normalized)
	for _, prefix := range protectedPaths {
		if strings.HasPrefix(name, strings.ToLower(prefix)) {
			return true
		}
	}
	for _, file := range protectedFiles {
		if name == strings.ToLower(normalizePath(file)) {
			return true
		}
	}
	return false
}

var defaultSecretPatterns = []string{
	`(?i)password\s*[:=]\s*["'][^"']+`,
	`(?i)api[_-]?key\s*[:=]\s*["'][^"']+`,
	`(?i)secret[_-]?key\s*[:=]\s*["'][^"']+`,
	`(?i)access[_-]?token\s*[:=]\s*["'][^"']+`,
	`(?i)private[_-]?key\s*[:=]`,
	`(?i)AWS_SECRET_ACCESS_KEY`,
	`(?i)GITHUB_TOKEN`,
	`(?i)BEGIN\s+(RSA|DSA|EC|OPENSSH)\s+PRIVATE\s+KEY`,
}

// CheckScriptModifications checks for changes in protected paths.
func CheckScriptModifications(ctx context.Context, wtDir string, allowChanges bool, whitelist string) ([]string, error) {
	output, err := gitOutput(ctx, wtDir, "diff", "--cached", "--name-only")
	if err != nil {
		return nil, err
	}

	files := splitLines(output)
	violations := findProtectedChanges(files, whitelist)
	if len(violations) > 0 && !allowChanges {
		return violations, fmt.Errorf("protected workflow files modified without approval: %s (to allow it, set 'allow_script_changes: true' in the ticket's '%s' block)", strings.Join(violations, ", "), ticket.Info)
	}

	return violations, nil
}

// CheckSensitiveInfo checks staged diff for sensitive patterns.
func CheckSensitiveInfo(ctx context.Context, wtDir string, allowSecrets bool, customPatterns []string) ([]string, error) {
	diff, err := gitOutput(ctx, wtDir, "diff", "--cached")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(diff) == "" {
		return nil, nil
	}

	matches := findSensitiveMatches(diff, customPatterns)
	if len(matches) > 0 && !allowSecrets {
		return matches, fmt.Errorf("sensitive patterns detected in staged changes")
	}

	return matches, nil
}

// PolicyError is a change the diff policy refuses: AWK's own files changed,
// or a secret, which the ticket does not allow.
type PolicyError struct {
	// Protected are the workflow's files changed; Secrets the patterns of
	// secrets found.
	Protected []string
	Secrets   []string
}

func (e *PolicyError) Error() string {
	var said []string
	if len(e.Protected) > 0 {
		said = append(said, fmt.Sprintf("protected workflow files modified without approval: %s (to allow it, set 'allow_script_changes: true' in the ticket's '%s' block)", strings.Join(e.Protected, ", "), ticket.Info))
	}
	if len(e.Secrets) > 0 {
		said = append(said, fmt.Sprintf("sensitive patterns detected: %s (to allow it, set 'allow_secrets: true' in the ticket's '%s' block)", strings.Join(e.Secrets, ", "), ticket.Info))
	}
	return strings.Join(said, "; ")
}

// ScopeError is a change that touches files its ticket's paths do not name.
type ScopeError struct {
	Outside []string
	Paths   []string
}

func (e *ScopeError) Error() string {
	return fmt.Sprintf("the change touches files outside the ticket's paths: %s (the ticket's paths: %s); change only what they name, or ask for the paths to be widened",
		strings.Join(e.Outside, ", "), strings.Join(e.Paths, ", "))
}

// CheckScope holds a staged change to its ticket's paths (design.md §9.4):
// each file it touches, deleted ones too, is to be matched by one of them,
// without case (as the scheduler compares them). A *ScopeError names the
// files that are not; another error, one git could not show.
func CheckScope(ctx context.Context, wtDir string, paths []string) error {
	names, err := gitOutput(ctx, wtDir, "diff", "--cached", "--name-only", "--no-renames")
	if err != nil {
		return err
	}
	var outside []string
	for _, file := range splitLines(names) {
		if !inScope(file, paths) {
			outside = append(outside, file)
		}
	}
	if len(outside) > 0 {
		return &ScopeError{Outside: outside, Paths: paths}
	}
	return nil
}

// holdToScope holds a Worker's staged change to its ticket's paths, when it
// has any: a submodule's change is in its own repository, and is not held
// to them here.
func holdToScope(ctx context.Context, wtDir, repoType string, paths []string) error {
	if repoType == "submodule" || len(paths) == 0 {
		return nil
	}
	return CheckScope(ctx, wtDir, paths)
}

func inScope(file string, paths []string) bool {
	lower := strings.ToLower(strings.ReplaceAll(file, "\\", "/"))
	for _, p := range paths {
		if ticket.Overlap(strings.ToLower(p), lower) {
			return true
		}
	}
	return false
}

// CheckCommits holds the change of revs, in git's notation (for instance
// "origin/develop...HEAD"), to the rules a Worker's staged change is held to
// before it is handed in (CheckScriptModifications, CheckSensitiveInfo),
// with what the ticket's metadata allows: the engine's merge queue checks a
// change again once it rebased it. A *PolicyError is a change the rules
// refuse; another error, one git could not show.
func CheckCommits(ctx context.Context, wtDir, revs string, meta *TicketMetadata) error {
	if meta == nil {
		meta = &TicketMetadata{}
	}
	names, err := gitOutput(ctx, wtDir, "diff", "--name-only", revs)
	if err != nil {
		return err
	}
	diff, err := gitOutput(ctx, wtDir, "diff", revs)
	if err != nil {
		return err
	}
	refused := &PolicyError{}
	if protected := findProtectedChanges(splitLines(names), meta.ScriptWhitelist); !meta.AllowScriptChanges {
		refused.Protected = protected
	}
	if strings.TrimSpace(diff) != "" && !meta.AllowSecrets {
		refused.Secrets = findSensitiveMatches(diff, meta.SecretPatterns)
	}
	if len(refused.Protected)+len(refused.Secrets) > 0 {
		return refused
	}
	return nil
}

func findProtectedChanges(files []string, whitelist string) []string {
	var violations []string
	for _, file := range files {
		if file == "" {
			continue
		}
		if !isProtectedPath(normalizePath(file)) {
			continue
		}
		if whitelist != "" && strings.Contains(whitelist, file) {
			continue
		}
		violations = append(violations, file)
	}
	return violations
}

func findSensitiveMatches(diff string, customPatterns []string) []string {
	var matches []string
	for _, pattern := range defaultSecretPatterns {
		if re, err := regexp.Compile(pattern); err == nil && re.MatchString(diff) {
			matches = append(matches, pattern)
		}
	}

	for _, raw := range customPatterns {
		if raw == "" {
			continue
		}
		re, err := regexp.Compile(raw)
		if err != nil {
			continue
		}
		if re.MatchString(diff) {
			matches = append(matches, raw)
		}
	}

	return matches
}

func normalizePath(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	path = strings.TrimSuffix(path, "/")
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return path
}

func splitLines(value string) []string {
	raw := strings.Split(value, "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// gitOutput is what git printed on its standard output, run in dir with the
// repository's lock. What it says on its standard error (a warning about line
// ends, say) is not part of what is read from it.
func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := gitlock.Output(ctx, dir, 0, args...)
	if err != nil {
		return "", err
	}
	return out, nil
}
