package updatecheck

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const DefaultRepo = "silver2dream/ai-workflow-kit"

type Options struct {
	Repo     string
	NoCache  bool
	CacheTTL time.Duration
	Timeout  time.Duration
	Now      func() time.Time
}

type Result struct {
	Current         string    `json:"current"`
	Latest          string    `json:"latest"`
	UpdateAvailable bool      `json:"update_available"`
	CurrentUnknown  bool      `json:"current_unknown"`
	Source          string    `json:"source"`
	CheckedAt       time.Time `json:"checked_at"`
	Error           string    `json:"error,omitempty"`
}

type cacheEntry struct {
	Latest    string    `json:"latest"`
	CheckedAt time.Time `json:"checked_at"`
}

func Check(current string, opts Options) Result {
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	if opts.Repo == "" {
		opts.Repo = DefaultRepo
	}
	if opts.CacheTTL == 0 {
		opts.CacheTTL = 24 * time.Hour
	}
	if opts.Timeout == 0 {
		opts.Timeout = 5 * time.Second
	}

	res := Result{
		Current:        current,
		CurrentUnknown: isUnknownVersion(current),
		CheckedAt:      now(),
	}

	if !opts.NoCache {
		if entry, ok := readCache(opts.CacheTTL, now()); ok {
			res.Latest = entry.Latest
			res.Source = "cache"
			res.CheckedAt = entry.CheckedAt
			return finalize(res)
		}
	}

	latest, err := fetchLatestRelease(opts.Repo, opts.Timeout)
	if err != nil {
		res.Error = err.Error()
		return finalize(res)
	}
	res.Latest = latest
	res.Source = "network"
	writeCache(cacheEntry{Latest: latest, CheckedAt: now()})
	return finalize(res)
}

func finalize(res Result) Result {
	if res.CurrentUnknown {
		return res
	}
	if res.Latest == "" {
		return res
	}
	cmp, ok := compareSemver(res.Current, res.Latest)
	if !ok {
		if res.Error == "" {
			res.Error = "cannot compare versions"
		}
		return res
	}
	res.UpdateAvailable = cmp < 0
	return res
}

func readCache(ttl time.Duration, now time.Time) (cacheEntry, bool) {
	path, err := cachePath()
	if err != nil {
		return cacheEntry{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cacheEntry{}, false
	}
	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return cacheEntry{}, false
	}
	if entry.CheckedAt.IsZero() {
		return cacheEntry{}, false
	}
	if now.Sub(entry.CheckedAt) > ttl {
		return cacheEntry{}, false
	}
	if entry.Latest == "" {
		return cacheEntry{}, false
	}
	return entry, true
}

func writeCache(entry cacheEntry) {
	path, err := cachePath()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o644)
}

func cachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil || dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".cache")
	}
	return filepath.Join(dir, "awkit", "update.json"), nil
}

func fetchLatestRelease(repo string, timeout time.Duration) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errors.New("unexpected response from GitHub API")
	}
	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if payload.TagName == "" {
		return "", errors.New("missing tag_name from GitHub API")
	}
	return payload.TagName, nil
}

func isUnknownVersion(version string) bool {
	v := strings.TrimSpace(strings.ToLower(version))
	return v == "" || v == "dev"
}

func compareSemver(a, b string) (int, bool) {
	aa, ok := parseSemver(a)
	if !ok {
		return 0, false
	}
	bb, ok := parseSemver(b)
	if !ok {
		return 0, false
	}
	maxLen := len(aa)
	if len(bb) > maxLen {
		maxLen = len(bb)
	}
	for i := 0; i < maxLen; i++ {
		av := 0
		bv := 0
		if i < len(aa) {
			av = aa[i]
		}
		if i < len(bb) {
			bv = bb[i]
		}
		if av < bv {
			return -1, true
		}
		if av > bv {
			return 1, true
		}
	}
	return 0, true
}

func parseSemver(version string) ([]int, bool) {
	v := strings.TrimSpace(version)
	v = strings.TrimPrefix(v, "v")
	v = strings.SplitN(v, "-", 2)[0]
	if v == "" {
		return nil, false
	}
	parts := strings.Split(v, ".")
	nums := make([]int, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			return nil, false
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, false
		}
		nums = append(nums, n)
	}
	return nums, true
}
