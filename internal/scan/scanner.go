package scan

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ScanResult contains the results of scanning a repository
type ScanResult struct {
	Language      string `json:"language"`        // "go", "unity", "unknown"
	TestFileCount int    `json:"test_file_count"` // count of test files
	HasSubmodules bool   `json:"has_submodules"`  // true if .gitmodules exists
	Branch        string `json:"branch"`          // current git branch
	HasClaudeMD   bool   `json:"has_claude_md"`   // true if CLAUDE.md exists
	HasAgentsMD   bool   `json:"has_agents_md"`   // true if AGENTS.md exists
	HasAIConfig   bool   `json:"has_ai_config"`   // true if .ai/config/workflow.yaml exists
}

// ScanRepo scans a repository and returns scan results
func ScanRepo(path string) (*ScanResult, error) {
	// Ensure path is absolute
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	// Verify path exists
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, os.ErrNotExist
	}

	language := DetectLanguage(absPath)

	result := &ScanResult{
		Language:      language,
		TestFileCount: CountTestFiles(absPath, language),
		HasSubmodules: HasSubmodules(absPath),
		Branch:        GetCurrentBranch(absPath),
		HasClaudeMD:   fileExists(filepath.Join(absPath, "CLAUDE.md")),
		HasAgentsMD:   fileExists(filepath.Join(absPath, "AGENTS.md")),
		HasAIConfig:   fileExists(filepath.Join(absPath, ".ai", "config", "workflow.yaml")),
	}

	return result, nil
}

// DetectLanguage detects the project language (go, unity, or unknown)
func DetectLanguage(path string) string {
	// Check for go.mod -> "go"
	if fileExists(filepath.Join(path, "go.mod")) {
		return "go"
	}

	// Check for ProjectSettings/ directory -> "unity"
	if dirExists(filepath.Join(path, "ProjectSettings")) {
		return "unity"
	}

	return "unknown"
}

// CountTestFiles counts test files based on language
func CountTestFiles(path string, language string) int {
	switch language {
	case "go":
		return countGoTestFiles(path)
	case "unity":
		return countUnityTestFiles(path)
	default:
		return 0
	}
}

// countGoTestFiles counts *_test.go files recursively
func countGoTestFiles(path string) int {
	count := 0
	_ = filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // continue walking on error
		}
		if info.IsDir() {
			// Skip hidden directories and vendor
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(info.Name(), "_test.go") {
			count++
		}
		return nil
	})
	return count
}

// countUnityTestFiles counts files in Assets/Tests/ or similar test directories
func countUnityTestFiles(path string) int {
	count := 0

	// Common Unity test directories
	testDirs := []string{
		filepath.Join(path, "Assets", "Tests"),
		filepath.Join(path, "Assets", "Editor", "Tests"),
		filepath.Join(path, "Assets", "PlayModeTests"),
		filepath.Join(path, "Assets", "EditModeTests"),
	}

	for _, testDir := range testDirs {
		if !dirExists(testDir) {
			continue
		}
		_ = filepath.Walk(testDir, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				return nil
			}
			// Count C# test files
			if strings.HasSuffix(info.Name(), ".cs") {
				count++
			}
			return nil
		})
	}

	return count
}

// HasSubmodules checks if the repo has git submodules
func HasSubmodules(path string) bool {
	return fileExists(filepath.Join(path, ".gitmodules"))
}

// GetCurrentBranch returns the current git branch
func GetCurrentBranch(path string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = path

	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(output))
}

// fileExists checks if a file exists and is not a directory
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// dirExists checks if a directory exists
func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
