package worker

import (
	"slices"
	"testing"
)

func TestDetectCodexArgs(t *testing.T) {
	const newHelp = `Run Codex non-interactively
  -s, --sandbox <SANDBOX_MODE>  [possible values: read-only, workspace-write, danger-full-access]
      --json  Emit events as JSONL`
	const oldHelp = `  --full-auto   Low-friction sandboxed automatic execution
      --json`
	const yoloHelp = `  --yolo   bypass everything
      --json`
	const bareHelp = `  -m, --model <MODEL>`

	tests := []struct {
		name      string
		help      string
		wantArgs  []string
		wantFlags codexFlagsInfo
	}{
		{
			name:      "codex 0.14x uses --sandbox workspace-write",
			help:      newHelp,
			wantArgs:  []string{"exec", "--sandbox", "workspace-write", "--json"},
			wantFlags: codexFlagsInfo{Sandbox: true},
		},
		{
			name:      "older codex prefers --full-auto",
			help:      oldHelp,
			wantArgs:  []string{"exec", "--full-auto", "--json"},
			wantFlags: codexFlagsInfo{FullAuto: true},
		},
		{
			name:      "--yolo when that is the only automation flag",
			help:      yoloHelp,
			wantArgs:  []string{"exec", "--yolo", "--json"},
			wantFlags: codexFlagsInfo{Yolo: true},
		},
		{
			name:      "no automation flag falls back to bare exec",
			help:      bareHelp,
			wantArgs:  []string{"exec"},
			wantFlags: codexFlagsInfo{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, flags := detectCodexArgs(tt.help)
			if !slices.Equal(args, tt.wantArgs) {
				t.Errorf("args = %v, want %v", args, tt.wantArgs)
			}
			if flags != tt.wantFlags {
				t.Errorf("flags = %+v, want %+v", flags, tt.wantFlags)
			}
		})
	}
}

// TestDetectCodexArgs_RealWorldRegression guards the exact bug that shipped:
// codex 0.142.5 dropped --full-auto/--yolo and only offers --sandbox, so the old
// detection fell through to a bare `codex exec` that ran read-only and made zero
// changes. The help text below is trimmed from real `codex exec --help` output.
func TestDetectCodexArgs_RealWorldRegression(t *testing.T) {
	const help = `Options:
  -s, --sandbox <SANDBOX_MODE>
          Select the sandbox policy to use when executing model-generated shell commands
          [possible values: read-only, workspace-write, danger-full-access]
      --dangerously-bypass-approvals-and-sandbox
          Skip all confirmation prompts and execute commands without sandboxing.`

	args, flags := detectCodexArgs(help)
	if !flags.Sandbox {
		t.Fatal("codex 0.142.5 help must trigger the --sandbox path, not bare exec")
	}
	if !slices.Contains(args, "--sandbox") || !slices.Contains(args, "workspace-write") {
		t.Errorf("args %v must contain '--sandbox workspace-write'", args)
	}
	if flags.FullAuto || flags.Yolo {
		t.Errorf("0.142.5 has no --full-auto/--yolo, flags=%+v", flags)
	}
}
