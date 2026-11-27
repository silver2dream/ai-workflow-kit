package worker

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/silver2dream/ai-workflow-kit/internal/gitlock"
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

// isAllowedCommitRune says whether a rune is kept in a commit's subject: a
// lower-case letter, a mark or a digit of any script, a space, an underscore
// or a hyphen. Punctuation and symbols become spaces. Only ASCII used to be
// kept, and a ticket titled in Chinese lost every word of its title.
func isAllowedCommitRune(r rune) bool {
	switch {
	case r == ' ' || r == '_' || r == '-':
		return true
	case unicode.IsUpper(r):
		return false
	default:
		return unicode.IsLetter(r) || unicode.IsMark(r) || unicode.IsDigit(r)
	}
}

// GitFetch runs git fetch with a timeout, and the repository's lock.
func GitFetch(ctx context.Context, repoPath string, timeout time.Duration) error {
	return gitlock.Run(ctx, repoPath, timeout, "fetch", "origin")
}

// GitPush runs git push with a timeout, and the repository's lock.
func GitPush(ctx context.Context, repoPath, branch string, timeout time.Duration) error {
	return gitlock.Run(ctx, repoPath, timeout, "push", "-u", "origin", branch)
}

func withOptionalTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}
