package repository

import (
	"sync"
	"testing"

	"github.com/foxzi/sendry/internal/web/models"
)

func TestTrackingSettings_DefaultsAndRoundTrip(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSettingsRepository(db)

	s, err := repo.GetTrackingSettings()
	if err != nil {
		t.Fatal(err)
	}
	if s.BaseURL != "" || !s.OpensDefault || !s.ClicksDefault || len(s.Secret) != 64 {
		t.Fatalf("defaults = %+v", s)
	}
	secret := s.Secret

	in := &models.TrackingSettings{BaseURL: "https://lnk.example.com", OpensDefault: false, ClicksDefault: true,
		Secret: "ignored"}
	if err := repo.SaveTrackingSettings(in); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetTrackingSettings()
	if err != nil {
		t.Fatal(err)
	}
	if got.BaseURL != in.BaseURL || got.OpensDefault || !got.ClicksDefault {
		t.Errorf("got = %+v", got)
	}
	if got.Secret != secret {
		t.Errorf("secret changed after save: %q -> %q", secret, got.Secret)
	}

	if err := repo.SaveTrackingSettings(&models.TrackingSettings{}); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.GetTrackingSettings()
	if got.OpensDefault || got.ClicksDefault || got.BaseURL != "" {
		t.Errorf("cleared = %+v", got)
	}
}

func TestTrackingSettings_SecretStableConcurrent(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSettingsRepository(db)

	var wg sync.WaitGroup
	secrets := make([]string, 8)
	for i := range secrets {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if s, err := repo.GetTrackingSettings(); err == nil {
				secrets[i] = s.Secret
			}
		}(i)
	}
	wg.Wait()
	for _, s := range secrets {
		if s == "" || s != secrets[0] {
			t.Fatalf("secrets differ: %v", secrets)
		}
	}
}
