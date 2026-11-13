package install

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/generate"
	"gopkg.in/yaml.v3"
)

// skeletonWorkflow is a generated workflow as a runner would carry it out.
type skeletonWorkflow struct {
	Jobs map[string]struct {
		Defaults struct {
			Run struct {
				WorkingDirectory string `yaml:"working-directory"`
			} `yaml:"run"`
		} `yaml:"defaults"`
		Steps []struct {
			Name string            `yaml:"name"`
			Uses string            `yaml:"uses"`
			With map[string]string `yaml:"with"`
			Run  string            `yaml:"run"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

// TestSkeleton_BuildsAndTestsWithItsGeneratedCI does what a new project's first
// CI run does: it initializes a godot-go project with its skeleton and carries
// out the run steps of every workflow generated for it, in the directory the
// workflow names. The commands come from the workflows, so the skeleton, the
// preset's verify commands and the CI generator cannot drift apart unnoticed.
//
// It needs Go, the .NET SDK and NuGet, so it runs only when asked:
//
//	AWKIT_SKELETON_E2E=1 go test ./internal/install/ -run TestSkeleton -v
func TestSkeleton_BuildsAndTestsWithItsGeneratedCI(t *testing.T) {
	if os.Getenv("AWKIT_SKELETON_E2E") == "" {
		t.Skip("set AWKIT_SKELETON_E2E=1 to build the godot-go skeleton (needs Go, the .NET SDK and NuGet)")
	}
	for _, tool := range []string{"sh", "go", "dotnet"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatalf("%s is needed to build the skeleton: %v", tool, err)
		}
	}

	dir := t.TempDir()
	result, err := Install(createMinimalMockFS(), dir, Options{
		Preset:      PresetGodotGo,
		ProjectName: "Sky Realm",
		Scaffold:    true,
		CI:          generate.CISync,
	})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if result.ScaffoldError != nil {
		t.Fatalf("scaffold: %v", result.ScaffoldError)
	}
	if result.GenerateError != nil {
		t.Fatalf("generate: %v", result.GenerateError)
	}

	workflows := workflowFiles(t, dir)
	if got := workflowNames(workflows); !reflect.DeepEqual(got, []string{"ci-backend.yml", "ci-frontend.yml"}) {
		t.Fatalf("workflows = %v, want one per repo", got)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	env := append(os.Environ(), "DOTNET_NOLOGO=1", "DOTNET_CLI_TELEMETRY_OPTOUT=1")

	for _, name := range workflowNames(workflows) {
		var workflow skeletonWorkflow
		if err := yaml.Unmarshal([]byte(workflows[name]), &workflow); err != nil {
			t.Fatalf("%s is not valid YAML: %v", name, err)
		}
		job, ok := workflow.Jobs["build"]
		if !ok {
			t.Fatalf("%s has no build job", name)
		}
		workDir := filepath.Join(dir, filepath.FromSlash(job.Defaults.Run.WorkingDirectory))
		if info, err := os.Stat(workDir); err != nil || !info.IsDir() {
			t.Fatalf("%s runs in %q, which the skeleton does not have", name, job.Defaults.Run.WorkingDirectory)
		}

		ran := 0
		for _, step := range job.Steps {
			// What a setup action is pointed at has to exist in the skeleton.
			if file := step.With["go-version-file"]; file != "" {
				if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(file))); err != nil {
					t.Errorf("%s: %s reads the Go version from %q, which the skeleton does not have", name, step.Uses, file)
				}
			}
			if step.Run == "" {
				continue
			}
			// A runner stops a script at the first command that fails.
			cmd := exec.CommandContext(ctx, "sh", "-c", "set -e\n"+step.Run)
			cmd.Dir = workDir
			cmd.Env = env
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s, step %q (%s) failed in %s: %v\n%s", name, step.Name, strings.TrimSpace(step.Run), job.Defaults.Run.WorkingDirectory, err, output)
			}
			t.Logf("%s: %s: %s: ok", name, step.Name, strings.TrimSpace(step.Run))
			ran++
		}
		if ran < 2 {
			t.Errorf("%s ran %d commands, want at least a build and a test", name, ran)
		}
	}
}
