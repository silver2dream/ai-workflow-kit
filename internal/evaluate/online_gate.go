package evaluate

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// CheckN1KickoffDryRun checks if kickoff can be executed in dry-run mode.
// This is a placeholder implementation.
func CheckN1KickoffDryRun(rootPath string) GateResult {
	// Placeholder: check if essential files exist for kickoff
	configPath := filepath.Join(rootPath, ".ai", "config", "workflow.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return Fail("workflow.yaml not found for kickoff")
	}

	skillPath := filepath.Join(rootPath, ".ai", "skills", "principal-workflow", "SKILL.md")
	if _, err := os.Stat(skillPath); os.IsNotExist(err) {
		return Fail("principal-workflow skill not found")
	}

	return Pass("kickoff prerequisites exist")
}

// CheckN2RollbackOutput checks if rollback produces expected output.
// This is a placeholder implementation.
func CheckN2RollbackOutput(rootPath string) GateResult {
	// Placeholder: check if rollback-related infrastructure exists
	return Skip("rollback output check not implemented")
}

// CheckN3StatsJSON checks if stats.json can be generated correctly.
// This is a placeholder implementation.
func CheckN3StatsJSON(rootPath string) GateResult {
	// Check if stats.json exists in results directory
	statsPath := filepath.Join(rootPath, ".ai", "results", "stats.json")
	data, err := os.ReadFile(statsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Skip("stats.json not found (no workflow runs yet)")
		}
		return Fail("cannot read stats.json: " + err.Error())
	}

	// Validate JSON structure
	var stats map[string]interface{}
	if err := json.Unmarshal(data, &stats); err != nil {
		return Fail("invalid JSON in stats.json: " + err.Error())
	}

	return Pass("stats.json is valid")
}

// RunOnlineGates executes all online gate checks for a given root path.
func RunOnlineGates(rootPath string) OnlineGateResults {
	return OnlineGateResults{
		N1: CheckN1KickoffDryRun(rootPath),
		N2: CheckN2RollbackOutput(rootPath),
		N3: CheckN3StatsJSON(rootPath),
	}
}
