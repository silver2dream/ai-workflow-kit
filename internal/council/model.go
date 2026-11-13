package council

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/claudecli"
)

// Model answers one prompt with one text. A role is never more than that: it
// has no tools and writes no files.
type Model interface {
	Ask(ctx context.Context, model, prompt string) (string, error)
}

// ClaudeCLI asks a headless Claude: a single turn, text only, with none of the
// project's own instructions loaded (see package claudecli).
type ClaudeCLI struct{}

// Ask runs one `claude --print` call with the prompt on stdin.
func (ClaudeCLI) Ask(ctx context.Context, model, prompt string) (string, error) {
	if _, err := exec.LookPath("claude"); err != nil {
		return "", errors.New("the claude CLI is not on the PATH")
	}
	cmd := exec.CommandContext(ctx, "claude", claudecli.OneShotArgs(model)...)
	cmd.Stdin = strings.NewReader(prompt)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("the call did not finish in time: %w", ctx.Err())
		}
		return "", fmt.Errorf("claude failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// fenced returns the code blocks of an answer: for each, its info string and
// what is in it.
func fenced(answer string) (blocks []struct{ info, body string }) {
	lines := strings.Split(strings.ReplaceAll(answer, "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "```") {
			continue
		}
		info := strings.ToLower(strings.TrimSpace(strings.TrimLeft(line, "`")))
		start := i + 1
		for i = start; i < len(lines) && strings.TrimSpace(lines[i]) != "```"; i++ {
		}
		if i == len(lines) {
			break // not closed: what follows is not a block
		}
		blocks = append(blocks, struct{ info, body string }{info, strings.Join(lines[start:i], "\n")})
	}
	return blocks
}

// decodeJSON reads the JSON object of an answer into a value. A role is asked
// for one object in a json block; an answer that has the object in a block
// without the word, or with no block at all and a sentence around it, is
// still read.
func decodeJSON(answer string, into any) error {
	var candidates []string
	blocks := fenced(answer)
	for _, block := range blocks {
		if block.info == "json" {
			candidates = append(candidates, block.body)
		}
	}
	for _, block := range blocks {
		if block.info == "" {
			candidates = append(candidates, block.body)
		}
	}
	if start, end := strings.Index(answer, "{"), strings.LastIndex(answer, "}"); start >= 0 && end > start {
		candidates = append(candidates, answer[start:end+1])
	}
	if len(candidates) == 0 {
		return errors.New("the answer holds no JSON object")
	}
	var firstErr error
	for _, candidate := range candidates {
		err := json.Unmarshal([]byte(candidate), into)
		if err == nil {
			return nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return fmt.Errorf("the JSON of the answer cannot be read: %w", firstErr)
}

var (
	documentOpen      = regexp.MustCompile(`^<<<DOCUMENT[ \t]+(.+?)[ \t]*>>>[ \t]*$`)
	documentClose     = regexp.MustCompile(`^<<<END>>>[ \t]*$`)
	documentUnchanged = regexp.MustCompile(`^<<<UNCHANGED[ \t]+(.+?)[ \t]*>>>[ \t]*$`)
)

// document is a file a role handed in, or one it leaves as it is.
type document struct {
	Path    string
	Content string
	// Unchanged says the role leaves the document as it stands.
	Unchanged bool
}

// parseDocuments reads the documents of an answer. Documents are long and are
// markdown themselves, so they do not travel inside JSON: each one sits
// between a line that names its path and a line that ends it. A document the
// role leaves as it is takes one line.
func parseDocuments(answer string) ([]document, error) {
	var docs []document
	lines := strings.Split(strings.ReplaceAll(answer, "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		if unchanged := documentUnchanged.FindStringSubmatch(lines[i]); unchanged != nil {
			docs = append(docs, document{Path: unchanged[1], Unchanged: true})
			continue
		}
		open := documentOpen.FindStringSubmatch(lines[i])
		if open == nil {
			continue
		}
		start := i + 1
		for i = start; i < len(lines) && !documentClose.MatchString(lines[i]); i++ {
		}
		if i == len(lines) {
			return nil, fmt.Errorf("the document %s is not ended with <<<END>>>", open[1])
		}
		content := strings.Trim(strings.Join(lines[start:i], "\n"), "\n")
		docs = append(docs, document{Path: open[1], Content: content + "\n"})
	}
	if len(docs) == 0 {
		return nil, errors.New("the answer holds no document: each one starts with a <<<DOCUMENT path>>> line and ends with <<<END>>>")
	}
	return docs, nil
}
