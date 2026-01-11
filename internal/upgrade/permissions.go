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
