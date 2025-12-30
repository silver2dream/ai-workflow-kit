"""Shared error handling for AI Workflow Kit scripts."""
from __future__ import annotations

import json
import sys
from dataclasses import dataclass, field
from typing import Any, Dict, Optional


EXIT_SUCCESS = 0
EXIT_ERROR = 1
EXIT_CONFIG_ERROR = 2
EXIT_VALIDATION_ERROR = 3


_TEMPLATES = {
    "config_error": {
        "reason": "Required configuration or dependency is missing.",
        "impact": "The workflow cannot continue with the current setup.",
        "suggestion": "Fix the configuration or install the missing dependency.",
    },
    "validation_error": {
        "reason": "Configuration failed validation.",
        "impact": "The workflow may behave unpredictably with invalid settings.",
        "suggestion": "Correct the invalid fields and re-run validation.",
    },
    "execution_error": {
        "reason": "An unexpected runtime error occurred.",
        "impact": "The current command did not complete successfully.",
        "suggestion": "Review the error details and try again.",
    },
}


@dataclass
class AWKError(Exception):
    message: str
    error_type: str = "execution_error"
    exit_code: int = EXIT_ERROR
    reason: Optional[str] = None
    impact: Optional[str] = None
    suggestion: Optional[str] = None
    details: Dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> Dict[str, Any]:
        template = _TEMPLATES.get(self.error_type, {})
        return {
            "error": {
                "type": self.error_type,
                "code": self.exit_code,
                "message": self.message,
                "reason": self.reason or template.get("reason", ""),
                "impact": self.impact or template.get("impact", ""),
                "suggestion": self.suggestion or template.get("suggestion", ""),
                "details": self.details or {},
            }
        }


class ConfigError(AWKError):
    def __init__(self, message: str, **kwargs: Any) -> None:
        super().__init__(
            message=message,
            error_type="config_error",
            exit_code=EXIT_CONFIG_ERROR,
            **kwargs,
        )


class ValidationError(AWKError):
    def __init__(self, message: str, **kwargs: Any) -> None:
        super().__init__(
            message=message,
            error_type="validation_error",
            exit_code=EXIT_VALIDATION_ERROR,
            **kwargs,
        )


class ExecutionError(AWKError):
    def __init__(self, message: str, **kwargs: Any) -> None:
        super().__init__(
            message=message,
            error_type="execution_error",
            exit_code=EXIT_ERROR,
            **kwargs,
        )


def print_error(err: AWKError, stream: Any = None) -> None:
    target = stream if stream is not None else sys.stderr
    payload = err.to_dict()
    target.write(json.dumps(payload, ensure_ascii=True))
    target.write("\n")


def handle_unexpected_error(exc: Exception) -> AWKError:
    return ExecutionError(str(exc), details={"exception": exc.__class__.__name__})
