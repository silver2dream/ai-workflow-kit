package worker

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

var protectedPaths = []string{
	".ai/scripts/",
	".ai/commands/",
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
		return violations, fmt.Errorf("protected scripts modified without approval")
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
		normalized := normalizePath(file)
		for _, protected := range protectedPaths {
			if strings.HasPrefix(normalized, protected) {
				if whitelist != "" && strings.Contains(whitelist, file) {
					continue
				}
				violations = append(violations, file)
				break
			}
		}
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
