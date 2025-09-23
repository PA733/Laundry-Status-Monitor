package store

import (
	"context"
	"database/sql/driver"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"laundry-status-backend/internal/model"
)

// A helper function to create a mock database connection.
func newTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	gormDB, err := gorm.Open(postgres.New(postgres.Config{
		Conn: db,
	}), &gorm.Config{})
	require.NoError(t, err)

	return gormDB, mock
}

func TestGormStore_UpdateOccupancy(t *testing.T) {
	now := time.Now()

	// Define a simple state classifier for testing.
	getStateType := func(state int) MachineStateType {
		if state == 1 {
			return StateTypeIdle
		}
		return StateTypeOccupied
	}

	testCases := []struct {
		name               string
		initialOpenRecords []model.OccupancyOpen
		apiItems           []ApiItem
		mockExpectations   func(mock sqlmock.Sqlmock)
		expectedNotifyIDs  []int64
		expectedErr        bool
	}{
		{
			name: "Machine becomes idle, should notify",
			initialOpenRecords: []model.OccupancyOpen{
				{MachineID: 101, Status: 2, ObservedAt: now.Add(-10 * time.Minute)},
			},
			apiItems: []ApiItem{
				{ID: 101, State: 1}, // State 1 is Idle
			},
			mockExpectations: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "occupancy_opens"`)).
					WillReturnRows(sqlmock.NewRows([]string{"machine_id", "status", "observed_at"}).
						AddRow(101, 2, now.Add(-10*time.Minute)))

				mock.ExpectBegin()
				mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "occupancy_histories"`)).
					WithArgs(101, Any{}, 2, "", Any{}, Any{}).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
				mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "occupancy_opens" WHERE "occupancy_opens"."machine_id" = $1`)).
					WithArgs(101).
					WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectCommit()
			},
			expectedNotifyIDs: []int64{101},
			expectedErr:       false,
		},
		{
			name: "Machine changes state but not to idle, should not notify",
			initialOpenRecords: []model.OccupancyOpen{
				{MachineID: 102, Status: 2, ObservedAt: now.Add(-10 * time.Minute)},
			},
			apiItems: []ApiItem{
				{ID: 102, State: 3}, // State 3 is still Occupied
			},
			mockExpectations: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "occupancy_opens"`)).
					WillReturnRows(sqlmock.NewRows([]string{"machine_id", "status", "observed_at"}).
						AddRow(102, 2, now.Add(-10*time.Minute)))

				mock.ExpectBegin()
				mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "occupancy_histories"`)).
					WithArgs(102, Any{}, 2, "", Any{}, Any{}).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
				// Expect an UPDATE (via Save)
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE "occupancy_opens"`)).
					WithArgs(Any{}, 3, "使用中", 0, 102).
					WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectCommit()
			},
			expectedNotifyIDs: nil,
			expectedErr:       false,
		},
		{
			name: "No state change, should do nothing and not notify",
			initialOpenRecords: []model.OccupancyOpen{
				{MachineID: 103, Status: 2, ObservedAt: now.Add(-10 * time.Minute)},
			},
			apiItems: []ApiItem{
				{ID: 103, State: 2}, // Same state
			},
			mockExpectations: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "occupancy_opens"`)).
					WillReturnRows(sqlmock.NewRows([]string{"machine_id", "status", "observed_at"}).
						AddRow(103, 2, now.Add(-10*time.Minute)))
				mock.ExpectBegin()
				// No database writes expected
				mock.ExpectCommit()
			},
			expectedNotifyIDs: nil,
			expectedErr:       false,
		},
		{
			name:               "New machine appears in occupied state, should create record and not notify",
			initialOpenRecords: []model.OccupancyOpen{},
			apiItems: []ApiItem{
				{ID: 104, State: 2}, // New machine, occupied
			},
			mockExpectations: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "occupancy_opens"`)).
					WillReturnRows(sqlmock.NewRows([]string{"machine_id", "status", "observed_at"}))

				mock.ExpectBegin()
				mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "occupancy_opens"`)).
					WithArgs(Any{}, 2, "使用中", 0, 104).
					WillReturnRows(sqlmock.NewRows([]string{"machine_id"}).AddRow(104))
				mock.ExpectCommit()
			},
			expectedNotifyIDs: nil,
			expectedErr:       false,
		},
		{
			name: "Machine disappears from API, should archive and not notify",
			initialOpenRecords: []model.OccupancyOpen{
				{MachineID: 105, Status: 2, ObservedAt: now.Add(-10 * time.Minute)},
			},
			apiItems: []ApiItem{}, // Machine 105 is gone
			mockExpectations: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "occupancy_opens"`)).
					WillReturnRows(sqlmock.NewRows([]string{"machine_id", "status", "observed_at"}).
						AddRow(105, 2, now.Add(-10*time.Minute)))

				mock.ExpectBegin()
				mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "occupancy_histories"`)).
					WithArgs(105, Any{}, 2, "", Any{}, Any{}).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
				mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "occupancy_opens"`)).
					WithArgs(105).
					WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectCommit()
			},
			expectedNotifyIDs: nil,
			expectedErr:       false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gormDB, mock := newTestDB(t)
			store := NewGormStore(gormDB)

			tc.mockExpectations(mock)

			notifyIDs, err := store.UpdateOccupancy(context.Background(), now, tc.apiItems, getStateType)

			if tc.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.ElementsMatch(t, tc.expectedNotifyIDs, notifyIDs)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// Any is a helper for sqlmock to match any argument.
type Any struct{}

// Match satisfies the sqlmock.Argument interface
func (a Any) Match(v driver.Value) bool {
	return true
}
