package workbench

import (
	"embed"
	"io/fs"
)

// The page is built from web/ into dist/ (npm --prefix web run build) before
// awkit is. Only dist/.keep is committed: an awkit built without the page
// still builds, and says so when the workbench is opened.
//
//go:embed all:dist
var dist embed.FS

// Assets are the files of the page, or nil when this awkit was built without
// them.
func Assets() fs.FS {
	files, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil
	}
	if _, err := fs.Stat(files, "index.html"); err != nil {
		return nil
	}
	return files
}
