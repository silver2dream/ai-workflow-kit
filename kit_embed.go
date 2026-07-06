package awkit

import "embed"

// KitFS contains the AWK kit files to be installed into a target project.
//
// NOTE: This embeds only tracked files. Runtime state under `.ai/state/` is ignored by git
// and should not be embedded into release binaries.
//
// Specs: only the example spec ships to user projects. AWK's own design specs
// (.ai/specs/awk-evolution, learning-loop, ...) are kit-internal documents;
// stale copies installed by older releases are removed via
// internal/install/deprecated.txt.
//
//go:embed .ai/config/* .ai/rules/_kit/*.md .ai/rules/_examples/*.md .ai/skills/principal-workflow/*.md .ai/skills/principal-workflow/*/*.md .ai/skills/post-mortem/*.md .ai/skills/post-mortem/*/*.md .ai/skills/release-checklist/*.md .ai/skills/release-checklist/*/*.md .ai/specs/example/*.md .ai/templates/*.example .claude/agents/*.md
var KitFS embed.FS
