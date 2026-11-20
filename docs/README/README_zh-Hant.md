# Debris

**彌合設計庫格式之間的鴻溝。**

[English](../../README.md) | [简体中文](README_zh-Hans.md)

Debris 是一款跨平台命令列工具與桌面端 GUI 應用（基於 MyGo + SolidJS 建構），用於檢查、規劃和轉換不同設計資產庫格式。它透過統一的內部領域模型直接進行格式轉換——無需借助中間格式即可保留資料夾、標籤、標籤群組、顏色、評分、備註、來源連結以及素材檔案。

## 支援的格式

| 格式 | 讀取（匯入） | 寫入（匯出） | 說明 |
| :--- | :---: | :---: | :--- |
| **Eagle** (`.library`, 4.x) | ✅ | ✅ | 已測試驗證可用（含 Hive 相容） |
| **Pixcall** (`.pixcall`, v22) | ✅ | ✅ | 已測試驗證可用（詳見 [Pixcall 相容說明](../pixcall-compatibility.md)） |
| **Billfish** (`.bf`) | ✅ | ⚠️ 不再維護 | 支援從 Billfish 遷出。由於 Billfish 已停止更新與維護，匯出為 Billfish 不再積極維護。 |

## 功能特性

* **桌面端 GUI（MyGo + SolidJS）：** 原生跨平台桌面應用（支援 macOS、Windows 與 Linux），提供目錄自動探測與庫統計預覽、轉換預檢（含相容損耗提示）、即時進度顯示與隨時取消、跟隨系統/淺色/深色主題切換，以及多語系介面（繁體中文、简体中文、English）。
* **跨格式直接轉換：** 在支援的格式之間直接轉換（例如 `billfish` → `eagle`、`billfish` → `pixcall`、`eagle` ↔ `pixcall`）。
* **自動識別與智慧輸出路徑：** 省略 `--from` / `--format` 時自動探測輸入素材庫格式；省略 `--output` 時依據輸入路徑與 `--to` 自動推導目標輸出目錄。
* **檢查與轉換規劃：** 使用 `inspect` 查看素材庫摘要；使用 `plan` 在實際轉換前預覽資產數量、警告訊息以及潛在的相容性損失。
* **靈活的素材傳輸模式：** 支援 `auto`（預設，同磁碟優先建立硬連結，跨磁碟自動回退為複製）、`hardlink` 與 `copy`（`--copy-mode`），並提供 `--overwrite`、`--include-deleted`、`--include-hidden` 和 `--name` 等選項。
* **腳本友善輸出：** 支援 TTY 行內進度顯示、`--quiet`（`-q`）靜默進度，以及透過 `--json` 輸出結構化 JSON / NDJSON 日誌。
* **跨平台相容：** 基於 Go（`1.27+`）建構，支援 macOS、Linux 與 Windows。

## 桌面端 GUI

桌面端程式碼位於 [`gui/`](../../gui/)，使用 **MyGo** 與 **SolidJS**（Tailwind CSS v4 + TypeScript + Vite）開發。

可在 Releases 頁面直接下載預編譯安裝包（macOS `.dmg`、Windows 安裝包 `.exe`、Linux `.deb` / `.rpm` / `.tar.gz`）。

### 從原始碼執行與打包

需要安裝 Go（`1.27+`）和 [Bun](https://bun.sh/)。

```sh
cd gui
bun install

# 開發模式啟動桌面應用
bun run dev

# 型別檢查與打包桌面應用
bun run typecheck
bun run build
```

## CLI 用法

### 轉換素材庫

`debris convert [input] [output]` 同時支援位置參數和旗標參數。若省略 `[output]` / `--output`，Debris 會依據輸入路徑和 `--to` 自動推導輸出路徑（例如 `./lib.billfish` 配合 `-t eagle` 將生成 `./lib.library`）。

```sh
# 使用位置參數將 Billfish 素材庫轉換為 Eagle 素材庫
debris convert ./lib.billfish ./lib.library -t eagle

# 將 Billfish 轉換為 Pixcall（自動識別來源格式並自動推導輸出路徑）
debris convert ./lib.billfish -t pixcall

# 使用旗標參數、硬連結模式將 Eagle 轉換為 Pixcall，並輸出 NDJSON 日誌
debris convert -t pixcall -i ./lib.library -o ./pixcall-lib --copy-mode hardlink --json

# 靜默模式將 Pixcall 轉換為 Eagle
debris convert ./pixcall-lib -t eagle -q
```

#### `convert` 參數

* `--from`, `-f`：來源格式（`billfish`、`eagle`、`pixcall`），省略時自動探測。
* `--to`, `-t`：目標格式（`eagle`、`pixcall`、`billfish`）。
* `--input`, `-i`：輸入素材庫路徑（也可作為第 1 個位置參數傳入）。
* `--output`, `-o`：輸出素材庫路徑（也可作為第 2 個位置參數傳入；省略時依據輸入路徑與 `--to` 自動推導）。
* `--copy-mode`：素材傳輸模式：`auto`（預設）、`copy` 或 `hardlink`。
* `--overwrite`：允許覆寫已存在的目標目錄內容。
* `--include-deleted`：包含已刪除 / 垃圾桶中的項目。
* `--include-hidden`：包含隱藏項目。
* `--name`：自訂目標素材庫名稱。
* `--quiet`, `-q`：在非 JSON 模式下隱藏進度輸出。
* `--json`：以 NDJSON 格式輸出進度事件與最終結果。

### 檢查素材庫

```sh
debris inspect ./lib.library
debris inspect -p ./pixcall-lib --json
```

#### `inspect` 參數

* `--path`, `-p`：待檢查的素材庫路徑（也可直接作為位置參數傳入）。
* `--format`, `-f`：來源格式（省略時自動探測）。
* `--include-deleted`：包含已刪除 / 回收站中的項目。
* `--include-hidden`：包含隱藏項目。
* `--json`：以 JSON 格式輸出檢查結果。

### 預覽轉換計畫

`debris plan [input] [output]` 支援與 `convert` 相同的來源、目標、傳輸與過濾參數（不含 `--quiet`），傳入 `--json` 時輸出 JSON 格式的預檢結果。

```sh
debris plan ./lib.billfish -t eagle
debris plan -t pixcall -i ./lib.library -o ./pixcall-lib --json
```
