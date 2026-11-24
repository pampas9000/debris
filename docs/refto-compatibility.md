# Refto 素材库格式兼容说明

本文档记录 `debris` 在读取与生成 Refto 素材库（`.reflib`，`act.db` `PRAGMA user_version = 25`）时遵循的结构与兼容性约定，便于后续维护与版本演进。

## 1. 目录与核心元数据文件

Refto 素材库目录（`.reflib`）采用“SQLite 中心库 + JSON 侧车 + 分桶缩略图”的混合结构：

- `act-library.json`：库级清单（`schema: 1`、26 位 ULID `id`、`name`、`createdAt`、`appVersion`、`storage`）。
- `act.db`：核心 SQLite 数据库（`PRAGMA journal_mode = WAL`、`PRAGMA user_version = 25`），包含 `items`、`folders`、`item_folders`、`tag_groups`、`tags`、`item_tags`、`palettes`、`items_fts`（FTS5 全文索引）与 `listing_gen` 等 30+ 张表及触发器。
- `tags.json`：标签与标签组侧车文件（含 `pinyin` 搜索索引与 `starred`）。
- `items/<ULID>/`：每个托管素材对应一个目录，包含原始文件与 `item.json` 侧车元数据。
- `thumbs/<ID末两位小写>/<ULID>.jpg`：按素材 ID 末两位十六进制/Base32 字符分桶存放的 JPEG 缩略图。

## 2. 关键数据库约束与兼容性契约

1. **SQLite 3.43+（`contentless_delete = 1`）**：
   - `act.db` 中的 `items_fts` 虚拟表使用 `content = '', contentless_delete = 1` 选项。
   - `debris` 已通过 [internal/kit/gormsqlite](../internal/kit/gormsqlite) 升级为纯 Go `modernc.org/sqlite`（内含 SQLite 3.50.4），确保建表与 `DELETE FROM items` 触发器在全平台无 CGo 正常执行。
2. **`listing_gen` 触发器前置初始化**：
   - `items`、`folders`、`tags`、`item_folders`、`item_tags` 的增删改触发器均会执行 `UPDATE listing_gen SET n = n + 1`。
   - 写入库时必须在建表后立即执行 `INSERT OR IGNORE INTO listing_gen (id, n) VALUES (1, 1)`。
3. **调色板颜色与 OKLab 坐标**：
   - `palettes.rgb` 为 24 位打包整数 `(R << 16) | (G << 8) | B`；`items.primary_color` 为小写 `#rrggbb`。
   - `palettes.l / a / b` 为 **OKLab** 色彩空间坐标，而非 CIE Lab。
   - `palettes.ratio` 在 Refto 中为 `0..1` 区间（Debris 内部领域模型统一为 `0..100` 百分比，读写时自动换算）。
4. **AI 建议标签（`item_tags.state`）与存储模式**：
   - Refto 会为图片/视频自动生成 `state = 'suggested'` 的未确认标签（及 `suggestion_tags` 表）。
   - Debris 默认仅导出/统计 `state = 'accepted'` 的已确认标签；可通过 `--include-suggested-tags`（或 GUI 对应选项）显式包含建议标签。
   - `storage = 'linked'` 的外链资产从 `abs_path` 读取，缺失或不可达时在 `inspect` / `plan` 告警并跳过；写入端统一生成自包含的 `storage = 'managed'` 库。
5. **归档状态与缩略图格式转换**：
   - Refto `items.archived_at > 0` 映射为 Debris `IsHidden = true`，复用 `--include-hidden`。
   - Refto 缩略图（`.jpg`）与 Eagle 缩略图（`_thumbnail.png`）在跨格式转换时会自动通过标准库进行 PNG ↔ JPEG 互转；未携带源缩略图时置 `thumb_state = 'pending'` 由 Refto 自行生成。
6. **文件夹/标签颜色、图标、排序与时间戳精度**：
   - Refto 桌面端 UI（`mygo/ui`）会直接将非空的 `folders.color`、`tag_groups.color` 与 `tags.color` 传入 `ui.Hex(...)`，遇到 `"blue"`、`"red"` 等非十六进制命名颜色时会触发 `panic: ui: invalid color` 导致软件启动闪退。因此 [writer.go](../internal/format/refto/writer.go) 会将已知命名颜色（含 Pixcall/Eagle 色板名）与合法 `#rgb` / `#rrggbb` / `#rrggbbaa` 统一规范化为小写 `#rrggbb`（无法识别的置为 `""`）。
   - 同时会过滤外部格式的 `folders.icon`（如 Pixcall `:script:`、Emoji 十六进制码点或 Billfish 纯数字 ID），将 `folders.position` 与 `tag_groups.position` 规范化为从 `0` 开始的连续序号，并在补写 `folders.cover_item_id` 时使用 `UpdateColumn` 配合关闭 GORM 自动时间戳钩子，避免将 13 位毫秒级 `updated_at` 覆盖为 10 位秒级时间戳。
7. **Beta 版本说明**：
   - Refto 当前处于 `0.1.0-beta.10`（`user_version = 25`）阶段，若未来官方升级 `user_version` 导致表结构变化，需同步检查 [schema.go](../internal/format/refto/model/schema.go)。
