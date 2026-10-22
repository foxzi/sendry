package repository

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/foxzi/sendry/internal/web/models"
)

// Keys in the settings table
const (
	SettingTrackingBaseURL       = "tracking.base_url"
	SettingTrackingOpensDefault  = "tracking.opens_default"
	SettingTrackingClicksDefault = "tracking.clicks_default"
	SettingTrackingSecret        = "tracking.secret" // hex
	// SettingTrackingPrevSecret is the secret before the last rotation: tokens
	// signed with it are still accepted so already sent mail keeps working.
	SettingTrackingPrevSecret = "tracking.secret_prev" // hex
)

// GetTrackingSettings reads tracking settings from the settings table.
// Defaults when absent: BaseURL "", OpensDefault true, ClicksDefault true.
// If no secret exists yet, a random 32-byte secret is
// generated, stored and returned (must be safe under concurrent first calls:
// use INSERT OR IGNORE then read back).
func (r *SettingsRepository) GetTrackingSettings() (*models.TrackingSettings, error) {
	s := &models.TrackingSettings{OpensDefault: true, ClicksDefault: true}

	var err error
	if s.BaseURL, err = r.GetSetting(SettingTrackingBaseURL); err != nil {
		return nil, err
	}
	if v, err := r.GetSetting(SettingTrackingOpensDefault); err != nil {
		return nil, err
	} else if v != "" {
		s.OpensDefault = v == "1"
	}
	if v, err := r.GetSetting(SettingTrackingClicksDefault); err != nil {
		return nil, err
	} else if v != "" {
		s.ClicksDefault = v == "1"
	}

	secret, err := r.GetSetting(SettingTrackingSecret)
	if err != nil {
		return nil, err
	}
	if secret == "" {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			return nil, fmt.Errorf("failed to generate tracking secret: %w", err)
		}
		// The first writer wins; everyone then reads the stored value
		if _, err := r.db.Exec(
			"INSERT OR IGNORE INTO settings (key, value, updated_at) VALUES (?, ?, ?)",
			SettingTrackingSecret, hex.EncodeToString(buf), time.Now().UTC(),
		); err != nil {
			return nil, err
		}
		if secret, err = r.GetSetting(SettingTrackingSecret); err != nil {
			return nil, err
		}
	}
	s.Secret = secret
	if s.PreviousSecret, err = r.GetSetting(SettingTrackingPrevSecret); err != nil {
		return nil, err
	}
	return s, nil
}

// RotateTrackingSecret generates a new token secret and keeps the current one
// as the previous secret (the one before it is dropped, so links signed with
// it stop working).
func (r *SettingsRepository) RotateTrackingSecret() error {
	cur, err := r.GetTrackingSettings() // makes sure a current secret exists
	if err != nil {
		return err
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Errorf("failed to generate tracking secret: %w", err)
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	for _, kv := range [][2]string{
		{SettingTrackingPrevSecret, cur.Secret},
		{SettingTrackingSecret, hex.EncodeToString(buf)},
	} {
		if _, err := tx.Exec(`
			INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
			kv[0], kv[1], now,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SaveTrackingSettings stores everything except the secrets.
func (r *SettingsRepository) SaveTrackingSettings(s *models.TrackingSettings) error {
	flag := func(b bool) string {
		if b {
			return "1"
		}
		return "0"
	}
	for _, kv := range [][2]string{
		{SettingTrackingBaseURL, s.BaseURL},
		{SettingTrackingOpensDefault, flag(s.OpensDefault)},
		{SettingTrackingClicksDefault, flag(s.ClicksDefault)},
	} {
		if err := r.SetSetting(kv[0], kv[1]); err != nil {
			return err
		}
	}
	return nil
}
