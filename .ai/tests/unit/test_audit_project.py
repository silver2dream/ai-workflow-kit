"""
Unit tests for audit_project.py
"""
import pytest
import subprocess
import json
import sys
import os
from pathlib import Path

# Add scripts directory to path for imports
SCRIPTS_DIR = Path(__file__).parent.parent.parent / "scripts"
sys.path.insert(0, str(SCRIPTS_DIR))


class TestAuditProjectFunctions:
    """Test individual functions from audit_project.py"""

    def test_import_audit_project(self):
        """Test that audit_project.py can be imported."""
        import audit_project
        assert hasattr(audit_project, 'audit_project')
        assert hasattr(audit_project, 'get_repo_root')
        assert hasattr(audit_project, 'is_clean')

    def test_is_clean_on_clean_repo(self, temp_git_repo):
        """Test is_clean returns True for clean repo."""
        import audit_project
        assert audit_project.is_clean(temp_git_repo) is True

    def test_is_clean_on_dirty_repo(self, temp_git_repo):
        """Test is_clean returns False for dirty repo."""
        import audit_project

        # Create an uncommitted file
        (temp_git_repo / "dirty.txt").write_text("dirty content")
        assert audit_project.is_clean(temp_git_repo) is False


class TestAuditProjectFindings:
    """Test audit_project finding generation."""

    def test_audit_detects_missing_claude_md(self, temp_git_repo):
        """Test audit detects missing CLAUDE.md as P0."""
        import audit_project

        # Create minimal .ai structure
        ai_dir = temp_git_repo / ".ai"
        (ai_dir / "config").mkdir(parents=True)
        (ai_dir / "scripts").mkdir(parents=True)
        (ai_dir / "rules" / "_kit").mkdir(parents=True)
        (ai_dir / "config" / "workflow.yaml").write_text("version: '1.0'\n")

        result = audit_project.audit_project(temp_git_repo)

        # Find the CLAUDE.md finding
        claude_findings = [f for f in result["findings"] if "CLAUDE.md" in f["message"]]
        assert len(claude_findings) == 1
        assert claude_findings[0]["severity"] == "P0"

    def test_audit_detects_missing_agents_md(self, temp_git_repo):
        """Test audit detects missing AGENTS.md as P0."""
        import audit_project

        # Create CLAUDE.md but not AGENTS.md
        (temp_git_repo / "CLAUDE.md").write_text("# CLAUDE.md\n")

        ai_dir = temp_git_repo / ".ai"
        (ai_dir / "config").mkdir(parents=True)
        (ai_dir / "scripts").mkdir(parents=True)
        (ai_dir / "rules" / "_kit").mkdir(parents=True)
        (ai_dir / "config" / "workflow.yaml").write_text("version: '1.0'\n")

        result = audit_project.audit_project(temp_git_repo)

        # Find the AGENTS.md finding
        agents_findings = [f for f in result["findings"] if "AGENTS.md" in f["message"]]
        assert len(agents_findings) == 1
        assert agents_findings[0]["severity"] == "P0"

    def test_audit_detects_missing_workflow_yaml(self, temp_git_repo):
        """Test audit detects missing workflow.yaml as P0."""
        import audit_project

        # Create CLAUDE.md and AGENTS.md but not workflow.yaml
        (temp_git_repo / "CLAUDE.md").write_text("# CLAUDE.md\n")
        (temp_git_repo / "AGENTS.md").write_text("# AGENTS.md\n")

        result = audit_project.audit_project(temp_git_repo)

        # Find the workflow.yaml finding
        yaml_findings = [f for f in result["findings"] if "workflow" in f["message"].lower()]
        assert len(yaml_findings) >= 1
        assert yaml_findings[0]["severity"] == "P0"

    def test_audit_detects_dirty_worktree_as_p1(self, temp_git_repo):
        """Test audit detects dirty worktree as P1 (not P0)."""
        import audit_project

        # Create all required files
        (temp_git_repo / "CLAUDE.md").write_text("# CLAUDE.md\n")
        (temp_git_repo / "AGENTS.md").write_text("# AGENTS.md\n")

        ai_dir = temp_git_repo / ".ai"
        (ai_dir / "config").mkdir(parents=True)
        (ai_dir / "scripts").mkdir(parents=True)
        (ai_dir / "rules" / "_kit").mkdir(parents=True)
        (ai_dir / "config" / "workflow.yaml").write_text("version: '1.0'\n")

        # Make it dirty
        (temp_git_repo / "uncommitted.txt").write_text("dirty\n")

        result = audit_project.audit_project(temp_git_repo)

        # Find the dirty worktree finding
        dirty_findings = [f for f in result["findings"] if f["type"] == "dirty_worktree"]
        assert len(dirty_findings) == 1
        assert dirty_findings[0]["severity"] == "P1"  # P1, not P0

    def test_audit_no_findings_for_complete_repo(self, mock_repo_structure):
        """Test audit has no P0 findings for complete repo."""
        import audit_project

        result = audit_project.audit_project(mock_repo_structure)

        p0_findings = [f for f in result["findings"] if f["severity"] == "P0"]
        assert len(p0_findings) == 0


class TestAuditProjectSummary:
    """Test audit_project summary calculation."""

    def test_summary_counts_severities(self, temp_git_repo):
        """Test summary correctly counts P0/P1/P2."""
        import audit_project

        # Create an incomplete repo to trigger multiple findings
        result = audit_project.audit_project(temp_git_repo)

        summary = result["summary"]
        assert "p0" in summary
        assert "p1" in summary
        assert "p2" in summary
        assert "total" in summary

        # Total should equal sum of p0+p1+p2
        assert summary["total"] == summary["p0"] + summary["p1"] + summary["p2"]

    def test_summary_total_matches_findings(self, temp_git_repo):
        """Test summary total matches findings list length."""
        import audit_project

        result = audit_project.audit_project(temp_git_repo)

        assert result["summary"]["total"] == len(result["findings"])


class TestAuditProjectCLI:
    """Test audit_project.py CLI execution."""

    def test_audit_cli_json_output(self, mock_repo_structure):
        """Test audit_project.py --json outputs valid JSON."""
        script_path = SCRIPTS_DIR / "audit_project.py"

        result = subprocess.run(
            [sys.executable, str(script_path), "--json"],
            cwd=mock_repo_structure,
            capture_output=True,
            text=True
        )

        assert result.returncode == 0
        # Should be valid JSON
        data = json.loads(result.stdout)
        assert "findings" in data
        assert "summary" in data

    def test_audit_cli_writes_state_file(self, mock_repo_structure):
        """Test audit_project.py writes state file."""
        script_path = SCRIPTS_DIR / "audit_project.py"

        result = subprocess.run(
            [sys.executable, str(script_path)],
            cwd=mock_repo_structure,
            capture_output=True,
            text=True
        )

        assert result.returncode == 0

        state_file = mock_repo_structure / ".ai" / "state" / "audit.json"
        assert state_file.exists()

        # Validate content
        with open(state_file) as f:
            data = json.load(f)
        assert "findings" in data
        assert "summary" in data

    def test_audit_cli_exit_code_on_p0(self, temp_git_repo):
        """Test audit_project.py exits with code 1 when P0 findings exist."""
        script_path = SCRIPTS_DIR / "audit_project.py"

        # temp_git_repo doesn't have required files, should have P0 findings
        result = subprocess.run(
            [sys.executable, str(script_path)],
            cwd=temp_git_repo,
            capture_output=True,
            text=True
        )

        # Should exit with 1 due to P0 findings
        assert result.returncode == 1

    def test_audit_cli_exit_code_on_clean(self, mock_repo_structure):
        """Test audit_project.py exits with code 0 when no P0 findings."""
        script_path = SCRIPTS_DIR / "audit_project.py"

        result = subprocess.run(
            [sys.executable, str(script_path)],
            cwd=mock_repo_structure,
            capture_output=True,
            text=True
        )

        # Should exit with 0 (no P0 findings)
        assert result.returncode == 0
