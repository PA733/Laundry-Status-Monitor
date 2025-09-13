package api

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/patrickmn/go-cache"
	"golang.org/x/time/rate"
	"gorm.io/gorm"

	"laundry-status-backend/internal/mw"
)

// NewRouter creates and configures a new Gin router.
func NewRouter(db *gorm.DB) *gin.Engine {
	r := gin.Default()

	// Initialize middleware
	// Rate limit: 10 requests per second with a burst of 5
	rateLimiter := mw.RateLimiter(rate.Limit(10), 5)

	// Cache: 5 minute default expiration, cleaned up every 10 minutes
	cacheStore := cache.New(5*time.Minute, 10*time.Minute)
	caching := mw.Cache(cacheStore, 5*time.Minute)

	// API group
	api := r.Group("/api")
	api.Use(rateLimiter)
	{
		// GET /api/dorms
		api.GET("/dorms", caching, GetDorms(db))

		// GET /api/dorms/{dorm_id}/machines
		api.GET("/dorms/:dorm_id/machines", caching, GetMachineStatus(db))
	}

	return r
}
