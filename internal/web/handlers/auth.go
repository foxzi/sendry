package handlers

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// LoginPage renders the login page
func (h *Handlers) LoginPage(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{
		"LocalEnabled":  h.cfg.Auth.LocalEnabled,
		"OIDCEnabled":   h.cfg.Auth.OIDC.Enabled,
		"OIDCProvider":  h.cfg.Auth.OIDC.Provider,
	}
	h.render(w, "login", data)
}

// Login handles login form submission
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderLoginError(w, "Invalid form data")
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")

	// Get user from DB
	var userID, passwordHash string
	err := h.db.QueryRow(
		"SELECT id, password_hash FROM users WHERE email = ?",
		email,
	).Scan(&userID, &passwordHash)

	if err != nil {
		h.renderLoginError(w, "Invalid email or password")
		return
	}

	// Check password
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
		h.renderLoginError(w, "Invalid email or password")
		return
	}

	// Create session
	sessionID := uuid.New().String()
	expiresAt := time.Now().Add(h.cfg.Auth.SessionTTL)

	_, err = h.db.Exec(
		"INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)",
		sessionID, userID, expiresAt,
	)
	if err != nil {
		h.logger.Error("failed to create session", "error", err)
		h.renderLoginError(w, "Login failed")
		return
	}

	// Set cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    sessionID,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   h.cfg.Server.TLS.Enabled,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Logout handles user logout
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	// Get session cookie
	cookie, err := r.Cookie("session")
	if err == nil {
		// Delete session from DB
		h.db.Exec("DELETE FROM sessions WHERE id = ?", cookie.Value)
	}

	// Clear cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
	})

	http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
}

// OIDCCallback handles OIDC callback
func (h *Handlers) OIDCCallback(w http.ResponseWriter, r *http.Request) {
	// TODO: implement OIDC callback
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handlers) renderLoginError(w http.ResponseWriter, message string) {
	data := map[string]any{
		"LocalEnabled":  h.cfg.Auth.LocalEnabled,
		"OIDCEnabled":   h.cfg.Auth.OIDC.Enabled,
		"OIDCProvider":  h.cfg.Auth.OIDC.Provider,
		"Error":         message,
	}
	h.render(w, "login", data)
}
