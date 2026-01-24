package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

var bucketSandbox = []byte("sandbox")

// Message represents a message captured in sandbox mode
type Message struct {
	ID           string    `json:"id"`
	From         string    `json:"from"`
	To           []string  `json:"to"`
	OriginalTo   []string  `json:"original_to,omitempty"` // Original recipients before redirect
	Subject      string    `json:"subject"`
	Data         []byte    `json:"data"`
	Domain       string    `json:"domain"`        // Sending domain
	Mode         string    `json:"mode"`          // sandbox, redirect, bcc
	CapturedAt   time.Time `json:"captured_at"`
	ClientIP     string    `json:"client_ip,omitempty"`
	SimulatedErr string    `json:"simulated_error,omitempty"` // For error simulation
}

// Storage provides sandbox message storage
type Storage struct {
	db *bolt.DB
}

// NewStorage creates a new sandbox storage using the provided BoltDB instance
func NewStorage(db *bolt.DB) (*Storage, error) {
	// Create bucket if not exists
	err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketSandbox)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create sandbox bucket: %w", err)
	}

	return &Storage{db: db}, nil
}

// Save stores a message in the sandbox
func (s *Storage) Save(ctx context.Context, msg *Message) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketSandbox)

		// Create index key with timestamp for ordering
		indexKey := makeIndexKey(msg.CapturedAt, msg.ID)

		data, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("failed to marshal message: %w", err)
		}

		return bucket.Put(indexKey, data)
	})
}

// Get retrieves a message by ID
func (s *Storage) Get(ctx context.Context, id string) (*Message, error) {
	var msg *Message

	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketSandbox)
		c := bucket.Cursor()

		// Search for message with matching ID
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var m Message
			if err := json.Unmarshal(v, &m); err != nil {
				continue
			}
			if m.ID == id {
				msg = &m
				return nil
			}
		}
		return nil
	})

	return msg, err
}

// ListFilter contains filters for listing messages
type ListFilter struct {
	Domain string
	Mode   string
	From   string
	Limit  int
	Offset int
}

// List returns messages matching the filter
func (s *Storage) List(ctx context.Context, filter ListFilter) ([]*Message, error) {
	var messages []*Message

	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketSandbox)
		c := bucket.Cursor()

		skipped := 0
		count := 0

		// Iterate in reverse order (newest first)
		for k, v := c.Last(); k != nil; k, v = c.Prev() {
			var msg Message
			if err := json.Unmarshal(v, &msg); err != nil {
				continue
			}

			// Apply filters
			if filter.Domain != "" && msg.Domain != filter.Domain {
				continue
			}
			if filter.Mode != "" && msg.Mode != filter.Mode {
				continue
			}
			if filter.From != "" && msg.From != filter.From {
				continue
			}

			// Apply offset
			if skipped < filter.Offset {
				skipped++
				continue
			}

			messages = append(messages, &Message{
				ID:         msg.ID,
				From:       msg.From,
				To:         msg.To,
				OriginalTo: msg.OriginalTo,
				Subject:    msg.Subject,
				Domain:     msg.Domain,
				Mode:       msg.Mode,
				CapturedAt: msg.CapturedAt,
				ClientIP:   msg.ClientIP,
			})
			count++

			// Apply limit
			if filter.Limit > 0 && count >= filter.Limit {
				break
			}
		}

		return nil
	})

	return messages, err
}

// Delete removes a message by ID
func (s *Storage) Delete(ctx context.Context, id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketSandbox)
		c := bucket.Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			var msg Message
			if err := json.Unmarshal(v, &msg); err != nil {
				continue
			}
			if msg.ID == id {
				return c.Delete()
			}
		}
		return nil
	})
}

// Clear removes all messages, optionally filtered by domain or age
func (s *Storage) Clear(ctx context.Context, domain string, olderThan time.Duration) (int, error) {
	var count int
	cutoff := time.Now().Add(-olderThan)

	err := s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketSandbox)
		c := bucket.Cursor()

		var keysToDelete [][]byte

		for k, v := c.First(); k != nil; k, v = c.Next() {
			var msg Message
			if err := json.Unmarshal(v, &msg); err != nil {
				continue
			}

			// Filter by domain
			if domain != "" && msg.Domain != domain {
				continue
			}

			// Filter by age
			if olderThan > 0 && msg.CapturedAt.After(cutoff) {
				continue
			}

			keysToDelete = append(keysToDelete, k)
		}

		for _, k := range keysToDelete {
			if err := bucket.Delete(k); err != nil {
				return err
			}
			count++
		}

		return nil
	})

	return count, err
}

// Stats returns sandbox statistics
type Stats struct {
	Total     int64            `json:"total"`
	ByDomain  map[string]int64 `json:"by_domain"`
	ByMode    map[string]int64 `json:"by_mode"`
	OldestAt  time.Time        `json:"oldest_at,omitempty"`
	NewestAt  time.Time        `json:"newest_at,omitempty"`
	TotalSize int64            `json:"total_size"`
}

// Stats returns sandbox statistics
func (s *Storage) Stats(ctx context.Context) (*Stats, error) {
	stats := &Stats{
		ByDomain: make(map[string]int64),
		ByMode:   make(map[string]int64),
	}

	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketSandbox)
		c := bucket.Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			var msg Message
			if err := json.Unmarshal(v, &msg); err != nil {
				continue
			}

			stats.Total++
			stats.TotalSize += int64(len(v))
			stats.ByDomain[msg.Domain]++
			stats.ByMode[msg.Mode]++

			if stats.OldestAt.IsZero() || msg.CapturedAt.Before(stats.OldestAt) {
				stats.OldestAt = msg.CapturedAt
			}
			if msg.CapturedAt.After(stats.NewestAt) {
				stats.NewestAt = msg.CapturedAt
			}
		}

		return nil
	})

	return stats, err
}

func makeIndexKey(t time.Time, id string) []byte {
	return []byte(t.Format(time.RFC3339Nano) + ":" + id)
}
