package repo

import "strings"

// The toolchains the kit has something specific to offer for: how to set one
// up in CI, how to make its test runner name the tests it ran, what a test
// file looks like.
const (
	ToolchainGo     = "go"
	ToolchainDotnet = "dotnet"
	ToolchainNode   = "node"
	ToolchainPython = "python"
)

// toolchains maps every name a workflow config may give a repo's language to
// its toolchain. This is the one list: nothing else matches language names.
var toolchains = map[string]string{
	"go": ToolchainGo, "golang": ToolchainGo,

	// A Godot project here is a Godot .NET project. It builds and tests with
	// the .NET SDK alone: the engine SDK is a NuGet package.
	"csharp": ToolchainDotnet, "c#": ToolchainDotnet, "cs": ToolchainDotnet,
	"dotnet": ToolchainDotnet, "godot": ToolchainDotnet,
	"aspnet": ToolchainDotnet, "blazor": ToolchainDotnet,

	"node": ToolchainNode, "nodejs": ToolchainNode,
	"typescript": ToolchainNode, "ts": ToolchainNode,
	"javascript": ToolchainNode, "js": ToolchainNode,
	"react": ToolchainNode, "vue": ToolchainNode, "angular": ToolchainNode,
	"nextjs": ToolchainNode, "nuxt": ToolchainNode, "svelte": ToolchainNode,
	"express": ToolchainNode, "nestjs": ToolchainNode,

	"python": ToolchainPython, "py": ToolchainPython,
	"django": ToolchainPython, "flask": ToolchainPython, "fastapi": ToolchainPython,
}

// ToolchainOf returns the toolchain of a repo language as the workflow config
// names it. It returns "" for any other language: the kit then runs the repo's
// own commands and assumes nothing about them.
func ToolchainOf(language string) string {
	return toolchains[strings.ToLower(strings.TrimSpace(language))]
}
