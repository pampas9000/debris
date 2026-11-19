package model

import (
	"time"

	"gorm.io/gorm"
)

// Entry represents a row in the Pixcall `entries` table (root, folder, or file).
type Entry struct {
	ID          int64     `gorm:"column:id;type:bigint;primaryKey;autoIncrement:false" json:"id"`
	Revision    int       `gorm:"column:revision;type:int;not null;default:1" json:"revision"`
	ParentID    int64     `gorm:"column:parent_id;type:bigint;not null" json:"parent_id"`
	Name        string    `gorm:"column:name;type:text;not null" json:"name"`
	Kind        int       `gorm:"column:kind;type:int;not null" json:"kind"`
	Description *string   `gorm:"column:description;type:text" json:"description,omitempty"`
	Link        *string   `gorm:"column:link;type:varchar(255)" json:"link,omitempty"`
	Tags        *string   `gorm:"column:tags;type:text" json:"tags,omitempty"`
	Rating      *int      `gorm:"column:rating;type:int" json:"rating,omitempty"`
	Size        int64     `gorm:"column:size;type:bigint;not null;default:0" json:"size"`
	ContentType string    `gorm:"column:content_type;type:varchar(100);not null;default:''" json:"content_type"`
	ContentHash *string   `gorm:"column:content_hash;type:varchar(64)" json:"content_hash,omitempty"`
	Metadata    *string   `gorm:"column:metadata;type:text" json:"metadata,omitempty"`
	Ranking     int64     `gorm:"column:ranking;type:bigint;not null;default:0" json:"ranking"`
	CreatedAt   time.Time `gorm:"column:created_at;type:timestamp;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at;type:timestamp;not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	SourcePath  *string   `gorm:"column:source_path;type:text" json:"source_path,omitempty"`
	NamePinyin  *string   `gorm:"column:name_pinyin;type:text" json:"name_pinyin,omitempty"`
	Status      int64     `gorm:"column:status;type:bigint;not null;default:0" json:"status"`
	Inode       *int64    `gorm:"column:inode;type:bigint" json:"inode,omitempty"`
	Mtime       *int64    `gorm:"column:mtime;type:bigint" json:"mtime,omitempty"`
	IsHidden    int       `gorm:"column:is_hidden;type:smallint;not null;default:0" json:"is_hidden"`
	IsDeleted   int       `gorm:"column:is_deleted;type:int;not null;default:0" json:"is_deleted"`
	IsPrivate   int       `gorm:"column:is_private;type:int;not null;default:0" json:"is_private"`
}

// TableName implements gorm.io/gorm/schema.Tabler.
func (Entry) TableName() string {
	return "entries"
}

// BeforeCreate populates default timestamps when omitted.
func (e *Entry) BeforeCreate(_ *gorm.DB) error {
	now := time.Now().UTC()
	if e.CreatedAt.IsZero() {
		e.CreatedAt = now
	}
	if e.UpdatedAt.IsZero() {
		e.UpdatedAt = now
	}
	return nil
}

// BeforeUpdate refreshes UpdatedAt when zero.
func (e *Entry) BeforeUpdate(_ *gorm.DB) error {
	if e.UpdatedAt.IsZero() {
		e.UpdatedAt = time.Now().UTC()
	}
	return nil
}

// RecentEntry represents a row in the `recent_entries` table.
type RecentEntry struct {
	EntryID      int64 `gorm:"column:entry_id;type:integer;primaryKey;autoIncrement:false" json:"entry_id"`
	LastOpenedAt int64 `gorm:"column:last_opened_at;type:bigint;not null" json:"last_opened_at"`
}

// TableName implements gorm.io/gorm/schema.Tabler.
func (RecentEntry) TableName() string {
	return "recent_entries"
}
