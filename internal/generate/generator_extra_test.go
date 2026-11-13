package generate

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// generateValidateSubmodulesWorkflow — pure string builder
// ---------------------------------------------------------------------------

func TestGenerateValidateSubmodulesWorkflow_ContainsYAML(t *testing.T) {
	git := GitConfig{
		IntegrationBranch: "develop",
		ReleaseBranch:     "main",
	}
	content := generateValidateSubmodulesWorkflow(git)
	if !strings.Contains(content, "validate-submodules") {
		t.Errorf("expected validate-submodules in content, got:\n%s", content)
	}
	if !strings.Contains(content, "develop") {
		t.Errorf("expected integration branch 'develop', got:\n%s", content)
	}
	if !strings.Contains(content, "main") {
		t.Errorf("expected release branch 'main', got:\n%s", content)
	}
}

func TestGenerateValidateSubmodulesWorkflow_SameBranch(t *testing.T) {
	git := GitConfig{
		IntegrationBranch: "main",
		ReleaseBranch:     "main",
	}
	content := generateValidateSubmodulesWorkflow(git)
	if !strings.Contains(content, "main") {
		t.Error("expected 'main' in content")
	}
}

func TestGenerateValidateSubmodulesWorkflow_EmptyReleaseBranch(t *testing.T) {
	git := GitConfig{
		IntegrationBranch: "develop",
		ReleaseBranch:     "",
	}
	content := generateValidateSubmodulesWorkflow(git)
	if !strings.Contains(content, "develop") {
		t.Error("expected 'develop' in content")
	}
}
