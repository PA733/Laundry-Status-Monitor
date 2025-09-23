package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"laundry-status-backend/config"
	"laundry-status-backend/internal/api"
	"laundry-status-backend/internal/db"
	"laundry-status-backend/internal/scraper"
	"laundry-status-backend/internal/store" // <- New import

	"github.com/SherClockHolmes/webpush-go"
)

func main() {
	// Setup logger
	logger := log.New(os.Stdout, "laundry-backend ", log.LstdFlags)

	// Load configuration
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "./config/config.yaml" // Default path for local development
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		logger.Fatalf("failed to load configuration from %s: %v", configPath, err)
	}
	logger.Printf("configuration loaded successfully from %s", configPath)

	// Check for VAPID keys
	if cfg.Push.PublicKey == "" || cfg.Push.PrivateKey == "" {
		logger.Fatalf("VAPID keys must be configured. Please generate them and add them to your config file.")
	}

	webpushOptions := webpush.Options{
		VAPIDPublicKey:  cfg.Push.PublicKey,
		VAPIDPrivateKey: cfg.Push.PrivateKey,
		Subscriber:      cfg.Push.Subject,
		TTL:             cfg.Push.TTL,
	}

	// Initialize database
	gormDB, err := db.Init(&cfg.Database)
	if err != nil {
		logger.Fatalf("failed to initialize database: %v", err)
	}
	logger.Println("database initialized successfully")

	// Create a context that can be cancelled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create the new store layer instance
	appStore := store.NewGormStore(gormDB)
	logger.Println("data store initialized")

	// Initialize and run the scraper in the background with the store
	scraperSvc := scraper.NewService(cfg, appStore) // <- Inject store instead of db
	go scraperSvc.Run(ctx)

	// Initialize router
	router := api.NewRouter(appStore, &webpushOptions)
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: router,
	}

	// Start the server in a goroutine
	go func() {
		logger.Printf("HTTP server starting on port %d", cfg.Server.Port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatalf("HTTP server ListenAndServe: %v", err)
		}
	}()

	// Setup signal handling for graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	// Block until a signal is received.
	<-stop
	logger.Println("Shutdown signal received, stopping services...")

	// Create a deadline to wait for.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Fatalf("HTTP server Shutdown: %v", err)
	}

	logger.Println("Server gracefully stopped")
}
