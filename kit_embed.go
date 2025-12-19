package awkit

import "embed"

// KitFS contains the AWK kit files to be installed into a target project.
//
// NOTE: This embeds only tracked files. Runtime state under `.ai/state/` is ignored by git
// and should not be embedded into release binaries.
//
//go:embed .ai/commands/*.md .ai/config/* .ai/docs/*.md .ai/rules/_kit/*.md .ai/rules/_examples/*.md .ai/scripts/*.sh .ai/scripts/*.py .ai/specs/*/*.md .ai/templates/* .ai/tests/*.sh
var KitFS embed.FS
