package hooks

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
)

// HookRunner executes lifecycle hooks.
type HookRunner struct {
	config  analyzer.HooksConfig
	workDir string
	logOut  io.Writer
}

// NewHookRunner creates a new HookRunner.
func NewHookRunner(cfg analyzer.HooksConfig, workDir string, logOut io.Writer) *HookRunner {
	return &HookRunner{
		config:  cfg,
		workDir: workDir,
		logOut:  logOut,
	}
}

// Fire executes all hooks for the given event sequentially.
// Returns error only if a hook with on_failure="abort" fails.
func (r *HookRunner) Fire(ctx context.Context, event string, envVars map[string]string) error {
	hooks := r.config.GetHooks(event)
	if len(hooks) == 0 {
		return nil
	}

	for i, h := range hooks {
		if err := r.runHook(ctx, event, i, h, envVars); err != nil {
			policy := h.OnFailure
			if policy == "" {
				policy = "warn"
			}
			switch policy {
			case "abort":
				return fmt.Errorf("hook %s[%d] aborted: %w", event, i, err)
			case "warn":
				fmt.Fprintf(r.logOut, "[hooks] warning: %s[%d] failed: %v\n", event, i, err)
			case "ignore":
				// silently continue
			default:
				fmt.Fprintf(r.logOut, "[hooks] warning: %s[%d] failed (unknown policy %q): %v\n", event, i, policy, err)
			}
		}
	}

	return nil
}

func (r *HookRunner) runHook(ctx context.Context, event string, index int, h analyzer.HookDef, envVars map[string]string) error {
	if h.Command == "" {
		return nil
	}

	// Parse timeout
	var timeout time.Duration
	if h.Timeout != "" {
		var err error
		timeout, err = time.ParseDuration(h.Timeout)
		if err != nil {
			return fmt.Errorf("invalid timeout %q: %w", h.Timeout, err)
		}
	}

	// Apply timeout if specified
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// Build command: sh -c on Unix, cmd /c on Windows
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/c", h.Command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", h.Command)
	}
	cmd.Dir = r.workDir
	setProcGroup(cmd)

	// Build environment: os.Environ + config env + event-specific env
	env := os.Environ()
	for k, v := range h.Env {
		env = append(env, k+"="+v)
	}
	for k, v := range envVars {
		env = append(env, k+"="+v)
	}
	env = append(env, "AWK_EVENT="+event)
	cmd.Env = env

	cmd.Stdout = r.logOut
	cmd.Stderr = r.logOut

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("hook timed out after %s", timeout)
		}
		return err
	}

	return nil
}
