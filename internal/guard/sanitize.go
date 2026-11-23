package guard

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// MaxPieceChars bounds one piece of context: what is longer is cut, and
// says so.
const MaxPieceChars = 12000

// hidden reports whether a character is one a person reading the text cannot
// see but a model reads: zero-width characters, the controls that reorder
// text (Trojan Source), and other format and control characters. Line ends
// and tabs are kept.
func hidden(r rune) bool {
	switch r {
	case '\n', '\t', '\r':
		return false
	case 0x200B, 0x200C, 0x200D, 0x2060, 0xFEFF, 0x180E, // zero-width
		0x202A, 0x202B, 0x202C, 0x202D, 0x202E, // embeddings and overrides
		0x2066, 0x2067, 0x2068, 0x2069, 0x200E, 0x200F: // isolates and marks
		return true
	}
	// Unicode tag characters spell out text no one sees.
	if r >= 0xE0000 && r <= 0xE007F {
		return true
	}
	return unicode.IsControl(r) || unicode.Is(unicode.Cf, r)
}

var htmlComment = regexp.MustCompile(`(?s)<!--.*?-->`)

// Sanitize takes out of a text what a person reading it cannot see: hidden
// characters and HTML comments (which GitHub does not show). It cuts what is
// longer than max characters. It returns the text and what it changed, one
// line for each kind of change.
func Sanitize(text string, max int) (string, []string) {
	var changes []string
	var b strings.Builder
	removed := 0
	for _, r := range text {
		if hidden(r) {
			removed++
			continue
		}
		b.WriteRune(r)
	}
	out := b.String()
	if removed > 0 {
		changes = append(changes, plural(removed, "hidden character")+" removed")
	}
	if comments := htmlComment.FindAllString(out, -1); len(comments) > 0 {
		out = htmlComment.ReplaceAllString(out, "")
		changes = append(changes, plural(len(comments), "HTML comment")+" removed")
	}
	if max > 0 {
		if runes := []rune(out); len(runes) > max {
			out = string(runes[:max]) + "\n[cut: the rest was longer than this piece may be]"
			changes = append(changes, "cut to its first "+strconv.Itoa(max)+" characters")
		}
	}
	return out, changes
}

func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return strconv.Itoa(n) + " " + what + "s"
}
