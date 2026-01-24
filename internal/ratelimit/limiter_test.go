package ratelimit

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func setupTestDB(t *testing.T) (*bolt.DB, func()) {
	t.Helper()

	dir, err := os.MkdirTemp("", "ratelimit_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(dir, "test.db")
	db, err := bolt.Open(dbPath, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		os.RemoveAll(dir)
		t.Fatalf("failed to open db: %v", err)
	}

	cleanup := func() {
		db.Close()
		os.RemoveAll(dir)
	}

	return db, cleanup
}

func TestNewLimiter(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cfg := &Config{
		Global: &LimitConfig{
			MessagesPerHour: 100,
			MessagesPerDay:  1000,
		},
		FlushInterval: 100 * time.Millisecond,
	}

	limiter, err := NewLimiter(db, cfg)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Stop()

	if limiter.config.Global.MessagesPerHour != 100 {
		t.Errorf("expected MessagesPerHour=100, got %d", limiter.config.Global.MessagesPerHour)
	}
}

func TestNewLimiterDefaultConfig(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	limiter, err := NewLimiter(db, nil)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Stop()

	if limiter.config.FlushInterval != 10*time.Second {
		t.Errorf("expected default FlushInterval=10s, got %v", limiter.config.FlushInterval)
	}
}

func TestAllowGlobalLimit(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cfg := &Config{
		Global: &LimitConfig{
			MessagesPerHour: 3,
			MessagesPerDay:  10,
		},
		FlushInterval: time.Hour, // Don't flush during test
	}

	limiter, err := NewLimiter(db, cfg)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Stop()

	ctx := context.Background()
	req := &Request{
		Domain: "example.com",
		Sender: "user@example.com",
	}

	// First 3 requests should be allowed
	for i := 0; i < 3; i++ {
		result, err := limiter.Allow(ctx, req)
		if err != nil {
			t.Fatalf("Allow failed: %v", err)
		}
		if !result.Allowed {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	// 4th request should be denied
	result, err := limiter.Allow(ctx, req)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if result.Allowed {
		t.Error("request 4 should be denied")
	}
	if result.DeniedBy != LevelGlobal {
		t.Errorf("expected DeniedBy=global, got %s", result.DeniedBy)
	}
	if result.RetryAfter <= 0 {
		t.Error("expected positive RetryAfter")
	}
}

func TestAllowDomainLimit(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cfg := &Config{
		DefaultDomain: &LimitConfig{
			MessagesPerHour: 2,
		},
		FlushInterval: time.Hour,
	}

	limiter, err := NewLimiter(db, cfg)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Stop()

	ctx := context.Background()

	// Domain A: 2 requests allowed
	reqA := &Request{Domain: "domain-a.com"}
	for i := 0; i < 2; i++ {
		result, _ := limiter.Allow(ctx, reqA)
		if !result.Allowed {
			t.Errorf("domain A request %d should be allowed", i+1)
		}
	}
	result, _ := limiter.Allow(ctx, reqA)
	if result.Allowed {
		t.Error("domain A request 3 should be denied")
	}

	// Domain B: should still have its own limit
	reqB := &Request{Domain: "domain-b.com"}
	result, _ = limiter.Allow(ctx, reqB)
	if !result.Allowed {
		t.Error("domain B request 1 should be allowed")
	}
}

func TestAllowSenderLimit(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cfg := &Config{
		DefaultSender: &LimitConfig{
			MessagesPerHour: 2,
		},
		FlushInterval: time.Hour,
	}

	limiter, err := NewLimiter(db, cfg)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Stop()

	ctx := context.Background()

	// Sender A: 2 requests allowed
	reqA := &Request{Sender: "alice@example.com"}
	for i := 0; i < 2; i++ {
		result, _ := limiter.Allow(ctx, reqA)
		if !result.Allowed {
			t.Errorf("sender A request %d should be allowed", i+1)
		}
	}
	result, _ := limiter.Allow(ctx, reqA)
	if result.Allowed {
		t.Error("sender A request 3 should be denied")
	}

	// Sender B: should still have its own limit
	reqB := &Request{Sender: "bob@example.com"}
	result, _ = limiter.Allow(ctx, reqB)
	if !result.Allowed {
		t.Error("sender B request 1 should be allowed")
	}
}

func TestAllowIPLimit(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cfg := &Config{
		DefaultIP: &LimitConfig{
			MessagesPerHour: 2,
		},
		FlushInterval: time.Hour,
	}

	limiter, err := NewLimiter(db, cfg)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Stop()

	ctx := context.Background()
	req := &Request{IP: "192.168.1.1"}

	for i := 0; i < 2; i++ {
		result, _ := limiter.Allow(ctx, req)
		if !result.Allowed {
			t.Errorf("IP request %d should be allowed", i+1)
		}
	}
	result, _ := limiter.Allow(ctx, req)
	if result.Allowed {
		t.Error("IP request 3 should be denied")
	}
	if result.DeniedBy != LevelIP {
		t.Errorf("expected DeniedBy=ip, got %s", result.DeniedBy)
	}
}

func TestAllowAPIKeyLimit(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cfg := &Config{
		DefaultAPIKey: &LimitConfig{
			MessagesPerHour: 2,
		},
		FlushInterval: time.Hour,
	}

	limiter, err := NewLimiter(db, cfg)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Stop()

	ctx := context.Background()
	req := &Request{APIKey: "key-123"}

	for i := 0; i < 2; i++ {
		result, _ := limiter.Allow(ctx, req)
		if !result.Allowed {
			t.Errorf("API key request %d should be allowed", i+1)
		}
	}
	result, _ := limiter.Allow(ctx, req)
	if result.Allowed {
		t.Error("API key request 3 should be denied")
	}
	if result.DeniedBy != LevelAPIKey {
		t.Errorf("expected DeniedBy=api_key, got %s", result.DeniedBy)
	}
}

func TestAllowDailyLimit(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cfg := &Config{
		Global: &LimitConfig{
			MessagesPerHour: 100, // High hourly limit
			MessagesPerDay:  3,   // Low daily limit
		},
		FlushInterval: time.Hour,
	}

	limiter, err := NewLimiter(db, cfg)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Stop()

	ctx := context.Background()
	req := &Request{Domain: "example.com"}

	for i := 0; i < 3; i++ {
		result, _ := limiter.Allow(ctx, req)
		if !result.Allowed {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	// Should hit daily limit
	result, _ := limiter.Allow(ctx, req)
	if result.Allowed {
		t.Error("request 4 should be denied by daily limit")
	}
}

func TestCheck(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cfg := &Config{
		Global: &LimitConfig{
			MessagesPerHour: 2,
		},
		FlushInterval: time.Hour,
	}

	limiter, err := NewLimiter(db, cfg)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Stop()

	ctx := context.Background()
	req := &Request{Domain: "example.com"}

	// Check should not increment counters
	for i := 0; i < 5; i++ {
		result, err := limiter.Check(ctx, req)
		if err != nil {
			t.Fatalf("Check failed: %v", err)
		}
		if !result.Allowed {
			t.Errorf("Check %d should return allowed (doesn't increment)", i+1)
		}
	}

	// Allow should still work since Check didn't increment
	result, _ := limiter.Allow(ctx, req)
	if !result.Allowed {
		t.Error("first Allow should be allowed")
	}
}

func TestGetStats(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cfg := &Config{
		Global: &LimitConfig{
			MessagesPerHour: 100,
		},
		FlushInterval: time.Hour,
	}

	limiter, err := NewLimiter(db, cfg)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Stop()

	ctx := context.Background()
	req := &Request{Domain: "example.com"}

	// Make 3 requests
	for i := 0; i < 3; i++ {
		limiter.Allow(ctx, req)
	}

	// Get stats
	stats, err := limiter.GetStats(ctx, LevelGlobal, "global")
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.HourlyCount != 3 {
		t.Errorf("expected HourlyCount=3, got %d", stats.HourlyCount)
	}
	if stats.DailyCount != 3 {
		t.Errorf("expected DailyCount=3, got %d", stats.DailyCount)
	}
}

func TestGetStatsNonExistent(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	limiter, err := NewLimiter(db, nil)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Stop()

	ctx := context.Background()

	// Get stats for non-existent key
	stats, err := limiter.GetStats(ctx, LevelDomain, "nonexistent.com")
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.HourlyCount != 0 {
		t.Errorf("expected HourlyCount=0, got %d", stats.HourlyCount)
	}
}

func TestMultipleLevels(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cfg := &Config{
		Global: &LimitConfig{
			MessagesPerHour: 100,
		},
		DefaultDomain: &LimitConfig{
			MessagesPerHour: 50,
		},
		DefaultSender: &LimitConfig{
			MessagesPerHour: 2, // Strictest limit
		},
		FlushInterval: time.Hour,
	}

	limiter, err := NewLimiter(db, cfg)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Stop()

	ctx := context.Background()
	req := &Request{
		Domain: "example.com",
		Sender: "user@example.com",
	}

	// First 2 requests allowed
	for i := 0; i < 2; i++ {
		result, _ := limiter.Allow(ctx, req)
		if !result.Allowed {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	// 3rd should be denied by sender limit (strictest)
	result, _ := limiter.Allow(ctx, req)
	if result.Allowed {
		t.Error("request 3 should be denied")
	}
	if result.DeniedBy != LevelSender {
		t.Errorf("expected DeniedBy=sender, got %s", result.DeniedBy)
	}
}

func TestPersistence(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cfg := &Config{
		Global: &LimitConfig{
			MessagesPerHour: 10,
		},
		FlushInterval: 50 * time.Millisecond,
	}

	// Create limiter and make requests
	limiter, err := NewLimiter(db, cfg)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	ctx := context.Background()
	req := &Request{Domain: "example.com"}

	for i := 0; i < 5; i++ {
		limiter.Allow(ctx, req)
	}

	// Wait for persistence
	time.Sleep(100 * time.Millisecond)
	limiter.Stop()

	// Create new limiter with same DB
	limiter2, err := NewLimiter(db, cfg)
	if err != nil {
		t.Fatalf("failed to create second limiter: %v", err)
	}
	defer limiter2.Stop()

	// Stats should be loaded
	stats, err := limiter2.GetStats(ctx, LevelGlobal, "global")
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.HourlyCount != 5 {
		t.Errorf("expected persisted HourlyCount=5, got %d", stats.HourlyCount)
	}
}

func TestMakeKey(t *testing.T) {
	tests := []struct {
		level    Level
		key      string
		expected string
	}{
		{LevelGlobal, "global", "global:global"},
		{LevelDomain, "example.com", "domain:example.com"},
		{LevelSender, "user@example.com", "sender:user@example.com"},
		{LevelIP, "192.168.1.1", "ip:192.168.1.1"},
		{LevelAPIKey, "key-123", "api_key:key-123"},
	}

	for _, tc := range tests {
		result := makeKey(tc.level, tc.key)
		if result != tc.expected {
			t.Errorf("makeKey(%s, %s) = %s, expected %s", tc.level, tc.key, result, tc.expected)
		}
	}
}

func TestZeroLimits(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Zero limits mean unlimited
	cfg := &Config{
		Global: &LimitConfig{
			MessagesPerHour: 0,
			MessagesPerDay:  0,
		},
		FlushInterval: time.Hour,
	}

	limiter, err := NewLimiter(db, cfg)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Stop()

	ctx := context.Background()
	req := &Request{Domain: "example.com"}

	// Many requests should be allowed
	for i := 0; i < 1000; i++ {
		result, _ := limiter.Allow(ctx, req)
		if !result.Allowed {
			t.Errorf("request %d should be allowed with zero limits", i+1)
			break
		}
	}
}
