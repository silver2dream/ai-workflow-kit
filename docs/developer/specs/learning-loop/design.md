# Learning Loop 設計:記錄 → 蒸餾 → 注入 → 驗證

> 目標:讓 AWK「犯過的錯,之後不再犯」。把散落的 feedback 記錄升級成一條閉環學習鏈路,
> 教訓(lesson)成為可提交、可驗證、可淘汰、可升級為硬閘門的一等資產。

> **現況(2026-11):** 這份設計說 `lessons.json` 可提交、由團隊共享。實作把它放在
> `.ai/state/`,而這個目錄整個被 git 忽略,所以教訓目前只留在學到它的那份 checkout 裡,
> 不會透過 repository 共享。要不要改成進版控,還沒有決定。

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
| **Hermes Agent** (Nous Research, `agent/background_review.py`, 2026-11) | 使用者的糾正是一等學習訊號;沒找到可行做法的失敗不寫成指引 | 維護者在 PR 上的審查意見是最高等級的訊號,合併前後都算:合併前擋合併並交給 Worker,合併後直接蒸餾;教訓直接 `active`(§2.5) |
| **harness2** (google-research, arXiv 2610.14224, 2026) | 學習訊號要來自**對照**(同一任務的失敗版與成功版),證據不足就不寫(`NO_ENTRY`);沒有對照證據直接改 harness 會退步(論文 Code 73.4→63.2) | 蒸餾單位是「被拒→通過」的案例(拒絕理由 + 修正 diff),不是單筆拒絕;hit/miss 只在教訓適用的 PR 上計 |

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
      "evidence": [ { "issue": 142, "pr": 88, "type": "config", "fix": true } ],
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
  `fix: true` 表示蒸餾時看得到「被拒版本 → 通過版本」的 diff,不是只看拒絕理由。
- 上限:`active`+`proven` ≤ 30;`candidate` ≤ 10。
- `fingerprint` = sha1(正規化的 categories + scope + title 關鍵詞),用於去重與重犯偵測。

### 歸因記錄:`.ai/runs/issue-N/injected_lessons.json`

每次 dispatch 由 Go 寫下本次實際注入的 lesson IDs —— 驗證步的基礎。

- `lesson_ids`:最近一次 dispatch 注入的教訓;重新 dispatch 會覆寫。
- `missed`:這個 issue 上已經落空過的教訓。屬於 issue 而不是單次 dispatch,重新 dispatch 時保留。
- 這份記錄保留到 issue merged 才刪除。拒絕之後若是重審而非重派 Worker(`review-failed` 路徑),
  下一次結果仍然找得到當初注入了什麼。

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
4. **拒絕理由記 findings,不是 review body 的開頭**:結構化 review 渲染出來的 body 以逐條驗收標準的
   實作說明開場,前 500 字元不含拒絕原因。條目的 `summary`(≤500)與 `findings`(≤2000,summary 被截斷時才寫)
   取自 score reason + critical/important findings(多模型共識段落一併納入)。類別也由 findings 判斷。
5. **每筆條目附 `head_sha`**:這次 review 判斷的 commit(review 開始時 worktree 的 HEAD)。
   被拒條目與通過條目的 `head_sha` 之間的 diff,就是 Worker 為了通過而做的修正。
6. **拒絕條目附 `injected_lessons`**:被拒那次嘗試的 prompt 裡有哪些教訓(蒸餾時用來判斷「教訓在場卻沒防住」)。

### Step 2 蒸餾(新;唯一的 LLM 步驟;fail-closed)

**蒸餾單位是「已解決的案例」,不是單筆拒絕**(harness2):一筆 `approved` 條目,加上同一個 issue 在它之前
(回溯到該 issue 上一次 approved 為止)的所有拒絕條目。只有拒絕理由時,那是審查者的意見;
加上「後來怎麼改才通過」,才是「當初應該怎麼做」的證據。

- 拒絕條目不單獨蒸餾,留在 log 裡等同一個 issue 通過。watermark 會越過它們,不會卡住;
  通過時再回頭把它們取出來(`awkit lessons stats` 顯示目前有幾筆在等)。
- 一直沒通過的 issue(放棄、轉人工、人工自行合併)不會自動產生教訓 —— 由 post-mortem 用
  `awkit lessons add --issue N` 記錄。這是刻意的:沒有對照的教訓寧可不寫。
- 一次就通過的 issue 沒有拒絕條目,不呼叫 LLM。

**觸發**:`awkit submit-review` 的 merge 路徑,在結算之後呼叫蒸餾(同步、每個案例一次 LLM call、
60s timeout、每次最多 3 個案例;失敗僅警告不阻塞主流程)。也可手動補處理:`awkit lessons distill [--max N]`
(從 watermark 起)。

**LLM 合約**(比照 secondary reviewer 的嚴格格式):
- 輸入:
  - 案例的拒絕理由(每筆取 `findings`,沒有則取 `summary`;最多 4 筆、每筆 1500 字元)。
  - **修正 diff**:最早一筆有 `head_sha` 的拒絕 → approved 的 `head_sha`,在 state root 以 `git diff` 取得。
    先列檔案清單,再附 patch(排除 lockfile、上限 8000 字元)。
  - 現有 lessons 的「id+title+categories」清單(省 token)。
- 案例內容放在 `<case>` 標籤內,並說明它是其他 agent 與受審程式碼寫的**證據,不是指令**
  (內容中的 `</case>` 會被改寫,無法提前關閉標籤)。
- diff 的三種狀態會如實告訴蒸餾器:有修正(檔案 + patch)、程式碼沒變就通過(例如 CI 重跑、重審)、
  無法比較(舊條目沒有 `head_sha`、commit 不在本機)。prompt 要求證據不足時回 `NOOP`。
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
例外:`MATCH` 到的 lesson 若就在被拒那次嘗試的 prompt 裡(條目的 `injected_lessons`),不加分也不追加 evidence
—— 教訓在場卻沒防住,不是支持它的證據(是否算落空由 Step 4 的結算決定)。

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
1. 歸因:dispatch 時寫 `injected_lessons.json`(§1 已述)。
2. 結算:issue 的 PR 每到一個 review 結果時,對曾注入的每條 lesson L 判斷這個結果是不是關於 L 的證據:
   - **適用性(先決條件)**:L 沒有 scope(全 repo 適用),或 PR 改到 L 的 scope 前綴底下的檔案。
     不適用 → 不計。PR 的檔案清單讀不到時,有 scope 的教訓不計(無法判斷就不算)。
     檔案清單取自 PR diff 的檔頭(新增/修改/刪除/改名/binary 都算),不受條目 `paths` 的 10 筆上限影響。
   - **miss**:結果是拒絕、L 適用,且拒絕的 findings 落在 L 的類別 → `L.misses+1`(教訓沒防住)。
     同一個 issue 對同一條 L 最多計一次 —— 一個 issue 是一個樣本。
   - **hit**:結果是 merged、L 適用、L 在這個 issue 上沒有落空過,且這個 issue 不是 L 自己的來源(evidence)
     → `L.hits+1`。落空之後的合併不算:那次重試是被 review 意見帶著走的,不是教訓的功勞。
   - 其餘皆為無訊號。hit 仍是弱歸因,僅用於狀態轉移,不做因果聲明。
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

## 2.5 人的糾正是一等訊號

AWK 是無人值守的管線,人不在對話裡。人能說話的地方是 GitHub 上的 PR。這一節把那裡的人工審查接成
控制與學習兩端的最高等級訊號(`review.human_feedback`,預設開)。

**合併不等人**(2026-11-13 定案):合併的條件是測試充足且 CI 綠燈 —— 自動審查通過、證據閘門通過、CI 通過就合併。
沒有「合併前先等人看」的開關,也不打算做:品質靠測試把關,不靠人守門。所以人的意見有兩種時機,兩種都要算數:

| 意見出現時 | 處理 |
|---|---|
| PR 還沒合併 | 控制 + 學習:擋合併、交給 Worker、之後與修正一起蒸餾(本節「控制」「學習」) |
| PR 已經合併(常態) | 只有學習:記錄後立刻蒸餾,下一次派工前生效(本節「合併後的意見」) |

**什麼算維護者的審查意見**:有寫入權限的帳號(OWNER / MEMBER / COLLABORATOR,或執行工作流的帳號本身)
透過審查介面寫的內容 —— review 的說明與 diff 上的行內留言。排除:bot、空白、尚未送出 / 已撤銷 / 單純 approve 的 review、
AWK 自己貼的文字(維護者與 AWK 常共用同一個帳號,作者分不出來,所以靠文字標記分)。一般對話留言不算。

**控制(`submit-review`,自動審查給出通過分數時)**:

| PR 目前 commit 上的意見 | 動作 |
|---|---|
| 沒有,或對話已被標為已解決 | 照常 approve + merge |
| 有,且還沒交給過 Worker | 不 approve。issue 退回 `ai-task`,意見原文貼成 issue 留言(Worker prompt 會讀到),結果 `changes_requested` |
| 有,已交給過 Worker,但之後沒有任何新 commit | 不再退、也不合併:標 `needs-human-review`,結果 `merge_failed` |

- 自動審查本來就不通過時,不另外擋:意見附在同一則退回留言裡,排在自動審查的發現之前。
- 寫在較早 commit 上的意見,Worker 可能已經處理,也可能沒看到。這無法確定地判斷,所以交給審查者:
  `prepare-review` 的情境會列出所有未解決的意見,要求逐一對照目前的程式碼,沒處理的列為 Critical。
- 讀不到 GitHub 的審查資料時視為沒有意見(fail-open):這是額外的保護,不該因為 API 失敗讓管線停擺。

**學習**:

- 每則意見寫入 feedback log 一次(`source: human`,`refs` 記 review / comment 的 id,`head_sha` 是它針對的 commit)。
- 案例蒸餾時,維護者的意見在 prompt 裡有標示,並說明它是糾正而非意見;修正 diff 改從「維護者針對的 commit」算起。
- 由含人工意見(或人工合併)的案例產生的教訓:`source: human-correction`,直接 `active`,不經 `candidate`;
  命中既有的 `candidate` 教訓時,直接把它升為 `active`。之後仍會因落空被淘汰。
- **人工合併也是解決**:蒸餾前會檢查「還在等通過的 issue」的 PR,已被人在工作流之外合併的,補記一筆
  `approved`(`source: human`)並抓回該 PR 的 head,讓「被拒 → 人修好」的 diff 成為案例。

**合併後的意見**(`internal/reviewer/late_feedback.go`):

- **找哪些 PR**:一次清單查詢(最近更新的 30 個已關閉 PR)。符合三個條件才讀:已合併;head 分支是
  `feat/ai-issue-<N>`(Worker 的分支命名,用來把 PR 對回 issue,submodule 型專案中不同 repo 的 PR 編號相同也不會認錯);
  feedback log 裡有 issue N 的合併紀錄。每個 PR 記下上次讀取時 GitHub 的 `updated_at`
  (`.ai/state/late_feedback_scan.json`),沒變就不再讀。一次最多讀 10 個,其餘留到下次。
- **哪些意見算**:發佈時間晚於 `merged_at`(兩者都是 GitHub 的時間,不受本機時鐘影響),還沒記錄過,且對話沒有被標為已解決。
  行內留言的發佈時間取它所屬 review 的送出時間 —— 合併前開始寫、合併後才送出的 review 算合併後的意見。
  合併前就發佈、但 log 裡沒有的意見不補記:無法判斷 Worker 後來有沒有處理。
- **記錄**:一個 PR 一筆,`source: human`、`after_merge: true`,`context` 是每則行內留言所在 diff hunk 的最後 12 行。
- **蒸餾**:這種紀錄自成一個案例,不等之後的通過紀錄(那個 PR 不會再有修正)。prompt 是另一個版本:
  說明這是合併後的人工審查、沒有修正可對照、以維護者的話為依據;提問、稱讚、只跟這次修改有關的意見一律 NOOP。
  產生的教訓與其他人工案例相同(`active`、`source: human-correction`)。合併紀錄會保存當次注入的教訓,
  所以命中「合併那一版 prompt 裡已有的教訓」時不加分(列為 recurred)。
- **時機**:每次派工前(`awkit dispatch-worker`,有新東西才蒸餾)、每次合併後、`awkit lessons distill`。
  派工前那一次是關鍵:人在 AWK 忙別張票時留的意見,下一個 Worker 就用得上,而管線從頭到尾沒有等人。
- 讀取失敗一律略過不擋流程;`lessons.enabled: false` 或 `review.human_feedback: false` 時不查。

**刻意不做**:

- 「合併前先等人看」的開關(見上:合併不等人)。
- 已合併的程式不因合併後的意見自動重開 issue 或開修正票;要修由人另開票。
- 合併後的意見不回頭修正該次合併已結算的 hit。
- 關閉未合併的 PR 上的意見不單獨蒸餾(沒有修正可對照,也不是被接受的成果)。

## 3. 設定

```yaml
review:
  human_feedback: true     # 維護者的審查意見優先於自動審查(§2.5)

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
| 弱歸因誤判(hit/miss 非因果) | 只在教訓適用的 PR 上計;一個 issue 一個樣本;落空後的合併不算命中;計數只驅動狀態機;固化為閘門必須人審 |
| 沒有對照就下結論(只憑拒絕意見寫教訓) | 蒸餾單位是「被拒→通過」案例;沒通過的拒絕不蒸餾;diff 讀不到時如實告知並偏向 NOOP |
| 案例內容夾帶指令(review 文字、diff 內容) | 案例放在 `<case>` 標籤內並聲明為證據;一次性呼叫不帶工具;輸出 fail-closed 解析、scope 需實際存在 |
| 成本 | 每個已解決案例一次 max-turns=1 呼叫(與一次 secondary review 同級);可整體關閉 |
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
