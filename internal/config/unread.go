package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

// UnreadKey is a key of a configuration file that nothing reads.
type UnreadKey struct {
	// Key is the way to the key through the file: escalation.triggers,
	// repos[1].rules.
	Key string
	// Line is the line of the file the key is on.
	Line int
}

func (k UnreadKey) String() string { return fmt.Sprintf("%s (line %d)", k.Key, k.Line) }

// UnreadKeys names the keys of the configuration at path that nothing reads,
// in the order of the file. Such a key changes nothing: an earlier awkit wrote
// it and no longer uses it, or it is misspelt, or it stands for something
// that was never built. Whoever set it believes otherwise, which is why
// `awkit validate` names them.
//
// The keys are the ones Config has no field for. What is under an unread key
// is not looked into: the key says it all.
func UnreadKeys(path string) ([]UnreadKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, &ParseError{File: filepath.ToSlash(path), Err: err}
	}
	if len(doc.Content) == 0 {
		return nil, nil
	}
	var unread []UnreadKey
	collectUnread(doc.Content[0], reflect.TypeOf(Config{}), "", &unread)
	return unread, nil
}

// collectUnread goes through node as a value of typ. A mapping read into a
// struct has its keys looked up among the struct's fields; a sequence is gone
// through item by item. Anything else (a scalar, a map of free names such as
// a hook's env) has no keys to check.
func collectUnread(node *yaml.Node, typ reflect.Type, at string, unread *[]UnreadKey) {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if node.Kind == yaml.AliasNode && node.Alias != nil {
		node = node.Alias
	}
	switch {
	case typ.Kind() == reflect.Struct && node.Kind == yaml.MappingNode:
		fields := map[string]reflect.Type{}
		for i := 0; i < typ.NumField(); i++ {
			if name, read := keyOf(typ.Field(i)); read {
				fields[name] = typ.Field(i).Type
			}
		}
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			if key.Tag == "!!merge" {
				continue
			}
			name := key.Value
			if at != "" {
				name = at + "." + key.Value
			}
			field, read := fields[key.Value]
			if !read {
				*unread = append(*unread, UnreadKey{Key: name, Line: key.Line})
				continue
			}
			collectUnread(value, field, name, unread)
		}
	case typ.Kind() == reflect.Slice && node.Kind == yaml.SequenceNode:
		for i, item := range node.Content {
			collectUnread(item, typ.Elem(), fmt.Sprintf("%s[%d]", at, i), unread)
		}
	}
}

// keyOf returns the key a field is read from, named the way the YAML decoder
// names it: by the field's tag, or by the field's name in lower case when it
// has none. A field that is not exported, or tagged "-", is read from no key.
func keyOf(field reflect.StructField) (key string, read bool) {
	if !field.IsExported() {
		return "", false
	}
	name, _, _ := strings.Cut(field.Tag.Get("yaml"), ",")
	switch name {
	case "-":
		return "", false
	case "":
		return strings.ToLower(field.Name), true
	}
	return name, true
}
