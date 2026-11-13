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

The `generic` preset assumes nothing about your project. Set `verify.build` and `verify.test` of the repo in `.ai/config.yaml` before the first run: the Worker runs them before it opens a pull request, the review runs the tests again, and CI is generated from them.

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

## 1) Configure `config.yaml`

Edit `.ai/config.yaml`:

- Set `repos[].type` based on your project structure:
  - `root`: Single repository (path must be `./`)
  - `directory`: Subdirectory in monorepo (shared .git)
  - `submodule`: Git submodule (independent .git)
- Set `git.integration_branch`: the branch pull requests target
- Set `repos[].verify`: the commands the Worker, the review and CI run

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

## 2) (Optional) Write the project's rules

AWK comes with one rule file, `.claude/rules/_kit/git-workflow.md`. The `godot-go` preset adds three starter rules for its stack (`backend`, `frontend`, `testing`) and lists them in the configuration; they are the project's own from then on.

To add a rule of your own, write `.claude/rules/<name>.md` and list the name in `.ai/config.yaml`:

```yaml
rules:
  kit:
    - git-workflow
  custom:
    - <name>
```

A rule file holds rules (what is required, what is forbidden, what to check), and a `paths:` block at its top says which files it is about. See [Skills and rules](user/skills.md).

Then regenerate helper docs (recommended):

```bash
awkit generate
```

This will also generate `.claude/settings.local.json` with pre-approved permissions for `gh`, `git`, `codex`, and your verify commands. This enables true autopilot mode without manual approval prompts.

## 3) Add your first ticket

For anything larger than one ticket, let the council plan it: `awkit plan "<what you want built>"` has the roles of `.ai/roster.yaml` argue the brief out, write the documents under `docs/`, record their decisions and propose the tickets; `awkit plan approve <plan>` creates the issues once you have read the list. See "Plan the work: the council" in the README.

The work is a queue of tickets: GitHub issues with the task label, each with a `yaml awk-ticket` block that names its repo and the issues it waits for. Write the body to a file (the template is in `.claude/skills/principal-workflow/references/ticket.md`) and create the issue:

```bash
awkit create-task --title "[feat] add a health endpoint" --body-file ticket.md
awkit run              # work through every open ticket
awkit run --issue 42   # or one ticket only
```

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
