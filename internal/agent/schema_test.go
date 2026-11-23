package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

const reviewSchema = `{
  "type": "object",
  "required": ["decision", "score", "findings"],
  "additionalProperties": false,
  "properties": {
    "decision": {"type": "string", "enum": ["approve", "changes_requested"]},
    "score": {"type": "integer", "minimum": 0, "maximum": 10},
    "summary": {"type": "string", "maxLength": 20},
    "findings": {"type": "array", "maxItems": 3, "items": {
      "type": "object", "required": ["file"], "properties": {"file": {"type": "string", "minLength": 1}, "line": {"type": ["integer", "null"]}}
    }},
    "version": {"const": 1}
  }
}`

func TestValidate(t *testing.T) {
	ok := []string{
		`{"decision":"approve","score":9,"findings":[]}`,
		`{"decision":"changes_requested","score":4,"findings":[{"file":"a.go","line":3},{"file":"b.go","line":null}],"version":1.0}`,
	}
	for _, v := range ok {
		if err := validate(json.RawMessage(reviewSchema), json.RawMessage(v)); err != nil {
			t.Errorf("%s: %v", v, err)
		}
	}
	for v, says := range map[string]string{
		`{"decision":"approve","score":9}`:                                                           `has no "findings"`,
		`{"decision":"merge","score":9,"findings":[]}`:                                               "not one of",
		`{"decision":"approve","score":9.5,"findings":[]}`:                                           "is number, not integer",
		`{"decision":"approve","score":11,"findings":[]}`:                                            "more than 10",
		`{"decision":"approve","score":9,"findings":[],"extra":true}`:                                `has "extra"`,
		`{"decision":"approve","score":9,"findings":[{"line":2}]}`:                                   `findings[0] has no "file"`,
		`{"decision":"approve","score":9,"findings":[{"file":""}]}`:                                  "shorter than 1",
		`{"decision":"approve","score":9,"findings":[{},{},{},{}]}`:                                  "more than 3",
		`{"decision":"approve","score":9,"findings":[],"summary":"` + strings.Repeat("x", 21) + `"}`: "longer than 20",
		`{"decision":"approve","score":9,"findings":[],"version":2}`:                                 "must be 1",
		`["approve"]`:     "is array, not object",
		`not json`:        "not JSON",
		`{"a":1} {"b":2}`: "more than one",
	} {
		err := validate(json.RawMessage(reviewSchema), json.RawMessage(v))
		if err == nil || !strings.Contains(err.Error(), says) {
			t.Errorf("%s: %v, want it to say %q", v, err, says)
		}
	}
}

// A schema that uses what the runtime does not check is refused, rather
// than half checked.
func TestParseSchema_Refuses(t *testing.T) {
	for _, s := range []string{
		`{"type":"string","pattern":"^a"}`,
		`{"type":"object","properties":{"x":{"oneOf":[{"type":"string"}]}}}`,
		`{"type":"array","items":{"format":"email"}}`,
		`[1,2]`,
	} {
		if _, err := parseSchema(json.RawMessage(s)); err == nil {
			t.Errorf("%s: read", s)
		}
	}
}
