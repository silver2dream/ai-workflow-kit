"""
Unit tests for worker prompt generation functionality.

**Feature: multi-repo-support**
**Property 9: Worker Prompt Contains Repo Context**
**Validates: Requirements 10.1, 10.2, 10.3, 10.4, 10.5**

Note: These tests validate the worker prompt generation logic.
"""
import pytest
from pathlib import Path


def generate_work_dir_instruction(
    repo_type: str,
    repo_path: str,
    work_dir: str
) -> str:
    """
    Generate work directory instruction for worker prompt.
    
    Property 9: Worker Prompt Contains Repo Context
    *For any* non-root repo type, the worker prompt SHALL contain:
    - WORK_DIR path
    - Instructions about file path relativity
    - For submodule: warning not to modify files outside submodule
    """
    if repo_type == "root":
        # Root type: no special path instructions (Req 10.1)
        return ""
    
    elif repo_type == "directory":
        # Directory type: explain paths relative to monorepo root (Req 10.2, 10.4)
        return f"""
IMPORTANT: You are working in a MONOREPO (directory type).
- Working directory: {work_dir}
- All file paths should be relative to the worktree root
- Example: {repo_path}/internal/foo.go (not internal/foo.go)
"""
    
    elif repo_type == "submodule":
        # Submodule type: warn about file boundary (Req 10.3, 10.5)
        return f"""
IMPORTANT: You are working in a SUBMODULE within a monorepo.
- Submodule path: {repo_path}
- Working directory: {work_dir}
- WARNING: Do NOT modify files outside the submodule directory!
- All changes must be within: {repo_path}/
- Files outside this boundary will cause the commit to fail.
"""
    
    return ""


def build_worker_prompt(
    repo_type: str,
    repo_path: str,
    work_dir: str,
    task_content: str
) -> str:
    """Build complete worker prompt with repo context."""
    work_dir_instruction = generate_work_dir_instruction(repo_type, repo_path, work_dir)
    
    return f"""You are an automated coding agent running inside a git worktree.

Repo rules:
- Read and follow CLAUDE.md and AGENTS.md.
- Keep changes minimal and strictly within ticket scope.
{work_dir_instruction}
IMPORTANT: Do NOT run any git commands (commit, push, etc.) or create PRs.
The runner script will handle git operations after you complete the code changes.
Your job is ONLY to:
1. Write/modify code files
2. Run verification commands
3. Report the results

Ticket:
{task_content}

After making changes:
- Print: git status --porcelain
- Print: git diff
- Run verification commands from the ticket.
- Do NOT commit or push - the runner will handle that.
"""


class TestWorkerPromptRepoContext:
    """Test worker prompt contains repo context.
    
    Property 9: Worker Prompt Contains Repo Context
    """

    def test_root_type_no_special_instructions(self):
        """Test root type has no special path instructions (Req 10.1)."""
        instruction = generate_work_dir_instruction("root", "./", "/worktree")
        
        assert instruction == ""

    def test_directory_type_has_work_dir(self):
        """Test directory type includes WORK_DIR path (Req 10.2)."""
        work_dir = "/worktree/backend"
        instruction = generate_work_dir_instruction("directory", "backend", work_dir)
        
        assert work_dir in instruction
        assert "Working directory:" in instruction

    def test_directory_type_has_path_relativity(self):
        """Test directory type explains path relativity (Req 10.4)."""
        instruction = generate_work_dir_instruction("directory", "backend", "/worktree/backend")
        
        assert "relative to the worktree root" in instruction
        assert "backend/" in instruction

    def test_directory_type_has_example(self):
        """Test directory type includes example path."""
        instruction = generate_work_dir_instruction("directory", "backend", "/worktree/backend")
        
        assert "Example:" in instruction
        assert "backend/" in instruction

    def test_submodule_type_has_work_dir(self):
        """Test submodule type includes WORK_DIR path (Req 10.3)."""
        work_dir = "/worktree/libs/shared"
        instruction = generate_work_dir_instruction("submodule", "libs/shared", work_dir)
        
        assert work_dir in instruction
        assert "Working directory:" in instruction

    def test_submodule_type_has_warning(self):
        """Test submodule type has warning about file boundary (Req 10.5)."""
        instruction = generate_work_dir_instruction("submodule", "libs/shared", "/worktree/libs/shared")
        
        assert "WARNING" in instruction
        assert "Do NOT modify files outside" in instruction

    def test_submodule_type_has_boundary_info(self):
        """Test submodule type specifies boundary."""
        instruction = generate_work_dir_instruction("submodule", "libs/shared", "/worktree/libs/shared")
        
        assert "libs/shared/" in instruction
        assert "All changes must be within" in instruction


class TestWorkerPromptComplete:
    """Test complete worker prompt generation."""

    def test_prompt_includes_repo_rules(self):
        """Test prompt includes repo rules section."""
        prompt = build_worker_prompt("root", "./", "/worktree", "Test task")
        
        assert "Repo rules:" in prompt
        assert "CLAUDE.md" in prompt
        assert "AGENTS.md" in prompt

    def test_prompt_includes_task_content(self):
        """Test prompt includes task content."""
        task = "# Test Task\n\nDo something"
        prompt = build_worker_prompt("root", "./", "/worktree", task)
        
        assert "Ticket:" in prompt
        assert task in prompt

    def test_prompt_includes_git_warning(self):
        """Test prompt warns not to run git commands."""
        prompt = build_worker_prompt("root", "./", "/worktree", "Test task")
        
        assert "Do NOT run any git commands" in prompt
        assert "Do NOT commit or push" in prompt

    def test_prompt_includes_verification_instructions(self):
        """Test prompt includes verification instructions."""
        prompt = build_worker_prompt("root", "./", "/worktree", "Test task")
        
        assert "git status --porcelain" in prompt
        assert "git diff" in prompt
        assert "verification commands" in prompt

    @pytest.mark.parametrize("repo_type,should_have_instruction", [
        ("root", False),
        ("directory", True),
        ("submodule", True),
    ])
    def test_prompt_has_work_dir_instruction_based_on_type(self, repo_type, should_have_instruction):
        """Test prompt includes work dir instruction based on repo type."""
        prompt = build_worker_prompt(repo_type, "backend", "/worktree/backend", "Test task")
        
        has_instruction = "IMPORTANT: You are working in" in prompt
        assert has_instruction == should_have_instruction


class TestWorkerPromptEdgeCases:
    """Test edge cases for worker prompt generation."""

    def test_unknown_repo_type_returns_empty(self):
        """Test unknown repo type returns empty instruction."""
        instruction = generate_work_dir_instruction("unknown", "path", "/worktree")
        
        assert instruction == ""

    def test_empty_repo_path(self):
        """Test empty repo path is handled."""
        instruction = generate_work_dir_instruction("directory", "", "/worktree")
        
        assert "Working directory:" in instruction

    def test_nested_repo_path(self):
        """Test nested repo path is handled."""
        instruction = generate_work_dir_instruction("submodule", "libs/shared/core", "/worktree/libs/shared/core")
        
        assert "libs/shared/core" in instruction
