# Agent Orchestration

## 平行執行

彼此獨立的工具呼叫（讀檔、搜尋、互不依賴的指令）放在同一則訊息一起發，有依賴才依序。

subagent 另當別論：它只看得到你寫給它的說明，回來只剩摘要，花費也比一般工具呼叫高得多。要讀很多檔才答得出來、或彼此獨立又夠大的工作，才考慮交給 subagent；開之前先跟 owner 說用途與粗估成本，owner 同意才開，不一次開一整波。

無人值守的 run（`awkit run`）沒有人可以問：只用流程自己的 subagent（`.claude/agents/`，何時用由 `principal-workflow` skill 決定），不另開。

## 審計

審計類任務遵循 `audit-checklist.md`；有用 subagent 時，不讓它們全部只讀同一層程式碼。

## 交付程式碼變更時

回報改了哪些檔、跑了哪些檢查、結果如何；程式碼本身看 diff，不必在回報裡整份重貼。動到伺服器端時另外說明：

- 驗證規則
- 儲存影響（schema、migration）
- 冪等策略（request id、條件更新、conflict 處理）
- 背景 worker 是否涉入

驗證步驟：

- 跑那個 repo 在 `.ai/config.yaml` 的 `verify` 指令（建置、測試），全部通過才算完成。
- 契約變更：先重新產生兩端的程式碼，再跑兩端的測試；兩端都用產生的程式碼，不手寫 schema。
