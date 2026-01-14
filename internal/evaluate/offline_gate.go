package evaluate

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// requiredIgnoredPaths lists paths that must be ignored by git.
var requiredIgnoredPaths = []string{
	".ai/state",
	".ai/results",
	".ai/runs",
	".ai/exe-logs",
}

// CheckO0GitIgnore checks if required paths are ignored by git.
// Returns PASS if all required paths are ignored, FAIL otherwise.
func CheckO0GitIgnore(rootPath string) GateResult {
	// Check if we're in a git repository
	gitDir := filepath.Join(rootPath, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return Skip("not a git repository")
	}

	var notIgnored []string
	for _, path := range requiredIgnoredPaths {
		fullPath := filepath.Join(rootPath, path)

		// Use git check-ignore to verify the path is ignored
		cmd := exec.Command("git", "-C", rootPath, "check-ignore", "-q", fullPath)
		err := cmd.Run()
		if err != nil {
			// Exit code != 0 means not ignored
			notIgnored = append(notIgnored, path)
		}
	}

	if len(notIgnored) > 0 {
		return Fail("paths not ignored: " + strings.Join(notIgnored, ", "))
	}

	return Pass("all required paths are git-ignored")
}

// CheckO5ConfigValidation validates the workflow.yaml configuration file.
// Returns PASS if the config is valid, FAIL otherwise.
func CheckO5ConfigValidation(rootPath string) GateResult {
	configPath := filepath.Join(rootPath, ".ai", "config", "workflow.yaml")

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Fail("workflow.yaml not found")
		}
		return Fail("cannot read workflow.yaml: " + err.Error())
	}

	// Parse YAML to validate structure
	var config map[string]interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return Fail("invalid YAML syntax: " + err.Error())
	}

	// Check required top-level fields
	requiredFields := []string{"project", "repos", "git"}
	var missingFields []string
	for _, field := range requiredFields {
		if _, ok := config[field]; !ok {
			missingFields = append(missingFields, field)
		}
	}

	if len(missingFields) > 0 {
		return Fail("missing required fields: " + strings.Join(missingFields, ", "))
	}

	// Check project has name and type
	project, ok := config["project"].(map[string]interface{})
	if !ok {
		return Fail("project must be a map")
	}

	if _, ok := project["name"]; !ok {
		return Fail("project.name is required")
	}
	if _, ok := project["type"]; !ok {
		return Fail("project.type is required")
	}

	// Check git has integration_branch
	git, ok := config["git"].(map[string]interface{})
	if !ok {
		return Fail("git must be a map")
	}
	if _, ok := git["integration_branch"]; !ok {
		return Fail("git.integration_branch is required")
	}

	return Pass("workflow.yaml is valid")
}

// CheckO7VersionSync checks version synchronization.
// This is a placeholder implementation.
func CheckO7VersionSync(rootPath string) GateResult {
	// Placeholder: check if version file exists
	versionPath := filepath.Join(rootPath, "internal", "buildinfo", "version.go")
	if _, err := os.Stat(versionPath); os.IsNotExist(err) {
		return Skip("version.go not found")
	}

	return Pass("version file exists")
}

// CheckO8FileEncoding checks for problematic file encodings (CRLF, UTF-16).
// Returns PASS if no problematic encodings found, FAIL otherwise.
func CheckO8FileEncoding(rootPath string) GateResult {
	var problems []string

	// Walk through .ai directory looking for problematic files
	aiDir := filepath.Join(rootPath, ".ai")
	if _, err := os.Stat(aiDir); os.IsNotExist(err) {
		return Skip(".ai directory not found")
	}

	err := filepath.Walk(aiDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}
		if info.IsDir() {
			// Skip state/results/runs/exe-logs directories
			base := filepath.Base(path)
			if base == "state" || base == "results" || base == "runs" || base == "exe-logs" {
				return filepath.SkipDir
			}
			return nil
		}

		// Only check text files
		ext := strings.ToLower(filepath.Ext(path))
		if !isTextExtension(ext) {
			return nil
		}

		// Check file encoding
		if hasCRLF, hasUTF16BOM := checkFileEncoding(path); hasCRLF || hasUTF16BOM {
			relPath, _ := filepath.Rel(rootPath, path)
			if hasCRLF {
				problems = append(problems, relPath+" (CRLF)")
			}
			if hasUTF16BOM {
				problems = append(problems, relPath+" (UTF-16 BOM)")
			}
		}

		return nil
	})

	if err != nil {
		return Fail("error walking directory: " + err.Error())
	}

	if len(problems) > 0 {
		return Fail("encoding issues: " + strings.Join(problems, ", "))
	}

	return Pass("no encoding issues found")
}

// isTextExtension returns true if the extension is a known text file type.
func isTextExtension(ext string) bool {
	textExtensions := map[string]bool{
		".md":   true,
		".yaml": true,
		".yml":  true,
		".json": true,
		".txt":  true,
		".sh":   true,
		".go":   true,
		".py":   true,
	}
	return textExtensions[ext]
}

// checkFileEncoding checks if a file has CRLF line endings or UTF-16 BOM.
func checkFileEncoding(path string) (hasCRLF bool, hasUTF16BOM bool) {
	file, err := os.Open(path)
	if err != nil {
		return false, false
	}
	defer file.Close()

	// Read first few bytes to check for BOM
	header := make([]byte, 4)
	n, err := file.Read(header)
	if err != nil || n < 2 {
		return false, false
	}

	// Check for UTF-16 BOM (LE: FF FE or BE: FE FF)
	if (header[0] == 0xFF && header[1] == 0xFE) || (header[0] == 0xFE && header[1] == 0xFF) {
		hasUTF16BOM = true
	}

	// Reset to beginning and check for CRLF
	file.Seek(0, 0)
	scanner := bufio.NewScanner(file)
	// Read raw bytes to detect CRLF
	scanner.Split(bufio.ScanBytes)

	prevByte := byte(0)
	for scanner.Scan() {
		b := scanner.Bytes()[0]
		if prevByte == '\r' && b == '\n' {
			hasCRLF = true
			break
		}
		prevByte = b
	}

	return hasCRLF, hasUTF16BOM
}
