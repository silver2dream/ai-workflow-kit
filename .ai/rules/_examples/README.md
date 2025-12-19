# Optional Example Rules

AWK ships with a minimal, generic default rule set under `.ai/rules/_kit/`.

This directory contains **optional example rule packs** that you can adopt for your project.

Available examples:
- `backend-go.md`
- `frontend-react.md`

## How To Enable An Example Rule Pack

1) Copy the example file into `.ai/rules/`:

- Example: copy `.ai/rules/_examples/backend-go.md` → `.ai/rules/backend-go.md`

2) Add the rule name (filename without `.md`) to `.ai/config/workflow.yaml`:

```yaml
rules:
  kit:
    - git-workflow
  custom:
    - backend-go
```

3) Regenerate helper docs (recommended):

```bash
bash .ai/scripts/generate.sh
```

This refreshes `AGENTS.md` and `CLAUDE.md` so agents will be instructed to read the enabled custom rules.

