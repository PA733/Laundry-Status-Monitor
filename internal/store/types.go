package store

import "time"

// ApiItem represents a single device record from the upstream API.
type ApiItem struct {
	ID                  int64      `json:"id"`
	Name                string     `json:"name"`
	IMEI                string     `json:"imei"`
	FloorCode           string     `json:"floorCode"`
	State               int        `json:"state"`
	EnableReserve       *bool      `json:"enableReserve"`
	ReserveState        *int       `json:"reserveState"`
	LastMaintenanceTime *string    `json:"lastMaintenanceTime"`
	FinishTime          *string    `json:"finishTime"`
	FinishTimeParsed    *time.Time `json:"-"`
	DeviceID            int64      `json:"deviceId"`
}

// MachineStateType defines the recognized states of a laundry machine.
type MachineStateType string

const (
	StateTypeIdle     MachineStateType = "idle"
	StateTypeOccupied MachineStateType = "occupied"
	StateTypeFaulty   MachineStateType = "faulty"
	StateTypeUnknown  MachineStateType = "unknown"
)
