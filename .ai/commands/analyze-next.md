分析下一個要執行的任務。

---

## Step 1: 讀取配置

```bash
cat .ai/config/workflow.yaml
```

獲取：
- `specs.base_path` - Spec 路徑
- `specs.active` - 活躍的 spec 列表

## Step 2: 檢查 Pending Issues

```bash
gh issue list --label ai-task --state open --json number,title,labels
```

如果有 pending issues，報告最高優先級的那個。

## Step 3: 讀取 Tasks

對每個 active spec：

```bash
cat <specs.base_path>/<spec_name>/tasks.md
```

找出所有 `- [ ]` 開頭的未完成任務。

## Step 4: 報告

輸出：
1. 當前 pending issues 數量
2. 未完成任務列表
3. 建議下一個要處理的任務（優先級：P0 > P1 > P2，同優先級取編號最小）
