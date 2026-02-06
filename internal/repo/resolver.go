// Package repo provides unified repository configuration and resolution.
package repo

import (
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/util"
)

// Config represents a unified repository configuration.
// This is the single source of truth for repo configuration across all AWK modules.
type Config struct {
	Name     string       `yaml:"name" json:"name"`
	Path     string       `yaml:"path" json:"path"`
	Type     string       `yaml:"type" json:"type"` // "root", "directory", "submodule"
	Language string       `yaml:"language" json:"language"`
	Verify   VerifyConfig `yaml:"verify" json:"verify"`
}

// VerifyConfig holds verification commands for a repository.
type VerifyConfig struct {
	Build string `yaml:"build" json:"build"`
	Test  string `yaml:"test" json:"test"`
}

// Resolver provides methods to find and match repos.
// It uses precise path matching instead of loose string contains.
type Resolver struct {
	repos []Config
}

// NewResolver creates a resolver from a list of repo configs.
func NewResolver(repos []Config) *Resolver {
	return &Resolver{repos: repos}
}

// FindByName finds a repo by exact name match (case-insensitive).
func (r *Resolver) FindByName(name string) *Config {
	name = strings.ToLower(strings.TrimSpace(name))
	for i := range r.repos {
		if strings.ToLower(r.repos[i].Name) == name {
			return &r.repos[i]
		}
	}
	return nil
}

// FindByWorktreePath finds a repo by analyzing worktree path.
// Uses precise path segment matching, not loose string contains.
//
// For a worktree path like "/project/.worktrees/issue-1/frontend",
// it will correctly match a repo with path "frontend" or name "frontend".
//
// Returns nil if no match is found (caller should handle this case).
func (r *Resolver) FindByWorktreePath(worktreePath string) *Config {
	if worktreePath == "" || worktreePath == "NOT_FOUND" {
		return nil
	}

	// Normalize the worktree path
	normalized := normalizePath(worktreePath)

	// Try to match by checking if worktree path contains repo path as a proper path segment
	for i := range r.repos {
		repoPath := normalizePath(r.repos[i].Path)
		repoName := strings.ToLower(r.repos[i].Name)

		// Skip root repos for path matching - they match everything
		if util.IsRootPath(r.repos[i].Path) {
			continue
		}

		// Check if worktree path contains the repo path or name as a proper path segment
		// e.g., "/project/.worktrees/issue-1/frontend" should match "frontend"
		// but "/project/backend-old/.worktrees/issue-1" should NOT match "backend"
		if containsPathSegment(normalized, repoPath) || containsPathSegment(normalized, repoName) {
			return &r.repos[i]
		}
	}

	// If no directory repo matched, check if there's a root repo
	for i := range r.repos {
		if util.IsRootPath(r.repos[i].Path) || r.repos[i].Type == "root" {
			return &r.repos[i]
		}
	}

	// No match found - return nil, let caller decide fallback
	return nil
}

// All returns all configured repos.
func (r *Resolver) All() []Config {
	return r.repos
}

// containsPathSegment checks if fullPath contains segment as a proper path component.
// This prevents false positives like "backend" matching in "backend-old" or "mybackend".
func containsPathSegment(fullPath, segment string) bool {
	if segment == "" {
		return false
	}

	// Split both paths into segments
	fullSegments := splitPath(fullPath)
	segmentParts := splitPath(segment)

	if len(segmentParts) == 0 {
		return false
	}

	// Check if segment appears as contiguous path components
	for i := 0; i <= len(fullSegments)-len(segmentParts); i++ {
		match := true
		for j := 0; j < len(segmentParts); j++ {
			if fullSegments[i+j] != segmentParts[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// splitPath splits a path into segments, filtering out empty strings.
func splitPath(path string) []string {
	parts := strings.Split(path, "/")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// normalizePath normalizes a path for comparison.
func normalizePath(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	path = strings.TrimRight(path, "/")
	return strings.ToLower(path)
}
