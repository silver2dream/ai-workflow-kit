package worker

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/ticket"
)

// protectedPaths are the workflow's own control files: the configuration,
// rules, skills and agent definitions that decide how work is dispatched and
// reviewed. A Worker that edits them changes the rules it is checked against,
// so the runner rejects such a change unless the ticket allows it
// (`allow_script_changes: true` in its awk-ticket block).
var protectedPaths = []string{
	".ai/config/",
	".ai/rules/",
	".ai/skills/",
	".claude/agents/",
}

// protectedFiles are single files at the repository root with the same standing.
var protectedFiles = []string{
	"CLAUDE.md",
	"AGENTS.md",
}

// protectedPathList renders the protected set for the Worker prompt, so the
// prompt names exactly what the runner enforces.
func protectedPathList() string {
	all := append(append([]string{}, protectedPaths...), protectedFiles...)
	return strings.Join(all, ", ")
}

// isProtectedPath reports whether a normalized, repository-relative path is
// one of the workflow's control files.
func isProtectedPath(normalized string) bool {
	for _, prefix := range protectedPaths {
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}
	for _, file := range protectedFiles {
		if normalized == normalizePath(file) {
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

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s failed: %w", strings.Join(args, " "), err)
	}
	return string(output), nil
}
