# Skills 與規則

AWK 告訴 agent 的事分成三種檔案，各管一件事：

| 檔案 | 管什麼 | 誰維護 |
|---|---|---|
| `CLAUDE.md`、`AGENTS.md` | 地圖：你是誰、每件事的定義處在哪裡 | `awkit generate` 從 `.ai/config.yaml` 產生，不要手改 |
| `.claude/rules/` | 規則：要求什麼、禁止什麼、要檢查什麼 | `_kit/` 下的隨 AWK 升級；其餘是專案自己的 |
| `.claude/skills/` | 流程：一件工作照什麼順序做 | AWK 附的三個隨升級更新；專案可以加自己的 |

一個主題只在一個地方定義。兩個檔案說法不同時，以定義處為準，並修正另一個。

---

## AWK 附的 Skills

三個都在 `.claude/skills/<名稱>/`，由 `awkit init` 安裝、`awkit upgrade` 更新。

### `principal-workflow`

`awkit run` 啟動的 Principal 用它跑主迴圈：問 `awkit analyze-next` 下一步 → 派工、檢查結果或交付審查 → 再問，直到沒有工作或需要人處理。你不需要自己呼叫它。

它的 `references/ticket.md` 是票券格式的定義處。在 Claude Code 裡說「幫我補一張票」時，agent 會照那份文件寫，再用 `awkit create-task` 建立。一批新工作用 `awkit plan` 規劃。

### `post-mortem`

流程出錯之後的失敗分析，只讀不改：收集證據 → 判定根因 → 評估影響範圍 → 提出復原步驟與預防措施。

```
/post-mortem issue 42 的 Worker 連續失敗三次
```

分析出的教訓可以用 `awkit lessons add` 記進[學習迴圈](configuration.md#lessons---學習迴圈設定)。

### `release-checklist`

發版前的檢查，只檢查、不修不推：建置與測試、工作目錄、版本與變更紀錄、相依套件、CI、與 release 分支的差距，最後給出 GO 或 NO-GO。分支與驗證指令讀 `.ai/config.yaml`。

```
/release-checklist
```

---

## 規則檔

### AWK 附的

`.claude/rules/_kit/git-workflow.md`：分支、commit 格式、PR 的規則，由 `awkit generate` 依設定檔產生。

### Preset 附的起始規則

`godot-go` preset 建立專案時，會寫下三份起始規則，並列在設定檔的 `rules.custom`：

| 檔案 | 範圍 |
|---|---|
| `.claude/rules/backend.md` | `backend/`：Go 的結構、錯誤處理、context 與並發、資料的修改 |
| `.claude/rules/frontend.md` | `frontend/`：全面 C#、Core 與 Godot 的分層、訊號、客戶端的界線 |
| `.claude/rules/testing.md` | 兩邊的測試：每個變更帶測試、三道關卡、失敗的測試怎麼處理 |

它們從寫下的那一刻起就是專案自己的：升級不會動，`awkit init` 再跑一次也不會覆寫。架構定案之後，把決定寫進文件，再回來把這些檢查清單收緊。`generic` preset 不假設技術棧，所以沒有起始規則。

### 寫一份規則檔

1. 建立 `.claude/rules/<名稱>.md`。
2. 把 `<名稱>` 加進 `.ai/config.yaml` 的 `rules.custom`。
3. 執行 `awkit generate`，`AGENTS.md` 就會要求 Worker 讀它。

寫法：

- 只放規則（要求、禁止、檢查清單），不寫現況。需要事實就指向定義處。
- 理由寫在句子裡，讀的人才知道邊界在哪。
- 開頭用 `paths:` 說明它管哪些檔案。Claude Code 只在動到那些檔案時才載入它，Principal 的對話不會被它佔掉：

```markdown
---
paths:
  - "backend/**"
---

# Backend Rules

- 每個錯誤不是處理就是回傳，不吞掉。
```

- 規則與程式碼衝突時以程式碼為準，並回修規則檔。

Worker 預設不能改 `.claude/` 下的任何檔案。要讓一張票修改規則或 skill，在它的 `awk-ticket` 區塊加上 `allow_script_changes: true`。

---

## 寫自己的 Skill

```
.claude/skills/<名稱>/
├── SKILL.md           # 短：步驟與界線
└── references/        # 範本、檢查清單、查閱用的表
```

`SKILL.md` 的開頭說明它是什麼、什麼時候用，Claude Code 靠這段決定要不要載入：

```markdown
---
name: my-skill
description: 做什麼（步驟 → 步驟）。什麼時候用；不適用時改用什麼。
---

# My Skill

權威文件在哪裡，這個 skill 只把步驟串起來。

## Workflow

1. 第一步
2. 第二步

## Guardrails

- 不能做的事，以及為什麼
```

- `name` 與資料夾同名。
- `description` 不要在一行裡寫半形冒號加空格（`: `），那不是合法的 YAML；用全形標點或加引號。
- `SKILL.md` 保持短。長的範本與檢查清單放 `references/`，在 `SKILL.md` 裡點名，需要時才讀。

---

## 更多資源

- [快速開始](getting-started.md)
- [配置說明](configuration.md)
- [架構說明](../developer/architecture.md)
