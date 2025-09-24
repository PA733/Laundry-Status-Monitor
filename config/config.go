package config

import (
	"log"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the overall application configuration.
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Scraper    ScraperConfig    `yaml:"scraper"`
	Database   DatabaseConfig   `yaml:"database"`
	Push       PushConfig       `yaml:"push"`
	WorkerPool WorkerPoolConfig `yaml:"worker_pool"`
}

// WorkerPoolConfig holds the configuration for the notification worker pool.
type WorkerPoolConfig struct {
	Size int `yaml:"size"`
}

// PushConfig holds the VAPID keys for web push notifications.
type PushConfig struct {
	PublicKey  string `yaml:"vapid_public_key"`
	PrivateKey string `yaml:"vapid_private_key"`
	Subject    string `yaml:"subject"`
	TTL        int    `yaml:"ttl"`
}

// ServerConfig holds the server-related configuration.
type ServerConfig struct {
	Port            int     `yaml:"port"`
	RequestIPHeader string  `yaml:"request_ip_header"`
	RateLimitPerSec float64 `yaml:"rate_limit_per_sec"`
	CacheTTLSeconds int     `yaml:"cache_ttl_seconds"`
}

// ScraperConfig holds the scraper-related configuration.
type ScraperConfig struct {
	Enabled             bool           `yaml:"enabled"`
	IntervalSeconds     int            `yaml:"interval_seconds"`
	Interval            time.Duration  `yaml:"-"` // Ignored by YAML parser
	HTTPProxy           string         `yaml:"http_proxy"`
	Timezone            string         `yaml:"timezone"`
	Request             ScraperRequest `yaml:"request"`
	StateIdleValues     []int          `yaml:"state_idle_values"`
	StateOccupiedValues []int          `yaml:"state_occupied_values"`
	StateFaultyValues   []int          `yaml:"state_faulty_values"`
}

// ScraperRequest defines the HTTP request for the scraper.
type ScraperRequest struct {
	URL      string            `yaml:"url"`
	Headers  map[string]string `yaml:"headers"`
	PageSize int               `yaml:"pageSize"`
	Payload  map[string]any    `yaml:"payload"`
}

// DatabaseConfig holds the database connection configuration.
type DatabaseConfig struct {
	DSN                    string `yaml:"dsn"`
	MaxOpenConns           int    `yaml:"max_open_conns"`
	MaxIdleConns           int    `yaml:"max_idle_conns"`
	ConnMaxLifetimeMinutes int    `yaml:"conn_max_lifetime_minutes"`
	EnableTimescale        bool   `yaml:"enable_timescale"`
}

// Load reads the configuration from the given path.
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var cfg Config
	decoder := yaml.NewDecoder(f)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, err
	}

	if cfg.Scraper.IntervalSeconds <= 0 {
		cfg.Scraper.IntervalSeconds = 60
	}
	cfg.Scraper.Interval = time.Duration(cfg.Scraper.IntervalSeconds) * time.Second

	if cfg.Scraper.Request.PageSize <= 0 {
		cfg.Scraper.Request.PageSize = 100
	}

	if cfg.Push.TTL <= 0 {
		cfg.Push.TTL = 3600
	}

	if cfg.WorkerPool.Size <= 0 {
		log.Printf("worker_pool.size is not set or invalid; defaulting to 1")
		cfg.WorkerPool.Size = 1
	}

	return &cfg, nil
}
