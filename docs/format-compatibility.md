# 各平台特性支持与跨格式处理指南

本文档面向开发者，说明 Debris 支持的设计素材库格式在**特性支持范围**与**跨格式转换处理方式**上的具体行为。各格式底层存储结构与客户端细节可配合以下专题文档阅读：

- [Pixcall 素材库格式兼容说明](pixcall-compatibility.md)
- [Refto 素材库格式兼容说明](refto-compatibility.md)

---

## 1. 格式支持总览矩阵

> 图标说明：`✅` 原生读写支持；`◐` 部分支持（读取或写入存在格式限制，详见下文）；`❌` 目标格式不支持或有意不迁移。

| 特性 / 维度 | Eagle (4.0.0 build 23 / Hive) | Pixcall (0.9.7+build5) | Refto (0.1.0-beta.10) | Billfish (3.0.32) |
| :--- | :---: | :---: | :---: | :---: |
| **读取（导入） / 写入（导出）** | ✅ / ✅ | ✅ / ✅ | ✅ / ✅ | ✅ / ⚠️ 不再维护 |
| **存储模型** | JSON 元数据 + 资源子目录 | 真实磁盘目录 + SQLite | SQLite + 侧车 JSON + `items/` | 真实磁盘目录 + SQLite |
| **嵌套文件夹** | ✅ | ✅ | ✅ | ✅ |
| **单素材多文件夹归属** | ✅ | ❌（仅首文件夹） | ✅ | ❌（仅首文件夹） |
| **标签与标签组** | ✅ | ✅ | ✅ | ✅ |
| **嵌套标签（父子标签）** | ❌（展平为一级） | ❌（展平为一级） | ❌（展平为一级） | ✅ |
| **星标标签** | ✅ | ❌ | ✅ | ❌ |
| **颜色调色板** | ✅ | ✅ | ✅ | ✅ |
| **评分 / 备注 / 来源链接** | ✅ | ✅ | ✅ | ✅ |
| **回收站（`--include-deleted`）** | ✅ | ✅ | ✅ | ✅ |
| **隐藏 / 归档（`--include-hidden`）** | ◐（读取无独立字段） | ✅ | ✅（映射归档状态） | ✅ |
| **缩略图迁移** | ✅（PNG） | ❌（由客户端生成） | ✅（JPEG） | ✅ |
| **文件夹颜色 / 图标** | ◐（原生读 / 写） | ◐（9 色枚举 / 内置图标或 Emoji） | ◐（仅 `#rrggbb` / ASCII 图标名） | ◐（整型色值 / 数字 ID） |
| **文件夹封面 / 自动标签** | ❌ | ◐（封面支持） | ✅（封面与 `auto_tags`） | ◐（封面支持） |
| **AI 建议标签（`--include-suggested-tags`）** | ❌ | ❌ | ◐（默认跳过，可选包含） | ❌ |
| **外链素材（Linked）** | ❌ | ❌ | ◐（读取本地可达文件并转为托管） | ❌ |
| **高阶独有特性**（白板 / 智能文件夹 / 标注高亮 / OCR 与向量 / 文件夹密码锁） | ◐（保留普通文件夹密码字段，智能文件夹不迁移） | ❌（画板、云端同步状态不迁移） | ❌（白板、智能文件夹、标注高亮、OCR/向量、密码锁不迁移） | ❌ |

---

## 2. 跨格式转换的处理规则

Debris 通过统一的内部领域模型（[internal/domain](../internal/domain/)）直接进行库间映射，不经由第三方中间文件。当源格式与目标格式的能力不一致时，会按以下规则处理并在 `plan` 预检阶段给出提示。

### 2.1 文件传输与命名规范

- **传输模式（`--copy-mode`）**：
  - `auto`（默认）：同文件系统优先创建硬链接，跨卷或不支持硬链接时自动回退为复制；
  - `hardlink`：仅使用硬链接（跨卷会报错）；
  - `copy`：完整复制原始文件。
- **同名冲突去重**：在同一目标文件夹下出现重名文件或重名子文件夹时，自动追加序号后缀去重（如 `name (2).png`），不会静默覆盖原有资产。
- **文件名与 Unicode 规范化**：替换路径非法字符（`/`、`\`、`\x00`），并在写入 Pixcall 等依赖磁盘扫描的格式时统一将名称规范化为 **Unicode NFC**，避免 macOS APFS 下的 NFD 拆解字符引发重复入库或缩略图失效。

### 2.2 单素材多文件夹归属

- **Eagle** 与 **Refto** 支持一个素材同时属于多个文件夹；
- **Pixcall** 与 **Billfish** 以真实磁盘目录结构存储文件，每个文件只能位于单一路径下。因此将多文件夹归属的素材导出到 Pixcall（或 Billfish）时，Debris 会取 `FolderIDs` 中**第一个有效文件夹**作为实际存放目录（删除状态的素材则会放入对应回收站目录），其余关联无法保留，并在 `plan` 的 `losses` 中提前告知。

### 2.3 标签、标签组、嵌套标签与星标

- **标签组**：四种格式均支持标签组，迁移时会按组名建立映射；若目标库同组或全局存在大小写冲突或重名标签，会按目标库约束自动合并去重（例如 Refto 的 `tags.name` 大小写不敏感唯一、Pixcall 同标签组内名称唯一），避免写入报错。
- **嵌套标签**：目前仅 **Billfish** 支持父子层级标签；迁入 Eagle、Pixcall 或 Refto 时会自动展平为普通一级标签，同时保留原本所属的标签组关系。
- **星标标签**：**Eagle**（库根目录 `tags.json`）与 **Refto**（`tags.starred`）均支持星标标签并可双向保留；迁往不支持星标的格式（如 Pixcall）时忽略星标属性。

### 2.4 文件夹与标签组的颜色、图标及排序

不同软件对颜色和图标的存储格式差异较大，直接透传会导致部分客户端报错甚至崩溃。Debris 在写入目标格式时会做如下规范化：

- **颜色映射**：
  - **写入 Pixcall**：Pixcall 仅识别 9 种内置命名色（`aqua`、`blue`、`purple`、`orange`、`gray`、`yellow`、`pink`、`green`、`red`）。写入时会保留合法命名色及常见别名，将 `#rgb` / `#rrggbb` 十六进制色值按 RGB 空间欧氏距离匹配到最接近的 Pixcall 内置颜色（接近中性灰或无法识别时置为 `NULL`）。
  - **写入 Refto**：Refto 前端 UI 会将非空颜色直接作为 `#rrggbb` 解析，传入 `"blue"` 等英文颜色名会直接触发 `panic` 闪退。写入时会将已知命名色（包含 Pixcall 与常见颜色名）以及 `#rgb` / `#rrggbbaa` 统一标准化为小写 `#rrggbb`，未知值置为 `""`。
- **图标过滤**：
  - **写入 Pixcall**：仅保留内置 SVG 标识（`:<name>:`）与合法的 Emoji 十六进制码点序列（如 `1f600`、`1f576-fe0f`）；来自 Refto（如 `global`）、Eagle（如 `star`）或 Billfish（纯数字 ID）的图标标识会导致 Pixcall 前端 `String.fromCodePoint(parseInt(...))` 抛出异常并白屏，因此统一置为 `NULL`（使用默认文件夹图标）。
  - **写入 Refto**：仅保留字母开头的简洁 ASCII 图标标识（如 `global`），过滤 Pixcall `:<name>:`、Emoji 码点与纯数字 ID，避免显示异常。
- **排序序号**：读取与写入都会保持同级文件夹与标签组的相对顺序；写入 Refto 时会把来自 Pixcall 的 `9000000000000000000` 级超大 `ranking` 规范化为从 `0` 开始的连续整数序号。

### 2.5 缩略图与媒体元数据

- **Eagle ↔ Refto 缩略图互转**：Eagle 使用 `<base>_thumbnail.png`，Refto 使用 `thumbs/<last2>/<ULID>.jpg`。跨格式转换时 Debris 会自动执行 **PNG ↔ JPEG** 编解码转存，无需目标软件从头重算；若源素材未携带缩略图，写入 Refto 时标记 `thumb_state = 'pending'` 交由 Refto 后台补齐。
- **写入 Pixcall**：`Capabilities.SupportsThumbnails` 为 `false`，Debris 不向 `thumbs.db` 预写入缩略图，而是直接写入文件真实 `inode`、磁盘 `mtime` 与 4 MiB 分块 SHA-256 `content_hash`（初始 `status = 1`），由 Pixcall 启动后的后台任务平稳生成缩略图、色板与元数据，避免 SQLite 写锁冲突。
- **Pixcall `media` 表约束**：仅对音频、视频（或时长大于 `0`）的资产写入 `media` 表记录；图片与文档即便携带容器格式字段（如 Refto 的 `container: "heic" / "pdf"`）也不会写入 `media` 表。

### 2.6 回收站、隐藏/归档状态与时间戳

- **默认过滤规则**：`inspect`、`plan` 与 `convert` 默认**不包含**回收站与隐藏/归档项目，需要时通过 `--include-deleted` 和 `--include-hidden`（或桌面端对应开关）显式开启。
- **归档 / 隐藏语义映射**：Refto 的 `archived_at > 0` 在内部模型中映射为 `IsHidden = true`，可与 Pixcall（`is_hidden`）及 Billfish 隐藏状态互通；由于 Eagle 没有独立的隐藏字段，启用 `--include-hidden` 迁入 Eagle 时会作为常规活跃素材保留。
- **颜色调色板与时间排序**：
  - 内部统一使用 RGB（`0..255`）与占比百分比（`0..100`），写入 Refto 时自动换算为 `0..1` 占比、OKLab 坐标与主色 `#rrggbb`；
  - 写入 Pixcall 时，按照资产 `CreatedAt` 顺序对齐 Pixcall 原生 Snowflake 纪元生成 `entries.id`，保证“按导入时间排序”与源库一致；写入 Refto 时按 `CreatedAt` 升序生成 ULID 并写入毫秒级 `added_at` / `updated_at`。

---

## 3. 各平台支持详情与注意事项

### 3.1 Eagle（4.0.0 build 23 / Hive 兼容）

- **结构**：以库根目录 `metadata.json`、`tags.json`、`mtime.json` 及 `images/<ID>.info/`（内含资产文件、`metadata.json` 与可选 `_thumbnail.png`）组织；兼容同结构的 Hive 素材库。
- **已支持**：嵌套文件夹（含描述、标签绑定、`modificationTime`、图标/颜色及文件夹密码字段透传）、多文件夹归属、标签与标签组、星标标签、调色板、评分、备注、来源 URL、回收站与缩略图迁移。
- **限制与不迁移项**：无独立的素材隐藏状态；智能文件夹（`smartFolders`）暂不迁移。

### 3.2 Pixcall（0.9.7+build5）

- **结构**：以真实文件夹树存储磁盘文件，元数据位于 `.pixcall/database/main.db`、`thumbs.db`、`config.json`、`<uuid>.id` 与加密 `keystore`。
- **已支持**：嵌套文件夹（含布局、排序与封面关联）、标签与标签组、拼音索引、调色板、评分、备注、来源 URL、EXIF、音视频媒体元数据、回收站与隐藏状态。
- **限制与不迁移项**：单文件仅支持单一文件夹归属；不导入源缩略图（由 Pixcall 打开后生成）；无星标标签；画板（`boards` / `board_entries`）、`captions` 与云端同步事件（`remote_events`）暂不迁移。
- **细节参考**：[Pixcall 素材库格式兼容说明](pixcall-compatibility.md)。

### 3.3 Refto（0.1.0-beta.10）

- **结构**：由 `act-library.json`、`act.db`（SQLite WAL + FTS5 全文索引）、`tags.json`、`items/<ULID>/`（文件与 `item.json`）及 `thumbs/<last2>/<ULID>.jpg` 组成。
- **已支持**：嵌套文件夹（含描述、封面、自动标签及手动排序）、多文件夹归属、标签与标签组（含拼音搜索索引）、星标标签、OKLab 调色板与主色、评分、备注、来源 URL、EXIF/媒体信息、回收站、归档（映射为隐藏）、AI 建议标签（可选）与缩略图迁移。
- **特殊处理与不迁移项**：
  - **AI 建议标签**：`item_tags.state = 'suggested'` 默认不会迁移，会在 `inspect` / `plan` 中提示数量，传入 `--include-suggested-tags` 或在桌面端点击包含即可一并导出为已确认标签；
  - **外链资产（`storage = 'linked'`）**：读取时校验 `abs_path` 可达性并跳过缺失文件，写入时统一导出为自包含托管库（`storage = 'managed'`）；
  - **明确不迁移项**：白板（`boards` / `board_nodes`）、智能文件夹（`smart_folders`）与保存筛选（`saved_filters`）、标注（`annotations`）与文本高亮（`highlights`）、OCR 与摘要文本（`texts`）、AI 向量与视觉/语音指纹（`segments` / `segment_vec768` / `vision` / `speech`）、文件夹密码锁（`folder_locks`）。
- **细节参考**：[Refto 素材库格式兼容说明](refto-compatibility.md)。

### 3.4 Billfish（3.0.32）

- **定位**：由于 Billfish 已停止更新维护，Debris 聚焦于**从 Billfish 无损迁出**到 Eagle、Pixcall 或 Refto（导出为 Billfish 的写入器保留但不再积极维护）。
- **已支持（读取）**：嵌套文件夹（含封面与关联标签）、标签与标签组、嵌套父子标签、调色板、评分、备注、来源 URL、回收站（`bf_recycle`）、隐藏状态与缩略图。
- **特殊处理**：Billfish 原生的嵌套标签在迁出到其他格式时会自动展平；库内文件名的 macOS NFD 字符串在导出至 Pixcall 等格式时会自动转为 NFC。
