# 修仙之路 App - MVP 需求文件

## 簡介

《修仙之路》是一款以 AI 敘事生成為核心的互動小說 App，結合「命格系統」、「靈氣節奏」與「沉浸式修仙世界觀」。玩家輸入姓名、生日與性別後，AI 將為其創造專屬的命格、師承與修行故事。這是一場修心體驗，玩家在閱讀中積聚靈氣、突破境界、領悟人生道理。

## 術語表

- **System**：修仙之路 App 系統
- **Player**：使用 App 的玩家（道友）
- **Qi**：靈氣，玩家修煉的核心資源
- **Realm**：境界，玩家修煉的階段（如煉氣期、築基期等）
- **Destiny**：命格，根據玩家生辰八字生成的五行屬性
- **Chapter**：章節，AI 生成的故事內容單元
- **Breakthrough**：突破，從一個境界晉升到下一個境界的過程
- **Heart Seal Card**：心印閃卡，突破後生成的可分享圖片
- **VIP Member**：修行之路會員，付費訂閱用戶
- **Spirit Stone**：靈石，遊戲內貨幣

## 需求

### 需求 1：命格生成系統

**用戶故事：** 作為一位新玩家，我想要輸入我的姓名、性別和生日，以便系統為我生成專屬的命格和修行起點。

#### 驗收標準

1. WHEN Player 首次啟動 App，THE System SHALL 顯示命格生成介面
2. THE System SHALL 接受 Player 輸入的姓名（1-20 個字元）、性別（男/女/其他）和生日（日期格式）
3. WHEN Player 提交個人資訊，THE System SHALL 呼叫 AI 服務生成五行命格（金木水火土比例）
4. THE System SHALL 根據命格生成 Player 的出身背景故事（200-500 字）
5. THE System SHALL 顯示命格卡片，包含姓名、五行屬性、靈根類型和宿命描述
6. THE System SHALL 將命格資料儲存至後端資料庫

### 需求 2：章節閱讀與靈氣消耗

**用戶故事：** 作為一位玩家，我想要閱讀 AI 生成的修煉故事章節，並透過消耗靈氣來推進劇情，以體驗我的修仙之路。

#### 驗收標準

1. THE System SHALL 每日自動為 Player 恢復 50 點靈氣
2. WHEN Player 選擇閱讀新章節，THE System SHALL 消耗 10 點靈氣
3. IF Player 靈氣不足 10 點，THEN THE System SHALL 顯示「靈氣微弱」提示並禁用閱讀按鈕
4. THE System SHALL 呼叫 AI 服務生成章節內容（每章 300-800 字）
5. THE System SHALL 在章節中融入 Player 的命格特徵和當前境界
6. THE System SHALL 記錄 Player 的閱讀進度和剩餘靈氣至後端
7. THE System SHALL 在章節閱讀介面顯示當前靈氣值和境界進度條

### 需求 3：靈氣恢復機制

**用戶故事：** 作為一位玩家，當我的靈氣耗盡時，我想要有多種方式恢復靈氣，以便繼續我的修煉之旅。

#### 驗收標準

1. THE System SHALL 每日 00:00 UTC 自動為所有 Player 恢復 50 點靈氣
2. WHEN Player 選擇「觀看廣告」，THE System SHALL 播放 15-30 秒廣告影片
3. WHEN 廣告播放完成，THE System SHALL 為 Player 恢復 10 點靈氣
4. WHEN Player 選擇「供奉靈石」，THE System SHALL 顯示靈石購買選項（30/50/100 靈石）
5. WHEN Player 完成靈石購買，THE System SHALL 立即補滿 Player 的靈氣至 50 點
6. WHERE Player 為 VIP Member，THE System SHALL 移除靈氣上限限制
7. THE System SHALL 以修煉語氣呈現所有靈氣恢復選項（如「吸收天地靈息」）

### 需求 4：境界突破系統

**用戶故事：** 作為一位玩家，當我累積足夠的修煉進度時，我想要進行境界突破，以提升我的修為並解鎖新的故事內容。

#### 驗收標準

1. THE System SHALL 追蹤 Player 的境界進度（0-100%）
2. WHEN Player 閱讀章節，THE System SHALL 增加境界進度 10%
3. WHEN 境界進度達到 100%，THE System SHALL 觸發突破事件通知
4. WHEN Player 選擇進行突破，THE System SHALL 生成「悟道篇」特殊章節
5. THE System SHALL 根據 Player 命格和當前境界計算突破成功率（60-95%）
6. THE System SHALL 執行突破判定並回傳結果（成功/失敗）
7. IF 突破成功，THEN THE System SHALL 更新 Player 境界至下一階段
8. IF 突破失敗，THEN THE System SHALL 保持當前境界並重置進度至 80%
9. THE System SHALL 記錄突破結果至後端並上報 Firebase Analytics

### 需求 5：心印閃卡生成與分享

**用戶故事：** 作為一位玩家，當我成功突破境界時，我想要獲得一張精美的心印閃卡，並能分享到社群媒體，以展示我的修煉成果。

#### 驗收標準

1. WHEN Player 成功突破境界，THE System SHALL 自動生成心印閃卡
2. THE System SHALL 在閃卡上顯示 Player 姓名、新境界名稱、突破日期和境界描述
3. THE System SHALL 使用 AI 圖像生成服務創建境界對應的視覺背景
4. THE System SHALL 提供分享按鈕，支援分享至 Threads、Discord、TikTok
5. WHEN Player 點擊分享，THE System SHALL 將閃卡圖片儲存至本地相簿
6. THE System SHALL 開啟系統分享介面，允許 Player 選擇分享目標
7. THE System SHALL 記錄分享事件至 Firebase Analytics

### 需求 6：VIP 會員系統

**用戶故事：** 作為一位玩家，我想要訂閱 VIP 會員，以獲得無限制的閱讀體驗和專屬特權。

#### 驗收標準

1. THE System SHALL 提供月訂閱制 VIP 會員選項（NT$150-200）
2. WHEN Player 訂閱 VIP，THE System SHALL 移除每日靈氣上限限制
3. WHERE Player 為 VIP Member，THE System SHALL 移除所有廣告顯示
4. WHERE Player 為 VIP Member，THE System SHALL 提供專屬光效和修行稱號
5. THE System SHALL 透過 App Store / Google Play 處理訂閱付款
6. THE System SHALL 驗證訂閱狀態並同步至後端伺服器
7. THE System SHALL 在 VIP 到期前 3 天發送續訂提醒通知

### 需求 7：Firebase Analytics 整合

**用戶故事：** 作為產品經理，我需要追蹤玩家行為數據，以優化遊戲體驗和商業策略。

#### 驗收標準

1. THE System SHALL 在 Player 完成命格生成時上報 `destiny_created` 事件
2. THE System SHALL 在 Player 閱讀章節時上報 `chapter_read` 事件（包含境界、靈氣消耗）
3. THE System SHALL 在 Player 嘗試突破時上報 `breakthrough_attempt` 事件
4. THE System SHALL 在突破成功/失敗時上報 `breakthrough_success` 或 `breakthrough_fail` 事件
5. THE System SHALL 在 Player 觀看廣告時上報 `ad_watched` 事件
6. THE System SHALL 在 Player 購買靈石時上報 `purchase` 事件（包含金額和貨幣類型）
7. THE System SHALL 在 Player 訂閱 VIP 時上報 `subscription_start` 事件
8. THE System SHALL 在 Player 分享閃卡時上報 `card_shared` 事件
9. THE System SHALL 在離線時將事件暫存至本地佇列，待網路恢復後批次上報

### 需求 8：UI/UX 與世界觀呈現

**用戶故事：** 作為一位玩家，我想要體驗沉浸式的修仙世界觀，透過詩意的語氣和精美的視覺設計感受修心之旅。

#### 驗收標準

1. THE System SHALL 使用「道友」稱呼 Player
2. THE System SHALL 以詩意、沉穩、哲理的語氣呈現所有文案
3. THE System SHALL 使用墨黑、朱紅、骨白為主要配色
4. THE System SHALL 在靈氣耗盡時顯示「道友，靈氣微弱，今日修行已達極限」等修煉語氣提示
5. THE System SHALL 在突破成功時播放視覺高潮動畫和音效
6. THE System SHALL 在主介面顯示當前境界、靈氣值和境界進度條
7. THE System SHALL 提供「靜坐休息」選項，以修煉語氣引導 Player 明日再修
8. THE System SHALL 使用 Unity Localization 框架實現多國語言支援
9. THE System SHALL 支援繁體中文、簡體中文和英文三種語言
10. THE System SHALL 根據裝置系統語言自動選擇對應語言
11. THE System SHALL 提供語言切換選項，允許 Player 手動變更介面語言
