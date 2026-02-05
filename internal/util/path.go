// Package util provides utility functions for cross-platform operations.
package util

import (
	"runtime"
	"strings"
	"sync"
	"time"
)

// IsRootPath checks if the given path represents a root/current directory.
// It handles "", ".", "./", ".\", and variants with trailing slashes.
//
// Property 21: Consistent Root Path Detection
// For any repo path comparison, the system SHALL use a unified function
// to determine if a path represents the root/current directory.
func IsRootPath(path string) bool {
	// Normalize: convert backslashes to forward slashes and remove trailing slashes
	normalized := strings.ReplaceAll(path, "\\", "/")
	normalized = strings.TrimRight(normalized, "/")
	return normalized == "" || normalized == "."
}

// NormalizePath normalizes a path for cross-platform comparison.
// It converts backslashes to forward slashes and removes trailing slashes.
// On Windows, it also converts the path to lowercase for case-insensitive comparison.
//
// Property 19: Cross-Platform Path Consistency
// For any repo path, the system SHALL handle case sensitivity correctly
// based on the filesystem and normalize paths to use forward slashes.
func NormalizePath(path string) string {
	// Convert backslashes to forward slashes (Req 27.3)
	path = strings.ReplaceAll(path, "\\", "/")

	// Remove trailing slashes
	path = strings.TrimRight(path, "/")

	// Convert to lowercase on Windows (Req 27.1, 27.2)
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}

	return path
}

// PathsEqual compares two paths case-insensitively on Windows.
//
// Property 19: Cross-Platform Path Consistency
func PathsEqual(path1, path2 string) bool {
	return NormalizePath(path1) == NormalizePath(path2)
}

// PushPermissionCache caches push permission results.
//
// Property 20: Submodule Push Permission Verification
// For any submodule-type repo, preflight SHALL verify push access
// to the submodule remote before proceeding.
type PushPermissionCache struct {
	mu         sync.RWMutex
	cache      map[string]permissionEntry
	TTLSeconds int
}

type permissionEntry struct {
	Allowed   bool
	Timestamp time.Time
}

// NewPushPermissionCache creates a new push permission cache.
func NewPushPermissionCache() *PushPermissionCache {
	return &PushPermissionCache{
		cache:      make(map[string]permissionEntry),
		TTLSeconds: 300, // 5 minutes
	}
}

// Get returns the cached permission result, or nil if not cached or expired.
func (c *PushPermissionCache) Get(remoteURL string) *bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.cache[remoteURL]
	if !ok {
		return nil
	}

	// Check TTL
	if time.Since(entry.Timestamp) > time.Duration(c.TTLSeconds)*time.Second {
		return nil
	}

	return &entry.Allowed
}

// Set stores the permission result in the cache.
func (c *PushPermissionCache) Set(remoteURL string, allowed bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache[remoteURL] = permissionEntry{
		Allowed:   allowed,
		Timestamp: time.Now(),
	}
}

// CheckFn is a function that checks push permission for a remote URL.
type CheckFn func(remoteURL string) bool

// CheckPushPermission checks push permission with caching.
//
// Property 20: Submodule Push Permission Verification
// For any submodule-type repo, preflight SHALL verify push access
// to the submodule remote before proceeding.
func CheckPushPermission(remoteURL string, cache *PushPermissionCache, checkFn CheckFn) bool {
	// Check cache first (Req 28.4)
	if cached := cache.Get(remoteURL); cached != nil {
		return *cached
	}

	// Perform actual check (Req 28.1, 28.2)
	var allowed bool
	if checkFn != nil {
		allowed = checkFn(remoteURL)
	} else {
		allowed = true // Default for when no check function provided
	}

	// Update cache (Req 28.3)
	cache.Set(remoteURL, allowed)

	return allowed
}
