# Debris

**弥合设计库格式之间的鸿沟。**

[English](../../README.md) | [繁體中文](README_zh-Hant.md)

Debris 是一款跨平台命令行工具与桌面端 GUI 应用（基于 MyGo + SolidJS 构建），用于检查、规划和转换不同设计资产库格式。它通过统一的内部领域模型直接进行格式转换——无需借助中间格式即可保留文件夹、标签、标签组、颜色、评分、备注、来源链接以及素材文件。

## 支持的格式

| 格式 | 读取（导入） | 写入（导出） | 说明 |
| :--- | :---: | :---: | :--- |
| **Eagle** (`.library`, 4.x) | ✅ | ✅ | 已测试验证可用（含 Hive 兼容） |
| **Pixcall** (`.pixcall`, v22) | ✅ | ✅ | 已测试验证可用（详见 [Pixcall 兼容说明](../pixcall-compatibility.md)） |
| **Billfish** (`.bf`) | ✅ | ⚠️ 不再维护 | 支持从 Billfish 迁出。由于 Billfish 已停止更新与维护，导出为 Billfish 不再积极维护。 |

## 功能特性

* **桌面端 GUI（MyGo + SolidJS）：** 原生跨平台桌面应用（支持 macOS、Windows 和 Linux），提供目录自动探测与库统计预览、转换预检（含兼容损耗提示）、实时进度展示与随时取消、跟随系统/浅色/深色主题切换，以及多语言界面（简体中文、繁體中文、English）。
* **跨格式直接转换：** 在支持的格式之间直接转换（例如 `billfish` → `eagle`、`billfish` → `pixcall`、`eagle` ↔ `pixcall`）。
* **自动识别与智能输出路径：** 省略 `--from` / `--format` 时自动探测输入素材库格式；省略 `--output` 时根据输入路径与 `--to` 自动推导目标输出目录。
* **检查与转换规划：** 使用 `inspect` 查看素材库摘要；使用 `plan` 在实际转换前预览资产数量、警告信息以及潜在的兼容性损失。
* **灵活的素材传输模式：** 支持 `auto`（默认，同磁盘优先硬链接，跨磁盘自动回退为复制）、`hardlink` 和 `copy`（`--copy-mode`），并提供 `--overwrite`、`--include-deleted`、`--include-hidden` 和 `--name` 等选项。
* **脚本友好输出：** 支持 TTY 行内进度展示、`--quiet`（`-q`）静默进度，以及通过 `--json` 输出结构化 JSON / NDJSON 日志。
* **跨平台兼容：** 基于 Go（`1.27+`）构建，支持 macOS、Linux 和 Windows。

## 桌面端 GUI

桌面端代码位于 [`gui/`](../../gui/)，使用 **MyGo** 与 **SolidJS**（Tailwind CSS v4 + TypeScript + Vite）开发。

可在 Releases 页面直接下载预编译安装包（macOS `.dmg`、Windows 安装包 `.exe`、Linux `.deb` / `.rpm` / `.tar.gz`）。

### 从源码运行与打包

需要安装 Go（`1.27+`）和 [Bun](https://bun.sh/)。

```sh
cd gui
bun install

# 开发模式启动桌面应用
bun run dev

# 类型检查与打包桌面应用
bun run typecheck
bun run build
```

## CLI 用法

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

#### `convert` 参数

* `--from`, `-f`：来源格式（`billfish`、`eagle`、`pixcall`），省略时自动探测。
* `--to`, `-t`：目标格式（`eagle`、`pixcall`、`billfish`）。
* `--input`, `-i`：输入素材库路径（也可作为第 1 个位置参数传入）。
* `--output`, `-o`：输出素材库路径（也可作为第 2 个位置参数传入；省略时根据输入路径与 `--to` 自动推导）。
* `--copy-mode`：素材传输模式：`auto`（默认）、`copy` 或 `hardlink`。
* `--overwrite`：允许覆盖已存在的目标目录内容。
* `--include-deleted`：包含已删除 / 回收站中的项目。
* `--include-hidden`：包含隐藏项目。
* `--name`：自定义目标素材库名称。
* `--quiet`, `-q`：在非 JSON 模式下隐藏进度输出。
* `--json`：以 NDJSON 格式输出进度事件与最终结果。

### 检查素材库

```sh
debris inspect ./lib.library
debris inspect -p ./pixcall-lib --json
```

#### `inspect` 参数

* `--path`, `-p`：待检查的素材库路径（也可直接作为位置参数传入）。
* `--format`, `-f`：来源格式（省略时自动探测）。
* `--include-deleted`：包含已删除 / 回收站中的项目。
* `--include-hidden`：包含隐藏项目。
* `--json`：以 JSON 格式输出检查结果。

### 预览转换计划

`debris plan [input] [output]` 支持与 `convert` 相同的来源、目标、传输与过滤参数（不含 `--quiet`），传入 `--json` 时输出 JSON 格式的预检结果。

```sh
debris plan ./lib.billfish -t eagle
debris plan -t pixcall -i ./lib.library -o ./pixcall-lib --json
```
