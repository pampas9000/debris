package model

import (
	"time"

	"gorm.io/gorm"
)

// RemoteEvents represents a row in the Pixcall `remote_events` table.
type RemoteEvents struct {
	ID        int64     `gorm:"column:id;type:integer;primaryKey;autoIncrement:false" json:"id"`
	Revision  int       `gorm:"column:revision;type:int;not null;default:1" json:"revision"`
	CreatedAt time.Time `gorm:"column:created_at;type:timestamp;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	Data      string    `gorm:"column:data;type:text;not null" json:"data"`
}

// TableName implements gorm.io/gorm/schema.Tabler.
func (RemoteEvents) TableName() string {
	return "remote_events"
}

// BeforeCreate populates CreatedAt when zero.
func (r *RemoteEvents) BeforeCreate(_ *gorm.DB) error {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	return nil
}
