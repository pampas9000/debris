# Pixcall 素材库格式兼容说明

本文档记录 `debris` 在生成 Pixcall 素材库（`schema_version = 22`）时必须满足的关键格式约定，及各自背后的原因，便于后续维护与排查。

## 1. 目录与核心元数据文件

Pixcall 桌面端校验与打开素材库时（`ValidateLibrary` / `open_metadata`），`.pixcall/` 目录下必须包含：

- `<library_id>.id`：空标记文件，文件名 UUID 即该库的 `library_id`。
- `keystore` 与 `keystore.backup`：使用固定密钥以 RFC 8439 ChaCha20-Poly1305 加密后的 JSON 元数据（载荷为 `ciphertext || tag(16) || nonce(12)`），其中的 `library_id` 必须与 `<library_id>.id` 一致，且与 `main.db` 中 `kvs["app.installation_id"]` 区分开。
- `config.json`：库级配置 JSON。
- `database/main.db`：主元数据 SQLite 数据库（`kvs["schema_version"] = '22'`）。
- `database/thumbs.db`：缩略图 SQLite 数据库（`thumbnails` 表与 `kvs` 表，且 `kvs["schema_version"] = '2'`）。导入插件与客户端均会打开该库；缺失会导致库校验或缩略图存取失败。

相关实现见 [keystore.go](../internal/format/pixcall/keystore.go)、[schema.go](../internal/format/pixcall/model/schema.go) 与 [writer.go](../internal/format/pixcall/writer.go)。

## 2. `entries.name` 必须统一为 Unicode NFC（重要）

**原因**：

- 在 macOS（APFS/HFS+）下，操作系统及上游素材库（如 Billfish）通过目录读取到的包含日文浊音/半浊音等组合字符的文件名往往是 **Unicode NFD 拆解形式**（例如 `ナヒーダ` 为 `タ + U+3099`，`おめでとう` 为 `て + U+3099`）。
- Pixcall 在首次打开素材库并运行磁盘扫描（`FS scan`）时，会先将磁盘文件名**规范化为 Unicode NFC**（如 `ナヒーダ`、`おめでとう`），再按 `(parent_id, name)` 到 `main.db` 的 `entries` 表匹配记录。
- 如果 `debris` 将 NFD 字符串写入 `entries.name`（且导入阶段未预填 `inode`），会导致：
  1. `FS scan` 匹配不到预写入的 NFD 记录，把磁盘上的同一个文件当成新文件重新创建一条 **NFC 重复记录**，使总文件数膨胀；
  2. 原 NFD 记录因为 `inode` 为 `NULL`，被 Pixcall 跳过清理（日志出现 `FS scan skip deleting entry without inode`），作为 `status = 0`、`content_hash = NULL` 的“幽灵记录”残留在库中；
  3. Pixcall UI 通过 `thumbs.db` 中的 `thumb/<content_hash>` 加载缩略图，幽灵记录因为没有 `content_hash` 无法显示缩略图；同时因为 `debris` 已导入尺寸/色板，且 APFS 在打开路径时兼容 NFC/NFD，所以会出现“**属性栏有信息、访达也能定位到文件，但界面没有缩略图**”的异常。

**约定**：[writer.go](../internal/format/pixcall/writer.go) 中的 `sanitizeEntryName` 对所有文件夹与文件条目统一执行 `norm.NFC.String(...)`，保证落库名称与去重判断均采用 Unicode NFC。

## 3. 时间戳、哈希与初始状态

- **时间戳格式**：`entries`、`boards`、`remote_events` 中的时间戳需以 Pixcall 原生 UTC 字符串 `YYYY-MM-DD HH:MM:SS.sss` 写入（见 [timestamp.go](../internal/format/pixcall/model/timestamp.go)），避免 GORM SQLite 默认写入 `+00:00` 时区后缀导致客户端解析异常。
- **`content_hash`**：Pixcall 使用 **64 位小写十六进制 BLAKE3** 作为 `content_hash` 与 `thumbs.db` 的索引键 `thumb/<content_hash>`。遇到非 64 位十六进制哈希（如 Billfish 的 32 位 MD5）时必须写 `NULL`，由 Pixcall 自行计算。
- **`status`**：由于 `debris` 导出时不直接生成 WebP 缩略图与 BLAKE3 哈希，文件条目初始 `status` 必须写为 `0`（文件夹写 `64`），让 Pixcall 后台任务自动补算元数据、哈希与缩略图并推进到就绪状态。
