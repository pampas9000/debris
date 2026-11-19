package model

// Tag represents the 'bf_tag_v2' table.
type Tag struct {
	ID    int64   `gorm:"primaryKey;autoIncrement" json:"id"`
	Name  string  `gorm:"type:text" json:"name"`
	Pid   int64   `json:"pid"`
	Seq   float64 `json:"seq"`
	Icon  int     `json:"icon"`
	Color int     `json:"color"`
	Born  int64   `json:"born"`
}

func (Tag) TableName() string {
	return "bf_tag_v2"
}

// LegacyTag represents the legacy 'bf_tag' table in billfish.db.
type LegacyTag struct {
	ID    int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Name  string `gorm:"type:text" json:"name"`
	Color int    `json:"color"`
	Born  int64  `json:"born"`
}

func (LegacyTag) TableName() string {
	return "bf_tag"
}
