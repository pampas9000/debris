package model

// TagJoinGroup represents the 'bf_tag_join_group' table.
type TagJoinGroup struct {
	ID    int64 `gorm:"primaryKey;autoIncrement" json:"id"`
	Gid   int64 `gorm:"index:bf_tag_join_group_idx;uniqueIndex:idx_bf_tag_join_group_pair" json:"gid"`
	TagID int64 `gorm:"uniqueIndex:idx_bf_tag_join_group_pair" json:"tag_id"`
	Born  int64 `json:"born"`
}

func (TagJoinGroup) TableName() string {
	return "bf_tag_join_group"
}
