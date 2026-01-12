package upgrade

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// RequiredTaskPermissions defines the Task tool permissions required for AWK workflow
var RequiredTaskPermissions = []string{
	"Task(pr-reviewer)",
	"Task(conflict-resolver)",
}

// PermissionsResult represents the result of a permissions upgrade
type PermissionsResult struct {
	Success bool
	Skipped bool
	Added   []string
	Message string
}

// UpgradePermissions adds missing Task tool permissions to settings.local.json
func UpgradePermissions(stateRoot string, dryRun bool) PermissionsResult {
	settingsPath := filepath.Join(stateRoot, ".claude", "settings.local.json")

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return PermissionsResult{
				Success: false,
				Message: ".claude/settings.local.json not found (run 'awkit generate' first)",
			}
		}
		return PermissionsResult{
			Success: false,
			Message: fmt.Sprintf("Failed to read settings: %v", err),
		}
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return PermissionsResult{
			Success: false,
			Message: fmt.Sprintf("Invalid JSON: %v", err),
		}
	}

	// Get or create permissions.allow
	permissions, ok := settings["permissions"].(map[string]interface{})
	if !ok {
		permissions = make(map[string]interface{})
		settings["permissions"] = permissions
	}

	allowRaw, ok := permissions["allow"].([]interface{})
	var allow []string
	if ok {
		for _, v := range allowRaw {
			if s, ok := v.(string); ok {
				allow = append(allow, s)
			}
		}
	}

	// Check for missing permissions
	allowSet := make(map[string]bool)
	for _, p := range allow {
		allowSet[p] = true
	}

	var added []string
	for _, required := range RequiredTaskPermissions {
		if !allowSet[required] {
			allow = append(allow, required)
			added = append(added, required)
		}
	}

	if len(added) == 0 {
		return PermissionsResult{
			Success: true,
			Skipped: true,
			Message: "All required permissions already present",
		}
	}

	if dryRun {
		return PermissionsResult{
			Success: true,
			Added:   added,
			Message: fmt.Sprintf("Would add: %v", added),
		}
	}

	// Update and write back
	permissions["allow"] = allow
	newData, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return PermissionsResult{
			Success: false,
			Message: fmt.Sprintf("Failed to marshal JSON: %v", err),
		}
	}

	if err := os.WriteFile(settingsPath, append(newData, '\n'), 0644); err != nil {
		return PermissionsResult{
			Success: false,
			Message: fmt.Sprintf("Failed to write settings: %v", err),
		}
	}

	return PermissionsResult{
		Success: true,
		Added:   added,
		Message: fmt.Sprintf("Added: %v", added),
	}
}

// CheckPermissions checks if settings.local.json has required Task tool permissions
// Returns missing permissions list (empty if all present)
func CheckPermissions(stateRoot string) []string {
	settingsPath := filepath.Join(stateRoot, ".claude", "settings.local.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return RequiredTaskPermissions // All missing if file doesn't exist
	}

	var settings struct {
		Permissions struct {
			Allow []string `json:"allow"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return RequiredTaskPermissions
	}

	allowSet := make(map[string]bool)
	for _, p := range settings.Permissions.Allow {
		allowSet[p] = true
	}

	var missing []string
	for _, required := range RequiredTaskPermissions {
		if !allowSet[required] {
			missing = append(missing, required)
		}
	}

	return missing
}

// AgentsResult represents the result of an agents upgrade
type AgentsResult struct {
	Success bool
	Skipped bool
	Created []string
	Message string
}

// UpgradeAgents installs missing agent definitions in .claude/agents/
func UpgradeAgents(stateRoot string, dryRun bool) AgentsResult {
	agentsDir := filepath.Join(stateRoot, ".claude", "agents")

	// Check what's missing
	agents := map[string]string{
		"pr-reviewer.md":       prReviewerAgent,
		"conflict-resolver.md": conflictResolverAgent,
	}

	var missing []string
	for name := range agents {
		path := filepath.Join(agentsDir, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			missing = append(missing, name)
		}
	}

	if len(missing) == 0 {
		return AgentsResult{
			Success: true,
			Skipped: true,
			Message: "All agent definitions already present",
		}
	}

	if dryRun {
		return AgentsResult{
			Success: true,
			Created: missing,
			Message: fmt.Sprintf("Would create: %v", missing),
		}
	}

	// Create agents directory
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		return AgentsResult{
			Success: false,
			Message: fmt.Sprintf("Failed to create agents dir: %v", err),
		}
	}

	// Install missing agents
	var created []string
	for name, content := range agents {
		path := filepath.Join(agentsDir, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				return AgentsResult{
					Success: false,
					Message: fmt.Sprintf("Failed to write %s: %v", name, err),
				}
			}
			created = append(created, name)
		}
	}

	return AgentsResult{
		Success: true,
		Created: created,
		Message: fmt.Sprintf("Created: %v", created),
	}
}

// CheckAgents checks if .claude/agents/ has required agent definitions
// Returns missing agent names (empty if all present)
func CheckAgents(stateRoot string) []string {
	agentsDir := filepath.Join(stateRoot, ".claude", "agents")
	required := []string{"pr-reviewer.md", "conflict-resolver.md"}

	var missing []string
	for _, name := range required {
		path := filepath.Join(agentsDir, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			missing = append(missing, name)
		}
	}
	return missing
}

const prReviewerAgent = `---
name: pr-reviewer
description: AWK PR Reviewer. Executes complete PR review flow: prepare -> review implementation -> verify tests -> submit. Used when analyze-next returns review_pr.
tools: Read, Grep, Glob, Bash
model: sonnet
---

You are the AWK PR Review Expert. You are responsible for executing the **complete review flow**.

## Input

You will receive PR number and Issue number.

## Execution Flow

### Step 1: Prepare Review Context

` + "```bash" + `
awkit prepare-review --pr $PR_NUMBER --issue $ISSUE_NUMBER
` + "```" + `

Record the output:
- ` + "`CI_STATUS`" + `: passed or failed
- ` + "`WORKTREE_PATH`" + `: worktree path
- ` + "`TEST_COMMAND`" + `: command to run tests
- ` + "`TICKET`" + `: Issue body with acceptance criteria

### Step 2: Extract Acceptance Criteria

From the TICKET output, identify all acceptance criteria (lines like ` + "`- [ ] criteria`" + `).

**These criteria are the foundation of your review.** Each criterion MUST be addressed.

### Step 3: Switch to Worktree and Review Implementation

` + "```bash" + `
cd $WORKTREE_PATH
` + "```" + `

**CRITICAL: You MUST actually review the implementation code.**

For EACH acceptance criterion:

1. **Find the implementation** - Use Grep/Read to locate the actual code that implements this criterion
2. **Understand the logic** - Read the code and understand how it works
3. **Write implementation description** - Describe the implementation in your own words (minimum 20 characters), including:
   - Which function/method implements this
   - What the key logic is
   - How it satisfies the criterion

**PROHIBITIONS:**
- **DO NOT** copy criterion text as implementation description
- **DO NOT** assume code structure from ticket requirements
- **DO NOT** write generic descriptions like "implemented as expected"
- **DO NOT** skip reading actual code

### Step 4: Review Tests

For EACH acceptance criterion:

1. **Find the test** - Locate the test function that verifies this criterion
2. **Read the test code** - Understand what the test is checking
3. **Copy key assertion** - Copy an actual assertion line from the test code

**PROHIBITIONS:**
- **DO NOT** invent test function names
- **DO NOT** assume assertion content
- **DO NOT** copy assertions from other files

### Step 5: Additional Review Checks

1. **Requirements Compliance**: Does PR complete ticket requirements?
2. **Commit Format**: Is it ` + "`[type] subject`" + ` (lowercase)?
3. **Scope Restriction**: Any changes beyond ticket scope?
4. **Architecture Compliance**: Does it follow project conventions?
5. **Code Quality**: Any debug code or obvious bugs?
6. **Security Check**: Any sensitive information leakage?

### Step 6: Submit Review

` + "```bash" + `
awkit submit-review \
  --pr $PR_NUMBER \
  --issue $ISSUE_NUMBER \
  --score $SCORE \
  --ci-status $CI_STATUS \
  --body "$REVIEW_BODY"
` + "```" + `

Scoring criteria:
- 9-10: Perfect completion
- 7-8: Completed with good quality
- 5-6: Partial completion, has issues
- 1-4: Not completed or major issues

### Step 7: Return Result

**Immediately return** the submit-review result to Principal:

| Result | Action |
|--------|--------|
| ` + "`merged`" + ` | PR merged, task complete |
| ` + "`changes_requested`" + ` | Review failed, Worker needs to fix |
| ` + "`review_blocked`" + ` | Verification failed, **DO NOT retry** |
| ` + "`merge_failed`" + ` | Merge failed (e.g., conflict) |

---

## Review Body Format

Your review body MUST follow this exact format:

` + "```markdown" + `
### Implementation Review

#### 1. [First Criterion Text]

**Implementation**: [Describe the actual implementation. Must be 20+ chars, include function names and key logic.]

**Code Location**: ` + "`path/to/file.go:LineNumber`" + `

#### 2. [Second Criterion Text]

**Implementation**: [Description...]

**Code Location**: ` + "`path/to/file.go:LineNumber`" + `

### Test Review

| Criteria | Test | Key Assertion |
|----------|------|---------------|
| [FULL Criterion 1 text from ticket] | ` + "`TestFunctionName`" + ` | ` + "`assert.Equal(t, expected, actual)`" + ` |
| [FULL Criterion 2 text from ticket] | ` + "`TestOtherFunction`" + ` | ` + "`require.NoError(t, err)`" + ` |

**CRITICAL**: The Criteria column MUST contain the **exact full text** from the ticket's acceptance criteria. Do NOT use shortened or paraphrased versions.

### Score Reason

[Explain why you gave this score]

### Suggested Improvements

[List any improvement suggestions, or "None" if perfect]

### Potential Risks

[List any potential risks, or "None identified"]
` + "```" + `

---

## Verification Rules (System Enforced)

The system will verify your submission:

1. **Completeness Check**: Every acceptance criterion must have:
   - Implementation description (minimum 20 characters)
   - Test name mapping
   - Key assertion

2. **Test Execution**: System will execute ` + "`$TEST_COMMAND`" + ` in worktree
   - All mapped tests must PASS
   - Failed tests will block the review

3. **Assertion Verification**: System will search test files
   - Your quoted assertions must actually exist in test code
   - Non-existent assertions will block the review

**If verification fails, the review is blocked. A NEW session will retry.**

---

## Common Mistakes to Avoid

### Implementation Description

Wrong:
` + "```" + `
**Implementation**: Implemented according to requirements
` + "```" + `

Wrong:
` + "```" + `
**Implementation**: The feature is complete
` + "```" + `

Correct:
` + "```" + `
**Implementation**: Implemented in ` + "`HandleCollision()`" + ` at engine.go:145. When snake head position matches wall boundary, sets ` + "`game.State = GameOver`" + ` and emits collision event.
` + "```" + `

### Test Assertion (Criteria Column)

Wrong (shortened text):
` + "```" + `
| Wall collision ends game | TestCollision | assert passes |
` + "```" + `

Wrong (paraphrased text):
` + "```" + `
| Collision detection works | TestWallCollision | ` + "`t.Error(\"should end\")`" + ` |
` + "```" + `

Correct (FULL criteria text from ticket + actual assertion):
` + "```" + `
| Wall collision ends game and game state changes to GameOver | TestCollisionScenarios | ` + "`assert.Equal(t, GameOver, game.State)`" + ` |
` + "```" + `

**The Criteria column must match the EXACT text from the ticket's ` + "`- [ ]`" + ` lines.**

---

## CRITICAL: No Retry Rule

**When ` + "`submit-review`" + ` returns ` + "`review_blocked`" + `:**

- **DO NOT** attempt to fix evidence and resubmit
- **DO NOT** analyze failure reasons and retry
- **MUST** immediately return ` + "`review_blocked`" + ` to Principal

**Violating this rule causes "self-dealing" problem - same session self-correction is invalid.**
`

const conflictResolverAgent = `---
name: conflict-resolver
description: AWK Merge Conflict Resolver. Resolves git merge conflicts in a worktree.
tools: Read, Grep, Glob, Bash, Edit
model: sonnet
---

You are the AWK Conflict Resolution Expert.

## Input
You will receive: WORKTREE_PATH, ISSUE_NUMBER, PR_NUMBER

## Execution Flow

### Step 1: Navigate to Worktree
` + "```bash" + `
cd $WORKTREE_PATH
` + "```" + `

### Step 2: Identify Conflicts
` + "```bash" + `
git status
git diff --name-only --diff-filter=U
` + "```" + `

### Step 3: Resolve Each Conflict
For each conflicted file:
1. Read the file to understand context
2. Identify conflict markers (<<<<<<, ======, >>>>>>)
3. Determine correct resolution based on:
   - Intent from both branches
   - Code logic
   - Project conventions
4. Edit to resolve (remove markers, keep correct code)
5. Stage the resolved file

### Step 4: Complete Resolution
` + "```bash" + `
git add .
git rebase --continue
` + "```" + `

Or if conflict is too complex:
` + "```bash" + `
git rebase --abort
` + "```" + `

### Step 5: Return Result
Return one of:
- RESOLVED: Conflict resolved successfully
- TOO_COMPLEX: Conflict requires human intervention
- FAILED: Resolution failed
`
