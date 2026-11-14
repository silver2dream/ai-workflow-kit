package config

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func unreadOf(t *testing.T, content string) []string {
	t.Helper()
	keys, err := UnreadKeys(Path(writeConfig(t, content)))
	if err != nil {
		t.Fatalf("UnreadKeys: %v", err)
	}
	var out []string
	for _, key := range keys {
		out = append(out, key.String())
	}
	return out
}

// A key the configuration has no field for changes nothing, and whoever set
// it believes it does. Each is named with the way to it and its line: the
// section nobody reads, the key inside a section that is read, the key of
// one repo out of several, and the one that is misspelt.
func TestUnreadKeys(t *testing.T) {
	got := unreadOf(t, `version: "1.2"
audit:
  checks: [dirty-worktree]
project: {name: demo, type: single-repo}
repos:
  - {name: root, path: ./}
  - name: backend
    path: backend/
    rules: [backend-go]
    verify:
      build: make
      lint: make lint
git:
  integration_branch: main
  commit_format: "[type] subject"
  pr_body_template: |
    Closes #${ISSUE_ID}
escalation:
  triggers:
    - pattern: "security|credential"
      action: require_human_approval
  max_consecutive_failures: 3
  retry_count: 2
  max_single_pr_files: 50
feedback: {enabled: true, max_history_in_prompt: 10}
reveiw: {score_threshold: 9}
worker: {backend: codex, codex: {max_attempts: 2}}
lessons: {distiller: {backend: claude, model: haiku}}
hooks:
  on_merge:
    - command: ./notify.sh
      retries: 3
      env: {ANY_NAME: is-free}
`)
	want := []string{
		"audit (line 2)",
		"repos[1].rules (line 9)",
		"repos[1].verify.lint (line 12)",
		"git.commit_format (line 15)",
		"git.pr_body_template (line 16)",
		"escalation.triggers (line 19)",
		"escalation.max_consecutive_failures (line 22)",
		"escalation.max_single_pr_files (line 24)",
		"feedback.max_history_in_prompt (line 25)",
		"reveiw (line 26)",
		"worker.codex.max_attempts (line 27)",
		"lessons.distiller.backend (line 28)",
		"hooks.on_merge[0].retries (line 32)",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("unread keys =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}

	// The name a label had in version 1.0 of the file is still read.
	if got := unreadOf(t, "github: {labels: {review_fail: review-failed}}\n"); got != nil {
		t.Errorf("the old name of a label is called unread: %v", got)
	}
	// A file that says nothing has nothing unread, and neither has one whose
	// sections are of the wrong kind: that is for Load to say.
	for _, content := range []string{"", "# nothing\n", "repos: 3\nreview: [squash]\n"} {
		if got := unreadOf(t, content); got != nil {
			t.Errorf("%q: unread keys %v, want none", content, got)
		}
	}
	// A merge key takes its keys from elsewhere; it is not a key of its own.
	if got := unreadOf(t, "base: &base {score_threshold: 8}\nreview:\n  <<: *base\n  merge_strategy: rebase\n"); !reflect.DeepEqual(got, []string{"base (line 1)"}) {
		t.Errorf("a file with a merge key: unread keys %v, want only the anchor's own key", got)
	}
	// A section given by reference is looked into where it is used.
	if got := unreadOf(t, "base: &base {score_threshold: 8, strictness: high}\nreview: *base\n"); !reflect.DeepEqual(got, []string{"base (line 1)", "review.strictness (line 1)"}) {
		t.Errorf("a section given by reference: unread keys %v", got)
	}
}

// A key is read when the decoder reads it: under the name in the field's tag,
// or under the field's name in lower case where there is no tag. A field
// tagged "-" and one that is not exported are read from no key.
func TestCollectUnread_NamesKeysAsTheDecoderDoes(t *testing.T) {
	type sample struct {
		Tagged   string `yaml:"tagged,omitempty"`
		Untagged string
		Skipped  string `yaml:"-"`
		hidden   string
	}
	const content = "tagged: a\nuntagged: b\nskipped: c\nhidden: d\nUntagged: e\n\"-\": f\n"

	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		t.Fatal(err)
	}
	var unread []UnreadKey
	collectUnread(doc.Content[0], reflect.TypeOf(sample{}), "", &unread)
	want := []UnreadKey{{"skipped", 3}, {"hidden", 4}, {"Untagged", 5}, {"-", 6}}
	if !reflect.DeepEqual(unread, want) {
		t.Errorf("unread keys = %v, want %v", unread, want)
	}

	// The decoder agrees: it fills the two fields and no other.
	var got sample
	if err := yaml.Unmarshal([]byte(content), &got); err != nil {
		t.Fatal(err)
	}
	if got != (sample{Tagged: "a", Untagged: "b"}) {
		t.Errorf("the decoder read %+v", got)
	}
}

// A file that cannot be read gives the error of the read; one that is not
// YAML is not valid, in the words Load uses.
func TestUnreadKeys_FilesThatCannotBeUsed(t *testing.T) {
	if _, err := UnreadKeys(Path(t.TempDir())); err == nil || !os.IsNotExist(err) {
		t.Errorf("a file that is not there: err = %v", err)
	}
	_, err := UnreadKeys(Path(writeConfig(t, "project: [unclosed\n")))
	var parse *ParseError
	if !errors.As(err, &parse) || !strings.Contains(err.Error(), "is not valid") {
		t.Errorf("a file that is not YAML: err = %v, want a ParseError", err)
	}
}

// The file that names every key has no key that is unread, and no field of
// the configuration is missing from it. Together with the test that reads it
// into the configuration, this is what makes "every key" true: a field that
// is added without the file fails here, and a key that is added to the file
// without a field fails here too.
func TestFull_NamesEveryKeyAndNoOther(t *testing.T) {
	if got := unreadOf(t, full); got != nil {
		t.Errorf("the file that names every key has keys nothing reads: %v", got)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(full), &doc); err != nil {
		t.Fatal(err)
	}
	named := map[string]bool{}
	keysNamed(doc.Content[0], reflect.TypeOf(Config{}), named)
	var missing []string
	fieldsNotNamed(reflect.TypeOf(Config{}), named, map[reflect.Type]bool{}, &missing)
	// The name review_failed had in version 1.0 is read and not written.
	if want := []string{"Labels.review_fail"}; !reflect.DeepEqual(missing, want) {
		t.Errorf("fields the file does not name = %v, want %v", missing, want)
	}
}

// keysNamed records, as Type.key, every key the node names of a struct.
func keysNamed(node *yaml.Node, typ reflect.Type, named map[string]bool) {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	switch {
	case typ.Kind() == reflect.Slice && node.Kind == yaml.SequenceNode:
		for _, item := range node.Content {
			keysNamed(item, typ.Elem(), named)
		}
	case typ.Kind() == reflect.Struct && node.Kind == yaml.MappingNode:
		fields := map[string]reflect.Type{}
		for i := 0; i < typ.NumField(); i++ {
			name, _, _ := strings.Cut(typ.Field(i).Tag.Get("yaml"), ",")
			fields[name] = typ.Field(i).Type
		}
		for i := 0; i+1 < len(node.Content); i += 2 {
			if field, ok := fields[node.Content[i].Value]; ok {
				named[typ.Name()+"."+node.Content[i].Value] = true
				keysNamed(node.Content[i+1], field, named)
			}
		}
	}
}

// fieldsNotNamed lists, as Type.key, the fields of typ and of every struct
// it holds that were not recorded.
func fieldsNotNamed(typ reflect.Type, named map[string]bool, done map[reflect.Type]bool, missing *[]string) {
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct || done[typ] {
		return
	}
	done[typ] = true
	for i := 0; i < typ.NumField(); i++ {
		name, _, _ := strings.Cut(typ.Field(i).Tag.Get("yaml"), ",")
		if !named[typ.Name()+"."+name] {
			*missing = append(*missing, typ.Name()+"."+name)
		}
		fieldsNotNamed(typ.Field(i).Type, named, done, missing)
	}
}
