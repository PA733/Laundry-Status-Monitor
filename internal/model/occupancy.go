package model

import (
	"time"
)

// OccupancyMeta defines the structured metadata for an occupancy record.
type OccupancyMeta struct {
	Status        int    `json:"status"`
	Message       string `json:"message"`
	TimeRemaining int    `json:"time_remaining"`
}

// OccupancyOpen represents the current status of a machine (hot table).
type OccupancyOpen struct {
	MachineID     int64     `gorm:"primaryKey"`
	ObservedAt    time.Time `gorm:"not null"`
	Status        int       `gorm:"not null"`
	Message       string    `gorm:"not null"`
	TimeRemaining int       `gorm:"not null"`
}

// OccupancyHistory represents the historical log of machine usage (cold table).
type OccupancyHistory struct {
	ID          int64     `gorm:"autoIncrement"`
	MachineID   int64     `gorm:"not null;index;primaryKey"`
	ObservedAt  time.Time `gorm:"not null;index;primaryKey"` // Time the state's END was observed
	Status      int       `gorm:"not null"`
	Message     string    `gorm:"not null"`
	PeriodStart time.Time `gorm:"not null"`
	PeriodEnd   time.Time `gorm:"not null"` // Predicted End Time
}
