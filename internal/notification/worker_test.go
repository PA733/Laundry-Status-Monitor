package notification

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/SherClockHolmes/webpush-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"laundry-status-backend/internal/model"
)

// mockSender is a mock implementation of the NotificationSender interface.
type mockSender struct {
	SendFunc func(payload []byte, sub *webpush.Subscription, options *webpush.Options) (*http.Response, error)
}

// Send calls the mock SendFunc.
func (m *mockSender) Send(payload []byte, sub *webpush.Subscription, options *webpush.Options) (*http.Response, error) {
	return m.SendFunc(payload, sub, options)
}

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

func TestWorkerPool_Dispatch(t *testing.T) {
	db, _ := newTestDB(t)
	wp := NewWorkerPool(1, db, &webpush.Options{})

	// Dispatch a job
	wp.Dispatch(123)

	// Check if the job is in the channel
	select {
	case job := <-wp.jobs:
		assert.Equal(t, int64(123), job)
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for job to be dispatched")
	}
}

func TestWorkerPool_WorkerLogic(t *testing.T) {
	gormDB, mock := newTestDB(t)
	wp := NewWorkerPool(1, gormDB, &webpush.Options{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wp.Start(ctx)

	// --- Test Case: One subscription found, notification sent ---
	t.Run("sends notification for one subscription", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Add(1)

		machineID := int64(101)
		subscription := model.PushSubscription{
			Endpoint: "https://example.com/push",
			P256DH:   "test_p256dh",
			Auth:     "test_auth",
		}

		// Set up the mock sender
		wp.sender = &mockSender{
			SendFunc: func(payload []byte, sub *webpush.Subscription, options *webpush.Options) (*http.Response, error) {
				assert.Equal(t, "https://example.com/push", sub.Endpoint)
				assert.Equal(t, "洗衣机 Washing Machine 101 已经可用！", string(payload))
				wg.Done()
				return &http.Response{
					StatusCode: http.StatusCreated,
					Body:       ioutil.NopCloser(bytes.NewBufferString("")),
				}, nil
			},
		}

		// Mock database query
		mock.ExpectQuery(`SELECT .* FROM "push_subscriptions".*JOIN .*subscription_machine_mapping.*WHERE .*smm\.machine_id = \$1`).
			WithArgs(machineID).
			WillReturnRows(sqlmock.NewRows([]string{"endpoint", "p256dh", "auth", "created_at"}).
				AddRow(subscription.Endpoint, subscription.P256DH, subscription.Auth, time.Now()))

		mock.ExpectQuery(`SELECT "display_name" FROM "machines" WHERE "machines"."id" = \$1 ORDER BY "machines"."id" LIMIT \$[0-9]+`).
			WithArgs(machineID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"display_name"}).AddRow("Washing Machine 101"))

		wp.Dispatch(machineID)
		wg.Wait()
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	// --- Test Case: Subscription expired, should be deleted ---
	t.Run("deletes expired subscription", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Add(1)

		machineID := int64(102)
		subscription := model.PushSubscription{
			Endpoint: "https://example.com/expired",
			P256DH:   "test_p256dh_expired",
			Auth:     "test_auth_expired",
		}

		// Set up the mock sender to return a 410 Gone status
		wp.sender = &mockSender{
			SendFunc: func(payload []byte, sub *webpush.Subscription, options *webpush.Options) (*http.Response, error) {
				// This will be called, but we wait on the DB operation
				return &http.Response{
					StatusCode: http.StatusGone,
					Body:       ioutil.NopCloser(bytes.NewBufferString("")),
				}, nil
			},
		}

		mock.ExpectQuery(`SELECT .* FROM "push_subscriptions".*JOIN .*subscription_machine_mapping.*WHERE .*smm\.machine_id = \$1`).
			WithArgs(machineID).
			WillReturnRows(sqlmock.NewRows([]string{"endpoint", "p256dh", "auth", "created_at"}).
				AddRow(subscription.Endpoint, subscription.P256DH, subscription.Auth, time.Now()))

		mock.ExpectQuery(`SELECT "display_name" FROM "machines" WHERE "machines"."id" = \$1 ORDER BY "machines"."id" LIMIT \$[0-9]+`).
			WithArgs(machineID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"display_name"}).AddRow("Machine 102"))

		// Expect the delete operation
		mock.ExpectBegin()
		mock.ExpectExec(`DELETE FROM "push_subscriptions" WHERE "push_subscriptions"."endpoint" = \$1`).
			WithArgs(subscription.Endpoint).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		wp.Dispatch(machineID)

		// A short sleep to allow the worker to process the job
		time.Sleep(100 * time.Millisecond)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	// --- Test Case: Machine lookup fails, fallback to ID ---
	t.Run("falls back to machine ID when lookup fails", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Add(1)

		machineID := int64(103)
		subscription := model.PushSubscription{
			Endpoint: "https://example.com/fallback",
			P256DH:   "test_p256dh_fallback",
			Auth:     "test_auth_fallback",
		}

		wp.sender = &mockSender{
			SendFunc: func(payload []byte, sub *webpush.Subscription, options *webpush.Options) (*http.Response, error) {
				assert.Equal(t, "https://example.com/fallback", sub.Endpoint)
				assert.Equal(t, "洗衣机 103 已经可用！", string(payload))
				wg.Done()
				return &http.Response{
					StatusCode: http.StatusCreated,
					Body:       ioutil.NopCloser(bytes.NewBufferString("")),
				}, nil
			},
		}

		mock.ExpectQuery(`SELECT .* FROM "push_subscriptions".*JOIN .*subscription_machine_mapping.*WHERE .*smm\.machine_id = \$1`).
			WithArgs(machineID).
			WillReturnRows(sqlmock.NewRows([]string{"endpoint", "p256dh", "auth", "created_at"}).
				AddRow(subscription.Endpoint, subscription.P256DH, subscription.Auth, time.Now()))

		mock.ExpectQuery(`SELECT "display_name" FROM "machines" WHERE "machines"."id" = \$1 ORDER BY "machines"."id" LIMIT \$[0-9]+`).
			WithArgs(machineID, 1).
			WillReturnError(fmt.Errorf("machine not found"))

		wp.Dispatch(machineID)
		wg.Wait()
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
