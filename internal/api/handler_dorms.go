package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"laundry-status-backend/internal/model"
)

// DormResponse represents the API response for a single dorm.
type DormResponse struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	MaxFloor      int    `json:"maxFloor"`
	TotalMachines int64  `json:"totalMachines"`
}

// GetDorms handles the GET /api/dorms request.
func GetDorms(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1) 一次取所有宿舍
		var dorms []model.Dorm
		if err := db.Find(&dorms).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve dorms"})
			return
		}

		// 2) 一次聚合出每个宿舍的统计
		type AggRow struct {
			DormID        int64
			TotalMachines int64
			MaxFloor      int
		}
		var aggs []AggRow
		if err := db.
			Model(&model.Machine{}).
			Select("dorm_id as dorm_id, COUNT(*) as total_machines, COALESCE(MAX(floor), 0) as max_floor").
			Group("dorm_id").
			Scan(&aggs).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to aggregate machines"})
			return
		}

		// 3) 合并
		aggMap := make(map[int64]AggRow, len(aggs))
		for _, a := range aggs {
			aggMap[a.DormID] = a
		}

		responses := make([]DormResponse, 0, len(dorms))
		for _, d := range dorms {
			a := aggMap[d.ID] // 不存在时使用零值
			responses = append(responses, DormResponse{
				ID: d.ID, Name: d.Name,
				MaxFloor: a.MaxFloor, TotalMachines: a.TotalMachines,
			})
		}
		c.JSON(http.StatusOK, responses)
	}
}
