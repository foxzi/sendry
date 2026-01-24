package queue

import (
	"context"
)

// Queue defines the interface for message queue operations
type Queue interface {
	// Enqueue adds a message to the queue
	Enqueue(ctx context.Context, msg *Message) error

	// Dequeue gets the next message for processing
	// Returns nil, nil if the queue is empty
	Dequeue(ctx context.Context) (*Message, error)

	// Update updates the message status
	Update(ctx context.Context, msg *Message) error

	// Get retrieves a message by ID
	Get(ctx context.Context, id string) (*Message, error)

	// List returns a list of messages with optional filtering
	List(ctx context.Context, filter ListFilter) ([]*Message, error)

	// Delete removes a message from the queue
	Delete(ctx context.Context, id string) error

	// Stats returns queue statistics
	Stats(ctx context.Context) (*QueueStats, error)

	// Close closes the storage connection
	Close() error
}
