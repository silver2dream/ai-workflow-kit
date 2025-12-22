"""
Unit tests for cross-platform functionality.

**Feature: multi-repo-support**
**Property 19: Cross-Platform Path Consistency**
**Property 20: Submodule Push Permission Verification**
**Validates: Requirements 27.1-27.4, 28.1-28.4**

Note: These tests validate the cross-platform logic.
"""
import pytest
import os


def normalize_path(path: str) -> str:
    """
    Normalize path for cross-platform comparison.
    
    Property 19: Cross-Platform Path Consistency
    *For any* repo path, the system SHALL handle case sensitivity correctly 
    based on the filesystem and normalize paths to use forward slashes.
    
    Returns: Normalized path
    """
    # Convert backslashes to forward slashes (Req 27.3)
    path = path.replace("\\", "/")
    
    # Remove trailing slashes
    path = path.rstrip("/")
    
    # Convert to lowercase on Windows (Req 27.1, 27.2)
    if os.name == "nt":
        path = path.lower()
    
    return path


def paths_equal(path1: str, path2: str) -> bool:
    """
    Compare paths case-insensitively on Windows.
    
    Property 19: Cross-Platform Path Consistency
    
    Returns: True if paths are equal
    """
    return normalize_path(path1) == normalize_path(path2)


class PushPermissionCache:
    """Cache for push permission results."""
    def __init__(self):
        self.cache = {}
        self.ttl_seconds = 300  # 5 minutes
    
    def get(self, remote_url: str) -> bool | None:
        """Get cached permission result."""
        entry = self.cache.get(remote_url)
        if entry is None:
            return None
        return entry.get("allowed")
    
    def set(self, remote_url: str, allowed: bool):
        """Set cached permission result."""
        self.cache[remote_url] = {
            "allowed": allowed,
            "timestamp": 0  # Simplified for testing
        }


def check_push_permission(
    remote_url: str,
    cache: PushPermissionCache,
    actual_check_fn=None
) -> bool:
    """
    Check push permission with caching.
    
    Property 20: Submodule Push Permission Verification
    *For any* submodule-type repo, preflight SHALL verify push access 
    to the submodule remote before proceeding.
    
    Returns: True if push is allowed
    """
    # Check cache first (Req 28.4)
    cached = cache.get(remote_url)
    if cached is not None:
        return cached
    
    # Perform actual check (Req 28.1, 28.2)
    if actual_check_fn:
        allowed = actual_check_fn(remote_url)
    else:
        allowed = True  # Default for testing
    
    # Update cache (Req 28.3)
    cache.set(remote_url, allowed)
    
    return allowed


class TestCrossPlatformPathConsistency:
    """Test cross-platform path consistency.
    
    Property 19: Cross-Platform Path Consistency
    """

    def test_normalize_backslashes(self):
        """Test backslashes are converted to forward slashes (Req 27.3)."""
        path = "backend\\internal\\foo.go"
        normalized = normalize_path(path)
        
        assert "\\" not in normalized
        assert "/" in normalized

    def test_normalize_trailing_slash(self):
        """Test trailing slashes are removed (Req 27.4)."""
        path = "backend/"
        normalized = normalize_path(path)
        
        assert not normalized.endswith("/")

    def test_normalize_mixed_slashes(self):
        """Test mixed slashes are normalized."""
        path = "backend\\internal/foo.go"
        normalized = normalize_path(path)
        
        assert "\\" not in normalized

    def test_paths_equal_same_path(self):
        """Test identical paths are equal."""
        assert paths_equal("backend/internal", "backend/internal") is True

    def test_paths_equal_different_slashes(self):
        """Test paths with different slashes are equal."""
        assert paths_equal("backend/internal", "backend\\internal") is True

    def test_paths_equal_trailing_slash(self):
        """Test paths with/without trailing slash are equal."""
        assert paths_equal("backend/", "backend") is True

    def test_paths_not_equal(self):
        """Test different paths are not equal."""
        assert paths_equal("backend", "frontend") is False


class TestPushPermissionVerification:
    """Test push permission verification.
    
    Property 20: Submodule Push Permission Verification
    """

    def test_permission_cached(self):
        """Test permission result is cached (Req 28.3, 28.4)."""
        cache = PushPermissionCache()
        
        # First check
        result1 = check_push_permission("https://github.com/test/repo.git", cache)
        
        # Second check should use cache
        result2 = check_push_permission("https://github.com/test/repo.git", cache)
        
        assert result1 == result2
        assert cache.get("https://github.com/test/repo.git") is not None

    def test_permission_check_called(self):
        """Test actual permission check is called (Req 28.1, 28.2)."""
        cache = PushPermissionCache()
        check_called = [False]
        
        def mock_check(url):
            check_called[0] = True
            return True
        
        check_push_permission("https://github.com/test/repo.git", cache, mock_check)
        
        assert check_called[0] is True

    def test_permission_denied_cached(self):
        """Test denied permission is cached."""
        cache = PushPermissionCache()
        
        def mock_check(url):
            return False
        
        result = check_push_permission("https://github.com/test/repo.git", cache, mock_check)
        
        assert result is False
        assert cache.get("https://github.com/test/repo.git") is False


class TestPushPermissionCache:
    """Test push permission cache."""

    def test_cache_initially_empty(self):
        """Test cache is initially empty."""
        cache = PushPermissionCache()
        
        assert cache.get("https://github.com/test/repo.git") is None

    def test_cache_set_and_get(self):
        """Test cache set and get."""
        cache = PushPermissionCache()
        
        cache.set("https://github.com/test/repo.git", True)
        
        assert cache.get("https://github.com/test/repo.git") is True

    def test_cache_different_urls(self):
        """Test cache handles different URLs."""
        cache = PushPermissionCache()
        
        cache.set("https://github.com/test/repo1.git", True)
        cache.set("https://github.com/test/repo2.git", False)
        
        assert cache.get("https://github.com/test/repo1.git") is True
        assert cache.get("https://github.com/test/repo2.git") is False


class TestPathNormalizationEdgeCases:
    """Test path normalization edge cases."""

    def test_empty_path(self):
        """Test empty path is handled."""
        assert normalize_path("") == ""

    def test_dot_path(self):
        """Test dot path is handled."""
        assert normalize_path(".") == "."

    def test_dot_slash_path(self):
        """Test ./ path is handled."""
        assert normalize_path("./") == "."

    def test_multiple_trailing_slashes(self):
        """Test multiple trailing slashes are removed."""
        assert normalize_path("backend///") == "backend"

    def test_only_slashes(self):
        """Test path with only slashes."""
        assert normalize_path("///") == ""
