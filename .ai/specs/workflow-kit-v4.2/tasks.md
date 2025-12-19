# AI Workflow Kit v4.2 - Tasks

## Task 1: O6 移出 Offline Gate

- [x] 1.1 在 evaluate.sh 中將 O6 從 Offline Gate 移到獨立的 Extensibility Checks 區塊
- [x] 1.2 O6 結果不影響 OFFLINE_PASS 變數
- [x] 1.3 當 .github/workflows 目錄不存在時，O6 標記為 SKIP
- [x] 1.4 當 .github/workflows 存在但為空時，O6 標記為 FAIL
- [x] 1.5 更新 evaluate.md 說明 O6 已移出 Offline Gate

## Task 2: SKIP 白名單

- [x] 2.1 定義 SKIP 白名單：僅允許「可選依賴缺少」或「明確不適用」
- [x] 2.2 O6 讀不到 integration_branch 時改為 FAIL（配置錯誤）
- [x] 2.3 Python 執行錯誤時改為 FAIL（環境問題）
- [x] 2.4 在 evaluate.md 中明確記錄 SKIP 白名單

## Task 3: --strict 模式

- [x] 3.1 在 evaluate.sh 中加入 --strict 參數解析
- [x] 3.2 --strict 模式下檢查 audit.json 是否有 P0 findings
- [x] 3.3 有 P0 findings 時標記 Offline Gate 為 FAIL
- [x] 3.4 在 evaluate.md 中記錄 --strict 模式用法

## Task 4: 補齊依賴宣告

- [x] 4.1 在 evaluate.md Online Gate 前置條件中加入 curl
- [x] 4.2 說明「Offline」意指評估時不需網路，但依賴需預先安裝
- [x] 4.3 提供 pip install 指令或 requirements.txt 路徑

## Task 5: 標註示意程式碼

- [x] 5.1 將 evaluate.md 中的嵌入式腳本標註為「示意」
- [x] 5.2 明確指向 .ai/scripts/evaluate.sh 為權威來源
- [x] 5.3 移除或精簡重複的完整腳本區塊

## Task 6: 等級映射明確化

- [x] 6.1 定義等級門檻：A (9.0-10.0), B (8.0-8.9), C (7.0-7.9), D (6.0-6.9), F (<6.0)
- [x] 6.2 說明 final_grade = grade(min(total_score, cap))
- [x] 6.3 說明 cap 如何限制可達等級

## Task 7: 版本更新

- [x] 7.1 更新 evaluate.sh 版本號為 v4.2
- [x] 7.2 更新 evaluate.md 版本號為 v4.2
- [x] 7.3 在版本紀錄中加入 v4.2 變更說明

## Task 8: 測試驗證 *

- [x] 8.1 執行 evaluate.sh 驗證 Offline Gate
- [x] 8.2 測試 --strict 模式
- [x] 8.3 驗證 O6 移出後不影響 Offline Gate 結果
