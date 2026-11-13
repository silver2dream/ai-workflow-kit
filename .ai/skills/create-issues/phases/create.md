# Phase 5: Issue Creation

Create the approved issues with `awkit create-task` and generate a final report.

## Prerequisites

- User has explicitly approved in Phase 4
- All proposal details are ready
- GitHub CLI (`gh`) is authenticated

## How the workflow reads an issue

An issue becomes a ticket of the AWK workflow through two things, and
`awkit create-task` takes care of both:

- the task label (`ai-task` unless the config says otherwise), and
- a `yaml awk-ticket` block in the body, which says which repo the ticket is
  for and which issues must be done before it starts.

Do not create these issues with `gh issue create`: it skips the check of the
body, and a ticket with a missing or broken block stops the workflow later.

The full description of the block is in
`.ai/skills/principal-workflow/tasks/create-task.md`.

## Step 1: Decide the order

The workflow takes tickets in the order of their issue numbers and skips a
ticket while one of the issues it depends on is still open. So:

1. Create an issue only after every issue it depends on has been created: its
   body needs their real numbers.
2. Among issues that are ready at the same time, create first what should be
   worked on first (P0, then P1, then P2).

Keep the mapping `proposal number -> GitHub issue number` as you go.

## Step 2: Write the body and create the issue

For each issue, in that order, write the body to a file and create the issue
from it:

````bash
mkdir -p .ai/temp
cat > .ai/temp/issue-1.md <<'EOF'
## Summary
{summary from the proposal}

## Scope
{scope list from the proposal}

## Acceptance Criteria
{criteria list from the proposal: `- [ ]` items that describe intent, not test function names}

## Testing Requirements
- New features MUST have corresponding unit tests
- Modified features MUST have updated or new test cases
- Test coverage should cover happy path and error cases

```yaml awk-ticket
schema: 1
repo: backend
severity: P1
depends_on: [45, 46]
```
EOF

awkit create-task --title "[type] description" --body-file .ai/temp/issue-1.md
````

The block:

| Field | Value |
|-------|-------|
| `schema` | always `1` |
| `repo` | the proposal's Repo: a repo name from `.ai/config/workflow.yaml`, or `root` |
| `severity` | the proposal's Priority (`P0`, `P1`, `P2`) |
| `depends_on` | the GitHub issue numbers of the proposal's dependencies. Leave the line out when there are none |

Write nothing else into the block: an unknown field makes `awkit create-task`
refuse the body. Proposal numbers must never end up in `depends_on`; replace
each with the GitHub issue number from your mapping.

`awkit create-task` prints `Created Issue #<N>` on success. Record `N` in the
mapping before you write the next body.

Use `--dry-run` to check a body without creating anything.

## Step 3: Generate Final Report

Output a comprehensive report:

```markdown
## Issues Created Successfully

| Proposal # | GitHub Issue | Title | Repo | Depends on | Status |
|------------|--------------|-------|------|------------|--------|
| 1 | #45 | [feat] add user repository | backend | - | Created |
| 2 | #46 | [feat] implement user service | backend | #45 | Created |
| 3 | #47 | [test] add user tests | backend | #46 | Created |

### Dependency Map (with actual numbers)

#45 [P0] --> #46 [P1] --> #47 [P2]

### Next Steps

The workflow starts with #45. #46 starts once #45 is merged and closed, #47
once #46 is.

To run the workflow:

    awkit run
```

## Error Handling

### `awkit create-task` refuses the body

The command exits non-zero and says what is wrong: a missing section, no
`- [ ]` checkbox, no `yaml awk-ticket` block, a field the block does not have,
or a repo the config does not have. Correct the body file and run the command
again. Nothing was created.

### GitHub fails

If the command fails on the GitHub side:

```markdown
Failed to create issue: {error message}

Possible causes:
- GitHub authentication issue: Run `gh auth login`
- Network error: Check connection
- Permission denied: Verify repo access

Created so far: {list of successfully created issues}
Remaining: {list of issues not yet created}

Would you like me to retry the failed issues? (yes/no)
```

### Partial Success

If some issues were created but not all:

1. Report which succeeded and which failed
2. Offer to retry the failed ones
3. Do not create an issue whose dependency was not created: its `depends_on`
   would have nothing to point at

## Example Complete Flow

```bash
# Issue 1 (no dependencies): the block has no depends_on line
awkit create-task --title "[feat] add user repository interface" --body-file .ai/temp/issue-1.md
# Created Issue #45

# Issue 2 (depends on proposal 1, now #45): depends_on: [45]
awkit create-task --title "[feat] implement user service" --body-file .ai/temp/issue-2.md
# Created Issue #46

# Issue 3 (depends on proposal 2, now #46): depends_on: [46]
awkit create-task --title "[test] add user service tests" --body-file .ai/temp/issue-3.md
# Created Issue #47
```

## Completion

After successful creation:

```markdown
All {N} issues have been created successfully.

Quick links:
{list of issue URLs}

The issues are ready for the AWK workflow. Run `awkit run` to start processing them.
```

## Self-Check Output

```
[CREATE-ISSUES] {timestamp} | Phase 5: Create | Creating {N} issues
[CREATE-ISSUES] {timestamp} | Created Issue #45: [feat] add user repository interface
[CREATE-ISSUES] {timestamp} | Created Issue #46: [feat] implement user service
[CREATE-ISSUES] {timestamp} | Created Issue #47: [test] add user service tests
[CREATE-ISSUES] {timestamp} | Phase 5: Complete | All issues created
```
