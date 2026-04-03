package repository

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/foxzi/sendry/internal/web/models"
	"github.com/google/uuid"
)

type BlockRepository struct {
	db *sql.DB
}

func NewBlockRepository(db *sql.DB) *BlockRepository {
	return &BlockRepository{db: db}
}

func (r *BlockRepository) Create(b *models.EmailBlock) error {
	b.ID = uuid.New().String()
	b.CreatedAt = time.Now()
	b.UpdatedAt = b.CreatedAt

	_, err := r.db.Exec(`
		INSERT INTO email_blocks (id, name, category, html, preview_text, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		b.ID, b.Name, b.Category, b.HTML, b.PreviewText, b.CreatedAt, b.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create block: %w", err)
	}
	return nil
}

func (r *BlockRepository) GetByID(id string) (*models.EmailBlock, error) {
	b := &models.EmailBlock{}
	err := r.db.QueryRow(`
		SELECT id, name, category, html, preview_text, created_at, updated_at
		FROM email_blocks WHERE id = ?`, id,
	).Scan(&b.ID, &b.Name, &b.Category, &b.HTML, &b.PreviewText, &b.CreatedAt, &b.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (r *BlockRepository) List(filter models.BlockListFilter) ([]models.EmailBlock, int, error) {
	countQuery := "SELECT COUNT(*) FROM email_blocks WHERE 1=1"
	args := []any{}

	if filter.Search != "" {
		countQuery += " AND (name LIKE ? OR preview_text LIKE ?)"
		args = append(args, "%"+filter.Search+"%", "%"+filter.Search+"%")
	}
	if filter.Category != "" {
		countQuery += " AND category = ?"
		args = append(args, filter.Category)
	}

	var total int
	if err := r.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `SELECT id, name, category, html, preview_text, created_at, updated_at
		FROM email_blocks WHERE 1=1`

	queryArgs := []any{}
	if filter.Search != "" {
		query += " AND (name LIKE ? OR preview_text LIKE ?)"
		queryArgs = append(queryArgs, "%"+filter.Search+"%", "%"+filter.Search+"%")
	}
	if filter.Category != "" {
		query += " AND category = ?"
		queryArgs = append(queryArgs, filter.Category)
	}

	query += " ORDER BY category, name"

	if filter.Limit > 0 {
		query += " LIMIT ? OFFSET ?"
		queryArgs = append(queryArgs, filter.Limit, filter.Offset)
	}

	rows, err := r.db.Query(query, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var blocks []models.EmailBlock
	for rows.Next() {
		var b models.EmailBlock
		if err := rows.Scan(&b.ID, &b.Name, &b.Category, &b.HTML, &b.PreviewText, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, 0, err
		}
		blocks = append(blocks, b)
	}
	return blocks, total, nil
}

func (r *BlockRepository) Update(b *models.EmailBlock) error {
	b.UpdatedAt = time.Now()
	_, err := r.db.Exec(`
		UPDATE email_blocks SET name = ?, category = ?, html = ?, preview_text = ?, updated_at = ?
		WHERE id = ?`,
		b.Name, b.Category, b.HTML, b.PreviewText, b.UpdatedAt, b.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update block: %w", err)
	}
	return nil
}

func (r *BlockRepository) Delete(id string) error {
	_, err := r.db.Exec("DELETE FROM email_blocks WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete block: %w", err)
	}
	return nil
}

func (r *BlockRepository) GetCategories() ([]string, error) {
	rows, err := r.db.Query("SELECT DISTINCT category FROM email_blocks WHERE category != '' ORDER BY category")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var categories []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		categories = append(categories, c)
	}
	return categories, nil
}

func (r *BlockRepository) ListGroupedByCategory() ([]models.BlockCategory, error) {
	rows, err := r.db.Query(`
		SELECT id, name, category, html, preview_text, created_at, updated_at
		FROM email_blocks ORDER BY category, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	catMap := make(map[string]*models.BlockCategory)
	var catOrder []string

	for rows.Next() {
		var b models.EmailBlock
		if err := rows.Scan(&b.ID, &b.Name, &b.Category, &b.HTML, &b.PreviewText, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, err
		}

		cat, ok := catMap[b.Category]
		if !ok {
			cat = &models.BlockCategory{
				ID:   b.Category,
				Name: b.Category,
			}
			catMap[b.Category] = cat
			catOrder = append(catOrder, b.Category)
		}
		cat.Blocks = append(cat.Blocks, b)
	}

	categories := make([]models.BlockCategory, 0, len(catOrder))
	for _, name := range catOrder {
		categories = append(categories, *catMap[name])
	}
	return categories, nil
}
