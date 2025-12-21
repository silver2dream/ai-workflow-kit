"""
Unit tests for errors.py
"""
import io
import json
import sys
from pathlib import Path

# Add scripts directory to path for imports
SCRIPTS_DIR = Path(__file__).parent.parent.parent / "scripts"
sys.path.insert(0, str(SCRIPTS_DIR))


def test_error_exit_codes():
    """Verify exit codes for error types."""
    from lib.errors import ConfigError, ValidationError, ExecutionError

    assert ConfigError("config").exit_code == 2
    assert ValidationError("validation").exit_code == 3
    assert ExecutionError("execution").exit_code == 1


def test_print_error_output():
    """Ensure print_error emits structured JSON."""
    from lib.errors import ConfigError, print_error

    buf = io.StringIO()
    print_error(ConfigError("config missing"), stream=buf)
    payload = json.loads(buf.getvalue())
    assert "error" in payload
    assert payload["error"]["message"] == "config missing"
    assert payload["error"]["code"] == 2
