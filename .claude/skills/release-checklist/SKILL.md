---
name: release-checklist
description: 發版前的檢查：確認整合分支可以合併到 release 分支（建置與測試 → 工作目錄 → 版本與變更紀錄 → 相依套件 → CI → 與 release 分支的差距），給出 GO 或 NO-GO 與每一項的證據。準備開 release PR 或打 tag 之前使用；只檢查，不修、不推、不合併。
allowed-tools: Read, Grep, Glob, Bash
---

# Release Checklist

分支名稱與驗證指令的定義處是 `.ai/config.yaml`（`git.integration_branch`、`git.release_branch`、每個 repo 的 `verify`）；發版的分支規則在 `.claude/rules/_kit/git-workflow.md`。這個 skill 把檢查串起來。

## Read this reference

`references/verify-release.md`：每一項怎麼查、怎麼判定，以及報告範本。

## Workflow

1. 讀 `.ai/config.yaml`，確認目前在整合分支上（`git branch --show-current`）。在別的分支上就先問要檢查哪一個，不自己切換。
2. 照 reference 逐項檢查，每一項記下 PASS、WARN 或 FAIL 與實際輸出：建置與測試、工作目錄、版本與變更紀錄、相依套件、CI、與 release 分支的差距。
3. 判定：有任何一項 FAIL 就是 NO-GO；只有 WARN 是 GO，並列出要注意的事。
4. 用 reference 的範本回報。NO-GO 時每個阻擋項附上怎麼修；GO 時列出接下來的步驟。

## Guardrails

- 只檢查：不修問題、不 commit、不 push、不合併、不打 tag。發現問題就回報，由 owner 決定。
- 判定依據是這一次實際跑出來的輸出。沒跑的檢查寫「沒跑」與原因，不算 PASS。
- 一項失敗不停下，全部查完再回報，owner 才能一次看到所有阻擋項。
