package guard

import (
	"path"
	"strings"
)

// A command an agent asks to run is read here the way a shell would split
// it, closely enough to see every program it starts: commands chained with
// ;, &&, ||, | and &, on several lines, inside $(…), `…` and <(…), and behind
// wrappers that run what follows them (env, sudo, xargs, sh -c, cmd /c,
// powershell -Command). Reading less than a shell would is the safe side:
// what cannot be read is refused (see CheckTool).

// command is one simple command: the program and its arguments.
type command struct {
	program string // lower case, without directory or .exe
	args    []string
	raw     []string // the words as written, the program first
}

// splitCommands returns every simple command a command line runs. ok is false
// when the line cannot be read (an open quote, a nesting deeper than a shell
// script would use), which the caller refuses.
func splitCommands(line string) (cmds []command, ok bool) {
	return splitDepth(line, 0)
}

const maxNesting = 6

func splitDepth(line string, depth int) ([]command, bool) {
	if depth > maxNesting {
		return nil, false
	}
	segments, inner, ok := segment(line)
	if !ok {
		return nil, false
	}
	var cmds []command
	for _, s := range inner {
		more, ok := splitDepth(s, depth+1)
		if !ok {
			return nil, false
		}
		cmds = append(cmds, more...)
	}
	for _, s := range segments {
		words, ok := words(s)
		if !ok {
			return nil, false
		}
		more, ok := unwrap(words, depth)
		if !ok {
			return nil, false
		}
		cmds = append(cmds, more...)
	}
	return cmds, true
}

// segment cuts a line at the operators that end a simple command, outside
// quotes, and lifts out what $(…), `…` and <(…) / >(…) run.
func segment(line string) (segments, inner []string, ok bool) {
	var cur strings.Builder
	flush := func() {
		if s := strings.TrimSpace(cur.String()); s != "" {
			segments = append(segments, s)
		}
		cur.Reset()
	}
	runes := []rune(line)
	var quote rune
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case quote == '\'':
			cur.WriteRune(r)
			if r == '\'' {
				quote = 0
			}
			continue
		case r == '\\' && quote != '\'' && i+1 < len(runes):
			cur.WriteRune(r)
			cur.WriteRune(runes[i+1])
			i++
			continue
		case r == '"':
			if quote == '"' {
				quote = 0
			} else if quote == 0 {
				quote = '"'
			}
			cur.WriteRune(r)
			continue
		case r == '\'' && quote == 0:
			quote = '\''
			cur.WriteRune(r)
			continue
		}
		// $( … ), <( … ), >( … ): what runs inside is a command line of its
		// own, also inside double quotes.
		if (r == '$' || ((r == '<' || r == '>') && quote == 0)) && i+1 < len(runes) && runes[i+1] == '(' {
			end, found := closing(runes, i+1)
			if !found {
				return nil, nil, false
			}
			inner = append(inner, string(runes[i+2:end]))
			cur.WriteString(" _ ")
			i = end
			continue
		}
		if r == '`' {
			end := -1
			for j := i + 1; j < len(runes); j++ {
				if runes[j] == '`' && runes[j-1] != '\\' {
					end = j
					break
				}
			}
			if end < 0 {
				return nil, nil, false
			}
			inner = append(inner, string(runes[i+1:end]))
			cur.WriteString(" _ ")
			i = end
			continue
		}
		if quote == '"' {
			cur.WriteRune(r)
			continue
		}
		switch r {
		case ';', '|', '&', '\n', '\r':
			flush()
		case '(', ')', '{', '}':
			// Grouping: what is inside runs as well.
			cur.WriteRune(' ')
		default:
			cur.WriteRune(r)
		}
	}
	if quote != 0 {
		return nil, nil, false
	}
	flush()
	return segments, inner, true
}

// closing finds the parenthesis that closes the one at open, outside quotes.
func closing(runes []rune, open int) (int, bool) {
	depth := 0
	var quote rune
	for i := open; i < len(runes); i++ {
		r := runes[i]
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"':
			quote = r
		case r == '(':
			depth++
		case r == ')':
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}

// words splits a simple command into its words, taking quotes away.
func words(s string) ([]string, bool) {
	var out []string
	var cur strings.Builder
	in := false
	var quote rune
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else if r == '\\' && quote == '"' && i+1 < len(runes) {
				i++
				cur.WriteRune(runes[i])
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, in = r, true
		case r == '\\' && i+1 < len(runes):
			i++
			cur.WriteRune(runes[i])
			in = true
		case r == ' ' || r == '\t':
			if in {
				out = append(out, cur.String())
				cur.Reset()
				in = false
			}
		default:
			cur.WriteRune(r)
			in = true
		}
	}
	if quote != 0 {
		return nil, false
	}
	if in {
		out = append(out, cur.String())
	}
	return out, true
}

// programName is the program a word names: without its directory, its .exe
// and its case.
func programName(word string) string {
	word = strings.ReplaceAll(word, "\\", "/")
	name := strings.ToLower(path.Base(word))
	for _, ext := range []string{".exe", ".cmd", ".bat", ".ps1"} {
		name = strings.TrimSuffix(name, ext)
	}
	return name
}

// assignment is a word that sets a variable for the command after it.
func assignment(word string) bool {
	eq := strings.IndexByte(word, '=')
	if eq <= 0 {
		return false
	}
	for _, r := range word[:eq] {
		if !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

// unwrap turns the words of a simple command into the commands it runs: the
// command itself, and what a wrapper in front of it runs.
func unwrap(ws []string, depth int) ([]command, bool) {
	for len(ws) > 0 && assignment(ws[0]) {
		ws = ws[1:]
	}
	if len(ws) == 0 {
		return nil, true
	}
	c := command{program: programName(ws[0]), args: ws[1:], raw: ws}
	cmds := []command{c}
	switch c.program {
	case "sudo", "nohup", "time", "nice", "command", "exec", "builtin", "stdbuf", "timeout", "doas":
		rest := skipFlags(c.args)
		if c.program == "timeout" && len(rest) > 0 {
			rest = rest[1:]
		}
		more, ok := unwrap(rest, depth)
		return append(cmds, more...), ok
	case "env":
		rest := skipFlags(c.args)
		for len(rest) > 0 && assignment(rest[0]) {
			rest = rest[1:]
		}
		if len(rest) == 0 {
			return cmds, true
		}
		more, ok := unwrap(rest, depth)
		return append(cmds, more...), ok
	case "xargs":
		more, ok := unwrap(skipFlags(c.args), depth)
		return append(cmds, more...), ok
	case "sh", "bash", "zsh", "dash", "ksh", "fish":
		for i, a := range c.args {
			if (a == "-c" || strings.HasPrefix(a, "-") && strings.Contains(a, "c") && !strings.HasPrefix(a, "--")) && i+1 < len(c.args) {
				more, ok := splitDepth(c.args[i+1], depth+1)
				return append(cmds, more...), ok
			}
		}
	case "cmd":
		for i, a := range c.args {
			if strings.EqualFold(a, "/c") || strings.EqualFold(a, "/k") {
				more, ok := splitDepth(strings.Join(c.args[i+1:], " "), depth+1)
				return append(cmds, more...), ok
			}
		}
	case "powershell", "pwsh":
		for i, a := range c.args {
			la := strings.ToLower(a)
			if la == "-command" || la == "-c" {
				more, ok := splitDepth(strings.Join(c.args[i+1:], " "), depth+1)
				return append(cmds, more...), ok
			}
			if la == "-encodedcommand" || la == "-enc" || la == "-e" {
				// What it runs cannot be read here.
				return nil, false
			}
		}
	}
	return cmds, true
}

// skipFlags drops the options in front of the command a wrapper runs.
func skipFlags(args []string) []string {
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		args = args[1:]
	}
	return args
}
