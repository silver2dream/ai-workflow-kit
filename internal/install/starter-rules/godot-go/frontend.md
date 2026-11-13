---
paths:
  - "frontend/**"
---

# Frontend Rules

Scope：`frontend/`（Godot .NET 專案與純 C# 組件）；各資料夾的分工見 `frontend/README.md`。這是專案自己的規則檔，`awkit init` 只寫下起始內容，之後由專案維護。架構定案之後，把決定寫進文件，再回來把這份檢查清單收緊；規則與程式碼衝突時以程式碼為準，並回修本檔。

## 1) 語言

- 全面 C#，不寫 GDScript。參考資料的範例是 GDScript 時，只取做法，改寫成 C#。
- `Nullable` 開著、警告當錯誤（`Directory.Build.props`）：不用 `!` 壓掉可空性警告，不用 `#pragma warning disable` 過關。
- 自己的程式碼不用 reflection。以字串指定方法或節點路徑的呼叫（`Call("...")`、`GetNode("...")`）收在少數幾個地方，其他地方用有型別的參考。

## 2) 分層

- `src/Game.Core/` 是純 C#：遊戲狀態、規則、ViewModel。它不引用 GodotSharp，所以不用引擎就能測。
- `game/` 是 Godot 專案：場景與節點腳本。節點腳本只做三件事：把輸入轉成對 Core 的呼叫、訂閱 Core 的變更、把狀態畫出來。規則不寫在節點腳本裡。
- 資料只往一個方向走：操作 → Core 的命令 → 狀態改變 → 發出變更事件 → 畫面重繪。畫面不直接改狀態。
- 畫面之間不互相引用；共用的資料放 Core，切換畫面走同一個入口。

## 3) Godot 的用法

- 訊號用 `[Signal]` 委派宣告、用 `+=` 訂閱。自己用 `+=` 訂的，在 `_ExitTree` 用 `-=` 取消：自訂訊號與 Core 的事件不會隨節點釋放自動斷開。
- `_Process`、`_PhysicsProcess` 裡不配置記憶體、不做 I/O；能用訊號或計時器的就不輪詢。
- 節點參考在 `_Ready` 取一次存起來，不在每一幀 `GetNode`。
- 會顯示的字不寫死在程式裡，放翻譯表、用鍵取；顏色、字級、間距放 Theme 與 `.tres`。
- 數值（傷害、機率、冷卻）放資料檔，不寫死在 C#。

## 4) 命名與檔案

- 資料夾與場景檔用 snake_case；C# 檔用 PascalCase，檔名與類別同名，一個檔一個公開型別。
- 產生的程式碼不手改；`.godot/` 不進版控。

## 5) 客戶端的界線

- 打包出去的東西玩家都拿得到：不放密鑰、API key，或只該伺服器知道的數值。
- 伺服器說了算的事（貨幣、掉落、進度），客戶端只顯示，不自己算了就當真。
- 玩家輸入的字，顯示前先跳脫。

## 6) 大小

方法 50 行以內，巢狀不超過 4 層，檔案不超過 800 行。超過就拆。

## 7) 驗證

`dotnet build && dotnet test`，以 `.ai/config.yaml` 裡 frontend 的 `verify` 為準。新的規則與狀態寫在 `src/Game.Core`，測試寫在 `tests/Game.Core.Tests`。
