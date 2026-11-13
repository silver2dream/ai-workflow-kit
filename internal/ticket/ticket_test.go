package ticket

import (
	"reflect"
	"strings"
	"testing"
)

func fenced(lines ...string) string {
	return "```yaml awk-ticket\n" + strings.Join(lines, "\n") + "\n```\n"
}

func TestParse(t *testing.T) {
	body := "## Summary\nAdd a health endpoint.\n\n" + fenced(
		"schema: 1",
		"repo: root",
		"depends_on: [12, 15]",
		"severity: P1",
		"release: true",
		"allow_script_changes: true",
	) + "\n## Scope\n- backend/health\n"

	meta, found, err := Parse(body)

	if err != nil || !found {
		t.Fatalf("Parse: found=%v err=%v", found, err)
	}
	want := Meta{Schema: 1, Repo: "root", DependsOn: []int{12, 15}, Severity: "P1", Release: true, AllowScriptChanges: true}
	if !reflect.DeepEqual(meta, want) {
		t.Errorf("meta = %+v, want %+v", meta, want)
	}
}

func TestParse_EveryFlag(t *testing.T) {
	meta, found, err := Parse(fenced("schema: 1", "repo: backend", "allow_parent_changes: true", "allow_script_changes: true", "allow_secrets: true"))
	if err != nil || !found {
		t.Fatalf("Parse: found=%v err=%v", found, err)
	}
	want := Meta{Schema: 1, Repo: "backend", AllowParentChanges: true, AllowScriptChanges: true, AllowSecrets: true}
	if !reflect.DeepEqual(meta, want) {
		t.Errorf("meta = %+v, want %+v", meta, want)
	}
}

func TestParse_OnlyWhatIsNeeded(t *testing.T) {
	meta, found, err := Parse(fenced("schema: 1", "repo: root"))
	if err != nil || !found {
		t.Fatalf("Parse: found=%v err=%v", found, err)
	}
	if !reflect.DeepEqual(meta, Meta{Schema: 1, Repo: "root"}) {
		t.Errorf("meta = %+v", meta)
	}
}

// A body without the block is not an error: it is a ticket written by hand or
// before the block existed, and the caller decides what to do with it.
func TestParse_NoBlock(t *testing.T) {
	for name, body := range map[string]string{
		"empty":                    "",
		"prose only":               "## Summary\n- **Repo**: backend\n",
		"another yaml block":       "```yaml\nschema: 1\nrepo: backend\n```\n",
		"the info string in prose": "Put a `yaml awk-ticket` block here.\n",
		"inline code on one line":  "```yaml awk-ticket ``` is how the block starts.\n",
		"indented as code":         "    ```yaml awk-ticket\n    schema: 1\n    repo: x\n    ```\n",
		"another word":             "```yaml awk-ticket-draft\nschema: 1\nrepo: x\n```\n",
		// The block as an example, inside the code block that shows a template.
		"an example in a longer fence":      "````markdown\n## Summary\n" + fenced("schema: 1", "repo: x") + "````\n",
		"an example in a tilde fence":       "~~~\n" + fenced("schema: 1", "repo: x") + "~~~\n",
		"an example in an open fence":       "```text\nthe block looks like this:\n````yaml awk-ticket\nschema: 1\n",
		"an example, the fence longer":      "`````\n````yaml awk-ticket\nschema: 1\nrepo: x\n````\n`````\n",
		"an example, a shorter fence in it": "~~~~\n```yaml awk-ticket\nschema: 1\nrepo: x\n```\n~~~\nstill inside\n~~~~\n",
	} {
		meta, found, err := Parse(body)
		if found || err != nil || !reflect.DeepEqual(meta, Meta{}) {
			t.Errorf("%s: meta=%+v found=%v err=%v; want no block", name, meta, found, err)
		}
	}
}

// How the block is fenced and what line ends the body has does not matter.
func TestParse_FencesAndLineEnds(t *testing.T) {
	want := Meta{Schema: 1, Repo: "backend", DependsOn: []int{3}}
	for name, body := range map[string]string{
		"CRLF":                    "text\r\n```yaml awk-ticket\r\nschema: 1\r\nrepo: backend\r\ndepends_on: [3]\r\n```\r\nmore\r\n",
		"four backticks":          "````yaml awk-ticket\nschema: 1\nrepo: backend\ndepends_on: [3]\n````\n",
		"closed by a longer one":  "```yaml awk-ticket\nschema: 1\nrepo: backend\ndepends_on: [3]\n`````\n",
		"tildes":                  "~~~yaml awk-ticket\nschema: 1\nrepo: backend\ndepends_on: [3]\n~~~\n",
		"spaces after the info":   "```yaml awk-ticket  \nschema: 1\nrepo: backend\ndepends_on: [3]\n```  \n",
		"spaces around the info":  "```  yaml   awk-ticket\nschema: 1\nrepo: backend\ndepends_on: [3]\n```\n",
		"indented up to three":    "   ```yaml awk-ticket\nschema: 1\nrepo: backend\ndepends_on: [3]\n   ```\n",
		"block list form":         fenced("schema: 1", "repo: backend", "depends_on:", "  - 3"),
		"comments and blanks":     fenced("# what the workflow reads", "schema: 1", "", "repo: backend   # the Go service", "depends_on: [3]"),
		"last thing in the body":  "text\n```yaml awk-ticket\nschema: 1\nrepo: backend\ndepends_on: [3]\n```",
		"after another block":     "```go\nfunc main() {}\n```\n\n" + fenced("schema: 1", "repo: backend", "depends_on: [3]"),
		"after an example of it":  "````markdown\n" + fenced("schema: 1", "repo: example") + "````\n\n" + fenced("schema: 1", "repo: backend", "depends_on: [3]"),
		"a fence inside the yaml": fenced("schema: 1", "repo: backend", "depends_on: [3]", "# ~~~ and `` are only text here"),
		// A fence with an info string never closes a block: the example ends
		// at the first bare fence, and the block after it is the ticket's.
		"after an example in a fence of the same length": "```markdown\n```yaml awk-ticket\nschema: 1\nrepo: example\n```\n\n" +
			fenced("schema: 1", "repo: backend", "depends_on: [3]"),
	} {
		meta, found, err := Parse(body)
		if err != nil || !found {
			t.Errorf("%s: found=%v err=%v", name, found, err)
			continue
		}
		if !reflect.DeepEqual(meta, want) {
			t.Errorf("%s: meta = %+v, want %+v", name, meta, want)
		}
	}
}

// A block that is there and cannot be used is an error. Nothing is guessed
// and nothing else is read instead.
func TestParse_InvalidBlockIsAnError(t *testing.T) {
	tests := []struct {
		name, body, wantInError string
	}{
		{"unknown field", fenced("schema: 1", "repo: backend", "priority: high"), `unknown field "priority"`},
		{"a misspelt field", fenced("schema: 1", "repo: backend", "depend_on: [3]"), `unknown field "depend_on"`},
		{"no schema", fenced("repo: backend"), "schema is missing"},
		{"a schema from the future", fenced("schema: 2", "repo: backend"), "schema 2 is not one this version reads"},
		{"no repo", fenced("schema: 1"), "repo is missing"},
		{"empty repo", fenced("schema: 1", `repo: ""`), "repo is missing"},
		{"repo with a space", fenced("schema: 1", `repo: "back end"`), "whitespace"},
		{"dependency zero", fenced("schema: 1", "repo: backend", "depends_on: [0]"), "not an issue number"},
		{"negative dependency", fenced("schema: 1", "repo: backend", "depends_on: [-4]"), "not an issue number"},
		{"dependency twice", fenced("schema: 1", "repo: backend", "depends_on: [3, 3]"), "lists #3 twice"},
		{"dependency not a number", fenced("schema: 1", "repo: backend", "depends_on: [three]"), "not valid"},
		{"dependencies not a list", fenced("schema: 1", "repo: backend", "depends_on: 3"), "not valid"},
		{"severity out of range", fenced("schema: 1", "repo: backend", "severity: P9"), "not one of P0, P1, P2, P3"},
		{"a flag that is not a boolean", fenced("schema: 1", "repo: backend", "allow_secrets: maybe"), "not valid"},
		{"schema as text", fenced(`schema: "one"`, "repo: backend"), "not valid"},
		{"a key given twice", fenced("schema: 1", "repo: backend", "repo: frontend"), "not valid"},
		{"not a mapping", fenced("- schema: 1"), "not valid"},
		{"broken YAML", fenced("schema: 1", "repo: [backend"), "not valid"},
		{"a release outside the root repo", fenced("schema: 1", "repo: backend", "release: true"), "only a ticket for the root repo can be a release"},
		{"empty block", "```yaml awk-ticket\n```\n", "is empty"},
		{"only a comment", fenced("# to be filled in"), "is empty"},
		{"two documents", fenced("schema: 1", "repo: backend", "---", "schema: 1", "repo: frontend"), "more than one YAML document"},
		{"not closed", "```yaml awk-ticket\nschema: 1\nrepo: backend\n", "is not closed"},
		{"closed by a shorter fence", "````yaml awk-ticket\nschema: 1\nrepo: backend\n```\n", "is not closed"},
		{"closed by the other fence", "```yaml awk-ticket\nschema: 1\nrepo: backend\n~~~\n", "is not closed"},
		{"two blocks", fenced("schema: 1", "repo: backend") + "\n" + fenced("schema: 1", "repo: frontend"), "2 awk-ticket blocks"},
		// The author meant the block. Reading the ticket as one without a
		// block would be a guess.
		{"the info string misspelt", "```yml awk-ticket\nschema: 1\nrepo: backend\n```\n", `marked "yml awk-ticket"`},
		{"only half the info string", "```awk-ticket\nschema: 1\nrepo: backend\n```\n", `marked exactly "yaml awk-ticket"`},
		{"the info string in capitals", "```YAML AWK-TICKET\nschema: 1\nrepo: backend\n```\n", `marked "YAML AWK-TICKET"`},
		{"more in the info string", "```yaml awk-ticket v1\nschema: 1\nrepo: backend\n```\n", `marked "yaml awk-ticket v1"`},
	}
	for _, tt := range tests {
		meta, found, err := Parse(tt.body)
		if err == nil {
			t.Errorf("%s: accepted as %+v", tt.name, meta)
			continue
		}
		if !found {
			t.Errorf("%s: found is false; the caller would fall back to reading the prose", tt.name)
		}
		if !strings.Contains(err.Error(), tt.wantInError) {
			t.Errorf("%s: error = %q, want it to mention %q", tt.name, err, tt.wantInError)
		}
		if strings.Contains(err.Error(), "ticket.Meta") || strings.Contains(err.Error(), "\n") {
			t.Errorf("%s: the error is not written for the ticket's author: %q", tt.name, err)
		}
		if !reflect.DeepEqual(meta, Meta{}) {
			t.Errorf("%s: a half-read block was returned: %+v", tt.name, meta)
		}
	}
}

// The root repo may be written in any case; the workflow lowers repo names.
func TestValidate_ReleaseForTheRootRepo(t *testing.T) {
	for _, repo := range []string{"root", "Root", "ROOT"} {
		if err := (Meta{Schema: 1, Repo: repo, Release: true}).Validate(); err != nil {
			t.Errorf("repo %q: a release ticket was refused: %v", repo, err)
		}
	}
	if err := (Meta{Schema: 1, Repo: "backend"}).Validate(); err != nil {
		t.Errorf("a ticket that is not a release was refused: %v", err)
	}
}

func TestRender(t *testing.T) {
	tests := []struct {
		name string
		meta Meta
		want string
	}{
		{"the least", Meta{Schema: 1, Repo: "root"}, "```yaml awk-ticket\nschema: 1\nrepo: root\n```\n"},
		{"everything", Meta{Schema: 1, Repo: "root", DependsOn: []int{12, 15}, Severity: "P0", Release: true,
			AllowParentChanges: true, AllowScriptChanges: true, AllowSecrets: true},
			"```yaml awk-ticket\nschema: 1\nrepo: root\ndepends_on: [12, 15]\nseverity: P0\nrelease: true\n" +
				"allow_parent_changes: true\nallow_script_changes: true\nallow_secrets: true\n```\n"},
	}
	for _, tt := range tests {
		if got := Render(tt.meta); got != tt.want {
			t.Errorf("%s: Render =\n%s\nwant\n%s", tt.name, got, tt.want)
		}
	}
}

// Whatever Render writes, Parse reads back as the same thing, also for repo
// names YAML would otherwise take for something else.
func TestRender_RoundTrips(t *testing.T) {
	repos := []string{"root", "backend", "game-client", "services/api", "api.v2", "true", "no", "null", "123", "1.5", "~", "a:b", "#web", "名稱"}
	for _, repo := range repos {
		meta := Meta{Schema: 1, Repo: repo, DependsOn: []int{7, 2, 40}, Severity: "P2", AllowSecrets: true}

		got, found, err := Parse("intro\n\n" + Render(meta) + "\noutro\n")

		if err != nil || !found {
			t.Errorf("repo %q: found=%v err=%v\n%s", repo, found, err, Render(meta))
			continue
		}
		if !reflect.DeepEqual(got, meta) {
			t.Errorf("repo %q: read back %+v, wrote %+v", repo, got, meta)
		}
	}
}
