package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func setupSubscriptionRouter() *gin.Engine {
	r := gin.Default()
	handler := NewHandler(nil, nil)
	r.PUT("/api/subscriptions", handler.PutSubscription)
	return r
}

func TestPutSubscription(t *testing.T) {
	router := setupSubscriptionRouter()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/api/subscriptions", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.JSONEq(t, `{"error":"invalid request"}`, w.Body.String())
}
