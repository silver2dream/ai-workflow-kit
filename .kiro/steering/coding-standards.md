# Coding Standards

## File Encoding

- All files MUST be written in UTF-8 encoding
- This applies to all text files: code, documentation, configuration, etc.

## Documentation Sync

When updating documentation files:

1. **README sync**: When updating `README.md`, also update `README-zh-TW.md` (Traditional Chinese version)
2. **Cross-reference check**: Review all related documentation files for consistency
3. **Architecture docs**: Check if `docs/ai-workflow-architecture.md` needs corresponding updates

## Git Commit Message

When writing git commit messages, follow the rules in `.ai/rules/_kit/git-workflow.md`:

- **Format**: `[type] subject` + optional body
- **Rules**:
  1. Type MUST be inside square brackets `[]`
  2. Subject MUST be lowercase
  3. NO colon after the bracket
  4. Body uses bullet points with `-` prefix for details
- **Allowed Types**: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `chore`
- **Examples**:
  - ✅ Simple: `[docs] update readme`
  - ✅ With body:
    ```
    [feat] add user authentication module
    
    - Add login and logout endpoints
    - Implement JWT token validation
    - Add unit tests for auth service
    ```
  - ❌ `feat: add feature` (Forbidden - no colon format)

## Language

- User prefers Traditional Chinese (繁體中文) for communication
- Documentation should maintain both English and Traditional Chinese versions where applicable
