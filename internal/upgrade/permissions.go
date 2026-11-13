package upgrade

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	awkit "github.com/silver2dream/ai-workflow-kit"
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

// requiredAgentFiles are the built-in agent definitions every project needs.
var requiredAgentFiles = []string{"pr-reviewer.md", "conflict-resolver.md"}

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

	// Agent definitions come from the kit (.claude/agents/*.md in KitFS), the
	// same files install and generate write - there is no second copy to drift.
	agents := map[string]string{}
	for _, name := range requiredAgentFiles {
		content, err := awkit.KitFS.ReadFile(".claude/agents/" + name)
		if err != nil {
			return AgentsResult{
				Success: false,
				Message: fmt.Sprintf("Built-in agent %s is missing from the kit: %v", name, err),
			}
		}
		agents[name] = string(content)
	}

	// Check what's missing

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

	var missing []string
	for _, name := range requiredAgentFiles {
		path := filepath.Join(agentsDir, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			missing = append(missing, name)
		}
	}
	return missing
}
