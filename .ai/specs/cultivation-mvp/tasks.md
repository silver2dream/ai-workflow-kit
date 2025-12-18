# 修仙之路 App - MVP 實作任務列表

## 任務概述

本任務列表基於需求文件和設計文件，將 MVP 功能拆分為可執行的開發任務。所有任務都專注於程式碼實作，並採用非同步架構設計。

## 任務列表

- [x] 1. 建立配置系統基礎架構（Firebase Remote Config 整合）





  - 建立配置資料模型類別（GlobalConfig, RealmConfig, SpiritualRootConfig）
  - 實作 ConfigLoader 整合 Firebase Remote Config
  - 實作非同步初始化 `InitializeAsync()`
  - 實作配置版本校驗機制（hash 比對）
  - 實作每日 Config Version Audit（後端比對 Firebase 與本地版本）
  - 實作版本不符自動警報和重新 fetch
  - 實作後端記錄每次 Config 更新的 hash
  - 實作 ConfigService 提供配置查詢方法
  - 建立配置 JSON 檔案範本（本地 fallback）
  - 設定 Firebase Remote Config 預設值
  - 實作 LiveOps 活動控制面板（從 Remote Config 讀取）
  - 實作 A/B 測試邏輯（突破率、靈氣恢復效率等）
  - 建立 liveops_experiments 資料表
  - 實作活動指標上報至 Firebase Analytics（group_id, variant_id）
  - _需求: 8.8, 8.9, 8.10, 8.11_

- [x] 2. 建立後端資料庫架構
  - 建立 MongoDB collections schema（players, destinies, cultivations, chapters, heart_seal_cards, cultivation_history）
  - 建立 PostgreSQL tables（subscriptions, transactions）
  - 設定 Redis key patterns 和 TTL 策略
  - 實作 MongoDB 和 PostgreSQL 客戶端連接
  - 加入 schema_version 和 last_migrated_at 欄位
  - 設定 golang-migrate 工具
  - _需求: 1.6, 2.6, 4.9_

- [x] 3. 實作命格生成系統（前端 - 分離刷數據與 AI 生成）
  - 建立 DestinyModel 資料模型
  - 建立 DestinyCreationView UI 介面（姓名、性別、生日輸入）
  - 建立 DestinyCreationViewModel 處理使用者輸入
  - 實作「① 推演命格」按鈕（呼叫 CalculateDestiny，< 0.5 秒返回）
  - 實作命格數據即時顯示（五行屬性、靈根類型、品質）
  - 實作「② 重新推演」按鈕（無限制、免費、快速刷新）
  - 實作「③ 鎖定此命格入道」按鈕（醒目、承諾點）
  - 實作確認對話框（「天命已定，此生無法更改。是否確認？」）
  - 實作 DestinyService.ConfirmDestinyAsync() 方法（非同步）
  - 實作沉浸式等待動畫（「天機推演中，正在為您撰寫出身...」）
  - 實作輪詢 PollDestinyStatusAsync()
  - 實作命格卡片華麗展示（數據 + AI 生成的故事）
  - 實作「開始修煉」按鈕（進入第一章載入）
  - 實作錯誤處理
  - _需求: 1.1, 1.2, 1.3, 1.4, 1.5_

- [x] 4. 實作命格生成系統（後端 - 分離刷數據與 AI 生成）
  - 建立 destiny module 和 RPC 註冊
  - 實作 CalculateDestiny RPC 處理器（快速、免費、無 AI）
  - 實作五行屬性計算邏輯（根據生辰八字，純數學計算）
  - 實作靈根類型判定邏輯
  - 實作靈根品質判定（天選之人、上品、中品、下品）
  - **不儲存資料，不呼叫 AI，立即返回**
  - 實作 ConfirmDestiny RPC 處理器（非同步，一次性 AI 生成）
  - 實作數據驗證（重新計算五行，確認與提交數據一致）
  - 實作防竄改檢查（五行總和、靈根匹配）
  - 創建非同步生成任務，加入佇列
  - 實作 GetDestinyStatus RPC 處理器（輪詢）
  - 實作 DestinyWorker 背景生成命格故事
  - 整合 AI 服務生成命格描述和出身故事（基於鎖定的數據）
  - 實作 Prompt 模板管理系統（多語言支援）
  - 儲存命格資料至 MongoDB（僅在確認後）
  - 初始化修煉資料（煉氣初期，50 靈氣）
  - 實作 AI 成本追蹤記錄（僅記錄 ConfirmDestiny 的成本）
  - _需求: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6_

- [x] 5. 實作章節閱讀系統（前端 - 非同步架構）
  - 建立 ChapterModel 資料模型
  - 建立 ChapterReadingView UI 介面（境界、靈氣、進度條、章節內容）
  - 建立 ChapterReadingViewModel 處理閱讀邏輯
  - 實作 ChapterService.RequestChapterAsync() 方法
  - 實作輪詢機制 PollChapterStatusAsync()
  - 實作載入動畫和生成進度顯示
  - 實作靈氣檢查和消耗顯示
  - 實作章節內容打字機效果
  - 實作錯誤處理（生成失敗、網路超時）
  - _需求: 2.1, 2.2, 2.3, 2.4, 2.5, 2.7_

- [x] 6. 實作章節閱讀系統（後端 - 非同步架構）



  - 建立 chapter module 和 RPC 註冊
  - 實作 RequestChapter RPC 處理器（快速返回，加入 request_id）
  - 實作 GetChapterStatus RPC 處理器（輪詢）
  - 實作 GenerationQueue 和 GenerationWorker
  - 實作章節快取機制（Redis，prompt_hash + model_version + language）
  - 實作快取重用邏輯（24 小時內相同 hash 直接返回）
  - 整合 AI 服務生成章節內容（帶故事弧線上下文）
  - 實作章節版本鎖（ai_metadata 欄位：ai_model, prompt_version, language, prompt_hash）
  - 實作靈氣扣除邏輯（僅在生成成功後扣除）
  - 實作境界進度增加邏輯
  - 實作冪等性機制（chapter_consumed key + request_id 檢查）
  - 實作重試行為追蹤（記錄重試次數和上次成功時間）
  - 儲存章節資料至 MongoDB（包含完整 ai_metadata）
  - 實作 AI 成本追蹤和 Token 配額管理
  - 實作快取統計納入成本報表
  - _需求: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6_

- [x] 7. 實作靈氣恢復系統（前端）
  - 在 ChapterReadingView 加入「吸收靈息」按鈕（觀看廣告）
  - 在 ChapterReadingView 加入「供奉靈石」按鈕（購買）
  - 實作廣告播放邏輯（整合廣告 SDK）- TODO: 整合真實廣告 SDK
  - 實作靈石購買流程（整合 IAP）- TODO: 整合真實 IAP SDK
  - 整合 NetworkManager 呼叫 RestoreQi RPC
  - 實作每日靈氣自動恢復提示
  - 實作冪等性機制（request_id）
  - _需求: 3.1, 3.2, 3.3, 3.4, 3.5, 3.7_

- [x] 8. 實作靈氣恢復系統（後端）
  - 實作 RestoreQi RPC 處理器
  - 實作廣告觀看驗證邏輯 - TODO: 整合真實廣告驗證
  - 實作購買交易驗證邏輯（IAP 收據驗證 + 簽章）- TODO: 整合真實 IAP 驗證
  - 實作靈氣恢復邏輯（廣告 +10，購買補滿至 50）
  - 建立 qi_transactions collection (MongoDB)
  - 實作 RestoreQi RPC，更新修煉資料至 MongoDB（MongoDB 為遊戲資料 SoT）
  - 實作冪等性檢查（request_id）
  - _需求: 3.1, 3.2, 3.3, 3.4, 3.5, 3.6_

- [ ] 9. 實作境界突破系統（前端 - 非同步架構）
  - 建立 BreakthroughView UI 介面（當前境界、成功率、突破按鈕）
  - 建立 BreakthroughViewModel 處理突破邏輯
  - 實作 BreakthroughService.AttemptBreakthroughAsync() 方法
  - 實作突破動畫（視覺特效）
  - 實作突破結果顯示（成功/失敗）
  - 實作背景輪詢 PollBreakthroughContentAsync()
  - 實作「悟道內容生成中」橫幅（非阻塞）
  - 實作生成完成通知
  - 整合 NetworkManager 呼叫 AttemptBreakthrough 和 GetBreakthroughStatus RPC
  - 實作境界進度達到 100% 時自動觸發突破介面
  - _需求: 4.1, 4.2, 4.3, 4.4, 4.7, 4.8_

- [ ] 10. 實作境界突破系統（後端 - 非同步架構）
  - 實作 AttemptBreakthrough RPC 處理器（快速返回判定結果）
  - 實作 GetBreakthroughStatus RPC 處理器（輪詢）
  - 實作境界進度驗證邏輯（≥100%）
  - 使用 ConfigService 查詢境界配置和成功率
  - 實作突破成功率計算（基礎率 + 靈根加成）
  - 實作隨機判定邏輯
  - 實作成功邏輯：更新境界、重置進度、創建生成任務
  - 實作失敗邏輯：進度重置至 80%
  - 實作 BreakthroughWorker 背景生成悟道章節和心印閃卡
  - 整合 AI 服務生成悟道章節（GPT-4）
  - 整合 AI 圖像生成服務（Stable Diffusion）
  - 儲存突破結果至 MongoDB
  - 實作錯誤處理（生成失敗不影響突破成功）
  - _需求: 4.1, 4.2, 4.3, 4.4, 4.5, 4.6, 4.7, 4.8, 4.9_

- [ ] 11. 實作心印閃卡系統（前端）
  - 建立 HeartSealCardView UI 介面（閃卡圖像、境界名稱、日期）
  - 建立 HeartSealCardViewModel 處理分享邏輯
  - 實作閃卡圖像顯示
  - 實作分享按鈕（儲存至相簿、開啟系統分享）
  - 整合系統分享 API（iOS/Android）
  - 實作分享事件記錄至 Firebase Analytics
  - 實作修行歷程卡功能（多個境界的時間軸）
  - _需求: 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 5.7_


- [ ] 12. 實作 VIP 訂閱系統（前端）
  - 建立 VIP 訂閱介面（價格、特權說明）
  - 整合 App Store / Google Play 訂閱 API
  - 實作訂閱購買流程
  - 實作訂閱狀態檢查（啟動時驗證）
  - 實作 VIP 特權顯示（無廣告、無靈氣限制、專屬光效）
  - 整合 NetworkManager 呼叫 VerifySubscription RPC
  - 實作訂閱到期提醒
  - _需求: 6.1, 6.2, 6.3, 6.4, 6.6_

- [ ] 13. 實作 VIP 訂閱系統（後端）
  - 建立 subscription module 和 RPC 註冊
  - 實作 VerifySubscription RPC 處理器
  - 整合 Apple / Google 收據驗證 API
  - 實作收據簽章驗證機制
  - 實作訂閱資料解析和驗證
  - 儲存訂閱資料至 PostgreSQL
  - 實作 VIP 狀態查詢邏輯（快取至 Redis）
  - 實作訂閱到期檢查和通知
  - _需求: 6.1, 6.2, 6.3, 6.4, 6.5, 6.6, 6.7_

- [ ] 14. 實作 Firebase Analytics 整合
  - 初始化 Firebase SDK
  - 實作 AnalyticsManager 封裝 Firebase Analytics API
  - 實作命格相關事件（destiny_created, destiny_viewed）
  - 實作章節相關事件（chapter_read, chapter_generated）
  - 實作靈氣相關事件（qi_consumed, qi_restored_ad, qi_restored_purchase）
  - 實作突破相關事件（breakthrough_attempt, breakthrough_success, breakthrough_fail）
  - 實作分享相關事件（card_shared, card_generated）
  - 實作訂閱相關事件（subscription_start, subscription_cancel, subscription_renew）
  - 實作新手引導事件（onboarding_destiny_created, onboarding_first_chapter, onboarding_completed）
  - 實作留存漏斗事件（chapter_depth, return_interval, content_preference）
  - 實作離線事件佇列（SQLite）
  - 實作事件批次上報邏輯
  - _需求: 7.1, 7.2, 7.3, 7.4, 7.5, 7.6, 7.7, 7.8, 7.9_

- [ ] 15. 實作多國語言系統
  - 整合 Unity Localization Package
  - 建立本地化表（UI_Strings, Game_Content, System_Messages）
  - 實作語言偵測邏輯（系統語言）
  - 實作語言切換功能
  - 翻譯所有 UI 文案（繁中、簡中、英文）
  - 建立 Game_Content 表（境界、靈根名稱和描述）
  - 實作 ConfigService 使用 Unity Localization 獲取本地化文字
  - 實作 AI Prompt 多語言模板
  - 實作語言切換事件記錄
  - _需求: 8.8, 8.9, 8.10, 8.11_

- [ ] 16. 實作 AI 服務閘道（後端）
  - 建立 AI Gateway 統一介面
  - 實作 OpenAI 客戶端（GPT-4, GPT-4-mini）
  - 實作 Claude 客戶端（Claude 3 Haiku）
  - 實作圖像生成客戶端（Stable Diffusion / Flux）
  - 實作 Prompt 模板管理系統（版本化、多語言）
  - 建立 ai_templates_history 表追蹤模板版本
  - 實作 AI 成本追蹤服務（記錄 model, tokens, cost, prompt_version）
  - 實作 AI 快取策略（prompt_hash + model_version + language）
  - 實作快取命中率統計和 24 小時重用機制
  - 實作 API 限流處理
  - 實作錯誤處理和重試邏輯（3 次重試，記錄重試次數）
  - 實作成本追蹤和日誌記錄
  - 實作 Token 配額管理（每用戶每日上限 + 全域上限）
  - 實作動態模型選擇策略（根據負載和成本自動降級 GPT-4 → GPT-4-mini → Claude Haiku）
  - 實作每日 Token 成本報表生成
  - 實作成本預警機制（Slack / Email 通知）
  - 實作預算閾值檢查（每日與每週）
  - 實作超預算自動降級通知
  - _需求: 1.4, 2.4, 4.5, 5.2_

- [ ] 17. 實作故事弧線系統
  - 擴展 players collection 加入 story_progress, arc_id, arc_state 欄位
  - 建立 StoryArcConfig.json 配置檔案（包含主線/支線型態）
  - 實作 ArcTracker 服務追蹤弧線進度
  - 實作關鍵事件觸發機制
  - 實作敘事上下文管理（師父、宗門、對手、主線節點等）
  - 建立 ArcContext 資料結構
  - 整合 AI 生成時注入 ArcContext 至 Prompt
  - 實作弧線完成特別章節或悟道事件
  - 實作弧線完成事件記錄
  - 實作弧線進度顯示 UI
  - 實作主線/支線切換邏輯
  - _需求: 2.4_

- [ ] 18. 實作新手引導系統（優化前 5 分鐘體驗）
  - 建立 OnboardingProgressView UI 介面
  - 實作入道指引流程（刷命格 → 鎖定命格 → 沉浸式載入第一章）
  - 實作「刷命格」階段 UI（推演、重新推演、鎖定按鈕）
  - 實作命格數據即時顯示和刷新動畫
  - 實作「鎖定命格」確認對話框
  - 實作命格故事生成等待動畫（「天機推演中...」）
  - 實作命格卡片華麗展示（數據 + 故事）
  - 實作「開始修煉」過場動畫（「道途開啟...」、「引氣入體...」）
  - 實作第一章背景生成和進度顯示（綁定 OnGenerationProgress）
  - 實作無縫進入 ChapterReadingView（第一章已載入）
  - 實作新手引導進度追蹤（MongoDB: onboarding_progress）
  - 實作引導步驟完成檢查
  - 實作新手獎勵發放（+20 靈氣）
  - 實作 OnboardingTracker 追蹤所有引導事件（destiny_calculated, destiny_rerolled, destiny_confirmed）
  - 整合 Firebase Analytics 事件
  - _需求: 8.1, 8.2, 8.3, 8.4, 8.5, 8.6, 8.7_

- [ ] 19. 實作推播通知系統
  - 整合 Firebase Cloud Messaging
  - 實作 PushService 推播服務
  - 實作不活躍用戶推播（1 天、3 天未登入）
  - 實作突破準備完成推播
  - 實作悟道內容生成完成推播
  - 設定 Firebase Remote Config 推播配置
  - 實作推播事件記錄
  - 實作推播權限請求 UI
  - _需求: 8.1, 8.2, 8.3, 8.4, 8.5, 8.6, 8.7_

- [ ] 20. 實作安全性機制
  - 實作 API Rate Limiter（每用戶每分鐘限制）
  - 實作不同 RPC 的個別限流規則
  - 實作異常行為偵測（靈氣異常、RPC 頻率）
  - 實作自動封鎖機制（24 小時）
  - 實作請求簽章驗證（token + timestamp + salt）
  - 實作 IAP 收據簽章驗證
  - 實作冪等性機制（所有有副作用的 RPC）
  - 實作結構化日誌（user_id + session_id）
  - 實作關鍵操作日誌記錄
  - _需求: 7.1, 7.2, 7.3, 7.4, 7.5, 7.6, 7.7_

- [ ] 21. 實作玩家進度回溯系統
  - 實作 SnapshotService 每日快照服務
  - 建立 snapshots collection（MongoDB）
  - 實作自動快照生成（每日凌晨）
  - 實作 RestoreService 進度回溯服務
  - 建立 ProgressRestoreView UI 介面
  - 實作快照列表顯示（最近 3 天）
  - 實作回溯確認對話框
  - 實作回溯操作和備份機制
  - 實作回溯事件記錄
  - _需求: 所有需求_

- [ ] 22. 實作離線快取與同步
  - 建立 OfflineCacheData 資料結構
  - 實作 LocalStorageManager 本地儲存管理
  - 實作 SyncManager 同步管理器
  - 實作離線章節快取
  - 實作時間戳比對同步策略
  - 實作資料合併邏輯
  - 實作同步衝突處理
  - 實作同步狀態顯示 UI
  - _需求: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7_

- [ ] 23. 實作 UI/UX 與世界觀呈現
  - 設計並實作主題配色（墨黑、朱紅、骨白）
  - 實作修煉語氣文案（「道友」稱呼、詩意提示）
  - 實作突破成功動畫和音效
  - 實作靈氣耗盡提示動畫
  - 實作境界進度條視覺效果
  - 實作「靜坐休息」引導動畫
  - 實作載入動畫（「天機推演中」、「翻閱古籍」）
  - 實作打字機效果
  - 實作 VIP 專屬光效
  - _需求: 8.1, 8.2, 8.3, 8.4, 8.5, 8.6, 8.7_

- [ ] 24. 實作資料清理與備份策略
  - 實作 CleanupService 資料清理服務
  - 實作不活躍用戶 Redis 快取清理（30 天）
  - 實作 AI 快取清理（未命中 30 天的內容）
  - 實作快取清理統計納入成本報表
  - 實作 BackupService 備份服務
  - 實作每日備份關鍵 collections
  - 實作備份至雲端儲存（S3 / Firebase Storage）
  - 設定定時任務（Nakama Cron）
  - 實作快取清理任務（每日執行）
  - _需求: 所有需求_

- [ ] 25. 整合測試與優化
  - 撰寫單元測試（ConfigService, DestinyService, CultivationService, BreakthroughService）
  - 撰寫冪等性測試（重複請求不重複扣除）
  - 撰寫 API 重試測試
  - 建立 Mock AI Gateway
  - 撰寫整合測試（命格生成、章節閱讀、突破流程）
  - 執行 E2E 測試（完整使用者流程）
  - 效能測試（UI 流暢度、API 回應時間）
  - 記憶體洩漏檢測
  - 優化 AI API 呼叫頻率
  - 優化資料庫查詢效能
  - 優化 Redis 快取策略
  - _需求: 所有需求_

- [ ] 26. 部署與監控設定
  - 設定 Nakama 生產環境
  - 設定 MongoDB 生產資料庫（Replica Set）
  - 設定 PostgreSQL 生產資料庫
  - 設定 Redis 快取叢集
  - 設定 Firebase Analytics 和 Remote Config
  - 設定日誌收集（ELK Stack / CloudWatch）
  - 設定監控告警（API 錯誤率、回應時間、AI 成本）
  - 建立部署腳本和 CI/CD pipeline
  - 設定 KPI 監控儀表板
  - 設定成本效益追蹤報表
  - 設定 AI 成本監控儀表板（Grafana / Firebase Analytics）
  - 實作每日 AI Token 用量統計（依模型分組）
  - 設定預算閾值警報（每日與每週）
  - 實作超預算自動降級通知
  - 設定快取命中率監控
  - 設定 A/B 測試結果追蹤報表
  - _需求: 所有需求_



## 可選優化任務（後期追加）

以下任務為可選的優化項目，可在 MVP 穩定後逐步實作：

- [ ]* 27. AI 成本追蹤可視化強化
  - 實作 Grafana dashboard 顯示每日成本時間序列圖
  - 按模型分組顯示成本趨勢（GPT-4, GPT-4-mini, Claude, SD）
  - 實作成本預測曲線（基於歷史數據）
  - 實作成本異常偵測（突然增長警報）
  - 實作每用戶成本排行榜（識別高消費用戶）
  - 實作 ROI 趨勢圖（AI 成本 vs VIP 收入）

- [ ]* 28. 玩家行為留存模型優化
  - 實作自定義留存分群（Firebase Analytics）
  - 追蹤「心印卡分享 → 回流率」相關性
  - 追蹤「突破成功 → 次日留存」相關性
  - 追蹤「VIP 訂閱 → 長期留存」相關性
  - 實作玩家生命週期階段分類（新手、活躍、流失、回流）
  - 實作針對不同階段的推播策略
  - 建立留存預測模型

- [ ]* 29. AI 快取策略細化
  - 實作 TTL 分層策略（基於使用頻率）
  - 高頻 prompt_hash：TTL 7 天
  - 中頻 prompt_hash：TTL 3 天
  - 低頻 prompt_hash：TTL 1 天
  - 實作快取熱度追蹤（記錄每個 hash 的命中次數）
  - 實作動態 TTL 延長（命中次數 > 閾值時自動延長）
  - 實作快取預熱機制（預先生成熱門章節）
  - 實作快取命中率優化報告

- [ ]* 30. AI 模型異常熔斷機制
  - 實作模型錯誤率追蹤（每 5 分鐘統計）
  - 設定錯誤率閾值（例如：> 30%）
  - 實作自動熔斷邏輯（暫時封鎖該模型 1 小時）
  - 實作降級策略（自動切換至備用模型）
  - 實作熔斷恢復機制（1 小時後自動嘗試恢復）
  - 實作熔斷事件通知（Slack / Email）
  - 實作熔斷歷史記錄和分析報表
  - 實作模型健康度儀表板

- [ ]* 31. 進階內容審查系統
  - 整合內容審查 API（OpenAI Moderation / Perspective API）
  - 實作敏感詞過濾
  - 實作內容品質評分
  - 實作不當內容自動重新生成
  - 實作審查日誌記錄
  - 實作人工審查介面（標記可疑內容）

- [ ]* 32. 玩家社群功能
  - 實作好友系統
  - 實作宗門系統（玩家可創建或加入宗門）
  - 實作宗門成員間互贈靈氣
  - 實作宗門排行榜
  - 實作宗門聊天室
  - 實作修煉進度分享至宗門

- [ ]* 33. 進階分析與預測
  - 實作玩家流失預測模型
  - 實作 LTV（生命週期價值）預測
  - 實作最佳靈氣消耗節奏分析
  - 實作最佳突破率分析
  - 實作 A/B 測試自動化分析
  - 實作營收預測模型

