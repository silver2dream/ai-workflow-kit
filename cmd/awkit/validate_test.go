package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/layout"
)

// `awkit validate` names the keys of the configuration that nothing reads:
// the ones an earlier awkit wrote and no longer uses, and the ones that are
// misspelt. The file is valid all the same; the list is for whoever set one
// of them and takes it to be in force (a trigger that was to ask for a person
// on anything that mentions security, say). A label under another name is an
// error, because half of the workflow writes the name it knows.
func TestCmdValidate_NamesTheKeysNothingReads(t *testing.T) {
	dir := t.TempDir()
	file := layout.Config(dir)
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	const valid = "version: \"1.2\"\nproject: {name: demo, type: single-repo}\nrepos:\n  - {name: root, path: ./}\ngit: {integration_branch: main}\n"
	validate := func(content string) (int, string) {
		t.Helper()
		if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		code := -1
		out := captureStdout(t, func() { code = cmdValidate([]string{"--config", file}) })
		return code, out
	}

	if code, out := validate(valid); code != 0 || !strings.Contains(out, "Configuration is valid") || strings.Contains(out, "read by nothing") {
		t.Errorf("a file with no key too many: validate = %d\n%s", code, out)
	}

	code, out := validate(valid + "escalation:\n  triggers:\n    - {pattern: security, action: require_human_approval}\n  retry_count: 1\nreveiw: {score_threshold: 9}\n")
	for _, want := range []string{
		"Configuration is valid",
		"2 keys of the configuration are read by nothing, and change nothing:\n  - escalation.triggers (line 7)\n  - reveiw (line 10)\n",
		"Delete them, or correct the spelling.",
	} {
		if code != 0 || !strings.Contains(out, want) {
			t.Errorf("two keys nothing reads: validate = %d, without %q\n%s", code, want, out)
		}
	}

	if code, out := validate(valid + "audit: {}\n"); code != 0 || !strings.Contains(out, "1 key of the configuration is read by nothing, and changes nothing:\n  - audit (line 6)\n") {
		t.Errorf("one key nothing reads: validate = %d\n%s", code, out)
	}

	code, out = validate(valid + "github: {labels: {task: todo}}\n")
	if code != 1 || !strings.Contains(out, `github.labels.task: the label cannot be renamed ("todo")`) || strings.Contains(out, "Configuration is valid") {
		t.Errorf("a renamed label: validate = %d\n%s", code, out)
	}
}
