package worker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// NewWorkerPreflight defaults
// ---------------------------------------------------------------------------

func TestCov_NewWorkerPreflight_Defaults(t *testing.T) {
	pf := NewWorkerPreflight("/tmp/root", "", "")
	if pf.RepoType != "root" {
		t.Errorf("expected RepoType 'root', got %q", pf.RepoType)
	}
	if pf.RepoPath != "." {
		t.Errorf("expected RepoPath '.', got %q", pf.RepoPath)
	}
	if pf.Timeout != 60*time.Second {
		t.Errorf("expected Timeout 60s, got %v", pf.Timeout)
	}
}

func TestCov_NewWorkerPreflight_CustomValues(t *testing.T) {
	pf := NewWorkerPreflight("/tmp/root", "submodule", "backend/")
	if pf.RepoType != "submodule" {
		t.Errorf("expected RepoType 'submodule', got %q", pf.RepoType)
	}
	if pf.RepoPath != "backend/" {
		t.Errorf("expected RepoPath 'backend/', got %q", pf.RepoPath)
	}
}

// ---------------------------------------------------------------------------
// CacheEntry
// ---------------------------------------------------------------------------

func TestCov_CacheEntryJSON(t *testing.T) {
	entry := CacheEntry{
		Accessible: true,
		Timestamp:  float64(time.Now().Unix()),
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var loaded CacheEntry
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !loaded.Accessible {
		t.Error("expected Accessible=true")
	}
}

// ---------------------------------------------------------------------------
// checkCache / updateCache
// ---------------------------------------------------------------------------

func TestCov_CheckCache_MissingFile(t *testing.T) {
	pf := NewWorkerPreflight(t.TempDir(), "root", ".")
	_, ok := pf.checkCache("/nonexistent/cache.json", "key")
	if ok {
		t.Error("expected cache miss for missing file")
	}
}

func TestCov_CheckCache_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "cache.json")
	os.WriteFile(cachePath, []byte("not json"), 0644)

	pf := NewWorkerPreflight(dir, "root", ".")
	_, ok := pf.checkCache(cachePath, "key")
	if ok {
		t.Error("expected cache miss for invalid JSON")
	}
}

func TestCov_CheckCache_ExpiredEntry(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "cache.json")

	cache := map[string]CacheEntry{
		"key": {
			Accessible: true,
			Timestamp:  float64(time.Now().Unix() - cacheTTLSeconds - 100), // expired
		},
	}
	data, _ := json.Marshal(cache)
	os.WriteFile(cachePath, data, 0644)

	pf := NewWorkerPreflight(dir, "root", ".")
	_, ok := pf.checkCache(cachePath, "key")
	if ok {
		t.Error("expected cache miss for expired entry")
	}
}

func TestCov_CheckCache_ValidEntry(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "cache.json")

	cache := map[string]CacheEntry{
		"key": {
			Accessible: true,
			Timestamp:  float64(time.Now().Unix()), // fresh
		},
	}
	data, _ := json.Marshal(cache)
	os.WriteFile(cachePath, data, 0644)

	pf := NewWorkerPreflight(dir, "root", ".")
	accessible, ok := pf.checkCache(cachePath, "key")
	if !ok {
		t.Error("expected cache hit")
	}
	if !accessible {
		t.Error("expected accessible=true")
	}
}

func TestCov_UpdateCache(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "cache.json")

	pf := NewWorkerPreflight(dir, "root", ".")
	pf.updateCache(cachePath, "remote:https://github.com/org/repo", true)

	// Verify the cache was written
	data, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("cache file not found: %v", err)
	}

	var cache map[string]CacheEntry
	if err := json.Unmarshal(data, &cache); err != nil {
		t.Fatalf("failed to parse cache: %v", err)
	}

	entry, exists := cache["remote:https://github.com/org/repo"]
	if !exists {
		t.Fatal("expected cache entry")
	}
	if !entry.Accessible {
		t.Error("expected accessible=true")
	}
}

func TestCov_UpdateCache_OverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "cache.json")

	pf := NewWorkerPreflight(dir, "root", ".")

	// Write first entry
	pf.updateCache(cachePath, "key1", true)
	// Write second entry
	pf.updateCache(cachePath, "key2", false)

	data, _ := os.ReadFile(cachePath)
	var cache map[string]CacheEntry
	json.Unmarshal(data, &cache)

	if len(cache) != 2 {
		t.Errorf("expected 2 entries, got %d", len(cache))
	}
	if !cache["key1"].Accessible {
		t.Error("expected key1 accessible=true")
	}
	if cache["key2"].Accessible {
		t.Error("expected key2 accessible=false")
	}
}
