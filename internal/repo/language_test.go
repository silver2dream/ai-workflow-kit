package repo

import "testing"

func TestToolchainOf(t *testing.T) {
	tests := map[string]string{
		"go":         ToolchainGo,
		"Golang":     ToolchainGo,
		" go ":       ToolchainGo,
		"csharp":     ToolchainDotnet,
		"C#":         ToolchainDotnet,
		"cs":         ToolchainDotnet,
		"dotnet":     ToolchainDotnet,
		"godot":      ToolchainDotnet, // a Godot project here is a Godot .NET project
		"typescript": ToolchainNode,
		"TS":         ToolchainNode,
		"react":      ToolchainNode,
		"python":     ToolchainPython,
		"fastapi":    ToolchainPython,
		// Languages the kit assumes nothing about.
		"generic":  "",
		"gdscript": "",
		"rust":     "",
		"unknown":  "",
		"":         "",
	}
	for language, want := range tests {
		if got := ToolchainOf(language); got != want {
			t.Errorf("ToolchainOf(%q) = %q, want %q", language, got, want)
		}
	}
}

// Every toolchain name is itself a language name, so a config may simply say
// what it means.
func TestToolchainOf_ToolchainNamesAreLanguages(t *testing.T) {
	for _, toolchain := range []string{ToolchainGo, ToolchainDotnet, ToolchainNode, ToolchainPython} {
		if got := ToolchainOf(toolchain); got != toolchain {
			t.Errorf("ToolchainOf(%q) = %q, want itself", toolchain, got)
		}
	}
	for language, toolchain := range toolchains {
		if ToolchainOf(toolchain) != toolchain {
			t.Errorf("language %q maps to %q, which is not a toolchain", language, toolchain)
		}
	}
}
