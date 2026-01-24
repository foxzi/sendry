package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
)

var (
	bucketMessages   = []byte("messages")
	bucketPending    = []byte("pending")
	bucketDeferred   = []byte("deferred")
	bucketDeadLetter = []byte("dead_letter")
)

// BoltStorage implements Queue interface using BoltDB
type BoltStorage struct {
	db *bolt.DB
}

// NewBoltStorage creates a new BoltDB storage
func NewBoltStorage(path string) (*BoltStorage, error) {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	db, err := bolt.Open(path, 0600, &bolt.Options{
		Timeout: 5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create buckets
	err = db.Update(func(tx *bolt.Tx) error {
		for _, bucket := range [][]byte{bucketMessages, bucketPending, bucketDeferred, bucketDeadLetter} {
			if _, err := tx.CreateBucketIfNotExists(bucket); err != nil {
				return fmt.Errorf("failed to create bucket %s: %w", bucket, err)
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, err
	}

	return &BoltStorage{db: db}, nil
}

// Enqueue adds a message to the queue
func (s *BoltStorage) Enqueue(ctx context.Context, msg *Message) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		// Store message
		msgBucket := tx.Bucket(bucketMessages)
		data, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("failed to marshal message: %w", err)
		}
		if err := msgBucket.Put([]byte(msg.ID), data); err != nil {
			return fmt.Errorf("failed to store message: %w", err)
		}

		// Add to pending index
		pendingBucket := tx.Bucket(bucketPending)
		indexKey := makeIndexKey(msg.CreatedAt, msg.ID)
		if err := pendingBucket.Put(indexKey, []byte(msg.ID)); err != nil {
			return fmt.Errorf("failed to add to pending index: %w", err)
		}

		return nil
	})
}

// Dequeue gets the next message for processing
func (s *BoltStorage) Dequeue(ctx context.Context) (*Message, error) {
	var msg *Message

	err := s.db.Update(func(tx *bolt.Tx) error {
		// First check deferred messages that are ready for retry
		deferredBucket := tx.Bucket(bucketDeferred)
		msgBucket := tx.Bucket(bucketMessages)

		c := deferredBucket.Cursor()
		now := time.Now()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			// Parse timestamp from key
			ts := parseTimestampFromKey(k)
			if ts.After(now) {
				break // All remaining are in the future
			}

			// Get the message
			msgData := msgBucket.Get(v)
			if msgData == nil {
				// Message was deleted, clean up index
				c.Delete()
				continue
			}

			var m Message
			if err := json.Unmarshal(msgData, &m); err != nil {
				continue
			}

			// Update status to sending
			m.Status = StatusSending
			m.UpdatedAt = now

			data, err := json.Marshal(&m)
			if err != nil {
				return err
			}

			if err := msgBucket.Put([]byte(m.ID), data); err != nil {
				return err
			}

			// Remove from deferred index
			if err := c.Delete(); err != nil {
				return err
			}

			msg = &m
			return nil
		}

		// If no deferred messages, check pending
		pendingBucket := tx.Bucket(bucketPending)
		c = pendingBucket.Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			msgData := msgBucket.Get(v)
			if msgData == nil {
				c.Delete()
				continue
			}

			var m Message
			if err := json.Unmarshal(msgData, &m); err != nil {
				continue
			}

			// Update status to sending
			m.Status = StatusSending
			m.UpdatedAt = now

			data, err := json.Marshal(&m)
			if err != nil {
				return err
			}

			if err := msgBucket.Put([]byte(m.ID), data); err != nil {
				return err
			}

			// Remove from pending index
			if err := c.Delete(); err != nil {
				return err
			}

			msg = &m
			return nil
		}

		return nil
	})

	return msg, err
}

// Update updates the message status
func (s *BoltStorage) Update(ctx context.Context, msg *Message) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		msgBucket := tx.Bucket(bucketMessages)

		msg.UpdatedAt = time.Now()

		data, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("failed to marshal message: %w", err)
		}

		if err := msgBucket.Put([]byte(msg.ID), data); err != nil {
			return fmt.Errorf("failed to update message: %w", err)
		}

		// If deferred, add to deferred index
		if msg.Status == StatusDeferred {
			deferredBucket := tx.Bucket(bucketDeferred)
			indexKey := makeIndexKey(msg.NextRetryAt, msg.ID)
			if err := deferredBucket.Put(indexKey, []byte(msg.ID)); err != nil {
				return fmt.Errorf("failed to add to deferred index: %w", err)
			}
		}

		return nil
	})
}

// Get retrieves a message by ID
func (s *BoltStorage) Get(ctx context.Context, id string) (*Message, error) {
	var msg *Message

	err := s.db.View(func(tx *bolt.Tx) error {
		msgBucket := tx.Bucket(bucketMessages)
		data := msgBucket.Get([]byte(id))
		if data == nil {
			return nil
		}

		msg = &Message{}
		return json.Unmarshal(data, msg)
	})

	return msg, err
}

// List returns a list of messages with optional filtering
func (s *BoltStorage) List(ctx context.Context, filter ListFilter) ([]*Message, error) {
	var messages []*Message

	err := s.db.View(func(tx *bolt.Tx) error {
		msgBucket := tx.Bucket(bucketMessages)
		c := msgBucket.Cursor()

		count := 0
		skipped := 0

		for k, v := c.First(); k != nil; k, v = c.Next() {
			var msg Message
			if err := json.Unmarshal(v, &msg); err != nil {
				continue
			}

			// Apply status filter
			if filter.Status != "" && msg.Status != filter.Status {
				continue
			}

			// Apply offset
			if skipped < filter.Offset {
				skipped++
				continue
			}

			messages = append(messages, &msg)
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

// Delete removes a message from the queue
func (s *BoltStorage) Delete(ctx context.Context, id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		msgBucket := tx.Bucket(bucketMessages)

		// Get message first to clean up indexes
		data := msgBucket.Get([]byte(id))
		if data != nil {
			var msg Message
			if err := json.Unmarshal(data, &msg); err == nil {
				// Clean up pending index
				pendingBucket := tx.Bucket(bucketPending)
				pendingKey := makeIndexKey(msg.CreatedAt, msg.ID)
				pendingBucket.Delete(pendingKey)

				// Clean up deferred index
				deferredBucket := tx.Bucket(bucketDeferred)
				deferredKey := makeIndexKey(msg.NextRetryAt, msg.ID)
				deferredBucket.Delete(deferredKey)
			}
		}

		return msgBucket.Delete([]byte(id))
	})
}

// Stats returns queue statistics
func (s *BoltStorage) Stats(ctx context.Context) (*QueueStats, error) {
	stats := &QueueStats{}

	err := s.db.View(func(tx *bolt.Tx) error {
		msgBucket := tx.Bucket(bucketMessages)
		c := msgBucket.Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			var msg Message
			if err := json.Unmarshal(v, &msg); err != nil {
				continue
			}

			stats.Total++
			switch msg.Status {
			case StatusPending:
				stats.Pending++
			case StatusSending:
				stats.Sending++
			case StatusDelivered:
				stats.Delivered++
			case StatusFailed:
				stats.Failed++
			case StatusDeferred:
				stats.Deferred++
			}
		}

		return nil
	})

	return stats, err
}

// Close closes the database connection
func (s *BoltStorage) Close() error {
	return s.db.Close()
}

// DB returns the underlying bolt.DB instance
func (s *BoltStorage) DB() *bolt.DB {
	return s.db
}

// makeIndexKey creates a sortable key from timestamp and ID
func makeIndexKey(t time.Time, id string) []byte {
	// Format: timestamp (RFC3339Nano) + ":" + id
	return []byte(t.Format(time.RFC3339Nano) + ":" + id)
}

// parseTimestampFromKey extracts timestamp from index key
func parseTimestampFromKey(key []byte) time.Time {
	s := string(key)
	// Find the separator
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ':' {
			ts, _ := time.Parse(time.RFC3339Nano, s[:i])
			return ts
		}
	}
	return time.Time{}
}

// Dead Letter Queue methods

// MoveToDLQ moves a failed message to the dead letter queue
func (s *BoltStorage) MoveToDLQ(ctx context.Context, msg *Message) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		dlqBucket := tx.Bucket(bucketDeadLetter)
		msgBucket := tx.Bucket(bucketMessages)

		// Store in DLQ with timestamp key for ordering
		msg.Status = StatusFailed
		msg.UpdatedAt = time.Now()

		data, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("failed to marshal message: %w", err)
		}

		// Add to DLQ index
		indexKey := makeIndexKey(msg.UpdatedAt, msg.ID)
		if err := dlqBucket.Put(indexKey, []byte(msg.ID)); err != nil {
			return fmt.Errorf("failed to add to DLQ index: %w", err)
		}

		// Update message in main storage
		if err := msgBucket.Put([]byte(msg.ID), data); err != nil {
			return fmt.Errorf("failed to update message: %w", err)
		}

		return nil
	})
}

// ListDLQ returns messages in the dead letter queue
func (s *BoltStorage) ListDLQ(ctx context.Context, limit, offset int) ([]*Message, error) {
	var messages []*Message

	err := s.db.View(func(tx *bolt.Tx) error {
		dlqBucket := tx.Bucket(bucketDeadLetter)
		msgBucket := tx.Bucket(bucketMessages)
		c := dlqBucket.Cursor()

		count := 0
		skipped := 0

		for k, v := c.First(); k != nil; k, v = c.Next() {
			// Apply offset
			if skipped < offset {
				skipped++
				continue
			}

			// Get message data
			msgData := msgBucket.Get(v)
			if msgData == nil {
				continue
			}

			var msg Message
			if err := json.Unmarshal(msgData, &msg); err != nil {
				continue
			}

			messages = append(messages, &msg)
			count++

			// Apply limit
			if limit > 0 && count >= limit {
				break
			}
		}

		return nil
	})

	return messages, err
}

// GetFromDLQ retrieves a message from the dead letter queue
func (s *BoltStorage) GetFromDLQ(ctx context.Context, id string) (*Message, error) {
	var msg *Message

	err := s.db.View(func(tx *bolt.Tx) error {
		msgBucket := tx.Bucket(bucketMessages)
		data := msgBucket.Get([]byte(id))
		if data == nil {
			return nil
		}

		msg = &Message{}
		if err := json.Unmarshal(data, msg); err != nil {
			return err
		}

		// Verify it's in DLQ (status = failed)
		if msg.Status != StatusFailed {
			msg = nil
		}

		return nil
	})

	return msg, err
}

// RetryFromDLQ moves a message from DLQ back to pending queue for retry
func (s *BoltStorage) RetryFromDLQ(ctx context.Context, id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		dlqBucket := tx.Bucket(bucketDeadLetter)
		msgBucket := tx.Bucket(bucketMessages)
		pendingBucket := tx.Bucket(bucketPending)

		// Get message
		data := msgBucket.Get([]byte(id))
		if data == nil {
			return fmt.Errorf("message not found: %s", id)
		}

		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			return fmt.Errorf("failed to unmarshal message: %w", err)
		}

		// Remove from DLQ index
		c := dlqBucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			if string(v) == id {
				if err := c.Delete(); err != nil {
					return err
				}
				break
			}
		}

		// Reset message status
		msg.Status = StatusPending
		msg.RetryCount = 0
		msg.LastError = ""
		msg.UpdatedAt = time.Now()

		newData, err := json.Marshal(&msg)
		if err != nil {
			return fmt.Errorf("failed to marshal message: %w", err)
		}

		// Update message
		if err := msgBucket.Put([]byte(id), newData); err != nil {
			return fmt.Errorf("failed to update message: %w", err)
		}

		// Add to pending queue
		indexKey := makeIndexKey(msg.UpdatedAt, msg.ID)
		if err := pendingBucket.Put(indexKey, []byte(msg.ID)); err != nil {
			return fmt.Errorf("failed to add to pending: %w", err)
		}

		return nil
	})
}

// DeleteFromDLQ permanently deletes a message from the dead letter queue
func (s *BoltStorage) DeleteFromDLQ(ctx context.Context, id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		dlqBucket := tx.Bucket(bucketDeadLetter)
		msgBucket := tx.Bucket(bucketMessages)

		// Remove from DLQ index
		c := dlqBucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			if string(v) == id {
				if err := c.Delete(); err != nil {
					return err
				}
				break
			}
		}

		// Delete message
		return msgBucket.Delete([]byte(id))
	})
}

// DLQStats returns dead letter queue statistics
func (s *BoltStorage) DLQStats(ctx context.Context) (*DLQStats, error) {
	stats := &DLQStats{}

	err := s.db.View(func(tx *bolt.Tx) error {
		dlqBucket := tx.Bucket(bucketDeadLetter)
		msgBucket := tx.Bucket(bucketMessages)

		c := dlqBucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			stats.Total++

			// Get oldest message timestamp
			if stats.Total == 1 {
				stats.OldestAt = parseTimestampFromKey(k)
			}

			// Get message for size calculation
			msgData := msgBucket.Get(v)
			if msgData != nil {
				stats.TotalSize += int64(len(msgData))
			}
		}

		return nil
	})

	return stats, err
}

// DLQStats contains dead letter queue statistics
type DLQStats struct {
	Total     int64     `json:"total"`
	TotalSize int64     `json:"total_size"`
	OldestAt  time.Time `json:"oldest_at,omitempty"`
}

// Cleanup methods

// CleanupDelivered removes delivered messages older than maxAge
func (s *BoltStorage) CleanupDelivered(ctx context.Context, maxAge time.Duration) (int, error) {
	if maxAge <= 0 {
		return 0, nil
	}

	cutoff := time.Now().Add(-maxAge)
	deleted := 0

	err := s.db.Update(func(tx *bolt.Tx) error {
		msgBucket := tx.Bucket(bucketMessages)
		c := msgBucket.Cursor()

		var toDelete [][]byte

		for k, v := c.First(); k != nil; k, v = c.Next() {
			var msg Message
			if err := json.Unmarshal(v, &msg); err != nil {
				continue
			}

			// Only delete delivered messages older than cutoff
			if msg.Status == StatusDelivered && msg.UpdatedAt.Before(cutoff) {
				toDelete = append(toDelete, append([]byte{}, k...))
			}
		}

		// Delete collected messages
		for _, k := range toDelete {
			if err := msgBucket.Delete(k); err != nil {
				return err
			}
			deleted++
		}

		return nil
	})

	return deleted, err
}

// CleanupDLQ removes DLQ messages by age and enforces max count (FIFO)
func (s *BoltStorage) CleanupDLQ(ctx context.Context, maxAge time.Duration, maxCount int) (int, error) {
	deleted := 0

	err := s.db.Update(func(tx *bolt.Tx) error {
		dlqBucket := tx.Bucket(bucketDeadLetter)
		msgBucket := tx.Bucket(bucketMessages)

		// Count current DLQ size and collect items to delete
		var toDeleteByAge []struct {
			indexKey []byte
			msgID    []byte
		}
		var allItems []struct {
			indexKey []byte
			msgID    []byte
		}

		now := time.Now()
		cutoff := now.Add(-maxAge)

		c := dlqBucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			item := struct {
				indexKey []byte
				msgID    []byte
			}{
				indexKey: append([]byte{}, k...),
				msgID:    append([]byte{}, v...),
			}
			allItems = append(allItems, item)

			// Check age-based deletion
			if maxAge > 0 {
				ts := parseTimestampFromKey(k)
				if ts.Before(cutoff) {
					toDeleteByAge = append(toDeleteByAge, item)
				}
			}
		}

		// Delete by age first
		for _, item := range toDeleteByAge {
			if err := dlqBucket.Delete(item.indexKey); err != nil {
				return err
			}
			if err := msgBucket.Delete(item.msgID); err != nil {
				return err
			}
			deleted++
		}

		// Calculate remaining count after age deletion
		remainingCount := len(allItems) - len(toDeleteByAge)

		// Enforce max count (delete oldest first - FIFO)
		if maxCount > 0 && remainingCount > maxCount {
			toDeleteCount := remainingCount - maxCount
			deleteCount := 0

			for _, item := range allItems {
				// Skip items already deleted by age
				alreadyDeleted := false
				for _, aged := range toDeleteByAge {
					if string(item.indexKey) == string(aged.indexKey) {
						alreadyDeleted = true
						break
					}
				}
				if alreadyDeleted {
					continue
				}

				if deleteCount >= toDeleteCount {
					break
				}

				if err := dlqBucket.Delete(item.indexKey); err != nil {
					return err
				}
				if err := msgBucket.Delete(item.msgID); err != nil {
					return err
				}
				deleted++
				deleteCount++
			}
		}

		return nil
	})

	return deleted, err
}
