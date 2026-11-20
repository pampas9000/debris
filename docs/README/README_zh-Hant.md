# Debris

**彌合設計庫格式之間的鴻溝。**

[English](../../README.md) | [简体中文](README_zh-Hans.md)

Debris 是一款跨平台桌面端 GUI 應用（基於 MyGo + SolidJS）與命令列工具，用於檢查、規劃和轉換不同設計資產庫格式。它透過統一的內部領域模型直接進行格式轉換——無需借助中間格式即可保留資料夾、標籤、標籤群組、顏色、評分、備註、來源連結以及素材檔案。

![Debris 桌面端介面](../assets/screenshot.avif)

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

## 安裝

前往 **[GitHub Releases](https://github.com/pampas9000/debris/releases/latest)** 下載對應平台的預編譯安裝包或命令列程式：

* **桌面端 GUI：**
  * **macOS**（Universal 通用版，支援 Apple Silicon 與 Intel）：`Debris-*.dmg`
  * **Windows**（`amd64` / `arm64`）：`Debris-*-setup-*.exe`
  * **Linux**（`amd64` / `arm64`）：`.deb`、`.rpm` 或 `.tar.gz`
* **命令列工具（`debris`）：**
  * 下載對應平台的 `debris-<version>-<os>-<arch>.zip` 壓縮檔，解壓後將 `debris` 加入 `PATH`；或透過 Go 直接安裝：
    ```sh
    go install github.com/pampas9000/debris@latest
    ```

> [!TIP]
> 在 macOS 上首次開啟桌面端時，如系統提示「無法驗證開發者」，可右鍵點擊 `Debris.app` 選擇**開啟**（或在**系統設定 → 隱私權與安全性**中允許執行）。

## 桌面應用

1. **選擇來源素材庫：** 在**來源素材庫**卡片中點擊**選擇目錄**，選取現有庫目錄（`.bf` / `.billfish`、`.library` 或 `.pixcall`）。Debris 會自動辨識格式並顯示資產數、資料夾數、標籤數與總大小。
2. **設定目標輸出：** 選擇**目標格式**（`Eagle` 或 `Pixcall`）、**儲存位置**（父目錄），並依需求修改**目標庫名稱（資料夾名）**。
3. **選擇傳輸模式與遷移選項：**
   * **自動**（推薦）：同磁碟優先建立硬連結節省空間，跨磁碟自動改為複製。
   * **硬連結**：不額外佔用磁碟空間（要求來源庫與目標庫位於同一磁碟）。
   * **複製**：完整複製所有素材檔案（跨磁碟最穩妥，會佔用額外磁碟空間）。
   * 依需求勾選**覆寫既有目標目錄**、**包含垃圾桶項目**或**包含隱藏項目**。
4. **預檢與轉換：** 點擊**預檢**可在寫入磁碟前預覽預計遷移數量及相容性提示；確認無誤後點擊**開始轉換**，轉換過程中可隨時點擊**取消**中斷。
5. **主題與語系：** 可透過右上角控制項切換**跟隨系統 / 淺色模式 / 深色模式**及介面語系（**繁體中文 / 简体中文 / English**）。

## 命令列用法

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

**`convert` 參數：**

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

**`inspect` 參數：**

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

## 開發與建置

### 環境相依

* **Go** `1.27+`
* **[Bun](https://bun.sh/)**（僅開發或打包桌面端 GUI 時需要）

### 專案結構

* [`cmd/`](../../cmd/) — Cobra CLI 命令實作（`convert`、`inspect`、`plan`）
* [`pkg/debris/`](../../pkg/debris/) — 對外公開的 Go 引擎 API（`Engine`、`Probe`、`Inspect`、`Plan`、`Convert`）
* [`internal/domain/`](../../internal/domain/) — 統一領域模型（`Library`、`Asset`、`Folder`、`Tag`）
* [`internal/engine/`](../../internal/engine/) — 轉換執行器、預檢規劃器與格式註冊表
* [`internal/format/`](../../internal/format/) — 各格式適配器（`eagle`、`pixcall`、`billfish`）
* [`gui/`](../../gui/) — 基於 **MyGo** + **SolidJS** + **Tailwind CSS v4** + **Vite** 的桌面端 GUI

### 建置與測試 CLI

```sh
# 執行全部 Go 測試
go test ./...

# 執行靜態檢查
go vet ./...

# 編譯 CLI 二進位檔
go build -o debris .
```

### 開發與打包桌面端 GUI

```sh
cd gui
bun install

# 開發模式啟動桌面應用（支援前端熱更新與 Go 後端聯動）
bun run dev

# 前端型別檢查
bun run typecheck

# 打包桌面端發行產物
bun run build
```

### 作為 Go 函式庫整合

可透過 [`cmd.NewDefaultEngine()`](../../cmd/root.go)（或使用 [`pkg/debris`](../../pkg/debris/) 自訂註冊適配器）在 Go 程式碼中直接呼叫 `Probe`、`Inspect`、`Plan` 與 `Convert`。
