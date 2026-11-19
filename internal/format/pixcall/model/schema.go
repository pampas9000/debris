package model

import "gorm.io/gorm"

// SchemaVersion is the Pixcall database schema version supported by this package.
const SchemaVersion = "22"

// AllModels returns all Pixcall main.db GORM model pointers for AutoMigrate.
func AllModels() []any {
	return []any{
		&Entry{},
		&Folders{},
		&Tag{},
		&TagGroups{},
		&Board{},
		&BoardEntry{},
		&Kvs{},
		&Media{},
		&Exif{},
		&RemoteEvents{},
		&RecentEntry{},
		&Caption{},
	}
}

var schemaIndexes = []string{
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_entries_parent_id_name ON entries (parent_id, name)`,
	`CREATE INDEX IF NOT EXISTS idx_entries_content_hash ON entries (content_hash)`,
	`CREATE INDEX IF NOT EXISTS idx_entries_inode ON entries (inode)`,
	`CREATE INDEX IF NOT EXISTS idx_entries_ranking ON entries (parent_id, ranking)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_tags_group_id_name ON tags (IFNULL(group_id, 0), name)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_tag_groups_name ON tag_groups (name)`,
	`CREATE INDEX IF NOT EXISTS idx_recent_entries_last_opened_at ON recent_entries (last_opened_at DESC)`,
}

// Migrate creates all Pixcall schema_version 22 tables and indexes in db.
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(AllModels()...); err != nil {
		return err
	}
	for _, stmt := range schemaIndexes {
		if err := db.Exec(stmt).Error; err != nil {
			return err
		}
	}
	return nil
}

// ThumbsSchemaVersion is the Pixcall thumbs.db schema version.
const ThumbsSchemaVersion = "2"

var thumbsSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS thumbnails (
		key          VARCHAR(50) NOT NULL PRIMARY KEY,
		content_type VARCHAR(50) NOT NULL,
		width        INT DEFAULT 0 NOT NULL,
		height       INT DEFAULT 0 NOT NULL,
		size         INT NOT NULL,
		hash         VARCHAR(64) NOT NULL,
		created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		accessed_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		data         BLOB NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS kvs (
		k VARCHAR(100) NOT NULL PRIMARY KEY,
		v TEXT NOT NULL
	)`,
	`REPLACE INTO kvs (k, v) VALUES ('schema_version', '2')`,
}

// MigrateThumbs creates the Pixcall thumbs.db schema and seed kvs rows in db.
func MigrateThumbs(db *gorm.DB) error {
	for _, stmt := range thumbsSchemaStatements {
		if err := db.Exec(stmt).Error; err != nil {
			return err
		}
	}
	return nil
}
