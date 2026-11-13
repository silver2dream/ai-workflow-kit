package install

import (
	"embed"
	"io/fs"
	"path"
	"path/filepath"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/layout"
)

// starterRulesFS holds the rules the projects of a preset start with, as
// starter-rules/<preset>/<name>.md. They say how code of that stack is
// written. Unlike the kit's rules they are the project's own from the moment
// they are written: an upgrade does not touch them, and the project is
// expected to tighten them as its architecture settles.
//
//go:embed starter-rules
var starterRulesFS embed.FS

const starterRulesRoot = "starter-rules"

// StarterRules lists the names of the rules the projects of a preset start
// with, in the order of their names (fs.ReadDir sorts). A preset that assumes
// no stack has none.
func StarterRules(preset string) []string {
	entries, err := fs.ReadDir(starterRulesFS, path.Join(starterRulesRoot, presetName(preset)))
	if err != nil {
		return nil
	}
	var names []string
	for _, entry := range entries {
		if name := entry.Name(); !entry.IsDir() && strings.HasSuffix(name, ".md") {
			names = append(names, strings.TrimSuffix(name, ".md"))
		}
	}
	return names
}

// starterRule returns the text of one starter rule of a preset.
func starterRule(preset, name string) ([]byte, error) {
	return starterRulesFS.ReadFile(path.Join(starterRulesRoot, presetName(preset), name+".md"))
}

// ensureStarterRules writes the starter rules of a preset into a project. A
// rule file that is there is the project's own and is kept.
func ensureStarterRules(targetDir, preset string) error {
	for _, name := range StarterRules(preset) {
		data, err := starterRule(preset, name)
		if err != nil {
			return err
		}
		file := filepath.Join(layout.Rules(targetDir), name+".md")
		if _, _, err := writeFileIfNotExists(file, data, false); err != nil {
			return err
		}
	}
	return nil
}
