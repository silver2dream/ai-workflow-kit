// Package gamedata reads and checks a project's game data: the numbers and
// the text a game reads, kept as tables that Luban
// (github.com/focus-creative-games/luban) turns into code and data for the
// client and the server.
//
// The game data is kept the way Luban reads it. luban.conf, in a folder of
// its own, names the schema (XML files of beans and tables) and the folder of
// the data, which holds one CSV file per table. The text a player reads is
// not in the tables: a table holds a text key, and the text tables, in the
// folder text/ of the data folder, hold the text of each key in each
// language.
//
// What is checked here is what Luban does not check: a number left blank,
// which Luban reads as 0; a table whose columns are not its schema's; a group
// left unmarked, which ships a table to the client; a text key the text
// tables do not have; a translation older than the text it translates. Luban
// itself checks the rest when the project generates its data.
package gamedata

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

// ConfFile is Luban's configuration. The folder that holds it holds the game
// data.
const ConfFile = "luban.conf"

// TextFolder is the folder of the text tables, inside the data folder.
const TextFolder = "text/"

// Layout is where a project keeps its game data, as its luban.conf says.
// Every path is from the project's root, with forward slashes; a folder ends
// in a slash.
type Layout struct {
	// Dir is the folder of luban.conf.
	Dir string
	// Schemas are the schema files and folders luban.conf names.
	Schemas []string
	// Data is the folder of the tables.
	Data string
	// Groups are the groups luban.conf names (c for the client, s for the
	// server, in a project that starts with the kit's).
	Groups []string
}

// Conf is the path of luban.conf.
func (l *Layout) Conf() string {
	return l.Dir + ConfFile
}

// Text is the folder of the text tables.
func (l *Layout) Text() string {
	return l.Data + TextFolder
}

// Holds reports whether a file is part of the game data: a schema file, a
// table, a text table.
func (l *Layout) Holds(file string) bool {
	switch {
	case strings.HasPrefix(file, l.Data):
		return strings.HasSuffix(strings.ToLower(file), ".csv")
	case strings.HasSuffix(strings.ToLower(file), ".xml"):
		for _, schema := range l.Schemas {
			if file == schema || strings.HasSuffix(schema, "/") && strings.HasPrefix(file, schema) {
				return true
			}
		}
	}
	return false
}

// IsText reports whether a file is a text table.
func (l *Layout) IsText(file string) bool {
	return strings.HasPrefix(file, l.Text()) && strings.HasSuffix(strings.ToLower(file), ".csv")
}

// conf is what is read from luban.conf.
type conf struct {
	Groups []struct {
		Names []string `json:"names"`
	} `json:"groups"`
	SchemaFiles []struct {
		FileName string `json:"fileName"`
	} `json:"schemaFiles"`
	DataDir string `json:"dataDir"`
}

// Find looks for the game data of a project: a luban.conf in a folder at the
// project's root (gamedata/luban.conf, in a project that starts with the
// kit's). It returns nil when there is none. root is the project's root.
func Find(root fs.FS) (*Layout, error) {
	entries, err := fs.ReadDir(root, ".")
	if err != nil {
		return nil, err
	}
	var found []string
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}
		if info, err := fs.Stat(root, name+"/"+ConfFile); err == nil && !info.IsDir() {
			found = append(found, name+"/")
		}
	}
	switch len(found) {
	case 0:
		return nil, nil
	case 1:
		return ReadLayout(root, found[0])
	default:
		sort.Strings(found)
		return nil, fmt.Errorf("%s holds a %s, and so does %s: the game data of a project is kept in one folder", found[0], ConfFile, strings.Join(found[1:], ", "))
	}
}

// ReadLayout reads the luban.conf of a folder (gamedata/).
func ReadLayout(root fs.FS, dir string) (*Layout, error) {
	dir = strings.TrimSuffix(dir, "/") + "/"
	file := dir + ConfFile
	data, err := fs.ReadFile(root, file)
	if err != nil {
		return nil, err
	}
	var c conf
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	if strings.TrimSpace(c.DataDir) == "" {
		return nil, fmt.Errorf("%s names no dataDir: the folder of the tables", file)
	}
	if len(c.SchemaFiles) == 0 {
		return nil, fmt.Errorf("%s names no schemaFiles: the schema of the tables", file)
	}
	layout := &Layout{Dir: dir}
	if layout.Data, err = inside(dir, c.DataDir); err != nil {
		return nil, fmt.Errorf("%s: dataDir: %w", file, err)
	}
	layout.Data += "/"
	for _, schema := range c.SchemaFiles {
		at, err := inside(dir, schema.FileName)
		if err != nil {
			return nil, fmt.Errorf("%s: schemaFiles: %w", file, err)
		}
		if info, err := fs.Stat(root, at); err == nil && info.IsDir() || err != nil && path.Ext(at) == "" {
			at += "/"
		}
		layout.Schemas = append(layout.Schemas, at)
	}
	for _, group := range c.Groups {
		layout.Groups = append(layout.Groups, group.Names...)
	}
	return layout, nil
}

// inside joins a path luban.conf names to the folder of luban.conf, and
// refuses one that leaves the folder: the game data is kept in one place.
func inside(dir, name string) (string, error) {
	name = strings.TrimSpace(strings.ReplaceAll(name, `\`, "/"))
	if name == "" {
		return "", errors.New("the path is empty")
	}
	if path.IsAbs(name) || strings.Contains(name, ":") {
		return "", fmt.Errorf("%q is not a path inside %s", name, dir)
	}
	joined := path.Join(dir, name)
	if !strings.HasPrefix(joined+"/", dir) || joined+"/" == dir {
		return "", fmt.Errorf("%q is not a path inside %s", name, dir)
	}
	return joined, nil
}
