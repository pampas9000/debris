# Debris

**弥合设计库格式之间的鸿沟。**

[English](../../README.md) | [繁體中文](README_zh-Hant.md)

Debris 是一款跨平台桌面端 GUI 应用（基于 MyGo + SolidJS）与命令行工具，用于检查、规划和转换不同设计资产库格式。它通过统一的内部领域模型直接进行格式转换——无需借助中间格式即可保留文件夹、标签、标签组、颜色、评分、备注、来源链接以及素材文件。

![Debris 桌面端界面](../assets/screenshot_zh-Hans.avif)

## 支持的格式

| 格式 | 读取（导入） | 写入（导出） | 说明 |
| :--- | :---: | :---: | :--- |
| **Eagle** (`.library`, 4.x) | ✅ | ✅ | 已测试验证可用（含 Hive 兼容） |
| **Pixcall** (`.pixcall`, v22) | ✅ | ✅ | 已测试验证可用（详见 [Pixcall 兼容说明](../pixcall-compatibility.md)） |
| **Refto** (`.reflib`, v25 / Beta) | ✅ | ✅ | 已测试验证可用（详见 [Refto 兼容说明](../refto-compatibility.md)） |
| **Billfish** (`.bf`) | ✅ | ⚠️ 不再维护 | 支持从 Billfish 迁出。由于 Billfish 已停止更新与维护，导出为 Billfish 不再积极维护。 |

关于各平台支持的特性矩阵以及跨格式迁移时的兼容处理细节，详见 [各平台特性支持与跨格式处理指南](../format-compatibility.md)。

## 功能特性

* **桌面端 GUI（MyGo + SolidJS）：** 原生跨平台桌面应用（支持 macOS、Windows 和 Linux），提供目录自动探测与库统计预览、转换预检（含兼容损耗提示）、实时进度展示与随时取消、跟随系统/浅色/深色主题切换，以及多语言界面（简体中文、繁體中文、English）。
* **跨格式直接转换：** 在支持的格式之间直接转换（例如 `eagle` ↔ `pixcall` ↔ `refto`、`billfish` → `eagle` / `pixcall` / `refto`）。
* **自动识别与智能输出路径：** 省略 `--from` / `--format` 时自动探测输入素材库格式；省略 `--output` 时根据输入路径与 `--to` 自动推导目标输出目录。
* **检查与转换规划：** 使用 `inspect` 查看素材库摘要；使用 `plan` 在实际转换前预览资产数量、警告信息以及潜在的兼容性损失。
* **灵活的素材传输模式：** 支持 `auto`（默认，同磁盘优先硬链接，跨磁盘自动回退为复制）、`hardlink` 和 `copy`（`--copy-mode`），并提供 `--overwrite`、`--include-deleted`、`--include-hidden`、`--include-suggested-tags` 和 `--name` 等选项。
* **脚本友好输出：** 支持 TTY 行内进度展示、`--quiet`（`-q`）静默进度，以及通过 `--json` 输出结构化 JSON / NDJSON 日志。

## 安装

前往 **[GitHub Releases](https://github.com/pampas9000/debris/releases/latest)** 下载对应平台的预编译安装包或命令行程序：

* **桌面端 GUI：**
  * **macOS**（Universal 通用包，支持 Apple Silicon 与 Intel）：`Debris-*.dmg`
  * **Windows**（`amd64` / `arm64`）：`Debris-*-setup-*.exe`
  * **Linux**（`amd64` / `arm64`）：`.deb`、`.rpm` 或 `.tar.gz`
* **命令行工具（`debris`）：**
  * 下载对应平台的 `debris-<version>-<os>-<arch>.zip` 压缩包，解压后将 `debris` 放入 `PATH`；或通过 Go 直接安装：
    ```sh
    go install github.com/pampas9000/debris@latest
    ```

> [!TIP]
> 在 macOS 上首次打开桌面端时，如系统提示“无法验证开发者”，可右键点击 `Debris.app` 选择**打开**（或在**系统设置 → 隐私与安全性**中允许运行）。

## 桌面应用

1. **选择源素材库：** 在**源素材库**卡片中点击**选择目录**，选中现有库目录（`.bf` / `.billfish`、`.library`、`.pixcall` 或 `.reflib`）。Debris 会自动识别格式并展示资产数、文件夹数、标签数与总体积。
2. **设置目标输出：** 选择**目标格式**（`Eagle`、`Pixcall` 或 `Refto`）、**保存位置**（父目录），并按需修改**目标库名称（文件夹名）**。
3. **选择传输模式与迁移选项：**
   * **自动**（推荐）：同磁盘优先创建硬链接节省空间，跨磁盘自动回退为复制。
   * **硬链接**：不额外占用磁盘空间（要求源库与目标库位于同一磁盘）。
   * **复制**：完整复制所有素材文件（跨盘最稳妥，会占用额外磁盘空间）。
   * 按需勾选**覆盖已有目标目录**、**包含回收站项目**、**包含隐藏项目**或**包含 AI 建议标签**（当来源格式支持时显示）。
4. **预检与转换：** 点击**预检**可在写入磁盘前预览预计迁移数量及兼容性提示；确认无误后点击**开始转换**，转换过程中可随时点击**取消**中断。
5. **主题与语言：** 可通过右上角控件切换**跟随系统 / 浅色模式 / 深色模式**及界面语言（**简体中文 / 繁體中文 / English**）。

## 命令行用法

### 转换素材库

`debris convert [input] [output]` 同时支持位置参数和标志参数。若省略 `[output]` / `--output`，Debris 会根据输入路径和 `--to` 自动推导输出路径（例如 `./lib.billfish` 配合 `-t eagle` 将生成 `./lib.library`）。

```sh
# 使用位置参数将 Billfish 素材库转换为 Eagle 素材库
debris convert ./lib.billfish ./lib.library -t eagle

# 将 Billfish 转换为 Pixcall（自动识别来源格式并自动推导输出路径）
debris convert ./lib.billfish -t pixcall

# 使用标志参数、硬链接模式将 Eagle 转换为 Pixcall，并输出 NDJSON 日志
debris convert -t pixcall -i ./lib.library -o ./pixcall-lib --copy-mode hardlink --json

# 静默模式将 Pixcall 转换为 Eagle
debris convert ./pixcall-lib -t eagle -q
```

**`convert` 参数：**

* `--from`, `-f`：来源格式（`billfish`、`eagle`、`pixcall`、`refto`），省略时自动探测。
* `--to`, `-t`：目标格式（`eagle`、`pixcall`、`refto`、`billfish`）。
* `--input`, `-i`：输入素材库路径（也可作为第 1 个位置参数传入）。
* `--output`, `-o`：输出素材库路径（也可作为第 2 个位置参数传入；省略时根据输入路径与 `--to` 自动推导）。
* `--copy-mode`：素材传输模式：`auto`（默认）、`copy` 或 `hardlink`。
* `--overwrite`：允许覆盖已存在的目标目录内容。
* `--include-deleted`：包含已删除 / 回收站中的项目。
* `--include-hidden`：包含隐藏 / 归档项目。
* `--include-suggested-tags`：包含源格式（如 Refto）中的 AI 建议（未确认）标签。
* `--name`：自定义目标素材库名称。
* `--quiet`, `-q`：在非 JSON 模式下隐藏进度输出。
* `--json`：以 NDJSON 格式输出进度事件与最终结果。

### 检查素材库

```sh
debris inspect ./lib.library
debris inspect -p ./pixcall-lib --json
```

**`inspect` 参数：**

* `--path`, `-p`：待检查的素材库路径（也可直接作为位置参数传入）。
* `--format`, `-f`：来源格式（省略时自动探测）。
* `--include-deleted`：包含已删除 / 回收站中的项目。
* `--include-hidden`：包含隐藏 / 归档项目。
* `--include-suggested-tags`：包含源格式（如 Refto）中的 AI 建议标签。
* `--json`：以 JSON 格式输出检查结果。

### 预览转换计划

`debris plan [input] [output]` 支持与 `convert` 相同的来源、目标、传输与过滤参数（不含 `--quiet`），传入 `--json` 时输出 JSON 格式的预检结果。

```sh
debris plan ./lib.billfish -t eagle
debris plan -t pixcall -i ./lib.library -o ./pixcall-lib --json
```

## 开发与构建

### 环境依赖

* **Go** `1.27+`
* **[Bun](https://bun.sh/)**（仅开发或打包桌面端 GUI 时需要）

### 项目结构

* [`cmd/`](../../cmd/) — Cobra CLI 命令实现（`convert`、`inspect`、`plan`）
* [`pkg/debris/`](../../pkg/debris/) — 对外暴露的 Go 引擎 API（`Engine`、`Probe`、`Inspect`、`Plan`、`Convert`）
* [`internal/domain/`](../../internal/domain/) — 统一领域模型（`Library`、`Asset`、`Folder`、`Tag`）
* [`internal/engine/`](../../internal/engine/) — 转换执行器、预检规划器与格式注册表
* [`internal/format/`](../../internal/format/) — 各格式适配器（`eagle`、`pixcall`、`refto`、`billfish`）
* [`gui/`](../../gui/) — 基于 **MyGo** + **SolidJS** + **Tailwind CSS v4** + **Vite** 的桌面端 GUI

### 构建与测试 CLI

```sh
# 运行全部 Go 测试
go test ./...

# 运行静态检查
go vet ./...

# 编译 CLI 二进制
go build -o debris .
```

### 开发与打包桌面端 GUI

```sh
cd gui
bun install

# 开发模式启动桌面应用（支持前端热更新与 Go 后端联动）
bun run dev

# 前端类型检查
bun run typecheck

# 打包桌面端发行产物
bun run build
```

### 作为 Go 库集成

可通过 [`cmd.NewDefaultEngine()`](../../cmd/root.go)（或使用 [`pkg/debris`](../../pkg/debris/) 自定义注册适配器）在 Go 代码中直接调用 `Probe`、`Inspect`、`Plan` 与 `Convert`。
