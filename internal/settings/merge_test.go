package settings

import (
	"errors"
	"testing"
)

func TestMergeConfig(t *testing.T) {
	was := readConfig(t, configFile)

	// No retries is a change from the default, though both read as zero.
	mine := readConfig(t, configFile)
	mine.Retries.Worker = intOf(0)
	merged, err := mergeConfig(was, mine, readConfig(t, configFile))
	if err != nil || merged.Retries.Worker == nil || *merged.Retries.Worker != 0 {
		t.Errorf("retries = %+v, %v", merged.Retries, err)
	}

	// A repo the person changed that the branch no longer has.
	mine = readConfig(t, configFile)
	mine.Repos[0].Build = "make"
	theirs := readConfig(t, configFile)
	theirs.Repos = []RepoCommands{{Name: "server", Path: "server/"}}
	var conflict *Conflict
	if _, err := mergeConfig(was, mine, theirs); !errors.As(err, &conflict) {
		t.Errorf("err = %v", err)
	}
	// One the person left alone is not.
	mine = readConfig(t, configFile)
	mine.Name = "Arena"
	if merged, err := mergeConfig(was, mine, theirs); err != nil || merged.Name != "Arena" || len(merged.Repos) != 1 || merged.Repos[0].Name != "server" {
		t.Errorf("merged = %+v, %v", merged, err)
	}
}

func TestMergeRoster(t *testing.T) {
	was := readRoster(t, rosterFile)
	// Both made the same change to the roles: no conflict.
	mine, theirs := readRoster(t, rosterFile), readRoster(t, rosterFile)
	mine.Roles[2].Title, theirs.Roles[2].Title = "Critic", "Critic"
	mine.Roles[2].From = "skeptic"
	if _, err := mergeRoster(was, mine, theirs); err != nil {
		t.Errorf("same change: %v", err)
	}
	// A limit set back to its default is a change.
	mine = readRoster(t, rosterFile)
	mine.Limits.Topics = nil
	theirs = readRoster(t, rosterFile)
	if merged, err := mergeRoster(was, mine, theirs); err != nil || merged.Limits.Topics != nil || merged.Limits.Calls == nil {
		t.Errorf("limits = %+v, %v", merged.Limits, err)
	}
}

// Each setting of an agent the person changed is theirs; the others are the
// branch's.
func TestMergeConfig_TheAgents(t *testing.T) {
	was := readConfig(t, configFile)
	mine := readConfig(t, configFile)
	mine.Agents.Reviewer.Model = "opus"
	usd := 3.0
	mine.Agents.Resolver.USD = &usd
	theirs := readConfig(t, configFile)
	theirs.Agents.Reviewer.Minutes = intOf(30)
	theirs.Agents.Resolver.Backend = "claude-code"
	merged, err := mergeConfig(was, mine, theirs)
	if err != nil {
		t.Fatal(err)
	}
	r, s := merged.Agents.Reviewer, merged.Agents.Resolver
	if r.Model != "opus" || r.Minutes == nil || *r.Minutes != 30 || s.Backend != "claude-code" || s.USD == nil || *s.USD != 3 {
		t.Errorf("merged = %+v %+v", r, s)
	}
	// Taking a limit out is a change too.
	was.Agents.Reviewer.USD = &usd
	mine = readConfig(t, configFile)
	theirs = readConfig(t, configFile)
	theirs.Agents.Reviewer.USD = &usd
	if merged, _ := mergeConfig(was, mine, theirs); merged.Agents.Reviewer.USD != nil {
		t.Errorf("a limit taken out is put back: %v", *merged.Agents.Reviewer.USD)
	}
}

// How much the engine does at once is merged number by number, backend by
// backend: what the person changed, over what the branch has.
func TestMergeConfig_TheEngine(t *testing.T) {
	was := readConfig(t, configFile)
	was.Engine.MaxPerBackend = map[string]int{"codex": 2, "claude-code": 1}
	mine := readConfig(t, configFile)
	mine.Engine = Engine{MaxParallel: intOf(4), MaxPerBackend: map[string]int{"codex": 3, "claude-code": 1}}
	theirs := readConfig(t, configFile)
	theirs.Engine = Engine{MaxParallel: intOf(2), MaxPerBackend: map[string]int{"codex": 2}}
	merged, err := mergeConfig(was, mine, theirs)
	if err != nil {
		t.Fatal(err)
	}
	// Mine changed the number and codex; theirs took claude-code out.
	if merged.Engine.MaxParallel == nil || *merged.Engine.MaxParallel != 4 || len(merged.Engine.MaxPerBackend) != 1 || merged.Engine.MaxPerBackend["codex"] != 3 {
		t.Errorf("merged = %+v %v", merged.Engine, merged.Engine.MaxPerBackend)
	}
	// What only theirs changed is theirs; a limit only mine took out goes.
	mine = readConfig(t, configFile)
	mine.Engine.MaxPerBackend = map[string]int{"codex": 2}
	theirs = readConfig(t, configFile)
	theirs.Engine = Engine{MaxParallel: intOf(5), MaxPerBackend: map[string]int{"codex": 2, "claude-code": 1}}
	merged, _ = mergeConfig(was, mine, theirs)
	if *merged.Engine.MaxParallel != 5 || len(merged.Engine.MaxPerBackend) != 1 || merged.Engine.MaxPerBackend["codex"] != 2 {
		t.Errorf("merged = %+v %v", merged.Engine, merged.Engine.MaxPerBackend)
	}
}
