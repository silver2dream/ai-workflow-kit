# Phase 2: Requirement Breakdown

Systematically decompose the user's requirement into discrete, actionable issues.

## Step 1: Understand the Requirement

Parse the user's input to identify:
- **Core functionality** - What needs to be built/changed
- **Affected areas** - Which repos/modules are involved
- **Implicit requirements** - Testing, documentation, migrations
- **Constraints** - Performance, compatibility, security

## Step 2: Decomposition Rules

### Rule 1: Single Responsibility
Each issue should do ONE thing well:
- One module change per issue (prefer)
- One feature per issue
- One fix per issue

### Rule 2: Testable Scope
Each issue should be independently verifiable:
- Can be built without other issues
- Can be tested in isolation (or with explicit dependencies)
- Has clear acceptance criteria

### Rule 3: Reasonable Size
Target complexity that can be done in one PR:
- 1-2 files for small issues
- 3-5 files for medium issues
- Avoid 10+ file changes in single issue

### Rule 4: Phase Grouping
Group issues into execution phases:

| Phase | Purpose | Examples |
|-------|---------|----------|
| **Setup** | Foundation work | Schema, config, interfaces |
| **Implementation** | Core functionality | Services, repositories, handlers |
| **Integration** | Connect components | Wiring, registration, events |
| **Testing** | Verification | Unit tests, integration tests |
| **Polish** | Finalization | Docs, cleanup, optimization |

## Step 3: Dependency Detection

Identify dependencies between issues:

### Explicit Dependencies
- Issue B requires types defined in Issue A
- Issue C requires API from Issue B
- Issue D requires database schema from Issue A

### Implicit Dependencies
- Tests depend on implementation
- Integration depends on components
- Documentation depends on stable API

### Dependency Rules
1. **No circular dependencies** - If A depends on B, B cannot depend on A
2. **Minimize depth** - Prefer wide over deep dependency chains
3. **Explicit over implicit** - Document all dependencies clearly

## Step 4: Complexity Scoring

Assign complexity score (1-5) based on:

| Score | Criteria | Example |
|-------|----------|---------|
| **1** | Single file, trivial change | Config update, constant change |
| **2** | 1-2 files, straightforward | Add new field, simple function |
| **3** | 3-5 files, moderate logic | New endpoint, new repository method |
| **4** | 5-10 files, significant logic | New module, complex feature |
| **5** | 10+ files, architecture change | Major refactor, new subsystem |

## Step 5: Priority Assignment

Assign priority based on dependency and importance:

| Priority | Criteria | Order |
|----------|----------|-------|
| **P0** | Blocking other issues, critical path | Execute first |
| **P1** | Important but not blocking | Execute second |
| **P2** | Nice to have, polish | Execute last |

### Priority Rules
1. **Dependencies are P0** - If something blocks others, it's P0
2. **Core functionality is P1** - Main implementation work
3. **Tests and docs are P2** - Unless explicitly required first

## Step 6: Issue Type Assignment

Determine issue type for commit format:

| Type | Use When |
|------|----------|
| `feat` | New functionality |
| `fix` | Bug fix |
| `refactor` | Code restructuring (no behavior change) |
| `test` | Adding/fixing tests |
| `docs` | Documentation |
| `chore` | Build, config, tooling |
| `perf` | Performance improvement |

## Step 7: Build Issue List

For each identified task, create an entry:

```yaml
- id: 1
  type: feat
  title: "[feat] add user repository interface"
  repo: backend
  priority: P0
  complexity: 2
  phase: Setup
  depends_on: []
  scope:
    - path: "backend/internal/modules/user/user_repository.go"
      action: "create"
  objective: "Define repository port for user data access"
  criteria:
    - "Repository interface defined with CRUD methods"
    - "Domain errors defined (ErrNotFound, ErrConflict)"
```

## Output Format

Generate a structured list:

```markdown
## Breakdown Summary

**Requirement**: {original requirement}

**Total Issues**: N
**By Priority**: P0: X, P1: Y, P2: Z
**By Phase**: Setup: A, Implementation: B, Testing: C

### Issue List

| # | Type | Title | Repo | Priority | Complexity | Depends On |
|---|------|-------|------|----------|------------|------------|
| 1 | feat | Add user repository | backend | P0 | 2 | - |
| 2 | feat | Add user service | backend | P1 | 3 | #1 |
| 3 | test | Add user service tests | backend | P2 | 2 | #2 |

### Dependency Graph

```
#1 (P0) --> #2 (P1) --> #3 (P2)
            |
            +--> #4 (P1)
```
```

## Next Phase

Proceed to **Phase 3: Propose** - Read `phases/propose.md`
