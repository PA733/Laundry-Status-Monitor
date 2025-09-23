package model

import "time"

// PushSubscription holds the information for a browser push subscription.
type PushSubscription struct {
	Endpoint string `gorm:"primaryKey"`
	P256DH   string `gorm:"column:p256dh;not null"`
	Auth     string `gorm:"not null"`
	CreatedAt time.Time `gorm:"not null"`

	// Associations
	Machines []*Machine `gorm:"many2many:subscription_machine_mapping;"`
}