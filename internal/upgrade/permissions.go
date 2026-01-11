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
description: AWK PR Reviewer. Executes complete PR review flow.
tools: Read, Grep, Glob, Bash
model: sonnet
---

You are the AWK PR Review Expert.

## Execution Flow

### Step 1: Prepare Review Context
` + "```bash" + `
awkit prepare-review --pr $PR_NUMBER --issue $ISSUE_NUMBER
` + "```" + `

Record: CI_STATUS, WORKTREE_PATH, TEST_COMMAND, TICKET

### Step 2: Extract Acceptance Criteria
From TICKET, identify all acceptance criteria.

### Step 3: Review Implementation
` + "```bash" + `
cd $WORKTREE_PATH
` + "```" + `

For EACH acceptance criterion:
1. Find the implementation code
2. Describe implementation (20+ chars, include function names)
3. Note code location

### Step 4: Review Tests
For EACH criterion:
1. Find the test function
2. Copy an actual assertion line

### Step 5: Submit Review
` + "```bash" + `
awkit submit-review --pr $PR_NUMBER --issue $ISSUE_NUMBER --score $SCORE --ci-status $CI_STATUS --body "$REVIEW_BODY"
` + "```" + `

Score: 9-10 perfect, 7-8 good, 5-6 partial, 1-4 major issues

### Step 6: Return Result
Return submit-review result to Principal immediately:
- merged: PR merged
- changes_requested: Review failed
- review_blocked: Verification failed, DO NOT retry
- merge_failed: Merge failed

## CRITICAL: No Retry Rule
When submit-review returns review_blocked:
- DO NOT attempt to fix and resubmit
- MUST immediately return review_blocked to Principal
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
