package upgrade

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// makeSettingsDir creates a temp dir with .claude/settings.local.json.
func makeSettingsDir(t *testing.T, allow []string) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "upgrade_test_*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	settings := map[string]interface{}{
		"permissions": map[string]interface{}{
			"allow": allow,
		},
	}
	data, _ := json.MarshalIndent(settings, "", "  ")
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.local.json"), data, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return dir
}

func TestCheckPermissions_AllPresent_ReturnsEmpty(t *testing.T) {
	dir := makeSettingsDir(t, RequiredTaskPermissions)
	missing := CheckPermissions(dir)
	if len(missing) != 0 {
		t.Errorf("expected no missing permissions, got %v", missing)
	}
}

func TestCheckPermissions_AllMissing_ReturnsAll(t *testing.T) {
	dir := makeSettingsDir(t, []string{})
	missing := CheckPermissions(dir)
	if len(missing) != len(RequiredTaskPermissions) {
		t.Errorf("expected %d missing, got %d: %v", len(RequiredTaskPermissions), len(missing), missing)
	}
}

func TestCheckPermissions_FileNotFound_ReturnsAllRequired(t *testing.T) {
	dir, _ := os.MkdirTemp("", "upgrade_test_nofile_*")
	defer os.RemoveAll(dir)
	missing := CheckPermissions(dir)
	if len(missing) != len(RequiredTaskPermissions) {
		t.Errorf("expected all required permissions missing when no file, got %v", missing)
	}
}

func TestUpgradePermissions_DryRun_DoesNotWrite(t *testing.T) {
	dir := makeSettingsDir(t, []string{})
	result := UpgradePermissions(dir, true /* dryRun */)
	if !result.Success {
		t.Errorf("expected success, got message: %s", result.Message)
	}
	if result.Skipped {
		t.Error("expected not skipped (permissions missing)")
	}
	if len(result.Added) == 0 {
		t.Error("expected Added to list would-be additions")
	}
	// Verify file not modified: read back and confirm still empty allow.
	missing := CheckPermissions(dir)
	if len(missing) == 0 {
		t.Error("dry-run should not have modified the file")
	}
}

func TestUpgradePermissions_WritesPermissions(t *testing.T) {
	dir := makeSettingsDir(t, []string{})
	result := UpgradePermissions(dir, false)
	if !result.Success {
		t.Fatalf("UpgradePermissions failed: %s", result.Message)
	}
	if len(result.Added) == 0 {
		t.Error("expected at least one permission to be added")
	}
	// Verify file was updated.
	missing := CheckPermissions(dir)
	if len(missing) != 0 {
		t.Errorf("after upgrade, still missing: %v", missing)
	}
}

func TestUpgradePermissions_AlreadyPresent_Skipped(t *testing.T) {
	dir := makeSettingsDir(t, RequiredTaskPermissions)
	result := UpgradePermissions(dir, false)
	if !result.Success {
		t.Fatalf("unexpected failure: %s", result.Message)
	}
	if !result.Skipped {
		t.Error("expected Skipped=true when all permissions already present")
	}
}

func TestCheckAgents_AllMissing(t *testing.T) {
	dir, _ := os.MkdirTemp("", "upgrade_test_agents_*")
	defer os.RemoveAll(dir)
	missing := CheckAgents(dir)
	if len(missing) == 0 {
		t.Error("expected agents to be missing in empty dir")
	}
}

func TestCheckAgents_AllPresent(t *testing.T) {
	dir, _ := os.MkdirTemp("", "upgrade_test_agents_present_*")
	defer os.RemoveAll(dir)
	agentsDir := filepath.Join(dir, ".claude", "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	for _, name := range []string{"pr-reviewer.md", "conflict-resolver.md"} {
		if err := os.WriteFile(filepath.Join(agentsDir, name), []byte("---\nname: "+name+"\n---\n"), 0644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}
	missing := CheckAgents(dir)
	if len(missing) != 0 {
		t.Errorf("expected no missing agents, got %v", missing)
	}
}
