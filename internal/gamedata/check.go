package gamedata

import (
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"
)

// Finding is one place where the game data is not as it has to be.
type Finding struct {
	// File is the file, from the project's root.
	File string
	// Line is the line in it; 0 for the file as a whole.
	Line int
	// Key is the text key or the field the finding is about, when there is
	// one.
	Key string
	// Message says what is wrong and what to do about it.
	Message string
}

func (f Finding) String() string {
	at := f.File
	if f.Line > 0 {
		at = fmt.Sprintf("%s:%d", f.File, f.Line)
	}
	return at + ": " + f.Message
}

// MissingKey is the start of the message of a finding that a table holds a
// text key the text tables do not have. Such a finding is put right in the
// text tables, by whoever writes the text, or in the table.
const MissingKey = "the text tables have no key "

// IsMissingKey reports whether a finding is a text key the text tables do
// not have.
func (f Finding) IsMissingKey() bool {
	return strings.Contains(f.Message, MissingKey)
}

// Check reads the game data of a project and lists what is not as it has to
// be, ordered by file and line.
func Check(root fs.FS, layout *Layout) (*Data, []Finding, error) {
	d, err := Load(root, layout)
	if err != nil {
		return nil, nil, err
	}
	findings := append([]Finding(nil), d.problems...)
	findings = append(findings, d.checkTables(root)...)
	findings = append(findings, d.checkGroups()...)
	findings = append(findings, d.checkText()...)
	findings = append(findings, d.checkTextKeys()...)
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		return findings[i].Line < findings[j].Line
	})
	return d, findings, nil
}

// checkTables checks each table against its schema: what Luban does not.
// The columns are the bean's fields; a number or a truth value is never left
// blank, because Luban reads a blank as 0 or false: a field that may have no
// value is written null.
func (d *Data) checkTables(root fs.FS) []Finding {
	var findings []Finding
	add := func(file string, line int, key, format string, args ...any) {
		findings = append(findings, Finding{File: file, Line: line, Key: key, Message: fmt.Sprintf(format, args...)})
	}
	names := map[string]string{}
	for _, t := range d.Tables {
		if earlier, ok := names[t.Name]; ok {
			add(t.File, 0, t.Name, "defines the table %s, and so does %s", t.Name, earlier)
		}
		names[t.Name] = t.File
		if t.Bean == nil {
			add(t.File, 0, t.Name, "the table %s has the value %q, which the schema does not define as a bean", t.Name, t.Value)
			continue
		}
		if len(t.Inputs) == 0 {
			add(t.File, 0, t.Name, "the table %s reads nothing: name its data file in input", t.Name)
		}
		for _, input := range t.Inputs {
			if _, err := fs.Stat(root, input); err != nil {
				add(t.File, 0, t.Name, "the table %s reads %s, which is not there", t.Name, input)
			}
		}
		for _, sheet := range t.Sheets {
			findings = append(findings, checkSheet(t, sheet)...)
		}
	}
	return findings
}

func checkSheet(t *Table, sheet *Sheet) []Finding {
	var findings []Finding
	add := func(line int, key, format string, args ...any) {
		findings = append(findings, Finding{File: sheet.File, Line: line, Key: key, Message: fmt.Sprintf(format, args...)})
	}
	if !sheet.header {
		add(0, "", "has no ##var row: the first row is ##var and the names of the fields of %s, one per column after the first", t.Bean.Name)
		return findings
	}
	fields := map[string]bool{}
	for _, field := range sheet.Fields {
		fields[field] = true
	}
	var extra, missing []string
	for field := range fields {
		if _, ok := t.Bean.Var(field); !ok {
			extra = append(extra, field)
		}
	}
	for _, v := range t.Bean.Vars {
		if !fields[v.Name] {
			missing = append(missing, v.Name)
		}
	}
	sort.Strings(extra)
	if len(extra)+len(missing) > 0 {
		var parts []string
		if len(extra) > 0 {
			parts = append(parts, "it has "+strings.Join(extra, ", ")+", which "+t.Bean.Name+" does not")
		}
		if len(missing) > 0 {
			parts = append(parts, "it lacks "+strings.Join(missing, ", "))
		}
		add(0, "", "the columns are not the fields of %s (%s, in %s): %s", t.Bean.Name, t.Name, t.File, strings.Join(parts, "; "))
		return findings
	}
	for _, row := range sheet.Rows {
		for i, field := range sheet.Fields {
			v, _ := t.Bean.Var(field)
			if strings.TrimSpace(row.Cells[i]) == "" && !blankOK(v.Type) {
				add(row.Line, field, "%s is blank, and Luban reads a blank %s as 0 or false: write the value, or null where it may have none", field, baseType(v.Type))
			}
		}
	}
	return findings
}

// baseType is a field's type without what follows a # (#range=..., #sep=...).
func baseType(t string) string {
	if at := strings.Index(t, "#"); at >= 0 {
		t = t[:at]
	}
	return strings.TrimSpace(t)
}

// blankOK reports whether a field of a type can be left blank: a string (the
// empty string) and a container (an empty one) can; a number, a truth value
// and a record cannot.
func blankOK(t string) bool {
	base := baseType(t)
	if base == "string" {
		return true
	}
	kind := strings.TrimSpace(strings.SplitN(strings.TrimLeft(base, "("), ",", 2)[0])
	switch kind {
	case "list", "array", "set", "map":
		return true
	}
	return false
}

func isContainer(t string) bool {
	return baseType(t) != "string" && blankOK(t)
}

// probability names a field whose value is an odds or a weight: the client
// must not have it.
var probability = regexp.MustCompile(`(^|_)(weight|odds|prob|chance|w|p)$`)

// groupSet reads a group: nil when none is marked, empty when it names a
// group luban.conf does not have.
func (d *Data) groupSet(value string) map[string]bool {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	known := d.Layout.Groups
	if len(known) == 0 {
		known = []string{"c", "s"}
	}
	set := map[string]bool{}
	for word := range strings.SplitSeq(value, ",") {
		word = strings.TrimSpace(word)
		if !contains(known, word) {
			return map[string]bool{}
		}
		set[word] = true
	}
	return set
}

// checkGroups checks that every table says which side has it, and that a
// table the client has says it of every field: what ships with the client is
// public. Odds and weights are the server's only.
func (d *Data) checkGroups() []Finding {
	var findings []Finding
	groups := strings.Join(d.groupNames(), ", ")
	for _, t := range d.Tables {
		tgroup := d.groupSet(t.Group)
		switch {
		case tgroup == nil:
			findings = append(findings, Finding{File: t.File, Key: t.Name, Message: fmt.Sprintf(
				"the table %s marks no group: mark it, s for a table only the server reads, c,s for one the client has as well", t.Name)})
			continue
		case len(tgroup) == 0:
			findings = append(findings, Finding{File: t.File, Key: t.Name, Message: fmt.Sprintf(
				"the table %s has the group %q; a group is one of %s, or more of them with commas", t.Name, t.Group, groups)})
			continue
		case t.Bean == nil:
			continue
		}
		for _, v := range t.Bean.Vars {
			vgroup := d.groupSet(v.Group)
			label := t.Bean.Name + "." + v.Name
			var message string
			switch {
			case vgroup != nil && len(vgroup) == 0:
				message = fmt.Sprintf("%s has the group %q; a group is one of %s, or more of them with commas", label, v.Group, groups)
			case tgroup["c"] && vgroup == nil:
				message = fmt.Sprintf("%s marks no group, and the table %s ships with the client: mark every field of it", label, t.Name)
			case tgroup["c"] && probability.MatchString(v.Name) && !(len(vgroup) == 1 && vgroup["s"]):
				message = fmt.Sprintf("%s is an odds or a weight, and those are the server's only (s): what ships with the client is public", label)
			case !tgroup["c"] && vgroup["c"]:
				message = fmt.Sprintf("%s is marked c, and the table %s is the server's only", label, t.Name)
			default:
				continue
			}
			findings = append(findings, Finding{File: t.Bean.File, Key: label, Message: message})
		}
	}
	return findings
}

func (d *Data) groupNames() []string {
	if len(d.Layout.Groups) > 0 {
		return d.Layout.Groups
	}
	return []string{"c", "s"}
}

// IsTextKey reports whether a field holds a text key: its comment says so
// (文字鍵, or text key).
func IsTextKey(v Var) bool {
	comment := strings.ToLower(v.Comment)
	return strings.Contains(comment, "文字鍵") || strings.Contains(comment, "text key")
}

// checkTextKeys checks that every text key a table holds is in the text
// tables: one that is not shows nothing on the screen.
func (d *Data) checkTextKeys() []Finding {
	keys := d.Keys()
	var findings []Finding
	for _, t := range d.Tables {
		if t.Bean == nil {
			continue
		}
		for _, v := range t.Bean.Vars {
			if !IsTextKey(v) || isContainer(v.Type) {
				continue
			}
			for _, sheet := range t.Sheets {
				if !contains(sheet.Fields, v.Name) {
					continue
				}
				for _, row := range sheet.Rows {
					key := strings.TrimSpace(sheet.Cell(row, v.Name))
					switch {
					case key == "null" && strings.HasSuffix(baseType(v.Type), "?"):
					case key == "":
						findings = append(findings, Finding{File: sheet.File, Line: row.Line, Key: v.Name,
							Message: fmt.Sprintf("%s holds no text key; it holds the key of the text the player reads", v.Name)})
					default:
						if _, ok := keys[key]; !ok {
							findings = append(findings, Finding{File: sheet.File, Line: row.Line, Key: key,
								Message: fmt.Sprintf("%s holds %s, and %s%s: write its text in %s", v.Name, key, MissingKey, key, d.Layout.Text())})
						}
					}
				}
			}
		}
	}
	return findings
}

var (
	// textKey is a key of the text tables: ui.title.start, err.auth.expired.
	textKey = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z0-9_]+)+$`)
	// placeholder is what the game fills in: {name}, {icon:fire}.
	placeholder = regexp.MustCompile(`\{[^{}\r\n]+\}`)
	// tag is markup the game reads: [em], [/em], [kw=fire].
	tag = regexp.MustCompile(`\[(/?)([a-z][a-z0-9_]*)(=[^\]\s]*)?\]`)
	// forbidden is what text does not hold: a color, a font, the path of a
	// file. The game's theme decides those.
	forbidden = regexp.MustCompile(`res://|user://|#[0-9a-fA-F]{6}\b|\.(png|jpg|jpeg|webp|svg|ttf|otf|tres|tscn)\b`)
)

// checkText checks the text tables: each key once and well formed, the
// source text written, each translation there, up to date with the source,
// within its limit and with the same placeholders and markup.
func (d *Data) checkText() []Finding {
	var findings []Finding
	seen := map[string]TextRow{}
	for _, text := range d.Texts {
		source := text.Source()
		for _, row := range text.Rows {
			add := func(format string, args ...any) {
				findings = append(findings, Finding{File: row.File, Line: row.Line, Key: row.Key, Message: fmt.Sprintf(format, args...)})
			}
			if !textKey.MatchString(row.Key) {
				add("the key %q is not a key: lower case, in parts joined by dots, such as ui.title.start", row.Key)
			}
			if first, ok := seen[row.Key]; ok {
				add("the key %s is in %s:%d as well; a key is in the text tables once", row.Key, first.File, first.Line)
			} else {
				seen[row.Key] = row
			}
			original := row.Text[source]
			if strings.TrimSpace(original) == "" {
				add("%s has no %s text; %s is the language the text is written in, and the others are translated from it", row.Key, source, source)
				continue
			}
			checkMarkup(source, original, add)
			for _, language := range text.Languages[1:] {
				translation := row.Text[language]
				translated := contains(text.Translated, language)
				if strings.TrimSpace(translation) == "" {
					if translated {
						add("%s has no %s text", row.Key, language)
					}
					continue
				}
				checkMarkup(language, translation, add)
				if !translated {
					continue
				}
				if row.Basis[language] != original {
					add("the %s of %s is out of date: its basis is not the %s text as it stands. Look at the %s again, then set its basis to the %s text",
						language, row.Key, source, language, source)
				}
				if n := utf8.RuneCountInString(translation); row.Limit > 0 && n > row.Limit {
					add("the %s of %s is %d characters long, over its limit of %d", language, row.Key, n, row.Limit)
				}
				if a, b := sortedMatches(placeholder, original), sortedMatches(placeholder, translation); !equal(a, b) {
					add("the %s and %s of %s fill in different things: %s against %s", source, language, row.Key, listed(a), listed(b))
				}
				if a, b := sortedMatches(tag, original), sortedMatches(tag, translation); !equal(a, b) {
					add("the %s and %s of %s have different markup: %s against %s", source, language, row.Key, listed(a), listed(b))
				}
			}
		}
	}
	return findings
}

// checkMarkup checks the text of one language: no color, font or path in it,
// and markup that is closed is closed in the order it is opened.
func checkMarkup(language, text string, add func(format string, args ...any)) {
	if found := forbidden.FindString(text); found != "" {
		add("the %s text holds %q: text holds no color, font or path of a file; the game's theme decides those", language, found)
	}
	closed := map[string]bool{}
	for _, m := range tag.FindAllStringSubmatch(text, -1) {
		if m[1] == "/" {
			closed[m[2]] = true
		}
	}
	var open []string
	for _, m := range tag.FindAllStringSubmatch(text, -1) {
		name := m[2]
		switch {
		case !closed[name]:
			// markup that stands alone, such as [br]
		case m[1] == "":
			open = append(open, name)
		case len(open) == 0 || open[len(open)-1] != name:
			add("the %s text closes [%s] where it is not open", language, name)
			return
		default:
			open = open[:len(open)-1]
		}
	}
	if len(open) > 0 {
		add("the %s text opens [%s] and does not close it", language, open[len(open)-1])
	}
}

func sortedMatches(re *regexp.Regexp, text string) []string {
	found := re.FindAllString(text, -1)
	sort.Strings(found)
	return found
}

func equal(a, b []string) bool {
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

func listed(items []string) string {
	if len(items) == 0 {
		return "nothing"
	}
	return strings.Join(items, " ")
}

func contains(items []string, item string) bool {
	return slices.Contains(items, item)
}
