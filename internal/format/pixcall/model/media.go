package model

// Media represents a row in the Pixcall `media` table.
type Media struct {
	EntryID  int64  `gorm:"column:entry_id;type:integer;primaryKey;autoIncrement:false;not null" json:"entry_id"`
	Metadata string `gorm:"column:metadata;type:text;not null" json:"metadata"`
}

// TableName implements gorm.io/gorm/schema.Tabler.
func (Media) TableName() string {
	return "media"
}

// Exif represents a row in the Pixcall `exif` table.
type Exif struct {
	EntryID  int64  `gorm:"column:entry_id;type:integer;primaryKey;autoIncrement:false;not null" json:"entry_id"`
	Metadata string `gorm:"column:metadata;type:text;not null" json:"metadata"`
}

// TableName implements gorm.io/gorm/schema.Tabler.
func (Exif) TableName() string {
	return "exif"
}

// Caption represents a row in the Pixcall `captions` table.
type Caption struct {
	EntryID int64  `gorm:"column:entry_id;type:integer;primaryKey;autoIncrement:false;not null" json:"entry_id"`
	Summary string `gorm:"column:summary;type:text;not null" json:"summary"`
}

// TableName implements gorm.io/gorm/schema.Tabler.
func (Caption) TableName() string {
	return "captions"
}
