package worker

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var commitPrefixPattern = regexp.MustCompile(`^\[([a-z]+)\]\s+`)

// BuildCommitMessage normalizes a title into the required [type] subject format.
func BuildCommitMessage(titleLine string) string {
	titleLine = strings.TrimSpace(titleLine)
	if titleLine == "" {
		titleLine = "issue"
	}

	commitType := "chore"
	subject := titleLine

	if match := commitPrefixPattern.FindStringSubmatch(titleLine); len(match) > 1 {
		commitType = match[1]
		subject = strings.TrimSpace(titleLine[len(match[0]):])
	}

	subject = normalizeCommitSubject(subject)
	if subject == "" {
		subject = "issue"
	}

	return fmt.Sprintf("[%s] %s", commitType, subject)
}

func normalizeCommitSubject(value string) string {
	value = strings.ToLower(value)
	var buf bytes.Buffer
	for _, r := range value {
		if isAllowedCommitRune(r) {
			buf.WriteRune(r)
		} else {
			buf.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(buf.String()), " ")
}

func isAllowedCommitRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return true
	case r >= '0' && r <= '9':
		return true
	case r == ' ' || r == '_' || r == '-':
		return true
	default:
		return false
	}
}

// GitFetch runs git fetch with a timeout.
func GitFetch(ctx context.Context, repoPath string, timeout time.Duration) error {
	ctx, cancel := withOptionalTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "fetch", "origin")
	return cmd.Run()
}

// GitPush runs git push with a timeout.
func GitPush(ctx context.Context, repoPath, branch string, timeout time.Duration) error {
	ctx, cancel := withOptionalTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "push", "-u", "origin", branch)
	return cmd.Run()
}

func withOptionalTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}
