package notification

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/SherClockHolmes/webpush-go"
	"gorm.io/gorm"

	"laundry-status-backend/internal/model"
)

// NotificationSender defines the interface for sending a web push notification.
type NotificationSender interface {
	Send(payload []byte, sub *webpush.Subscription, options *webpush.Options) (*http.Response, error)
}

// WebPushSender is a real implementation of NotificationSender using the webpush library.
type WebPushSender struct{}

// Send sends a notification using the webpush library.
func (s *WebPushSender) Send(payload []byte, sub *webpush.Subscription, options *webpush.Options) (*http.Response, error) {
	return webpush.SendNotification(payload, sub, options)
}

// WorkerPool manages a pool of workers for sending notifications.
type WorkerPool struct {
	size    int
	jobs    chan int64
	db      *gorm.DB
	webpush *webpush.Options
	sender  NotificationSender
}

// NewWorkerPool creates a new worker pool.
func NewWorkerPool(size int, db *gorm.DB, webpushOptions *webpush.Options) *WorkerPool {
	return &WorkerPool{
		size:    size,
		jobs:    make(chan int64, size), // Buffered channel
		db:      db,
		webpush: webpushOptions,
		sender:  &WebPushSender{}, // Use the real sender by default
	}
}

// Start launches the worker goroutines.
func (wp *WorkerPool) Start(ctx context.Context) {
	for i := 0; i < wp.size; i++ {
		go wp.worker(ctx, i)
	}
}

// worker is the actual worker goroutine.
func (wp *WorkerPool) worker(ctx context.Context, id int) {
	log.Printf("Worker %d started", id)
	for {
		select {
		case machineID := <-wp.jobs:
			log.Printf("Worker %d processing machine %d", id, machineID)
			wp.sendNotificationsForMachine(ctx, machineID)
		case <-ctx.Done():
			log.Printf("Worker %d shutting down", id)
			return
		}
	}
}

// Dispatch sends a job to the worker pool.
func (wp *WorkerPool) Dispatch(machineID int64) {
	wp.jobs <- machineID
}

// Jobs returns the jobs channel for testing.
func (wp *WorkerPool) Jobs() chan int64 {
	return wp.jobs
}

// sendNotificationsForMachine fetches subscriptions and sends notifications for a given machine.
func (wp *WorkerPool) sendNotificationsForMachine(ctx context.Context, machineID int64) {
	var subscriptions []model.PushSubscription
	err := wp.db.WithContext(ctx).
		Joins("JOIN subscription_machine_mapping smm ON smm.push_subscription_endpoint = push_subscriptions.endpoint").
		Where("smm.machine_id = ?", machineID).
		Find(&subscriptions).Error
	if err != nil {
		log.Printf("Error fetching subscriptions for machine %d: %v", machineID, err)
		return
	}

	if len(subscriptions) == 0 {
		return
	}

	log.Printf("Sending %d notifications for machine %d", len(subscriptions), machineID)

	var machine model.Machine
	machineLabel := fmt.Sprintf("%d", machineID)
	if err := wp.db.WithContext(ctx).
		Select("display_name").
		First(&machine, machineID).Error; err != nil {
		log.Printf("Error fetching machine %d: %v", machineID, err)
	} else if machine.DisplayName != "" {
		machineLabel = machine.DisplayName
	}

	message := fmt.Sprintf("洗衣机 %s 已经可用！", machineLabel)
	for _, sub := range subscriptions {
		wp.sendNotification(ctx, sub, []byte(message))
	}
}

// sendNotification sends a single web push notification.
func (wp *WorkerPool) sendNotification(ctx context.Context, sub model.PushSubscription, payload []byte) {
	// Manually construct the webpush.Subscription object
	wpSub := &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys: webpush.Keys{
			P256dh: sub.P256DH,
			Auth:   sub.Auth,
		},
	}

	resp, err := wp.sender.Send(payload, wpSub, wp.webpush)
	if err != nil {
		log.Printf("Error sending notification to %s: %v", sub.Endpoint, err)
		return
	}
	defer resp.Body.Close()

	// Handle expired subscriptions
	if resp.StatusCode == 410 {
		log.Printf("Subscription for endpoint %s is expired. Deleting.", sub.Endpoint)
		if err := wp.db.WithContext(ctx).Delete(&sub).Error; err != nil {
			log.Printf("Failed to delete expired subscription %s: %v", sub.Endpoint, err)
		}
	}
}
