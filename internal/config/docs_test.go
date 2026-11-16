package config

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// docs/user/configuration.md describes every key the configuration reads,
// and no key it does not read. The document once described a dozen keys that
// nothing read (limits on the size of a pull request, triggers that were to
// ask for a person); this is what keeps the two together. A field that is
// added without a line in the document fails here, and so does a line for a
// key that has no field.
//
// The document has a section per top-level key, headed "## `key`". Inside it
// a key is named relative to the section, in backticks: `verify.build` under
// repos, `claude_code.model` under worker. The keys of an item of a list
// (a hook, a custom agent, a secondary reviewer) are named on their own.
func TestConfigurationDocument_DescribesEveryKeyAndNoOther(t *testing.T) {
	file := filepath.Join("..", "..", "docs", "user", "configuration.md")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("the document that describes the configuration: %v", err)
	}
	sections := documentSections(string(data))

	// The names of the labels are fixed, so the document says that and does
	// not list the keys; review_fail among them is the name a key had in
	// version 1.0.
	notListed := func(section, key string) bool {
		return section == "github" && strings.HasPrefix(key, "labels.")
	}

	root := reflect.TypeOf(Config{})
	for i := 0; i < root.NumField(); i++ {
		top, _ := keyOf(root.Field(i))
		section, found := sections[top]
		if !found {
			t.Errorf("the document has no section headed ## `%s`", top)
			continue
		}
		read := keysUnder(root.Field(i).Type)

		for _, key := range read {
			if notListed(top, key) {
				continue
			}
			if !strings.Contains(section, "`"+key+"`") {
				t.Errorf("section %s does not describe `%s`, which the configuration reads", top, key)
			}
		}
		for _, key := range keysTheSectionLists(section) {
			if !containsString(read, key) {
				t.Errorf("section %s lists `%s`, and the configuration has no field for it", top, key)
			}
		}
	}

	if !strings.Contains(sections["github"], "`github.labels`") {
		t.Error("the github section does not say what becomes of github.labels")
	}
	// A section for a key the configuration does not have.
	var extra []string
	for name := range sections {
		known := false
		for i := 0; i < root.NumField(); i++ {
			if key, _ := keyOf(root.Field(i)); key == name {
				known = true
			}
		}
		if !known {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	if len(extra) > 0 {
		t.Errorf("the document has sections for keys the configuration does not have: %v", extra)
	}
}

var sectionHeading = regexp.MustCompile("(?m)^## `([a-z_]+)`\\s*$")

// documentSections returns the text of each section headed "## `key`", up to
// the next heading of that level.
func documentSections(doc string) map[string]string {
	sections := map[string]string{}
	heads := sectionHeading.FindAllStringSubmatchIndex(doc, -1)
	for _, head := range heads {
		name := doc[head[2]:head[3]]
		rest := doc[head[1]:]
		if next := strings.Index(rest, "\n## "); next >= 0 {
			rest = rest[:next]
		}
		sections[name] = rest
	}
	return sections
}

var (
	tableRow   = regexp.MustCompile("(?m)^\\| (`[^|]+) \\|")
	backticked = regexp.MustCompile("`([a-z_.]+)`")
)

// keysTheSectionLists returns the keys a section lists: what is in backticks
// in the first cell of the rows of its tables.
func keysTheSectionLists(section string) []string {
	var keys []string
	for _, row := range tableRow.FindAllStringSubmatch(section, -1) {
		for _, key := range backticked.FindAllStringSubmatch(row[1], -1) {
			keys = append(keys, key[1])
		}
	}
	return keys
}

// keysUnder lists the keys of a value of typ, as the document names them:
// with the way to them through nested sections, and starting over at the
// items of a list.
func keysUnder(typ reflect.Type) []string {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	switch typ.Kind() {
	case reflect.Slice:
		return keysUnder(typ.Elem())
	case reflect.Struct:
		var keys []string
		for i := 0; i < typ.NumField(); i++ {
			key, read := keyOf(typ.Field(i))
			if !read {
				continue
			}
			field := typ.Field(i).Type
			for field.Kind() == reflect.Pointer {
				field = field.Elem()
			}
			switch {
			case field.Kind() == reflect.Struct:
				for _, under := range keysUnder(field) {
					keys = append(keys, key+"."+under)
				}
			case field.Kind() == reflect.Slice && field.Elem().Kind() == reflect.Struct:
				keys = append(keys, key)
				keys = append(keys, keysUnder(field.Elem())...)
			default:
				keys = append(keys, key)
			}
		}
		return keys
	}
	return nil
}

func containsString(list []string, item string) bool {
	for _, have := range list {
		if have == item {
			return true
		}
	}
	return false
}
