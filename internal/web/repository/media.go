package repository

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/foxzi/sendry/internal/web/models"
	"github.com/google/uuid"
)

type MediaRepository struct {
	db *sql.DB
}

func NewMediaRepository(db *sql.DB) *MediaRepository {
	return &MediaRepository{db: db}
}

func (r *MediaRepository) Create(m *models.MediaFile) error {
	m.ID = uuid.New().String()
	m.CreatedAt = time.Now()

	_, err := r.db.Exec(`
		INSERT INTO media_files (id, name, orig_name, mime_type, size, url, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.Name, m.OrigName, m.MimeType, m.Size, m.URL, m.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create media file: %w", err)
	}
	return nil
}

func (r *MediaRepository) GetByID(id string) (*models.MediaFile, error) {
	m := &models.MediaFile{}
	err := r.db.QueryRow(`
		SELECT id, name, orig_name, mime_type, size, url, created_at
		FROM media_files WHERE id = ?`, id,
	).Scan(&m.ID, &m.Name, &m.OrigName, &m.MimeType, &m.Size, &m.URL, &m.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (r *MediaRepository) List(filter models.MediaListFilter) ([]models.MediaFile, int, error) {
	countQuery := "SELECT COUNT(*) FROM media_files WHERE 1=1"
	args := []any{}

	if filter.Search != "" {
		countQuery += " AND (orig_name LIKE ? OR name LIKE ?)"
		args = append(args, "%"+filter.Search+"%", "%"+filter.Search+"%")
	}

	var total int
	if err := r.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `SELECT id, name, orig_name, mime_type, size, url, created_at
		FROM media_files WHERE 1=1`

	queryArgs := []any{}
	if filter.Search != "" {
		query += " AND (orig_name LIKE ? OR name LIKE ?)"
		queryArgs = append(queryArgs, "%"+filter.Search+"%", "%"+filter.Search+"%")
	}

	query += " ORDER BY created_at DESC"

	if filter.Limit > 0 {
		query += " LIMIT ? OFFSET ?"
		queryArgs = append(queryArgs, filter.Limit, filter.Offset)
	}

	rows, err := r.db.Query(query, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var files []models.MediaFile
	for rows.Next() {
		var m models.MediaFile
		if err := rows.Scan(&m.ID, &m.Name, &m.OrigName, &m.MimeType, &m.Size, &m.URL, &m.CreatedAt); err != nil {
			return nil, 0, err
		}
		files = append(files, m)
	}
	return files, total, nil
}

func (r *MediaRepository) Delete(id string) error {
	_, err := r.db.Exec("DELETE FROM media_files WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete media file: %w", err)
	}
	return nil
}
