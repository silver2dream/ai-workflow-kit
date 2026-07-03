package worker

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
)

// LogUsage holds LLM token/cost usage parsed from a worker log.
//
// Numbers are best-effort: they reflect the LAST usage report seen in the
// log, which is exact for backends that emit one final report (claude JSON
// output) and a lower bound for backends that emit cumulative per-turn
// reports. Backends that log plain text (claude --print) yield zeros.
type LogUsage struct {
	TokensIn  int64
	TokensOut int64
	CostUSD   float64
}

// ScanUsageFromLog tolerantly scans a log file for JSON lines carrying LLM
// usage data (claude stream-json "result" events, codex --json events) and
// returns the last-reported usage. Missing or unparseable files return zeros.
func ScanUsageFromLog(logPath string) LogUsage {
	f, err := os.Open(logPath)
	if err != nil {
		return LogUsage{}
	}
	defer f.Close()

	var usage LogUsage
	scanner := bufio.NewScanner(f)
	// Worker logs can contain long JSON lines (full diffs in events).
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		if cost, ok := event["total_cost_usd"].(float64); ok && cost > 0 {
			usage.CostUSD = cost
		}
		if in, out, ok := extractUsageTokens(event); ok {
			usage.TokensIn = in
			usage.TokensOut = out
		}
	}
	return usage
}

// extractUsageTokens finds a "usage" object with input/output token counts,
// either at the top level or nested one level down (e.g. codex "info" or
// claude "message" wrappers).
func extractUsageTokens(event map[string]any) (in, out int64, ok bool) {
	if u, found := usageTokensFrom(event["usage"]); found {
		return u[0], u[1], true
	}
	for _, v := range event {
		nested, isMap := v.(map[string]any)
		if !isMap {
			continue
		}
		if u, found := usageTokensFrom(nested["usage"]); found {
			return u[0], u[1], true
		}
	}
	return 0, 0, false
}

// usageTokensFrom reads input/output token counts from a usage object.
func usageTokensFrom(v any) ([2]int64, bool) {
	usage, ok := v.(map[string]any)
	if !ok {
		return [2]int64{}, false
	}
	in, inOK := numericField(usage, "input_tokens")
	out, outOK := numericField(usage, "output_tokens")
	if !inOK && !outOK {
		return [2]int64{}, false
	}
	return [2]int64{in, out}, true
}

func numericField(m map[string]any, key string) (int64, bool) {
	if f, ok := m[key].(float64); ok {
		return int64(f), true
	}
	return 0, false
}
