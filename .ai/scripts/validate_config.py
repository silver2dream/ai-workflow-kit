#!/usr/bin/env python3
"""
Validate workflow.yaml against JSON Schema.
Usage: python3 .ai/scripts/validate_config.py [config_path]
"""
import sys
import os
import json

# Add scripts directory to Python path for lib imports
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
if SCRIPT_DIR not in sys.path:
    sys.path.insert(0, SCRIPT_DIR)

from lib.errors import AWKError, ConfigError, ValidationError, handle_unexpected_error, print_error
from lib.logger import Logger, split_log_level


def _format_schema_error(error) -> dict:
    path = ".".join(str(p) for p in error.absolute_path) if error.absolute_path else "<root>"
    expected = ""
    schema = getattr(error, "schema", {}) or {}
    if "enum" in schema:
        expected = f"one of {schema['enum']}"
    elif "type" in schema:
        expected = schema["type"]
    elif "minimum" in schema:
        expected = f">= {schema['minimum']}"
    elif "maximum" in schema:
        expected = f"<= {schema['maximum']}"
    return {
        "path": path,
        "message": error.message,
        "expected": expected,
        "validator": getattr(error, "validator", ""),
    }


def main():
    script_dir = os.path.dirname(os.path.abspath(__file__))
    ai_root = os.path.dirname(script_dir)
    
    args = sys.argv[1:]
    log_level, args, log_error = split_log_level(args)
    if log_error:
        print_error(ConfigError(log_error))
        sys.exit(2)

    config_path = args[0] if args else os.path.join(ai_root, 'config', 'workflow.yaml')
    schema_path = os.path.join(ai_root, 'config', 'workflow.schema.json')
    logger = Logger("validate_config", os.path.join(ai_root, 'logs'), level=log_level)

    # Check dependencies
    try:
        import yaml
    except ImportError as exc:
        err = ConfigError(
            "Missing dependency: pyyaml",
            suggestion="Please install: pip3 install pyyaml",
        )
        logger.error("dependency missing", {"dependency": "pyyaml"})
        print_error(err)
        sys.exit(err.exit_code)

    try:
        import jsonschema
    except ImportError as exc:
        err = ConfigError(
            "Missing dependency: jsonschema",
            suggestion="Please install: pip3 install jsonschema",
        )
        logger.error("dependency missing", {"dependency": "jsonschema"})
        print_error(err)
        sys.exit(err.exit_code)

    try:
        logger.info("validate start", {"config_path": config_path})
        # Load schema
        if not os.path.exists(schema_path):
            raise ConfigError(
                f"Schema not found: {schema_path}",
                suggestion="Regenerate the kit files or restore the schema.",
            )

        with open(schema_path, 'r', encoding='utf-8') as f:
            schema = json.load(f)

        # Load config
        if not os.path.exists(config_path):
            raise ConfigError(
                f"Config not found: {config_path}",
                suggestion="Check the path or run the kit initializer.",
            )

        with open(config_path, 'r', encoding='utf-8') as f:
            config = yaml.safe_load(f)

        # Validate
        try:
            jsonschema.validate(instance=config, schema=schema)
        except jsonschema.ValidationError as e:
            details = _format_schema_error(e)
            raise ValidationError(
                "Schema validation failed.",
                details=details,
                suggestion="Update the invalid fields and retry.",
            ) from e

        print(f"[validate] Config is valid: {config_path}")

        # Additional semantic checks
        errors = []
        warnings = []

        # Check repos with type-specific validation
        mono_root = os.path.dirname(ai_root)
        gitmodules_path = os.path.join(mono_root, '.gitmodules')
        gitmodules_content = ''
        gitmodules_paths = []
        if os.path.exists(gitmodules_path):
            with open(gitmodules_path, 'r', encoding='utf-8') as f:
                gitmodules_content = f.read()
            # Parse submodule paths from .gitmodules
            import re
            gitmodules_paths = re.findall(r'path\s*=\s*(.+)', gitmodules_content)
            gitmodules_paths = [p.strip() for p in gitmodules_paths]

        for repo in config.get('repos', []):
            repo_name = repo.get('name', 'unknown')
            repo_path = repo.get('path', '').rstrip('/')
            repo_type = repo.get('type', 'directory')
            full_path = os.path.join(mono_root, repo_path) if repo_path else mono_root

            # Security: Path traversal prevention (Req 22.1-22.4)
            if '..' in repo_path:
                errors.append(f"Repo '{repo_name}': path '{repo_path}' contains '..' (path traversal not allowed)")
                continue
            
            # Verify path is within worktree (Req 9.4)
            if repo_path:
                try:
                    resolved_path = os.path.realpath(full_path)
                    resolved_root = os.path.realpath(mono_root)
                    if not resolved_path.startswith(resolved_root):
                        errors.append(f"Repo '{repo_name}': path '{repo_path}' resolves outside monorepo root")
                        continue
                except Exception:
                    pass  # Path doesn't exist yet, skip this check

            # Type-specific validation
            if repo_type == 'submodule':
                # Req 1.5, 1.6, 9.1, 9.2, 26.1-26.4: Submodule validation
                if not os.path.exists(gitmodules_path):
                    errors.append(f"Repo '{repo_name}': type=submodule but .gitmodules not found (Req 26.1)")
                elif repo_path and repo_path not in gitmodules_paths:
                    errors.append(f"Repo '{repo_name}': type=submodule but path '{repo_path}' not in .gitmodules (Req 26.2)")
                
                # Check submodule has .git (Req 26.3)
                if repo_path and os.path.exists(full_path):
                    git_path = os.path.join(full_path, '.git')
                    if not os.path.exists(git_path):
                        errors.append(f"Repo '{repo_name}': type=submodule but '{repo_path}' has no .git file/directory (Req 26.3)")
                    else:
                        # Verify remote URL matches .gitmodules (Req 26.4)
                        try:
                            import subprocess
                            result = subprocess.run(
                                ['git', '-C', full_path, 'remote', 'get-url', 'origin'],
                                capture_output=True, text=True, timeout=5
                            )
                            if result.returncode == 0:
                                actual_remote = result.stdout.strip()
                                # Parse expected remote from .gitmodules
                                url_pattern = rf'submodule\s+["\']?{re.escape(repo_path)}["\']?\s*\].*?url\s*=\s*(.+)'
                                url_match = re.search(url_pattern, gitmodules_content, re.DOTALL | re.IGNORECASE)
                                if url_match:
                                    expected_remote = url_match.group(1).strip()
                                    if actual_remote != expected_remote:
                                        warnings.append(
                                            f"Repo '{repo_name}': submodule remote URL mismatch. "
                                            f"Expected: {expected_remote}, Actual: {actual_remote} (Req 26.4)"
                                        )
                        except Exception:
                            pass  # Skip remote validation if git command fails

            elif repo_type == 'directory':
                # Req 9.3, 9.5: Directory type validation
                if repo_path and not os.path.isdir(full_path):
                    warnings.append(f"Repo '{repo_name}': path '{repo_path}' does not exist or is not a directory (Req 9.5)")
                elif repo_path and os.path.exists(os.path.join(full_path, '.git')):
                    warnings.append(
                        f"Repo '{repo_name}': type=directory but '{repo_path}' has .git (consider type=submodule?) (Req 9.3)"
                    )

            elif repo_type == 'root':
                # Req 9.4: Root type path validation
                if repo_path not in ['.', './', '']:
                    errors.append(f"Repo '{repo_name}': type=root but path is '{repo_path}' (should be './' or empty) (Req 9.4)")

        # Check rules files exist
        rules_dir = os.path.join(ai_root, 'rules')
        kit_rules_dir = os.path.join(rules_dir, '_kit')

        for rule in config.get('rules', {}).get('kit', []):
            rule_path = os.path.join(kit_rules_dir, f'{rule}.md')
            if not os.path.exists(rule_path):
                warnings.append(f"Kit rule not found: {rule_path} (run generate.sh)")

        for rule in config.get('rules', {}).get('custom', []):
            rule_path = os.path.join(rules_dir, f'{rule}.md')
            if not os.path.exists(rule_path):
                errors.append(f"Custom rule not found: {rule_path}")

        # Check specs exist
        specs_base = config.get('specs', {}).get('base_path', '.ai/specs')
        for spec in config.get('specs', {}).get('active', []):
            spec_path = os.path.join(os.path.dirname(ai_root), specs_base, spec)
            if not os.path.exists(spec_path):
                warnings.append(f"Active spec not found: {spec_path}")

        # Validate state files if they exist
        state_dir = os.path.join(ai_root, 'state')

        # Validate repo_scan.json
        repo_scan_path = os.path.join(state_dir, 'repo_scan.json')
        repo_scan_schema_path = os.path.join(ai_root, 'config', 'repo_scan.schema.json')
        if os.path.exists(repo_scan_path) and os.path.exists(repo_scan_schema_path):
            with open(repo_scan_schema_path, 'r', encoding='utf-8') as f:
                repo_scan_schema = json.load(f)
            with open(repo_scan_path, 'r', encoding='utf-8') as f:
                repo_scan = json.load(f)
            try:
                jsonschema.validate(instance=repo_scan, schema=repo_scan_schema)
            except jsonschema.ValidationError as e:
                warnings.append(f"repo_scan.json schema mismatch: {e.message}")

        # Validate audit.json
        audit_path = os.path.join(state_dir, 'audit.json')
        audit_schema_path = os.path.join(ai_root, 'config', 'audit.schema.json')
        if os.path.exists(audit_path) and os.path.exists(audit_schema_path):
            with open(audit_schema_path, 'r', encoding='utf-8') as f:
                audit_schema = json.load(f)
            with open(audit_path, 'r', encoding='utf-8') as f:
                audit = json.load(f)
            try:
                jsonschema.validate(instance=audit, schema=audit_schema)
            except jsonschema.ValidationError as e:
                warnings.append(f"audit.json schema mismatch: {e.message}")

        if warnings:
            print(f"\n[validate] Warnings ({len(warnings)}):")
            for w in warnings:
                print(f"  - {w}")

        if errors:
            raise ValidationError(
                "Semantic validation failed.",
                details={"errors": errors},
                suggestion="Fix the configuration issues and re-run validation.",
            )

        logger.info("validate complete", {"config_path": config_path})
        sys.exit(0)
    except AWKError as err:
        logger.error("validate failed", {"error": err.message})
        print_error(err)
        sys.exit(err.exit_code)
    except Exception as exc:
        err = handle_unexpected_error(exc)
        logger.error("validate failed", {"error": str(exc)})
        print_error(err)
        sys.exit(err.exit_code)

if __name__ == '__main__':
    main()
