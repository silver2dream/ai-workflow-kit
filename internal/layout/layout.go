// Package layout says where the files of a project are: what AWK installs
// into a project, and what it reads and writes there.
//
// A location is decided here and nowhere else. Code that needs a path asks
// this package for it; code that prints one, in a message, a prompt or a
// generated document, prints one of the constants.
package layout

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Where things are, relative to the root of the project, with forward
// slashes.
//
// A project holds three things of AWK's under .ai: what the project says
// (ConfigFile, RosterFile) and what a run leaves behind (StateDir). What
// agents read is under .claude, where Claude Code looks for it.
const (
	// ConfigFile is the configuration of the project.
	ConfigFile = ".ai/config.yaml"
	// RosterFile names the roles that plan when `awkit plan` runs.
	RosterFile = ".ai/roster.yaml"

	// RulesDir holds the rule files agents are told to read. The ones in
	// KitRulesDir are AWK's: an upgrade replaces them. The others are the
	// project's own.
	RulesDir    = ".claude/rules"
	KitRulesDir = ".claude/rules/_kit"
	// SkillsDir holds the skills the Principal works from.
	SkillsDir = ".claude/skills"
	// AgentsDir holds the subagents the Principal hands work to.
	AgentsDir = ".claude/agents"

	// StateDir holds what a run leaves behind: what it remembers from one
	// step to the next, its results and its logs. Nothing in it is
	// committed.
	StateDir = ".ai/state"
	// ResultsDir holds one file per issue: what the Worker's run came to.
	ResultsDir = StateDir + "/results"
	// RunsDir holds one directory per issue: the Worker's prompt, its plan
	// and its summary.
	RunsDir = StateDir + "/runs"
	// LogsDir holds the logs of the Principal and of the Workers.
	LogsDir = StateDir + "/logs"
	// TempDir holds files that live for one step, such as the ticket a
	// dispatch hands the Worker.
	TempDir = StateDir + "/tmp"
)

// Where the same things were before. `awkit upgrade` moves a project from
// there to here; a command that finds a project still there says so.
const (
	OldConfigFile = ".ai/config/workflow.yaml"
	OldRulesDir   = ".ai/rules"
	OldSkillsDir  = ".ai/skills"
	OldResultsDir = ".ai/results"
	OldRunsDir    = ".ai/runs"
	OldLogsDir    = ".ai/exe-logs"
	OldTempDir    = ".ai/temp"
)

// In returns a location of the project at root, as a path of this system.
func In(root, location string) string {
	return filepath.Join(root, filepath.FromSlash(location))
}

// Config returns the configuration file of the project at root.
func Config(root string) string { return In(root, ConfigFile) }

// ProjectOf returns the root of the project a configuration file belongs to:
// as many directories up as ConfigFile is deep. A file that lies directly
// where a command was started gives ".".
func ProjectOf(configFile string) string {
	project := configFile
	for range strings.Split(ConfigFile, "/") {
		project = filepath.Dir(project)
	}
	return project
}

// Roster returns the roster file of the project at root.
func Roster(root string) string { return In(root, RosterFile) }

// Rules returns the directory of the project's rule files.
func Rules(root string) string { return In(root, RulesDir) }

// KitRules returns the directory of the rule files AWK ships.
func KitRules(root string) string { return In(root, KitRulesDir) }

// Skills returns the directory of the project's skills.
func Skills(root string) string { return In(root, SkillsDir) }

// Agents returns the directory of the project's subagents.
func Agents(root string) string { return In(root, AgentsDir) }

// State returns the state directory, or a path below it.
func State(root string, elem ...string) string {
	return filepath.Join(append([]string{In(root, StateDir)}, elem...)...)
}

// Results returns the directory of the result files.
func Results(root string) string { return In(root, ResultsDir) }

// Result returns the result file of an issue.
func Result(root string, issue int) string {
	return filepath.Join(Results(root), fmt.Sprintf("issue-%d.json", issue))
}

// Runs returns the directory that holds the run directories.
func Runs(root string) string { return In(root, RunsDir) }

// Run returns the run directory of an issue.
func Run(root string, issue int) string {
	return filepath.Join(Runs(root), RunName(issue))
}

// RunName is the name of an issue's run directory. The Worker writes into a
// directory of this name in its own worktree as well.
func RunName(issue int) string { return fmt.Sprintf("issue-%d", issue) }

// Logs returns the directory of the logs.
func Logs(root string) string { return In(root, LogsDir) }

// PrincipalLog returns the log of the Principal.
func PrincipalLog(root string) string { return filepath.Join(Logs(root), "principal.log") }

// WorkerLog returns the log of the Worker that ran for an issue.
func WorkerLog(root string, issue int) string {
	return filepath.Join(Logs(root), fmt.Sprintf("issue-%d.worker.log", issue))
}

// Temp returns the directory of the short-lived files.
func Temp(root string) string { return In(root, TempDir) }

// Ticket returns the ticket file a dispatch writes for an issue's Worker.
func Ticket(root string, issue int) string {
	return filepath.Join(Temp(root), fmt.Sprintf("ticket-%d.md", issue))
}
