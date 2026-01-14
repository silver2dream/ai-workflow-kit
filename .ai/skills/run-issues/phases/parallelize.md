# Phase: Parallelize Execution

## Overview

This phase determines which issues can run in parallel and creates execution batches.

## Constraints

### Maximum Concurrent Subagents

```
MAX_CONCURRENT = 3
```

Rationale:
- Prevents resource exhaustion
- Avoids git conflicts from too many parallel branches
- Maintains manageable context for verification

### Dependency Respect

An issue can only start when ALL its dependencies are completed.

## Step 1: Identify Independent Issues

Two issues are independent if:
1. Neither depends on the other (directly or transitively)
2. They don't modify the same files (if detectable from issue description)

### Independence Check Algorithm

```
function areIndependent(issueA, issueB, dependencyGraph):
    # Check direct dependencies
    if issueA in dependencyGraph[issueB].dependencies:
        return false
    if issueB in dependencyGraph[issueA].dependencies:
        return false

    # Check transitive dependencies
    if hasPath(dependencyGraph, issueA, issueB):
        return false
    if hasPath(dependencyGraph, issueB, issueA):
        return false

    return true
```

## Step 2: Create Execution Batches

### Batch Formation Rules

1. **Batch 0**: All issues with no dependencies (roots)
2. **Batch N**: Issues whose dependencies are all in batches < N

### Algorithm

```
function createBatches(issues, dependencyGraph):
    batches = []
    completed = set()
    remaining = set(all issue numbers)

    while remaining is not empty:
        # Find issues ready to run
        ready = []
        for issue in remaining:
            deps = dependencyGraph[issue].dependencies
            if all(dep in completed for dep in deps):
                ready.append(issue)

        if ready is empty and remaining is not empty:
            # Cycle detected or error
            break

        # Sort ready issues by priority score (descending)
        ready.sort(by=score, descending)

        # Create batch (max MAX_CONCURRENT)
        batch = ready[:MAX_CONCURRENT]
        batches.append(batch)

        # Mark as will-be-completed
        for issue in batch:
            completed.add(issue)
            remaining.remove(issue)

    return batches
```

## Step 3: Optimize Batches

### Priority-Based Selection

When more issues are ready than MAX_CONCURRENT:

1. Sort by priority score (descending)
2. Prefer P0 > P1 > P2
3. Within same priority, prefer issues with more dependents

### File Conflict Avoidance

If issue descriptions mention specific files:

1. Extract file paths from issue body
2. Avoid putting issues touching same files in same batch
3. This reduces merge conflicts

Example detection patterns:
```
Files mentioned:
- `backend/internal/...`
- `frontend/Assets/...`
- File paths in code blocks
```

## Step 4: Execution Plan Output

Produce execution plan:

```markdown
## Execution Plan

### Batch 1 (Parallel)
| Issue | Title | Priority | Dependencies |
|-------|-------|----------|--------------|
| #5 | Base feature | P0 | None |
| #8 | Independent fix | P0 | None |
| #15 | Doc update | P2 | None |

### Batch 2 (After Batch 1)
| Issue | Title | Priority | Dependencies |
|-------|-------|----------|--------------|
| #10 | Enhancement A | P1 | #5 |
| #11 | Enhancement B | P1 | #5 |

### Batch 3 (After Batch 2)
| Issue | Title | Priority | Dependencies |
|-------|-------|----------|--------------|
| #12 | Final integration | P1 | #10 |
```

## Step 5: Spawn Subagents

For each batch, spawn subagents using Task tool:

```
For each issue in current_batch (parallel):
    Task tool:
    - subagent_type: general-purpose
    - description: "Work on Issue #<number>: <title>"
    - prompt: |
        Complete GitHub Issue #<number>.

        Title: <title>
        Body: <body excerpt, max 500 chars>

        Requirements:
        1. Create branch: feat/ai-issue-<number> or fix/ai-issue-<number>
        2. Implement the requested changes
        3. Run tests: go test ./...
        4. Commit with format: [type] subject
           - Type in brackets, NO colon
           - Subject MUST be lowercase
        5. Create PR targeting the integration branch
        6. PR body MUST include: Closes #<number>

        Commit format (STRICT - from .ai/rules/_kit/git-workflow.md):
        ✅ [feat] add user authentication
        ✅ [fix] resolve null pointer in handler
        ✅ [docs] update api documentation
        ❌ [Feat] Add user authentication (uppercase = WRONG)
        ❌ feat: add user authentication (colon = WRONG)
```

Wait for all subagents in batch to complete before proceeding to next batch.

## Error Handling

### Subagent Failure

If a subagent fails:

1. Log the failure
2. Mark issue as failed
3. Mark all dependent issues as skipped
4. Continue with remaining independent issues

### Batch Timeout

If a batch takes too long (> 30 minutes):

1. Log timeout warning
2. Allow running subagents to complete
3. Mark incomplete issues as timed-out
4. Continue to verification phase

## Self-Check

After planning:
```
[RUN-ISSUES] <timestamp> | parallelize | Batches: <n>, Total issues: <n>, Max parallel: <n>
```

After each batch:
```
[RUN-ISSUES] <timestamp> | parallelize | Batch <n>/<total> complete: <success>/<total> succeeded
```
