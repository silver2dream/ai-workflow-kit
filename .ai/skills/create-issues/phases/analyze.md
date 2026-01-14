# Phase 1: Codebase Analysis

Scan the project structure to understand the codebase before breaking down requirements.

## Step 1: Read Project Configuration

```bash
cat .ai/config/workflow.yaml
```

Extract:
- `project.name` - Project name
- `project.type` - monorepo or single-repo
- `repos[]` - Repository configurations
  - `name` - Repo identifier (backend, frontend, etc.)
  - `path` - Directory path
  - `language` - Programming language
  - `verify.build` - Build command
  - `verify.test` - Test command
- `git.integration_branch` - Target branch for PRs
- `git.commit_format` - Commit message format

## Step 2: Scan Project Structure

Use Glob to identify key directories and files:

```
# Find source files by language
Glob: **/*.go          # Go files
Glob: **/*.cs          # C# files
Glob: **/*.ts          # TypeScript files
Glob: **/*.py          # Python files

# Find test files
Glob: **/*_test.go     # Go tests
Glob: **/test_*.py     # Python tests
Glob: **/*.test.ts     # TypeScript tests
Glob: **/*Tests.cs     # C# tests

# Find config files
Glob: **/go.mod        # Go modules
Glob: **/package.json  # Node packages
Glob: **/*.csproj      # C# projects
```

## Step 3: Identify Module Structure

For each repo in workflow.yaml:

### Go Projects
```
Glob: {repo_path}/internal/modules/*/
Glob: {repo_path}/pkg/*/
Glob: {repo_path}/cmd/*/
```

Key patterns to identify:
- `*_module.go` - Module registration
- `*_service.go` - Business logic
- `*_repository.go` - Data access ports
- `*_repository_mongo.go` - MongoDB adapters
- `*_cache_redis.go` - Redis cache adapters

### Unity Projects
```
Glob: {repo_path}/Assets/Scripts/*/
Glob: {repo_path}/Assets/Scripts/Domain/*/
Glob: {repo_path}/Assets/Scripts/UI/*/
```

## Step 4: Check Test Coverage

Identify existing test patterns:

```bash
# For Go
find {repo_path} -name "*_test.go" | wc -l

# List modules without tests
# Compare module directories vs test file presence
```

## Step 5: Summarize Analysis

Output a structured summary:

```markdown
## Codebase Analysis Summary

### Project
- **Name**: {project.name}
- **Type**: {project.type}
- **Integration Branch**: {git.integration_branch}

### Repositories

#### {repo.name}
- **Path**: {repo.path}
- **Language**: {repo.language}
- **Build**: `{repo.verify.build}`
- **Test**: `{repo.verify.test}`

**Modules Found**:
- `module_a/` - {description}
- `module_b/` - {description}

**Test Coverage**:
- Total source files: N
- Total test files: M
- Modules without tests: [list]

### Patterns Identified
- Repository pattern: Yes/No
- Service layer: Yes/No
- Event bus: Yes/No
- Dependency injection: Yes/No
```

## Output

Store the analysis summary for use in Phase 2 (Breakdown).

The analysis informs:
1. Which repo(s) the requirement affects
2. Which modules need modification
3. What verification commands to include
4. What patterns to follow

## Next Phase

Proceed to **Phase 2: Breakdown** - Read `phases/breakdown.md`
