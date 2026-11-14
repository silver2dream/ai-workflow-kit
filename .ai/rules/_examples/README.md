# Optional Example Rules

AWK ships with a minimal, generic default rule set under `.ai/rules/_kit/`.

This directory contains **optional example rule packs** that you can adopt for your project.

Available examples:

### Methodology Rules
- `testing-strategy.md` — Test pyramid, coverage, mocking strategy
- `api-design.md` — REST conventions, versioning, error responses, pagination
- `database-migrations.md` — Migration safety, rollback, zero-downtime deployments
- `security-checklist.md` — OWASP top 10, input validation, auth, injection prevention
- `performance-budget.md` — Bundle size budgets, Core Web Vitals, caching strategy
- `accessibility.md` — WCAG 2.1 AA compliance, ARIA, keyboard navigation
- `documentation.md` — ADRs, API docs, code comments, changelog standards

## How To Enable An Example Rule Pack

1) Copy the example file into `.ai/rules/`:

- Example: copy `.ai/rules/_examples/testing-strategy.md` → `.ai/rules/testing-strategy.md`

2) Add the rule name (filename without `.md`) to `.ai/config/workflow.yaml`:

```yaml
rules:
  kit:
    - git-workflow
  custom:
    - testing-strategy
```

3) Regenerate helper docs (recommended):

```bash
awkit generate
```

This refreshes `AGENTS.md` and `CLAUDE.md` so agents will be instructed to read the enabled custom rules.

