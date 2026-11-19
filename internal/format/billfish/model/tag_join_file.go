package model

// TagJoinFile represents the 'bf_tag_join_file' table.
type TagJoinFile struct {
	ID     int64 `gorm:"primaryKey;autoIncrement" json:"id"`
	FileID int64 `gorm:"uniqueIndex:idx_bf_tag_join_file_pair" json:"file_id"`
	TagID  int64 `gorm:"index:bf_tag_join_file_idx;uniqueIndex:idx_bf_tag_join_file_pair" json:"tag_id"`
	Born   int64 `json:"born"`
}

func (TagJoinFile) TableName() string {
	return "bf_tag_join_file"
}
