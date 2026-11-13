---
paths:
  - "backend/**"
---

# Backend Rules

Scope：`backend/`（Go）。這是專案自己的規則檔，`awkit init` 只寫下起始內容，之後由專案維護。架構定案之後，把決定寫進文件，再回來把這份檢查清單收緊；規則與程式碼衝突時以程式碼為準，並回修本檔。

## 1) 結構

- 依功能分 package，不依型別分：不開 `models/`、`utils/`、`helpers/` 這類什麼都放的 package。對外不需要的放 `internal/`。
- package 之間的依賴只往一個方向；出現循環，是邊界切錯了。
- 依賴由建構子傳入（`NewService(repo Repository)`），不在函式裡自己建，也不放在 package 層的可變變數；`init()` 不做註冊、不做 I/O。
- 介面定義在使用它的那一方，只列用得到的方法。
- 產生的程式碼不手改；要改就改來源，再重新產生。

## 2) 錯誤

- 每個錯誤不是處理就是回傳，不吞掉。往上傳時用 `fmt.Errorf("...: %w", err)` 補上這一層在做什麼。
- 呼叫端要分辨的錯誤，定義成 sentinel（`var ErrNotFound = errors.New(...)`）或型別，用 `errors.Is`、`errors.As` 判斷；不比對錯誤字串。
- 預期內的失敗回傳 error；`panic` 只留給程式本身的錯。
- 回給客戶端的訊息不帶內部細節（SQL、路徑、stack）；細節寫進 log。

## 3) Context 與並發

- 會做 I/O 或可能等待的函式，第一個參數是 `ctx context.Context`，一路往下傳，不存進 struct。
- 每個 goroutine 都要說得出誰等它、它怎麼結束；會擋住的操作要聽 `ctx.Done()`。
- 共享的狀態用 mutex 或 channel 保護，並寫明由誰保護；`go test -race` 要過。

## 4) 資料的修改

- 自己擁有的資料可以直接改。收到的、共享的、從快取拿出來的，先複製再改；真的要就地改，就在函式名稱上寫明（`MutateItems`）。
- 以值接收的 struct 改了，呼叫端看不到；要改就收指標。

## 5) 輸入與機密

- 外部輸入在進入點驗證（型別、長度、範圍），業務邏輯只收驗過的資料。
- SQL 一律用參數，不拼字串。
- 密鑰、token、連線字串從環境變數或設定讀，不寫進程式碼，也不寫進 log。

## 6) 大小

函式 50 行以內，巢狀不超過 4 層，檔案不超過 800 行（產生的程式碼不算）。超過就拆。

## 7) 驗證

`go build ./... && go vet ./... && go test ./...`，以 `.ai/config.yaml` 裡 backend 的 `verify` 為準；`gofmt -l .` 不能列出任何檔案。
