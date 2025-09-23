package scraper

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	"laundry-status-backend/config"
	"laundry-status-backend/internal/notification"
	"laundry-status-backend/internal/store"
)

// ApiData mirrors the unexported struct in scraper.go for testing purposes.
type ApiData struct {
	Total int             `json:"total"`
	Items []store.ApiItem `json:"items"`
}

// mockStore is a mock implementation of the store.Store interface.
type mockStore struct {
	UpsertDormsAndMachinesFunc func(ctx context.Context, items []store.ApiItem) error
	UpdateOccupancyFunc        func(ctx context.Context, now time.Time, items []store.ApiItem, getStateType func(int) store.MachineStateType) ([]int64, error)
	DBFunc                     func() *gorm.DB
}

func (m *mockStore) UpsertDormsAndMachines(ctx context.Context, items []store.ApiItem) error {
	return m.UpsertDormsAndMachinesFunc(ctx, items)
}

func (m *mockStore) UpdateOccupancy(ctx context.Context, now time.Time, items []store.ApiItem, getStateType func(int) store.MachineStateType) ([]int64, error) {
	return m.UpdateOccupancyFunc(ctx, now, items, getStateType)
}

func (m *mockStore) DB() *gorm.DB {
	return m.DBFunc()
}

func TestScraper_Integration(t *testing.T) {
	// --- Setup ---
	var wg sync.WaitGroup
	wg.Add(1) // We expect one machine ID to be dispatched

	// Mock upstream API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := struct {
			Code int     `json:"code"`
			Data ApiData `json:"data"`
		}{
			Code: 0,
			Data: ApiData{
				Total: 1,
				Items: []store.ApiItem{
					{ID: 101, Name: "Test Machine", State: 1},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Mock store
	mockStore := &mockStore{
		UpsertDormsAndMachinesFunc: func(ctx context.Context, items []store.ApiItem) error {
			return nil // Do nothing
		},
		UpdateOccupancyFunc: func(ctx context.Context, now time.Time, items []store.ApiItem, getStateType func(int) store.MachineStateType) ([]int64, error) {
			// Simulate that machine 101 became idle and needs a notification
			return []int64{101}, nil
		},
		DBFunc: func() *gorm.DB {
			return nil // Not needed for this test
		},
	}

	// Create a minimal config for the test
	cfg := &config.Config{
		Scraper: config.ScraperConfig{
			Request: config.ScraperRequest{
				URL:      server.URL,
				PageSize: 10, // Set a PageSize to avoid division by zero
			},
			StateIdleValues: []int{1},
		},
		WorkerPool: config.WorkerPoolConfig{
			Size: 1,
		},
	}

	// Create the scraper service with the mock store
	service := NewService(cfg, mockStore)

	// Replace the real worker pool with a mock one
	mockWorkerPool := notification.NewWorkerPool(1, nil, nil)
	service.workerPool = mockWorkerPool

	// Start the mock worker pool and listen for dispatched jobs
	var dispatchedID int64
	go func() {
		for id := range mockWorkerPool.Jobs() {
			dispatchedID = id
			wg.Done()
		}
	}()

	// --- Execution ---
	service.ScrapeOnce(context.Background())

	// --- Verification ---
	wg.Wait() // Wait for the job to be dispatched
	assert.Equal(t, int64(101), dispatchedID, "The machine ID returned by UpdateOccupancy should be dispatched to the worker pool")
}
