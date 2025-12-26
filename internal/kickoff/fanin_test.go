package kickoff

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFanInManager_BasicOperation(t *testing.T) {
	fanIn := NewFanInManager(100)
	defer fanIn.Stop()

	// Test sending Claude lines
	go func() {
		fanIn.SendClaudeLine("claude line 1")
		fanIn.SendClaudeLine("claude line 2")
	}()

	// Read from channel
	var lines []LogLine
	timeout := time.After(500 * time.Millisecond)
	for i := 0; i < 2; i++ {
		select {
		case line := <-fanIn.Channel():
			lines = append(lines, line)
		case <-timeout:
			t.Fatal("Timeout waiting for lines")
		}
	}

	if len(lines) != 2 {
		t.Errorf("Expected 2 lines, got %d", len(lines))
	}

	if lines[0].Source != "claude" {
		t.Errorf("Expected source 'claude', got '%s'", lines[0].Source)
	}
	if lines[0].Text != "claude line 1" {
		t.Errorf("Expected 'claude line 1', got '%s'", lines[0].Text)
	}
}

func TestFanInManager_PrincipalTailer(t *testing.T) {
	tmpDir := t.TempDir()

	fanIn := NewFanInManager(100)
	defer fanIn.Stop()

	// Start principal tailer
	fanIn.StartPrincipalTailer(tmpDir)

	// Give tailer time to start
	time.Sleep(150 * time.Millisecond)

	// Create and write to principal.log
	logPath := filepath.Join(tmpDir, "principal.log")
	f, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("Failed to create log file: %v", err)
	}
	f.Close()

	time.Sleep(150 * time.Millisecond)

	f, _ = os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
	f.WriteString("[PRINCIPAL] test message\n")
	f.Close()

	// Wait for line to be read
	timeout := time.After(500 * time.Millisecond)
	select {
	case line := <-fanIn.Channel():
		if line.Source != "principal" {
			t.Errorf("Expected source 'principal', got '%s'", line.Source)
		}
		if line.Text != "[PRINCIPAL] test message" {
			t.Errorf("Expected '[PRINCIPAL] test message', got '%s'", line.Text)
		}
	case <-timeout:
		t.Fatal("Timeout waiting for principal log line")
	}
}

func TestFanInManager_WorkerTailerDynamic(t *testing.T) {
	tmpDir := t.TempDir()

	fanIn := NewFanInManager(100)
	defer fanIn.Stop()

	// Start worker tailer for issue 42
	fanIn.StartWorkerTailer(tmpDir, 42)

	if fanIn.CurrentIssueID() != 42 {
		t.Errorf("Expected current issue ID 42, got %d", fanIn.CurrentIssueID())
	}

	// Give tailer time to start
	time.Sleep(150 * time.Millisecond)

	// Create and write to worker log
	logPath := filepath.Join(tmpDir, "issue-42.worker.log")
	f, _ := os.Create(logPath)
	f.Close()

	time.Sleep(150 * time.Millisecond)

	f, _ = os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
	f.WriteString("[WORKER] worker message\n")
	f.Close()

	// Wait for line
	timeout := time.After(500 * time.Millisecond)
	select {
	case line := <-fanIn.Channel():
		if line.Source != "worker" {
			t.Errorf("Expected source 'worker', got '%s'", line.Source)
		}
		if line.IssueID != 42 {
			t.Errorf("Expected issue ID 42, got %d", line.IssueID)
		}
	case <-timeout:
		t.Fatal("Timeout waiting for worker log line")
	}

	// Stop worker tailer
	fanIn.StopWorkerTailer()

	if fanIn.CurrentIssueID() != 0 {
		t.Errorf("Expected current issue ID 0 after stop, got %d", fanIn.CurrentIssueID())
	}
}

func TestFanInManager_WorkerTailerReplacement(t *testing.T) {
	tmpDir := t.TempDir()

	fanIn := NewFanInManager(100)
	defer fanIn.Stop()

	// Start worker tailer for issue 1
	fanIn.StartWorkerTailer(tmpDir, 1)

	if fanIn.CurrentIssueID() != 1 {
		t.Errorf("Expected issue ID 1, got %d", fanIn.CurrentIssueID())
	}

	// Replace with issue 2
	fanIn.StartWorkerTailer(tmpDir, 2)

	if fanIn.CurrentIssueID() != 2 {
		t.Errorf("Expected issue ID 2, got %d", fanIn.CurrentIssueID())
	}
}

func TestFanInManager_GracefulShutdown(t *testing.T) {
	tmpDir := t.TempDir()

	fanIn := NewFanInManager(100)

	// Start both tailers
	fanIn.StartPrincipalTailer(tmpDir)
	fanIn.StartWorkerTailer(tmpDir, 99)

	time.Sleep(100 * time.Millisecond)

	// Stop should complete gracefully
	start := time.Now()
	fanIn.Stop()
	elapsed := time.Since(start)

	// Should complete within 2 seconds (1 second per tailer max)
	if elapsed > 2500*time.Millisecond {
		t.Errorf("Stop took too long: %v", elapsed)
	}

	// Channel should be closed
	_, ok := <-fanIn.Channel()
	if ok {
		t.Error("Channel should be closed after Stop()")
	}
}

func TestFanInManager_IdempotentStop(t *testing.T) {
	fanIn := NewFanInManager(100)

	// Multiple stops should not panic
	fanIn.Stop()
	fanIn.Stop()
	fanIn.Stop()
}

func TestFanInManager_SendAfterStop(t *testing.T) {
	fanIn := NewFanInManager(100)
	fanIn.Stop()

	// SendClaudeLine after stop should not panic or block
	done := make(chan bool)
	go func() {
		fanIn.SendClaudeLine("should be ignored")
		done <- true
	}()

	select {
	case <-done:
		// OK
	case <-time.After(100 * time.Millisecond):
		t.Error("SendClaudeLine blocked after Stop()")
	}
}

func TestFanInManager_MultipleSources(t *testing.T) {
	tmpDir := t.TempDir()

	fanIn := NewFanInManager(100)
	defer fanIn.Stop()

	// Start both tailers
	fanIn.StartPrincipalTailer(tmpDir)
	fanIn.StartWorkerTailer(tmpDir, 5)

	time.Sleep(150 * time.Millisecond)

	// Create log files
	principalLog := filepath.Join(tmpDir, "principal.log")
	workerLog := filepath.Join(tmpDir, "issue-5.worker.log")

	f1, _ := os.Create(principalLog)
	f1.Close()
	f2, _ := os.Create(workerLog)
	f2.Close()

	time.Sleep(150 * time.Millisecond)

	// Write to both files and send Claude line
	go func() {
		fanIn.SendClaudeLine("from claude")
	}()

	f1, _ = os.OpenFile(principalLog, os.O_APPEND|os.O_WRONLY, 0644)
	f1.WriteString("from principal\n")
	f1.Close()

	f2, _ = os.OpenFile(workerLog, os.O_APPEND|os.O_WRONLY, 0644)
	f2.WriteString("from worker\n")
	f2.Close()

	// Read all lines
	sources := make(map[string]bool)
	timeout := time.After(1 * time.Second)

	for i := 0; i < 3; i++ {
		select {
		case line := <-fanIn.Channel():
			sources[line.Source] = true
		case <-timeout:
			break
		}
	}

	// Should have received from all three sources
	if !sources["claude"] {
		t.Error("Missing line from claude")
	}
	if !sources["principal"] {
		t.Error("Missing line from principal")
	}
	if !sources["worker"] {
		t.Error("Missing line from worker")
	}
}
