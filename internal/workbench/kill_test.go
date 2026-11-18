package workbench

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/kickoff"
)

// Ending a run at once ends what it started too: a process started as the
// page starts a run, which starts another, is ended with all it started.
func TestKillTree_EndsWhatTheProcessStarted(t *testing.T) {
	if testing.Short() {
		t.Skip("starts processes")
	}
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		script := "$p = Start-Process -FilePath ping -ArgumentList '-n','120','127.0.0.1' -PassThru -WindowStyle Hidden; " +
			"Set-Content -Path '" + pidFile + "' -Value $p.Id; Start-Sleep -Seconds 120"
		cmd = exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	} else {
		cmd = exec.Command("sh", "-c", "sleep 120 & echo $! > '"+pidFile+"'; wait")
	}
	cmd.SysProcAttr = detached()
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start a process to end: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	go func() { _ = cmd.Wait() }()

	child := 0
	for i := 0; i < 200 && child == 0; i++ {
		if data, err := os.ReadFile(pidFile); err == nil {
			child, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(string(data), string(rune(0xFEFF)))))
		}
		if child == 0 {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if child == 0 || !kickoff.ProcessAlive(child) {
		t.Fatalf("the process it started is not there: %d", child)
	}

	if err := killTree(cmd.Process.Pid); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100 && (kickoff.ProcessAlive(child) || kickoff.ProcessAlive(cmd.Process.Pid)); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if kickoff.ProcessAlive(cmd.Process.Pid) {
		t.Error("the process was not ended")
	}
	if kickoff.ProcessAlive(child) {
		t.Error("the process it started was not ended")
	}
}
