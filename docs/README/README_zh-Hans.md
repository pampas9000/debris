# Debris

**弥合设计库格式之间的鸿沟。**

[English](../../README.md) | [繁體中文](README_zh-Hant.md)

Debris 是一款跨平台命令行工具（同时提供 JSON-RPC 2.0 stdio 服务），用于检查、规划和转换不同设计资产库格式。它通过统一的内部领域模型直接进行格式转换——无需借助中间格式即可保留文件夹、标签、标签组、颜色、评分、备注、来源链接以及素材文件。

## 支持的格式

| 格式 | 读取（导入） | 写入（导出） | 说明 |
| :--- | :---: | :---: | :--- |
| **Eagle** (`.library`, 4.x) | ✅ | ✅ | 已测试验证可用（含 Hive 兼容） |
| **Pixcall** (`.pixcall`, v22) | ✅ | ✅ | 已测试验证可用（详见 [Pixcall 兼容说明](../pixcall-compatibility.md)） |
| **Billfish** (`.bf`) | ✅ | ⚠️ 不再维护 | 支持从 Billfish 迁出。由于 Billfish 已停止更新与维护，导出为 Billfish 不再积极维护。 |

## 功能特性

* **跨格式直接转换：** 在支持的格式之间直接转换（例如 `billfish` → `eagle`、`billfish` → `pixcall`、`eagle` ↔ `pixcall`）。
* **来源格式自动识别：** 省略 `--from` / `--format` 时，Debris 会自动探测输入素材库的格式。
* **检查与转换规划：** 使用 `inspect` 查看素材库摘要；使用 `plan` 在实际转换前预览资产数量、警告信息以及潜在的兼容性损失。
* **灵活的素材传输模式：** 支持 `copy`、`hardlink` 和 `auto`（`--copy-mode`），并提供 `--overwrite`、`--include-deleted`、`--include-hidden` 和 `--name` 等选项。
* **机器可读输出：** 通过 `--json` 输出 NDJSON 进度事件与最终结果。
* **跨平台兼容：** 基于 Go（`1.27+`）构建，支持 macOS、Linux 和 Windows。

## 用法

### 转换素材库

```sh
# 将 Billfish 素材库转换为 Eagle 素材库
debris convert --from billfish --to eagle --input ./lib.billfish --output ./lib.library

# 将 Billfish 素材库转换为 Pixcall 素材库（自动识别来源格式）
debris convert -t pixcall -i ./lib.billfish -o ./pixcall-lib

# 使用硬链接将 Eagle 转换为 Pixcall，并输出 NDJSON 日志
debris convert -t pixcall -i ./lib.library -o ./pixcall-lib --copy-mode hardlink --json

# 将 Pixcall 素材库转换为 Eagle 素材库
debris convert -t eagle -i ./pixcall-lib -o ./lib.library
```

#### 常用 `convert` 参数

* `--from`, `-f`：来源格式（`billfish`、`eagle`、`pixcall`），省略时自动探测。
* `--to`, `-t`：目标格式（`eagle`、`pixcall`）。
* `--input`, `-i`：输入素材库目录路径。
* `--output`, `-o`：输出素材库目录路径。
* `--copy-mode`：素材传输模式：`auto`（默认）、`copy` 或 `hardlink`。
* `--overwrite`：允许覆盖已存在的目标目录内容。
* `--include-deleted`：包含已删除 / 回收站中的项目。
* `--include-hidden`：包含隐藏项目。
* `--name`：自定义目标素材库名称。
* `--json`：以 NDJSON 格式输出进度事件与最终结果。

### 检查素材库

```sh
debris inspect --path ./lib.library
debris inspect -p ./pixcall-lib --json
```

### 预览转换计划

```sh
debris plan --to pixcall --input ./lib.library --output ./pixcall-lib
```
