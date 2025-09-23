package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"laundry-status-backend/config"
	"laundry-status-backend/internal/model"
	"laundry-status-backend/internal/scraper"
	"laundry-status-backend/internal/store"
)

// TestOccupancyLifecycle simulates the entire lifecycle of a machine's occupancy,
// from occupied to idle, and verifies the database state at each step.
func TestOccupancyLifecycle(t *testing.T) {
	// --- Test Setup ---

	// 1. Setup an in-memory SQLite database for testing.
	testDB, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect to the in-memory database: %v", err)
	}
	sqlDB, _ := testDB.DB()
	defer sqlDB.Close()

	// Run database migrations.
	err = testDB.AutoMigrate(&model.Machine{}, &model.OccupancyOpen{}, &model.OccupancyHistory{})
	assert.NoError(t, err)

	// 2. Create a mock configuration.
	mockConfig := &config.Config{
		Scraper: config.ScraperConfig{
			StateIdleValues:     []int{1},
			StateOccupiedValues: []int{2},
			Request: config.ScraperRequest{
				PageSize: 10, // Keep it simple for the test
			},
			Timezone: "Asia/Shanghai", // Explicitly set timezone for consistency
		},
	}
	mockConfig.WorkerPool.Size = 4

	// 3. Mock server to simulate the API responses.
	var requestCount int
	var expectedFinishTime time.Time
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var apiItems []store.ApiItem
		// On the first call, return an "occupied" status.
		if requestCount == 0 {
			// Use a consistent timezone for the test
			loc, _ := time.LoadLocation(mockConfig.Scraper.Timezone)
			expectedFinishTime = time.Now().In(loc).Add(30 * time.Minute)
			finishTimeStr := expectedFinishTime.Format("2006-01-02 15:04:05")
			apiItems = []store.ApiItem{{ID: 101, State: 2, FinishTime: &finishTimeStr, Name: "A栋1-1"}}
		} else {
			// On the second call, return an "idle" status.
			apiItems = []store.ApiItem{{ID: 101, State: 1, Name: "A栋1-1"}}
		}
		requestCount++

		// Construct the full API response structure.
		response := scraper.ApiResponse{
			Code: 0,
			Data: struct {
				Page     int             `json:"page"`
				PageSize int             `json:"pageSize"`
				Total    int             `json:"total"`
				Items    []store.ApiItem `json:"items"`
			}{
				Page:     1,
				PageSize: 10,
				Total:    len(apiItems),
				Items:    apiItems,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(response)
		assert.NoError(t, err)
	}))
	defer server.Close()

	// Update the mock config to use the test server's URL.
	mockConfig.Scraper.Request.URL = server.URL

	// 4. Instantiate the store and scraper service.
	gormStore := store.NewGormStore(testDB)
	scraperService := scraper.NewService(mockConfig, gormStore)

	// 5. Pre-populate the database with a machine to be tested.
	machine := model.Machine{ID: 101, DormID: 1, DisplayName: "Washing Machine 101"}
	err = testDB.Create(&machine).Error
	assert.NoError(t, err)

	// --- Cycle 1: Machine becomes Occupied ---
	var firstCycleObservedAt time.Time
	t.Run("Cycle 1: Machine Becomes Occupied", func(t *testing.T) {
		// Action: Run the scraper against the mock server.
		scraperService.ScrapeOnce(context.Background())

		// Assertions for Cycle 1:
		var openOccupancy model.OccupancyOpen
		err := testDB.Where("machine_id = ?", 101).First(&openOccupancy).Error
		assert.NoError(t, err, "Expected to find one record in occupancy_opens")
		assert.Equal(t, int64(101), openOccupancy.MachineID, "MachineID should match")
		assert.Equal(t, 2, openOccupancy.Status, "Status should be 'occupied' (2)")
		assert.Equal(t, "使用中", openOccupancy.Message, "Message should be correct for occupied state")
		assert.True(t, openOccupancy.TimeRemaining > 0, "TimeRemaining should be positive")
		assert.WithinDuration(t, time.Now(), openOccupancy.ObservedAt, 5*time.Second, "ObservedAt should be recent")

		var historyCount int64
		testDB.Model(&model.OccupancyHistory{}).Where("machine_id = ?", 101).Count(&historyCount)
		assert.Equal(t, int64(0), historyCount, "occupancy_history should be empty")
		firstCycleObservedAt = openOccupancy.ObservedAt // Save for next cycle's assertion
	})

	// --- Cycle 2: Machine becomes Idle ---
	t.Run("Cycle 2: Machine Becomes Idle", func(t *testing.T) {
		// Action: Run the scraper again.
		scraperService.ScrapeOnce(context.Background())

		// Assertions for Cycle 2:
		var openCount int64
		testDB.Model(&model.OccupancyOpen{}).Where("machine_id = ?", 101).Count(&openCount)
		assert.Equal(t, int64(0), openCount, "occupancy_opens should be empty")

		var historyOccupancy model.OccupancyHistory
		err := testDB.Where("machine_id = ?", 101).First(&historyOccupancy).Error
		assert.NoError(t, err, "Expected to find one record in occupancy_history")
		assert.Equal(t, int64(101), historyOccupancy.MachineID, "MachineID should match in history")
		assert.Equal(t, 2, historyOccupancy.Status, "Archived status should be correct")
		assert.Equal(t, "使用中", historyOccupancy.Message, "Archived message should be correct")
		// Assert the new PeriodStart and PeriodEnd fields.
		assert.NotNil(t, firstCycleObservedAt, "firstCycleObservedAt should have been captured")
		assert.Equal(t, firstCycleObservedAt.Unix(), historyOccupancy.PeriodStart.Unix(), "PeriodStart should match the initial observation time")
		assert.WithinDuration(t, expectedFinishTime.UTC(), historyOccupancy.PeriodEnd.UTC(), 2*time.Second, "PeriodEnd should match the expected finish time")
		// The new ObservedAt is the time the state change was detected (the second scrape).
		assert.True(t, historyOccupancy.ObservedAt.After(firstCycleObservedAt), "History's ObservedAt should be after the first cycle's observation")
	})
}

// TestOccupancyHistoryScenarios covers edge cases for archiving occupancy records.
func TestOccupancyHistoryScenarios(t *testing.T) {
	// --- Common Test Setup ---
	setupTest := func() (*gorm.DB, *scraper.Service, *httptest.Server, func([][]store.ApiItem)) {
		testDB, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
		assert.NoError(t, err)

		err = testDB.AutoMigrate(&model.Machine{}, &model.OccupancyOpen{}, &model.OccupancyHistory{})
		assert.NoError(t, err)

		mockConfig := &config.Config{
			Scraper: config.ScraperConfig{
				StateIdleValues:     []int{1},
				StateOccupiedValues: []int{2},
				StateFaultyValues:   []int{3},
				Request: config.ScraperRequest{
					PageSize: 10,
				},
				Timezone: "Asia/Shanghai",
			},
		}
		mockConfig.WorkerPool.Size = 4

		var mockResponses [][]store.ApiItem
		var currentResponseIndex int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var apiItems []store.ApiItem
			if currentResponseIndex < len(mockResponses) {
				apiItems = mockResponses[currentResponseIndex]
				currentResponseIndex++
			}

			response := scraper.ApiResponse{
				Code: 0,
				Data: struct {
					Page     int             `json:"page"`
					PageSize int             `json:"pageSize"`
					Total    int             `json:"total"`
					Items    []store.ApiItem `json:"items"`
				}{Page: 1, PageSize: 10, Total: len(apiItems), Items: apiItems},
			}
			w.Header().Set("Content-Type", "application/json")
			err := json.NewEncoder(w).Encode(response)
			assert.NoError(t, err)
		}))

		mockConfig.Scraper.Request.URL = server.URL
		gormStore := store.NewGormStore(testDB)
		scraperService := scraper.NewService(mockConfig, gormStore)

		// This function is returned to the test to control the mock server's responses.
		setResponses := func(responses [][]store.ApiItem) {
			mockResponses = responses
			currentResponseIndex = 0
		}

		return testDB, scraperService, server, setResponses
	}

	t.Run("Machine Disappears From API", func(t *testing.T) {
		testDB, scraperService, server, setResponses := setupTest()
		sqlDB, _ := testDB.DB()
		defer sqlDB.Close()
		defer server.Close()

		// Arrange: A machine exists in the database.
		machine := model.Machine{ID: 201, DormID: 2, DisplayName: "Machine 201"}
		err := testDB.Create(&machine).Error
		assert.NoError(t, err)

		// Act & Assert
		// Cycle 1: Machine is occupied.
		setResponses([][]store.ApiItem{
			{{ID: 201, State: 2, Name: "A栋2-1"}}, // Occupied
			{},                                   // Disappears
		})
		scraperService.ScrapeOnce(context.Background())

		// Assert Cycle 1: Machine is in occupancy_opens.
		var openOccupancy model.OccupancyOpen
		err = testDB.Where("machine_id = ?", 201).First(&openOccupancy).Error
		assert.NoError(t, err)
		assert.Equal(t, int64(201), openOccupancy.MachineID)

		// Cycle 2: Machine disappears from the API.
		scraperService.ScrapeOnce(context.Background())

		// Assert Cycle 2: Machine is moved to history.
		var openCount int64
		testDB.Model(&model.OccupancyOpen{}).Where("machine_id = ?", 201).Count(&openCount)
		assert.Equal(t, int64(0), openCount, "occupancy_opens should be empty")

		var historyOccupancy model.OccupancyHistory
		err = testDB.Where("machine_id = ?", 201).First(&historyOccupancy).Error
		assert.NoError(t, err, "A history record should be created for the disappeared machine")
		assert.Equal(t, int64(201), historyOccupancy.MachineID)
	})

	t.Run("Machine Goes From Faulty to Idle", func(t *testing.T) {
		testDB, scraperService, server, setResponses := setupTest()
		sqlDB, _ := testDB.DB()
		defer sqlDB.Close()
		defer server.Close()

		// Arrange: A machine exists in the database.
		machine := model.Machine{ID: 301, DormID: 3, DisplayName: "Machine 301"}
		err := testDB.Create(&machine).Error
		assert.NoError(t, err)

		// Act & Assert
		// Cycle 1: Machine is faulty.
		setResponses([][]store.ApiItem{
			{{ID: 301, State: 3, Name: "A栋3-1"}}, // Faulty
			{{ID: 301, State: 1, Name: "A栋3-1"}}, // Idle
		})
		scraperService.ScrapeOnce(context.Background())

		// Assert Cycle 1: Machine is in occupancy_opens with faulty state.
		var openOccupancy model.OccupancyOpen
		err = testDB.Where("machine_id = ?", 301).First(&openOccupancy).Error
		assert.NoError(t, err)
		assert.Equal(t, 3, openOccupancy.Status)
		assert.Equal(t, "设备故障", openOccupancy.Message)

		// Cycle 2: Machine becomes idle.
		scraperService.ScrapeOnce(context.Background())

		// Assert Cycle 2: Machine is moved to history.
		var openCount int64
		testDB.Model(&model.OccupancyOpen{}).Where("machine_id = ?", 301).Count(&openCount)
		assert.Equal(t, int64(0), openCount)

		var historyOccupancy model.OccupancyHistory
		err = testDB.Where("machine_id = ?", 301).First(&historyOccupancy).Error
		assert.NoError(t, err)
		assert.Equal(t, int64(301), historyOccupancy.MachineID)
		assert.Equal(t, 3, historyOccupancy.Status, "Archived status should be faulty")
	})
}
