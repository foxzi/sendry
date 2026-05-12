package repository

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/foxzi/sendry/internal/web/models"
	"github.com/google/uuid"
)

type UserSMTPRepository struct {
	db *sql.DB
}

func NewUserSMTPRepository(db *sql.DB) *UserSMTPRepository {
	return &UserSMTPRepository{db: db}
}

func (r *UserSMTPRepository) Create(s *models.UserSMTPServer) error {
	if s.ID == "" {
		s.ID = uuid.New().String()
	}
	now := time.Now()
	s.CreatedAt = now
	s.UpdatedAt = now
	_, err := r.db.Exec(`
		INSERT INTO user_smtp_servers
			(id, user_id, name, host, port, username, password_enc, encryption, from_address, from_name, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.UserID, s.Name, s.Host, s.Port, s.Username, s.PasswordEnc, s.Encryption, s.FromAddress, s.FromName, s.CreatedAt, s.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create smtp: %w", err)
	}
	return nil
}

func (r *UserSMTPRepository) Update(s *models.UserSMTPServer) error {
	s.UpdatedAt = time.Now()
	if s.PasswordEnc == "" {
		_, err := r.db.Exec(`
			UPDATE user_smtp_servers
			SET name = ?, host = ?, port = ?, username = ?, encryption = ?, from_address = ?, from_name = ?, updated_at = ?
			WHERE id = ? AND user_id = ?`,
			s.Name, s.Host, s.Port, s.Username, s.Encryption, s.FromAddress, s.FromName, s.UpdatedAt, s.ID, s.UserID,
		)
		if err != nil {
			return fmt.Errorf("update smtp: %w", err)
		}
		return nil
	}
	_, err := r.db.Exec(`
		UPDATE user_smtp_servers
		SET name = ?, host = ?, port = ?, username = ?, password_enc = ?, encryption = ?, from_address = ?, from_name = ?, updated_at = ?
		WHERE id = ? AND user_id = ?`,
		s.Name, s.Host, s.Port, s.Username, s.PasswordEnc, s.Encryption, s.FromAddress, s.FromName, s.UpdatedAt, s.ID, s.UserID,
	)
	if err != nil {
		return fmt.Errorf("update smtp: %w", err)
	}
	return nil
}

func (r *UserSMTPRepository) GetByID(id, userID string) (*models.UserSMTPServer, error) {
	s := &models.UserSMTPServer{}
	err := r.db.QueryRow(`
		SELECT id, user_id, name, host, port, username, password_enc, encryption, from_address, from_name, created_at, updated_at
		FROM user_smtp_servers WHERE id = ? AND user_id = ?`, id, userID,
	).Scan(&s.ID, &s.UserID, &s.Name, &s.Host, &s.Port, &s.Username, &s.PasswordEnc, &s.Encryption, &s.FromAddress, &s.FromName, &s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *UserSMTPRepository) ListByUser(userID string) ([]models.UserSMTPServer, error) {
	rows, err := r.db.Query(`
		SELECT id, user_id, name, host, port, username, password_enc, encryption, from_address, from_name, created_at, updated_at
		FROM user_smtp_servers WHERE user_id = ? ORDER BY name`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.UserSMTPServer{}
	for rows.Next() {
		var s models.UserSMTPServer
		if err := rows.Scan(&s.ID, &s.UserID, &s.Name, &s.Host, &s.Port, &s.Username, &s.PasswordEnc, &s.Encryption, &s.FromAddress, &s.FromName, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *UserSMTPRepository) Delete(id, userID string) error {
	_, err := r.db.Exec(`DELETE FROM user_smtp_servers WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return fmt.Errorf("delete smtp: %w", err)
	}
	return nil
}
