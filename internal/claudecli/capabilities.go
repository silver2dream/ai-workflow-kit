package claudecli

import (
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Supports reports whether the claude CLI declares an option in its help.
func Supports(option string) bool { return hasOption(helpTextFunc(), option) }

// Mentions reports whether the claude CLI's help names a word anywhere, as
// it names the values an option takes ("dontAsk").
func Mentions(word string) bool { return strings.Contains(helpTextFunc(), word) }

var (
	versionOnce sync.Once
	versionText string
)

// versionTextFunc returns the output of `claude --version`, fetched once per
// process. A variable, so tests can supply one.
var versionTextFunc = func() string {
	versionOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if out, err := exec.CommandContext(ctx, "claude", "--version").CombinedOutput(); err == nil {
			versionText = string(out)
		}
	})
	return versionText
}

var versionNumber = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

// AtLeast reports whether the claude CLI is this version or newer; false
// when its version cannot be told.
func AtLeast(major, minor, patch int) bool {
	m := versionNumber.FindStringSubmatch(versionTextFunc())
	if m == nil {
		return false
	}
	have := [3]int{}
	for i := range have {
		have[i], _ = strconv.Atoi(m[i+1])
	}
	want := [3]int{major, minor, patch}
	for i := range have {
		if have[i] != want[i] {
			return have[i] > want[i]
		}
	}
	return true
}
