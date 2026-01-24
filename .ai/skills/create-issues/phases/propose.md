# Phase 3: Issue Proposal Generation

Generate detailed proposals for each issue identified in the breakdown phase.

## Proposal Format

For each issue, generate a detailed proposal using this exact format:

```markdown
---

### Issue #<sequential_number>

| Field | Value |
|-------|-------|
| **Title** | [type] description |
| **Repo** | backend / frontend |
| **Priority** | P0 / P1 / P2 |
| **Complexity** | 1-5 |
| **Depends On** | Issue #X, #Y or None |
| **Labels** | feat, backend, ai-task |

**Objective:**
Clear statement of what this issue achieves. One to two sentences maximum.

**Scope:**
- `path/to/file.go` - create (new file)
- `path/to/existing.go` - modify (add method X)
- `path/to/another.go` - modify (update function Y)

**Acceptance Criteria:**
- [ ] Feature/behavior description (what it does, not test function name)
- [ ] Edge case handling description (what edge cases are covered)
- [ ] Unit tests added for new functionality
- [ ] All tests pass (`go test ./...` or equivalent)
- [ ] Code builds without errors

**IMPORTANT**: Acceptance Criteria should describe the INTENT (what behavior is expected), NOT specific test function names. The Worker decides how to name and structure tests.

**Verification:**
```bash
go build ./...
go test ./...
```

---
```

## Field Guidelines

### Title
- Format: `[type] short description`
- Types: feat, fix, refactor, test, docs, chore, perf
- Keep under 60 characters
- Use lowercase
- Examples:
  - `[feat] add user repository interface`
  - `[fix] handle nil pointer in auth service`
  - `[refactor] extract validation logic to separate file`

### Repo
- Use exact repo name from workflow.yaml
- Common values: `backend`, `frontend`

### Priority
- **P0**: Critical path, blocks other issues
- **P1**: Important, core functionality
- **P2**: Nice to have, polish, tests

### Complexity
- **1**: Trivial (< 30 min)
- **2**: Simple (30 min - 1 hr)
- **3**: Moderate (1-2 hrs)
- **4**: Complex (2-4 hrs)
- **5**: Major (4+ hrs)

### Labels
Always include:
- Issue type: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`
- Repo name: `backend`, `frontend`
- Workflow label: `ai-task`

Optional:
- Priority: `priority-p0`, `priority-p1`, `priority-p2`
- Area: `api`, `database`, `ui`, `config`

### Objective
- One to two sentences
- Focus on the "why" and "what"
- Avoid implementation details

### Scope
List all files that will be touched:
- Use relative paths from repo root
- Indicate action: create, modify, delete
- Brief note on what changes

### Acceptance Criteria
- Use checkbox format: `- [ ]`
- Make criteria measurable and verifiable
- Describe the INTENT/BEHAVIOR, NOT specific test function names
- Include "Unit tests added for new functionality"
- Include "All tests pass" and "builds without errors"
- 3-5 criteria per issue
- **DO NOT** pre-specify exact test function names (e.g., avoid "TestFooBar passes")
- **DO** describe what behavior should be tested (e.g., "Unit tests cover edge case X")

### Verification
Include actual commands to verify:
- For Go: `go build ./...` and `go test ./...`
- For Node: `npm run build` and `npm run test`
- For Unity: `echo 'Unity build via Editor'`

## Summary Table

After all proposals, include a summary table:

```markdown
## Summary

| # | Title | Repo | Priority | Complexity | Depends On |
|---|-------|------|----------|------------|------------|
| 1 | [feat] add user repository interface | backend | P0 | 2 | - |
| 2 | [feat] implement user service | backend | P1 | 3 | #1 |
| 3 | [test] add user service unit tests | backend | P2 | 2 | #2 |
```

## Dependency Graph

Visualize the dependency structure:

```markdown
## Dependency Graph

```
Phase 1 (Setup):
  #1 [P0] add user repository interface
    |
Phase 2 (Implementation):
    +---> #2 [P1] implement user service
    |       |
    |       +---> #4 [P1] add user RPC handler
    |
    +---> #3 [P1] implement user cache
            |
Phase 3 (Testing):
            +---> #5 [P2] add integration tests
```
```

## Example Complete Proposal

```markdown
---

### Issue #1

| Field | Value |
|-------|-------|
| **Title** | [feat] add user repository interface |
| **Repo** | backend |
| **Priority** | P0 |
| **Complexity** | 2 |
| **Depends On** | None |
| **Labels** | feat, backend, ai-task |

**Objective:**
Define the repository port interface for user data access, establishing the contract between the service layer and data persistence.

**Scope:**
- `backend/internal/modules/user/user_repository.go` - create (new file with interface and domain errors)

**Acceptance Criteria:**
- [ ] UserRepository interface defined with GetByID, Create, Update methods
- [ ] Domain errors defined: ErrUserNotFound, ErrUserConflict
- [ ] Interface follows existing repository patterns in codebase
- [ ] Unit tests added for new functionality
- [ ] All tests pass (`go test ./...`)
- [ ] Code builds without errors

**Verification:**
```bash
go build ./...
go test ./...
```

---
```

## Output

Present all proposals to the user in the format above, followed by the summary table and dependency graph.

**Important**: Do not proceed to Phase 4 until all proposals are displayed.

## Next Phase

Proceed to **Phase 4: Approval** - Read `phases/approval.md`
