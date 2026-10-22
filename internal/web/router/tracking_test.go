package router

import (
	"strings"
	"testing"

	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/tracking"
)

func boolPtr(b bool) *bool { return &b }

func TestEffectiveTracking(t *testing.T) {
	key := &models.APIKey{TrackOpens: true, TrackClicks: false}
	tests := []struct {
		name         string
		req          APISendRequest
		key          *models.APIKey
		opens, click bool
	}{
		{"no key no fields", APISendRequest{}, nil, false, false},
		{"key defaults", APISendRequest{}, key, true, false},
		{"request overrides key", APISendRequest{TrackOpens: boolPtr(false), TrackClicks: boolPtr(true)}, key, false, true},
		{"request without key", APISendRequest{TrackOpens: boolPtr(true)}, nil, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, c := effectiveTracking(&tt.req, tt.key)
			if o != tt.opens || c != tt.click {
				t.Errorf("got %v/%v, want %v/%v", o, c, tt.opens, tt.click)
			}
		})
	}
}

func TestTrackHTML_RawHTML(t *testing.T) {
	signer := tracking.NewSigner([]byte("secret"))
	src := `<html><body><a href="https://shop.example.com/order/1">Order</a></body></html>`

	got := trackHTML(src, "https://lnk.example.com", signer, "send1", true, true)
	if !strings.Contains(got, "https://lnk.example.com/t/o/") {
		t.Errorf("pixel missing: %s", got)
	}
	if !strings.Contains(got, "https://lnk.example.com/t/c/") || strings.Contains(got, "shop.example.com/order/1") {
		t.Errorf("link not rewritten: %s", got)
	}
	if strings.Contains(got, "data-sl") {
		t.Errorf("data-sl leaked: %s", got)
	}

	// Nothing enabled or no send ID: untouched
	if trackHTML(src, "https://lnk.example.com", signer, "send1", false, false) != src {
		t.Error("tracking off must not change HTML")
	}
	if trackHTML(src, "https://lnk.example.com", signer, "", true, true) != src {
		t.Error("empty send ID must not change HTML")
	}

	// Only opens: link stays
	got = trackHTML(src, "https://lnk.example.com", signer, "send1", true, false)
	if !strings.Contains(got, "shop.example.com/order/1") || !strings.Contains(got, "/t/o/") {
		t.Errorf("opens only: %s", got)
	}
}
