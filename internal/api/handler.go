package api

import (
	"laundry-status-backend/internal/store"

	"github.com/SherClockHolmes/webpush-go"
)

// Handler holds shared dependencies for API handlers.
type Handler struct {
	store   store.Store
	webpush *webpush.Options
}

// NewHandler creates a new API handler.
func NewHandler(s store.Store, webpushOptions *webpush.Options) *Handler {
	return &Handler{
		store:   s,
		webpush: webpushOptions,
	}
}