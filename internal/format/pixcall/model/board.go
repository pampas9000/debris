package model

import (
	"time"

	"gorm.io/gorm"
)

// Board represents a row in the Pixcall `boards` table.
type Board struct {
	ID          int64     `gorm:"column:id;type:bigint;primaryKey;autoIncrement:false" json:"id"`
	Revision    int       `gorm:"column:revision;type:int;not null;default:1" json:"revision"`
	ParentID    *int64    `gorm:"column:parent_id;type:bigint" json:"parent_id,omitempty"`
	Name        string    `gorm:"column:name;type:text;not null" json:"name"`
	NamePinyin  *string   `gorm:"column:name_pinyin;type:text" json:"name_pinyin,omitempty"`
	Description *string   `gorm:"column:description;type:text" json:"description,omitempty"`
	Filters     string    `gorm:"column:filters;type:text;not null" json:"filters"`
	FileCount   int       `gorm:"column:file_count;type:int;not null;default:0" json:"file_count"`
	FolderCount int       `gorm:"column:folder_count;type:int;not null;default:0" json:"folder_count"`
	FileSize    int64     `gorm:"column:file_size;type:bigint;not null;default:0" json:"file_size"`
	Ranking     int64     `gorm:"column:ranking;type:bigint;not null;default:1" json:"ranking"`
	Layout      *string   `gorm:"column:layout;type:varchar(64)" json:"layout,omitempty"`
	SortBy      *string   `gorm:"column:sort_by;type:varchar(64)" json:"sort_by,omitempty"`
	SortOrder   *string   `gorm:"column:sort_order;type:varchar(16)" json:"sort_order,omitempty"`
	Icon        *string   `gorm:"column:icon;type:varchar(32)" json:"icon,omitempty"`
	IconColor   *string   `gorm:"column:icon_color;type:varchar(16)" json:"icon_color,omitempty"`
	CreatedAt   time.Time `gorm:"column:created_at;type:timestamp;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
}

// TableName implements gorm.io/gorm/schema.Tabler.
func (Board) TableName() string {
	return "boards"
}

// BeforeCreate populates CreatedAt when zero.
func (b *Board) BeforeCreate(_ *gorm.DB) error {
	if b.CreatedAt.IsZero() {
		b.CreatedAt = time.Now().UTC()
	}
	return nil
}

// BoardEntry represents a row in the Pixcall `board_entries` join table.
type BoardEntry struct {
	BoardID   int64 `gorm:"column:board_id;type:bigint;primaryKey;autoIncrement:false;not null" json:"board_id"`
	EntryID   int64 `gorm:"column:entry_id;type:bigint;primaryKey;autoIncrement:false;not null" json:"entry_id"`
	EntryKind int   `gorm:"column:entry_kind;type:int;not null" json:"entry_kind"`
}

// TableName implements gorm.io/gorm/schema.Tabler.
func (BoardEntry) TableName() string {
	return "board_entries"
}
