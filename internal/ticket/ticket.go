// Package ticket reads and writes the machine-readable part of a ticket.
//
// A ticket is a GitHub issue. What the workflow needs to know about it (which
// repo it is for, which tickets must land first, which safety flags it sets)
// sits in the issue body as a fenced YAML block:
//
//	```yaml awk-ticket
//	schema: 1
//	repo: backend
//	depends_on: [12, 15]
//	```
//
// The block is read strictly. A field this version does not know, a schema it
// does not read or a value that makes no sense is an error, never a guess: the
// block decides where a Worker runs and what it may touch.
package ticket

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Schema is the version of the block this build reads and writes.
const Schema = 1

// Info is the info string of the fenced block.
const Info = "yaml awk-ticket"

// RootRepo names the repository root. It is a repo of every workflow config.
const RootRepo = "root"

// Meta is what the workflow needs to know about a ticket.
type Meta struct {
	Schema int `yaml:"schema"`
	// Repo names the repo of the workflow config the ticket is for.
	Repo string `yaml:"repo"`
	// DependsOn lists the issues that must be done before this one starts.
	DependsOn []int `yaml:"depends_on"`
	// Severity is P0 to P3, or empty.
	Severity string `yaml:"severity"`
	// Release marks a ticket whose pull request goes to the release branch.
	Release bool `yaml:"release"`
	// The safety flags a ticket may set for its Worker.
	AllowParentChanges bool `yaml:"allow_parent_changes"`
	AllowScriptChanges bool `yaml:"allow_script_changes"`
	AllowSecrets       bool `yaml:"allow_secrets"`
}

var (
	fenceLine    = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})(.*)$")
	unknownField = regexp.MustCompile(`field (\S+) not found in type \S+`)
	spaces       = regexp.MustCompile(`\s+`)
	severities   = map[string]bool{"": true, "P0": true, "P1": true, "P2": true, "P3": true}
)

// Parse returns the ticket block of an issue body.
//
// found is false when the body has no block: a ticket written by hand, or
// before the block existed. An error means the body has a block and the block
// cannot be used; the caller must not fall back to anything else.
func Parse(body string) (meta Meta, found bool, err error) {
	blocks, err := blocksIn(body)
	if err != nil {
		return Meta{}, true, err
	}
	switch len(blocks) {
	case 0:
		return Meta{}, false, nil
	case 1:
	default:
		return Meta{}, true, fmt.Errorf("the ticket has %d awk-ticket blocks, it must have exactly one", len(blocks))
	}

	decoder := yaml.NewDecoder(strings.NewReader(blocks[0]))
	decoder.KnownFields(true)
	if err := decoder.Decode(&meta); err != nil {
		if errors.Is(err, io.EOF) {
			return Meta{}, true, errors.New("the awk-ticket block is empty")
		}
		return Meta{}, true, fmt.Errorf("the awk-ticket block is not valid: %s", yamlProblem(err))
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return Meta{}, true, errors.New("the awk-ticket block holds more than one YAML document")
	}
	if err := meta.Validate(); err != nil {
		return Meta{}, true, fmt.Errorf("the awk-ticket block is not valid: %w", err)
	}
	return meta, true, nil
}

// yamlProblem words a decoding error for the person who wrote the ticket: on
// one line, and without the names of Go types.
func yamlProblem(err error) string {
	text := strings.TrimPrefix(err.Error(), "yaml: ")
	text = strings.TrimPrefix(text, "unmarshal errors:")
	text = unknownField.ReplaceAllString(text, `unknown field "$1"`)
	text = strings.ReplaceAll(text, "into ticket.Meta", "into the block's fields (write the block as key: value lines)")
	return strings.TrimSpace(spaces.ReplaceAllString(text, " "))
}

// fence is the line that opens a fenced code block.
type fence struct {
	char byte   // ` or ~
	size int    // how many of them
	info string // what follows them: the info string
}

// openingFence reads a line as the opening of a fenced code block.
func openingFence(line string) (fence, bool) {
	m := fenceLine.FindStringSubmatch(line)
	if m == nil {
		return fence{}, false
	}
	info := strings.TrimSpace(m[2])
	if m[1][0] == '`' && strings.Contains(info, "`") {
		// Backticks after the run of backticks: inline code, not a block.
		return fence{}, false
	}
	return fence{char: m[1][0], size: len(m[1]), info: info}, true
}

// closedBy reports whether a line closes the block the fence opened.
func (f fence) closedBy(line string) bool {
	m := fenceLine.FindStringSubmatch(line)
	return m != nil && m[1][0] == f.char && len(m[1]) >= f.size && strings.TrimSpace(m[2]) == ""
}

// namesTicket reports whether the info string has the word awk-ticket in it.
func (f fence) namesTicket() bool {
	for _, word := range strings.Fields(f.info) {
		if strings.EqualFold(word, "awk-ticket") {
			return true
		}
	}
	return false
}

// opensTicketBlock reports whether the fence opens the ticket's block. A fence
// that names awk-ticket in any other way is an error: its author meant the
// block, and reading the ticket as if it had none would be a guess.
func (f fence) opensTicketBlock() (bool, error) {
	if !f.namesTicket() {
		return false, nil
	}
	if strings.Join(strings.Fields(f.info), " ") != Info {
		return false, fmt.Errorf("a code block is marked %q; the ticket block is marked exactly %q", f.info, Info)
	}
	return true, nil
}

// blocksIn returns the content of every awk-ticket block of a body. A block
// shown inside another code block, as an example, is not one of them.
func blocksIn(body string) ([]string, error) {
	var blocks []string
	lines := strings.Split(body, "\n") // a line may end in \r: nothing below minds
	for i := 0; i < len(lines); i++ {
		open, ok := openingFence(lines[i])
		if !ok {
			continue
		}
		isTicket, err := open.opensTicketBlock()
		if err != nil {
			return nil, err
		}
		start := i + 1
		for i = start; i < len(lines) && !open.closedBy(lines[i]); i++ {
		}
		if !isTicket {
			// Another code block. One that is not closed runs to the end.
			continue
		}
		if i == len(lines) {
			return nil, errors.New("the awk-ticket block is not closed")
		}
		blocks = append(blocks, strings.Join(lines[start:i], "\n"))
	}
	return blocks, nil
}

// Validate checks the values of a block.
func (m Meta) Validate() error {
	switch {
	case m.Schema == 0:
		return fmt.Errorf("schema is missing (this version reads schema %d)", Schema)
	case m.Schema != Schema:
		return fmt.Errorf("schema %d is not one this version reads (it reads schema %d)", m.Schema, Schema)
	}
	if m.Repo == "" {
		return errors.New("repo is missing: name the repo of the workflow config this ticket is for")
	}
	if strings.ContainsAny(m.Repo, " \t\r\n") {
		return fmt.Errorf("repo %q has whitespace in it", m.Repo)
	}
	seen := map[int]bool{}
	for _, issue := range m.DependsOn {
		if issue <= 0 {
			return fmt.Errorf("depends_on has %d, which is not an issue number", issue)
		}
		if seen[issue] {
			return fmt.Errorf("depends_on lists #%d twice", issue)
		}
		seen[issue] = true
	}
	if !severities[m.Severity] {
		return fmt.Errorf("severity %q is not one of P0, P1, P2, P3", m.Severity)
	}
	if m.Release && !strings.EqualFold(m.Repo, RootRepo) {
		return fmt.Errorf("release is set for repo %q; only a ticket for the %s repo can be a release", m.Repo, RootRepo)
	}
	return nil
}

var plainScalar = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_./-]*$`)

// Render returns the block as it goes into an issue body, fences included.
// Only what is set is written, in a fixed order.
func Render(meta Meta) string {
	var b strings.Builder
	b.WriteString("```" + Info + "\n")
	fmt.Fprintf(&b, "schema: %d\n", meta.Schema)
	fmt.Fprintf(&b, "repo: %s\n", yamlScalar(meta.Repo))
	if len(meta.DependsOn) > 0 {
		numbers := make([]string, len(meta.DependsOn))
		for i, issue := range meta.DependsOn {
			numbers[i] = strconv.Itoa(issue)
		}
		fmt.Fprintf(&b, "depends_on: [%s]\n", strings.Join(numbers, ", "))
	}
	if meta.Severity != "" {
		fmt.Fprintf(&b, "severity: %s\n", meta.Severity)
	}
	for _, flag := range []struct {
		name string
		set  bool
	}{
		{"release", meta.Release},
		{"allow_parent_changes", meta.AllowParentChanges},
		{"allow_script_changes", meta.AllowScriptChanges},
		{"allow_secrets", meta.AllowSecrets},
	} {
		if flag.set {
			fmt.Fprintf(&b, "%s: true\n", flag.name)
		}
	}
	b.WriteString("```\n")
	return b.String()
}

// yamlScalar writes a string so that YAML reads the same string back.
func yamlScalar(value string) string {
	switch strings.ToLower(value) {
	case "true", "false", "yes", "no", "on", "off", "null", "y", "n":
		return strconv.Quote(value)
	}
	if plainScalar.MatchString(value) {
		return value
	}
	return strconv.Quote(value)
}
