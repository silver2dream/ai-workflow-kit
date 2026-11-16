---
name: post-mortem
description: 工作流程出錯之後的失敗分析：收集證據 → 判定根因 → 評估影響範圍 → 提出復原步驟與預防措施，產出一份報告。Worker 失敗或逾時、審查卡住、合併失敗、awkit run 異常停止之後使用；只讀不改，要修的事另外開票。
allowed-tools: Read, Grep, Glob, Bash
---

# Post-Mortem

流程留下的紀錄以 `awkit status`、`awkit events` 與 `.ai/state/` 為準。這個 skill 固定分析的順序，讓結論站在證據上。

## Read this reference

`references/analyze-failure.md`：證據在哪裡、根因的分類與徵兆、報告範本。

## Workflow

1. **界定**：什麼失敗、什麼時候、預期與實際各是什麼。問不到就從 `awkit status` 與最近的事件推定，並在報告裡標明是推定。
2. **收集證據**：照 reference 的清單收集；錯誤訊息逐字記下，不改寫。
3. **根因**：歸到一個類別（設定、程式、環境、外部），寫出元件、說明、把握程度，以及支持它的那一筆證據。把握不高時，列出其他假設與各自要查什麼才能排除。
4. **影響範圍**：哪些 issue、PR、分支受影響；有沒有做到一半的變更已經進去；流程現在是不是卡住。
5. **復原**：給出確切的指令與順序，以及怎麼確認復原成功。只列出，不執行。
6. **預防**：這一類失敗怎樣才不會再發生；每一條寫類型、要改的位置、優先度。
7. 用 reference 的範本回報。

## Guardrails

- 分析期間不改任何檔案、標籤或分支，所以 `awkit run` 還在跑的時候也可以做。
- 先有證據才下結論；沒有證據支持的寫成假設。
- 復原步驟做不做由 owner 決定。要修程式或設定的事寫成票券（格式見 `.claude/skills/principal-workflow/references/ticket.md`），不在這裡動手。
- 根因不確定也要交報告，把還不知道什麼寫清楚。
