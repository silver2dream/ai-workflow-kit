# After Compaction

對話接近上限時，Claude Code 會把前面的訊息壓縮成摘要。長時間的執行一定會遇到，不是錯誤。

## 什麼沒有丟

流程的狀態都不在對話裡，壓縮不會動到它們：

- 每張票走到哪一步：GitHub 上 issue 的標籤
- Worker 的結果：`.ai/state/results/`
- 迴圈次數、連續失敗次數、每個 PR 審查過幾次：`.ai/state/`
- 被退回的審查意見：`.ai/state/review_feedback.jsonl`

## Steps

1. 重讀 `.claude/skills/principal-workflow/SKILL.md`：摘要不會逐字留下指令格式。
2. 從主迴圈的第 1 步接著做：`awkit analyze-next --json` 回報的就是現在該做的事，不必回想壓縮前做到哪裡。
3. 需要看全貌時用 `awkit status`，不要自己去讀 `.ai/state/` 下的檔案。

壓縮前正在等的指令（派工、subagent）如果沒看到結果，不要重做：照第 2 步問 `awkit analyze-next`，做完的它不會再排，沒做完的它會排 `check_result`。
