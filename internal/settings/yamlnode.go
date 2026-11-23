package settings

import (
	"bytes"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// The settings are files a person keeps, with comments that say what each
// key is for. They are changed by their YAML nodes, not written anew: a value
// that changed is set where it is, a key that is missing is added, and the
// rest of the file, comments with it, stays as it was.

// document returns the mapping a file holds, making one for an empty file.
func document(node *yaml.Node) *yaml.Node {
	if node.Kind == 0 {
		node.Kind = yaml.DocumentNode
	}
	if node.Kind == yaml.DocumentNode {
		if len(node.Content) == 0 {
			node.Content = append(node.Content, &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"})
		}
		return node.Content[0]
	}
	return node
}

// get returns the value of a key of a mapping, or nil.
func get(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// child returns the mapping under a key, adding an empty one when the key
// is missing or holds nothing.
func child(mapping *yaml.Node, key string) *yaml.Node {
	if value := get(mapping, key); value != nil {
		if value.Kind != yaml.MappingNode {
			*value = yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		}
		return value
	}
	value := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, value)
	return value
}

// set puts a value under a key: in place of the one there, keeping the
// comments around it, or at the end of the mapping. A value that is the one
// there is left as it is written, quotes and all.
func set(mapping *yaml.Node, key string, value *yaml.Node) {
	if old := get(mapping, key); old != nil {
		if same(old, value) {
			return
		}
		// A text written in quotes stays in quotes. An empty one was quoted
		// only because it was empty.
		if old.Kind == yaml.ScalarNode && value.Kind == yaml.ScalarNode && value.Style == 0 && old.Value != "" && old.ShortTag() == value.ShortTag() {
			value.Style = old.Style
		}
		value.HeadComment, value.LineComment, value.FootComment = old.HeadComment, old.LineComment, old.FootComment
		*old = *value
		return
	}
	mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, value)
}

// same says two values read the same: scalars of one kind and text, or lists
// of such scalars.
func same(a, b *yaml.Node) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case yaml.ScalarNode:
		return a.ShortTag() == b.ShortTag() && a.Value == b.Value
	case yaml.SequenceNode:
		if len(a.Content) != len(b.Content) {
			return false
		}
		for i := range a.Content {
			if !same(a.Content[i], b.Content[i]) {
				return false
			}
		}
		return true
	}
	return false
}

// setString sets a text, leaving the key out when it is empty and was not
// there: an empty value and a missing one mean the same.
func setString(mapping *yaml.Node, key, value string) {
	if value == "" && get(mapping, key) == nil {
		return
	}
	node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
	if value == "" {
		node.Style = yaml.DoubleQuotedStyle
	}
	set(mapping, key, node)
}

// setOptional sets a text whose absence means its default: an empty one is
// taken out.
func setOptional(mapping *yaml.Node, key, value string) {
	if value == "" {
		remove(mapping, key)
		return
	}
	set(mapping, key, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
}

// setOptionalInt sets a number whose absence means its default: nil takes
// it out.
func setOptionalInt(mapping *yaml.Node, key string, value *int) {
	if value == nil {
		remove(mapping, key)
		return
	}
	set(mapping, key, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(*value)})
}

// setText sets a text that may run over several lines, as a block when it
// does.
func setText(mapping *yaml.Node, key, value string) {
	node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
	if strings.Contains(value, "\n") {
		node.Style = yaml.LiteralStyle
	}
	set(mapping, key, node)
}

// setFlag sets a flag that is false when it is not there: a false one is
// taken out.
func setFlag(mapping *yaml.Node, key string, value bool) {
	if !value {
		remove(mapping, key)
		return
	}
	set(mapping, key, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"})
}

// setList sets a list of texts; an empty one is taken out.
func setList(mapping *yaml.Node, key string, values []string) {
	if len(values) == 0 {
		remove(mapping, key)
		return
	}
	list := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, value := range values {
		list.Content = append(list.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
	}
	set(mapping, key, list)
}

// parse reads a file's YAML. Line ends are made plain first: YAML reads a
// comment above a key as the document's when its lines end in CRLF.
func parse(data []byte) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n")), &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

// keysOf lists the keys a mapping has.
func keysOf(mapping *yaml.Node) map[string]bool {
	keys := map[string]bool{}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		keys[mapping.Content[i].Value] = true
	}
	return keys
}

// arrange moves the keys added to a mapping, the ones it did not have, to
// where order puts them: before the first key order puts after them. The
// keys it had stay where they are.
func arrange(mapping *yaml.Node, order []string, had map[string]bool) {
	rank := map[string]int{}
	for i, key := range order {
		rank[key] = i
	}
	for _, key := range order {
		if had[key] {
			continue
		}
		at := -1
		for i := 0; i+1 < len(mapping.Content); i += 2 {
			if mapping.Content[i].Value == key {
				at = i
				break
			}
		}
		if at < 0 {
			continue
		}
		pair := []*yaml.Node{mapping.Content[at], mapping.Content[at+1]}
		rest := append(append([]*yaml.Node{}, mapping.Content[:at]...), mapping.Content[at+2:]...)
		to := len(rest)
		for i := 0; i+1 < len(rest); i += 2 {
			if r, ok := rank[rest[i].Value]; ok && r > rank[key] {
				to = i
				break
			}
		}
		mapping.Content = append(append(append([]*yaml.Node{}, rest[:to]...), pair...), rest[to:]...)
	}
}

// text returns the text under a key, "" when there is none.
func text(mapping *yaml.Node, key string) string {
	if value := get(mapping, key); value != nil && value.Kind == yaml.ScalarNode {
		return value.Value
	}
	return ""
}

// remove takes a key out of a mapping.
func remove(mapping *yaml.Node, key string) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			return
		}
	}
}

// encode writes a document as the file it came from was written: two spaces
// to a level, its blank lines where they were, and its line ends.
func encode(doc *yaml.Node, original []byte, spaced ...func(line string) bool) ([]byte, error) {
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(doc); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	data := keepBlankLines(original, out.Bytes(), spaced...)
	if bytes.Contains(original, []byte("\r\n")) {
		data = bytes.ReplaceAll(data, []byte("\n"), []byte("\r\n"))
	}
	return data, nil
}

// keepBlankLines puts back the blank lines the YAML encoder drops: before a
// line that had one in the original, and, for each kind of line given (a
// top-level key, an item of a list), before every line of that kind but the
// first when the original set them apart so. A comment right above such a
// line goes with it.
func keepBlankLines(original, encoded []byte, spaced ...func(line string) bool) []byte {
	lines := func(data []byte) []string {
		text := strings.ReplaceAll(string(data), "\r\n", "\n")
		return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	}
	blank := func(line string) bool { return strings.TrimSpace(line) == "" }
	before, after := lines(original), lines(encoded)

	// The lines that came after a blank one, in order.
	var anchors []string
	for i := 1; i < len(before); i++ {
		if !blank(before[i]) && blank(before[i-1]) {
			anchors = append(anchors, before[i])
		}
	}
	gap := make([]bool, len(after))
	next := 0
	for i, line := range after {
		for k := next; k < len(anchors); k++ {
			if anchors[k] == line {
				gap[i], next = true, k+1
				break
			}
		}
	}

	// blockStart is where a line's block begins: at the comment right above
	// it, when it has one.
	blockStart := func(all []string, i int) int {
		for i > 0 && strings.HasPrefix(strings.TrimSpace(all[i-1]), "#") {
			i--
		}
		return i
	}
	for _, kind := range spaced {
		apart, first := false, true
		for i, line := range before {
			if !kind(line) {
				continue
			}
			if !first {
				if start := blockStart(before, i); start > 0 && blank(before[start-1]) {
					apart = true
					break
				}
			}
			first = false
		}
		if !apart {
			continue
		}
		first = true
		for i, line := range after {
			if !kind(line) {
				continue
			}
			if !first {
				gap[blockStart(after, i)] = true
			}
			first = false
		}
	}

	var out strings.Builder
	for i, line := range after {
		if gap[i] && i > 0 && !blank(after[i-1]) {
			out.WriteString("\n")
		}
		out.WriteString(line)
		out.WriteString("\n")
	}
	return []byte(out.String())
}

// topLevelKey is a line that starts a key of the document.
func topLevelKey(line string) bool {
	return line != "" && line[0] != ' ' && line[0] != '#' && line[0] != '-' && strings.Contains(line, ":")
}
