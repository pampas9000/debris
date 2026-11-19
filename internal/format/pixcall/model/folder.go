package model

// Folders represents a row in the Pixcall `folders` table.
type Folders struct {
	EntryID        int64   `gorm:"column:entry_id;type:bigint;primaryKey;autoIncrement:false;not null" json:"entry_id"`
	Revision       int     `gorm:"column:revision;type:int;not null;default:1" json:"revision"`
	FileCount      int     `gorm:"column:file_count;type:int;not null;default:0" json:"file_count"`
	FolderCount    int     `gorm:"column:folder_count;type:int;not null;default:0" json:"folder_count"`
	FileSize       int64   `gorm:"column:file_size;type:bigint;not null;default:0" json:"file_size"`
	PubFileCount   int     `gorm:"column:pub_file_count;type:int;not null;default:0" json:"pub_file_count"`
	PubFolderCount int     `gorm:"column:pub_folder_count;type:int;not null;default:0" json:"pub_folder_count"`
	PubFileSize    int64   `gorm:"column:pub_file_size;type:bigint;not null;default:0" json:"pub_file_size"`
	Ranking        int64   `gorm:"column:ranking;type:bigint;not null;default:0" json:"ranking"`
	Layout         *string `gorm:"column:layout;type:varchar(64)" json:"layout,omitempty"`
	SortBy         *string `gorm:"column:sort_by;type:varchar(64)" json:"sort_by,omitempty"`
	SortOrder      *string `gorm:"column:sort_order;type:varchar(16)" json:"sort_order,omitempty"`
	Icon           *string `gorm:"column:icon;type:varchar(32)" json:"icon,omitempty"`
	IconColor      *string `gorm:"column:icon_color;type:varchar(16)" json:"icon_color,omitempty"`
	CoverID        *int64  `gorm:"column:cover_id;type:bigint" json:"cover_id,omitempty"`
	FixedCoverID   *int64  `gorm:"column:fixed_cover_id;type:bigint" json:"fixed_cover_id,omitempty"`
}

// TableName implements gorm.io/gorm/schema.Tabler.
func (Folders) TableName() string {
	return "folders"
}
