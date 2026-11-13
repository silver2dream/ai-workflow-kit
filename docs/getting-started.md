# Getting Started

**AWK (AI Workflow Kit)** installs into your repository. This guide takes a project from `awkit init` to its first run.

## 0) Install AWK into your repo

Install the `awkit` CLI from GitHub Releases, then run it in your project directory:

```bash
# Any stack: a single repo, you name the build and test commands
awkit init

# Go backend + Godot C# frontend, with a project skeleton
awkit init --preset godot-go --scaffold

# Or name the project's path
awkit init /path/to/your-repo --preset godot-go --scaffold
```

## Available Presets

`awkit list-presets` prints them.

| Preset | Project | Repos | Verify commands |
|--------|---------|-------|-----------------|
| `generic` (default) | single repo, any stack | `root` at `./`, language `generic` | empty: you set them |
| `godot-go` | monorepo | `backend/` (Go), `frontend/` (Godot C#) | `go build ./...`, `go test ./...`; `dotnet build`, `dotnet test` |

The `generic` preset assumes nothing about your project. Set `verify.build` and `verify.test` of the repo in `.ai/config/workflow.yaml` before the first run: the Worker runs them before it opens a pull request, the review runs the tests again, and CI is generated from them.

## Using --scaffold

`--scaffold` writes a project skeleton. Only the `godot-go` preset has one; the `generic` preset knows nothing it could write.

```bash
awkit init --preset godot-go --scaffold

# Preview what would be created
awkit init --preset godot-go --scaffold --dry-run
```

```
.
├── backend/                       # Go module
│   ├── go.mod
│   ├── main.go
│   ├── health/
│   │   ├── health.go
│   │   └── health_test.go
│   └── README.md
└── frontend/                      # Godot C# solution
    ├── .gitignore
    ├── Frontend.sln
    ├── Directory.Build.props
    ├── game/                      # the Godot project
    │   ├── Game.csproj
    │   └── project.godot
    ├── src/Game.Core/             # game logic that does not depend on the engine
    │   ├── Game.Core.csproj
    │   └── Counter.cs
    ├── tests/Game.Core.Tests/     # xUnit tests of that logic
    │   ├── Game.Core.Tests.csproj
    │   └── CounterTests.cs
    └── README.md
```

The skeleton builds and passes its tests as it is: `go build ./... && go test ./...` in `backend/`, `dotnet build && dotnet test` in `frontend/`. Building needs the .NET 8 SDK; the Godot engine (4.x, .NET edition) is needed only to open and run the game.

Files that already exist are kept; `--force` overwrites them.

## 1) Configure `workflow.yaml`

Edit `.ai/config/workflow.yaml`:

- Set `repos[].type` based on your project structure:
  - `root`: Single repository (path must be `./`)
  - `directory`: Subdirectory in monorepo (shared .git)
  - `submodule`: Git submodule (independent .git)
- Set `git.integration_branch` (default: `feat/example`)
- Keep `specs.active: []` until you add your own spec

Example configurations:

```yaml
# Single repo
repos:
  - name: root
    path: ./
    type: root

# Monorepo with directories
repos:
  - name: backend
    path: backend/
    type: directory
  - name: frontend
    path: frontend/
    type: directory

# Monorepo with submodules
repos:
  - name: backend
    path: backend/
    type: submodule
```

If you don't use an integration/release branch split, set `git.integration_branch` to the same value as `git.release_branch` (for example: both `main`).

## 2) (Optional) Enable rule packs

AWK defaults to a minimal rule set under `.ai/rules/_kit/`.

If you want stricter, tech-specific rules, copy them from `.ai/rules/_examples/` into `.ai/rules/`, then enable them in `.ai/config/workflow.yaml` under `rules.custom`.

Example:

- Copy `.ai/rules/_examples/testing-strategy.md` → `.ai/rules/testing-strategy.md`
- Update `.ai/config/workflow.yaml`:

```yaml
rules:
  kit:
    - git-workflow
  custom:
    - testing-strategy
```

Then regenerate helper docs (recommended):

```bash
awkit generate
```

This will also generate `.claude/settings.local.json` with pre-approved permissions for `gh`, `git`, `codex`, and your verify commands. This enables true autopilot mode without manual approval prompts.

## 3) Add your first spec

Create a new folder under `.ai/specs/<your-feature>/` and add at least `tasks.md`.

You can use `.ai/specs/example/` as a template (it includes `requirements.md`, `design.md`, and `tasks.md`).

## 4) Run offline verification

```bash
awkit evaluate --offline
go test ./...
```

## 5) CI (GitHub Actions)

`awkit init` and `awkit generate` write a CI workflow for every repo that has a build or a test command:

| Repo type | Workflow |
|-----------|----------|
| `root` | `.github/workflows/ci.yml` |
| `directory` | `.github/workflows/ci-<repo>.yml`, run in the repo's directory |
| `submodule` | `.github/workflows/ci.yml` inside the submodule |

Each workflow checks the code out, sets up the toolchain of the repo's language (Go from the module's `go.mod`, the .NET SDK for C# and Godot) and runs `verify.setup`, `verify.build` and `verify.test`: the commands the Worker verifies with. A repo with no build and no test command gets no workflow.

A generated workflow starts with a comment that says so. While that comment is there, awkit keeps the file in step with the config: change a branch or a command, run `awkit generate`, and the workflow follows. To change a workflow by hand, delete the comment; from then on the file is yours and awkit leaves it alone. `awkit generate --generate-ci` rewrites every workflow from the config, including the ones you took over.

A project that already has its own `.github/workflows/ci.yml` keeps it, and no per-repo workflow is added next to it.
