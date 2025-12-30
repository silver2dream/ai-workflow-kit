package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// WorkerPreflight performs pre-flight checks for worker execution
type WorkerPreflight struct {
	StateRoot string
	RepoType  string // "root" | "directory" | "submodule"
	RepoPath  string
	Timeout   time.Duration
}

// PreflightResult represents the result of a preflight check
type PreflightResult struct {
	Passed  bool
	Message string
}

// CacheEntry represents a cache entry for remote accessibility
type CacheEntry struct {
	Accessible bool    `json:"accessible"`
	Timestamp  float64 `json:"timestamp"`
}

const cacheDir = ".ai/state/cache"
const remoteCacheFile = "remote_accessibility.json"
const cacheTTLSeconds = 300 // 5 minutes

// NewWorkerPreflight creates a new WorkerPreflight
func NewWorkerPreflight(stateRoot, repoType, repoPath string) *WorkerPreflight {
	if repoType == "" {
		repoType = "root"
	}
	if repoPath == "" {
		repoPath = "."
	}
	return &WorkerPreflight{
		StateRoot: stateRoot,
		RepoType:  repoType,
		RepoPath:  repoPath,
		Timeout:   60 * time.Second,
	}
}

// Check performs the preflight check
func (p *WorkerPreflight) Check(ctx context.Context) (*PreflightResult, error) {
	// Ensure cache directory exists
	cachePath := filepath.Join(p.StateRoot, cacheDir)
	_ = os.MkdirAll(cachePath, 0755)

	// Common check: root working tree must be clean
	if dirty, output := p.isWorkingTreeDirty(ctx, p.StateRoot); dirty {
		return &PreflightResult{
			Passed:  false,
			Message: fmt.Sprintf("root working tree not clean: %s", output),
		}, nil
	}

	// Type-specific checks
	switch p.RepoType {
	case "root":
		return p.checkRoot(ctx)
	case "directory":
		return p.checkDirectory(ctx)
	case "submodule":
		return p.checkSubmodule(ctx)
	default:
		return nil, fmt.Errorf("unknown repo type: %s", p.RepoType)
	}
}

// checkRoot performs preflight for root type
func (p *WorkerPreflight) checkRoot(ctx context.Context) (*PreflightResult, error) {
	// Sync and update submodules
	cmd := exec.CommandContext(ctx, "git", "submodule", "sync", "--recursive")
	cmd.Dir = p.StateRoot
	_ = cmd.Run()

	cmd = exec.CommandContext(ctx, "git", "submodule", "update", "--init", "--recursive")
	cmd.Dir = p.StateRoot
	if err := cmd.Run(); err != nil {
		return &PreflightResult{
			Passed:  false,
			Message: fmt.Sprintf("failed to update submodules: %v", err),
		}, nil
	}

	// Verify submodule working trees are clean
	submodules := p.getSubmodulePaths(ctx)
	for _, sm := range submodules {
		smPath := filepath.Join(p.StateRoot, sm)
		if dirty, output := p.isWorkingTreeDirty(ctx, smPath); dirty {
			return &PreflightResult{
				Passed:  false,
				Message: fmt.Sprintf("submodule '%s' not clean: %s", sm, output),
			}, nil
		}
	}

	return &PreflightResult{Passed: true, Message: "ok"}, nil
}

// checkDirectory performs preflight for directory type
func (p *WorkerPreflight) checkDirectory(ctx context.Context) (*PreflightResult, error) {
	dirPath := filepath.Join(p.StateRoot, p.RepoPath)
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		return &PreflightResult{
			Passed:  false,
			Message: fmt.Sprintf("directory path '%s' does not exist", p.RepoPath),
		}, nil
	}
	return &PreflightResult{Passed: true, Message: "ok"}, nil
}

// checkSubmodule performs preflight for submodule type
func (p *WorkerPreflight) checkSubmodule(ctx context.Context) (*PreflightResult, error) {
	smPath := filepath.Join(p.StateRoot, p.RepoPath)

	// Verify submodule exists
	if _, err := os.Stat(smPath); os.IsNotExist(err) {
		return &PreflightResult{
			Passed:  false,
			Message: fmt.Sprintf("submodule path '%s' does not exist", p.RepoPath),
		}, nil
	}

	// Verify it's a valid submodule
	gitPath := filepath.Join(smPath, ".git")
	if _, err := os.Stat(gitPath); os.IsNotExist(err) {
		return &PreflightResult{
			Passed:  false,
			Message: fmt.Sprintf("'%s' is not a valid submodule (no .git)", p.RepoPath),
		}, nil
	}

	// Check working tree is clean
	if dirty, output := p.isWorkingTreeDirty(ctx, smPath); dirty {
		return &PreflightResult{
			Passed:  false,
			Message: fmt.Sprintf("submodule '%s' not clean: %s", p.RepoPath, output),
		}, nil
	}

	// Check remote accessibility
	remoteURL := p.getRemoteURL(ctx, smPath)
	if remoteURL != "" {
		if !p.checkRemoteAccessible(ctx, remoteURL) {
			return &PreflightResult{
				Passed:  false,
				Message: fmt.Sprintf("submodule '%s' remote '%s' not accessible", p.RepoPath, remoteURL),
			}, nil
		}
	}

	return &PreflightResult{Passed: true, Message: "ok"}, nil
}

// isWorkingTreeDirty checks if a working tree has uncommitted changes
func (p *WorkerPreflight) isWorkingTreeDirty(ctx context.Context, dir string) (bool, string) {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return false, ""
	}
	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" {
		return false, ""
	}
	return true, outputStr
}

// getSubmodulePaths returns the paths of all submodules
func (p *WorkerPreflight) getSubmodulePaths(ctx context.Context) []string {
	cmd := exec.CommandContext(ctx, "git", "config", "-f", ".gitmodules", "--get-regexp", "path")
	cmd.Dir = p.StateRoot
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	var paths []string
	for _, line := range strings.Split(string(output), "\n") {
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			paths = append(paths, parts[1])
		}
	}
	return paths
}

// getRemoteURL gets the origin remote URL
func (p *WorkerPreflight) getRemoteURL(ctx context.Context, dir string) string {
	cmd := exec.CommandContext(ctx, "git", "remote", "get-url", "origin")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// checkRemoteAccessible checks if a remote is accessible (with caching)
func (p *WorkerPreflight) checkRemoteAccessible(ctx context.Context, remoteURL string) bool {
	cacheKey := "remote:" + remoteURL
	cachePath := filepath.Join(p.StateRoot, cacheDir, remoteCacheFile)

	// Check cache first
	if accessible, ok := p.checkCache(cachePath, cacheKey); ok {
		return accessible
	}

	// Try to reach remote
	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--exit-code", remoteURL, "HEAD")
	err := cmd.Run()
	accessible := err == nil

	// Update cache
	p.updateCache(cachePath, cacheKey, accessible)

	return accessible
}

// checkCache checks the cache for a key
func (p *WorkerPreflight) checkCache(cachePath, key string) (bool, bool) {
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return false, false
	}

	var cache map[string]CacheEntry
	if err := json.Unmarshal(data, &cache); err != nil {
		return false, false
	}

	entry, exists := cache[key]
	if !exists {
		return false, false
	}

	// Check TTL
	now := float64(time.Now().Unix())
	if now-entry.Timestamp > cacheTTLSeconds {
		return false, false
	}

	return entry.Accessible, true
}

// updateCache updates the cache with a new value
func (p *WorkerPreflight) updateCache(cachePath, key string, accessible bool) {
	var cache map[string]CacheEntry

	data, err := os.ReadFile(cachePath)
	if err == nil {
		_ = json.Unmarshal(data, &cache)
	}
	if cache == nil {
		cache = make(map[string]CacheEntry)
	}

	cache[key] = CacheEntry{
		Accessible: accessible,
		Timestamp:  float64(time.Now().Unix()),
	}

	newData, _ := json.MarshalIndent(cache, "", "  ")
	_ = os.WriteFile(cachePath, newData, 0644)
}
