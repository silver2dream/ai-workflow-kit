package awkit

import "embed"

// KitFS holds what AWK installs into a project: the kit's rules, skills and
// subagents, at the paths they have in a project (see internal/layout).
//
// This repository is a project of its own, so the kit is its own .claude
// directory. Only what is named here ships. The repository's configuration
// and roster under .ai do not: a project gets those from a preset.
//
//go:embed .claude/rules/_kit/*.md .claude/skills/principal-workflow/*.md .claude/skills/principal-workflow/*/*.md .claude/skills/post-mortem/*.md .claude/skills/post-mortem/*/*.md .claude/skills/release-checklist/*.md .claude/skills/release-checklist/*/*.md .claude/agents/*.md
var KitFS embed.FS
