package generate

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	awkit "github.com/silver2dream/ai-workflow-kit"
)

// Built-in agent definitions have one source of truth: the files the kit ships
// under .claude/agents/. generate used to carry its own copies, which drifted
// and then overwrote the current definitions on every install/generate.
func TestInstallAgentsDir_WritesKitDefinitions(t *testing.T) {
	agentsDir := filepath.Join(t.TempDir(), "agents")
	if err := installAgentsDir(agentsDir, AgentsConfig{}); err != nil {
		t.Fatalf("installAgentsDir() error = %v", err)
	}

	for _, name := range builtinAgentFiles {
		want, err := awkit.KitFS.ReadFile(".claude/agents/" + name)
		if err != nil {
			t.Fatalf("kit is missing %s: %v", name, err)
		}
		got, err := os.ReadFile(filepath.Join(agentsDir, name))
		if err != nil {
			t.Fatalf("generated %s is missing: %v", name, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("generated %s differs from the kit's .claude/agents/%s", name, name)
		}
	}
}
