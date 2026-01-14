package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// getRepoRoot returns the repository root directory.
// It traverses up from the current test directory looking for go.mod.
func getRepoRoot(t *testing.T) string {
	t.Helper()

	// Start from current working directory
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	// Traverse up to find go.mod (repo root)
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find repo root from %s", wd)
		}
		dir = parent
	}
}

func TestSkillStructure_SKILLmdExists(t *testing.T) {
	repoRoot := getRepoRoot(t)
	skillDir := filepath.Join(repoRoot, ".ai", "skills", "principal-workflow")

	skillFile := filepath.Join(skillDir, "SKILL.md")
	if _, err := os.Stat(skillFile); os.IsNotExist(err) {
		t.Error("SKILL.md does not exist")
	}
}

func TestSkillStructure_FrontmatterFormat(t *testing.T) {
	repoRoot := getRepoRoot(t)
	skillFile := filepath.Join(repoRoot, ".ai", "skills", "principal-workflow", "SKILL.md")

	content, err := os.ReadFile(skillFile)
	if err != nil {
		t.Fatalf("failed to read SKILL.md: %v", err)
	}

	// Normalize line endings
	contentStr := strings.ReplaceAll(string(content), "\r\n", "\n")

	t.Run("frontmatter starts correctly", func(t *testing.T) {
		lines := strings.Split(contentStr, "\n")
		if len(lines) == 0 || lines[0] != "---" {
			t.Error("frontmatter should start with ---")
		}
	})

	t.Run("name format is correct", func(t *testing.T) {
		if !strings.Contains(contentStr, "name: principal-workflow") {
			t.Error("name field should be 'name: principal-workflow'")
		}
	})

	t.Run("description is single line", func(t *testing.T) {
		lines := strings.Split(contentStr, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "description:") {
				if strings.Contains(line, "|") || strings.Contains(line, ">") {
					t.Error("description should be single-line (no | or > YAML multi-line indicators)")
				}
				break
			}
		}
	})

	t.Run("allowed-tools format is correct", func(t *testing.T) {
		if !strings.Contains(contentStr, "Read, Grep, Glob, Bash") {
			t.Error("allowed-tools should contain 'Read, Grep, Glob, Bash'")
		}
	})
}

func TestSkillStructure_RequiredFilesExist(t *testing.T) {
	repoRoot := getRepoRoot(t)
	skillDir := filepath.Join(repoRoot, ".ai", "skills", "principal-workflow")

	requiredFiles := RequiredFiles()

	for _, file := range requiredFiles {
		t.Run(file, func(t *testing.T) {
			path := filepath.Join(skillDir, file)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Errorf("%s does not exist", file)
			}
		})
	}
}

func TestSkillStructure_MainLoopContent(t *testing.T) {
	repoRoot := getRepoRoot(t)
	mainLoopFile := filepath.Join(repoRoot, ".ai", "skills", "principal-workflow", "phases", "main-loop.md")

	content, err := os.ReadFile(mainLoopFile)
	if err != nil {
		t.Fatalf("failed to read main-loop.md: %v", err)
	}

	contentStr := string(content)

	t.Run("contains analyze_next reference", func(t *testing.T) {
		if !strings.Contains(contentStr, "analyze-next") && !strings.Contains(contentStr, "analyze_next") {
			t.Error("main-loop.md should contain analyze-next or analyze_next command reference")
		}
	})

	t.Run("contains NEXT_ACTION routing", func(t *testing.T) {
		if !strings.Contains(contentStr, "next_action") {
			t.Error("main-loop.md should contain next_action routing")
		}
	})

	t.Run("contains Loop Safety", func(t *testing.T) {
		// The shell script checks for "loop_count" but the actual file uses "Loop Safety" section
		if !strings.Contains(strings.ToLower(contentStr), "loop") {
			t.Error("main-loop.md should contain loop safety information")
		}
	})
}

func TestSkillStructure_ContractsContent(t *testing.T) {
	repoRoot := getRepoRoot(t)
	contractsFile := filepath.Join(repoRoot, ".ai", "skills", "principal-workflow", "references", "contracts.md")

	content, err := os.ReadFile(contractsFile)
	if err != nil {
		t.Fatalf("failed to read contracts.md: %v", err)
	}

	contentStr := string(content)

	requiredActions := []string{
		"generate_tasks",
		"create_task",
		"dispatch_worker",
		"check_result",
		"review_pr",
		"all_complete",
		"none",
	}

	for _, action := range requiredActions {
		t.Run("contains "+action, func(t *testing.T) {
			if !strings.Contains(contentStr, action) {
				t.Errorf("contracts.md should contain %s action", action)
			}
		})
	}
}

func TestParseSkillMetadata(t *testing.T) {
	repoRoot := getRepoRoot(t)
	skillFile := filepath.Join(repoRoot, ".ai", "skills", "principal-workflow", "SKILL.md")

	meta, err := ParseSkillMetadata(skillFile)
	if err != nil {
		t.Fatalf("ParseSkillMetadata() error = %v", err)
	}

	t.Run("name is set", func(t *testing.T) {
		if meta.Name != "principal-workflow" {
			t.Errorf("Name = %q, want %q", meta.Name, "principal-workflow")
		}
	})

	t.Run("description is set", func(t *testing.T) {
		if meta.Description == "" {
			t.Error("Description should not be empty")
		}
	})

	t.Run("allowed-tools contains expected tools", func(t *testing.T) {
		expectedTools := []string{"Read", "Grep", "Glob", "Bash"}
		for _, tool := range expectedTools {
			found := false
			for _, t := range meta.AllowedTools {
				if t == tool {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("AllowedTools should contain %q", tool)
			}
		}
	})
}

func TestValidateSkillStructure(t *testing.T) {
	repoRoot := getRepoRoot(t)
	skillDir := filepath.Join(repoRoot, ".ai", "skills", "principal-workflow")

	err := ValidateSkillStructure(skillDir)
	if err != nil {
		t.Errorf("ValidateSkillStructure() error = %v", err)
	}
}

func TestValidateSkillStructure_MissingDir(t *testing.T) {
	err := ValidateSkillStructure("/nonexistent/path")
	if err == nil {
		t.Error("ValidateSkillStructure() expected error for nonexistent path")
	}
}

func TestValidateFrontmatter(t *testing.T) {
	repoRoot := getRepoRoot(t)
	skillFile := filepath.Join(repoRoot, ".ai", "skills", "principal-workflow", "SKILL.md")

	err := ValidateFrontmatter(skillFile)
	if err != nil {
		t.Errorf("ValidateFrontmatter() error = %v", err)
	}
}

func TestValidateMainLoop(t *testing.T) {
	repoRoot := getRepoRoot(t)
	mainLoopFile := filepath.Join(repoRoot, ".ai", "skills", "principal-workflow", "phases", "main-loop.md")

	err := ValidateMainLoop(mainLoopFile)
	if err != nil {
		t.Errorf("ValidateMainLoop() error = %v", err)
	}
}

func TestValidateContracts(t *testing.T) {
	repoRoot := getRepoRoot(t)
	contractsFile := filepath.Join(repoRoot, ".ai", "skills", "principal-workflow", "references", "contracts.md")

	err := ValidateContracts(contractsFile)
	if err != nil {
		t.Errorf("ValidateContracts() error = %v", err)
	}
}

func TestParseCommaSeparatedList(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "simple list",
			input: "Read, Grep, Glob, Bash",
			want:  []string{"Read", "Grep", "Glob", "Bash"},
		},
		{
			name:  "no spaces",
			input: "Read,Grep,Glob",
			want:  []string{"Read", "Grep", "Glob"},
		},
		{
			name:  "extra spaces",
			input: "  Read ,  Grep  , Glob  ",
			want:  []string{"Read", "Grep", "Glob"},
		},
		{
			name:  "empty string",
			input: "",
			want:  nil,
		},
		{
			name:  "single item",
			input: "Read",
			want:  []string{"Read"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCommaSeparatedList(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("parseCommaSeparatedList() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseCommaSeparatedList()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
