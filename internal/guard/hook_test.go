package guard

import (
	"encoding/json"
	"runtime"
	"testing"
)

// The hook names the awkit that checks by its full path, then the role,
// the project and the branches; a branch left empty is no branch. A check
// that fails refuses the call where the CLI knows to.
func TestHook(t *testing.T) {
	h := Hook{Awkit: "C:/tools/awkit.exe", Role: RolePrincipal, StateRoot: "D:/games/rally", Protected: []string{"develop", "", "main"}}
	want := `"C:/tools/awkit.exe" guard tool --role principal --state-root "D:/games/rally" --protect "develop" --protect "main"`
	if got := h.Command(); got != want {
		t.Errorf("command = %s\nwant      %s", got, want)
	}
	// On Windows its paths are written with forward slashes, which the
	// shell that runs the hook reads as they are.
	if runtime.GOOS == "windows" {
		h.Awkit, h.StateRoot = `C:\tools\awkit.exe`, `D:\games\rally`
		if got := h.Command(); got != want {
			t.Errorf("command = %s\nwant      %s", got, want)
		}
	}
	h = Hook{Awkit: "/usr/local/bin/awkit", Role: RoleImplementer, AllowControl: true}
	if got, want := h.Command(), `"/usr/local/bin/awkit" guard tool --role implementer --allow-control`; got != want {
		t.Errorf("command = %s\nwant      %s", got, want)
	}

	for _, block := range []bool{true, false} {
		data, _ := json.Marshal(h.PreToolUse(block))
		var entry struct {
			Matcher string
			Hooks   []map[string]any
		}
		if err := json.Unmarshal(data, &entry); err != nil {
			t.Fatal(err)
		}
		if entry.Matcher != "*" || len(entry.Hooks) != 1 || entry.Hooks[0]["command"] != h.Command() || entry.Hooks[0]["type"] != "command" || entry.Hooks[0]["timeout"] != float64(HookTimeoutSeconds) {
			t.Errorf("entry = %s", data)
		}
		if _, has := entry.Hooks[0]["onFailure"]; has != block {
			t.Errorf("block %v: entry = %s", block, data)
		}
	}
}
