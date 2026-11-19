package model

// Tag represents a row in the Pixcall `tags` table.
type Tag struct {
	ID       int64  `gorm:"column:id;type:bigint;primaryKey;autoIncrement:false;not null" json:"id"`
	Revision int    `gorm:"column:revision;type:int;not null;default:1" json:"revision"`
	GroupID  *int64 `gorm:"column:group_id;type:bigint" json:"group_id,omitempty"`
	Name     string `gorm:"column:name;type:varchar(100);not null" json:"name"`
	Pinyin   string `gorm:"column:pinyin;type:varchar(100);not null" json:"pinyin"`
	Category string `gorm:"column:category;type:varchar(1);not null" json:"category"`
}

// TableName implements gorm.io/gorm/schema.Tabler.
func (Tag) TableName() string {
	return "tags"
}
