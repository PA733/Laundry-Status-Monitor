package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetVAPIDPublicKey returns the VAPID public key to the client.
func (h *Handler) GetVAPIDPublicKey(c *gin.Context) {
	if h.webpush == nil || h.webpush.VAPIDPublicKey == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "vapid keys are not configured"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"public_key": h.webpush.VAPIDPublicKey})
}