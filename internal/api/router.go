package api

import (
	"time"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/gin-gonic/gin"
	"github.com/patrickmn/go-cache"
	"golang.org/x/time/rate"

	"laundry-status-backend/internal/mw"
	"laundry-status-backend/internal/store"
)

// NewRouter creates and configures a new Gin router.
func NewRouter(s store.Store, webpushOptions *webpush.Options) *gin.Engine {
	r := gin.Default()

	db := s.DB()
	handler := NewHandler(s, webpushOptions)

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

		// Add these lines inside the api.Use(rateLimiter) block
		api.GET("/subscriptions", handler.GetSubscription)
		api.PUT("/subscriptions", handler.PutSubscription)
		api.DELETE("/subscriptions", handler.DeleteSubscription)
		api.GET("/vapid_public_key", handler.GetVAPIDPublicKey)
	}

	return r
}
