# Debris

**彌合設計庫格式之間的鴻溝。**

[English](../../README.md) | [简体中文](README_zh-Hans.md)

Debris 是一款跨平台命令列工具（同時提供 JSON-RPC 2.0 stdio 服務），用於檢查、規劃和轉換不同設計資產庫格式。它透過統一的內部領域模型直接進行格式轉換——無需借助中間格式即可保留資料夾、標籤、標籤群組、顏色、評分、備註、來源連結以及素材檔案。

## 支援的格式

| 格式 | 讀取（匯入） | 寫入（匯出） | 說明 |
| :--- | :---: | :---: | :--- |
| **Eagle** (`.library`, 4.x) | ✅ | ✅ | 已測試驗證可用（含 Hive 相容） |
| **Pixcall** (`.pixcall`, v22) | ✅ | ✅ | 已測試驗證可用（詳見 [Pixcall 相容說明](../pixcall-compatibility.md)） |
| **Billfish** (`.bf`) | ✅ | ⚠️ 不再維護 | 支援從 Billfish 遷出。由於 Billfish 已停止更新與維護，匯出為 Billfish 不再積極維護。 |

## 功能特性

* **跨格式直接轉換：** 在支援的格式之間直接轉換（例如 `billfish` → `eagle`、`billfish` → `pixcall`、`eagle` ↔ `pixcall`）。
* **來源格式自動識別：** 省略 `--from` / `--format` 時，Debris 會自動探測輸入素材庫的格式。
* **檢查與轉換規劃：** 使用 `inspect` 查看素材庫摘要；使用 `plan` 在實際轉換前預覽資產數量、警告訊息以及潛在的相容性損失。
* **靈活的素材傳輸模式：** 支援 `copy`、`hardlink` 與 `auto`（`--copy-mode`），並提供 `--overwrite`、`--include-deleted`、`--include-hidden` 和 `--name` 等選項。
* **機器可讀輸出：** 透過 `--json` 輸出 NDJSON 進度事件與最終結果。
* **跨平台相容：** 基於 Go（`1.23+`）建構，支援 macOS、Linux 與 Windows。

## 用法

### 轉換素材庫

```sh
# 將 Billfish 素材庫轉換為 Eagle 素材庫
debris convert --from billfish --to eagle --input ./lib.billfish --output ./lib.library

# 將 Billfish 素材庫轉換為 Pixcall 素材庫（自動識別來源格式）
debris convert -t pixcall -i ./lib.billfish -o ./pixcall-lib

# 使用硬連結將 Eagle 轉換為 Pixcall，並輸出 NDJSON 日誌
debris convert -t pixcall -i ./lib.library -o ./pixcall-lib --copy-mode hardlink --json

# 將 Pixcall 素材庫轉換為 Eagle 素材庫
debris convert -t eagle -i ./pixcall-lib -o ./lib.library
```

#### 常用 `convert` 參數

* `--from`, `-f`：來源格式（`billfish`、`eagle`、`pixcall`），省略時自動探測。
* `--to`, `-t`：目標格式（`eagle`、`pixcall`）。
* `--input`, `-i`：輸入素材庫目錄路徑。
* `--output`, `-o`：輸出素材庫目錄路徑。
* `--copy-mode`：素材傳輸模式：`auto`（預設）、`copy` 或 `hardlink`。
* `--overwrite`：允許覆寫已存在的目標目錄內容。
* `--include-deleted`：包含已刪除 / 垃圾桶中的項目。
* `--include-hidden`：包含隱藏項目。
* `--name`：自訂目標素材庫名稱。
* `--json`：以 NDJSON 格式輸出進度事件與最終結果。

### 檢查素材庫

```sh
debris inspect --path ./lib.library
debris inspect -p ./pixcall-lib --json
```

### 預覽轉換計畫

```sh
debris plan --to pixcall --input ./lib.library --output ./pixcall-lib
```
