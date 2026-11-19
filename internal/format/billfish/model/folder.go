package model

// Folder represents the 'bf_folder' table.
type Folder struct {
	ID        int64   `gorm:"primaryKey;autoIncrement" json:"id"`
	Born      int64   `json:"born"`
	Pid       int64   `gorm:"not null;index:bf_folder_pid_idx" json:"pid"`
	Name      string  `gorm:"type:text" json:"name"`
	Desc      string  `gorm:"type:text" json:"desc"`
	CoverTid  int64   `json:"cover_tid"`
	Hide      int     `json:"hide"`
	Seq       float64 `json:"seq"`
	Color     int     `json:"color"`
	IsRecycle int     `json:"is_recycle"`
}

func (Folder) TableName() string {
	return "bf_folder"
}
