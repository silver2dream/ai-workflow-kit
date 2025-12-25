# Generate Tasks Command

從 design.md 生成 tasks.md（如果需要）。

**用途：**
- 在 start-work.md 的 Phase 0 中自動調用
- 可獨立執行：`/generate-tasks` 或 `/generate-tasks --autonomous`

**參數：**
- `--autonomous`: 自動化模式，不詢問用戶確認

**輸出：**
- 為需要的 spec 生成 tasks.md
- 返回 0 表示成功，非 0 表示失敗

---

## Step 1: 檢查環境變數

```bash
# 確保必要的環境變數已設置（由 preflight.md 導出）
if [[ -z "$SPEC_BASE_PATH" ]]; then
  SPEC_BASE_PATH=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(c.get('specs',{}).get('base_path', '.ai/specs'))" 2>/dev/null || echo ".ai/specs")
fi

if [[ -z "$ACTIVE_SPECS" ]]; then
  ACTIVE_SPECS=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(','.join(c.get('specs',{}).get('active', [])))" 2>/dev/null || echo "")
fi

if [[ -z "$AUTO_GENERATE_TASKS" ]]; then
  AUTO_GENERATE_TASKS=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(str(c.get('specs',{}).get('auto_generate_tasks', True)).lower())" 2>/dev/null || echo "true")
fi

# 檢查是否為自動化模式
AUTONOMOUS_MODE=false
if [[ "$1" == "--autonomous" ]]; then
  AUTONOMOUS_MODE=true
fi

echo "[PRINCIPAL] $(date +%H:%M:%S) | Spec base path: $SPEC_BASE_PATH"
echo "[PRINCIPAL] $(date +%H:%M:%S) | Active specs: $ACTIVE_SPECS"
echo "[PRINCIPAL] $(date +%H:%M:%S) | Auto generate: $AUTO_GENERATE_TASKS"
echo "[PRINCIPAL] $(date +%H:%M:%S) | Autonomous mode: $AUTONOMOUS_MODE"
```

---

## Step 2: 檢查是否需要生成

```bash
# 如果沒有 active specs，跳過
if [[ -z "$ACTIVE_SPECS" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ 沒有啟用的 spec，跳過"
  exit 0
fi

# 如果 auto_generate_tasks 為 false，跳過
if [[ "$AUTO_GENERATE_TASKS" != "true" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ 自動生成已禁用，跳過"
  exit 0
fi
```

---

## Step 3: 對每個 Active Spec 檢查並生成

```bash
# 分割 active specs 列表
IFS=',' read -ra SPEC_LIST <<< "$ACTIVE_SPECS"

GENERATED_COUNT=0
SKIPPED_COUNT=0

for SPEC_NAME in "${SPEC_LIST[@]}"; do
  # 移除空白
  SPEC_NAME=$(echo "$SPEC_NAME" | tr -d ' ')
  
  if [[ -z "$SPEC_NAME" ]]; then
    continue
  fi
  
  SPEC_PATH="${SPEC_BASE_PATH}/${SPEC_NAME}"
  
  echo ""
  echo "[PRINCIPAL] $(date +%H:%M:%S) | 檢查 spec: $SPEC_NAME"
  
  # 檢查 spec 目錄是否存在
  if [[ ! -d "$SPEC_PATH" ]]; then
    echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ Spec 目錄不存在：$SPEC_PATH"
    SKIPPED_COUNT=$((SKIPPED_COUNT + 1))
    continue
  fi
  
  # 檢查 tasks.md 是否存在且有未完成任務
  if [[ -f "$SPEC_PATH/tasks.md" ]]; then
    UNCOMPLETED=$(grep -c '^\- \[ \]' "$SPEC_PATH/tasks.md" 2>/dev/null || echo "0")
    if [[ "$UNCOMPLETED" -gt 0 ]]; then
      echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ tasks.md 已存在，有 $UNCOMPLETED 個未完成任務"
      SKIPPED_COUNT=$((SKIPPED_COUNT + 1))
      continue
    else
      echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ tasks.md 已存在但所有任務已完成"
    fi
  fi
  
  # 檢查 design.md 是否存在
  if [[ ! -f "$SPEC_PATH/design.md" ]]; then
    echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ design.md 不存在，無法生成 tasks.md"
    SKIPPED_COUNT=$((SKIPPED_COUNT + 1))
    continue
  fi
  
  # 需要生成 tasks.md
  echo "[PRINCIPAL] $(date +%H:%M:%S) | 從 design.md 生成 tasks.md..."
  
  # 讀取 design.md
  DESIGN_CONTENT=$(cat "$SPEC_PATH/design.md")
  
  # 提取 feature name（取第一個 # 標題）
  FEATURE_NAME=$(echo "$DESIGN_CONTENT" | grep -m1 '^# ' | sed 's/^# //' || echo "$SPEC_NAME")
  
  # 提取 overview
  OVERVIEW=$(echo "$DESIGN_CONTENT" | awk '/^## (Overview|概述)/,/^## / {if (!/^## /) print}' | head -20 || echo "請參考 design.md")
```

  # 生成完整的 tasks.md（符合舊版格式）
  cat > "$SPEC_PATH/tasks.md" << 'TASKS_EOF'
# $FEATURE_NAME - Implementation Plan

## 目標

$OVERVIEW

---

## Tasks

**注意：** 此文件由 generate-tasks.md 自動生成，請根據 design.md 的內容調整任務列表。

- [ ] 1. 閱讀 design.md 並規劃實現
  - Repo: root
  - Coordination: sequential
  - Sync: independent
  - Priority: P2
  - Release: false
  - [ ] 1.1 閱讀完整的 design.md
    - 理解系統架構和設計
    - 識別關鍵接口、數據模型
    - _Requirements: 參考 requirements.md_
  
  - [ ] 1.2 識別實現階段
    - 確定依賴關係
    - 規劃實現步驟
    - _Requirements: 參考 requirements.md_

- [ ] 2. 實現核心功能
  - Repo: root
  - Coordination: sequential
  - Sync: independent
  - Priority: P2
  - Release: false
  - [ ] 2.1 實現核心組件
    - 根據 design.md 的 Components 章節實現
    - _Requirements: 參考 requirements.md_
  
  - [ ] 2.2 實現接口、數據模型
    - 根據 design.md 的 Interfaces 和 Data Models 章節實現
    - _Requirements: 參考 requirements.md_
  
  - [ ]* 2.3 添加單元測試
    - 測試核心功能
    - _Requirements: 參考 requirements.md_

- [ ] 3. Checkpoint
  - Ensure tests pass. In autonomous mode, log issues and continue (do not ask).

- [ ] 4. 集成測試
  - Repo: root
  - Coordination: sequential
  - Sync: independent
  - Priority: P2
  - Release: false
  - [ ] 4.1 集成各組件
    - 確保組件能正確交互
    - _Requirements: 參考 requirements.md_
  
  - [ ]* 4.2 添加集成測試
    - 測試端到端流程
    - _Requirements: 參考 requirements.md_
  
  - [ ] 4.3 執行驗證命令
    - 執行 build 和 test 命令
    - _Requirements: 參考 requirements.md_

- [ ] 5. 完善文檔
  - Repo: root
  - Coordination: sequential
  - Sync: independent
  - Priority: P2
  - Release: false
  - [ ] 5.1 更新文檔
    - 更新 README 和相關文檔
    - _Requirements: 參考 requirements.md_
  
  - [ ] 5.2 準備審查材料
    - 清理代碼註釋
    - 準備功能演示
    - _Requirements: 參考 requirements.md_

- [ ] 6. Final Checkpoint
  - Ensure tests pass. In autonomous mode, log issues and continue (do not ask).

---

## 注意事項

1. **任務格式：**
   - 主任務：`- [ ] N. 任務名稱`
   - 子任務：`- [ ] N.M 子任務名稱`
   - 可選任務（測試）：`- [ ]* N.M 任務名稱`

2. **Metadata 欄位：**
   - `Repo`: 目標 repo（root, backend, frontend 等）
   - `Coordination`: 協調模式（sequential | parallel）
   - `Sync`: 同步模式（required | independent）
   - `Priority`: 優先級（P0 | P1 | P2）
   - `Release`: 是否為 release（true | false）

3. **Requirements 引用：**
   - 每個任務應引用對應的 requirements.md 章節
   - 格式：`_Requirements: X.X_`

4. **Checkpoint 任務：**
   - 在關鍵階段添加 Checkpoint
   - 最後一個任務必須是 Final Checkpoint

5. **自動化模式：**
   - Checkpoint 任務在自動化模式下不會詢問用戶
   - 記錄完成並繼續執行

6. **調整建議：**
   - 請根據 design.md 的實際內容調整任務列表
   - 添加更具體的子任務描述
   - 確保任務順序符合依賴關係
   - 為每個主任務添加正確的 Metadata
TASKS_EOF

  # 替換變數
  sed -i "s/\$FEATURE_NAME/$FEATURE_NAME/g" "$SPEC_PATH/tasks.md"
  sed -i "s/\$OVERVIEW/$OVERVIEW/g" "$SPEC_PATH/tasks.md"

  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ 已生成 tasks.md: $SPEC_PATH/tasks.md"
  GENERATED_COUNT=$((GENERATED_COUNT + 1))
  
  # 非自動化模式，詢問用戶是否調整
  if [[ "$AUTONOMOUS_MODE" != "true" ]]; then
    echo ""
    echo "已生成基本的 tasks.md 模板。"
    echo "建議您根據 design.md 的內容調整任務列表。"
    echo ""
    echo "是否要現在調整？(y/n)"
    read -r ADJUST
    
    if [[ "$ADJUST" == "y" || "$ADJUST" == "Y" ]]; then
      echo "請編輯文件：$SPEC_PATH/tasks.md"
      echo "編輯完成後按 Enter 繼續..."
      read -r
    fi
  fi
done
```

---

## Step 4: 輸出結果

```bash
echo ""
echo "[PRINCIPAL] $(date +%H:%M:%S) | ===== 生成結果 ====="
echo "[PRINCIPAL] $(date +%H:%M:%S) | 已生成 $GENERATED_COUNT 個 tasks.md"
echo "[PRINCIPAL] $(date +%H:%M:%S) | 已跳過 $SKIPPED_COUNT 個 spec"
echo ""

if [[ "$GENERATED_COUNT" -gt 0 ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ 生成完成"
  
  if [[ "$AUTONOMOUS_MODE" != "true" ]]; then
    echo ""
    echo "建議檢查生成的 tasks.md 並根據需要調整。"
    echo "調整完成後可以繼續執行工作流。"
  fi
else
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ 沒有生成任何 tasks.md"
fi

exit 0
```

---

## tasks.md 格式規範（Kiro 相容）

生成的 tasks.md 必須符合以下規範：

1. **主任務格式：** `- [ ] N. 任務名稱`
2. **子任務格式：** `- [ ] N.M 子任務名稱`
3. **可選任務（測試）：** `- [ ]* N.M 任務名稱`
4. **Metadata 欄位：**
   - `Repo`: 目標 repo
   - `Coordination`: sequential | parallel
   - `Sync`: required | independent
   - `Priority`: P0 | P1 | P2
   - `Release`: true | false
5. **任務描述：** 每個任務應用縮進列出描述
6. **Requirements 引用：** 使用 `_Requirements: X.X_` 引用
7. **Checkpoint 任務：** 在關鍵階段添加 Checkpoint
8. **Final Checkpoint：** 最後一個任務必須是 Final Checkpoint

---

## 使用範例

**在 start-work.md 中調用（自動化模式）：**
```bash
# Phase 0: 生成 tasks.md（如果需要）
bash .ai/commands/generate-tasks.md --autonomous

if [[ $? -ne 0 ]]; then
  echo "tasks.md 生成失敗"
  exit 1
fi
```

**獨立執行（互動模式）：**
```bash
# 生成 tasks.md 並詢問是否調整
bash .ai/commands/generate-tasks.md
```

**獨立執行（自動化模式）：**
```bash
# 生成 tasks.md 不詢問
bash .ai/commands/generate-tasks.md --autonomous
```

---

## 注意事項

1. **模板生成：** 此命令生成包含完整 Metadata 的模板
2. **人工審查：** 非自動化模式下，建議人工審查並調整生成的 tasks.md
3. **自動化模式：** 在自動化模式下會直接使用生成的模板
4. **依賴關係：** 確保 preflight.md 已執行，環境變數已設置
