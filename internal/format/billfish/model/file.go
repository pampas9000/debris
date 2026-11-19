package model

// File represents the 'bf_file' table.
type File struct {
	ID       int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Name     string `gorm:"type:text" json:"name"`
	Pid      int64  `gorm:"not null;index:bf_file_pid_idx" json:"pid"`
	IsHide   int    `json:"is_hide"`
	IsLink   int    `json:"is_link"`
	FileSize int64  `json:"file_size"`
	Ctime    int64  `json:"ctime"`
	Mtime    int64  `json:"mtime"`
	Md5      string `gorm:"type:text" json:"md5"`
	Tid      int64  `json:"tid"`
	Born     int64  `json:"born"`
	Ttid     int64  `json:"ttid"`
}

func (File) TableName() string {
	return "bf_file"
}
