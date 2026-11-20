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

## 3. 时间戳、`content_hash`（4 MiB 分块 SHA-256）、`inode`/`mtime` 与初始状态

- **时间戳格式**：`entries`、`boards`、`remote_events` 中的时间戳需以 Pixcall 原生 UTC 字符串 `YYYY-MM-DD HH:MM:SS.sss` 写入（见 [timestamp.go](../internal/format/pixcall/model/timestamp.go)），避免 GORM SQLite 默认写入 `+00:00` 时区后缀导致客户端解析异常。
- **`content_hash` 算法**：Pixcall（`crates/file_hasher`）使用 **4 MiB 分块双层 SHA-256**，而非原始单次 SHA-256 或 BLAKE3（见 [file_hash.go](../internal/format/pixcall/file_hash.go)）：
  - 将文件按 `4 * 1024 * 1024` 字节切块并分别计算每块的 32 字节原始 `SHA-256`；
  - 再对所有分块摘要顺序拼接后做一次 `SHA-256`，得到 64 位小写十六进制字符串（同时也是 `thumbs.db` 中 `thumb/<content_hash>` 的键）。
  - 因此绝不能把 Billfish 的 32 位 MD5 或其他格式的原始哈希直接写入 `content_hash`。
- **预填 `content_hash`、`inode`、磁盘 `mtime` 与 `status = 1` 的原因**：
  - Pixcall 打开素材库时，会并发启动 `FS scan` 与 `CheckEntryState` 后台任务读写 `main.db`。
  - 如果导入阶段留空 `inode = NULL`、`content_hash = NULL` 且 `status = 0`，`FS scan` 会对全部文件逐条发起 `UPDATE entries` 事务（`FS scan update with no inode file` / `FS scan update file content`），与同时在写元数据、缩略图和色板的 `CheckEntryState` 抢占 SQLite 写锁，在大库下直接触发 `Diesel(DatabaseError(Unknown, "database is locked"))`，导致部分条目失败后永久卡在 `status = 0`、`content_hash = NULL` 且界面裂图（仅“在访达中显示”正常）。
  - 由 `debris` 在导出时直接计算真实的 4 MiB 分块 SHA-256，并从目标磁盘路径写入真实 `inode`、磁盘 `mtime`（毫秒时间戳）及 `status = 1`（文件夹保持 `status = 64`），`FS scan` 扫描时即可判定全部文件与磁盘一致从而零写锁冲突，`CheckEntryState` 只需平稳补齐元数据与 `thumbs.db` 缩略图并推进到 `status = 2069`。

## 4. Snowflake 主键 `id` 的纪元与按 `created_at` 生成（导入时间排序）

**原因**：

- Pixcall 桌面端的「导入时间」排序实际上直接按主键 `entries.id`（64 位 Snowflake 整数）排序，而不是按 `entries.created_at` 字段排序。
- Pixcall 原生使用的 Snowflake 基准纪元为 **`2021-11-11T11:11:11Z`（`1636629071000` ms）**，高 41 位为相对该纪元的毫秒偏移，低 22 位为 `workerID (10 bit) | seq (12 bit)`。
- 如果 `debris` 生成 `entries.id` 时没有对齐这一机制，会出现两类排序异常：
  1. **纪元不一致**：若使用更早的纪元（如 `2020-01-01T00:00:00Z`），转换出的历史条目 `id` 数值会整体大于 Pixcall 后续正常导入生成的 `id`，导致新导入的素材在「导入时间降序」下反而排在历史迁移素材之后；
  2. **按遍历顺序使用 `time.Now()` 生成**：会导致 `entries.id` 反映的是导入扫描顺序而非资产原本的 `created_at`，迁移库内的历史素材在按「导入时间」排序时完全乱序。

**约定**（见 [idgen.go](../internal/kit/idgen.go) 与 [writer.go](../internal/format/pixcall/writer.go)）：

- `snowflakeEpochMs` 统一使用 Pixcall 原生纪元 `1636629071000`（`2021-11-11T11:11:11Z`）。
- 写入资产条目时，通过 `kit.AssignSnowflakeIDs` 依据每个资产的 `CreatedAt`（零值回退为当前时间）按时间升序生成 `entries.id`，保证 `id` 顺序与 `created_at` 严格一致，并与 Pixcall 后续新导入条目的 `id` 处于同一时间轴。
- 对于早于或等于 `2021-11-11T11:11:11Z` 的历史时间戳，按时间升序映射到 `[1000, 1<<22)` 区间，既避开根目录/回收站保留 ID（`1`、`2`），又保证严格小于所有纪元后的 ID。
- `AssignSnowflakeIDs` 使用与 `NewSnowflakeID()` 互斥的 `workerID`（翻转 bit 0），避免与文件夹、标签等条目发生主键冲突。
