package worker

import (
	"os"
	"path/filepath"
	"testing"
)

func writeLog(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "issue-1.worker.log")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write log: %v", err)
	}
	return path
}

func TestScanUsageFromLog_ClaudeResultEvent(t *testing.T) {
	log := `starting worker...
{"type":"system","subtype":"init"}
{"type":"result","subtype":"success","total_cost_usd":0.1234,"usage":{"input_tokens":1500,"output_tokens":800}}
done
`
	got := ScanUsageFromLog(writeLog(t, log))
	if got.CostUSD != 0.1234 {
		t.Errorf("CostUSD = %v, want 0.1234", got.CostUSD)
	}
	if got.TokensIn != 1500 || got.TokensOut != 800 {
		t.Errorf("tokens = %d/%d, want 1500/800", got.TokensIn, got.TokensOut)
	}
}

func TestScanUsageFromLog_NestedUsageAndLastWins(t *testing.T) {
	// Codex-style: usage nested inside a wrapper object, cumulative reports —
	// the last one wins.
	log := `{"type":"turn.completed","info":{"usage":{"input_tokens":100,"output_tokens":50}}}
{"type":"turn.completed","info":{"usage":{"input_tokens":300,"output_tokens":120}}}
`
	got := ScanUsageFromLog(writeLog(t, log))
	if got.TokensIn != 300 || got.TokensOut != 120 {
		t.Errorf("tokens = %d/%d, want last-reported 300/120", got.TokensIn, got.TokensOut)
	}
}

func TestScanUsageFromLog_PlainTextLogYieldsZero(t *testing.T) {
	log := "worker started\nrunning go test ./...\nok\n"
	got := ScanUsageFromLog(writeLog(t, log))
	if got != (LogUsage{}) {
		t.Errorf("expected zero usage for plain text log, got %+v", got)
	}
}

func TestScanUsageFromLog_MissingFileYieldsZero(t *testing.T) {
	got := ScanUsageFromLog(filepath.Join(t.TempDir(), "nope.log"))
	if got != (LogUsage{}) {
		t.Errorf("expected zero usage for missing file, got %+v", got)
	}
}

func TestScanUsageFromLog_MalformedJSONIgnored(t *testing.T) {
	log := `{not json at all
{"type":"result","total_cost_usd":0.5,"usage":{"input_tokens":10,"output_tokens":5}}
`
	got := ScanUsageFromLog(writeLog(t, log))
	if got.CostUSD != 0.5 || got.TokensIn != 10 {
		t.Errorf("expected malformed lines skipped, got %+v", got)
	}
}
