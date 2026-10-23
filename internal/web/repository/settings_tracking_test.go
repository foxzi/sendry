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
	if s.BaseURL != "" || len(s.Secret) != 64 {
		t.Fatalf("defaults = %+v", s)
	}
	secret := s.Secret

	in := &models.TrackingSettings{BaseURL: "https://lnk.example.com", Secret: "ignored"}
	if err := repo.SaveTrackingSettings(in); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetTrackingSettings()
	if err != nil {
		t.Fatal(err)
	}
	if got.BaseURL != in.BaseURL {
		t.Errorf("got = %+v", got)
	}
	if got.Secret != secret {
		t.Errorf("secret changed after save: %q -> %q", secret, got.Secret)
	}

	if err := repo.SaveTrackingSettings(&models.TrackingSettings{}); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.GetTrackingSettings()
	if got.BaseURL != "" {
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

func TestTrackingSettings_RotateSecret(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSettingsRepository(db)

	// Rotation works before the secret was ever generated
	if err := repo.RotateTrackingSecret(); err != nil {
		t.Fatal(err)
	}
	s1, err := repo.GetTrackingSettings()
	if err != nil || len(s1.Secret) != 64 || len(s1.PreviousSecret) != 64 || s1.Secret == s1.PreviousSecret {
		t.Fatalf("after first rotation = %+v, %v", s1, err)
	}

	if err := repo.RotateTrackingSecret(); err != nil {
		t.Fatal(err)
	}
	s2, _ := repo.GetTrackingSettings()
	if s2.PreviousSecret != s1.Secret || s2.Secret == s1.Secret || s2.Secret == s1.PreviousSecret {
		t.Errorf("after second rotation: prev=%q (want %q), cur=%q", s2.PreviousSecret, s1.Secret, s2.Secret)
	}

	// Saving the other settings keeps both secrets
	if err := repo.SaveTrackingSettings(&models.TrackingSettings{BaseURL: "https://lnk.example.com"}); err != nil {
		t.Fatal(err)
	}
	if s3, _ := repo.GetTrackingSettings(); s3.Secret != s2.Secret || s3.PreviousSecret != s2.PreviousSecret {
		t.Errorf("save changed secrets: %+v", s3)
	}
}
