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
