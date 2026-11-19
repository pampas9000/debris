package model

// TagJoinFolder represents the 'bf_tag_join_folder' table.
type TagJoinFolder struct {
	ID       int64 `gorm:"primaryKey;autoIncrement" json:"id"`
	FolderID int64 `gorm:"index:bf_tag_join_folder_idx;uniqueIndex:idx_bf_tag_join_folder_pair" json:"folder_id"`
	TagID    int64 `gorm:"uniqueIndex:idx_bf_tag_join_folder_pair" json:"tag_id"`
}

func (TagJoinFolder) TableName() string {
	return "bf_tag_join_folder"
}
