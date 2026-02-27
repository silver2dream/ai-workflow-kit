package migrate

import (
	"gopkg.in/yaml.v3"
)

// migrationV1_1ToV1_2 migrates config from v1.1 to v1.2.
//
// Changes:
//   - Add agents section (if absent): builtin pr-reviewer + conflict-resolver
//   - Add escalation section (if absent): triggers, failure limits, PR size limits
//   - Add feedback section (if absent): enabled + max_history_in_prompt
//   - Add hooks section (if absent): empty map
//   - Add worker section (if absent): backend codex
//   - Bump version "1.1" → "1.2"
var migrationV1_1ToV1_2 = Migration{
	FromVersion: "1.1",
	ToVersion:   "1.2",
	Description: "add agents, escalation, feedback, hooks, worker sections",
	Apply:       applyV1_1ToV1_2,
}

func applyV1_1ToV1_2(doc *yaml.Node) error {
	root := doc.Content[0] // mapping node

	// 1. Bump version
	setScalarValue(root, "version", "1.2")

	// 2. Add agents section if absent
	if findMapKey(root, "agents") == nil {
		builtinSeq := &yaml.Node{
			Kind: yaml.SequenceNode,
			Tag:  "!!seq",
			Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: "pr-reviewer"},
				{Kind: yaml.ScalarNode, Value: "conflict-resolver"},
			},
		}
		customSeq := &yaml.Node{
			Kind: yaml.SequenceNode,
			Tag:  "!!seq",
		}
		agentsMap := &yaml.Node{
			Kind: yaml.MappingNode,
			Tag:  "!!map",
			Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: "builtin"},
				builtinSeq,
				{Kind: yaml.ScalarNode, Value: "custom"},
				customSeq,
			},
		}
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "agents"},
			agentsMap,
		)
	}

	// 3. Add escalation section if absent
	if findMapKey(root, "escalation") == nil {
		triggersSeq := &yaml.Node{
			Kind: yaml.SequenceNode,
			Tag:  "!!seq",
		}
		escalationMap := &yaml.Node{
			Kind: yaml.MappingNode,
			Tag:  "!!map",
			Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: "triggers"},
				triggersSeq,
				{Kind: yaml.ScalarNode, Value: "max_consecutive_failures"},
				{Kind: yaml.ScalarNode, Value: "3", Tag: "!!int"},
				{Kind: yaml.ScalarNode, Value: "retry_count"},
				{Kind: yaml.ScalarNode, Value: "2", Tag: "!!int"},
				{Kind: yaml.ScalarNode, Value: "retry_delay_seconds"},
				{Kind: yaml.ScalarNode, Value: "5", Tag: "!!int"},
				{Kind: yaml.ScalarNode, Value: "max_single_pr_files"},
				{Kind: yaml.ScalarNode, Value: "50", Tag: "!!int"},
				{Kind: yaml.ScalarNode, Value: "max_single_pr_lines"},
				{Kind: yaml.ScalarNode, Value: "500", Tag: "!!int"},
			},
		}
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "escalation"},
			escalationMap,
		)
	}

	// 4. Add feedback section if absent
	if findMapKey(root, "feedback") == nil {
		feedbackMap := &yaml.Node{
			Kind: yaml.MappingNode,
			Tag:  "!!map",
			Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: "enabled"},
				{Kind: yaml.ScalarNode, Value: "true", Tag: "!!bool"},
				{Kind: yaml.ScalarNode, Value: "max_history_in_prompt"},
				{Kind: yaml.ScalarNode, Value: "10", Tag: "!!int"},
			},
		}
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "feedback"},
			feedbackMap,
		)
	}

	// 5. Add hooks section if absent (empty map)
	if findMapKey(root, "hooks") == nil {
		hooksMap := &yaml.Node{
			Kind: yaml.MappingNode,
			Tag:  "!!map",
		}
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "hooks"},
			hooksMap,
		)
	}

	// 6. Add worker section if absent
	if findMapKey(root, "worker") == nil {
		workerMap := &yaml.Node{
			Kind: yaml.MappingNode,
			Tag:  "!!map",
			Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: "backend"},
				{Kind: yaml.ScalarNode, Value: "codex"},
			},
		}
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "worker"},
			workerMap,
		)
	}

	return nil
}
