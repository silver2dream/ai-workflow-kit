package worker

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/ticket"
)

// TicketMetadata is what the workflow needs to know about a ticket to run a
// Worker on it.
type TicketMetadata struct {
	Repo               string // the repo of the workflow config the ticket is for
	Severity           string // P0 to P3, or empty
	Release            bool   // the pull request goes to the release branch
	AllowParentChanges bool   // changes outside the submodule are allowed
	AllowScriptChanges bool   // changes to the workflow's own files (protectedPaths) are allowed
	AllowSecrets       bool   // secrets in the diff are allowed

	// Legacy is set for a ticket without an awk-ticket block: one written
	// before the block existed, read from its prose the way it used to be.
	Legacy bool
	// What only a legacy ticket can say.
	ScriptWhitelist string   // protected files the ticket may change
	SecretPatterns  []string // more patterns that count as secrets
}

// dispatchNotesMark separates, in a ticket file, the ticket as it stands on
// GitHub from what the dispatch adds for the Worker: what the last review
// found, how to deal with a merge conflict. Where the Worker runs, what it may
// touch and which commands verify its work is for the ticket alone to say;
// text that came out of a review must not be able to.
const dispatchNotesMark = "<!-- awk:dispatch-notes -->"

// A ticket that has the mark in its own text gets this in its place, so that
// the first mark of a ticket file is always the dispatch's.
const quotedNotesMark = "<!-- awk:dispatch-notes (written in the ticket) -->"

// ticketFileContent puts a ticket file together: the issue body, then what the
// dispatch has to add (see dispatchNotes).
func ticketFileContent(issueBody, reviewReason, mergeIssue, baseBranch string) string {
	content := strings.ReplaceAll(issueBody, dispatchNotesMark, quotedNotesMark)
	notes := dispatchNotes(reviewReason, mergeIssue, baseBranch)
	if notes == "" {
		return content
	}
	return content + "\n\n" + dispatchNotesMark + notes
}

// dispatchNotes words what the dispatch tells the Worker on top of the ticket:
// why the last review was blocked, and what to do about a pull request that no
// longer merges.
func dispatchNotes(reviewReason, mergeIssue, baseBranch string) string {
	notes := ""
	if reviewReason != "" {
		notes += fmt.Sprintf(`

---

## ⚠️ 上次 Review 被阻擋 - 請修復以下問題

%s

**請根據以上錯誤修復代碼，確保所有測試通過後再提交。**
`, reviewReason)
	}
	switch mergeIssue {
	case "conflict":
		notes += fmt.Sprintf(`

---

## ⚠️ MERGE CONFLICT - 請先解決

PR 有 merge conflict，請執行以下步驟：

1. 在 worktree 中執行 `+"`git fetch origin && git rebase origin/%s`"+`
2. 解決衝突後 `+"`git add . && git rebase --continue`"+`
3. 推送更新 `+"`git push --force-with-lease`"+`
4. 確認 CI 通過

**這是最優先的任務，必須先完成才能繼續其他工作。**`, baseBranch)
	case "rebase":
		notes += fmt.Sprintf(`

---

## ⚠️ BRANCH BEHIND - 請先 Rebase

PR 分支落後 base branch，請執行以下步驟：

1. 在 worktree 中執行 `+"`git fetch origin && git rebase origin/%s`"+`
2. 推送更新 `+"`git push --force-with-lease`"+`
3. 確認 CI 通過

**這是最優先的任務，必須先完成才能繼續其他工作。**`, baseBranch)
	}
	return notes
}

// ticketOf returns the ticket itself from the content of a ticket file:
// what comes before the dispatch's notes.
func ticketOf(content string) string {
	if i := strings.Index(content, dispatchNotesMark); i >= 0 {
		return content[:i]
	}
	return content
}

// ReadTicketMetadata returns what the workflow needs to know about a ticket,
// from its body: the issue body, or the ticket of a ticket file (see ticketOf).
//
// It reads the ticket's awk-ticket block and nothing else: no prose can set a
// flag. A block that cannot be used is an error, and the prose is not read in
// its place. Only a ticket without a block is read from its prose; Legacy is
// then set.
func ReadTicketMetadata(body string) (*TicketMetadata, error) {
	block, found, err := ticket.Parse(body)
	if err != nil {
		return nil, err
	}
	if !found {
		return readLegacyTicket(body), nil
	}
	return &TicketMetadata{
		Repo:               block.Repo,
		Severity:           block.Severity,
		Release:            block.Release,
		AllowParentChanges: block.AllowParentChanges,
		AllowScriptChanges: block.AllowScriptChanges,
		AllowSecrets:       block.AllowSecrets,
	}, nil
}

// readLegacyTicket reads a ticket that has no awk-ticket block from its prose:
// "**Repo**: backend" lines, flags under "## Constraints", "- key: value"
// lines. This is how every ticket was read before the block existed.
func readLegacyTicket(body string) *TicketMetadata {
	meta := &TicketMetadata{
		Repo:   ticket.RootRepo, // the repo of a ticket that names none
		Legacy: true,
	}

	// Parse Repo: **Repo**: backend or - Repo: backend
	repoPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?im)\*\*Repo\*\*\s*:\s*([^\s\r\n]+)`),
		regexp.MustCompile(`(?im)^[-*]\s*Repo\s*:\s*([^\s\r\n]+)`),
	}
	for _, pattern := range repoPatterns {
		if matches := pattern.FindStringSubmatch(body); len(matches) > 1 {
			meta.Repo = strings.TrimSpace(matches[1])
			break
		}
	}

	// Parse Severity: **Severity**: P0 or - Severity: P0
	severityPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?im)\*\*Severity\*\*\s*:\s*(P[0-3])`),
		regexp.MustCompile(`(?im)^[-*]\s*Severity\s*:\s*(P[0-3])`),
	}
	for _, pattern := range severityPatterns {
		if matches := pattern.FindStringSubmatch(body); len(matches) > 1 {
			meta.Severity = strings.TrimSpace(matches[1])
			break
		}
	}

	// Parse Release: **Release**: true/false
	releasePatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?im)\*\*Release\*\*\s*:\s*(true|false)`),
		regexp.MustCompile(`(?im)^[-*]\s*Release\s*:\s*(true|false)`),
	}
	for _, pattern := range releasePatterns {
		if matches := pattern.FindStringSubmatch(body); len(matches) > 1 {
			meta.Release = strings.ToLower(matches[1]) == "true"
			break
		}
	}

	// Parse special flags in Constraints section
	// Look for: allow-parent-changes, allow-script-changes, allow-secrets
	constraintsPattern := regexp.MustCompile(`(?is)##\s*Constraints\s*\n(.*?)(?:\n##|\z)`)
	if matches := constraintsPattern.FindStringSubmatch(body); len(matches) > 1 {
		constraints := matches[1]
		meta.AllowParentChanges = strings.Contains(strings.ToLower(constraints), "allow-parent-changes")
		meta.AllowScriptChanges = strings.Contains(strings.ToLower(constraints), "allow-script-changes")
		meta.AllowSecrets = strings.Contains(strings.ToLower(constraints), "allow-secrets")
	}

	// A "- key: value" line says the same; for a flag it has the last word.
	if !meta.Release {
		meta.Release = parseBool(extractTicketValue(body, "Release"))
	}
	if value := extractTicketValue(body, "allow_script_changes"); value != "" {
		meta.AllowScriptChanges = parseBool(value)
	}
	if value := extractTicketValue(body, "allow_secrets"); value != "" {
		meta.AllowSecrets = parseBool(value)
	}
	if value := extractTicketValue(body, "allow_parent_changes"); value != "" {
		meta.AllowParentChanges = parseBool(value)
	}
	meta.ScriptWhitelist = extractTicketValue(body, "script_whitelist")
	if patterns := strings.Fields(extractTicketValue(body, "secret_patterns")); len(patterns) > 0 {
		meta.SecretPatterns = patterns
	}

	return meta
}

// SaveTicketFile saves the issue body to a ticket file
func SaveTicketFile(stateRoot string, issueNumber int, body string) (string, error) {
	tempDir := filepath.Join(stateRoot, ".ai", "temp")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	ticketPath := filepath.Join(tempDir, fmt.Sprintf("ticket-%d.md", issueNumber))
	if err := os.WriteFile(ticketPath, []byte(body), 0644); err != nil {
		return "", fmt.Errorf("failed to write ticket file: %w", err)
	}

	return ticketPath, nil
}

// LoadTicketFile loads a ticket file content
func LoadTicketFile(stateRoot string, issueNumber int) (string, error) {
	ticketPath := filepath.Join(stateRoot, ".ai", "temp", fmt.Sprintf("ticket-%d.md", issueNumber))
	data, err := os.ReadFile(ticketPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// CleanupTicketFile removes the ticket file
func CleanupTicketFile(stateRoot string, issueNumber int) error {
	ticketPath := filepath.Join(stateRoot, ".ai", "temp", fmt.Sprintf("ticket-%d.md", issueNumber))
	err := os.Remove(ticketPath)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// VerificationCommands contains parsed verification commands from a ticket
type VerificationCommands struct {
	Repo     string   // repo name (backend, frontend, root, etc.)
	Commands []string // list of commands to run
}

// ParseVerificationCommands extracts verification commands from ticket body
// Format: ## Verification
//
//   - backend: `go build ./...` and `go test ./...`
//   - frontend: `npm run build` and `npm run test`
func ParseVerificationCommands(body string) []VerificationCommands {
	var results []VerificationCommands

	// Find ## Verification section
	verificationPattern := regexp.MustCompile(`(?is)##\s*Verification\s*\n(.*?)(?:\n##|\z)`)
	matches := verificationPattern.FindStringSubmatch(body)
	if len(matches) < 2 {
		return results
	}

	section := matches[1]

	// Parse each line: - repo: `cmd1` and `cmd2`
	linePattern := regexp.MustCompile(`(?m)^[-*]\s*(\w+)\s*:\s*(.+)$`)
	lineMatches := linePattern.FindAllStringSubmatch(section, -1)

	for _, match := range lineMatches {
		if len(match) < 3 {
			continue
		}

		repo := strings.TrimSpace(match[1])
		cmdPart := match[2]

		// Extract commands from backticks
		cmdPattern := regexp.MustCompile("`([^`]+)`")
		cmdMatches := cmdPattern.FindAllStringSubmatch(cmdPart, -1)

		var commands []string
		for _, cm := range cmdMatches {
			if len(cm) > 1 {
				cmd := strings.TrimSpace(cm[1])
				if cmd != "" {
					commands = append(commands, cmd)
				}
			}
		}

		if len(commands) > 0 {
			results = append(results, VerificationCommands{
				Repo:     strings.ToLower(repo),
				Commands: commands,
			})
		}
	}

	return results
}

// GetVerificationCommandsForRepo returns verification commands for a specific repo
func GetVerificationCommandsForRepo(body, repoName string) []string {
	allCommands := ParseVerificationCommands(body)
	repoName = strings.ToLower(strings.TrimSpace(repoName))

	for _, vc := range allCommands {
		if vc.Repo == repoName {
			return vc.Commands
		}
	}

	// Fallback: if repo is empty or "root", try to find root commands
	if repoName == "" || repoName == "root" {
		for _, vc := range allCommands {
			if vc.Repo == "root" {
				return vc.Commands
			}
		}
	}

	return nil
}
