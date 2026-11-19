package model

// VideoUserdata represents the 'bf_video_userdata' table.
type VideoUserdata struct {
	ID          int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	FileID      int64  `gorm:"not null;uniqueIndex:bf_video_userdata_idx" json:"file_id"`
	Version     int    `json:"version"`
	Lut         string `gorm:"type:text" json:"lut"`
	Captions    string `gorm:"type:text" json:"captions"`
	Comments    string `gorm:"type:text" json:"comments"`
	Rules       string `gorm:"type:text" json:"rules"`
	PlaySetting string `gorm:"type:text" json:"play_setting"`
}

func (VideoUserdata) TableName() string {
	return "bf_video_userdata"
}
