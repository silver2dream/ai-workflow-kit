"""
Unit tests for security functionality.

**Feature: multi-repo-support**
**Property 17: Script Modification Protection**
**Property 21: Sensitive Information Detection**
**Validates: Requirements 25.1-25.5, 29.1-29.5**

Note: These tests validate the security functions.
"""
import pytest
import re
from typing import List, Tuple


# ============================================================
# Script Modification Protection (Req 25.1-25.5)
# ============================================================

PROTECTED_PATHS = [".ai/scripts/", ".ai/commands/"]


def check_script_modifications(
    changed_files: List[str],
    allow_script_changes: bool = False,
    whitelist: List[str] = None
) -> Tuple[bool, List[str]]:
    """
    Check for modifications to protected scripts.
    
    Property 17: Script Modification Protection
    *For any* commit that modifies files in `.ai/scripts/` or `.ai/commands/`, 
    the system SHALL require explicit approval unless whitelisted.
    
    Returns: (is_allowed, list_of_violations)
    """
    whitelist = whitelist or []
    violations = []
    
    for file in changed_files:
        for protected in PROTECTED_PATHS:
            if file.startswith(protected):
                # Check if file is in whitelist (Req 25.3)
                if file in whitelist:
                    continue
                violations.append(file)
    
    if violations:
        # Check if explicitly approved (Req 25.2)
        if allow_script_changes:
            return True, violations  # Allowed but reported
        return False, violations
    
    return True, []


# ============================================================
# Sensitive Information Detection (Req 29.1-29.5)
# ============================================================

DEFAULT_SECRET_PATTERNS = [
    r'password\s*[:=]\s*["\'][^"\']+["\']',
    r'api[_-]?key\s*[:=]\s*["\'][^"\']+["\']',
    r'secret[_-]?key\s*[:=]\s*["\'][^"\']+["\']',
    r'access[_-]?token\s*[:=]\s*["\'][^"\']+["\']',
    r'private[_-]?key\s*[:=]',
    r'AWS_SECRET_ACCESS_KEY',
    r'GITHUB_TOKEN',
    r'BEGIN\s+(RSA|DSA|EC|OPENSSH)\s+PRIVATE\s+KEY',
]


def check_sensitive_info(
    diff_content: str,
    allow_secrets: bool = False,
    custom_patterns: List[str] = None
) -> Tuple[bool, List[str]]:
    """
    Check for sensitive information in diff content.
    
    Property 21: Sensitive Information Detection
    *For any* commit, the system SHALL scan staged changes for common 
    secret patterns and block commits containing potential secrets 
    unless explicitly overridden.
    
    Returns: (is_allowed, list_of_matched_patterns)
    """
    custom_patterns = custom_patterns or []
    all_patterns = DEFAULT_SECRET_PATTERNS + custom_patterns
    
    matched_patterns = []
    
    for pattern in all_patterns:
        if re.search(pattern, diff_content, re.IGNORECASE):
            matched_patterns.append(pattern)
    
    if matched_patterns:
        # Check if explicitly allowed (Req 29.4)
        if allow_secrets:
            return True, matched_patterns  # Allowed but reported
        return False, matched_patterns
    
    return True, []


# ============================================================
# Tests
# ============================================================

class TestScriptModificationProtection:
    """Test script modification protection.
    
    Property 17: Script Modification Protection
    """

    def test_script_modification_blocked(self):
        """Test script modifications are blocked by default (Req 25.1)."""
        changed_files = [".ai/scripts/run_issue_codex.sh"]
        
        is_allowed, violations = check_script_modifications(changed_files)
        
        assert is_allowed is False
        assert ".ai/scripts/run_issue_codex.sh" in violations

    def test_command_modification_blocked(self):
        """Test command modifications are blocked by default (Req 25.1)."""
        changed_files = [".ai/commands/start-work.md"]
        
        is_allowed, violations = check_script_modifications(changed_files)
        
        assert is_allowed is False
        assert ".ai/commands/start-work.md" in violations

    def test_script_modification_allowed_with_flag(self):
        """Test script modifications allowed with approval flag (Req 25.2)."""
        changed_files = [".ai/scripts/run_issue_codex.sh"]
        
        is_allowed, violations = check_script_modifications(
            changed_files, 
            allow_script_changes=True
        )
        
        assert is_allowed is True
        assert len(violations) == 1  # Still reported

    def test_whitelisted_script_allowed(self):
        """Test whitelisted scripts are allowed (Req 25.3)."""
        changed_files = [".ai/scripts/run_issue_codex.sh"]
        whitelist = [".ai/scripts/run_issue_codex.sh"]
        
        is_allowed, violations = check_script_modifications(
            changed_files,
            whitelist=whitelist
        )
        
        assert is_allowed is True
        assert len(violations) == 0

    def test_non_script_files_allowed(self):
        """Test non-script files are allowed (Req 25.4)."""
        changed_files = ["backend/main.go", "README.md"]
        
        is_allowed, violations = check_script_modifications(changed_files)
        
        assert is_allowed is True
        assert len(violations) == 0

    def test_mixed_files_partial_block(self):
        """Test mixed files with some scripts (Req 25.5)."""
        changed_files = [
            "backend/main.go",
            ".ai/scripts/run_issue_codex.sh",
            "README.md"
        ]
        
        is_allowed, violations = check_script_modifications(changed_files)
        
        assert is_allowed is False
        assert len(violations) == 1
        assert ".ai/scripts/run_issue_codex.sh" in violations


class TestSensitiveInfoDetection:
    """Test sensitive information detection.
    
    Property 21: Sensitive Information Detection
    """

    def test_detect_password(self):
        """Test detection of password patterns (Req 29.1)."""
        diff = 'password = "secret123"'
        
        is_allowed, patterns = check_sensitive_info(diff)
        
        assert is_allowed is False
        assert len(patterns) > 0

    def test_detect_api_key(self):
        """Test detection of API key patterns (Req 29.1)."""
        diff = 'api_key = "sk-1234567890"'
        
        is_allowed, patterns = check_sensitive_info(diff)
        
        assert is_allowed is False
        assert len(patterns) > 0

    def test_detect_aws_secret(self):
        """Test detection of AWS secret (Req 29.1)."""
        diff = 'AWS_SECRET_ACCESS_KEY=abcdef123456'
        
        is_allowed, patterns = check_sensitive_info(diff)
        
        assert is_allowed is False
        assert len(patterns) > 0

    def test_detect_private_key(self):
        """Test detection of private key (Req 29.1)."""
        diff = '-----BEGIN RSA PRIVATE KEY-----'
        
        is_allowed, patterns = check_sensitive_info(diff)
        
        assert is_allowed is False
        assert len(patterns) > 0

    def test_detect_github_token(self):
        """Test detection of GitHub token (Req 29.1)."""
        diff = 'GITHUB_TOKEN=ghp_1234567890'
        
        is_allowed, patterns = check_sensitive_info(diff)
        
        assert is_allowed is False
        assert len(patterns) > 0

    def test_no_secrets_allowed(self):
        """Test content without secrets is allowed (Req 29.2)."""
        diff = '''
func main() {
    fmt.Println("Hello, World!")
}
'''
        
        is_allowed, patterns = check_sensitive_info(diff)
        
        assert is_allowed is True
        assert len(patterns) == 0

    def test_secrets_allowed_with_flag(self):
        """Test secrets allowed with override flag (Req 29.4)."""
        diff = 'password = "secret123"'
        
        is_allowed, patterns = check_sensitive_info(diff, allow_secrets=True)
        
        assert is_allowed is True
        assert len(patterns) > 0  # Still reported

    def test_custom_patterns(self):
        """Test custom secret patterns (Req 29.3)."""
        diff = 'MY_CUSTOM_SECRET=abc123'
        custom = [r'MY_CUSTOM_SECRET']
        
        is_allowed, patterns = check_sensitive_info(diff, custom_patterns=custom)
        
        assert is_allowed is False
        assert any('MY_CUSTOM_SECRET' in p for p in patterns)


class TestSecurityEdgeCases:
    """Test security edge cases."""

    def test_empty_changed_files(self):
        """Test empty changed files list."""
        is_allowed, violations = check_script_modifications([])
        
        assert is_allowed is True
        assert len(violations) == 0

    def test_empty_diff_content(self):
        """Test empty diff content."""
        is_allowed, patterns = check_sensitive_info("")
        
        assert is_allowed is True
        assert len(patterns) == 0

    def test_case_insensitive_secret_detection(self):
        """Test case insensitive secret detection (Req 29.5)."""
        diff = 'PASSWORD = "secret123"'
        
        is_allowed, patterns = check_sensitive_info(diff)
        
        assert is_allowed is False

    def test_nested_script_path(self):
        """Test nested script path detection."""
        changed_files = [".ai/scripts/subdir/script.sh"]
        
        is_allowed, violations = check_script_modifications(changed_files)
        
        assert is_allowed is False
        assert ".ai/scripts/subdir/script.sh" in violations


class TestProtectedPaths:
    """Test protected path definitions."""

    def test_scripts_path_protected(self):
        """Test .ai/scripts/ is protected."""
        assert ".ai/scripts/" in PROTECTED_PATHS

    def test_commands_path_protected(self):
        """Test .ai/commands/ is protected."""
        assert ".ai/commands/" in PROTECTED_PATHS

    @pytest.mark.parametrize("path,should_block", [
        (".ai/scripts/run.sh", True),
        (".ai/commands/start.md", True),
        (".ai/config/workflow.yaml", False),
        (".ai/rules/custom.md", False),
        ("backend/main.go", False),
    ])
    def test_path_protection(self, path, should_block):
        """Test various paths for protection status."""
        is_allowed, violations = check_script_modifications([path])
        
        if should_block:
            assert is_allowed is False
            assert path in violations
        else:
            assert is_allowed is True
            assert len(violations) == 0
