package gamedata

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// The review pages show the game data to a person: the copy as it reads, in
// each language side by side, and the tables with what each column holds.
// They are made from the files, never edited: the files are the truth.

// use is a place a text key is used: a field of a row of a table.
type use struct {
	table, field, file string
	line               int
}

func (u use) String() string {
	return fmt.Sprintf("%s.%s (%s:%d)", u.table, u.field, path.Base(u.file), u.line)
}

// uses lists, for each text key, the rows of the tables that hold it.
func (d *Data) uses() map[string][]use {
	found := map[string][]use{}
	for _, t := range d.Tables {
		if t.Bean == nil {
			continue
		}
		for _, v := range t.Bean.Vars {
			if !IsTextKey(v) || isContainer(v.Type) {
				continue
			}
			for _, sheet := range t.Sheets {
				for _, row := range sheet.Rows {
					if key := strings.TrimSpace(sheet.Cell(row, v.Name)); key != "" {
						found[key] = append(found[key], use{t.Name, v.Name, sheet.File, row.Line})
					}
				}
			}
		}
	}
	return found
}

// group is what a key belongs to, for the copy to be read by it: the table
// (grade.mortal.name: grade), or the screen (ui.title.start: ui.title).
func group(key string) string {
	parts := strings.Split(key, ".")
	if len(parts) >= 3 && parts[0] == "ui" {
		return parts[0] + "." + parts[1]
	}
	return parts[0]
}

// cell writes a value into a cell of a Markdown table.
func cell(text string) string {
	text = strings.ReplaceAll(strings.TrimSpace(text), "\r\n", "\n")
	text = strings.ReplaceAll(text, "|", `\|`)
	return strings.ReplaceAll(text, "\n", "<br>")
}

// findingsBy sorts findings by file, and by key or line within it.
func findingsBy(findings []Finding) map[string][]Finding {
	by := map[string][]Finding{}
	for _, f := range findings {
		by[f.File] = append(by[f.File], f)
	}
	return by
}

// CopyReview writes the text of some text tables for a person to read and
// review: the keys of each table grouped by what they belong to, each
// language side by side, where each key is used and what to look at. files
// are the text tables to show; all of them when it is empty. findings are
// what Check found.
func (d *Data) CopyReview(files []string, findings []Finding) string {
	uses := d.uses()
	by := findingsBy(findings)
	var b strings.Builder
	for _, text := range d.Texts {
		if len(files) > 0 && !contains(files, text.File) {
			continue
		}
		fmt.Fprintf(&b, "## %s\n\n", text.File)
		if text.Source() != "" {
			fmt.Fprintf(&b, "Written in %s", text.Source())
			if len(text.Translated) > 0 {
				fmt.Fprintf(&b, "; translated into %s", strings.Join(text.Translated, ", "))
			}
			b.WriteString(".\n\n")
		}
		notes := map[string][]string{}
		var loose []Finding
		for _, f := range by[text.File] {
			if f.Key != "" {
				notes[f.Key] = append(notes[f.Key], f.Message)
			} else {
				loose = append(loose, f)
			}
		}
		var groups []string
		rows := map[string][]TextRow{}
		for _, row := range text.Rows {
			g := group(row.Key)
			if _, ok := rows[g]; !ok {
				groups = append(groups, g)
			}
			rows[g] = append(rows[g], row)
		}
		for _, g := range groups {
			fmt.Fprintf(&b, "### %s\n\n", g)
			fmt.Fprintf(&b, "| Key | %s", strings.Join(text.Languages, " | "))
			if text.limit {
				b.WriteString(" | Limit")
			}
			b.WriteString(" | Used in | To look at |\n|---|")
			b.WriteString(strings.Repeat("---|", len(text.Languages)))
			if text.limit {
				b.WriteString("---|")
			}
			b.WriteString("---|---|\n")
			for _, row := range rows[g] {
				fmt.Fprintf(&b, "| `%s` |", cell(row.Key))
				for _, language := range text.Languages {
					fmt.Fprintf(&b, " %s |", cell(row.Text[language]))
				}
				if text.limit {
					limit := ""
					if row.Limit > 0 {
						limit = fmt.Sprint(row.Limit)
					}
					fmt.Fprintf(&b, " %s |", limit)
				}
				var used []string
				for _, u := range uses[row.Key] {
					used = append(used, u.String())
				}
				fmt.Fprintf(&b, " %s | %s |\n", cell(strings.Join(used, "\n")), cell(strings.Join(notes[row.Key], "\n")))
			}
			b.WriteString("\n")
		}
		if len(loose) > 0 {
			b.WriteString("To look at:\n")
			for _, f := range loose {
				fmt.Fprintf(&b, "- %s\n", f)
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

// DataReview writes some tables for a person to read and review: each with
// what it is, where it is defined and which side has it, then its rows, each
// column with what it holds in words and each text key with its text. files
// are the files to show: a table is shown when its data or its schema is
// among them; all tables when it is empty. findings are what Check found.
func (d *Data) DataReview(files []string, findings []Finding) string {
	by := findingsBy(findings)
	var b strings.Builder
	for _, t := range d.Tables {
		if !shown(t, files) || t.Bean == nil {
			continue
		}
		fmt.Fprintf(&b, "## %s\n\n", t.Name)
		if about := firstNonEmpty(t.Comment, t.Bean.Comment); about != "" {
			fmt.Fprintf(&b, "%s\n\n", cell(about))
		}
		var inputs []string
		for _, input := range t.Inputs {
			inputs = append(inputs, "`"+input+"`")
		}
		fmt.Fprintf(&b, "Schema `%s` (%s) · data %s · group %s", t.File, t.Bean.Name, strings.Join(inputs, ", "), firstNonEmpty(t.Group, "(none)"))
		if t.Mode != "" {
			fmt.Fprintf(&b, " · mode %s", t.Mode)
		}
		b.WriteString("\n\n")
		for _, sheet := range t.Sheets {
			if len(t.Sheets) > 1 {
				fmt.Fprintf(&b, "### %s\n\n", sheet.File)
			}
			d.writeSheet(&b, t, sheet)
		}
		var look []Finding
		look = append(look, by[t.File]...)
		if t.Bean.File != t.File {
			look = append(look, by[t.Bean.File]...)
		}
		for _, sheet := range t.Sheets {
			look = append(look, by[sheet.File]...)
		}
		if len(look) > 0 {
			b.WriteString("To look at:\n")
			for _, f := range look {
				fmt.Fprintf(&b, "- %s\n", f)
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

func shown(t *Table, files []string) bool {
	if len(files) == 0 || contains(files, t.File) || t.Bean != nil && contains(files, t.Bean.File) {
		return true
	}
	for _, input := range t.Inputs {
		if contains(files, input) {
			return true
		}
	}
	return false
}

// writeSheet writes the rows of a sheet: a table of one row field by field,
// down the page, and a table of more rows across it.
func (d *Data) writeSheet(b *strings.Builder, t *Table, sheet *Sheet) {
	about := func(i int, field string) string {
		if i < len(sheet.Notes) && strings.TrimSpace(sheet.Notes[i]) != "" {
			return sheet.Notes[i]
		}
		v, _ := t.Bean.Var(field)
		return v.Comment
	}
	value := func(field, raw string) string {
		v, ok := t.Bean.Var(field)
		if !ok || !IsTextKey(v) || isContainer(v.Type) {
			return cell(raw)
		}
		key := strings.TrimSpace(raw)
		if text, ok := d.SourceText(key); ok {
			return fmt.Sprintf("`%s`<br>%s", cell(key), cell(text))
		}
		return fmt.Sprintf("`%s`", cell(key))
	}
	switch {
	case len(sheet.Rows) == 0:
		b.WriteString("(no rows)\n\n")
	case len(sheet.Rows) == 1:
		b.WriteString("| Field | What it holds | Type | Group | Value |\n|---|---|---|---|---|\n")
		for i, field := range sheet.Fields {
			v, _ := t.Bean.Var(field)
			fmt.Fprintf(b, "| `%s` | %s | `%s` | %s | %s |\n", cell(field), cell(about(i, field)), cell(v.Type), cell(v.Group), value(field, sheet.Rows[0].Cells[i]))
		}
		b.WriteString("\n")
	default:
		b.WriteString("|")
		for i, field := range sheet.Fields {
			if note := about(i, field); note != "" {
				fmt.Fprintf(b, " `%s`<br>%s |", cell(field), cell(note))
			} else {
				fmt.Fprintf(b, " `%s` |", cell(field))
			}
		}
		fmt.Fprintf(b, "\n|%s\n", strings.Repeat("---|", len(sheet.Fields)))
		for _, row := range sheet.Rows {
			b.WriteString("|")
			for i, field := range sheet.Fields {
				fmt.Fprintf(b, " %s |", value(field, row.Cells[i]))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// TablesOf lists the files a set of files of the game data stands for in a
// review of the tables: the files themselves, sorted, the text tables left
// out.
func (l *Layout) TablesOf(files []string) []string {
	var out []string
	for _, file := range files {
		if l.Holds(file) && !l.IsText(file) && !contains(out, file) {
			out = append(out, file)
		}
	}
	sort.Strings(out)
	return out
}

// TextsOf lists the text tables among a set of files, sorted.
func (l *Layout) TextsOf(files []string) []string {
	var out []string
	for _, file := range files {
		if l.IsText(file) && !contains(out, file) {
			out = append(out, file)
		}
	}
	sort.Strings(out)
	return out
}
