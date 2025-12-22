"""
Unit tests for repo type detection functionality.

**Feature: multi-repo-support, Property 1: Repo Type Detection Consistency**
**Validates: Requirements 1.1, 1.2, 1.3, 1.4, 15.1, 15.2, 15.3, 15.4**
"""
import pytest
import yaml
from pathlib import Path


def get_repo_type(config: dict, repo_name: str) -> str:
    """Get repo type from config dict (same logic as run_issue_codex.sh)."""
    for repo in config.get('repos', []):
        if repo.get('name') == repo_name:
            return repo.get('type', 'directory')
    return 'directory'


def get_repo_path(config: dict, repo_name: str) -> str:
    """Get repo path from config dict (same logic as run_issue_codex.sh)."""
    for repo in config.get('repos', []):
        if repo.get('name') == repo_name:
            return repo.get('path', '.')
    return '.'


class TestRepoTypeDetection:
    """Test repo type detection from workflow.yaml.
    
    Property 1: Repo Type Detection Consistency
    *For any* valid workflow.yaml configuration and any repo name, 
    the system SHALL return the correct repo type as specified in the config, 
    or default to `directory` if not specified.
    """

    def test_detect_root_type(self, temp_git_repo):
        """Test detection of root type repo."""
        ai_config = temp_git_repo / ".ai" / "config"
        ai_config.mkdir(parents=True, exist_ok=True)
        
        config_content = """version: "1.0"
project:
  name: "test"
  type: "single-repo"
repos:
  - name: root
    path: ./
    type: root
    language: go
    verify:
      build: "go build ./..."
      test: "go test ./..."
git:
  integration_branch: "develop"
  release_branch: "main"
  commit_format: "[type] subject"
"""
        config_path = ai_config / "workflow.yaml"
        config_path.write_text(config_content)
        
        with open(config_path) as f:
            config = yaml.safe_load(f)
        
        assert get_repo_type(config, 'root') == 'root'

    def test_detect_directory_type(self, temp_git_repo):
        """Test detection of directory type repo."""
        ai_config = temp_git_repo / ".ai" / "config"
        ai_config.mkdir(parents=True, exist_ok=True)
        
        config_content = """version: "1.0"
project:
  name: "test"
  type: "monorepo"
repos:
  - name: backend
    path: backend/
    type: directory
    language: go
    verify:
      build: "go build ./..."
      test: "go test ./..."
git:
  integration_branch: "develop"
  release_branch: "main"
  commit_format: "[type] subject"
"""
        config_path = ai_config / "workflow.yaml"
        config_path.write_text(config_content)
        
        with open(config_path) as f:
            config = yaml.safe_load(f)
        
        assert get_repo_type(config, 'backend') == 'directory'

    def test_detect_submodule_type(self, temp_git_repo):
        """Test detection of submodule type repo."""
        ai_config = temp_git_repo / ".ai" / "config"
        ai_config.mkdir(parents=True, exist_ok=True)
        
        config_content = """version: "1.0"
project:
  name: "test"
  type: "monorepo"
repos:
  - name: backend
    path: backend/
    type: submodule
    language: go
    verify:
      build: "go build ./..."
      test: "go test ./..."
git:
  integration_branch: "develop"
  release_branch: "main"
  commit_format: "[type] subject"
"""
        config_path = ai_config / "workflow.yaml"
        config_path.write_text(config_content)
        
        with open(config_path) as f:
            config = yaml.safe_load(f)
        
        assert get_repo_type(config, 'backend') == 'submodule'

    def test_default_to_directory_when_type_missing(self, temp_git_repo):
        """Test default to directory when type is not specified."""
        ai_config = temp_git_repo / ".ai" / "config"
        ai_config.mkdir(parents=True, exist_ok=True)
        
        # Config without type field
        config_content = """version: "1.0"
project:
  name: "test"
  type: "monorepo"
repos:
  - name: backend
    path: backend/
    language: go
    verify:
      build: "go build ./..."
      test: "go test ./..."
git:
  integration_branch: "develop"
  release_branch: "main"
  commit_format: "[type] subject"
"""
        config_path = ai_config / "workflow.yaml"
        config_path.write_text(config_content)
        
        with open(config_path) as f:
            config = yaml.safe_load(f)
        
        assert get_repo_type(config, 'backend') == 'directory'

    def test_default_to_directory_when_repo_not_found(self, temp_git_repo):
        """Test default to directory when repo name not found."""
        ai_config = temp_git_repo / ".ai" / "config"
        ai_config.mkdir(parents=True, exist_ok=True)
        
        config_content = """version: "1.0"
project:
  name: "test"
  type: "monorepo"
repos:
  - name: backend
    path: backend/
    type: root
    language: go
    verify:
      build: "go build ./..."
      test: "go test ./..."
git:
  integration_branch: "develop"
  release_branch: "main"
  commit_format: "[type] subject"
"""
        config_path = ai_config / "workflow.yaml"
        config_path.write_text(config_content)
        
        with open(config_path) as f:
            config = yaml.safe_load(f)
        
        # Query for non-existent repo
        assert get_repo_type(config, 'nonexistent') == 'directory'

    def test_all_valid_repo_types(self):
        """Test all valid repo types are recognized."""
        valid_types = ['root', 'directory', 'submodule']
        
        for repo_type in valid_types:
            config = {
                'repos': [
                    {'name': 'test', 'type': repo_type}
                ]
            }
            assert get_repo_type(config, 'test') == repo_type


class TestRepoPathDetection:
    """Test repo path detection from workflow.yaml.
    
    Validates: Requirements 15.1, 15.2, 15.3, 15.4
    """

    def test_detect_repo_path(self, temp_git_repo):
        """Test detection of repo path."""
        ai_config = temp_git_repo / ".ai" / "config"
        ai_config.mkdir(parents=True, exist_ok=True)
        
        config_content = """version: "1.0"
project:
  name: "test"
  type: "monorepo"
repos:
  - name: backend
    path: backend/
    type: directory
    language: go
    verify:
      build: "go build ./..."
      test: "go test ./..."
git:
  integration_branch: "develop"
  release_branch: "main"
  commit_format: "[type] subject"
"""
        config_path = ai_config / "workflow.yaml"
        config_path.write_text(config_content)
        
        with open(config_path) as f:
            config = yaml.safe_load(f)
        
        assert get_repo_path(config, 'backend') == 'backend/'

    def test_root_type_path_is_dot(self, temp_git_repo):
        """Test root type has path ./ or ."""
        ai_config = temp_git_repo / ".ai" / "config"
        ai_config.mkdir(parents=True, exist_ok=True)
        
        config_content = """version: "1.0"
project:
  name: "test"
  type: "single-repo"
repos:
  - name: root
    path: ./
    type: root
    language: go
    verify:
      build: "go build ./..."
      test: "go test ./..."
git:
  integration_branch: "develop"
  release_branch: "main"
  commit_format: "[type] subject"
"""
        config_path = ai_config / "workflow.yaml"
        config_path.write_text(config_content)
        
        with open(config_path) as f:
            config = yaml.safe_load(f)
        
        path = get_repo_path(config, 'root')
        # Normalize path - ./ and . are equivalent
        assert path in ('./', '.', '')

    def test_default_path_when_not_specified(self):
        """Test default path is . when not specified."""
        config = {
            'repos': [
                {'name': 'test', 'type': 'root'}
            ]
        }
        assert get_repo_path(config, 'test') == '.'

    def test_default_path_when_repo_not_found(self):
        """Test default path is . when repo not found."""
        config = {
            'repos': [
                {'name': 'backend', 'path': 'backend/', 'type': 'directory'}
            ]
        }
        assert get_repo_path(config, 'nonexistent') == '.'
