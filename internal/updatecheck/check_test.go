package updatecheck

import (
	"testing"
	"time"
)

func TestIsUnknownVersion(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{"", true},
		{"dev", true},
		{"DEV", true},
		{"v1.0.0", false},
		{"1.0.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			if got := isUnknownVersion(tt.version); got != tt.want {
				t.Errorf("isUnknownVersion(%q) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}

func TestParseSemver(t *testing.T) {
	tests := []struct {
		version string
		want    []int
		ok      bool
	}{
		{"1.0.0", []int{1, 0, 0}, true},
		{"v1.2.3", []int{1, 2, 3}, true},
		{"2.0", []int{2, 0}, true},
		{"1.0.0-beta", []int{1, 0, 0}, true},
		{"", nil, false},
		{"invalid", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			got, ok := parseSemver(tt.version)
			if ok != tt.ok {
				t.Errorf("parseSemver(%q) ok = %v, want %v", tt.version, ok, tt.ok)
				return
			}
			if ok && !equalSlice(got, tt.want) {
				t.Errorf("parseSemver(%q) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		a, b string
		want int
		ok   bool
	}{
		{"1.0.0", "1.0.0", 0, true},
		{"1.0.0", "1.0.1", -1, true},
		{"1.0.1", "1.0.0", 1, true},
		{"1.0.0", "2.0.0", -1, true},
		{"v1.0.0", "v1.0.1", -1, true},
		{"invalid", "1.0.0", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			got, ok := compareSemver(tt.a, tt.b)
			if ok != tt.ok {
				t.Errorf("compareSemver(%q, %q) ok = %v, want %v", tt.a, tt.b, ok, tt.ok)
				return
			}
			if ok && got != tt.want {
				t.Errorf("compareSemver(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestCheckWithUnknownVersion(t *testing.T) {
	result := Check("dev", Options{NoCache: true, Timeout: 1 * time.Millisecond})

	if !result.CurrentUnknown {
		t.Error("CurrentUnknown should be true for 'dev' version")
	}
	if result.Current != "dev" {
		t.Errorf("Current = %q, want 'dev'", result.Current)
	}
}

func TestFinalize(t *testing.T) {
	tests := []struct {
		name       string
		res        Result
		wantUpdate bool
	}{
		{
			name:       "newer available",
			res:        Result{Current: "1.0.0", Latest: "1.0.1"},
			wantUpdate: true,
		},
		{
			name:       "same version",
			res:        Result{Current: "1.0.0", Latest: "1.0.0"},
			wantUpdate: false,
		},
		{
			name:       "current is newer",
			res:        Result{Current: "2.0.0", Latest: "1.0.0"},
			wantUpdate: false,
		},
		{
			name:       "unknown current",
			res:        Result{Current: "dev", CurrentUnknown: true, Latest: "1.0.0"},
			wantUpdate: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := finalize(tt.res)
			if got.UpdateAvailable != tt.wantUpdate {
				t.Errorf("finalize() UpdateAvailable = %v, want %v", got.UpdateAvailable, tt.wantUpdate)
			}
		})
	}
}

func equalSlice(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
