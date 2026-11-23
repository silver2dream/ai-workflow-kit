# Agent Engine — Tasks

設計：[`design.md`](design.md)　需求：[`requirements.md`](requirements.md)

- 每個階段是一或多個 pull request；一個 pull request 完成一組可以單獨上線的任務，含測試與文件（R0）。
- 每個階段結束前，在專案外的測試專案實測並把結果寫進 pull request（`design.md` §15）。
- 「相依」指同一份 spec 裡必須先完成的任務。

---

## P0 在現有架構上先補安全

- [x] 0.1 `internal/guard`：判定模型與事件
  - 相依：—
  - `Verdict`、`guardrail_verdict` 事件類型；沒有事件串流的程序（Worker 單獨執行、hook）寫到 `.ai/state/events/guard.jsonl`
  - P0 的檢查是各自執行點上的函式，各自 fail closed（`Prepare` 遇到沒有信任等級的片段就擋、hook 讀不懂的呼叫就擋、CLI 太舊就不執行）；§5.7 的共用 `Check` 介面移到 4.1，組 pipeline 時建立
  - 需求：R0、R13
  - 完成：#276
- [x] 0.2 規則層注入偵測與紅隊語料
  - 相依：0.1
  - 有版本的規則（`injection-rules/1`，中英文、簡繁）；清理（零寬、雙向控制字元、HTML 註解、長度）；`internal/guard/testdata/injection/` 的語料與召回率／誤報率測試
  - 完成時的語料結果：block 19/19、flag 13/13、正常內容誤報 0/20；已知漏網 5 則記在 `known-misses`，留給 P4 的分類器
  - 需求：R9
  - 完成：#276
- [x] 0.3 執行者 prompt 的來源標記與資料區塊
  - 相依：0.2
  - Worker 的 prompt 由片段組成，每段帶 source／author／trust：工單（owner）、程式碼地圖（repo）、審查意見與教訓（agent）、統計（system）；不可信片段以資料區塊放入並過規則層，明顯的注入擋下（結果 `blocked`）
  - PR 上的意見：有寫入權限者的以 agent 等級放入，因為審查 agent 用同一個帳號發言，分不出是人寫的；沒有寫入權限者的不放。依作者細分的信任等級移到 1.3
  - 可疑片段由 Worker 在回答最後一行 `AWK_SUSPICIOUS:` 回報，寫 flag 判定；現行 Worker 沒有結果 schema，結構化的 `suspicious` 欄位移到 1.2
  - 需求：R9
  - 完成：#276
- [ ] 0.4 指令政策 hook
  - 相依：0.1
  - `awkit guard tool --role <role>`（PreToolUse；exit 2＋原因；`onFailure: block`）；組合指令的解析；P0 的政策：主控 session 是主控、審查、解衝突需求的聯集，禁止 `gh pr merge`、推到整合與發版分支、讀家目錄的憑證、列印環境變數、對白名單以外主機的下載；`awkit generate` 把 hook 寫進 `settings.local.json`
  - 需求：R10
- [ ] 0.5 Claude 執行者的角色設定與環境變數清理
  - 相依：0.4
  - Claude 執行者帶 engine 產生的 `--settings`、`--setting-sources`、`--permission-mode dontAsk`、允許與禁止清單、hook；兩種執行者都以清過的環境變數啟動（不給 gh 憑證）
  - 需求：R10
- [ ] 0.6 對外文字掃描
  - 相依：0.1
  - awkit 送到 GitHub 的 PR 描述、留言、issue 內容先掃密鑰與個資；觸發就不送並轉人
  - 需求：R11
- [ ] 0.7 教訓的篩檢關卡
  - 相依：0.2
  - `quarantined` 狀態；第一次注入前過規則層與內容政策；候選教訓以 agent 等級的資料區塊注入並標「未驗證」；工作台列出隔離中的教訓（核可、刪除）
  - 需求：R12
- [ ] 0.8 可疑模式的 diff 檢查（只記錄）
  - 相依：0.1
  - 新增對外連線、動態執行、改 CI、新增相依套件、二進位檔、大量刪除；寫判定事件；P2 起接到風險規則
  - 需求：R11

## P1 Engine 取代 LLM 主控（行為等同）

- [ ] 1.1 `internal/engine`：線的狀態機、事件、快照與重建
  - 相依：0.1
  - `design.md` §6 的狀態與轉換；事件日誌與快照；從事件重建快照；GitHub 標籤投影
  - 需求：R1、R13、R14
- [ ] 1.2 Agent runtime 與角色設定
  - 相依：0.5
  - 契約 `Run(ctx, envelope) → Result`；Claude Code 與 Codex 轉接器（§5.5 的展開表）；能力探測；結構化輸出（`--json-schema`／`--output-schema`）；預算；展開結果寫事件；角色設定的預設值與上限
  - 需求：R2
- [ ] 1.3 Context builder 與任務信封
  - 相依：0.3、1.2
  - 每個角色的最少 context 清單；片段信任等級的推導；信封存檔（遮蔽後）
  - 需求：R9
- [ ] 1.4 審查拆成獨立程序
  - 相依：1.2、1.3
  - engine 做 prepare（含自己跑測試）→ reviewer（唯讀、無 Bash、預設不同模型）→ 證據關卡核對 engine 的測試結果 → 發表審查；合併從 `submit-review` 移出（P1 先由 engine 直接合併，P2 接合併佇列）
  - 需求：R2、R11
- [ ] 1.5 解衝突拆成獨立程序
  - 相依：1.2
  - resolver 的結果 schema；engine 檢查沒有衝突標記、改動只在衝突檔案；標籤由 engine 改
  - 需求：R2、R11
- [ ] 1.6 Engine 主迴圈、鎖、租約、對帳、停止
  - 相依：1.1、1.4、1.5
  - 事件驅動的迴圈（平行上限固定為 1）；單一實例鎖；租約與心跳；當掉重開的接回；GitHub 對帳與人的動作轉指令；`awkit stop` 兩種停止；`awkit detach` 啟動
  - 需求：R1、R14
- [ ] 1.7 影子模式與切換
  - 相依：1.6
  - `AWKIT_ENGINE=shadow`：engine 與現行主控對同一狀態算決定，逐筆比對寫事件；工作台與 `awkit status` 顯示分歧；測試專案跑完一份規劃無分歧後，預設改為 engine，保留 `AWKIT_ENGINE=principal` 一個版本；主控相關指令的 deprecation 事件
  - 需求：R1
- [ ] 1.8 工作台與 `awkit status` 讀 engine 的狀態
  - 相依：1.6
  - 執行頁改顯示線（P1 只有一條）；串流的 `engine` 區塊
  - 需求：R13

## P2 平行與人的控制

- [ ] 2.1 工單 schema v2：`paths`、`risk`
  - 相依：—
  - `internal/ticket` schema 2 與 schema 1 的相容路徑（事件、移除版本）；議會規劃角色輸出 `paths`、`risk` 並驗證；核可畫面顯示與修改；`plan approve` 寫進 issue
  - 需求：R6
- [ ] 2.2 排程器
  - 相依：1.6、2.1
  - `engine.max_parallel`、`engine.max_per_backend`；依賴的拓撲順序與環的偵測；路徑避撞；人的優先與固定組；名額不含等核可與等 CI 的線
  - 需求：R3
- [ ] 2.3 合併佇列
  - 相依：2.2、1.5
  - FIFO 加人的優先；rebase、衝突交給 resolver、重驗（diff 政策與驗證）、push、CI、合併；「先別合併」
  - 需求：R4
- [ ] 2.4 只停那一條線
  - 相依：2.2
  - 移除「任何一張需要人就結束整輪」；每條線與整輪的上限分開
  - 需求：R5
- [ ] 2.5 範圍檢查與高風險關卡
  - 相依：2.1、0.8
  - diff 對 `paths` 的範圍檢查（退回重做）；內建高風險規則與設定的加碼；`awaiting_approval`；工作台的核可畫面
  - 需求：R8、R11
- [ ] 2.6 指令通道與工作台的控制
  - 相依：1.6
  - 收件匣與回覆；`awkit lane …` CLI；工作台的線視圖與每個指令的控制項；補說明以 owner 等級進下一次派工
  - 需求：R7
- [ ] 2.7 預算與異常偵測
  - 相依：2.2
  - 三層預算；用量顯示；異常規則自動暫停那條線
  - 需求：R13、R5

## P3 容器隔離

- [ ] 3.1 映像
  - 相依：1.2
  - 各語言的 Dockerfile 範本、固定版本的 agent CLI；`.ai/agent/Dockerfile` 擴充；以內容雜湊快取；`awkit doctor` 檢查
  - 需求：R10.5
- [ ] 3.2 Egress proxy
  - 相依：0.1
  - `awkit guard egress`：網域白名單（CONNECT／Host）、每次連線寫事件；認證代填（`ANTHROPIC_BASE_URL`、Codex 自訂 provider 位址）；每個專案的 Docker internal 網路
  - 需求：R10.5
- [ ] 3.3 Docker runtime
  - 相依：3.1、3.2
  - 每次派工一個容器（非 root、只掛 worktree 與快取、資源上限、結束即刪）；驗證指令也在容器裡；作業系統憑證儲存區讀取無法代填的認證；`isolation: host` 的提醒
  - 需求：R10.5
- [ ] 3.4 `sbx` runtime 評估
  - 相依：3.3
  - 在 Windows 11 與 Linux 評估 Docker Sandboxes；可用就加為第二種實作；結果寫進 `design.md` §5.8
  - 需求：R10.5
- [ ] 3.5 Windows 實測
  - 相依：3.3
  - worktree 掛載的效能與行尾；必要時改由 engine 同步；訂閱帳號 token 需要的網域清單寫進 `design.md`
  - 需求：R10.5

## P4 完整 guardrail pipeline

- [ ] 4.1 分類器介面與 sidecar
  - 相依：0.2、3.3
  - `Classify` 介面、切段、快取、門檻、fail closed；`judge`、`prompt-guard-2-86m`、`qwen3guard-0.6b` 三種實作（後兩者為 sidecar 容器）
  - §5.7 的 `Check` 介面，把 P0 的各項檢查（注入規則、工具政策、對外文字、diff 訊號、教訓篩檢）接成一條 pipeline
  - 需求：R9.4
- [ ] 4.2 分類器評估與預設
  - 相依：4.1
  - 以紅隊語料評估召回率、誤報率、CPU 延遲、成本；決定預設並寫進 `design.md` §9.7
  - 需求：R9.4
- [ ] 4.3 議會的出處檢查
  - 相依：—
  - 需求與決策的 `sources` 欄位；engine 核對出處存在；核可畫面標出沒有出處的內容
  - 需求：R11
- [ ] 4.4 內容審核
  - 相依：4.1
  - 設定的產品內容路徑經審核模型；flag 或 block
  - 需求：R11
- [ ] 4.5 可設定的政策與判定稽核頁
  - 相依：2.6
  - `guardrails` 設定鍵；工作台的判定紀錄（依線、依規則篩選）
  - 需求：R11、R13
- [ ] 4.6 教訓篩檢接上分類器
  - 相依：0.7、4.1
  - 需求：R12

## P5 多廠商與 LLM 輔助角色

- [ ] 5.1 Provider 介面
  - 相依：1.2
  - 一次性文字呼叫（議會、判讀、分診、翻譯、濃縮）經過 provider；Claude CLI 與至少一家其他廠商的 API
  - 需求：R16
- [ ] 5.2 更多 coding agent 轉接器
  - 相依：1.2、3.3
  - 至少一家（例如 Gemini CLI）；角色設定展開與能力探測
  - 需求：R16
- [ ] 5.3 分診 agent
  - 相依：1.6
  - 錯誤對照表外的錯誤交給 triage；處置清單、次數上限、`skip` 需人確認
  - 需求：R15
- [ ] 5.4 白話指令翻譯
  - 相依：2.6
  - interpreter；工作台的輸入框與確認畫面
  - 需求：R7、R15
- [ ] 5.5 移除主控路徑
  - 相依：1.7（切換後的下一個版本）
  - 刪除 `principal-workflow` skill、`analyze-next`、`check-result`、`stop-workflow` 與相關文件；遷移設定的舊鍵
  - 需求：R1
