# Phase 5: Issue Creation

Create the approved issues via GitHub CLI and generate a final report.

## Prerequisites

- User has explicitly approved in Phase 4
- All proposal details are ready
- GitHub CLI (`gh`) is authenticated

## Step 1: Prepare Issue Bodies

For each issue, prepare the full body using this template:

```markdown
## Objective
{objective from proposal}

## Scope
{scope list from proposal}

## Acceptance Criteria
{criteria list from proposal}

## Verification
```bash
{verification commands}
```

## Dependencies
{dependency information - will be updated with actual issue numbers}

## Constraints
- Follow commit format: `[type] subject`
- Obey AGENTS.md
- Obey `.ai/rules/_kit/git-workflow.md`
```

## Step 2: Create Issues in Dependency Order

Create issues starting with P0 (no dependencies) first, then P1, then P2.

### Order Rules
1. Issues with no dependencies first
2. Then issues whose dependencies are already created
3. Track created issue numbers for dependency updates

### Create Command

For each issue:

```bash
gh issue create \
  --title "[type] description" \
  --body "$(cat <<'EOF'
## Objective
{objective}

## Scope
{scope list}

## Acceptance Criteria
{criteria list}

## Verification
```bash
{commands}
```

## Dependencies
{dependencies or "None"}

## Constraints
- Follow commit format: `[type] subject`
- Obey AGENTS.md
- Obey `.ai/rules/_kit/git-workflow.md`
EOF
)" \
  --label "feat,backend,ai-task"
```

### Capture Issue Number

Parse the output to get the created issue number:

```bash
# gh issue create returns: https://github.com/owner/repo/issues/123
# Extract the issue number from the URL
```

Store the mapping: `proposal_id -> github_issue_number`

## Step 3: Update Dependencies

After all issues are created, update issue bodies with actual GitHub issue numbers.

For issues that reference other issues:
1. Get the actual issue numbers from the mapping
2. Update the issue body with correct references

```bash
gh issue edit {issue_number} --body "$(cat <<'EOF'
{updated body with actual issue numbers}
EOF
)"
```

### Dependency Format in Body

Replace placeholder references:
- Before: `Depends on: Issue #1, #2` (proposal numbers)
- After: `Depends on: #45, #46` (GitHub issue numbers)

## Step 4: Add Cross-References

Optionally add comments linking related issues:

```bash
gh issue comment {issue_number} --body "This issue is part of a feature set. Related issues: #45, #46, #47"
```

## Step 5: Generate Final Report

Output a comprehensive report:

```markdown
## Issues Created Successfully

| Proposal # | GitHub Issue | Title | Labels | Status |
|------------|--------------|-------|--------|--------|
| 1 | #45 | [feat] add user repository | feat, backend | Created |
| 2 | #46 | [feat] implement user service | feat, backend | Created |
| 3 | #47 | [test] add user tests | test, backend | Created |

### Issue Links
- #45: https://github.com/{owner}/{repo}/issues/45
- #46: https://github.com/{owner}/{repo}/issues/46
- #47: https://github.com/{owner}/{repo}/issues/47

### Dependency Map (with actual numbers)
```
#45 [P0] --> #46 [P1] --> #47 [P2]
```

### Next Steps
1. Start with Issue #45 (P0, no dependencies)
2. After #45 is merged, work on #46
3. Finally, complete #47

To run the workflow:
```bash
awkit kickoff
```
```

## Error Handling

### Issue Creation Fails

If `gh issue create` fails:

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
2. Offer to retry failed ones
3. Ensure dependency references are still correct for created issues

## Example Complete Flow

```bash
# Issue 1 (no dependencies)
gh issue create --title "[feat] add user repository interface" --body "..." --label "feat,backend,ai-task"
# Returns: https://github.com/owner/repo/issues/45

# Issue 2 (depends on #1, now #45)
gh issue create --title "[feat] implement user service" --body "...Depends on: #45..." --label "feat,backend,ai-task"
# Returns: https://github.com/owner/repo/issues/46

# Issue 3 (depends on #2, now #46)
gh issue create --title "[test] add user service tests" --body "...Depends on: #46..." --label "test,backend,ai-task"
# Returns: https://github.com/owner/repo/issues/47
```

## Completion

After successful creation:

```markdown
All {N} issues have been created successfully.

Quick links:
{list of issue URLs}

The issues are ready for the AWK workflow. Run `awkit kickoff` to start processing them.
```

## Self-Check Output

```
[CREATE-ISSUES] {timestamp} | Phase 5: Create | Creating {N} issues
[CREATE-ISSUES] {timestamp} | Created Issue #45: [feat] add user repository interface
[CREATE-ISSUES] {timestamp} | Created Issue #46: [feat] implement user service
[CREATE-ISSUES] {timestamp} | Created Issue #47: [test] add user service tests
[CREATE-ISSUES] {timestamp} | Phase 5: Complete | All issues created
```
