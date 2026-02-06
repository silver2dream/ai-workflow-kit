package migrate

import (
	"gopkg.in/yaml.v3"
)

// migrationV1_0ToV1_1 migrates config from v1.0 to v1.1.
//
// Changes:
//   - Fix label value: review_failed "review-fail" → "review-failed"
//   - Add missing labels: merge_conflict, needs_rebase, completed (if absent)
//   - Add gh_retry_count and gh_retry_base_delay to timeouts (if absent)
//   - Add review section with score_threshold and merge_strategy (if absent)
//   - Bump version "1.0" → "1.1"
var migrationV1_0ToV1_1 = Migration{
	FromVersion: "1.0",
	ToVersion:   "1.1",
	Description: "fix review_failed label value, add missing labels/timeouts/review section",
	Apply:       applyV1_0ToV1_1,
}

func applyV1_0ToV1_1(doc *yaml.Node) error {
	root := doc.Content[0] // mapping node

	// 1. Bump version
	setScalarValue(root, "version", "1.1")

	// 2. Fix label value and add missing labels
	if github := findMapKey(root, "github"); github != nil {
		if labels := findMapKey(github, "labels"); labels != nil {
			// Fix review_failed value: "review-fail" → "review-failed"
			if v := findMapKey(labels, "review_failed"); v != nil {
				if v.Value == "review-fail" {
					v.Value = "review-failed"
				}
			}

			// Add missing labels with defaults if not present
			ensureMapEntry(labels, "merge_conflict", "merge-conflict")
			ensureMapEntry(labels, "needs_rebase", "needs-rebase")
			ensureMapEntry(labels, "completed", "completed")
		}
	}

	// 3. Add missing timeout fields
	if timeouts := findMapKey(root, "timeouts"); timeouts != nil {
		ensureMapEntry(timeouts, "gh_retry_count", "3")
		ensureMapEntry(timeouts, "gh_retry_base_delay", "2")
	}

	// 4. Add review section if absent
	if findMapKey(root, "review") == nil {
		reviewMap := &yaml.Node{
			Kind: yaml.MappingNode,
			Tag:  "!!map",
		}
		reviewMap.Content = append(reviewMap.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "score_threshold"},
			&yaml.Node{Kind: yaml.ScalarNode, Value: "7", Tag: "!!int"},
			&yaml.Node{Kind: yaml.ScalarNode, Value: "merge_strategy"},
			&yaml.Node{Kind: yaml.ScalarNode, Value: "squash"},
		)
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "review"},
			reviewMap,
		)
	}

	return nil
}

// findMapKey returns the value node for a given key in a mapping node.
func findMapKey(mapping *yaml.Node, key string) *yaml.Node {
	if mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// setScalarValue sets the value of a top-level scalar key.
func setScalarValue(mapping *yaml.Node, key, value string) {
	if v := findMapKey(mapping, key); v != nil {
		v.Value = value
		return
	}
	// Key doesn't exist, add it
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Value: value},
	)
}

// ensureMapEntry adds a key-value pair to a mapping only if the key is not present.
func ensureMapEntry(mapping *yaml.Node, key, value string) {
	if findMapKey(mapping, key) != nil {
		return // already exists
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Value: value},
	)
}
