package skills

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// SkillValidationError.Error() (skills.go)
// ---------------------------------------------------------------------------

func TestSkillValidationError_Error(t *testing.T) {
	err := &SkillValidationError{Message: "test error message"}
	if err.Error() != "test error message" {
		t.Errorf("Error() = %q, want 'test error message'", err.Error())
	}
}

func TestSkillValidationError_EmptyMessage(t *testing.T) {
	err := &SkillValidationError{Message: ""}
	if err.Error() != "" {
		t.Errorf("Error() = %q, want empty string", err.Error())
	}
}

// ---------------------------------------------------------------------------
// ValidateFrontmatter (skills.go) - missing name, description, allowed-tools
// ---------------------------------------------------------------------------

func TestValidateFrontmatter_MissingName(t *testing.T) {
	dir := t.TempDir()
	skillFile := filepath.Join(dir, "SKILL.md")
	content := `---
description: "A skill"
allowed-tools: "Bash"
---
# Content
`
	os.WriteFile(skillFile, []byte(content), 0644)
	err := ValidateFrontmatter(skillFile)
	if err == nil {
		t.Error("ValidateFrontmatter with missing name should return error")
	}
}

func TestValidateFrontmatter_MissingDescription(t *testing.T) {
	dir := t.TempDir()
	skillFile := filepath.Join(dir, "SKILL.md")
	content := `---
name: "my-skill"
allowed-tools: "Bash"
---
# Content
`
	os.WriteFile(skillFile, []byte(content), 0644)
	err := ValidateFrontmatter(skillFile)
	if err == nil {
		t.Error("ValidateFrontmatter with missing description should return error")
	}
}

func TestValidateFrontmatter_MissingAllowedTools(t *testing.T) {
	dir := t.TempDir()
	skillFile := filepath.Join(dir, "SKILL.md")
	content := `---
name: "my-skill"
description: "A skill"
---
# Content
`
	os.WriteFile(skillFile, []byte(content), 0644)
	err := ValidateFrontmatter(skillFile)
	if err == nil {
		t.Error("ValidateFrontmatter with missing allowed-tools should return error")
	}
}

func TestValidateFrontmatter_FileNotFound(t *testing.T) {
	err := ValidateFrontmatter("/nonexistent/SKILL.md")
	if err == nil {
		t.Error("ValidateFrontmatter with missing file should return error")
	}
}

// ---------------------------------------------------------------------------
// ValidateMainLoop (skills.go)
// ---------------------------------------------------------------------------

func TestValidateMainLoop_MissingContent(t *testing.T) {
	dir := t.TempDir()
	mainLoopFile := filepath.Join(dir, "main-loop.md")
	// Write content that lacks required patterns
	os.WriteFile(mainLoopFile, []byte("# Main Loop\n\nSome content without required patterns.\n"), 0644)
	err := ValidateMainLoop(mainLoopFile)
	if err == nil {
		t.Error("ValidateMainLoop with missing required content should return error")
	}
}

func TestValidateMainLoop_FileNotFound(t *testing.T) {
	err := ValidateMainLoop("/nonexistent/main-loop.md")
	if err == nil {
		t.Error("ValidateMainLoop with missing file should return error")
	}
}

// ---------------------------------------------------------------------------
// ValidateContracts (skills.go)
// ---------------------------------------------------------------------------

func TestValidateContracts_MissingActions(t *testing.T) {
	dir := t.TempDir()
	contractsFile := filepath.Join(dir, "contracts.md")
	// Write content lacking required actions
	os.WriteFile(contractsFile, []byte("# Contracts\n\nJust some text.\n"), 0644)
	err := ValidateContracts(contractsFile)
	if err == nil {
		t.Error("ValidateContracts with missing actions should return error")
	}
}

func TestValidateContracts_FileNotFound(t *testing.T) {
	err := ValidateContracts("/nonexistent/contracts.md")
	if err == nil {
		t.Error("ValidateContracts with missing file should return error")
	}
}

// ---------------------------------------------------------------------------
// ParseSkillMetadata (skills.go) - edge cases
// ---------------------------------------------------------------------------

func TestParseSkillMetadata_NoFrontmatter(t *testing.T) {
	dir := t.TempDir()
	skillFile := filepath.Join(dir, "SKILL.md")
	os.WriteFile(skillFile, []byte("# No frontmatter here\n"), 0644)
	// Should not panic, may return empty or error
	meta, err := ParseSkillMetadata(skillFile)
	_ = meta
	_ = err // either outcome is acceptable
}

func TestParseSkillMetadata_FileNotFound(t *testing.T) {
	_, err := ParseSkillMetadata("/nonexistent/SKILL.md")
	if err == nil {
		t.Error("ParseSkillMetadata with missing file should return error")
	}
}
