package gamedata

import (
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Data is the game data of a project as it stands: its schema, its tables
// and its text.
type Data struct {
	Layout *Layout
	// Beans are the beans of the schema by name, and by the name with the
	// module's in front where the module has one.
	Beans map[string]*Bean
	// Tables are the tables of the schema, in the order of the schema files.
	Tables []*Table
	// Texts are the text tables, by file.
	Texts []*TextFile
	// problems are what could not be read; Check reports them.
	problems []Finding
}

// Bean is a record type of the schema: the fields of a row.
type Bean struct {
	Name    string
	Comment string
	// File is the schema file that defines the bean.
	File string
	// Vars are the bean's fields, its parent's first.
	Vars []Var

	module, parent string
	own            []Var
}

// Var is a field of a bean.
type Var struct {
	Name    string
	Type    string
	Group   string
	Comment string
}

// Var returns the field of a name.
func (b *Bean) Var(name string) (Var, bool) {
	for _, v := range b.Vars {
		if v.Name == name {
			return v, true
		}
	}
	return Var{}, false
}

// Table is a table of the schema, and the data it reads.
type Table struct {
	Name    string
	Value   string
	Mode    string
	Group   string
	Comment string
	// File is the schema file that defines the table.
	File string
	// Inputs are the files and folders the table reads, from the project's
	// root.
	Inputs []string
	// Bean is the record type of a row; nil when the schema has none of
	// that name.
	Bean *Bean
	// Sheets are the CSV files among the inputs, as read.
	Sheets []*Sheet

	module string
}

// Sheet is one CSV file of a table.
type Sheet struct {
	File string
	// Fields are the names of the columns, from the ##var row; the first
	// column, which Luban keeps for itself, is not among them.
	Fields []string
	// Notes say in words what each column holds, from the ## row; empty
	// where there is none.
	Notes []string
	// Rows are the rows of data, without the header rows.
	Rows []Row
	// header is false when the file has no ##var row.
	header bool
}

// Row is one row of data, its cells in the order of the sheet's fields.
type Row struct {
	Line  int
	Cells []string
}

// Cell returns the cell of a field.
func (s *Sheet) Cell(row Row, field string) string {
	for i, name := range s.Fields {
		if name == field && i < len(row.Cells) {
			return row.Cells[i]
		}
	}
	return ""
}

// TextFile is one text table.
type TextFile struct {
	File string
	// Languages are the columns of text, the source language first: the
	// columns other than key whose names do not start with an underscore.
	Languages []string
	// Translated are the languages that are translated from the source and
	// kept up with it: the ones with a basis column (_en_依據).
	Translated []string
	Rows       []TextRow

	limit bool
}

// Source is the language the text is written in; the others are translated
// from it.
func (t *TextFile) Source() string {
	if len(t.Languages) == 0 {
		return ""
	}
	return t.Languages[0]
}

// TextRow is the text of one key.
type TextRow struct {
	File string
	Line int
	Key  string
	// Text is the text of each language.
	Text map[string]string
	// Basis is, for a translated language, the source text it was translated
	// from.
	Basis map[string]string
	// Limit is the most characters a translation may have; 0 when there is
	// no limit.
	Limit int
}

// Load reads the game data of a project. root is the project's root. What
// cannot be read is reported by Check, not returned as an error: the error is
// for a project whose files cannot be listed at all.
func Load(root fs.FS, layout *Layout) (*Data, error) {
	d := &Data{Layout: layout, Beans: map[string]*Bean{}}
	var schemaFiles []string
	for _, schema := range layout.Schemas {
		if !strings.HasSuffix(schema, "/") {
			schemaFiles = append(schemaFiles, schema)
			continue
		}
		files, err := filesIn(root, schema, ".xml", false)
		if err != nil {
			return nil, err
		}
		schemaFiles = append(schemaFiles, files...)
	}
	for _, file := range schemaFiles {
		d.readSchema(root, file)
	}
	d.resolve()
	for _, table := range d.Tables {
		for _, input := range table.Inputs {
			if strings.ToLower(path.Ext(input)) != ".csv" {
				continue // a folder of records, a sheet of another kind: Luban reads it
			}
			if sheet := d.readSheet(root, input); sheet != nil {
				table.Sheets = append(table.Sheets, sheet)
			}
		}
	}
	textFiles, err := filesIn(root, layout.Text(), ".csv", true)
	if err != nil {
		return nil, err
	}
	for _, file := range textFiles {
		if text := d.readText(root, file); text != nil {
			d.Texts = append(d.Texts, text)
		}
	}
	return d, nil
}

// filesIn lists the files of a kind in a folder, sorted. A folder that is not
// there holds none.
func filesIn(root fs.FS, dir, ext string, deep bool) ([]string, error) {
	dir = strings.TrimSuffix(dir, "/")
	var files []string
	err := fs.WalkDir(root, dir, func(file string, entry fs.DirEntry, err error) error {
		switch {
		case err != nil && file == dir && errors.Is(err, fs.ErrNotExist):
			return fs.SkipAll
		case err != nil:
			return err
		case entry.IsDir() && file != dir && !deep:
			return fs.SkipDir
		case !entry.IsDir() && strings.EqualFold(path.Ext(file), ext):
			files = append(files, file)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

func (d *Data) problem(file string, line int, format string, args ...any) {
	d.problems = append(d.problems, Finding{File: file, Line: line, Message: fmt.Sprintf(format, args...)})
}

// The schema as XML.
type xmlModule struct {
	XMLName xml.Name
	Name    string      `xml:"name,attr"`
	Beans   []xmlBean   `xml:"bean"`
	Tables  []xmlTable  `xml:"table"`
	Modules []xmlModule `xml:"module"`
}

type xmlBean struct {
	Name    string    `xml:"name,attr"`
	Parent  string    `xml:"parent,attr"`
	Comment string    `xml:"comment,attr"`
	Vars    []xmlVar  `xml:"var"`
	Beans   []xmlBean `xml:"bean"`
}

type xmlVar struct {
	Name    string `xml:"name,attr"`
	Type    string `xml:"type,attr"`
	Group   string `xml:"group,attr"`
	Comment string `xml:"comment,attr"`
}

type xmlTable struct {
	Name    string `xml:"name,attr"`
	Value   string `xml:"value,attr"`
	Input   string `xml:"input,attr"`
	Mode    string `xml:"mode,attr"`
	Group   string `xml:"group,attr"`
	Comment string `xml:"comment,attr"`
}

func (d *Data) readSchema(root fs.FS, file string) {
	data, err := fs.ReadFile(root, file)
	if err != nil {
		d.problem(file, 0, "cannot be read: %v", err)
		return
	}
	var module xmlModule
	if err := xml.Unmarshal(data, &module); err != nil {
		d.problem(file, 0, "is not a schema Luban can read: %v", err)
		return
	}
	if module.XMLName.Local != "module" {
		d.problem(file, 0, "starts with <%s>; a schema file is one <module>", module.XMLName.Local)
		return
	}
	d.addModule(file, module, "")
}

func (d *Data) addModule(file string, module xmlModule, outer string) {
	name := joinName(outer, module.Name)
	for _, bean := range module.Beans {
		d.addBean(file, name, bean, "")
	}
	for _, table := range module.Tables {
		t := &Table{Name: table.Name, Value: strings.TrimSpace(table.Value), Mode: table.Mode, Group: table.Group,
			Comment: table.Comment, File: file, module: name}
		for input := range strings.SplitSeq(table.Input, ",") {
			input = strings.TrimSpace(strings.ReplaceAll(input, `\`, "/"))
			if input == "" {
				continue
			}
			// A sheet of a workbook is named after an @: the file is what
			// comes after it.
			if at := strings.LastIndex(input, "@"); at >= 0 {
				input = input[at+1:]
			}
			at, err := inside(d.Layout.Data, input)
			if err != nil {
				d.problem(file, 0, "table %s reads %s", table.Name, err)
				continue
			}
			t.Inputs = append(t.Inputs, at)
		}
		d.Tables = append(d.Tables, t)
	}
	for _, inner := range module.Modules {
		d.addModule(file, inner, name)
	}
}

func (d *Data) addBean(file, module string, bean xmlBean, parent string) {
	if bean.Parent != "" {
		parent = bean.Parent
	}
	b := &Bean{Name: bean.Name, Comment: bean.Comment, File: file, module: module, parent: parent}
	for _, v := range bean.Vars {
		b.own = append(b.own, Var{Name: v.Name, Type: v.Type, Group: v.Group, Comment: v.Comment})
	}
	full := joinName(module, bean.Name)
	if earlier, ok := d.Beans[full]; ok && earlier != b {
		d.problem(file, 0, "defines the bean %s, and so does %s", full, earlier.File)
	}
	d.Beans[full] = b
	if _, taken := d.Beans[bean.Name]; !taken {
		d.Beans[bean.Name] = b
	}
	for _, child := range bean.Beans {
		d.addBean(file, module, child, full)
	}
}

func joinName(module, name string) string {
	if module == "" {
		return name
	}
	return module + "." + name
}

// bean finds a bean by the name a table or a bean in a module calls it.
func (d *Data) bean(module, name string) *Bean {
	if b, ok := d.Beans[joinName(module, name)]; ok {
		return b
	}
	return d.Beans[name]
}

// resolve gives each bean its parent's fields and each table its bean.
func (d *Data) resolve() {
	var fields func(b *Bean, seen map[*Bean]bool) []Var
	fields = func(b *Bean, seen map[*Bean]bool) []Var {
		if seen[b] {
			d.problem(b.File, 0, "the bean %s is its own parent", b.Name)
			return nil
		}
		seen[b] = true
		var vars []Var
		if b.parent != "" {
			if parent := d.bean(b.module, b.parent); parent != nil {
				vars = append(vars, fields(parent, seen)...)
			} else {
				d.problem(b.File, 0, "the bean %s has the parent %s, which the schema does not define", b.Name, b.parent)
			}
		}
		return append(vars, b.own...)
	}
	done := map[*Bean]bool{}
	for _, b := range d.Beans {
		if !done[b] {
			done[b] = true
			b.Vars = fields(b, map[*Bean]bool{})
		}
	}
	for _, t := range d.Tables {
		t.Bean = d.bean(t.module, t.Value)
	}
}

// readCSV reads a CSV file, each record with the line it starts on.
func readCSV(root fs.FS, file string) ([][]string, []int, error) {
	data, err := fs.ReadFile(root, file)
	if err != nil {
		return nil, nil, err
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1
	var records [][]string
	var lines []int
	for {
		record, err := reader.Read()
		if err == io.EOF {
			return records, lines, nil
		}
		if err != nil {
			return nil, nil, err
		}
		line, _ := reader.FieldPos(0)
		records = append(records, record)
		lines = append(lines, line)
	}
}

func (d *Data) readSheet(root fs.FS, file string) *Sheet {
	records, lines, err := readCSV(root, file)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil // reported with the table that reads it
		}
		d.problem(file, 0, "is not CSV Luban can read: %v", err)
		return nil
	}
	sheet := &Sheet{File: file}
	for i, record := range records {
		first := strings.TrimSpace(record[0])
		switch {
		case first == "##var":
			if sheet.header {
				d.problem(file, lines[i], "has a second ##var row")
				continue
			}
			sheet.header = true
			for _, field := range record[1:] {
				sheet.Fields = append(sheet.Fields, strings.TrimSpace(field))
			}
		case first == "##" && sheet.Notes == nil:
			sheet.Notes = append([]string(nil), record[1:]...)
		case strings.HasPrefix(first, "#"):
			// ##type, ##group, a comment: not data
		case len(record) == 1 && first == "":
			// an empty row
		default:
			cells := make([]string, len(sheet.Fields))
			copy(cells, record[1:])
			if len(record)-1 > len(sheet.Fields) && sheet.header {
				d.problem(file, lines[i], "has %d cells, and the ##var row names %d fields", len(record)-1, len(sheet.Fields))
			}
			sheet.Rows = append(sheet.Rows, Row{Line: lines[i], Cells: cells})
		}
	}
	return sheet
}

// The names a text table's columns may have, besides the languages: a
// basis column per translated language and the limit of a translation's
// length. Both are written the way the project that the kit's rules come
// from writes them, or in English.
const (
	keyColumn = "key"
)

func basisColumns(language string) []string {
	return []string{"_" + language + "_依據", "_" + language + "_basis"}
}

// languageCode is the name of a column of text: a language, and the country
// or the script where the language has more than one (zh_TW, en, pt-BR).
var languageCode = regexp.MustCompile(`^[a-z]{2,3}([_-][A-Za-z0-9]{2,4})?$`)

// basisOf reads a column as the basis of a translation: the language it is
// the basis of, and whether it is one. "_依據" alone is one that names none.
func basisOf(column string) (string, bool) {
	for _, suffix := range []string{"依據", "basis"} {
		if column == "_"+suffix {
			return "", true
		}
		if language, ok := strings.CutSuffix(column, "_"+suffix); ok && strings.HasPrefix(language, "_") {
			return strings.TrimPrefix(language, "_"), true
		}
	}
	return "", false
}

var limitColumns = []string{"_上限", "_limit"}

func (d *Data) readText(root fs.FS, file string) *TextFile {
	records, lines, err := readCSV(root, file)
	if err != nil {
		d.problem(file, 0, "is not CSV: %v", err)
		return nil
	}
	if len(records) == 0 {
		d.problem(file, 0, "is empty; a text table starts with its header: key, then a column per language")
		return nil
	}
	header := map[string]int{}
	text := &TextFile{File: file}
	for i, column := range records[0] {
		column = strings.TrimSpace(column)
		if _, twice := header[column]; twice {
			d.problem(file, lines[0], "has the column %s twice", column)
		}
		header[column] = i
		switch {
		case column == keyColumn || column == "" || strings.HasPrefix(column, "_"):
		case languageCode.MatchString(column):
			text.Languages = append(text.Languages, column)
		default:
			d.problem(file, lines[0], "has the column %q, which is neither a language (a language code: zh_TW, en, ja) nor a note (a name that starts with _)", column)
		}
	}
	for _, column := range records[0] {
		column = strings.TrimSpace(column)
		if language, ok := basisOf(column); ok && (language == text.Source() || !contains(text.Languages, language)) {
			switch {
			case language != "" && language == text.Source():
				d.problem(file, lines[0], "has the column %s, and %s is the language the text is written in: it is not translated from another", column, language)
			case language == "":
				d.problem(file, lines[0], "has the column %s, which names no language: the basis of a translation is _<language>_依據 (_en_依據), the text in the source language it was translated from", column)
			default:
				d.problem(file, lines[0], "has the column %s, the basis of %s, and no column of %s", column, language, language)
			}
		}
	}
	if _, ok := header[keyColumn]; !ok {
		d.problem(file, lines[0], "has no column %q; a text table starts with its header: key, then a column per language", keyColumn)
		return nil
	}
	if len(text.Languages) == 0 {
		d.problem(file, lines[0], "has no column of text: a column per language follows key, the source language first")
		return nil
	}
	basis := map[string]int{}
	for _, language := range text.Languages[1:] {
		for _, name := range basisColumns(language) {
			if at, ok := header[name]; ok {
				basis[language] = at
				text.Translated = append(text.Translated, language)
				break
			}
		}
	}
	limitAt := -1
	for _, name := range limitColumns {
		if at, ok := header[name]; ok {
			limitAt = at
			text.limit = true
			break
		}
	}
	cell := func(record []string, at int) string {
		if at >= 0 && at < len(record) {
			return record[at]
		}
		return ""
	}
	for i, record := range records[1:] {
		line := lines[i+1]
		if len(record) == 1 && strings.TrimSpace(record[0]) == "" {
			continue
		}
		row := TextRow{File: file, Line: line, Key: strings.TrimSpace(cell(record, header[keyColumn])),
			Text: map[string]string{}, Basis: map[string]string{}}
		for _, language := range text.Languages {
			row.Text[language] = cell(record, header[language])
		}
		for language, at := range basis {
			row.Basis[language] = cell(record, at)
		}
		if limit := strings.TrimSpace(cell(record, limitAt)); limit != "" {
			n, err := strconv.Atoi(limit)
			if err != nil || n < 0 {
				d.problem(file, line, "the limit %q is not a whole number", limit)
			} else {
				row.Limit = n
			}
		}
		text.Rows = append(text.Rows, row)
	}
	return text
}

// Keys returns the text of each key in the source language, and where each
// key is.
func (d *Data) Keys() map[string]TextRow {
	keys := map[string]TextRow{}
	for _, text := range d.Texts {
		for _, row := range text.Rows {
			if _, ok := keys[row.Key]; !ok {
				keys[row.Key] = row
			}
		}
	}
	return keys
}

// SourceText returns the text of a key in the language it is written in.
func (d *Data) SourceText(key string) (string, bool) {
	for _, text := range d.Texts {
		for _, row := range text.Rows {
			if row.Key == key {
				return row.Text[text.Source()], true
			}
		}
	}
	return "", false
}
