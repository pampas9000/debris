package model

// Kvs represents a key-value entry in the Pixcall `kvs` table.
type Kvs struct {
	K string `gorm:"column:k;type:varchar(100);primaryKey;not null" json:"k"`
	V string `gorm:"column:v;type:text;not null" json:"v"`
}

// TableName implements gorm.io/gorm/schema.Tabler.
func (Kvs) TableName() string {
	return "kvs"
}
