package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractSpecFromSource(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{"empty source", "", ""},
		{"audit source", "audit:finding-1", ""},
		{"tasks.md source", "tasks.md #5", ""},
		{"plain spec name", "my-project", "my-project"},
		{"spec with spaces trimmed", "  my-project  ", "my-project"},
		{"path traversal blocked", "../evil", ""},
		{"slash blocked", "path/to/spec", ""},
		{"backslash blocked", "path\\to\\spec", ""},
		{"dots blocked", "some..name", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractSpecFromSource(tt.source)
			if result != tt.expected {
				t.Errorf("extractSpecFromSource(%q) = %q, want %q", tt.source, result, tt.expected)
			}
		})
	}
}

func TestResolveDesignDoc(t *testing.T) {
	t.Run("no_spec_name_returns_empty", func(t *testing.T) {
		tmpDir := t.TempDir()
		ticket := "# Task\n- Repo: backend\n- Severity: P1\n"
		result := resolveDesignDoc(tmpDir, ticket, nil)
		if result != "" {
			t.Errorf("expected empty, got %q", result)
		}
	})

	t.Run("spec_from_ticket_metadata", func(t *testing.T) {
		tmpDir := t.TempDir()
		specDir := filepath.Join(tmpDir, ".ai", "specs", "my-feature")
		if err := os.MkdirAll(specDir, 0755); err != nil {
			t.Fatal(err)
		}
		designContent := "# Design\n\nThis is the design doc."
		if err := os.WriteFile(filepath.Join(specDir, "design.md"), []byte(designContent), 0644); err != nil {
			t.Fatal(err)
		}

		ticket := "# Task\n- Repo: backend\n- Spec: my-feature\n"
		result := resolveDesignDoc(tmpDir, ticket, nil)
		if result != designContent {
			t.Errorf("expected design content, got %q", result)
		}
	})

	t.Run("spec_from_env_variable", func(t *testing.T) {
		tmpDir := t.TempDir()
		specDir := filepath.Join(tmpDir, ".ai", "specs", "env-spec")
		if err := os.MkdirAll(specDir, 0755); err != nil {
			t.Fatal(err)
		}
		designContent := "# Env Spec Design"
		if err := os.WriteFile(filepath.Join(specDir, "design.md"), []byte(designContent), 0644); err != nil {
			t.Fatal(err)
		}

		t.Setenv("AI_SPEC_NAME", "env-spec")
		ticket := "# Task\n- Repo: backend\n"
		result := resolveDesignDoc(tmpDir, ticket, nil)
		if result != designContent {
			t.Errorf("expected design content from env spec, got %q", result)
		}
	})

	t.Run("design_file_not_found_returns_empty", func(t *testing.T) {
		tmpDir := t.TempDir()
		ticket := "# Task\n- Spec: nonexistent-spec\n"
		result := resolveDesignDoc(tmpDir, ticket, nil)
		if result != "" {
			t.Errorf("expected empty for missing design file, got %q", result)
		}
	})

	t.Run("content_capped_at_4000_chars", func(t *testing.T) {
		tmpDir := t.TempDir()
		specDir := filepath.Join(tmpDir, ".ai", "specs", "big-spec")
		if err := os.MkdirAll(specDir, 0755); err != nil {
			t.Fatal(err)
		}
		// Create content larger than 4000 chars
		longContent := strings.Repeat("a", 5000)
		if err := os.WriteFile(filepath.Join(specDir, "design.md"), []byte(longContent), 0644); err != nil {
			t.Fatal(err)
		}

		ticket := "# Task\n- Spec: big-spec\n"
		result := resolveDesignDoc(tmpDir, ticket, nil)
		if len(result) > maxDesignDocChars+100 { // allow some room for truncation message
			t.Errorf("expected content to be capped, got length %d", len(result))
		}
		if !strings.Contains(result, "[... truncated at 4000 characters ...]") {
			t.Error("expected truncation message")
		}
	})

	t.Run("custom_specs_config", func(t *testing.T) {
		tmpDir := t.TempDir()
		specDir := filepath.Join(tmpDir, "custom", "specs", "my-feature")
		if err := os.MkdirAll(specDir, 0755); err != nil {
			t.Fatal(err)
		}
		designContent := "# Custom Design"
		if err := os.WriteFile(filepath.Join(specDir, "overview.md"), []byte(designContent), 0644); err != nil {
			t.Fatal(err)
		}

		ticket := "# Task\n- Spec: my-feature\n"
		cfg := &workflowSpecs{
			BasePath: "custom/specs",
			Files:    workflowSpecFiles{Design: "overview.md"},
		}
		result := resolveDesignDoc(tmpDir, ticket, cfg)
		if result != designContent {
			t.Errorf("expected custom design content, got %q", result)
		}
	})

	t.Run("path_traversal_blocked", func(t *testing.T) {
		tmpDir := t.TempDir()
		// Create a file outside the specs directory
		secretFile := filepath.Join(tmpDir, "secret.txt")
		if err := os.WriteFile(secretFile, []byte("secret data"), 0644); err != nil {
			t.Fatal(err)
		}

		// Attempt to use path traversal via spec name — should be blocked by ParseTicketMetadata
		ticket := "# Task\n- Spec: ../../secret\n"
		result := resolveDesignDoc(tmpDir, ticket, nil)
		if result != "" {
			t.Errorf("expected empty for path traversal attempt, got %q", result)
		}
	})

	t.Run("source_field_plain_spec_name", func(t *testing.T) {
		tmpDir := t.TempDir()
		specDir := filepath.Join(tmpDir, ".ai", "specs", "from-source")
		if err := os.MkdirAll(specDir, 0755); err != nil {
			t.Fatal(err)
		}
		designContent := "# Source-derived Design"
		if err := os.WriteFile(filepath.Join(specDir, "design.md"), []byte(designContent), 0644); err != nil {
			t.Fatal(err)
		}

		// Source is a plain spec name (not audit: or tasks.md)
		ticket := "# Task\n- Source: from-source\n"
		result := resolveDesignDoc(tmpDir, ticket, nil)
		if result != designContent {
			t.Errorf("expected design content from source field, got %q", result)
		}
	})

	t.Run("audit_source_not_used_as_spec", func(t *testing.T) {
		tmpDir := t.TempDir()
		ticket := "# Task\n- Source: audit:finding-1\n"
		result := resolveDesignDoc(tmpDir, ticket, nil)
		if result != "" {
			t.Errorf("expected empty for audit source, got %q", result)
		}
	})
}

func TestWritePromptFileWithDesignDoc(t *testing.T) {
	t.Run("prompt_includes_design_context", func(t *testing.T) {
		tmpDir := t.TempDir()
		specDir := filepath.Join(tmpDir, ".ai", "specs", "test-spec")
		if err := os.MkdirAll(specDir, 0755); err != nil {
			t.Fatal(err)
		}
		designContent := "# Test Design\nSome design details."
		if err := os.WriteFile(filepath.Join(specDir, "design.md"), []byte(designContent), 0644); err != nil {
			t.Fatal(err)
		}

		promptPath := filepath.Join(tmpDir, "prompt.txt")
		ticket := "# Task\n- Spec: test-spec\n- Repo: backend\n"
		err := writePromptFile(promptPath, "", ticket, tmpDir, 999, 0, 0, nil, nil)
		if err != nil {
			t.Fatal(err)
		}

		data, err := os.ReadFile(promptPath)
		if err != nil {
			t.Fatal(err)
		}
		prompt := string(data)

		if !strings.Contains(prompt, "DESIGN CONTEXT") {
			t.Error("expected prompt to contain DESIGN CONTEXT header")
		}
		if !strings.Contains(prompt, designContent) {
			t.Error("expected prompt to contain design doc content")
		}
		// Design context should appear before the ticket
		designIdx := strings.Index(prompt, "DESIGN CONTEXT")
		ticketIdx := strings.Index(prompt, "Ticket:")
		if designIdx > ticketIdx {
			t.Error("expected DESIGN CONTEXT to appear before Ticket")
		}
	})

	t.Run("prompt_without_design_doc", func(t *testing.T) {
		tmpDir := t.TempDir()
		promptPath := filepath.Join(tmpDir, "prompt.txt")
		ticket := "# Task\n- Repo: backend\n"
		err := writePromptFile(promptPath, "", ticket, tmpDir, 999, 0, 0, nil, nil)
		if err != nil {
			t.Fatal(err)
		}

		data, err := os.ReadFile(promptPath)
		if err != nil {
			t.Fatal(err)
		}
		prompt := string(data)

		if strings.Contains(prompt, "DESIGN CONTEXT") {
			t.Error("expected prompt NOT to contain DESIGN CONTEXT when no spec")
		}
	})
}
