# AI Workflow Kit v5.0 - Design

## Overview

v5.0 修正 evaluate 框架的架構一致性問題，確保：
1. Offline Gate 真正不需網路
2. audit/scan 產物 schema 統一
3. --strict 行為可追溯

## Architecture

### 變更範圍

1. `.ai/scripts/audit_project.sh` - 移除 git fetch，統一 schema
2. `.ai/scripts/audit_project.py` - 統一 schema
3. `.ai/scripts/scan_repo.sh` - 統一 schema
4. `.ai/scripts/scan_repo.py` - 統一 schema
5. `.ai/scripts/evaluate.sh` - 新增 --check-origin 選項
6. `.ai/docs/evaluate.md` - 更新文檔
7. `.ai/config/audit.schema.json` - 新增 schema 定義
8. `.ai/config/repo_scan.schema.json` - 新增 schema 定義

---

## Components and Interfaces

### 1. 統一 repo_scan.json schema

```json
{
  "timestamp_utc": "2025-12-19T12:00:00Z",
  "root": {
    "path": "/path/to/repo",
    "branch": "feat/example",
    "head": "abc123",
    "clean": true,
    "status": "## feat/example"
  },
  "submodules": [
    {
      "path": "backend",
      "exists": true,
      "clean": true,
      "branch": "main",
      "head": "def456",
      "kind": "go",
      "has_workflows": true
    }
  ],
  "presence": {
    "gitmodules": false,
    "claude_md": true,
    "agents_md": true
  },
  "ai_config": {
    "exists": true,
    "workflow_yaml": true,
    "scripts_dir": true
  }
}
```

### 2. 統一 audit.json schema

```json
{
  "timestamp_utc": "2025-12-19T12:00:00Z",
  "root": "/path/to/repo",
  "findings": [
    {
      "id": "F001",
      "severity": "P1",
      "type": "dirty_worktree",
      "path": "/path/to/repo",
      "message": "Working tree has uncommitted changes"
    }
  ],
  "summary": {
    "p0": 0,
    "p1": 1,
    "p2": 0,
    "total": 1
  }
}
```

### 3. Offline Gate 移除網路操作

```bash
# audit_project.sh 變更

# 舊：在 Offline Gate 中執行
# for sm in scan["submodules"]:
#     rc = subprocess.call(["git","-C",p,"fetch","-q","origin",sha,"--depth=1"], ...)
#     if rc != 0:
#         add(findings, "P0", "submodule pinned sha not found on origin", ...)

# 新：移到獨立函數，只在 --check-origin 時執行
def check_submodule_origin(submodules, findings):
    """Check if submodule pinned sha exists on origin. Requires network."""
    for sm in submodules:
        # ... git fetch logic ...
        pass

# evaluate.sh 新增選項
# --check-origin: 執行 submodule origin 檢查（需要網路）
```

### 4. dirty_worktree 統一為 P1

```python
# audit_project.py 和 audit_project.sh 都改為：
if not is_clean:
    findings.append({
        "severity": "P1",  # 統一為 P1，不是 P0
        "type": "dirty_worktree",
        "message": "Working tree has uncommitted changes"
    })
```

---

## Data Models

### audit.schema.json

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["root", "findings", "summary"],
  "properties": {
    "timestamp_utc": { "type": "string" },
    "root": { "type": "string" },
    "findings": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["severity", "type", "message"],
        "properties": {
          "id": { "type": "string" },
          "severity": { "enum": ["P0", "P1", "P2"] },
          "type": { "type": "string" },
          "path": { "type": "string" },
          "message": { "type": "string" }
        }
      }
    },
    "summary": {
      "type": "object",
      "required": ["p0", "p1", "p2", "total"],
      "properties": {
        "p0": { "type": "integer" },
        "p1": { "type": "integer" },
        "p2": { "type": "integer" },
        "total": { "type": "integer" }
      }
    }
  }
}
```

---

## Correctness Properties

### Property 1: Offline Gate 無網路操作

*For any* repo (with or without submodules), running `evaluate.sh --offline` SHALL NOT perform any network operations.

**Validates: Requirements 1.1, 1.2**

### Property 2: audit.json schema 一致性

*For any* execution of audit_project.sh or audit_project.py, the output SHALL validate against audit.schema.json.

**Validates: Requirements 2.1, 2.2, 2.3**

### Property 3: dirty_worktree severity 一致性

*For any* dirty worktree detection, the severity SHALL be P1 regardless of which script produces it.

**Validates: Requirements 3.1, 3.2**

### Property 4: repo_scan.json schema 一致性

*For any* execution of scan_repo.sh or scan_repo.py, the output SHALL validate against repo_scan.schema.json.

**Validates: Requirements 4.1, 4.2, 4.3**

---

## Error Handling

| 錯誤情況 | 處理方式 |
|----------|----------|
| --check-origin 無網路 | FAIL（使用者明確要求檢查） |
| --check-origin 無 submodules | SKIP（不適用） |
| --check-origin SHA 不存在 | FAIL（配置問題） |
| audit.json schema 不符 | FAIL |
| repo_scan.json schema 不符 | FAIL |

---

## Testing Strategy

### Unit Tests

1. 測試 scan_repo.sh 和 scan_repo.py 產出相同 schema
2. 測試 audit_project.sh 和 audit_project.py 產出相同 schema
3. 測試 dirty_worktree 在兩個版本都是 P1
4. 測試 Offline Gate 不執行 git fetch

### Property Tests

1. audit.json 必須符合 schema
2. repo_scan.json 必須符合 schema
