package guard

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// What awkit writes to GitHub is read there by people and by agents, and by
// anyone when the repository is public. A secret or a person's data in it is
// out, whoever wrote the text: an agent's summary, a review, a ticket. So
// every text a gh call carries is scanned first, and a call that would carry
// one is not made. Only what is plainly a credential or a person's number is
// looked for: the name of a variable in a sentence about it is not one, and a
// number is one only when its check digit says so.

// OutboundVersion names the patterns below in every verdict they give.
const OutboundVersion = "outbound/1"

// ErrBlocked is what a refused call's error wraps: the caller tells a refusal
// from a failure with errors.Is, and does not try again with the same text.
var ErrBlocked = errors.New("AWK guard")

type outboundPattern struct {
	id      string
	kind    string // "a credential" or "personal data"
	pattern *regexp.Regexp
	valid   func(match string) bool // nil: every match counts
}

var outboundPatterns = []outboundPattern{
	{"github-token", "a credential", regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,}\b`), nil},
	{"github-pat", "a credential", regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{60,}`), nil},
	{"anthropic-key", "a credential", regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_-]{20,}`), nil},
	{"openai-key", "a credential", regexp.MustCompile(`\bsk-(proj-|svcacct-|admin-)?[A-Za-z0-9_-]{32,}`), nil},
	{"aws-access-key", "a credential", regexp.MustCompile(`\b(AKIA|ASIA)[0-9A-Z]{16}\b`), nil},
	{"private-key", "a credential", regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----`), nil},
	{"slack-token", "a credential", regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}`), nil},
	{"google-api-key", "a credential", regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`), nil},
	{"jwt", "a credential", regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`), nil},
	{"card-number", "personal data", regexp.MustCompile(`\b(?:\d[ -]?){12,18}\d\b`), cardNumber},
	{"tw-national-id", "personal data", regexp.MustCompile(`\b[A-Z][12]\d{8}\b`), twNationalID},
}

// ScanOutbound looks for credentials and personal data in a text about to be
// written to GitHub. Each one found blocks; one is named once, by the first
// pattern that knows it (an Anthropic key is no OpenAI key too).
func ScanOutbound(text, source string) []Verdict {
	var verdicts []Verdict
	var taken [][]int
	for _, p := range outboundPatterns {
		for _, span := range p.pattern.FindAllStringIndex(text, -1) {
			match := text[span[0]:span[1]]
			if overlaps(taken, span) || (p.valid != nil && !p.valid(match)) {
				continue
			}
			taken = append(taken, span)
			verdicts = append(verdicts, Verdict{
				Result: Block, Class: Security, Stage: StageOutput,
				Rule: OutboundVersion + "#" + p.id, Source: source,
				Reason:   "what is about to be written to GitHub holds what looks like " + p.kind + " (" + p.id + ")",
				Evidence: redact(match),
			})
			break
		}
	}
	return verdicts
}

// Redact masks the credentials and personal data of a text, each as
// [REDACTED:<kind>], before the text is given to a model: what a model is
// shown it may repeat. It returns the kinds it masked.
func Redact(text string) (string, []string) {
	type hit struct {
		span []int
		id   string
	}
	var hits []hit
	var taken [][]int
	for _, p := range outboundPatterns {
		for _, span := range p.pattern.FindAllStringIndex(text, -1) {
			if overlaps(taken, span) || (p.valid != nil && !p.valid(text[span[0]:span[1]])) {
				continue
			}
			taken = append(taken, span)
			hits = append(hits, hit{span, p.id})
		}
	}
	if len(hits) == 0 {
		return text, nil
	}
	sort.Slice(hits, func(a, b int) bool { return hits[a].span[0] < hits[b].span[0] })
	var b strings.Builder
	last := 0
	var kinds []string
	for _, h := range hits {
		b.WriteString(text[last:h.span[0]])
		b.WriteString("[REDACTED:" + h.id + "]")
		last = h.span[1]
		if !slices.Contains(kinds, h.id) {
			kinds = append(kinds, h.id)
		}
	}
	b.WriteString(text[last:])
	return b.String(), kinds
}

func overlaps(spans [][]int, span []int) bool {
	for _, s := range spans {
		if span[0] < s[1] && s[0] < span[1] {
			return true
		}
	}
	return false
}

// redact keeps enough of a match to tell which it was, not to use it.
func redact(secret string) string {
	r := []rune(secret)
	if len(r) <= 8 {
		return "[REDACTED]"
	}
	return string(r[:4]) + "…[REDACTED]"
}

// testCards are the card numbers payment providers publish for tests: a pull
// request about payments names them, and they belong to no one.
var testCards = map[string]bool{
	"4242424242424242": true, "4111111111111111": true, "4000056655665556": true,
	"4012888888881881": true, "5555555555554444": true, "5105105105105100": true,
	"2223003122003222": true, "378282246310005": true, "371449635398431": true,
	"6011111111111117": true, "6011000990139424": true, "3056930009020004": true,
	"36227206271667": true, "3566002020360505": true, "6200000000000005": true,
}

// cardNumber says whether digits are a payment card's: an issuer's prefix, a
// card's length and a Luhn check digit.
func cardNumber(match string) bool {
	digits := strings.NewReplacer(" ", "", "-", "").Replace(match)
	n := len(digits)
	if n < 13 || n > 19 || testCards[digits] {
		return false
	}
	switch {
	case digits[0] == '4',
		digits[0] == '5' && digits[1] >= '1' && digits[1] <= '5',
		digits[0] == '2' && digits[1] >= '2' && digits[1] <= '7',
		digits[0] == '3' && (digits[1] == '4' || digits[1] == '7'),
		strings.HasPrefix(digits, "6011"), strings.HasPrefix(digits, "65"),
		strings.HasPrefix(digits, "35"), strings.HasPrefix(digits, "62"):
	default:
		return false
	}
	sum := 0
	for i := 0; i < n; i++ {
		d := int(digits[n-1-i] - '0')
		if i%2 == 1 {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
	}
	return sum%10 == 0
}

// twLetters gives the number each first letter of a Taiwan national ID
// stands for.
var twLetters = map[byte]int{
	'A': 10, 'B': 11, 'C': 12, 'D': 13, 'E': 14, 'F': 15, 'G': 16, 'H': 17, 'I': 34,
	'J': 18, 'K': 19, 'L': 20, 'M': 21, 'N': 22, 'O': 35, 'P': 23, 'Q': 24, 'R': 25,
	'S': 26, 'T': 27, 'U': 28, 'V': 29, 'W': 32, 'X': 30, 'Y': 31, 'Z': 33,
}

// twNationalID says whether a match is a Taiwan national ID: its check digit
// is right, and it is not the one every form shows as an example.
func twNationalID(match string) bool {
	if len(match) != 10 || match == "A123456789" {
		return false
	}
	code, ok := twLetters[match[0]]
	if !ok {
		return false
	}
	sum := code/10 + (code%10)*9
	for i := 1; i <= 8; i++ {
		sum += int(match[i]-'0') * (9 - i)
	}
	sum += int(match[9] - '0')
	return sum%10 == 0
}

// ghTexts are the texts a gh call carries to GitHub: titles, bodies (also
// from a file), and the fields of gh api.
func ghTexts(args []string) []string {
	api := len(args) > 0 && args[0] == "api"
	var texts []string
	value := func(i int) string {
		if i+1 < len(args) {
			return args[i+1]
		}
		return ""
	}
	fromFile := func(path string) string {
		if path == "" || path == "-" {
			return ""
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return ""
		}
		return string(data)
	}
	field := func(kv string) {
		if eq := strings.IndexByte(kv, '='); eq >= 0 {
			v := kv[eq+1:]
			if strings.HasPrefix(v, "@") {
				v = fromFile(v[1:])
			}
			texts = append(texts, v)
		}
	}
	for i, a := range args {
		name, inline, hasInline := strings.Cut(a, "=")
		if !strings.HasPrefix(a, "-") {
			continue
		}
		get := func() string {
			if hasInline {
				return inline
			}
			return value(i)
		}
		switch {
		case name == "--body" || name == "-b" || name == "--title" || name == "-t" || name == "--message" || name == "-m":
			texts = append(texts, get())
		case name == "--body-file" || (!api && name == "-F"):
			texts = append(texts, fromFile(get()))
		case api && (name == "-f" || name == "-F" || name == "--field" || name == "--raw-field"):
			field(get())
		case api && name == "--input":
			texts = append(texts, fromFile(get()))
		}
	}
	return texts
}

// CheckGH scans what a gh call would write to GitHub, and returns what it
// found; any verdict blocks.
func CheckGH(args []string) []Verdict {
	var verdicts []Verdict
	source := "gh " + strings.Join(firstWords(args, 2), " ")
	for _, text := range ghTexts(args) {
		verdicts = append(verdicts, ScanOutbound(text, source)...)
	}
	return verdicts
}

func firstWords(args []string, n int) []string {
	var out []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") || len(out) == n {
			break
		}
		out = append(out, a)
	}
	return out
}

// Scope says, to a check deep in a call, whose work the call is: the project
// its verdicts are recorded in, and the issue they belong to.
type Scope struct {
	StateRoot string
	Issue     int
}

type scopeKey struct{}

// WithScope gives the calls made with ctx a scope; what it leaves empty is
// kept from the scope ctx already has.
func WithScope(ctx context.Context, s Scope) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	have := ScopeOf(ctx)
	if s.StateRoot == "" {
		s.StateRoot = have.StateRoot
	}
	if s.Issue == 0 {
		s.Issue = have.Issue
	}
	return context.WithValue(ctx, scopeKey{}, s)
}

// ScopeOf is the scope of ctx; empty when it has none.
func ScopeOf(ctx context.Context) Scope {
	if ctx == nil {
		return Scope{}
	}
	s, _ := ctx.Value(scopeKey{}).(Scope)
	return s
}

// GHGate refuses a gh call that would write a credential or personal data to
// GitHub, and records why: in the project and against the issue of ctx's
// scope, else of the current directory. It is what every gh call of awkit
// that carries text goes through before it is made; the error of a refusal
// wraps ErrBlocked.
func GHGate(ctx context.Context, args []string) error {
	verdicts := CheckGH(args)
	if !Blocks(verdicts) {
		return nil
	}
	scope := ScopeOf(ctx)
	if scope.StateRoot == "" {
		scope.StateRoot = WorkingProject()
	}
	if scope.Issue == 0 {
		if dir, err := os.Getwd(); err == nil {
			scope.Issue = IssueOfDir(dir)
		}
	}
	Record(scope.StateRoot, verdicts, scope.Issue)
	return fmt.Errorf("%w: %s; it was not sent (%s)", ErrBlocked, verdicts[0].Reason, verdicts[0].Rule)
}
