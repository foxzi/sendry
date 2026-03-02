package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetUserEmail(t *testing.T) {
	tests := []struct {
		name      string
		email     string
		wantEmail string
	}{
		{"with email in context", "user@example.com", "user@example.com"},
		{"with empty email", "", ""},
		{"no email in context", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			if tt.email != "" {
				ctx := context.WithValue(r.Context(), ctxKeyUserEmail, tt.email)
				r = r.WithContext(ctx)
			}
			got := GetUserEmail(r)
			if got != tt.wantEmail {
				t.Errorf("GetUserEmail() = %q, want %q", got, tt.wantEmail)
			}
		})
	}
}

func TestGetUserEmail_WrongType(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	ctx := context.WithValue(r.Context(), ctxKeyUserEmail, 12345)
	r = r.WithContext(ctx)
	got := GetUserEmail(r)
	if got != "" {
		t.Errorf("GetUserEmail() with wrong type = %q, want empty", got)
	}
}

func TestGetAPIKeyFromContext_Nil(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	got := GetAPIKeyFromContext(r)
	if got != nil {
		t.Errorf("GetAPIKeyFromContext() without key = %v, want nil", got)
	}
}

func TestMethodOverride(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		formMethod string
		wantMethod string
	}{
		{"POST with DELETE override", "POST", "DELETE", "DELETE"},
		{"POST with PUT override", "POST", "PUT", "PUT"},
		{"POST without override", "POST", "", "POST"},
		{"GET ignores override", "GET", "DELETE", "GET"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotMethod string
			handler := MethodOverride(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
			}))

			r := httptest.NewRequest(tt.method, "/test", nil)
			if tt.formMethod != "" {
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				r.Form = map[string][]string{"_method": {tt.formMethod}}
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)

			if gotMethod != tt.wantMethod {
				t.Errorf("MethodOverride() method = %q, want %q", gotMethod, tt.wantMethod)
			}
		})
	}
}
