package landing

import (
	"context"
	"strings"
	"testing"
)

// The pull request that brings a plan's documents to the integration branch
// carries their summary to GitHub: a credential in it stops the call.
func TestGH_CreatePRWithACredentialIsNotMade(t *testing.T) {
	called := false
	gh := GH{Run: func(ctx context.Context, dir, name string, args ...string) (string, error) {
		called = true
		return "https://github.com/o/r/pull/3\n", nil
	}}
	secret := "sk-" + "ant-api03-" + strings.Repeat("aB3dE5", 7)
	_, err := gh.CreatePR(context.Background(), t.TempDir(), "develop", "docs/plan", "[docs] plan", "key: "+secret)
	if err == nil || !strings.HasPrefix(err.Error(), "AWK guard:") {
		t.Fatalf("err = %v, want the guard's refusal", err)
	}
	if called {
		t.Error("gh was run")
	}

	pr, err := gh.CreatePR(context.Background(), t.TempDir(), "develop", "docs/plan", "[docs] plan", "The plan's documents.")
	if err != nil || pr == nil || pr.Number != 3 {
		t.Fatalf("a clean pull request: %+v, %v", pr, err)
	}
}
