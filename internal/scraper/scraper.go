package scraper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"laundry-status-backend/config"
	"laundry-status-backend/internal/notification"
	"laundry-status-backend/internal/store"

	"github.com/SherClockHolmes/webpush-go"
)

// Service orchestrates the data scraping process. It now uses a Store for persistence.
type Service struct {
	cfg        *config.Config
	store      store.Store
	client     *http.Client
	workerPool *notification.WorkerPool // New field for the worker pool
}

// NewService creates and initializes a new scraper service.
// It now accepts a store.Store instead of a *gorm.DB.
func NewService(cfg *config.Config, store store.Store) *Service {
	var transport http.RoundTripper = &http.Transport{}
	if cfg.Scraper.HTTPProxy != "" {
		proxyURL, err := url.Parse(cfg.Scraper.HTTPProxy)
		if err != nil {
			log.Printf("Warning: Invalid proxy URL %q: %v. Scraper will not use a proxy.", cfg.Scraper.HTTPProxy, err)
		} else {
			transport = &http.Transport{Proxy: http.ProxyURL(proxyURL)}
		}
	}

	webpushOptions := webpush.Options{
		VAPIDPublicKey:  cfg.Push.PublicKey,
		VAPIDPrivateKey: cfg.Push.PrivateKey,
		Subscriber:      cfg.Push.Subject,
		TTL:             cfg.Push.TTL,
	}

	// Initialize the worker pool
	workerPool := notification.NewWorkerPool(cfg.WorkerPool.Size, store.DB(), &webpushOptions)

	return &Service{
		cfg:   cfg,
		store: store,
		client: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
		workerPool: workerPool,
	}
}

// getStateType determines the machine's state type based on the raw state code.
func (s *Service) getStateType(stateCode int) store.MachineStateType {
	for _, idleVal := range s.cfg.Scraper.StateIdleValues {
		if stateCode == idleVal {
			return store.StateTypeIdle
		}
	}
	for _, occupiedVal := range s.cfg.Scraper.StateOccupiedValues {
		if stateCode == occupiedVal {
			return store.StateTypeOccupied
		}
	}
	for _, faultyVal := range s.cfg.Scraper.StateFaultyValues {
		if stateCode == faultyVal {
			return store.StateTypeFaulty
		}
	}
	return store.StateTypeUnknown
}

// Run starts the scraping process in a loop.
func (s *Service) Run(ctx context.Context) {
	if !s.cfg.Scraper.Enabled {
		log.Println("Scraper is disabled. Not starting.")
		return
	}
	log.Println("Starting scraper service...")

	// Start the worker pool
	s.workerPool.Start(ctx)

	s.ScrapeOnce(ctx)

	timer := time.NewTimer(s.cfg.Scraper.Interval)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Scraper service shutting down.")
			return
		case <-timer.C:
			s.ScrapeOnce(ctx)
			timer.Reset(s.cfg.Scraper.Interval)
		}
	}
}

// ScrapeOnce performs a single round of data scraping and calls the store to persist changes.
func (s *Service) ScrapeOnce(ctx context.Context) {
	log.Println("Executing scrape cycle...")
	now := time.Now().UTC()

	// Step 1: Fetch all data from the upstream API
	var allItems []store.ApiItem
	total := 1
	pageSize := s.cfg.Scraper.Request.PageSize
	var fetchErr error
	for page := 1; (page-1)*pageSize < total; page++ {
		resp, err := s.fetchPage(ctx, page)
		if err != nil {
			log.Printf("Error fetching page %d: %v", page, err)
			fetchErr = err
			break
		}
		if resp.Data.Total == 0 || len(resp.Data.Items) == 0 {
			break
		}
		total = resp.Data.Total
		allItems = append(allItems, resp.Data.Items...)
		log.Printf("Fetched page %d/%d, total items so far: %d", page, (total/pageSize)+1, len(allItems))
	}

	// If the fetch failed and resulted in zero items, abort to avoid clearing state.
	if fetchErr != nil && len(allItems) == 0 {
		log.Println("Scrape cycle aborted due to fetch error with no items retrieved. Occupancy data will not be updated.")
		return
	}

	// After the fetch loop
	for i := range allItems {
		parsedTime, err := s.parseTimestamp(allItems[i].FinishTime)
		if err != nil {
			log.Printf("Warning: could not parse finishTime for machine %d: %v", allItems[i].ID, err)
			continue
		}
		allItems[i].FinishTimeParsed = parsedTime
	}

	if len(allItems) == 0 {
		log.Println("Scrape cycle finished: no items to process.")
		// Still need to process occupancy to archive any remaining open sessions.
	}

	// Step 2: Delegate persistence to the store layer
	if err := s.store.UpsertDormsAndMachines(ctx, allItems); err != nil {
		log.Printf("Error processing dorms and machines: %v", err)
		return // Return early if machine metadata fails
	}

	// Step 3: Delegate occupancy updates to the store layer
	machineIDsToNotify, err := s.store.UpdateOccupancy(ctx, now, allItems, s.getStateType)
	if err != nil {
		log.Printf("Error processing occupancy changes: %v", err)
	}

	// Dispatch notification jobs to the worker pool
	if len(machineIDsToNotify) > 0 {
		log.Printf("Dispatching notifications for %d machines", len(machineIDsToNotify))
		for _, machineID := range machineIDsToNotify {
			s.workerPool.Dispatch(machineID)
		}
	}

	log.Println("Scrape cycle finished.")
}

// parseTimestamp converts the API's timestamp string into a time.Time object, respecting the configured timezone.
func (s *Service) parseTimestamp(tsStr *string) (*time.Time, error) {
	if tsStr == nil || *tsStr == "" {
		return nil, nil
	}

	loc, err := time.LoadLocation(s.cfg.Scraper.Timezone)
	if err != nil {
		return nil, fmt.Errorf("failed to load timezone %q: %w", s.cfg.Scraper.Timezone, err)
	}

	layout := "2006-01-02 15:04:05" // The layout of the timestamp from the API
	parsedTime, err := time.ParseInLocation(layout, *tsStr, loc)
	if err != nil {
		return nil, fmt.Errorf("failed to parse timestamp %q: %w", *tsStr, err)
	}

	return &parsedTime, nil
}

// fetchPage fetches a single page of device data from the upstream API. (This function remains unchanged)
func (s *Service) fetchPage(ctx context.Context, page int) (*ApiResponse, error) {
	payload := make(map[string]any)
	for k, v := range s.cfg.Scraper.Request.Payload {
		payload[k] = v
	}
	payload["page"] = page

	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.Scraper.Request.URL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for key, value := range s.cfg.Scraper.Request.Headers {
		req.Header.Set(key, value)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received non-200 status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var apiResp ApiResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal api response: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("API returned non-zero application code: %d", apiResp.Code)
	}

	return &apiResp, nil
}
