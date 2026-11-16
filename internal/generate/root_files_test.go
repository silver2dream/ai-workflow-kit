package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/layout"
)

// The repository's own CLAUDE.md, AGENTS.md and git-workflow rule file are
// generator output for the kit's .ai/config/workflow.yaml. Edited by hand they
// drifted from what user projects receive (a merged ticket-format line, a
// duplicated section number, sections the generator never emitted). This test
// keeps them regenerable: change the generator, then run `awkit generate`.
func TestRepoInstructionFilesMatchGenerator(t *testing.T) {
	repoRoot := filepath.Join("..", "..")

	config, err := loadConfig(layout.Config(repoRoot))
	if err != nil {
		t.Fatalf("load the kit's own workflow.yaml: %v", err)
	}
	ctx := buildContext(config)

	files := []struct {
		repoPath string
		generate func(string, *TemplateContext) error
	}{
		{"CLAUDE.md", generateClaudeMd},
		{"AGENTS.md", generateAgentsMd},
		{filepath.Join(layout.KitRules(""), "git-workflow.md"), generateGitWorkflowMd},
	}

	for _, f := range files {
		out := filepath.Join(t.TempDir(), filepath.Base(f.repoPath))
		if err := f.generate(out, ctx); err != nil {
			t.Fatalf("generate %s: %v", f.repoPath, err)
		}
		want := readNormalized(t, out)
		got := readNormalized(t, filepath.Join(repoRoot, f.repoPath))
		if got != want {
			t.Errorf("%s is out of sync with the generator; run `awkit generate` in the repository root and commit the result", f.repoPath)
		}
	}
}

// readNormalized reads a file with CRLF line endings folded to LF, so a
// Windows checkout compares equal to the generator's output.
func readNormalized(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}
