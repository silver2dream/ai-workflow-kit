#!/usr/bin/env python3
"""
Validate workflow.yaml against JSON Schema.
Usage: python3 .ai/scripts/validate_config.py [config_path]
"""
import sys
import os
import json

def main():
    script_dir = os.path.dirname(os.path.abspath(__file__))
    ai_root = os.path.dirname(script_dir)
    
    config_path = sys.argv[1] if len(sys.argv) > 1 else os.path.join(ai_root, 'config', 'workflow.yaml')
    schema_path = os.path.join(ai_root, 'config', 'workflow.schema.json')
    
    # Check dependencies
    try:
        import yaml
    except ImportError:
        print("[validate] ERROR: Missing dependency: pyyaml")
        print("[validate] Please install: pip3 install pyyaml")
        sys.exit(1)
    
    try:
        import jsonschema
    except ImportError:
        print("[validate] ERROR: Missing dependency: jsonschema")
        print("[validate] Please install: pip3 install jsonschema")
        sys.exit(1)
    
    # Load schema
    if not os.path.exists(schema_path):
        print(f"[validate] ERROR: Schema not found: {schema_path}")
        sys.exit(1)
    
    with open(schema_path, 'r', encoding='utf-8') as f:
        schema = json.load(f)
    
    # Load config
    if not os.path.exists(config_path):
        print(f"[validate] ERROR: Config not found: {config_path}")
        sys.exit(1)
    
    with open(config_path, 'r', encoding='utf-8') as f:
        config = yaml.safe_load(f)
    
    # Validate
    try:
        jsonschema.validate(instance=config, schema=schema)
        print(f"[validate] ✅ Config is valid: {config_path}")
        
        # Additional semantic checks
        errors = []
        warnings = []
        
        # Check repos
        for repo in config.get('repos', []):
            repo_name = repo.get('name', 'unknown')
            
            # Check path exists (warning only)
            repo_path = repo.get('path', '')
            if repo_path and repo_path != './' and not os.path.exists(os.path.join(os.path.dirname(ai_root), repo_path)):
                warnings.append(f"Repo '{repo_name}': path '{repo_path}' does not exist")
        
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
        
        # Report
        if warnings:
            print(f"\n[validate] ⚠️  Warnings ({len(warnings)}):")
            for w in warnings:
                print(f"  - {w}")
        
        if errors:
            print(f"\n[validate] ❌ Errors ({len(errors)}):")
            for e in errors:
                print(f"  - {e}")
            sys.exit(1)
        
        sys.exit(0)
        
    except jsonschema.ValidationError as e:
        print(f"[validate] ❌ Validation failed:")
        print(f"  Path: {' -> '.join(str(p) for p in e.absolute_path)}")
        print(f"  Error: {e.message}")
        sys.exit(1)

if __name__ == '__main__':
    main()
