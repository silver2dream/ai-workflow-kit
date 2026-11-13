package layout

import (
	"path/filepath"
	"testing"
)

// Where the files of a project are. A location that changes here changes for
// every project: `awkit upgrade` has to move what is already there.
func TestLocations(t *testing.T) {
	root := filepath.Join("some", "project")
	at := func(parts ...string) string {
		return filepath.Join(append([]string{root}, parts...)...)
	}
	tests := []struct {
		name, got, want string
	}{
		{"the configuration", Config(root), at(".ai", "config", "workflow.yaml")},
		{"the roster", Roster(root), at(".ai", "roster.yaml")},
		{"the rules", Rules(root), at(".ai", "rules")},
		{"the kit's rules", KitRules(root), at(".ai", "rules", "_kit")},
		{"the skills", Skills(root), at(".ai", "skills")},
		{"the state", State(root), at(".ai", "state")},
		{"a file of the state", State(root, "STOP"), at(".ai", "state", "STOP")},
		{"a file below the state", State(root, "principal", "session.json"), at(".ai", "state", "principal", "session.json")},
		{"the results", Results(root), at(".ai", "results")},
		{"the result of an issue", Result(root, 12), at(".ai", "results", "issue-12.json")},
		{"the runs", Runs(root), at(".ai", "runs")},
		{"the run of an issue", Run(root, 12), at(".ai", "runs", "issue-12")},
		{"the logs", Logs(root), at(".ai", "exe-logs")},
		{"the Principal's log", PrincipalLog(root), at(".ai", "exe-logs", "principal.log")},
		{"a Worker's log", WorkerLog(root, 12), at(".ai", "exe-logs", "issue-12.worker.log")},
		{"the short-lived files", Temp(root), at(".ai", "temp")},
		{"the ticket of a dispatch", Ticket(root, 12), at(".ai", "temp", "ticket-12.md")},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s is at %q, want %q", tt.name, tt.got, tt.want)
		}
	}

	if got := RunName(12); got != "issue-12" {
		t.Errorf("the run directory of issue 12 is called %q", got)
	}

	// From a configuration file back to its project, wherever the project is.
	for _, project := range []string{root, ".", filepath.Join("..", "other")} {
		if got := ProjectOf(Config(project)); got != filepath.Clean(project) {
			t.Errorf("the project of %q is %q, want %q", Config(project), got, filepath.Clean(project))
		}
	}
}

// What is printed is the location from the root of the project, with forward
// slashes whatever the system.
func TestLocationsAsPrinted(t *testing.T) {
	for name, pair := range map[string][2]string{
		"ConfigFile":  {ConfigFile, ".ai/config/workflow.yaml"},
		"RosterFile":  {RosterFile, ".ai/roster.yaml"},
		"RulesDir":    {RulesDir, ".ai/rules"},
		"KitRulesDir": {KitRulesDir, ".ai/rules/_kit"},
		"SkillsDir":   {SkillsDir, ".ai/skills"},
		"StateDir":    {StateDir, ".ai/state"},
		"ResultsDir":  {ResultsDir, ".ai/results"},
		"RunsDir":     {RunsDir, ".ai/runs"},
		"LogsDir":     {LogsDir, ".ai/exe-logs"},
		"TempDir":     {TempDir, ".ai/temp"},
	} {
		if pair[0] != pair[1] {
			t.Errorf("%s = %q, want %q", name, pair[0], pair[1])
		}
	}

	// Without a root, a path is the location itself, in this system's form:
	// what a command that works from the project's root uses.
	if got, want := Logs(""), filepath.Join(".ai", "exe-logs"); got != want {
		t.Errorf("Logs without a root = %q, want %q", got, want)
	}
	if got, want := In(".", StateDir), filepath.Join(".ai", "state"); got != want {
		t.Errorf("In(., StateDir) = %q, want %q", got, want)
	}
}
