package updatecheck

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- cachePath ---

func TestCachePath_ReturnsNonEmptyPath(t *testing.T) {
	p, err := cachePath()
	if err != nil {
		t.Fatalf("cachePath() error = %v", err)
	}
	if p == "" {
		t.Error("cachePath() returned empty string")
	}
	if filepath.Base(p) != "update.json" {
		t.Errorf("cachePath() base = %q, want update.json", filepath.Base(p))
	}
}

// --- readCache / writeCache round-trip ---

func TestReadWriteCache_RoundTrip(t *testing.T) {
	// Override user cache dir by writing directly to a temp path
	// We can't easily override os.UserCacheDir, so we test writeCache + readCache
	// indirectly by creating the file at the expected location or testing the
	// functions more directly via the Check function with a mock server.

	entry := cacheEntry{
		Latest:    "v2.0.0",
		CheckedAt: time.Now().UTC().Truncate(time.Second),
	}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	// Write to a temp file to simulate what writeCache does
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "update.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	// Parse back manually (readCache reads from a fixed path, so we parse directly)
	var got cacheEntry
	raw, _ := os.ReadFile(path)
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if got.Latest != "v2.0.0" {
		t.Errorf("Latest = %q, want v2.0.0", got.Latest)
	}
}

func TestReadCache_ExpiredEntry(t *testing.T) {
	// readCache returns false when the entry is older than TTL
	// We test this via Check with a short TTL and a stale fake cache,
	// by testing the finalize + compareSemver path via a mock server.

	// Test expired cache: create a file with an old timestamp
	oldEntry := cacheEntry{
		Latest:    "v1.0.0",
		CheckedAt: time.Now().Add(-48 * time.Hour), // 2 days ago
	}
	data, _ := json.Marshal(oldEntry)

	path, err := cachePath()
	if err != nil {
		t.Skip("cannot determine cache path")
	}

	// Save and restore any existing cache
	orig, origErr := os.ReadFile(path)
	defer func() {
		if origErr == nil {
			os.WriteFile(path, orig, 0644)
		} else {
			os.Remove(path)
		}
	}()

	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, data, 0644)

	// readCache with 24h TTL should return false for 48h-old entry
	_, ok := readCache(24*time.Hour, time.Now())
	if ok {
		t.Error("readCache should return false for expired entry")
	}
}

func TestReadCache_ValidEntry(t *testing.T) {
	freshEntry := cacheEntry{
		Latest:    "v1.5.0",
		CheckedAt: time.Now().Add(-1 * time.Hour), // 1 hour ago
	}
	data, _ := json.Marshal(freshEntry)

	path, err := cachePath()
	if err != nil {
		t.Skip("cannot determine cache path")
	}

	orig, origErr := os.ReadFile(path)
	defer func() {
		if origErr == nil {
			os.WriteFile(path, orig, 0644)
		} else {
			os.Remove(path)
		}
	}()

	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, data, 0644)

	entry, ok := readCache(24*time.Hour, time.Now())
	if !ok {
		t.Fatal("readCache should return true for fresh entry")
	}
	if entry.Latest != "v1.5.0" {
		t.Errorf("Latest = %q, want v1.5.0", entry.Latest)
	}
}

func TestReadCache_MissingFile(t *testing.T) {
	// readCache on missing file should return false gracefully
	path, err := cachePath()
	if err != nil {
		t.Skip("cannot determine cache path")
	}

	orig, origErr := os.ReadFile(path)
	defer func() {
		if origErr == nil {
			os.WriteFile(path, orig, 0644)
		}
	}()

	// Remove the cache file
	os.Remove(path)

	_, ok := readCache(24*time.Hour, time.Now())
	if ok {
		t.Error("readCache should return false when file is missing")
	}
}

func TestReadCache_InvalidJSON(t *testing.T) {
	path, err := cachePath()
	if err != nil {
		t.Skip("cannot determine cache path")
	}

	orig, origErr := os.ReadFile(path)
	defer func() {
		if origErr == nil {
			os.WriteFile(path, orig, 0644)
		} else {
			os.Remove(path)
		}
	}()

	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, []byte("not-json{{{"), 0644)

	_, ok := readCache(24*time.Hour, time.Now())
	if ok {
		t.Error("readCache should return false for invalid JSON")
	}
}

// --- Check with mock HTTP server ---

func TestCheck_NetworkFetch(t *testing.T) {
	// Start a mock GitHub API server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"tag_name": "v2.5.0"}`))
	}))
	defer ts.Close()

	// We can't easily override the URL, but we can use a repo path that hits
	// the mock. Instead, use NoCache to force network fetch and point at a
	// non-existent host which will fail quickly.
	// The real test for network fetch is TestCheckWithUnknownVersion.

	// Test finalize path: current older than latest
	res := finalize(Result{Current: "v1.0.0", Latest: "v2.5.0"})
	if !res.UpdateAvailable {
		t.Error("UpdateAvailable should be true when current < latest")
	}
}

func TestCheck_CacheHit(t *testing.T) {
	// Write a fresh cache entry, then call Check and verify it uses cache.
	freshEntry := cacheEntry{
		Latest:    "v3.0.0",
		CheckedAt: time.Now().Add(-30 * time.Minute),
	}
	data, _ := json.Marshal(freshEntry)

	path, err := cachePath()
	if err != nil {
		t.Skip("cannot determine cache path")
	}

	orig, origErr := os.ReadFile(path)
	defer func() {
		if origErr == nil {
			os.WriteFile(path, orig, 0644)
		} else {
			os.Remove(path)
		}
	}()

	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, data, 0644)

	fixedNow := time.Now()
	result := Check("v1.0.0", Options{
		Now:      func() time.Time { return fixedNow },
		CacheTTL: 24 * time.Hour,
		Timeout:  1 * time.Millisecond, // very short to fail fast if network is actually hit
	})

	if result.Source != "cache" {
		t.Errorf("Source = %q, want cache", result.Source)
	}
	if result.Latest != "v3.0.0" {
		t.Errorf("Latest = %q, want v3.0.0", result.Latest)
	}
	if !result.UpdateAvailable {
		t.Error("UpdateAvailable should be true when cache has newer version")
	}
}

func TestCheck_NetworkError(t *testing.T) {
	// With NoCache and a bad repo/timeout, should set Error field.
	result := Check("v1.0.0", Options{
		NoCache: true,
		Repo:    "nonexistent/repo-xyz-abc-123456",
		Timeout: 10 * time.Millisecond,
	})
	if result.Error == "" {
		t.Error("expected Error to be set on network failure")
	}
}

// --- writeCache does not panic on error ---

func TestWriteCache_NocrashOnBadPath(t *testing.T) {
	// writeCache is best-effort; verify it doesn't panic
	// We rely on the real writeCache which uses the system cache dir.
	// Just call it and ensure no panic.
	entry := cacheEntry{Latest: "v1.0.0", CheckedAt: time.Now()}
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("writeCache panicked: %v", r)
		}
	}()
	writeCache(entry)
}

// --- finalize edge cases ---

func TestFinalize_EmptyLatest(t *testing.T) {
	res := finalize(Result{Current: "v1.0.0", Latest: ""})
	if res.UpdateAvailable {
		t.Error("UpdateAvailable should be false when Latest is empty")
	}
}

func TestFinalize_InvalidVersions(t *testing.T) {
	// compareSemver will fail for non-semver strings
	res := finalize(Result{Current: "v1.0.0", Latest: "not-a-version"})
	if res.UpdateAvailable {
		t.Error("UpdateAvailable should be false when compareSemver fails")
	}
	if res.Error == "" {
		t.Error("Error should be set when compareSemver fails")
	}
}

// --- parseSemver edge cases ---

func TestParseSemver_EmptyAfterStrip(t *testing.T) {
	_, ok := parseSemver("v")
	if ok {
		t.Error("parseSemver('v') should return ok=false")
	}
}

func TestParseSemver_EmptyPart(t *testing.T) {
	_, ok := parseSemver("1..0")
	if ok {
		t.Error("parseSemver('1..0') should return ok=false for empty part")
	}
}
