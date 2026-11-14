package migrate

import (
	"gopkg.in/yaml.v3"
)

// migrationV1_1ToV1_2 migrates config from v1.1 to v1.2.
//
// Version 1.2 is the one that reads the sections agents, escalation,
// feedback, hooks and worker. A file that leaves a section out has its
// defaults, so the migration writes none of them and only the version
// changes. It used to fill the sections in, and half of what it wrote were
// keys nothing reads: triggers and size limits under escalation, a history
// length under feedback.
var migrationV1_1ToV1_2 = Migration{
	FromVersion: "1.1",
	ToVersion:   "1.2",
	Description: "agents, escalation, feedback, hooks and worker are read; a section that is left out has its defaults",
	Apply:       applyV1_1ToV1_2,
}

func applyV1_1ToV1_2(doc *yaml.Node) error {
	setScalarValue(doc.Content[0], "version", "1.2")
	return nil
}
