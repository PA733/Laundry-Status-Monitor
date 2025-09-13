package model

import "time"

// Machine represents a washing machine's basic information.
type Machine struct {
	ID          int64  `gorm:"primaryKey"` // Upstream ID
	DormID      int64  `gorm:"index;not null"`
	DisplayName string `gorm:"size:256;not null"`
	IMEI        string `gorm:"size:64"`
	DeviceID    int64
	FloorCode   string `gorm:"size:32"`
	Floor       int
	Seq         int
	CreatedAt   time.Time
	UpdatedAt   time.Time

	// Associations
	Dorm Dorm `gorm:"constraint:OnDelete:CASCADE"`
}
