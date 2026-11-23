package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
)

// The runtime checks every structured answer against its schema itself,
// whether or not the backend was given the schema: what the engine acts on
// is what it checked. The schemas are the engine's own (a review, a
// resolution, a triage), so the part of JSON Schema they use is what is
// checked: type, properties, required, additionalProperties: false, enum,
// const, items, minItems, maxItems, minimum, maximum, minLength and
// maxLength. A keyword outside it is refused when the schema is read, rather
// than passed over unchecked.

var schemaKeywords = []string{
	"$schema", "$id", "title", "description", "default", "examples",
	"type", "properties", "required", "additionalProperties", "enum", "const",
	"items", "minItems", "maxItems", "minimum", "maximum", "minLength", "maxLength",
}

type schema struct {
	Type                 any                `json:"type"`
	Properties           map[string]*schema `json:"properties"`
	Required             []string           `json:"required"`
	AdditionalProperties *bool              `json:"additionalProperties"`
	Enum                 []any              `json:"enum"`
	Const                any                `json:"const"`
	HasConst             bool               `json:"-"`
	Items                *schema            `json:"items"`
	MinItems             *int               `json:"minItems"`
	MaxItems             *int               `json:"maxItems"`
	Minimum              *float64           `json:"minimum"`
	Maximum              *float64           `json:"maximum"`
	MinLength            *int               `json:"minLength"`
	MaxLength            *int               `json:"maxLength"`
}

// parseSchema reads a schema, refusing one that uses what is not checked.
func parseSchema(raw json.RawMessage) (*schema, error) {
	if err := knownKeywords(raw, "schema"); err != nil {
		return nil, err
	}
	var s schema
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("the schema is not valid JSON: %w", err)
	}
	return &s, nil
}

// UnmarshalJSON reads a schema, and whether it has a const: a const of null
// is one, which the field alone cannot tell.
func (s *schema) UnmarshalJSON(data []byte) error {
	type plain schema
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	var probe map[string]json.RawMessage
	_ = json.Unmarshal(data, &probe)
	_, p.HasConst = probe["const"]
	*s = schema(p)
	return nil
}

func knownKeywords(raw json.RawMessage, at string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return fmt.Errorf("%s is not a JSON object", at)
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !slices.Contains(schemaKeywords, k) {
			return fmt.Errorf("%s uses %q, which the runtime does not check", at, k)
		}
	}
	if props, ok := fields["properties"]; ok {
		var each map[string]json.RawMessage
		if err := json.Unmarshal(props, &each); err != nil {
			return fmt.Errorf("%s.properties is not an object", at)
		}
		for name, sub := range each {
			if err := knownKeywords(sub, at+"."+name); err != nil {
				return err
			}
		}
	}
	if items, ok := fields["items"]; ok {
		if err := knownKeywords(items, at+"[]"); err != nil {
			return err
		}
	}
	return nil
}

// validate checks a JSON value against a schema; the error says where it
// does not match.
func validate(raw json.RawMessage, value json.RawMessage) error {
	s, err := parseSchema(raw)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(value))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return fmt.Errorf("the answer is not JSON: %w", err)
	}
	if dec.More() {
		return fmt.Errorf("the answer is more than one JSON value")
	}
	return s.check(v, "the answer")
}

func typeOf(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case json.Number:
		if f, err := x.Float64(); err == nil && f == math.Trunc(f) && !strings.ContainsAny(string(x), ".eE") {
			return "integer"
		}
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return "unknown"
}

func (s *schema) types() []string {
	switch t := s.Type.(type) {
	case string:
		return []string{t}
	case []any:
		var out []string
		for _, x := range t {
			if name, ok := x.(string); ok {
				out = append(out, name)
			}
		}
		return out
	}
	return nil
}

func (s *schema) check(v any, at string) error {
	if want := s.types(); len(want) > 0 {
		got := typeOf(v)
		ok := slices.Contains(want, got) || (got == "integer" && slices.Contains(want, "number"))
		if !ok {
			return fmt.Errorf("%s is %s, not %s", at, got, strings.Join(want, " or "))
		}
	}
	if len(s.Enum) > 0 && !slices.ContainsFunc(s.Enum, func(e any) bool { return same(e, v) }) {
		return fmt.Errorf("%s is %s, which is not one of %s", at, show(v), show(s.Enum))
	}
	if s.HasConst && !same(s.Const, v) {
		return fmt.Errorf("%s must be %s", at, show(s.Const))
	}
	switch x := v.(type) {
	case map[string]any:
		for _, name := range s.Required {
			if _, ok := x[name]; !ok {
				return fmt.Errorf("%s has no %q", at, name)
			}
		}
		names := make([]string, 0, len(x))
		for name := range x {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			sub, ok := s.Properties[name]
			if !ok {
				if s.AdditionalProperties != nil && !*s.AdditionalProperties {
					return fmt.Errorf("%s has %q, which the schema does not have", at, name)
				}
				continue
			}
			if err := sub.check(x[name], at+"."+name); err != nil {
				return err
			}
		}
	case []any:
		if s.MinItems != nil && len(x) < *s.MinItems {
			return fmt.Errorf("%s has %d items, fewer than %d", at, len(x), *s.MinItems)
		}
		if s.MaxItems != nil && len(x) > *s.MaxItems {
			return fmt.Errorf("%s has %d items, more than %d", at, len(x), *s.MaxItems)
		}
		if s.Items != nil {
			for i, item := range x {
				if err := s.Items.check(item, fmt.Sprintf("%s[%d]", at, i)); err != nil {
					return err
				}
			}
		}
	case json.Number:
		f, _ := x.Float64()
		if s.Minimum != nil && f < *s.Minimum {
			return fmt.Errorf("%s is %s, less than %v", at, x, *s.Minimum)
		}
		if s.Maximum != nil && f > *s.Maximum {
			return fmt.Errorf("%s is %s, more than %v", at, x, *s.Maximum)
		}
	case string:
		n := len([]rune(x))
		if s.MinLength != nil && n < *s.MinLength {
			return fmt.Errorf("%s is shorter than %d characters", at, *s.MinLength)
		}
		if s.MaxLength != nil && n > *s.MaxLength {
			return fmt.Errorf("%s is longer than %d characters", at, *s.MaxLength)
		}
	}
	return nil
}

// same compares JSON values as JSON: 4 and 4.0 are the same number.
func same(a, b any) bool {
	na, aok := number(a)
	nb, bok := number(b)
	if aok && bok {
		return na == nb
	}
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return bytes.Equal(ja, jb)
}

func number(v any) (float64, bool) {
	switch x := v.(type) {
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	case float64:
		return x, true
	}
	return 0, false
}

func show(v any) string {
	data, _ := json.Marshal(v)
	return string(data)
}
