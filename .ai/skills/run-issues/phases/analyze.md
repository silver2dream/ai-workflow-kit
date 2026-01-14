# Phase: Analyze Issues

## Overview

This phase analyzes fetched issues to determine priorities and dependencies.

## Step 1: Fetch Issues

Execute the following command to get all open issues:

```bash
gh issue list --state open --json number,title,body,labels,assignees --limit 50
```

Store the JSON output for processing.

## Step 2: Priority Extraction

### Label-Based Priority

Check issue labels for explicit priority markers:

| Label Pattern | Priority | Score |
|---------------|----------|-------|
| `P0`, `critical`, `urgent`, `blocker` | P0 | 100 |
| `P1`, `high`, `important` | P1 | 50 |
| `P2`, `medium`, `normal` | P2 | 25 |
| `P3`, `low`, `minor` | P3 | 10 |
| No priority label | P2 (default) | 25 |

### Type-Based Priority Boost

Add bonus points based on issue type labels:

| Label Pattern | Bonus |
|---------------|-------|
| `bug`, `fix`, `security` | +20 |
| `feat`, `feature`, `enhancement` | +10 |
| `docs`, `documentation` | +5 |
| `chore`, `refactor` | +5 |

### Final Priority Score

```
final_score = base_priority_score + type_bonus
```

Sort issues by `final_score` descending. Higher score = process first.

## Step 3: Dependency Detection

### From Issue Body

Scan issue body for dependency keywords:

```
Patterns to detect:
- "depends on #<number>"
- "blocked by #<number>"
- "after #<number>"
- "requires #<number>"
- "parent: #<number>"
```

Example regex:
```
(?:depends on|blocked by|after|requires|parent:?)\s*#(\d+)
```

### From Issue Title

Check title for dependency indicators:

```
Patterns:
- "[depends #<number>]"
- "(after #<number>)"
```

### From Labels

Check for dependency labels:

```
Labels:
- "depends-on-<number>"
- "blocked"
- "waiting"
```

## Step 4: Build Dependency Graph

Create a directed graph where:
- Nodes = issue numbers
- Edges = dependencies (A -> B means A depends on B)

```
Example:
Issue #10 depends on #5
Issue #11 depends on #5
Issue #12 depends on #10

Graph:
#5 <- #10 <- #12
#5 <- #11
```

### Detect Cycles

Check for circular dependencies:
1. Perform topological sort
2. If sort fails, there's a cycle
3. Report cyclic issues and exclude from processing

## Step 5: Output Format

After analysis, produce a structured summary:

```json
{
  "issues": [
    {
      "number": 5,
      "title": "Base feature",
      "priority": "P0",
      "score": 120,
      "dependencies": [],
      "dependents": [10, 11]
    },
    {
      "number": 10,
      "title": "Enhancement A",
      "priority": "P1",
      "score": 60,
      "dependencies": [5],
      "dependents": [12]
    }
  ],
  "execution_order": [5, 10, 11, 12],
  "cycles": []
}
```

## Decision Rules

### When to Skip an Issue

Skip processing if:
- Issue is assigned to someone else
- Issue has label `wip`, `in-progress`, `do-not-process`
- Issue has label `needs-info`, `needs-discussion`
- Issue is part of a dependency cycle

### When to Prioritize

Always process first:
- Issues with `P0`, `critical`, `blocker` labels
- Issues with `security` label
- Issues that are dependencies for many other issues

## Self-Check

After completing analysis:
```
[RUN-ISSUES] <timestamp> | analyze | Total: <n> issues, P0: <n>, P1: <n>, P2: <n>, Skipped: <n>
```
