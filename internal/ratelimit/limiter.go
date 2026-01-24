package ratelimit

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

var bucketRateLimits = []byte("rate_limits")

// Level represents the level of rate limiting
type Level string

const (
	LevelGlobal    Level = "global"
	LevelDomain    Level = "domain"
	LevelSender    Level = "sender"
	LevelIP        Level = "ip"
	LevelAPIKey    Level = "api_key"
	LevelRecipient Level = "recipient_domain"
)

// Config contains rate limit configuration
type Config struct {
	// Global limits
	Global *LimitConfig `yaml:"global,omitempty"`

	// Default limits for domains without specific config
	DefaultDomain *LimitConfig `yaml:"default_domain,omitempty"`

	// Default limits for senders without specific config
	DefaultSender *LimitConfig `yaml:"default_sender,omitempty"`

	// Default limits for IPs without specific config
	DefaultIP *LimitConfig `yaml:"default_ip,omitempty"`

	// Default limits for API keys without specific config
	DefaultAPIKey *LimitConfig `yaml:"default_api_key,omitempty"`

	// Persistence settings
	FlushInterval time.Duration `yaml:"flush_interval,omitempty"`
}

// LimitConfig contains rate limit values
type LimitConfig struct {
	MessagesPerHour int `yaml:"messages_per_hour" json:"messages_per_hour"`
	MessagesPerDay  int `yaml:"messages_per_day" json:"messages_per_day"`
}

// Counter tracks rate limit counters
type Counter struct {
	HourlyCount int       `json:"hourly_count"`
	DailyCount  int       `json:"daily_count"`
	HourStart   time.Time `json:"hour_start"`
	DayStart    time.Time `json:"day_start"`
}

// Limiter implements rate limiting with multiple levels
type Limiter struct {
	db       *bolt.DB
	config   *Config
	counters map[string]*Counter // key -> counter
	mu       sync.RWMutex
	stopCh   chan struct{}
}

// NewLimiter creates a new rate limiter
func NewLimiter(db *bolt.DB, cfg *Config) (*Limiter, error) {
	if cfg == nil {
		cfg = &Config{}
	}

	if cfg.FlushInterval == 0 {
		cfg.FlushInterval = 10 * time.Second
	}

	// Create bucket if not exists
	err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketRateLimits)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create rate limits bucket: %w", err)
	}

	l := &Limiter{
		db:       db,
		config:   cfg,
		counters: make(map[string]*Counter),
		stopCh:   make(chan struct{}),
	}

	// Load persisted counters
	if err := l.loadCounters(); err != nil {
		return nil, fmt.Errorf("failed to load counters: %w", err)
	}

	// Start background persistence
	go l.persistLoop()

	return l, nil
}

// Allow checks if the action is allowed and increments counters
func (l *Limiter) Allow(ctx context.Context, req *Request) (*Result, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	result := &Result{
		Allowed: true,
	}

	now := time.Now()

	// Check all applicable limits
	checks := l.getChecks(req)

	for _, check := range checks {
		counter := l.getOrCreateCounter(check.key, now)

		// Reset counters if time window has passed
		l.resetExpiredCounters(counter, now)

		// Check hourly limit
		if check.limit.MessagesPerHour > 0 && counter.HourlyCount >= check.limit.MessagesPerHour {
			result.Allowed = false
			result.DeniedBy = check.level
			result.DeniedKey = check.key
			result.RetryAfter = counter.HourStart.Add(time.Hour).Sub(now)
			return result, nil
		}

		// Check daily limit
		if check.limit.MessagesPerDay > 0 && counter.DailyCount >= check.limit.MessagesPerDay {
			result.Allowed = false
			result.DeniedBy = check.level
			result.DeniedKey = check.key
			result.RetryAfter = counter.DayStart.Add(24 * time.Hour).Sub(now)
			return result, nil
		}
	}

	// Increment all counters if allowed
	for _, check := range checks {
		counter := l.counters[check.key]
		counter.HourlyCount++
		counter.DailyCount++
	}

	return result, nil
}

// Check checks if the action would be allowed without incrementing counters
func (l *Limiter) Check(ctx context.Context, req *Request) (*Result, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := &Result{
		Allowed: true,
	}

	now := time.Now()
	checks := l.getChecks(req)

	for _, check := range checks {
		counter, exists := l.counters[check.key]
		if !exists {
			continue
		}

		// Check if counters are still valid
		hourlyCount := counter.HourlyCount
		dailyCount := counter.DailyCount

		if now.Sub(counter.HourStart) >= time.Hour {
			hourlyCount = 0
		}
		if now.Sub(counter.DayStart) >= 24*time.Hour {
			dailyCount = 0
		}

		// Check hourly limit
		if check.limit.MessagesPerHour > 0 && hourlyCount >= check.limit.MessagesPerHour {
			result.Allowed = false
			result.DeniedBy = check.level
			result.DeniedKey = check.key
			result.RetryAfter = counter.HourStart.Add(time.Hour).Sub(now)
			return result, nil
		}

		// Check daily limit
		if check.limit.MessagesPerDay > 0 && dailyCount >= check.limit.MessagesPerDay {
			result.Allowed = false
			result.DeniedBy = check.level
			result.DeniedKey = check.key
			result.RetryAfter = counter.DayStart.Add(24 * time.Hour).Sub(now)
			return result, nil
		}
	}

	return result, nil
}

// GetStats returns current rate limit statistics
func (l *Limiter) GetStats(ctx context.Context, level Level, key string) (*Stats, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	fullKey := makeKey(level, key)
	counter, exists := l.counters[fullKey]
	if !exists {
		return &Stats{
			Level: level,
			Key:   key,
		}, nil
	}

	now := time.Now()
	stats := &Stats{
		Level:       level,
		Key:         key,
		HourlyCount: counter.HourlyCount,
		DailyCount:  counter.DailyCount,
		HourStart:   counter.HourStart,
		DayStart:    counter.DayStart,
	}

	// Reset if expired
	if now.Sub(counter.HourStart) >= time.Hour {
		stats.HourlyCount = 0
	}
	if now.Sub(counter.DayStart) >= 24*time.Hour {
		stats.DailyCount = 0
	}

	return stats, nil
}

// Stop stops the rate limiter and persists counters
func (l *Limiter) Stop() error {
	close(l.stopCh)
	return l.persistCounters()
}

// Request contains information about the rate limit request
type Request struct {
	Domain    string // Sender domain
	Sender    string // Sender email
	IP        string // Client IP
	APIKey    string // API key (if from API)
	Recipient string // Recipient domain (optional)
}

// Result contains the rate limit check result
type Result struct {
	Allowed    bool
	DeniedBy   Level
	DeniedKey  string
	RetryAfter time.Duration
}

// Stats contains rate limit statistics
type Stats struct {
	Level       Level
	Key         string
	HourlyCount int
	DailyCount  int
	HourStart   time.Time
	DayStart    time.Time
}

type limitCheck struct {
	level Level
	key   string
	limit *LimitConfig
}

func (l *Limiter) getChecks(req *Request) []limitCheck {
	var checks []limitCheck

	// Global limit
	if l.config.Global != nil {
		checks = append(checks, limitCheck{
			level: LevelGlobal,
			key:   makeKey(LevelGlobal, "global"),
			limit: l.config.Global,
		})
	}

	// Domain limit
	if req.Domain != "" && l.config.DefaultDomain != nil {
		checks = append(checks, limitCheck{
			level: LevelDomain,
			key:   makeKey(LevelDomain, req.Domain),
			limit: l.config.DefaultDomain,
		})
	}

	// Sender limit
	if req.Sender != "" && l.config.DefaultSender != nil {
		checks = append(checks, limitCheck{
			level: LevelSender,
			key:   makeKey(LevelSender, req.Sender),
			limit: l.config.DefaultSender,
		})
	}

	// IP limit
	if req.IP != "" && l.config.DefaultIP != nil {
		checks = append(checks, limitCheck{
			level: LevelIP,
			key:   makeKey(LevelIP, req.IP),
			limit: l.config.DefaultIP,
		})
	}

	// API key limit
	if req.APIKey != "" && l.config.DefaultAPIKey != nil {
		checks = append(checks, limitCheck{
			level: LevelAPIKey,
			key:   makeKey(LevelAPIKey, req.APIKey),
			limit: l.config.DefaultAPIKey,
		})
	}

	return checks
}

func (l *Limiter) getOrCreateCounter(key string, now time.Time) *Counter {
	counter, exists := l.counters[key]
	if !exists {
		counter = &Counter{
			HourStart: now,
			DayStart:  now,
		}
		l.counters[key] = counter
	}
	return counter
}

func (l *Limiter) resetExpiredCounters(counter *Counter, now time.Time) {
	if now.Sub(counter.HourStart) >= time.Hour {
		counter.HourlyCount = 0
		counter.HourStart = now
	}
	if now.Sub(counter.DayStart) >= 24*time.Hour {
		counter.DailyCount = 0
		counter.DayStart = now
	}
}

func (l *Limiter) loadCounters() error {
	return l.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketRateLimits)
		if bucket == nil {
			return nil
		}

		return bucket.ForEach(func(k, v []byte) error {
			var counter Counter
			if err := json.Unmarshal(v, &counter); err != nil {
				return nil // Skip invalid entries
			}
			l.counters[string(k)] = &counter
			return nil
		})
	})
}

func (l *Limiter) persistCounters() error {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketRateLimits)
		if bucket == nil {
			return nil
		}

		for key, counter := range l.counters {
			data, err := json.Marshal(counter)
			if err != nil {
				continue
			}
			if err := bucket.Put([]byte(key), data); err != nil {
				return err
			}
		}
		return nil
	})
}

func (l *Limiter) persistLoop() {
	ticker := time.NewTicker(l.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-l.stopCh:
			return
		case <-ticker.C:
			l.persistCounters()
		}
	}
}

func makeKey(level Level, key string) string {
	return string(level) + ":" + key
}
