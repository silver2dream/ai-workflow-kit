package repo

import "testing"

func TestIsTestFile(t *testing.T) {
	tests := map[string]bool{
		// Go
		"internal/health/health_test.go": true,
		"health_test.go":                 true,
		"internal/health/health.go":      false,
		"internal/testutil/helpers.go":   false,

		// JavaScript and TypeScript
		"src/app.test.ts":         true,
		"src/App.test.tsx":        true,
		"src/app.spec.js":         true,
		"src/app.jittest.test.ts": true,
		"src/app.ts":              false,
		"src/test.ts":             false,
		"src/latest.ts":           false,
		"src/app.test.css":        false,

		// Python
		"tests/test_app.py": true,
		"app/app_test.py":   true,
		"app/app.py":        false,
		"app/contest.py":    false,

		// C#: by file name
		"frontend/tests/Game.Core.Tests/CounterTests.cs": true,
		"src/Game.Core/CounterTest.cs":                   true,
		"src/Game.Core/Counter.cs":                       false,
		// C#: by test project, whatever the file is called
		"frontend/tests/Game.Core.Tests/Fixtures.cs": true,
		"Game.Core.Tests/Counter.jittest.cs":         true,
		"UnitTests/Helpers.cs":                       true,
		"test/Helpers.cs":                            true,
		`frontend\tests\Game.Core.Tests\Fixtures.cs`: true,
		// C#: build output never counts
		"frontend/tests/Game.Core.Tests/obj/Debug/net8.0/Game.Core.Tests.AssemblyInfo.cs": false,
		"frontend/tests/Game.Core.Tests/bin/Debug/Generated.cs":                           false,
		"frontend/game/.godot/mono/temp/obj/GameTests.cs":                                 false,

		// Not code
		"docs/testing.md": false,
		"":                false,
	}
	for path, want := range tests {
		if got := IsTestFile(path); got != want {
			t.Errorf("IsTestFile(%q) = %v, want %v", path, got, want)
		}
	}
}
