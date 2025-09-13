package model

import "time"

// Dorm represents a dormitory building.
type Dorm struct {
	ID        int64     `gorm:"primaryKey"`
	Name      string    `gorm:"uniqueIndex;size:128;not null"`
	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`

	// Associations
	Machines []Machine `gorm:"foreignKey:DormID"`
}
