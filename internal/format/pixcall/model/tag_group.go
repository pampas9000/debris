package model

// TagGroups represents a row in the Pixcall `tag_groups` table.
type TagGroups struct {
	ID       int64   `gorm:"column:id;type:bigint;primaryKey;autoIncrement:false;not null" json:"id"`
	Revision int     `gorm:"column:revision;type:int;not null;default:1" json:"revision"`
	Name     string  `gorm:"column:name;type:varchar(100);not null" json:"name"`
	Pinyin   string  `gorm:"column:pinyin;type:varchar(100);not null" json:"pinyin"`
	Color    *string `gorm:"column:color;type:varchar(16)" json:"color,omitempty"`
	Ranking  int64   `gorm:"column:ranking;type:bigint;not null;default:1" json:"ranking"`
}

// TableName implements gorm.io/gorm/schema.Tabler.
func (TagGroups) TableName() string {
	return "tag_groups"
}
