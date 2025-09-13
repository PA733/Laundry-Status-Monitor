package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"laundry-status-backend/internal/model"
)

// GetMachineStatus handles the GET /api/dorms/{dorm_id}/machines request.
func GetMachineStatus(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		dormID, err := strconv.ParseInt(c.Param("dorm_id"), 10, 64)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid dorm ID"})
			return
		}

		atParam := c.Query("at")
		if atParam == "" {
			getCurrentStatus(c, db, dormID)
		} else {
			getHistoricalStatus(c, db, dormID, atParam)
		}
	}
}

// machineStatusResponse is the flattened structure for the API response.
type machineStatusResponse struct {
	model.Machine
	State         int        `json:"state"`
	IsAvailable   bool       `json:"isAvailable"`
	Message       string     `json:"message"`
	TimeRemaining int        `json:"timeRemaining"`
	FinishTime    *time.Time `json:"finishTime"`
	ObservedAt    time.Time  `json:"observedAt"`
}

func getCurrentStatus(c *gin.Context, db *gorm.DB, dormID int64) {
	var machines []model.Machine
	if err := db.Preload("Dorm").Where("dorm_id = ?", dormID).Find(&machines).Error; err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve machines"})
		return
	}

	var openStatuses []model.OccupancyOpen
	machineIDs := make([]int64, len(machines))
	for i, m := range machines {
		machineIDs[i] = m.ID
	}
	db.Where("machine_id IN ?", machineIDs).Find(&openStatuses)

	statusMap := make(map[int64]model.OccupancyOpen)
	for _, s := range openStatuses {
		statusMap[s.MachineID] = s
	}

	var response []machineStatusResponse
	for _, machine := range machines {
		if status, ok := statusMap[machine.ID]; ok {
			// Machine is not idle (occupied, faulty, etc.)
			var finishTime *time.Time
			if status.TimeRemaining > 0 {
				ft := status.ObservedAt.Add(time.Duration(status.TimeRemaining) * time.Second)
				finishTime = &ft
			}

			response = append(response, machineStatusResponse{
				Machine:       machine,
				State:         status.Status,
				IsAvailable:   false, // A machine with an open status is never available.
				Message:       status.Message,
				TimeRemaining: status.TimeRemaining,
				FinishTime:    finishTime,
				ObservedAt:    status.ObservedAt,
			})
		} else {
			// Machine is idle
			response = append(response, machineStatusResponse{
				Machine:       machine,
				State:         1, // Default to configured idle status
				IsAvailable:   true,
				Message:       "空闲",
				TimeRemaining: 0,
				FinishTime:    nil,
				ObservedAt:    time.Now().UTC(),
			})
		}
	}
	c.JSON(http.StatusOK, response)
}

func getHistoricalStatus(c *gin.Context, db *gorm.DB, dormID int64, atParam string) {
	at, err := time.Parse(time.RFC3339, atParam)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid 'at' timestamp format. Use RFC3339."})
		return
	}

	var machines []model.Machine
	if err := db.Preload("Dorm").Where("dorm_id = ?", dormID).Find(&machines).Error; err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve machines"})
		return
	}

	var response []machineStatusResponse
	for _, machine := range machines {
		var history model.OccupancyHistory
		// Find the last record before or at the given time
		err := db.Preload("Dorm").Where("machine_id = ? AND observed_at <= ?", machine.ID, at).
			Order("observed_at DESC").
			First(&history).Error

		if err == gorm.ErrRecordNotFound {
			continue // No historical record for this machine at the given time
		}
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Database error during historical lookup"})
			return
		}

		var finishTime *time.Time
		var timeRemaining int
		// If there is a predicted end time, calculate duration and set finish time.
		if !history.PeriodEnd.IsZero() && history.PeriodEnd.After(history.PeriodStart) {
			finishTime = &history.PeriodEnd
			timeRemaining = int(history.PeriodEnd.Sub(history.PeriodStart).Seconds())
		}

		response = append(response, machineStatusResponse{
			Machine:     machine,
			State:       history.Status,
			IsAvailable: history.Status == 1, // 1 is idle status
			Message:     history.Message,
			// For consistency with getCurrentStatus, ObservedAt should be the start of the state.
			TimeRemaining: timeRemaining,
			FinishTime:    finishTime,
			ObservedAt:    history.PeriodStart, // Show the start time of the state
		})
	}

	c.JSON(http.StatusOK, response)
}
