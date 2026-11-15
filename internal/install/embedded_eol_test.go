package install

import (
	"bytes"
	"io/fs"
	"strings"
	"testing"
)

// What is embedded is written into projects byte for byte, so it has to be
// the same bytes whatever system awkit was built on. A checkout that turns
// line endings into CRLF put CRLF into every skeleton a build from it wrote,
// and into the list of deprecated files, whose lines are paths.
//
// .gitattributes checks every text file out with LF. This fails where it
// does not, and says so; without it the first sign was a go.mod that did not
// compare equal on the Windows runner.
func TestEmbeddedFiles_CarryNoCarriageReturn(t *testing.T) {
	const hint = ": the checkout converted its line endings; .gitattributes has to say eol=lf"

	files := 0
	err := fs.WalkDir(scaffoldFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := scaffoldFS.ReadFile(path)
		if err != nil {
			return err
		}
		files++
		if bytes.Contains(data, []byte("\r\n")) {
			t.Errorf("%s is embedded with CRLF line endings%s", path, hint)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files == 0 {
		t.Error("no skeleton file is embedded: nothing was checked")
	}

	if strings.Contains(deprecatedFiles, "\r") {
		t.Errorf("deprecated.txt is embedded with CRLF line endings%s", hint)
	}
}
