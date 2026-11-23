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
