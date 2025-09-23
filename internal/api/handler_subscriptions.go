package api

import (
	"net/http"
	"strings"

	"laundry-status-backend/internal/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type putSubscriptionRequest struct {
	Endpoint           string  `json:"endpoint" binding:"required"`
	P256DH             string  `json:"p256dh" binding:"required"`
	Auth               string  `json:"auth" binding:"required"`
	SubscribedMachines []int64 `json:"subscribed_machines"`
}

// PutSubscription handles the creation or replacement of a subscription.
func (h *Handler) PutSubscription(c *gin.Context) {
	var req putSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	subscription := model.PushSubscription{
		Endpoint: req.Endpoint,
		P256DH:   req.P256DH,
		Auth:     req.Auth,
	}

	err := h.store.DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "endpoint"}},
			DoUpdates: clause.AssignmentColumns([]string{"p256dh", "auth"}),
		}).Create(&subscription).Error; err != nil {
			return err
		}

		var machines []model.Machine
		if len(req.SubscribedMachines) > 0 {
			if err := tx.Find(&machines, req.SubscribedMachines).Error; err != nil {
				return err
			}
		}

		if err := tx.Model(&subscription).Association("Machines").Replace(&machines); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusCreated)
}

type deleteSubscriptionRequest struct {
	Endpoint string `json:"endpoint" binding:"required"`
}

// DeleteSubscription handles the deletion of a subscription.
func (h *Handler) DeleteSubscription(c *gin.Context) {
	var req deleteSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.store.DB().Delete(&model.PushSubscription{Endpoint: req.Endpoint}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

func rawQueryParam(rawQuery, key string) (string, bool) {
	for _, kv := range strings.Split(rawQuery, "&") {
		if strings.HasPrefix(kv, key+"=") {
			return kv[len(key)+1:], true // 不做 URL 解码
		}
	}
	return "", false
}

// GetSubscription handles the retrieval of a subscription.
func (h *Handler) GetSubscription(c *gin.Context) {
	raw, ok := rawQueryParam(c.Request.URL.RawQuery, "endpoint")
	if !ok || raw == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "endpoint is required"})
		return
	}

	var subscription model.PushSubscription
	if err := h.store.DB().Preload("Machines").First(&subscription, "endpoint = ?", raw).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "subscription not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	machineIDs := make([]int64, len(subscription.Machines))
	for i, machine := range subscription.Machines {
		machineIDs[i] = machine.ID
	}

	c.JSON(http.StatusOK, gin.H{"subscribed_machines": machineIDs})
}
