package template

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	bolt "go.etcd.io/bbolt"
)

var (
	bucketTemplates    = []byte("templates")
	bucketTemplateNames = []byte("template_names")
)

// Storage provides template storage operations
type Storage struct {
	db *bolt.DB
}

// NewStorage creates a new template storage
func NewStorage(db *bolt.DB) (*Storage, error) {
	err := db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(bucketTemplates); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists(bucketTemplateNames); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create template buckets: %w", err)
	}
	return &Storage{db: db}, nil
}

// Create creates a new template
func (s *Storage) Create(ctx context.Context, tmpl *Template) error {
	if tmpl.Name == "" {
		return fmt.Errorf("template name is required")
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		templates := tx.Bucket(bucketTemplates)
		names := tx.Bucket(bucketTemplateNames)

		// Check name uniqueness
		if existing := names.Get([]byte(tmpl.Name)); existing != nil {
			return fmt.Errorf("template with name %q already exists", tmpl.Name)
		}

		// Generate ID and set metadata
		tmpl.ID = uuid.New().String()
		tmpl.Version = 1
		tmpl.CreatedAt = time.Now()
		tmpl.UpdatedAt = tmpl.CreatedAt

		data, err := json.Marshal(tmpl)
		if err != nil {
			return fmt.Errorf("failed to marshal template: %w", err)
		}

		// Store template
		if err := templates.Put([]byte(tmpl.ID), data); err != nil {
			return err
		}

		// Create name index
		return names.Put([]byte(tmpl.Name), []byte(tmpl.ID))
	})
}

// Get retrieves a template by ID
func (s *Storage) Get(ctx context.Context, id string) (*Template, error) {
	var tmpl *Template

	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketTemplates)
		data := bucket.Get([]byte(id))
		if data == nil {
			return nil
		}

		tmpl = &Template{}
		return json.Unmarshal(data, tmpl)
	})

	return tmpl, err
}

// GetByName retrieves a template by name
func (s *Storage) GetByName(ctx context.Context, name string) (*Template, error) {
	var tmpl *Template

	err := s.db.View(func(tx *bolt.Tx) error {
		names := tx.Bucket(bucketTemplateNames)
		id := names.Get([]byte(name))
		if id == nil {
			return nil
		}

		templates := tx.Bucket(bucketTemplates)
		data := templates.Get(id)
		if data == nil {
			return nil
		}

		tmpl = &Template{}
		return json.Unmarshal(data, tmpl)
	})

	return tmpl, err
}

// List returns templates with optional filtering
func (s *Storage) List(ctx context.Context, filter ListFilter) ([]*Template, error) {
	var templates []*Template

	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketTemplates)
		c := bucket.Cursor()

		skipped := 0
		count := 0

		for k, v := c.First(); k != nil; k, v = c.Next() {
			var tmpl Template
			if err := json.Unmarshal(v, &tmpl); err != nil {
				continue
			}

			// Apply search filter
			if filter.Search != "" {
				search := strings.ToLower(filter.Search)
				name := strings.ToLower(tmpl.Name)
				desc := strings.ToLower(tmpl.Description)
				if !strings.Contains(name, search) && !strings.Contains(desc, search) {
					continue
				}
			}

			// Apply offset
			if skipped < filter.Offset {
				skipped++
				continue
			}

			templates = append(templates, &tmpl)
			count++

			// Apply limit
			if filter.Limit > 0 && count >= filter.Limit {
				break
			}
		}

		return nil
	})

	return templates, err
}

// Update updates an existing template
func (s *Storage) Update(ctx context.Context, tmpl *Template) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		templates := tx.Bucket(bucketTemplates)
		names := tx.Bucket(bucketTemplateNames)

		// Get existing template
		existingData := templates.Get([]byte(tmpl.ID))
		if existingData == nil {
			return fmt.Errorf("template not found")
		}

		var existing Template
		if err := json.Unmarshal(existingData, &existing); err != nil {
			return err
		}

		// If name changed, update index
		if existing.Name != tmpl.Name {
			// Check new name is unique
			if existingID := names.Get([]byte(tmpl.Name)); existingID != nil {
				return fmt.Errorf("template with name %q already exists", tmpl.Name)
			}
			// Remove old name index
			if err := names.Delete([]byte(existing.Name)); err != nil {
				return err
			}
			// Add new name index
			if err := names.Put([]byte(tmpl.Name), []byte(tmpl.ID)); err != nil {
				return err
			}
		}

		// Update metadata
		tmpl.Version = existing.Version + 1
		tmpl.CreatedAt = existing.CreatedAt
		tmpl.UpdatedAt = time.Now()

		data, err := json.Marshal(tmpl)
		if err != nil {
			return fmt.Errorf("failed to marshal template: %w", err)
		}

		return templates.Put([]byte(tmpl.ID), data)
	})
}

// Delete removes a template by ID
func (s *Storage) Delete(ctx context.Context, id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		templates := tx.Bucket(bucketTemplates)
		names := tx.Bucket(bucketTemplateNames)

		// Get template to find name
		data := templates.Get([]byte(id))
		if data == nil {
			return nil // Already deleted
		}

		var tmpl Template
		if err := json.Unmarshal(data, &tmpl); err != nil {
			return err
		}

		// Remove name index
		if err := names.Delete([]byte(tmpl.Name)); err != nil {
			return err
		}

		return templates.Delete([]byte(id))
	})
}

// Stats returns template statistics
func (s *Storage) Stats(ctx context.Context) (*Stats, error) {
	stats := &Stats{}

	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketTemplates)
		c := bucket.Cursor()

		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			stats.Total++
		}

		return nil
	})

	return stats, err
}
