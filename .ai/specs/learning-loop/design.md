# Learning Loop 設計:記錄 → 蒸餾 → 注入 → 驗證

> 目標:讓 AWK「犯過的錯,之後不再犯」。把散落的 feedback 記錄升級成一條閉環學習鏈路,
> 教訓(lesson)成為可提交、可驗證、可淘汰、可升級為硬閘門的一等資產。

## 0. 調研結論(我們借鑑什麼)

| 來源 | 借鑑的機制 | 落到本設計的哪裡 |
|---|---|---|
| **ExpeL** (2023) | 失敗軌跡蒸餾成共享 rule library;規則有 ADD/UPVOTE/DOWNVOTE/EDIT 操作與上限 | 蒸餾器的輸出操作集、curator 的維護操作 |
| **ReasoningBank** (Google, 2025) | 三段式 memory item(title / description / content=可執行的檢查與約束);成功與失敗都蒸餾;**檢索 k=1 最優、k=4 反而讓成功率從 49.7% 掉到 44.4%** | lesson schema;注入端 top-k 極小(≤3)與字元預算 |
| **Reflexion** (2023) | trial-level 口頭自我批評,注入下次嘗試 | AWK 既有的 retry 時 PREVIOUS REVIEW FEEDBACK 注入 —— 定位為「短期記憶」,與 lessons(長期記憶)分層,互不取代 |
| **Voyager / GRASP** (2023/2026) | skill library 需要結構、評估與**准入閘門**(有量測改進才收錄,防 regression) | lesson 狀態機 candidate→active→proven 的晉升條件 |
| **Generative Agents** (2023) | 檢索評分 = recency × importance × relevance | 注入端評分公式與淘汰分數 |
| **Mem0** (2024) | 兩階段:抽取 → 對既有記憶 ADD/UPDATE/DELETE/NOOP 的 curator | 蒸餾後的 Go curator 流程 |
| **Agentic Code Review 生產實務**(addyosmani;AWK 已借鑑其 anti-rationalization) | 只有高信心規則進 CI gate,雜訊規則維持 advisory | 升級階梯:prompt lesson → 人審 → 硬閘門 |
| **"Honest Lying" 記憶幻覺研究** (2026) | 反面教材:自我反思型記憶會被 agent 杜撰 | 證據欄位由 Go 填寫(issue/PR 編號),LLM 不得產生;fail-closed 解析 |

### Understand-Anything 的優勢如何複用(已整合,現在借它的骨架)

U-A 的五個機制與本鏈路一一對應:

1. **可提交的 JSON 產物**(`knowledge-graph.json`)→ `lessons.json` 同樣可提交:團隊共享學習成果、diff 可審、
   reset 不丟失。這一點勝過 hermes(其記憶是私有 runtime 狀態,無法 code review)。
2. **指紋式增量更新** → (a) feedback 高水位線:蒸餾只處理上次之後的新記錄;(b) lesson fingerprint 去重。
3. **確定性層(tree-sitter)+ LLM 層分工** → 本鏈路同構:記錄/注入/驗證全為確定性 Go,
   只有蒸餾用 LLM,且其輸出被 Go fail-closed 驗證。
4. **graph-reviewer(產物完整性驗證 agent)** → curator 驗證 pass:schema、證據、重複、矛盾。
5. **知識圖譜作為錨點(最大加成)** → lesson 帶 `scope`(路徑/模組);注入時用 ticket scope 經
   knowledge graph 鄰域擴展(含 dependents)命中 lessons。教訓被「標注在程式碼地圖上」:
   改 A 模組時,自動帶出 A 及其依賴者的歷史坑。直接重用 `internal/worker/knowledgegraph.go` 的相關性引擎。

---

## 1. 資料模型

### `.ai/state/lessons.json`(可提交;機器權威來源)

```json
{
  "version": 1,
  "watermark": { "feedback_line": 1234, "updated_at": "2026-07-03T12:00:00Z" },
  "lessons": [
    {
      "id": "L-012",
      "title": "config 結構變更必須同步 schema 與註解",
      "description": "workflow.yaml 加欄位時漏改 workflow.schema.json 導致 validate 失敗",
      "content": "- 修改 internal/analyzer/config.go 的結構時,檢查 .ai/config/workflow.schema.json 是否需同步\n- 同步更新 workflow.yaml 的說明註解",
      "kind": "pitfall",
      "categories": ["config", "schema"],
      "scope": ["internal/analyzer/", ".ai/config/"],
      "fingerprint": "a1b2c3d4",
      "status": "active",
      "hits": 3,
      "misses": 0,
      "evidence": [ { "issue": 142, "pr": 88, "type": "changes_requested" } ],
      "created_at": "2026-06-01T00:00:00Z",
      "last_hit_at": "2026-07-01T00:00:00Z",
      "source": "distiller"
    }
  ]
}
```

- `kind`: `pitfall`(失敗教訓)| `strategy`(成功策略,Phase C 才啟用)。
- `content` 遵循 ReasoningBank 三段式精神:可執行的檢查/約束條列,不是敘事。
- `evidence` **只由 Go 填寫**(來自 feedback 記錄),蒸餾 LLM 無權產生 —— 防記憶幻覺。
- 上限:`active`+`proven` ≤ 30;`candidate` ≤ 10。
- `fingerprint` = sha1(正規化的 categories + scope + title 關鍵詞),用於去重與重犯偵測。

### 歸因記錄:`.ai/runs/issue-N/injected_lessons.json`

每次 dispatch 由 Go 寫下本次實際注入的 lesson IDs 與當時狀態 —— 驗證步的基礎。

---

## 2. 四步鏈路

### Step 1 記錄(現有機制補強,全確定性)

已有:`review_feedback.jsonl`(拒絕)、`failure_history.jsonl`、trace 事件、severity-consistency 類別、token 成本。

補強三點:
1. **成功也記錄**(ReasoningBank):merged 時追加一筆 `outcome: approved` 條目(score、第幾次嘗試)。
   落點:`submit.go` merge 成功路徑,與現有 `RecordFeedback` 對稱。
2. **feedback 條目附 `paths`**:changed files top-N,取自 PR diff
   (重用 phase-2 的 `fetchPRDiff` + jittest 的 `parseDiffFiles`,零新輪子)。教訓因此能錨定 scope。
3. 條目附 repo 名與 attempt 序號(欄位已存在,補齊填寫)。

### Step 2 蒸餾(新;唯一的 LLM 步驟;fail-closed)

**觸發**:review 到達 `changes_requested` / `review_blocked` 終態後,由 Principal main-loop 呼叫
`awkit lessons distill`(同步、單次 LLM call、60s timeout;失敗僅警告不阻塞主流程)。
也可手動/批次:`awkit lessons distill --all`(從 watermark 起補處理)。

**LLM 合約**(比照 secondary reviewer 的嚴格格式):
- 輸入:新 feedback 條目(rejection 原因、review body 節選、paths)+ 現有 lessons 的「id+title+fingerprint」清單(省 token)。
- 輸出(Go 解析,格式錯誤即丟棄整筆):

```
DECISION: MATCH L-012 | NEW | NOOP
TITLE: ...            (NEW 時必填)
DESCRIPTION: ...
CONTENT:
- ...
CATEGORIES: config, schema
SCOPE: internal/analyzer/, .ai/config/
```

對應 Mem0 的 ADD/UPDATE/NOOP 與 ExpeL 的 UPVOTE(`MATCH` = 該 lesson `hits+1`、evidence 追加)。

**Go curator(確定性,緊接每次蒸餾後)**:
1. schema 驗證;`SCOPE` 必須是 repo 內真實存在的路徑前綴(防幻覺)。
2. 去重:fingerprint 完全相同 → 轉 MATCH;token overlap ≥ 0.7(重用 evidence.go 的 fuzzy 思路)→ 合併。
3. 淘汰:超上限時逐出分數最低者。
   `score = hits × e^(−λ·days_since_last_hit) − misses`(λ 取 90 天半衰;Generative Agents 式 recency×importance)。
4. 新教訓一律進 `candidate` 狀態(GRASP:未經驗證不得影響全量)。

### Step 3 注入(升級既有機制,全確定性)

**檢索評分**(無 embedding 依賴;Phase C 可選加 embedding):

```
relevance(L, ticket) = 3·scope_hit + 2·category_hit + 1·norm(hits) + 1·recency
scope_hit: ticket 的 paths/repo tokens(含 knowledge-graph 鄰域擴展)命中 L.scope
```

**預算(關鍵設計,來自 ReasoningBank 的 k=1 證據)**:
- `top_k = 3`、總量 ≤ 800 chars。**寧缺勿濫** —— 注入十條規則牆會讓成功率下降。
- 其中 `candidate` 最多佔 1 條(給新教訓上場驗證的機會 = 受控探索)。

**位置與格式**:`writePromptFile` 中**取代**現有的 `HISTORICAL REVIEW PATTERNS`(10 筆原始重播退役,
`COMMON REJECTION PATTERNS` top-3 類別行保留):

```
LESSONS FROM THIS PROJECT'S REVIEW HISTORY (follow these checks):
1. [L-012] config 結構變更必須同步 schema — 修改 config.go 結構時檢查 workflow.schema.json…
```

**Reviewer 端同樣注入**:`prepare-review` 輸出附上 category ∈ {severity-consistency, criteria-mapping,
assertion} 的 lessons —— reviewer 的格式錯誤(review_blocked 迴圈)同樣是可學習的錯。

**分層原則**:issue 內 retry 的 `PREVIOUS REVIEW FEEDBACK`(Reflexion 層,本 issue 的短期記憶)保留不動;
lessons 是跨 issue 的長期記憶。兩層各司其職。

### Step 4 驗證(新;兩個層次,全確定性)

**產物層**(每次蒸餾後):curator 的 schema/證據/去重/上限檢查(見 Step 2)。
可選:每 20 次蒸餾跑一次「curator LLM pass」檢查教訓間矛盾與過時 —— 對應 U-A 的 graph-reviewer。

**效果層(閉環,這是多數系統缺的一步)**:
1. 歸因:dispatch 時寫 `injected_lessons.json`(Step 1 已述)。
2. 結算:issue 的 PR 到達終態時:
   - **miss**:lesson L 曾注入,且本次 rejection 的 fingerprint/category 再次命中 L 的 pattern → `L.misses+1`(教訓沒防住)。
   - **hit**:L 曾注入、最終 merged 且無同 pattern rejection → `L.hits+1`(弱歸因,僅用於狀態轉移,不做因果聲明)。
3. 狀態機(GRASP 式准入):

```
candidate --(hits≥2, misses=0)--> active --(hits≥5, miss_rate<20%)--> proven
misses≥3           → 回爐重蒸餾(strengthen)或 retire
active 且 90 天無 hit → retired(自然衰減)
```

4. 指標:`awkit lessons stats` — per-lesson 重犯率、全域同類別重犯率(近 20 次 vs 整體,
   直接重用 `AnalyzeTrends`)、first-pass approval rate 趨勢。

**升級階梯(「不再犯」的最強保證)**:
`proven` 且 pattern 可機器檢查的教訓 → `awkit lessons promote L-012` 自動開 GitHub issue,
提議固化為:`.ai/rules/` 規則檔 / `audit.custom` 檢查 / escalation trigger / lifecycle hook。
**人審核合併** —— 系統永不自我修改強制層(這是與 hermes 全自主 skill 生成的刻意差異:
prompt 會被 rationalize,閘門不會忘記;而閘門的變更必須過人)。

---

## 3. 設定

```yaml
lessons:
  enabled: true            # false 完全關閉(記錄仍照舊)
  max_active: 30
  inject_top_k: 3
  inject_max_chars: 800
  distiller:
    backend: claude        # claude --print --max-turns 1
    model: sonnet
    timeout_seconds: 60
```

## 4. 失敗模式與對策

| 風險 | 對策 |
|---|---|
| Prompt rot / 規則牆越長越失效 | 硬上限 30 + 注入 k=3/800 chars + recency 衰減淘汰(ReasoningBank 實證) |
| 記憶幻覺(LLM 杜撰教訓) | evidence 由 Go 填寫;SCOPE 必須是真實路徑;fail-closed 解析 |
| 教訓互相矛盾 | fingerprint/overlap 合併;curator pass;衝突保留 hits 高者 |
| 弱歸因誤判(hit/miss 非因果) | 計數只驅動狀態機;固化為閘門必須人審 |
| 成本 | 每 rejection 一次 max-turns=1 呼叫(與一次 secondary review 同級);可整體關閉 |
| 教訓措辭導致 agent 過度迴避 | curator 要求措辭為「檢查/約束」而非「禁止」;pitfall 必附可執行檢查 |

## 5. 分階段實作

- **Phase A(純 Go,無新 LLM 呼叫;1 個 PR 的量)**
  `lessons.json` schema + `awkit lessons list|add|stats` + 注入端改造(取代 10 筆重播)
  + `injected_lessons.json` 歸因 + hit/miss 結算 + 淘汰。
  蒸餾暫由 post-mortem 技能(Principal)驅動:`awkit lessons add --from-issue N --title ... --content ...`。
- **Phase B(LLM 蒸餾器;1 個 PR)**
  `awkit lessons distill` + 嚴格輸出合約 + curator 操作集 + watermark 增量 + main-loop 掛載點。
- **Phase C(閉環強化)**
  狀態機自動晉升/退役 + promotion 階梯(自動開 issue)+ reviewer 端注入 + `strategy` 型教訓
  +(可選)embedding 檢索與 curator LLM pass。

## 6. 與既有機制的關係

- `FormatFeedbackForPrompt`(10 筆重播)→ 由 lessons 注入取代後**退役**。
- `TopCategories` 一行摘要 → 保留(便宜且與 lessons 互補)。
- post-mortem 技能 → Phase A 的蒸餾入口;Phase B 後改為覆核入口。
- knowledge graph 注入(`worker.knowledge_graph`)→ lessons 的 scope 相關性引擎直接重用其 token 匹配;
  兩者在 prompt 中相鄰呈現:「這是你要動的地圖 + 這張地圖上的歷史坑」。
- severity/evidence 閘門 → 升級階梯的終點範本:教訓固化後就是這一類 Go 閘門。
