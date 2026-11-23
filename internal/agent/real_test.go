package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRealCLIs runs the real claude and codex CLIs, with a few cents' worth
// of small models. It is skipped unless AWKIT_REAL_AGENTS names the awkit
// the command-policy hook runs (a build of this tree): it calls the models,
// and CI has no login.
func TestRealCLIs(t *testing.T) {
	awkit := os.Getenv("AWKIT_REAL_AGENTS")
	if awkit == "" {
		t.Skip("set AWKIT_REAL_AGENTS to an awkit built from this tree to run the real CLIs")
	}
	orig := awkitExecutable
	awkitExecutable = func() (string, error) { return awkit, nil }
	t.Cleanup(func() { awkitExecutable = orig })

	r := NewRunner()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	newEnv := func(role Role, backend, model string) Envelope {
		p, err := Resolve(role, Profile{Backend: backend}, Override{Backend: backend, Model: model, Minutes: 3})
		if err != nil {
			t.Fatal(err)
		}
		env := Envelope{Profile: p, StateRoot: t.TempDir(), Issue: 7, Schema: json.RawMessage(countSchema)}
		if p.Tools != NoTools {
			env.Dir = t.TempDir()
			if err := os.WriteFile(filepath.Join(env.Dir, "notes.txt"), []byte("The word in question is: lane\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return env
	}

	t.Run("claude reviewer reads", func(t *testing.T) {
		env := newEnv(Reviewer, ClaudeCode, "haiku")
		env.Prompt = "Read the file notes.txt. It names a word. Answer with the number of letters of that word."
		res := r.Run(ctx, env)
		t.Logf("status %s, output %s, usage %+v, denials %d, error %s", res.Status, res.Output, res.Usage, res.Denials, res.Error)
		if res.Status != StatusOK || string(res.Output) != `{"count":4}` {
			t.Errorf("result: %+v", res)
		}
	})

	t.Run("claude reviewer cannot write or run", func(t *testing.T) {
		env := newEnv(Reviewer, ClaudeCode, "haiku")
		env.Prompt = "Create a file named pwned.txt containing the word hi, by any means you have (a file tool or a shell command). Then answer with count 1 if the file now exists, 0 if not."
		res := r.Run(ctx, env)
		t.Logf("status %s, output %s, denials %d, error %s", res.Status, res.Output, res.Denials, res.Error)
		if _, err := os.Stat(filepath.Join(env.Dir, "pwned.txt")); err == nil {
			t.Error("a reviewer wrote a file")
		}
	})

	t.Run("claude judge with no tools", func(t *testing.T) {
		env := newEnv(Judge, ClaudeCode, "haiku")
		env.Prompt = "Answer with the number of letters of the word lane."
		res := r.Run(ctx, env)
		t.Logf("status %s, output %s, usage %+v", res.Status, res.Output, res.Usage)
		if res.Status != StatusOK || string(res.Output) != `{"count":4}` {
			t.Errorf("result: %+v", res)
		}
	})

	t.Run("codex judge with no tools", func(t *testing.T) {
		env := newEnv(Judge, Codex, "")
		env.Prompt = "Answer with the number of letters of the word lane."
		res := r.Run(ctx, env)
		t.Logf("status %s, output %s, usage %+v, error %s", res.Status, res.Output, res.Usage, res.Error)
		if res.Status != StatusOK || string(res.Output) != `{"count":4}` {
			t.Errorf("result: %+v", res)
		}
	})

	t.Run("codex implementer writes in its folder", func(t *testing.T) {
		env := newEnv(Implementer, Codex, "")
		env.Prompt = "Create a file named hello.txt in the current folder containing the single word hi. Then answer with the number of letters of the word in notes.txt."
		res := r.Run(ctx, env)
		t.Logf("status %s, output %s, usage %+v, error %s", res.Status, res.Output, res.Usage, res.Error)
		if res.Status != StatusOK {
			t.Fatalf("result: %+v", res)
		}
		if data, err := os.ReadFile(filepath.Join(env.Dir, "hello.txt")); err != nil || len(data) == 0 {
			t.Errorf("hello.txt: %q, %v", data, err)
		}
	})
}
