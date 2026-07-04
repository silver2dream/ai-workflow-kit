//go:build !windows

package session

import "testing"

// ---------------------------------------------------------------------------
// itoa (pid_check_unix.go) — only compiled on non-windows
// ---------------------------------------------------------------------------

func TestItoa_Zero(t *testing.T) {
	if itoa(0) != "0" {
		t.Errorf("itoa(0) = %q, want '0'", itoa(0))
	}
}

func TestItoa_Positive(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{1, "1"},
		{42, "42"},
		{12345, "12345"},
	}
	for _, tc := range cases {
		got := itoa(tc.n)
		if got != tc.want {
			t.Errorf("itoa(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestItoa_Negative(t *testing.T) {
	got := itoa(-7)
	if got != "-7" {
		t.Errorf("itoa(-7) = %q, want '-7'", got)
	}
}
