# (Example Rule Pack) Documentation Standards & Practices (STRICT)

This is an optional example rule pack for AWK users.
To enable it: copy this file to `.ai/rules/documentation.md`, then add `documentation` under `rules.custom` in `.ai/config/workflow.yaml`.

Role: Senior Technical Writer.
Goal: Maintain comprehensive, up-to-date documentation including architecture decision records, API documentation, code comments, and changelogs.

This document is the source of truth for documentation standards. Documentation gaps in new features are P2 and should be addressed before release.

---

## 0) Architecture Decision Records (STRICT)

### 0.1 When to Write an ADR
An ADR is REQUIRED when:
- Choosing between competing technologies or libraries
- Changing an architectural pattern (e.g., monolith to microservices)
- Introducing a new external dependency with long-term implications
- Deviating from an established pattern in the codebase

### 0.2 ADR Format
Store ADRs in `docs/adr/` with sequential numbering:

```
docs/adr/
  0001-use-postgresql-for-primary-database.md
  0002-adopt-event-sourcing-for-orders.md
  0003-switch-to-cursor-pagination.md
```

Each ADR MUST follow this template:

```markdown
# ADR-{NUMBER}: {TITLE}

## Status
Proposed | Accepted | Deprecated | Superseded by ADR-{N}

## Context
What is the problem or decision we need to make?

## Decision
What did we decide and why?

## Consequences
What are the positive, negative, and neutral outcomes?
```

### 0.3 Lifecycle
- ADRs MUST NOT be deleted; deprecated decisions are marked with `Status: Deprecated`.
- Superseded ADRs MUST link to the replacement.
- Review ADRs quarterly; flag any that no longer reflect reality.

---

## 1) API Documentation (REQUIRED)

### 1.1 OpenAPI / Swagger
- Every REST endpoint MUST have an OpenAPI spec entry.
- Spec MUST include: summary, description, parameters, request body schema, response schemas (success + errors), and examples.
- Keep the spec in sync with code. Prefer code-generated specs (annotations or codegen); if manually maintained, validate in CI.

### 1.2 Inline API Docs
- Controller/handler functions MUST have a doc comment describing:
  - What the endpoint does
  - Required authentication/authorization
  - Notable side effects

### 1.3 Client SDKs
- If the project generates client SDKs, document the generation process and verify generated code compiles.
- SDK README MUST include: installation, authentication setup, and 3 usage examples.

---

## 2) Code Comments (STRICT)

### 2.1 When to Comment
Comments MUST explain **why**, not **what**. The code itself should explain what.

REQUIRED comments:
- Non-obvious business rules or domain logic
- Workarounds for known bugs (include issue/ticket link)
- Performance-critical code paths (explain why this approach was chosen)
- Regular expressions (explain what the pattern matches)
- Magic numbers (explain the value's origin or meaning)

### 2.2 When NOT to Comment
Do NOT write comments that:
- Restate the code: `// increment counter` above `counter++`
- Explain basic language features
- Are outdated or describe removed code
- Are TODO/FIXME without a tracking issue link

### 2.3 TODO Format
```
// TODO(#123): description of what needs to be done
```
Every TODO MUST reference a tracking issue. Orphan TODOs are a P2 violation.

### 2.4 Public API Documentation
- All exported functions, types, and interfaces MUST have doc comments.
- Go: use standard godoc format. TypeScript: use JSDoc or TSDoc.
- Doc comments MUST describe: purpose, parameters, return values, errors/exceptions, and usage example (for complex APIs).

---

## 3) README Standards (MUST)

### 3.1 Required Sections
Every repository and significant package/module MUST have a README with:

1. **Title and description**: one-sentence summary of what it does.
2. **Prerequisites**: required tools, versions, environment setup.
3. **Quick Start**: minimum steps to get running (clone, install, run).
4. **Configuration**: environment variables, config files, and their purpose.
5. **Development**: how to build, test, lint, and run locally.
6. **Deployment**: how to deploy (or link to deployment docs).
7. **Contributing**: link to contributing guide or inline instructions.

### 3.2 Keep It Current
- README MUST be updated in the same PR that changes setup steps, configuration, or dependencies.
- Stale README sections are a P2 violation; flag and fix promptly.

---

## 4) Changelog (REQUIRED)

### 4.1 Format
Follow Keep a Changelog (https://keepachangelog.com/) format:

```markdown
# Changelog

## [Unreleased]

### Added
- New feature description (#issue-number)

### Changed
- Existing feature modification (#issue-number)

### Fixed
- Bug fix description (#issue-number)

### Removed
- Removed feature (#issue-number)
```

### 4.2 Rules
- Every user-facing change MUST have a changelog entry.
- Internal refactors, test additions, and CI changes do NOT need changelog entries.
- Changelog entries MUST reference the issue or PR number.
- On release, move `[Unreleased]` entries under the version header with the release date.

---

## 5) Inline Documentation Patterns (MUST)

### 5.1 Module/Package Level
- Each package/module MUST have a top-level doc comment (Go: `doc.go` or package comment; TS: module JSDoc).
- Describe: purpose, key types, usage patterns, and relationships to other packages.

### 5.2 Type/Interface Level
- All exported types and interfaces MUST have doc comments describing:
  - Purpose and responsibility
  - Thread-safety guarantees (if applicable)
  - Lifecycle (creation, usage, disposal)

### 5.3 Function/Method Level
- Exported functions MUST document:
  - What the function does (one sentence)
  - Parameters and their constraints
  - Return values and error conditions
  - Panics (if any, and under what conditions)

### 5.4 Configuration
- Every configuration option (env var, YAML key, CLI flag) MUST be documented with:
  - Description
  - Type and format
  - Default value
  - Example value

---

## 6) Diagrams & Visual Documentation (RECOMMENDED)

### 6.1 When to Include Diagrams
- System architecture overview (required for repos with 3+ services)
- Complex data flows or state machines
- Deployment topology
- Database schema relationships

### 6.2 Tooling
- Prefer text-based diagram tools that live in source control: Mermaid, PlantUML, or D2.
- Store diagram source files alongside the documentation they support.
- Do NOT use binary image files as the source of truth for diagrams (render from text source).

---

## 7) Review Checklist (REQUIRED for PRs)

Before approving a PR, verify documentation:

- [ ] New exported APIs have doc comments
- [ ] Non-obvious logic has explanatory comments
- [ ] README updated if setup/config/deps changed
- [ ] Changelog entry added for user-facing changes
- [ ] ADR written if an architectural decision was made
- [ ] OpenAPI spec updated for new/changed endpoints
- [ ] No orphan TODOs (every TODO links to an issue)
- [ ] Existing documentation updated to reflect code changes

---

## 8) Output Format (when producing documentation)

When creating or updating documentation, ALWAYS include:
1) File list + target paths
2) Full content or diff for each file
3) Notes:
   - what triggered the documentation change
   - cross-references to related docs
   - any diagrams that should be created/updated
4) Verification: links resolve, code examples compile/run

---

## 9) Definition of Done (Checklist)

- [ ] All exported APIs documented with doc comments
- [ ] ADR written for architectural decisions
- [ ] OpenAPI spec matches implementation
- [ ] README is current and includes all required sections
- [ ] Changelog entry present for user-facing changes
- [ ] No orphan TODOs in new code
- [ ] Code comments explain why, not what
- [ ] Configuration options documented with types and defaults
