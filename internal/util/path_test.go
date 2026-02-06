package util

import (
	"runtime"
	"testing"
)

// TestIsRootPath tests the IsRootPath function.
// Property 21: Consistent Root Path Detection
func TestIsRootPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Root paths (should return true)
		{name: "empty_string", input: "", expected: true},
		{name: "dot", input: ".", expected: true},
		{name: "dot_slash", input: "./", expected: true},
		{name: "dot_backslash", input: ".\\", expected: true},
		{name: "multiple_trailing_slashes", input: ".///", expected: true},
		{name: "single_slash", input: "/", expected: true},
		{name: "single_backslash", input: "\\", expected: true},

		// Non-root paths (should return false)
		{name: "backend", input: "backend", expected: false},
		{name: "backend_with_slash", input: "backend/", expected: false},
		{name: "backend_with_backslash", input: "backend\\", expected: false},
		{name: "relative_path", input: "./backend", expected: false},
		{name: "nested_path", input: "backend/internal", expected: false},
		{name: "dot_dot", input: "..", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRootPath(tt.input)
			if result != tt.expected {
				t.Errorf("IsRootPath(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

// TestCrossPlatformPathConsistency tests cross-platform path consistency.
// Property 19: Cross-Platform Path Consistency
func TestCrossPlatformPathConsistency(t *testing.T) {
	t.Run("normalize_backslashes", func(t *testing.T) {
		// Test backslashes are converted to forward slashes (Req 27.3)
		path := "backend\\internal\\foo.go"
		normalized := NormalizePath(path)

		if containsBackslash(normalized) {
			t.Errorf("NormalizePath should convert backslashes: got %q", normalized)
		}
		if !containsForwardSlash(normalized) {
			t.Errorf("NormalizePath should contain forward slashes: got %q", normalized)
		}
	})

	t.Run("normalize_trailing_slash", func(t *testing.T) {
		// Test trailing slashes are removed (Req 27.4)
		path := "backend/"
		normalized := NormalizePath(path)

		if hasSuffix(normalized, "/") {
			t.Errorf("NormalizePath should remove trailing slash: got %q", normalized)
		}
	})

	t.Run("normalize_mixed_slashes", func(t *testing.T) {
		// Test mixed slashes are normalized
		path := "backend\\internal/foo.go"
		normalized := NormalizePath(path)

		if containsBackslash(normalized) {
			t.Errorf("NormalizePath should convert all backslashes: got %q", normalized)
		}
	})

	t.Run("paths_equal_same_path", func(t *testing.T) {
		// Test identical paths are equal
		if !PathsEqual("backend/internal", "backend/internal") {
			t.Error("PathsEqual should return true for identical paths")
		}
	})

	t.Run("paths_equal_different_slashes", func(t *testing.T) {
		// Test paths with different slashes are equal
		if !PathsEqual("backend/internal", "backend\\internal") {
			t.Error("PathsEqual should return true for paths with different slashes")
		}
	})

	t.Run("paths_equal_trailing_slash", func(t *testing.T) {
		// Test paths with/without trailing slash are equal
		if !PathsEqual("backend/", "backend") {
			t.Error("PathsEqual should return true for paths with/without trailing slash")
		}
	})

	t.Run("paths_not_equal", func(t *testing.T) {
		// Test different paths are not equal
		if PathsEqual("backend", "frontend") {
			t.Error("PathsEqual should return false for different paths")
		}
	})
}

// TestPathNormalizationEdgeCases tests path normalization edge cases.
func TestPathNormalizationEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty_path",
			input:    "",
			expected: "",
		},
		{
			name:     "dot_path",
			input:    ".",
			expected: ".",
		},
		{
			name:     "dot_slash_path",
			input:    "./",
			expected: ".",
		},
		{
			name:     "multiple_trailing_slashes",
			input:    "backend///",
			expected: "backend",
		},
		{
			name:     "only_slashes",
			input:    "///",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizePath(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizePath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestPushPermissionVerification tests push permission verification.
// Property 20: Submodule Push Permission Verification
func TestPushPermissionVerification(t *testing.T) {
	t.Run("permission_cached", func(t *testing.T) {
		// Test permission result is cached (Req 28.3, 28.4)
		cache := NewPushPermissionCache()
		remoteURL := "https://github.com/test/repo.git"

		// First check
		result1 := CheckPushPermission(remoteURL, cache, nil)

		// Second check should use cache
		result2 := CheckPushPermission(remoteURL, cache, nil)

		if result1 != result2 {
			t.Errorf("CheckPushPermission should return same result: %v vs %v", result1, result2)
		}

		cached := cache.Get(remoteURL)
		if cached == nil {
			t.Error("Permission should be cached after check")
		}
	})

	t.Run("permission_check_called", func(t *testing.T) {
		// Test actual permission check is called (Req 28.1, 28.2)
		cache := NewPushPermissionCache()
		checkCalled := false

		mockCheck := func(url string) bool {
			checkCalled = true
			return true
		}

		CheckPushPermission("https://github.com/test/repo.git", cache, mockCheck)

		if !checkCalled {
			t.Error("Permission check function should be called")
		}
	})

	t.Run("permission_denied_cached", func(t *testing.T) {
		// Test denied permission is cached
		cache := NewPushPermissionCache()

		mockCheck := func(url string) bool {
			return false
		}

		remoteURL := "https://github.com/test/repo.git"
		result := CheckPushPermission(remoteURL, cache, mockCheck)

		if result != false {
			t.Error("CheckPushPermission should return false when denied")
		}

		cached := cache.Get(remoteURL)
		if cached == nil || *cached != false {
			t.Error("Denied permission should be cached as false")
		}
	})
}

// TestPushPermissionCache tests the push permission cache.
func TestPushPermissionCache(t *testing.T) {
	t.Run("cache_initially_empty", func(t *testing.T) {
		// Test cache is initially empty
		cache := NewPushPermissionCache()

		if cache.Get("https://github.com/test/repo.git") != nil {
			t.Error("Cache should initially return nil")
		}
	})

	t.Run("cache_set_and_get", func(t *testing.T) {
		// Test cache set and get
		cache := NewPushPermissionCache()
		remoteURL := "https://github.com/test/repo.git"

		cache.Set(remoteURL, true)

		result := cache.Get(remoteURL)
		if result == nil || *result != true {
			t.Error("Cache should return true after Set(true)")
		}
	})

	t.Run("cache_different_urls", func(t *testing.T) {
		// Test cache handles different URLs
		cache := NewPushPermissionCache()

		cache.Set("https://github.com/test/repo1.git", true)
		cache.Set("https://github.com/test/repo2.git", false)

		result1 := cache.Get("https://github.com/test/repo1.git")
		result2 := cache.Get("https://github.com/test/repo2.git")

		if result1 == nil || *result1 != true {
			t.Error("repo1 should be cached as true")
		}
		if result2 == nil || *result2 != false {
			t.Error("repo2 should be cached as false")
		}
	})

	t.Run("cache_ttl_expiry", func(t *testing.T) {
		// Test cache entry expiry (using a short TTL for testing)
		cache := NewPushPermissionCache()
		cache.TTLSeconds = 0 // Expire immediately

		remoteURL := "https://github.com/test/repo.git"
		cache.Set(remoteURL, true)

		// With TTL of 0, the entry should be expired
		result := cache.Get(remoteURL)
		if result != nil {
			t.Error("Expired cache entry should return nil")
		}
	})
}

// TestWindowsCaseInsensitivity tests Windows-specific case insensitivity.
func TestWindowsCaseInsensitivity(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows-specific test on non-Windows platform")
	}

	t.Run("case_insensitive_on_windows", func(t *testing.T) {
		// On Windows, paths should be case-insensitive
		if !PathsEqual("Backend/Internal", "backend/internal") {
			t.Error("PathsEqual should be case-insensitive on Windows")
		}
	})
}

// TestShellSafe tests the ShellSafe function for bash eval safety.
func TestShellSafe(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple string", "hello", "'hello'"},
		{"empty string", "", "''"},
		{"with spaces", "hello world", "'hello world'"},
		{"with single quote", "it's fine", `'it'\''s fine'`},
		{"with newline", "line1\nline2", "'line1 line2'"},
		{"with carriage return", "line1\r\nline2", "'line1 line2'"},
		{"shell injection attempt", "$(rm -rf /)", "'$(rm -rf /)'"},
		{"backtick injection", "`rm -rf /`", "'`rm -rf /`'"},
		{"double quotes", `he said "hello"`, `'he said "hello"'`},
		{"semicolon", "a; rm -rf /", "'a; rm -rf /'"},
		{"pipe", "a | cat /etc/passwd", "'a | cat /etc/passwd'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShellSafe(tt.input)
			if result != tt.expected {
				t.Errorf("ShellSafe(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// Helper functions for tests
func containsBackslash(s string) bool {
	for _, c := range s {
		if c == '\\' {
			return true
		}
	}
	return false
}

func containsForwardSlash(s string) bool {
	for _, c := range s {
		if c == '/' {
			return true
		}
	}
	return false
}

func hasSuffix(s, suffix string) bool {
	if len(s) < len(suffix) {
		return false
	}
	return s[len(s)-len(suffix):] == suffix
}
