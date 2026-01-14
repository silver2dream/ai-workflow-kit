package generate

import (
	"testing"
)

// =============================================================================
// Repo Type Detection Tests (migrated from test_repo_type.py)
// Property 1: Repo Type Detection Consistency
// =============================================================================

// GetRepoType returns the repo type from config for a given repo name.
// Returns "directory" as default if not specified or repo not found.
func GetRepoType(config *Config, repoName string) string {
	for _, repo := range config.Repos {
		if repo.Name == repoName {
			if repo.Type != "" {
				return repo.Type
			}
			return "directory"
		}
	}
	return "directory"
}

// GetRepoPath returns the repo path from config for a given repo name.
// Returns "." as default if not specified or repo not found.
func GetRepoPath(config *Config, repoName string) string {
	for _, repo := range config.Repos {
		if repo.Name == repoName {
			if repo.Path != "" {
				return repo.Path
			}
			return "."
		}
	}
	return "."
}

func TestGetRepoType_DetectRootType(t *testing.T) {
	// Test detection of root type repo
	config := &Config{
		Project: ProjectConfig{Name: "test", Type: "single-repo"},
		Git:     GitConfig{IntegrationBranch: "develop", ReleaseBranch: "main", CommitFormat: "[type] subject"},
		Repos: []RepoConfig{
			{
				Name:     "root",
				Path:     "./",
				Type:     "root",
				Language: "go",
				Verify:   VerifyConfig{Build: "go build ./...", Test: "go test ./..."},
			},
		},
	}

	repoType := GetRepoType(config, "root")
	if repoType != "root" {
		t.Errorf("GetRepoType() = %q, want %q", repoType, "root")
	}
}

func TestGetRepoType_DetectDirectoryType(t *testing.T) {
	// Test detection of directory type repo
	config := &Config{
		Project: ProjectConfig{Name: "test", Type: "monorepo"},
		Git:     GitConfig{IntegrationBranch: "develop", ReleaseBranch: "main", CommitFormat: "[type] subject"},
		Repos: []RepoConfig{
			{
				Name:     "backend",
				Path:     "backend/",
				Type:     "directory",
				Language: "go",
				Verify:   VerifyConfig{Build: "go build ./...", Test: "go test ./..."},
			},
		},
	}

	repoType := GetRepoType(config, "backend")
	if repoType != "directory" {
		t.Errorf("GetRepoType() = %q, want %q", repoType, "directory")
	}
}

func TestGetRepoType_DetectSubmoduleType(t *testing.T) {
	// Test detection of submodule type repo
	config := &Config{
		Project: ProjectConfig{Name: "test", Type: "monorepo"},
		Git:     GitConfig{IntegrationBranch: "develop", ReleaseBranch: "main", CommitFormat: "[type] subject"},
		Repos: []RepoConfig{
			{
				Name:     "backend",
				Path:     "backend/",
				Type:     "submodule",
				Language: "go",
				Verify:   VerifyConfig{Build: "go build ./...", Test: "go test ./..."},
			},
		},
	}

	repoType := GetRepoType(config, "backend")
	if repoType != "submodule" {
		t.Errorf("GetRepoType() = %q, want %q", repoType, "submodule")
	}
}

func TestGetRepoType_DefaultToDirectoryWhenTypeMissing(t *testing.T) {
	// Test default to directory when type is not specified
	config := &Config{
		Project: ProjectConfig{Name: "test", Type: "monorepo"},
		Git:     GitConfig{IntegrationBranch: "develop", ReleaseBranch: "main", CommitFormat: "[type] subject"},
		Repos: []RepoConfig{
			{
				Name:     "backend",
				Path:     "backend/",
				Type:     "", // Type not specified
				Language: "go",
				Verify:   VerifyConfig{Build: "go build ./...", Test: "go test ./..."},
			},
		},
	}

	repoType := GetRepoType(config, "backend")
	if repoType != "directory" {
		t.Errorf("GetRepoType() = %q, want %q (default)", repoType, "directory")
	}
}

func TestGetRepoType_DefaultToDirectoryWhenRepoNotFound(t *testing.T) {
	// Test default to directory when repo name not found
	config := &Config{
		Project: ProjectConfig{Name: "test", Type: "monorepo"},
		Git:     GitConfig{IntegrationBranch: "develop", ReleaseBranch: "main", CommitFormat: "[type] subject"},
		Repos: []RepoConfig{
			{
				Name:     "backend",
				Path:     "backend/",
				Type:     "root",
				Language: "go",
				Verify:   VerifyConfig{Build: "go build ./...", Test: "go test ./..."},
			},
		},
	}

	// Query for non-existent repo
	repoType := GetRepoType(config, "nonexistent")
	if repoType != "directory" {
		t.Errorf("GetRepoType() = %q, want %q (default)", repoType, "directory")
	}
}

func TestGetRepoType_AllValidTypes(t *testing.T) {
	// Test all valid repo types are recognized
	validTypes := []string{"root", "directory", "submodule"}

	for _, expectedType := range validTypes {
		t.Run(expectedType, func(t *testing.T) {
			config := &Config{
				Repos: []RepoConfig{
					{Name: "test", Type: expectedType},
				},
			}
			repoType := GetRepoType(config, "test")
			if repoType != expectedType {
				t.Errorf("GetRepoType() = %q, want %q", repoType, expectedType)
			}
		})
	}
}

// =============================================================================
// Repo Path Detection Tests
// Validates: Requirements 15.1, 15.2, 15.3, 15.4
// =============================================================================

func TestGetRepoPath_DetectRepoPath(t *testing.T) {
	// Test detection of repo path
	config := &Config{
		Project: ProjectConfig{Name: "test", Type: "monorepo"},
		Git:     GitConfig{IntegrationBranch: "develop", ReleaseBranch: "main", CommitFormat: "[type] subject"},
		Repos: []RepoConfig{
			{
				Name:     "backend",
				Path:     "backend/",
				Type:     "directory",
				Language: "go",
				Verify:   VerifyConfig{Build: "go build ./...", Test: "go test ./..."},
			},
		},
	}

	repoPath := GetRepoPath(config, "backend")
	if repoPath != "backend/" {
		t.Errorf("GetRepoPath() = %q, want %q", repoPath, "backend/")
	}
}

func TestGetRepoPath_RootTypePathIsDot(t *testing.T) {
	// Test root type has path ./ or .
	config := &Config{
		Project: ProjectConfig{Name: "test", Type: "single-repo"},
		Git:     GitConfig{IntegrationBranch: "develop", ReleaseBranch: "main", CommitFormat: "[type] subject"},
		Repos: []RepoConfig{
			{
				Name:     "root",
				Path:     "./",
				Type:     "root",
				Language: "go",
				Verify:   VerifyConfig{Build: "go build ./...", Test: "go test ./..."},
			},
		},
	}

	repoPath := GetRepoPath(config, "root")
	// Normalize path - ./ and . are equivalent
	if repoPath != "./" && repoPath != "." && repoPath != "" {
		t.Errorf("GetRepoPath() = %q, want './' or '.' or ''", repoPath)
	}
}

func TestGetRepoPath_DefaultPathWhenNotSpecified(t *testing.T) {
	// Test default path is . when not specified
	config := &Config{
		Repos: []RepoConfig{
			{Name: "test", Type: "root", Path: ""},
		},
	}

	repoPath := GetRepoPath(config, "test")
	if repoPath != "." {
		t.Errorf("GetRepoPath() = %q, want %q (default)", repoPath, ".")
	}
}

func TestGetRepoPath_DefaultPathWhenRepoNotFound(t *testing.T) {
	// Test default path is . when repo not found
	config := &Config{
		Repos: []RepoConfig{
			{Name: "backend", Path: "backend/", Type: "directory"},
		},
	}

	repoPath := GetRepoPath(config, "nonexistent")
	if repoPath != "." {
		t.Errorf("GetRepoPath() = %q, want %q (default)", repoPath, ".")
	}
}

// =============================================================================
// Table-Driven Tests for Repo Type and Path Detection
// =============================================================================

func TestRepoTypeDetection_TableDriven(t *testing.T) {
	tests := []struct {
		name         string
		repos        []RepoConfig
		queryRepo    string
		expectedType string
	}{
		{
			name: "root type detected",
			repos: []RepoConfig{
				{Name: "root", Type: "root"},
			},
			queryRepo:    "root",
			expectedType: "root",
		},
		{
			name: "directory type detected",
			repos: []RepoConfig{
				{Name: "backend", Type: "directory"},
			},
			queryRepo:    "backend",
			expectedType: "directory",
		},
		{
			name: "submodule type detected",
			repos: []RepoConfig{
				{Name: "frontend", Type: "submodule"},
			},
			queryRepo:    "frontend",
			expectedType: "submodule",
		},
		{
			name: "empty type defaults to directory",
			repos: []RepoConfig{
				{Name: "backend", Type: ""},
			},
			queryRepo:    "backend",
			expectedType: "directory",
		},
		{
			name: "nonexistent repo defaults to directory",
			repos: []RepoConfig{
				{Name: "backend", Type: "root"},
			},
			queryRepo:    "nonexistent",
			expectedType: "directory",
		},
		{
			name: "multiple repos finds correct one",
			repos: []RepoConfig{
				{Name: "backend", Type: "directory"},
				{Name: "frontend", Type: "submodule"},
				{Name: "shared", Type: "root"},
			},
			queryRepo:    "frontend",
			expectedType: "submodule",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{Repos: tt.repos}
			repoType := GetRepoType(config, tt.queryRepo)
			if repoType != tt.expectedType {
				t.Errorf("GetRepoType() = %q, want %q", repoType, tt.expectedType)
			}
		})
	}
}

func TestRepoPathDetection_TableDriven(t *testing.T) {
	tests := []struct {
		name         string
		repos        []RepoConfig
		queryRepo    string
		expectedPath string
	}{
		{
			name: "explicit path returned",
			repos: []RepoConfig{
				{Name: "backend", Path: "backend/"},
			},
			queryRepo:    "backend",
			expectedPath: "backend/",
		},
		{
			name: "nested path returned",
			repos: []RepoConfig{
				{Name: "shared", Path: "libs/shared/"},
			},
			queryRepo:    "shared",
			expectedPath: "libs/shared/",
		},
		{
			name: "empty path defaults to dot",
			repos: []RepoConfig{
				{Name: "root", Path: ""},
			},
			queryRepo:    "root",
			expectedPath: ".",
		},
		{
			name: "nonexistent repo defaults to dot",
			repos: []RepoConfig{
				{Name: "backend", Path: "backend/"},
			},
			queryRepo:    "nonexistent",
			expectedPath: ".",
		},
		{
			name: "dot path preserved",
			repos: []RepoConfig{
				{Name: "root", Path: "."},
			},
			queryRepo:    "root",
			expectedPath: ".",
		},
		{
			name: "dot-slash path preserved",
			repos: []RepoConfig{
				{Name: "root", Path: "./"},
			},
			queryRepo:    "root",
			expectedPath: "./",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{Repos: tt.repos}
			repoPath := GetRepoPath(config, tt.queryRepo)
			if repoPath != tt.expectedPath {
				t.Errorf("GetRepoPath() = %q, want %q", repoPath, tt.expectedPath)
			}
		})
	}
}
