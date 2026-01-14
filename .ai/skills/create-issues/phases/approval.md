# Phase 4: User Approval Gate

Present the proposals to the user and wait for explicit approval before creating any issues.

## Critical Rule

**NEVER create issues without explicit user approval.**

This phase is a mandatory gate. Do not proceed to Phase 5 until the user confirms.

## Step 1: Display Proposals

Ensure all proposals from Phase 3 have been displayed:
- Individual issue details (all fields)
- Summary table
- Dependency graph

## Step 2: Request Approval

After displaying all proposals, ask the user:

```markdown
---

## Ready to Create Issues

I have prepared **{N} issues** for creation:
- P0 (Critical): {count}
- P1 (Important): {count}
- P2 (Nice to have): {count}

**Options:**
- **yes** - Create all issues as proposed
- **no** - Cancel, do not create any issues
- **edit #N** - Show more details about Issue #N

Do you want me to create these issues? (yes/no/edit #N)
```

## Step 3: Handle User Response

### Response: "yes" or "y"

User approved. Proceed to **Phase 5: Create** - Read `phases/create.md`

### Response: "no" or "n"

User declined. Output:

```markdown
Understood. No issues were created.

If you would like to:
- Modify the requirement, please describe the changes
- Try again with different scope, just let me know
- Ask questions about the proposals, I'm happy to explain
```

**Do not proceed to Phase 5.**

### Response: "edit #N" or "details #N"

User wants more information. Show expanded details for the requested issue:

```markdown
### Issue #N - Expanded Details

**Title**: [type] description

**Full Objective**:
{expanded explanation of what this issue achieves and why}

**Detailed Scope**:
1. `path/to/file.go`
   - Action: create/modify
   - Changes: {detailed description of changes}
   - Lines affected: ~{estimate}

2. `path/to/another.go`
   - Action: modify
   - Changes: {detailed description}
   - Lines affected: ~{estimate}

**Implementation Notes**:
- {Technical consideration 1}
- {Technical consideration 2}
- {Pattern to follow}

**Dependencies Explained**:
- Depends on #X because: {reason}
- Blocks #Y because: {reason}

**Risk Assessment**:
- Complexity: {1-5} - {explanation}
- Risk level: Low/Medium/High
- Mitigation: {if medium/high risk}
```

Then ask again:
```markdown
Do you want me to create these issues? (yes/no/edit #N)
```

### Response: Modification Request

If user asks to change something (e.g., "combine #2 and #3", "remove #4", "add a test issue"):

1. Acknowledge the request
2. Update the proposals accordingly
3. Re-display the updated summary
4. Ask for approval again

Example:
```markdown
I've updated the proposals based on your feedback:
- Combined Issue #2 and #3 into a single issue
- Removed Issue #4 as requested
- Added new Issue #5 for unit tests

Updated summary:
{new summary table}

Do you want me to create these issues? (yes/no/edit #N)
```

## Step 4: Confirmation Before Proceeding

Before moving to Phase 5, confirm:

```markdown
Creating {N} issues now...
```

Only then proceed to **Phase 5: Create** - Read `phases/create.md`

## Approval Flowchart

```
Display Proposals
       |
       v
Request Approval
       |
       v
   User Response?
       |
  +----+----+----+
  |    |    |    |
 yes  no  edit  modify
  |    |    |    |
  v    v    |    |
Create END  |    |
       ^    |    |
       |    v    v
       +-- Show Details / Update Proposals
           Then request approval again
```

## Error Handling

If user response is unclear:

```markdown
I didn't understand your response. Please respond with:
- **yes** - to create all issues
- **no** - to cancel
- **edit #N** - to see more details about issue N

What would you like to do?
```

## Next Phase

Only proceed to **Phase 5: Create** when user explicitly says "yes".

Read `phases/create.md`
