package model

import (
	"fmt"

	"gorm.io/gorm"
)

const (
	// SchemaVersion is the Refto act.db PRAGMA user_version supported by this adapter.
	SchemaVersion = 25
	// ManifestSchema is the act-library.json schema version.
	ManifestSchema = 1
	// TagsSidecarVersion is the tags.json version.
	TagsSidecarVersion = 1
	// ItemSidecarVersion is the items/<id>/item.json version.
	ItemSidecarVersion = 1
	// DefaultAppVersion is the Refto application version written into act-library.json.
	DefaultAppVersion = "0.1.0-beta.10"
)

var schemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS items (
  rid           INTEGER PRIMARY KEY,
  id            TEXT NOT NULL UNIQUE,
  kind          TEXT NOT NULL CHECK (kind IN ('image','video','audio','pdf','office','text','web','font','color','other')),
  ext           TEXT NOT NULL DEFAULT '',
  mime          TEXT NOT NULL DEFAULT '',
  name          TEXT NOT NULL,
  file_name     TEXT NOT NULL DEFAULT '',
  storage       TEXT NOT NULL CHECK (storage IN ('managed','linked')),
  rel_path      TEXT NOT NULL DEFAULT '',
  abs_path      TEXT NOT NULL DEFAULT '',
  size          INTEGER NOT NULL DEFAULT 0,
  hash          TEXT NOT NULL DEFAULT '',
  fingerprint   TEXT NOT NULL DEFAULT '',
  width         INTEGER NOT NULL DEFAULT 0,
  height        INTEGER NOT NULL DEFAULT 0,
  duration      REAL NOT NULL DEFAULT 0,
  pages         INTEGER NOT NULL DEFAULT 0,
  meta          TEXT NOT NULL DEFAULT '',
  rating        INTEGER NOT NULL DEFAULT 0 CHECK (rating BETWEEN 0 AND 5),
  note          TEXT NOT NULL DEFAULT '',
  source_url    TEXT NOT NULL DEFAULT '',
  source_app    TEXT NOT NULL DEFAULT '',
  file_ctime    INTEGER,
  file_mtime    INTEGER,
  added_at      INTEGER NOT NULL,
  updated_at    INTEGER NOT NULL,
  deleted_at    INTEGER,
  primary_color TEXT NOT NULL DEFAULT '',
  thumb_state   TEXT NOT NULL DEFAULT 'pending' CHECK (thumb_state IN ('pending','ready','failed','none')),
  thumb_ext     TEXT NOT NULL DEFAULT '',
  thumb_w       INTEGER NOT NULL DEFAULT 0,
  thumb_h       INTEGER NOT NULL DEFAULT 0,
  ai_state      TEXT NOT NULL DEFAULT 'pending',
  missing       INTEGER NOT NULL DEFAULT 0,
  tags_text     TEXT NOT NULL DEFAULT '',
  pinned_at     INTEGER NOT NULL DEFAULT 0,
  cover         TEXT NOT NULL DEFAULT '',
  live          TEXT NOT NULL DEFAULT '',
  live_id       TEXT NOT NULL DEFAULT '',
  live_duration REAL NOT NULL DEFAULT 0,
  original_added_at INTEGER,
  archived_at   INTEGER NOT NULL DEFAULT 0,
  stack_id      TEXT NOT NULL DEFAULT '',
  stack_pos     INTEGER NOT NULL DEFAULT 0
)`,
	`CREATE INDEX IF NOT EXISTS items_hash ON items(hash) WHERE hash != ''`,
	`CREATE INDEX IF NOT EXISTS items_added ON items(deleted_at, added_at)`,
	`CREATE INDEX IF NOT EXISTS items_kind ON items(kind, deleted_at)`,
	`CREATE INDEX IF NOT EXISTS items_abs_path ON items(abs_path) WHERE abs_path != ''`,
	`CREATE INDEX IF NOT EXISTS items_pinned ON items(pinned_at) WHERE pinned_at > 0`,
	`CREATE INDEX IF NOT EXISTS items_live_id ON items(live_id) WHERE live_id != ''`,
	`CREATE INDEX IF NOT EXISTS items_cover ON items(deleted_at) WHERE cover != ''`,
	`CREATE INDEX IF NOT EXISTS items_view ON items(deleted_at, archived_at, pinned_at, added_at)`,
	`CREATE INDEX IF NOT EXISTS items_id_live ON items(id, deleted_at, archived_at)`,
	`CREATE INDEX IF NOT EXISTS items_stack ON items(stack_id, stack_pos) WHERE stack_id != ''`,

	`CREATE TABLE IF NOT EXISTS folders (
  id            TEXT PRIMARY KEY,
  parent_id     TEXT REFERENCES folders(id) ON DELETE CASCADE,
  position      INTEGER NOT NULL DEFAULT 0,
  name          TEXT NOT NULL,
  pinyin        TEXT NOT NULL DEFAULT '',
  description   TEXT NOT NULL DEFAULT '',
  icon          TEXT NOT NULL DEFAULT '',
  color         TEXT NOT NULL DEFAULT '',
  cover_item_id TEXT REFERENCES items(id) ON DELETE SET NULL,
  auto_tags     TEXT NOT NULL DEFAULT '[]',
  sort          TEXT NOT NULL DEFAULT '',
  created_at    INTEGER NOT NULL,
  updated_at    INTEGER NOT NULL
)`,
	`CREATE INDEX IF NOT EXISTS folders_parent ON folders(parent_id, position)`,

	`CREATE TABLE IF NOT EXISTS item_folders (
  item_id   TEXT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  folder_id TEXT NOT NULL REFERENCES folders(id) ON DELETE CASCADE,
  added_at  INTEGER NOT NULL,
  position  REAL NOT NULL DEFAULT 0,
  PRIMARY KEY (item_id, folder_id)
)`,
	`CREATE INDEX IF NOT EXISTS item_folders_folder ON item_folders(folder_id, item_id)`,
	`CREATE INDEX IF NOT EXISTS item_folders_position ON item_folders(folder_id, position)`,
	`CREATE TRIGGER IF NOT EXISTS item_folders_last AFTER INSERT ON item_folders WHEN NEW.position = 0
BEGIN
  UPDATE item_folders SET position = (SELECT COALESCE(MAX(position), 0) + 1 FROM item_folders WHERE folder_id = NEW.folder_id)
  WHERE item_id = NEW.item_id AND folder_id = NEW.folder_id;
END`,

	`CREATE TABLE IF NOT EXISTS tag_groups (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL UNIQUE,
  color      TEXT NOT NULL DEFAULT '',
  position   INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL
)`,

	`CREATE TABLE IF NOT EXISTS tags (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL UNIQUE COLLATE NOCASE,
  pinyin     TEXT NOT NULL DEFAULT '',
  gloss_en   TEXT NOT NULL DEFAULT '',
  color      TEXT NOT NULL DEFAULT '',
  starred    INTEGER NOT NULL DEFAULT 0,
  group_id   TEXT REFERENCES tag_groups(id) ON DELETE SET NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
)`,
	`CREATE INDEX IF NOT EXISTS tags_group ON tags(group_id)`,

	`CREATE TABLE IF NOT EXISTS smart_folders (
  id         TEXT PRIMARY KEY,
  parent_id  TEXT REFERENCES smart_folders(id) ON DELETE CASCADE,
  position   INTEGER NOT NULL DEFAULT 0,
  name       TEXT NOT NULL,
  icon       TEXT NOT NULL DEFAULT '',
  color      TEXT NOT NULL DEFAULT '',
  rules      TEXT NOT NULL DEFAULT '{}',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
)`,

	`CREATE TABLE IF NOT EXISTS saved_filters (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL,
  query      TEXT NOT NULL DEFAULT '{}',
  position   INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL
)`,

	`CREATE TABLE IF NOT EXISTS palettes (
  item_id TEXT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  rank    INTEGER NOT NULL,
  rgb     INTEGER NOT NULL,
  l       REAL NOT NULL,
  a       REAL NOT NULL,
  b       REAL NOT NULL,
  ratio   REAL NOT NULL,
  accent  INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (item_id, rank)
)`,

	`CREATE TABLE IF NOT EXISTS texts (
  item_rid      INTEGER NOT NULL REFERENCES items(rid) ON DELETE CASCADE,
  kind          TEXT NOT NULL CHECK (kind IN ('ocr','doc','transcript','caption','web')),
  lang          TEXT NOT NULL DEFAULT '',
  content       TEXT NOT NULL DEFAULT '',
  segments_json TEXT NOT NULL DEFAULT '[]',
  model_fp      TEXT NOT NULL DEFAULT '',
  updated_at    INTEGER NOT NULL,
  PRIMARY KEY (item_rid, kind)
) WITHOUT ROWID`,

	`CREATE VIRTUAL TABLE IF NOT EXISTS items_fts USING fts5(name, tags, note, source, body,
  tokenize = 'unicode61 remove_diacritics 2', content = '', contentless_delete = 1)`,
	`CREATE TRIGGER IF NOT EXISTS items_fts_delete AFTER DELETE ON items BEGIN
  DELETE FROM items_fts WHERE rowid BETWEEN old.rid << 4 AND (old.rid << 4) + 15;
END`,
	`CREATE TRIGGER IF NOT EXISTS texts_fts_delete AFTER DELETE ON texts BEGIN
  DELETE FROM items_fts WHERE rowid = (old.item_rid << 4) | (CASE old.kind WHEN 'ocr' THEN 1 WHEN 'doc' THEN 2 WHEN 'transcript' THEN 3 WHEN 'caption' THEN 4 ELSE 5 END);
END`,

	`CREATE TABLE IF NOT EXISTS fingerprints (
  id   INTEGER PRIMARY KEY,
  text TEXT NOT NULL UNIQUE
)`,

	`CREATE TABLE IF NOT EXISTS segments (
  id         INTEGER PRIMARY KEY,
  item_rid   INTEGER NOT NULL REFERENCES items(rid) ON DELETE CASCADE,
  kind       TEXT NOT NULL CHECK (kind IN ('image','page','page_image','frame','audio','chunk','caption','note','transcript')),
  modality   INTEGER NOT NULL CHECK (modality IN (0, 1, 2)),
  loc        REAL NOT NULL DEFAULT 0,
  loc_end    REAL NOT NULL DEFAULT 0,
  excerpt    TEXT NOT NULL DEFAULT '',
  sets_scale INTEGER NOT NULL DEFAULT 1,
  fp         INTEGER NOT NULL REFERENCES fingerprints(id),
  budget     INTEGER NOT NULL DEFAULT 0,
  vec256     BLOB,
  created_at INTEGER NOT NULL
)`,
	`CREATE INDEX IF NOT EXISTS segments_item ON segments(item_rid, kind, loc)`,
	`CREATE INDEX IF NOT EXISTS segments_fp ON segments(fp, budget)`,

	`CREATE TABLE IF NOT EXISTS segment_vec768 (
  seg_id INTEGER PRIMARY KEY REFERENCES segments(id) ON DELETE CASCADE,
  vec    BLOB NOT NULL
)`,

	`CREATE TABLE IF NOT EXISTS ai_runs (
  item_id    TEXT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  task       TEXT NOT NULL,
  model_fp   TEXT NOT NULL,
  status     TEXT NOT NULL CHECK (status IN ('ok','failed','skipped')),
  error      TEXT NOT NULL DEFAULT '',
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (item_id, task)
)`,
	`CREATE INDEX IF NOT EXISTS ai_runs_model ON ai_runs(task, model_fp)`,

	`CREATE TABLE IF NOT EXISTS tag_vectors (
  tag_id     TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
  model_fp   TEXT NOT NULL,
  gloss      TEXT NOT NULL,
  vec768     BLOB NOT NULL,
  vec_type   TEXT NOT NULL DEFAULT 'f16' CHECK (vec_type IN ('f16','i8')),
  vec_scale  REAL NOT NULL DEFAULT 1,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (tag_id, model_fp)
)`,

	`CREATE TABLE IF NOT EXISTS plugins (
  id                  TEXT PRIMARY KEY,
  version             TEXT NOT NULL DEFAULT '',
  enabled             INTEGER NOT NULL DEFAULT 1,
  granted_permissions TEXT NOT NULL DEFAULT '[]',
  settings_json       TEXT NOT NULL DEFAULT '{}',
  installed_at        INTEGER NOT NULL,
  updated_at          INTEGER NOT NULL
)`,

	`CREATE TABLE IF NOT EXISTS import_sources (
  id         TEXT PRIMARY KEY,
  kind       TEXT NOT NULL,
  path       TEXT NOT NULL,
  name       TEXT NOT NULL DEFAULT '',
  storage    TEXT NOT NULL DEFAULT 'managed' CHECK (storage IN ('managed','linked')),
  state      TEXT NOT NULL DEFAULT '{}',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  UNIQUE (kind, path)
)`,

	`CREATE TABLE IF NOT EXISTS import_map (
  source_id  TEXT NOT NULL REFERENCES import_sources(id) ON DELETE CASCADE,
  kind       TEXT NOT NULL CHECK (kind IN ('item','folder','smart','group')),
  ext_id     TEXT NOT NULL,
  act_id     TEXT NOT NULL DEFAULT '',
  status     TEXT NOT NULL DEFAULT 'ok',
  modified   INTEGER NOT NULL DEFAULT 0,
  base       TEXT NOT NULL DEFAULT '{}',
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (source_id, kind, ext_id)
) WITHOUT ROWID`,
	`CREATE INDEX IF NOT EXISTS import_map_act ON import_map(act_id) WHERE act_id != ''`,

	`CREATE TABLE IF NOT EXISTS vision (
  item_rid      INTEGER PRIMARY KEY REFERENCES items(rid) ON DELETE CASCADE,
  feature_print BLOB,
  aesthetics    REAL,
  utility       INTEGER NOT NULL DEFAULT 0,
  labels        TEXT NOT NULL DEFAULT '[]',
  faces         INTEGER NOT NULL DEFAULT 0,
  model_fp      TEXT NOT NULL DEFAULT '',
  updated_at    INTEGER NOT NULL
)`,

	`CREATE TABLE IF NOT EXISTS folder_suggestions (
  item_id    TEXT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  folder_id  TEXT NOT NULL REFERENCES folders(id) ON DELETE CASCADE,
  score      REAL NOT NULL DEFAULT 0,
  state      TEXT NOT NULL DEFAULT 'suggested' CHECK (state IN ('suggested','accepted','rejected')),
  created_at INTEGER NOT NULL,
  PRIMARY KEY (item_id, folder_id)
) WITHOUT ROWID`,
	`CREATE INDEX IF NOT EXISTS folder_suggestions_state ON folder_suggestions(state, folder_id)`,

	`CREATE TABLE IF NOT EXISTS versions (
  id         TEXT PRIMARY KEY,
  item_id    TEXT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  n          INTEGER NOT NULL,
  label      TEXT NOT NULL DEFAULT '',
  storage    TEXT NOT NULL CHECK (storage IN ('managed','linked')),
  rel_path   TEXT NOT NULL DEFAULT '',
  abs_path   TEXT NOT NULL DEFAULT '',
  file_name  TEXT NOT NULL DEFAULT '',
  ext        TEXT NOT NULL DEFAULT '',
  mime       TEXT NOT NULL DEFAULT '',
  size       INTEGER NOT NULL DEFAULT 0,
  hash       TEXT NOT NULL DEFAULT '',
  width      INTEGER NOT NULL DEFAULT 0,
  height     INTEGER NOT NULL DEFAULT 0,
  duration   REAL NOT NULL DEFAULT 0,
  plugin     TEXT NOT NULL DEFAULT '',
  params     TEXT NOT NULL DEFAULT '{}',
  created_at INTEGER NOT NULL,
  kept       INTEGER NOT NULL DEFAULT 0,
  UNIQUE (item_id, n)
)`,
	`CREATE INDEX IF NOT EXISTS versions_item ON versions(item_id, n)`,

	`CREATE TABLE IF NOT EXISTS chapters (
  item_id    TEXT PRIMARY KEY REFERENCES items(id) ON DELETE CASCADE,
  list       TEXT NOT NULL DEFAULT '[]',
  model_fp   TEXT NOT NULL DEFAULT '',
  updated_at INTEGER NOT NULL
)`,
	`CREATE TRIGGER IF NOT EXISTS chapters_fts_delete AFTER DELETE ON chapters BEGIN
  DELETE FROM items_fts WHERE rowid = ((SELECT rid FROM items WHERE id = old.item_id) << 4) | 7;
END`,

	`CREATE TABLE IF NOT EXISTS item_tags (
  item_id    TEXT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  tag_id     TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
  source     TEXT NOT NULL DEFAULT 'user' CHECK (source IN ('user','ai','vision','import','rule','meta')),
  state      TEXT NOT NULL DEFAULT 'accepted' CHECK (state IN ('accepted','suggested','rejected')),
  confidence REAL NOT NULL DEFAULT 1,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (item_id, tag_id)
)`,
	`CREATE INDEX IF NOT EXISTS item_tags_tag ON item_tags(tag_id, state)`,
	`CREATE INDEX IF NOT EXISTS item_tags_suggested ON item_tags(item_id) WHERE state = 'suggested'`,

	`CREATE TABLE IF NOT EXISTS field_suggestions (
  item_id    TEXT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  field      TEXT NOT NULL CHECK (field IN ('title','filename','new_tag','summary')),
  value      TEXT NOT NULL,
  model_fp   TEXT NOT NULL DEFAULT '',
  state      TEXT NOT NULL DEFAULT 'suggested' CHECK (state IN ('suggested','accepted','rejected')),
  created_at INTEGER NOT NULL,
  PRIMARY KEY (item_id, field, value)
) WITHOUT ROWID`,
	`CREATE INDEX IF NOT EXISTS field_suggestions_state ON field_suggestions(field, state)`,

	`CREATE TABLE IF NOT EXISTS speech (
  item_id    TEXT PRIMARY KEY REFERENCES items(id) ON DELETE CASCADE,
  state      TEXT NOT NULL,
  seconds    REAL NOT NULL DEFAULT 0,
  strong     REAL NOT NULL DEFAULT 0,
  music      REAL NOT NULL DEFAULT 0,
  duration   REAL NOT NULL DEFAULT 0,
  regions    TEXT NOT NULL DEFAULT '[]',
  model_fp   TEXT NOT NULL DEFAULT '',
  updated_at INTEGER NOT NULL
)`,
	`CREATE INDEX IF NOT EXISTS speech_state ON speech(state)`,

	`CREATE TABLE IF NOT EXISTS annotations (
  id         TEXT PRIMARY KEY,
  item_id    TEXT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  kind       TEXT NOT NULL CHECK (kind IN ('rect','time','point')),
  x          REAL NOT NULL DEFAULT 0,
  y          REAL NOT NULL DEFAULT 0,
  w          REAL NOT NULL DEFAULT 0,
  h          REAL NOT NULL DEFAULT 0,
  t0         REAL NOT NULL DEFAULT 0,
  t1         REAL NOT NULL DEFAULT 0,
  page       INTEGER NOT NULL DEFAULT 0,
  text       TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
)`,
	`CREATE INDEX IF NOT EXISTS annotations_item ON annotations(item_id)`,

	`CREATE TABLE IF NOT EXISTS boards (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL,
  bg          TEXT NOT NULL DEFAULT '',
  viewport_x  REAL NOT NULL DEFAULT 0,
  viewport_y  REAL NOT NULL DEFAULT 0,
  zoom        REAL NOT NULL DEFAULT 0,
  cover_cache TEXT NOT NULL DEFAULT '',
  pinned_at   INTEGER NOT NULL DEFAULT 0,
  position    INTEGER NOT NULL DEFAULT 0,
  opened_at   INTEGER NOT NULL DEFAULT 0
)`,

	`CREATE TABLE IF NOT EXISTS board_nodes (
  id        TEXT PRIMARY KEY,
  board_id  TEXT NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
  kind      TEXT NOT NULL CHECK (kind IN ('item','note','text','shape','line','stroke','sticker','frame','group')),
  item_id   TEXT,
  parent_id TEXT,
  x         REAL NOT NULL DEFAULT 0,
  y         REAL NOT NULL DEFAULT 0,
  w         REAL NOT NULL DEFAULT 0,
  h         REAL NOT NULL DEFAULT 0,
  rotation  REAL NOT NULL DEFAULT 0,
  z         INTEGER NOT NULL DEFAULT 0,
  locked    INTEGER NOT NULL DEFAULT 0,
  hidden    INTEGER NOT NULL DEFAULT 0,
  props     TEXT NOT NULL DEFAULT '{}'
)`,
	`CREATE INDEX IF NOT EXISTS board_nodes_board ON board_nodes(board_id, z)`,
	`CREATE INDEX IF NOT EXISTS board_nodes_item ON board_nodes(item_id) WHERE item_id IS NOT NULL`,

	`CREATE TABLE IF NOT EXISTS listing_gen (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  n  INTEGER NOT NULL
)`,
	`INSERT OR IGNORE INTO listing_gen (id, n) VALUES (1, 1)`,
	`CREATE TRIGGER IF NOT EXISTS listing_items_add AFTER INSERT ON items BEGIN UPDATE listing_gen SET n = n + 1; END`,
	`CREATE TRIGGER IF NOT EXISTS listing_items_del AFTER DELETE ON items BEGIN UPDATE listing_gen SET n = n + 1; END`,
	`CREATE TRIGGER IF NOT EXISTS listing_items_trash AFTER UPDATE OF deleted_at ON items BEGIN UPDATE listing_gen SET n = n + 1; END`,
	`CREATE TRIGGER IF NOT EXISTS listing_items_archive AFTER UPDATE OF archived_at ON items BEGIN UPDATE listing_gen SET n = n + 1; END`,
	`CREATE TRIGGER IF NOT EXISTS listing_item_tags_add AFTER INSERT ON item_tags BEGIN UPDATE listing_gen SET n = n + 1; END`,
	`CREATE TRIGGER IF NOT EXISTS listing_item_tags_del AFTER DELETE ON item_tags BEGIN UPDATE listing_gen SET n = n + 1; END`,
	`CREATE TRIGGER IF NOT EXISTS listing_item_tags_set AFTER UPDATE OF item_id, tag_id, state, source ON item_tags BEGIN UPDATE listing_gen SET n = n + 1; END`,
	`CREATE TRIGGER IF NOT EXISTS listing_item_folders_add AFTER INSERT ON item_folders BEGIN UPDATE listing_gen SET n = n + 1; END`,
	`CREATE TRIGGER IF NOT EXISTS listing_item_folders_del AFTER DELETE ON item_folders BEGIN UPDATE listing_gen SET n = n + 1; END`,
	`CREATE TRIGGER IF NOT EXISTS listing_item_folders_set AFTER UPDATE OF item_id, folder_id ON item_folders BEGIN UPDATE listing_gen SET n = n + 1; END`,
	`CREATE TRIGGER IF NOT EXISTS listing_tags_add AFTER INSERT ON tags BEGIN UPDATE listing_gen SET n = n + 1; END`,
	`CREATE TRIGGER IF NOT EXISTS listing_tags_del AFTER DELETE ON tags BEGIN UPDATE listing_gen SET n = n + 1; END`,
	`CREATE TRIGGER IF NOT EXISTS listing_tags_set AFTER UPDATE OF name, color, starred, group_id, updated_at ON tags BEGIN UPDATE listing_gen SET n = n + 1; END`,
	`CREATE TRIGGER IF NOT EXISTS listing_folders_add AFTER INSERT ON folders BEGIN UPDATE listing_gen SET n = n + 1; END`,
	`CREATE TRIGGER IF NOT EXISTS listing_folders_del AFTER DELETE ON folders BEGIN UPDATE listing_gen SET n = n + 1; END`,
	`CREATE TRIGGER IF NOT EXISTS listing_folders_set AFTER UPDATE ON folders BEGIN UPDATE listing_gen SET n = n + 1; END`,

	`CREATE TABLE IF NOT EXISTS folder_locks (
  folder_id  TEXT PRIMARY KEY REFERENCES folders(id) ON DELETE CASCADE,
  hash       TEXT NOT NULL,
  hint       TEXT NOT NULL DEFAULT '',
  agent      INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
)`,
	`CREATE TRIGGER IF NOT EXISTS folder_locks_gen_add AFTER INSERT ON folder_locks BEGIN UPDATE listing_gen SET n = n + 1; END`,
	`CREATE TRIGGER IF NOT EXISTS folder_locks_gen_del AFTER DELETE ON folder_locks BEGIN UPDATE listing_gen SET n = n + 1; END`,
	`CREATE TRIGGER IF NOT EXISTS folder_locks_gen_set AFTER UPDATE ON folder_locks BEGIN UPDATE listing_gen SET n = n + 1; END`,

	`CREATE TABLE IF NOT EXISTS highlights (
  id         TEXT PRIMARY KEY,
  item_id    TEXT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  color      TEXT NOT NULL DEFAULT 'yellow',
  quote      TEXT NOT NULL,
  prefix     TEXT NOT NULL DEFAULT '',
  suffix     TEXT NOT NULL DEFAULT '',
  start      INTEGER NOT NULL DEFAULT 0,
  stop       INTEGER NOT NULL DEFAULT 0,
  page       INTEGER NOT NULL DEFAULT 0,
  note       TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
)`,
	`CREATE INDEX IF NOT EXISTS highlights_item ON highlights(item_id, page, start)`,
	`CREATE INDEX IF NOT EXISTS highlights_updated ON highlights(updated_at, id)`,

	`CREATE TABLE IF NOT EXISTS look_prints (
  item_rid   INTEGER PRIMARY KEY REFERENCES items(rid) ON DELETE CASCADE,
  hash       TEXT NOT NULL DEFAULT '',
  version    INTEGER NOT NULL DEFAULT 0,
  aspect     REAL NOT NULL DEFAULT 0,
  phash      BLOB,
  tiny       BLOB,
  grid       BLOB,
  problem    TEXT NOT NULL DEFAULT '',
  matched    INTEGER NOT NULL DEFAULT 0,
  updated_at INTEGER NOT NULL
)`,
	`CREATE INDEX IF NOT EXISTS look_prints_unmatched ON look_prints (matched, version)`,

	`CREATE TABLE IF NOT EXISTS dup_pairs (
  a_rid INTEGER NOT NULL REFERENCES items(rid) ON DELETE CASCADE,
  b_rid INTEGER NOT NULL REFERENCES items(rid) ON DELETE CASCADE,
  diff  REAL NOT NULL,
  PRIMARY KEY (a_rid, b_rid)
) WITHOUT ROWID`,
	`CREATE INDEX IF NOT EXISTS dup_pairs_b ON dup_pairs (b_rid)`,

	`CREATE TABLE IF NOT EXISTS suggestion_tags (
  tag_id TEXT PRIMARY KEY REFERENCES tags(id) ON DELETE CASCADE
) WITHOUT ROWID`,
	`CREATE TRIGGER IF NOT EXISTS suggestion_tags_taken AFTER INSERT ON item_tags WHEN NEW.state = 'accepted' BEGIN DELETE FROM suggestion_tags WHERE tag_id = NEW.tag_id; END`,
	`CREATE TRIGGER IF NOT EXISTS suggestion_tags_taken_set AFTER UPDATE OF state ON item_tags WHEN NEW.state = 'accepted' BEGIN DELETE FROM suggestion_tags WHERE tag_id = NEW.tag_id; END`,
	`CREATE TRIGGER IF NOT EXISTS suggestion_tags_edited AFTER UPDATE OF name, color, starred, group_id, updated_at ON tags BEGIN DELETE FROM suggestion_tags WHERE tag_id = NEW.id; END`,

	`CREATE TABLE IF NOT EXISTS superseded (
  id         TEXT PRIMARY KEY,
  by_id      TEXT NOT NULL,
  created_at INTEGER NOT NULL
) WITHOUT ROWID`,
}

// Migrate initializes a Refto v25 SQLite database in db, enabling WAL mode and
// foreign keys, creating all tables/indexes/triggers, setting user_version = 25,
// and seeding listing_gen.
func Migrate(db *gorm.DB) error {
	for _, pragma := range []string{
		`PRAGMA journal_mode = WAL`,
		`PRAGMA foreign_keys = ON`,
	} {
		if err := db.Exec(pragma).Error; err != nil {
			return fmt.Errorf("refto: exec %s: %w", pragma, err)
		}
	}

	for _, stmt := range schemaStatements {
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("refto: migrate schema statement: %w", err)
		}
	}

	if err := db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, SchemaVersion)).Error; err != nil {
		return fmt.Errorf("refto: set user_version: %w", err)
	}
	return nil
}
