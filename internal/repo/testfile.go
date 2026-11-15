package repo

import (
	"path"
	"strings"
)

// IsTestFile reports whether a file is test code, going by the conventions of
// the toolchains the kit knows. The path may use either slash.
//
// C# has no convention for file names, so a .cs file also counts when it sits
// in a test project: a directory named like Game.Core.Tests, UnitTests or
// tests. Build output (bin, obj, Godot's .godot) never counts.
func IsTestFile(filePath string) bool {
	filePath = strings.ReplaceAll(filePath, "\\", "/")
	base := path.Base(filePath)

	switch {
	case strings.HasSuffix(base, "_test.go"):
		return true
	case strings.HasSuffix(base, ".py"):
		return strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py")
	case strings.HasSuffix(base, ".cs"):
		return isCSharpTestFile(filePath, base)
	}
	for _, marker := range []string{".test.", ".spec."} {
		if i := strings.LastIndex(base, marker); i > 0 {
			switch base[i+len(marker):] {
			case "ts", "tsx", "js", "jsx", "mjs", "cjs":
				return true
			}
		}
	}
	return false
}

func isCSharpTestFile(filePath, base string) bool {
	dirs := strings.Split(path.Dir(filePath), "/")
	for _, dir := range dirs {
		if dir == "bin" || dir == "obj" || dir == ".godot" {
			return false
		}
	}
	name := strings.TrimSuffix(base, ".cs")
	if strings.HasSuffix(name, "Tests") || strings.HasSuffix(name, "Test") {
		return true
	}
	for _, dir := range dirs {
		lower := strings.ToLower(dir)
		if lower == "test" || lower == "tests" ||
			strings.HasSuffix(lower, ".test") || strings.HasSuffix(lower, ".tests") ||
			strings.HasSuffix(dir, "Test") || strings.HasSuffix(dir, "Tests") {
			return true
		}
	}
	return false
}
