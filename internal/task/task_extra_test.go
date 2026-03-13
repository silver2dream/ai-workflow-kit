package task

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// --- AppendIssueRef edge cases ---

func TestAppendIssueRef_OutOfRange(t *testing.T) {
	tmpDir := t.TempDir()
	tasksPath := filepath.Join(tmpDir, "tasks.md")
	if err := os.WriteFile(tasksPath, []byte("line1\nline2\n"), 0644); err != nil {
		t.Fatal(err)
	}

	err := AppendIssueRef(tasksPath, 99, 1)
	if err == nil {
		t.Fatal("expected error for out-of-range line")
	}
	if !strings.Contains(err.Error(), "out of range") {
		t.Errorf("error = %q, want 'out of range'", err)
	}
}

func TestAppendIssueRef_AlreadyHasRef(t *testing.T) {
	tmpDir := t.TempDir()
	tasksPath := filepath.Join(tmpDir, "tasks.md")
	content := "- [ ] Task <!-- Issue #42 -->\n"
	if err := os.WriteFile(tasksPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Should be a no-op when ref already exists
	err := AppendIssueRef(tasksPath, 1, 99)
	if err != nil {
		t.Fatalf("AppendIssueRef() should not error when ref already exists: %v", err)
	}

	data, _ := os.ReadFile(tasksPath)
	if strings.Contains(string(data), "Issue #99") {
		t.Error("should not have overwritten existing issue ref")
	}
	if !strings.Contains(string(data), "Issue #42") {
		t.Error("original issue ref should still be present")
	}
}

func TestAppendIssueRef_FileNotFound(t *testing.T) {
	err := AppendIssueRef("/nonexistent/path/tasks.md", 1, 1)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// --- ValidateBody edge cases ---

func TestValidateBody_AllSectionsButNoCheckbox(t *testing.T) {
	body := `## Summary
Test summary
## Scope
Test scope
## Acceptance Criteria
All items are complete (no checkboxes listed)
## Testing Requirements
Test requirements
## Metadata
Test metadata`

	err := ValidateBody(body)
	if err == nil {
		t.Fatal("expected error when no checkbox - [ ] present")
	}
	if !strings.Contains(err.Error(), "checkboxes") {
		t.Errorf("error = %q, want mention of checkboxes", err)
	}
}

func TestValidateBody_MissingMultipleSections(t *testing.T) {
	body := `## Summary
Only summary here`

	err := ValidateBody(body)
	if err == nil {
		t.Fatal("expected error for missing sections")
	}
	// Should mention at least one of the missing sections
	if !strings.Contains(err.Error(), "missing required sections") {
		t.Errorf("error = %q, want 'missing required sections'", err)
	}
}

// --- EnsureAWKMetadata edge cases ---

func TestEnsureAWKMetadata_BothAlreadyPresent(t *testing.T) {
	body := "## Summary\nTest\n\n**Spec**: myspec\n**Task Line**: 7\n"
	result := EnsureAWKMetadata(body, "other-spec", 99)
	// Should return unchanged since both fields already present
	if result != body {
		t.Errorf("EnsureAWKMetadata() should return body unchanged when both fields present\ngot: %q\nwant: %q", result, body)
	}
}

func TestEnsureAWKMetadata_OnlySpecMissing(t *testing.T) {
	body := "## Summary\nTest\n\n**Task Line**: 5\n"
	result := EnsureAWKMetadata(body, "my-spec", 5)
	if !strings.Contains(result, "**Spec**: my-spec") {
		t.Error("result should contain Spec when only Spec was missing")
	}
	// Task Line should NOT be duplicated
	count := strings.Count(result, "**Task Line**")
	if count != 1 {
		t.Errorf("Task Line should appear exactly once, got %d", count)
	}
}

func TestEnsureAWKMetadata_OnlyTaskLineMissing(t *testing.T) {
	body := "## Summary\nTest\n\n**Spec**: myspec\n"
	result := EnsureAWKMetadata(body, "myspec", 42)
	if !strings.Contains(result, "**Task Line**: 42") {
		t.Error("result should contain Task Line when only Task Line was missing")
	}
	// Spec should NOT be duplicated
	count := strings.Count(result, "**Spec**")
	if count != 1 {
		t.Errorf("Spec should appear exactly once, got %d", count)
	}
}

// --- setMapValue / scalarNode ---

func TestSetMapValue_UpdateExisting(t *testing.T) {
	// Build a mapping node with an existing key
	mapping := &yaml.Node{Kind: yaml.MappingNode}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: "mode"}
	valNode := &yaml.Node{Kind: yaml.ScalarNode, Value: "old_value"}
	mapping.Content = append(mapping.Content, keyNode, valNode)

	setMapValue(mapping, "mode", "new_value")

	if len(mapping.Content) != 2 {
		t.Errorf("expected 2 nodes (key+value), got %d", len(mapping.Content))
	}
	if mapping.Content[1].Value != "new_value" {
		t.Errorf("value = %q, want %q", mapping.Content[1].Value, "new_value")
	}
}

func TestSetMapValue_AddNew(t *testing.T) {
	mapping := &yaml.Node{Kind: yaml.MappingNode}

	setMapValue(mapping, "newkey", "newval")

	if len(mapping.Content) != 2 {
		t.Errorf("expected 2 nodes after adding new key, got %d", len(mapping.Content))
	}
	if mapping.Content[0].Value != "newkey" {
		t.Errorf("key = %q, want %q", mapping.Content[0].Value, "newkey")
	}
	if mapping.Content[1].Value != "newval" {
		t.Errorf("value = %q, want %q", mapping.Content[1].Value, "newval")
	}
}

func TestSetMapValue_IntValue(t *testing.T) {
	mapping := &yaml.Node{Kind: yaml.MappingNode}

	setMapValue(mapping, "count", 42)

	if len(mapping.Content) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(mapping.Content))
	}
	if mapping.Content[1].Value != "42" {
		t.Errorf("value = %q, want %q", mapping.Content[1].Value, "42")
	}
	if mapping.Content[1].Tag != "!!int" {
		t.Errorf("tag = %q, want !!int", mapping.Content[1].Tag)
	}
}

func TestScalarNode_Int(t *testing.T) {
	node := scalarNode(123)
	if node.Kind != yaml.ScalarNode {
		t.Errorf("kind = %v, want ScalarNode", node.Kind)
	}
	if node.Value != "123" {
		t.Errorf("value = %q, want %q", node.Value, "123")
	}
	if node.Tag != "!!int" {
		t.Errorf("tag = %q, want !!int", node.Tag)
	}
}

func TestScalarNode_String(t *testing.T) {
	node := scalarNode("hello")
	if node.Value != "hello" {
		t.Errorf("value = %q, want %q", node.Value, "hello")
	}
}

func TestScalarNode_OtherType(t *testing.T) {
	// Default case: float, bool, etc. converted via Sprintf
	node := scalarNode(3.14)
	if node.Value == "" {
		t.Error("expected non-empty value for float")
	}
}

// --- DefaultIssueTitle whitespace normalization ---

func TestDefaultIssueTitle_WhitespaceNormalized(t *testing.T) {
	title := DefaultIssueTitle("  fix   multiple   spaces  ")
	if !strings.HasPrefix(title, "[feat] ") {
		t.Errorf("title = %q, should start with [feat]", title)
	}
	if strings.Contains(title, "  ") {
		t.Errorf("title = %q, should not contain double spaces", title)
	}
}
