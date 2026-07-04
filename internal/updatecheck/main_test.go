package updatecheck

import (
	"os"
	"testing"
)

// TestMain isolates every test in this package from the real user cache dir by
// pointing AWKIT_CACHE_DIR at a throwaway temp directory. Without this, tests
// that call writeCache/Check wrote to the actual ~/.cache/awkit/update.json —
// leaking a fake "v1.0.0" that made every developer's awkit report a bogus
// "update available".
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "awkit-updatecheck-cache-*")
	if err != nil {
		panic("updatecheck test setup: " + err.Error())
	}
	_ = os.Setenv(cacheDirEnv, dir)

	code := m.Run()

	_ = os.RemoveAll(dir)
	os.Exit(code)
}
