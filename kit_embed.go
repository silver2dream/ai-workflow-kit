package awkit

import "embed"

// KitFS contains the AWK kit files to be installed into a target project.
//
// NOTE: This embeds only tracked files. Runtime state under `.ai/state/` is ignored by git
// and should not be embedded into release binaries.
//
//go:embed .ai/config/* .ai/docs/*.md .ai/rules/_kit/*.md .ai/rules/_examples/*.md .ai/skills/principal-workflow/*.md .ai/skills/principal-workflow/*/*.md .ai/specs/*/*.md .ai/templates/* .ai/tests/*.sh .ai/tests/*.py .ai/tests/*.ini .ai/tests/fixtures/* .ai/tests/unit/*.py
var KitFS embed.FS
