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
	return s, nil
}

// SaveTrackingSettings stores everything except Secret.
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
