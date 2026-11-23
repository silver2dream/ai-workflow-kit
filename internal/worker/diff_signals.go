package worker

import (
	"context"

	"github.com/silver2dream/ai-workflow-kit/internal/guard"
)

// recordDiffSignals records what in the Worker's staged change a person would
// want to look at: a call to another host, code run at run time, a change to
// the build or the dependencies, a binary, a mass deletion. It only records:
// the change goes on to its tests and its review either way.
func recordDiffSignals(ctx context.Context, wtDir, stateRoot string, issue int, log func(string, ...interface{})) {
	diff, err := gitOutput(ctx, wtDir, "diff", "--cached")
	if err != nil {
		return
	}
	numstat, err := gitOutput(ctx, wtDir, "diff", "--cached", "--numstat")
	if err != nil {
		return
	}
	verdicts := guard.ScanDiff(diff, numstat)
	if len(verdicts) == 0 {
		return
	}
	guard.Record(stateRoot, verdicts, issue)
	for _, v := range verdicts {
		log("⚑ %s: %s (%s)", v.Rule, v.Reason, v.Evidence)
	}
}
