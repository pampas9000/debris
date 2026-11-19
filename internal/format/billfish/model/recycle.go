package model

// Recycle represents the 'bf_recycle' table.
type Recycle struct {
	ID           int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	FileID       int64  `gorm:"index:bf_recycle_file_id_idx" json:"file_id"`
	FolderID     int64  `gorm:"index:bf_recycle_folder_id_idx" json:"folder_id"`
	Born         int64  `json:"born"`
	OriginalName string `gorm:"type:text" json:"original_name"`
	RecycleName  string `gorm:"type:text" json:"recycle_name"`
}

func (Recycle) TableName() string {
	return "bf_recycle"
}
