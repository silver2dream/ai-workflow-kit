package awkit

import "embed"

// KitFS contains the AWK kit files to be installed into a target project.
//
// NOTE: This embeds only tracked files. Runtime state under `.ai/state/` is ignored by git
// and should not be embedded into release binaries.
//
// Specs: none ship to user projects. What is under .ai/specs in this
// repository are AWK's own design documents; copies that older releases
// installed are removed via internal/install/deprecated.txt.
//
//go:embed .ai/config/* .ai/rules/_kit/*.md .ai/rules/_examples/*.md .ai/skills/principal-workflow/*.md .ai/skills/principal-workflow/*/*.md .ai/skills/post-mortem/*.md .ai/skills/post-mortem/*/*.md .ai/skills/release-checklist/*.md .ai/skills/release-checklist/*/*.md .ai/templates/*.example .claude/agents/*.md
var KitFS embed.FS
