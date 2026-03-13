package upgrade

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// UpgradeAgents (permissions.go)
// ---------------------------------------------------------------------------

func TestUpgradeAgents_DryRun_MissingAgents(t *testing.T) {
	dir := t.TempDir()
	// No agents dir — all agents missing
	result := UpgradeAgents(dir, true /* dryRun */)
	if !result.Success {
		t.Error("UpgradeAgents(dryRun) should succeed")
	}
	if len(result.Created) == 0 {
		t.Error("UpgradeAgents(dryRun) should list missing agents to create")
	}
}

func TestUpgradeAgents_CreatesMissingAgents(t *testing.T) {
	dir := t.TempDir()
	result := UpgradeAgents(dir, false /* not dryRun */)
	if !result.Success {
		t.Errorf("UpgradeAgents should succeed, message: %q", result.Message)
	}

	agentsDir := filepath.Join(dir, ".claude", "agents")
	// Check that agent files were created
	for _, name := range result.Created {
		path := filepath.Join(agentsDir, name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("agent file %q not created: %v", name, err)
		}
	}
}

func TestUpgradeAgents_AllPresent_Skipped(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, ".claude", "agents")
	os.MkdirAll(agentsDir, 0755)

	// Pre-create both agent files
	for _, name := range []string{"pr-reviewer.md", "conflict-resolver.md"} {
		os.WriteFile(filepath.Join(agentsDir, name), []byte("existing"), 0644)
	}

	result := UpgradeAgents(dir, false)
	if !result.Success {
		t.Errorf("UpgradeAgents with all present should succeed, message: %q", result.Message)
	}
	if !result.Skipped {
		t.Error("UpgradeAgents with all present should be Skipped=true")
	}
}
